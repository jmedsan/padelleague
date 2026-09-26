import { randomUUID } from 'crypto';
import { readFileSync, writeFileSync } from 'fs';
import { join } from 'path';
import { runDataDir } from './run-dir';
import { SMTPServer } from 'smtp-server';
import { STAGE_ORDER, type StageName } from './scenario-registry';

export const PLAYER_PASSWORD = 'TestPass123456';

export function uniqueSuffix(): string {
  return randomUUID().slice(0, 8);
}

export interface ScenarioApi {
  baseURL: string;
  suToken: string;
  adminCookie: string;
}

export interface ScenarioCtx {
  api: ScenarioApi;
  competitionId: string;
  players: Array<{ id: string; email: string }>;
  pairs: Array<{ id: string; name: string; player1Idx: number; player2Idx: number }>;
  target: number;
  open: number;
  stage: string;
}

// API helpers — all throw on non-2xx responses.

export async function apiGet(api: ScenarioApi, path: string): Promise<any> {
  const resp = await fetch(`${api.baseURL}${path}`, {
    headers: { Authorization: api.suToken },
  });
  if (!resp.ok) throw new Error(`GET ${path}: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

export async function apiPost(api: ScenarioApi, path: string, data: any): Promise<any> {
  const resp = await fetch(`${api.baseURL}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: api.suToken },
    body: JSON.stringify(data),
  });
  if (!resp.ok) throw new Error(`POST ${path}: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

export async function apiPatch(api: ScenarioApi, path: string, data: any): Promise<any> {
  const resp = await fetch(`${api.baseURL}${path}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', Authorization: api.suToken },
    body: JSON.stringify(data),
  });
  if (!resp.ok) throw new Error(`PATCH ${path}: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

export async function apiDelete(api: ScenarioApi, path: string): Promise<void> {
  const resp = await fetch(`${api.baseURL}${path}`, {
    method: 'DELETE',
    headers: { Authorization: api.suToken },
  });
  if (!resp.ok) throw new Error(`DELETE ${path}: ${resp.status} ${await resp.text()}`);
}

// formPost sends a POST through the app's HTML endpoints using admin cookie auth.
export async function formPost(api: ScenarioApi, path: string): Promise<void> {
  const resp = await fetch(`${api.baseURL}${path}`, {
    method: 'POST',
    headers: { Cookie: api.adminCookie, 'HX-Request': 'true' },
    redirect: 'manual',
  });
  if (resp.status >= 400) throw new Error(`POST ${path}: ${resp.status}`);
}

// formPostData sends a form-urlencoded POST through the app's HTML endpoints
// using admin cookie auth — for handlers that read e.Request.FormValue(...),
// as opposed to formPost's empty body (button-only actions) or apiPatch's
// JSON body (the /api/collections/* REST layer, which parses fields
// differently than a real HTML form submission — see setDates in
// tour-helpers.ts for the field values a real form actually sends).
export async function formPostData(
  api: ScenarioApi,
  path: string,
  data: Record<string, string>,
): Promise<void> {
  const resp = await fetch(`${api.baseURL}${path}`, {
    method: 'POST',
    headers: {
      Cookie: api.adminCookie,
      'HX-Request': 'true',
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: new URLSearchParams(data).toString(),
    redirect: 'manual',
  });
  if (resp.status >= 400) throw new Error(`POST ${path}: ${resp.status} ${await resp.text()}`);
}

// Domain helpers

export async function createPlayers(
  api: ScenarioApi,
  count: number,
  suffix: string,
): Promise<Array<{ id: string; email: string }>> {
  const players: Array<{ id: string; email: string }> = [];
  for (let i = 1; i <= count; i++) {
    const email = `p${String(i).padStart(2, '0')}-${suffix}@test.local`;
    const record = await apiPost(api, '/api/collections/users/records', {
      email,
      password: PLAYER_PASSWORD,
      passwordConfirm: PLAYER_PASSWORD,
      display_name: `Player ${i}`,
      roles: ['player'],
      verified: true,
      gender: 'male',
    });
    players.push({ id: record.id, email });
  }
  return players;
}

// PAIR_LEVELS distributes 16 pairs across skill levels (strongest first) so
// leveled scenarios demonstrate skill-based matchmaking instead of a flat
// "everyone unranked" fixture: 2 advanced, 4 intermediate variants, 6
// beginner variants, 4 left unranked (no level field set — matches how a
// real pair looks before the admin classifies it).
const PAIR_LEVELS = [
  'advanced', 'advanced',
  'intermediate_high', 'intermediate', 'intermediate', 'intermediate_low',
  'beginner_high', 'beginner_high', 'beginner', 'beginner', 'beginner', 'beginner',
  '', '', '', '',
];

export async function createPairs(
  api: ScenarioApi,
  players: Array<{ id: string; email: string }>,
  suffix: string,
): Promise<Array<{ id: string; name: string; player1Idx: number; player2Idx: number }>> {
  const pairs: Array<{ id: string; name: string; player1Idx: number; player2Idx: number }> = [];
  for (let i = 0; i < players.length; i += 2) {
    const player1Idx = i;
    const player2Idx = i + 1;
    const name = `Pareja ${pairs.length + 1}-${suffix}`;
    const level = PAIR_LEVELS[pairs.length % PAIR_LEVELS.length];
    const body: Record<string, unknown> = {
      name,
      player1: players[player1Idx].id,
      player2: players[player2Idx].id,
      captain: players[player1Idx].id,
    };
    if (level) body.level = level;
    const record = await apiPost(api, '/api/collections/pairs/records', body);
    pairs.push({ id: record.id, name, player1Idx, player2Idx });
  }
  return pairs;
}

export async function createCompetition(
  api: ScenarioApi,
  name: string,
  target: number,
  open: number,
): Promise<string> {
  const record = await apiPost(api, '/api/collections/competitions/records', {
    name,
    type: 'league',
    active: true,
    target_matches: target,
    open_assignments: open,
    // CompetitionHandler.Create always defaults these two on the live path
    // (applyCompIdentity, setSchedulingFields) — match it here so the
    // scenario's admin activity log doesn't show a spurious change on the
    // first edit that touches neither field.
    gender_type: 'free',
    walkover_score: '6-0 6-0',
  });
  return record.id;
}

// 29/09/2036–10/12/2036 is a deliberately hard season: 73 inclusive days
// over target_matches=10 does not divide evenly (7.3 days/Jornada), so it
// exercises league.JornadaWindow's whole-day ceiling-division partition
// instead of a round number that would pass even with a fractional-day bug.
//
// Date-only strings (no time-of-day), matching exactly what the admin edit
// form's flatpickr inputs submit (dateFormat: 'Y-m-d' — views/layout.html).
// PocketBase's DateTime parses a bare "YYYY-MM-DD" as that day at midnight
// UTC, not noon: stageDated posts these through the real Update handler
// (formPostData) rather than an ISO timestamp via the JSON API, so the
// scenario's stored start_date/end_date time-of-day matches the live path
// exactly — load-bearing here since JornadaWindow's returned lo/hi carry
// start's time-of-day.
export function competitionDates(): { startDate: string; endDate: string } {
  return {
    startDate: '2036-09-29',
    endDate: '2036-12-10',
  };
}

const SPANISH_MONTHS = [
  'enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio',
  'julio', 'agosto', 'septiembre', 'octubre', 'noviembre', 'diciembre',
];

function fmtShortDate(d: Date): string {
  return `${d.getUTCDate()} ${SPANISH_MONTHS[d.getUTCMonth()]}`;
}

const MS_PER_DAY = 24 * 60 * 60 * 1000;

function ceilDiv(a: number, b: number): number {
  return Math.ceil(a / b);
}

// jornadaTitle mirrors league.JornadaWindow's whole-day ceiling-division
// partition (league/scheduling.go jornadaDayRange): the D = end-start+1
// inclusive days are split into `target` whole-day Jornadas via
// ceil((n-1)*D/target)..ceil(n*D/target)-1 day offsets from start, so
// every boundary lands on a whole calendar day even when D isn't a
// multiple of target.
export function jornadaTitle(startISO: string, endISO: string, target: number, n: number): string {
  const { lo, hi } = jornadaRange(startISO, endISO, target, n);
  return `Jornada ${n} · ${fmtShortDate(lo)} – ${fmtShortDate(hi)}`;
}

// jornadaRange returns the same [lo, hi] Date pair jornadaTitle formats for
// display — exposed separately so callers that need the exact day (e.g.
// asserting a match's arrange_by, stored as YYYY-MM-DD) don't have to parse
// jornadaTitle's Spanish short-date string back into a date.
export function jornadaRange(startISO: string, endISO: string, target: number, n: number): { lo: Date; hi: Date } {
  const start = new Date(startISO);
  const end = new Date(endISO);
  const days = Math.round((end.getTime() - start.getTime()) / MS_PER_DAY) + 1;
  const loDay = ceilDiv((n - 1) * days, target);
  const hiDay = ceilDiv(n * days, target) - 1;
  return {
    lo: new Date(start.getTime() + loDay * MS_PER_DAY),
    hi: new Date(start.getTime() + hiDay * MS_PER_DAY),
  };
}

// jornadaHiISO returns Jornada n's last day as a YYYY-MM-DD string, matching
// the format league.SlotDeadline stores on a match's arrange_by field.
export function jornadaHiISO(startISO: string, endISO: string, target: number, n: number): string {
  return jornadaRange(startISO, endISO, target, n).hi.toISOString().slice(0, 10);
}

export async function addPairToCompetition(
  api: ScenarioApi,
  compId: string,
  pairId: string,
): Promise<void> {
  // Fetch current pairs list first — PATCH with full array (PocketBase replaces, not appends).
  const comp = await apiGet(api, `/api/collections/competitions/records/${compId}`);
  const pairs: string[] = comp.pairs || [];
  pairs.push(pairId);
  await apiPatch(api, `/api/collections/competitions/records/${compId}`, { pairs });
}

export async function generateAssignments(api: ScenarioApi, compId: string): Promise<void> {
  await formPost(api, `/admin/competitions/${compId}/generate`);
}

export async function publishCalendar(api: ScenarioApi, compId: string): Promise<void> {
  await formPost(api, `/admin/competitions/${compId}/publish`);
}

// assertAssignmentInvariants checks the four core leveled-league invariants for
// every match in the competition.
export async function assertAssignmentInvariants(
  api: ScenarioApi,
  ctx: ScenarioCtx,
): Promise<void> {
  const data = await apiGet(
    api,
    `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`,
  );
  const matches: any[] = data.items;

  // 1. No self-match
  for (const m of matches) {
    if (m.pair1 === m.pair2) {
      throw new Error(`Self-match detected: match ${m.id} has pair1 === pair2 (${m.pair1})`);
    }
  }

  // 2. No duplicate pairing
  const seen = new Set<string>();
  for (const m of matches) {
    const key = [m.pair1, m.pair2].sort().join(':');
    if (seen.has(key)) {
      throw new Error(`Duplicate pairing: ${key} appears more than once`);
    }
    seen.add(key);
  }

  // 3. Pending per pair <= open_assignments + 1.
  // The backend's eligible() check allows a pair at exactly open_assignments to
  // still receive one more opponent assignment during a batch run, so the actual
  // ceiling is open+1. leveled_test.go line 127 asserts this explicitly.
  const pendingCount = new Map<string, number>();
  for (const m of matches) {
    if (m.status === 'pending') {
      pendingCount.set(m.pair1, (pendingCount.get(m.pair1) ?? 0) + 1);
      pendingCount.set(m.pair2, (pendingCount.get(m.pair2) ?? 0) + 1);
    }
  }
  for (const [pairId, count] of pendingCount) {
    if (count > ctx.open + 1) {
      throw new Error(`Pair ${pairId} has ${count} pending matches, exceeds open_assignments+1 (${ctx.open + 1})`);
    }
  }

  // 4. Total load per pair <= target_matches
  const loadCount = new Map<string, number>();
  for (const m of matches) {
    if (m.status === 'pending' || m.status === 'scheduled' || m.status === 'final') {
      loadCount.set(m.pair1, (loadCount.get(m.pair1) ?? 0) + 1);
      loadCount.set(m.pair2, (loadCount.get(m.pair2) ?? 0) + 1);
    }
  }
  for (const [pairId, count] of loadCount) {
    if (count > ctx.target) {
      throw new Error(`Pair ${pairId} has load ${count}, exceeds target_matches ${ctx.target}`);
    }
  }
}

// Stage functions — each advances the DB state and returns updated ctx.

type Stage = (api: ScenarioApi, ctx: ScenarioCtx) => Promise<ScenarioCtx>;

async function stageCreated(api: ScenarioApi, ctx: ScenarioCtx): Promise<ScenarioCtx> {
  const suffix = uniqueSuffix();
  const target = 10;
  const open = 3;
  const players = await createPlayers(api, 32, suffix);
  const pairs = await createPairs(api, players, suffix);
  const competitionId = await createCompetition(api, `Leveled-16-${suffix}`, target, open);
  for (const pair of pairs) {
    await addPairToCompetition(api, competitionId, pair.id);
  }
  return { ...ctx, competitionId, players, pairs, target, open, stage: 'created' };
}

async function stageDated(api: ScenarioApi, ctx: ScenarioCtx): Promise<ScenarioCtx> {
  const { startDate, endDate } = competitionDates();
  // Post through the real admin Update handler (POST /admin/competitions/:id)
  // instead of a raw PATCH to the JSON API, so start_date/end_date get
  // stored exactly as the live edit form stores them (see competitionDates'
  // comment). The edit form (views/admin/competition-detail.html) is
  // pre-filled with every field's CURRENT value and always submits the
  // full set — Update's handler treats an absent field as "reset to
  // handler default" for several of these (target_matches/open_assignments
  // in particular, since hasFixtures is false pre-generation), so posting
  // only start_date/end_date would silently zero the leveled-league fields.
  // Fetch the record first and echo every field back exactly as the
  // browser form would.
  const comp = await apiGet(api, `/api/collections/competitions/records/${ctx.competitionId}`);
  const body: Record<string, string> = {
    name: comp.name,
    type: comp.type,
    gender_type: comp.gender_type,
    quorum_timeout_hours: String(comp.quorum_timeout_hours ?? 0),
    start_date: startDate,
    end_date: endDate,
    arrange_grace_days: String(comp.arrange_grace_days || 3),
    walkover_score: comp.walkover_score || '6-0 6-0',
    default_penalty: String(comp.default_penalty || 3),
    recovery_days: String(comp.recovery_days || 14),
    max_pending_matches: String(comp.max_pending_matches ?? 0),
    match_reminder_hours: (comp.match_reminder_hours ?? []).join(', '),
    target_matches: String(comp.target_matches ?? 0),
    open_assignments: String(comp.open_assignments ?? 0),
  };
  if (comp.play_twice) body.play_twice = 'on'; // checkbox: present+"on" when checked, absent when not
  await formPostData(api, `/admin/competitions/${ctx.competitionId}`, body);
  return { ...ctx, stage: 'dated' };
}

async function stageAssigned(api: ScenarioApi, ctx: ScenarioCtx): Promise<ScenarioCtx> {
  await generateAssignments(api, ctx.competitionId);
  await publishCalendar(api, ctx.competitionId);
  return { ...ctx, stage: 'assigned' };
}

async function stageMidSeason(api: ScenarioApi, ctx: ScenarioCtx): Promise<ScenarioCtx> {
  return { ...ctx, stage: 'mid' };
}

async function stageEndSeason(api: ScenarioApi, ctx: ScenarioCtx): Promise<ScenarioCtx> {
  return { ...ctx, stage: 'end' };
}

const STAGES: Record<string, Stage> = {
  created: stageCreated,
  dated: stageDated,
  assigned: stageAssigned,
  mid: stageMidSeason,
  end: stageEndSeason,
};

function emptyCtx(api: ScenarioApi): ScenarioCtx {
  return {
    api,
    competitionId: '',
    players: [],
    pairs: [],
    target: 0,
    open: 0,
    stage: '',
  };
}

async function advanceToStage(api: ScenarioApi, ctx: ScenarioCtx, stageName: string): Promise<ScenarioCtx> {
  const current = STAGE_ORDER.indexOf(ctx.stage as StageName);
  const target = STAGE_ORDER.indexOf(stageName as StageName);
  if (target === -1) throw new Error(`Unknown stage: ${stageName}`);
  let sc = ctx;
  for (let i = Math.max(current + 1, 0); i <= target; i++) {
    sc = await STAGES[STAGE_ORDER[i]](api, sc);
    await assertAssignmentInvariants(api, sc);
  }
  return sc;
}

export async function buildToStage(api: ScenarioApi, stageName: string): Promise<ScenarioCtx> {
  return advanceToStage(api, emptyCtx(api), stageName);
}

const SCENARIO_JSON = join(runDataDir(8098), 'scenario.json');

export interface ScenarioData extends ScenarioCtx {
  baseURL: string;
  suToken: string;
  adminCookie: string;
}

export function loadCtx(): ScenarioData {
  return JSON.parse(readFileSync(SCENARIO_JSON, 'utf-8'));
}

export function saveCtx(ctx: ScenarioData): void {
  const { api: _, ...rest } = ctx as any;
  writeFileSync(SCENARIO_JSON, JSON.stringify(rest, null, 2));
}

export async function ensureStage(api: ScenarioApi, stage: StageName): Promise<ScenarioData> {
  const ctx = loadCtx();
  const current = STAGE_ORDER.indexOf(ctx.stage as StageName);
  const target = STAGE_ORDER.indexOf(stage);
  if (current >= target) return ctx;
  const sc = await advanceToStage(api, { ...ctx, api }, stage);
  const { api: _, ...rest } = sc as any;
  const updated: ScenarioData = { ...ctx, ...rest, stage: sc.stage };
  saveCtx(updated);
  return updated;
}

export function requireStage(ctx: ScenarioData, stage: StageName): void {
  if (ctx.stage !== stage) {
    throw new Error(
      `Scenario must start at stage '${stage}' (got '${ctx.stage}'). ` +
      `Run: make e2e-scenario SCENARIO=leveled-16-pregen`,
    );
  }
}

// SMTP sink — captures outbound emails for assertion in tests.

export interface SmtpSink {
  port: number;
  messages: Array<{ from: string; to: string[]; data: string }>;
  close(): Promise<void>;
}

export async function startSmtpSink(): Promise<SmtpSink> {
  const messages: SmtpSink['messages'] = [];
  const server = new SMTPServer({
    authOptional: true,
    disabledCommands: ['AUTH', 'STARTTLS'],  // plain-text only; no TLS negotiation
    onData(stream, session, callback) {
      let data = '';
      stream.on('data', (chunk: Buffer) => { data += chunk; });
      stream.on('end', () => {
        messages.push({
          from: session.envelope.mailFrom ? session.envelope.mailFrom.address : '',
          to: session.envelope.rcptTo.map((r: any) => r.address),
          data,
        });
        callback();
      });
    },
  });
  const port = await new Promise<number>((resolve) => {
    server.listen(0, () => resolve((server.server.address() as any).port));
  });
  return {
    port,
    messages,
    close: () => new Promise<void>((resolve) => server.close(() => resolve())),
  };
}

export async function enableSmtp(api: ScenarioApi, sinkPort: number): Promise<void> {
  await apiPatch(api, '/api/settings', {
    smtp: { enabled: true, host: '127.0.0.1', port: sinkPort, tls: false },
  });
}

export async function disableSmtp(api: ScenarioApi): Promise<void> {
  await apiPatch(api, '/api/settings', { smtp: { enabled: false } });
}
