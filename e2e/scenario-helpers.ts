import { randomUUID } from 'crypto';
import { readFileSync, writeFileSync } from 'fs';
import { join } from 'path';
import { runDataDir } from './run-dir';
import { SMTPServer } from 'smtp-server';
import { ADMIN_EMAIL, ADMIN_PASSWORD } from './helpers';
import { STAGE_ORDER, type StageName } from './scenario-registry';

export const PLAYER_PASSWORD = 'TestPass123456';

// Prints how to open a scenario page as a player: the credentials, the page,
// and a dev-only one-click link (/dev-login signs in and goes to the page).
export function printLogin(baseURL: string, email: string, path: string): void {
  const link = `${baseURL}/dev-login?${new URLSearchParams({ email, password: PLAYER_PASSWORD, next: path })}`;
  console.log(`\nLog in as ${email} / ${PLAYER_PASSWORD}`);
  console.log(`Match page: ${baseURL}${path}`);
  console.log(`One click:  ${link}`);
}

// Prints labeled one-click links, each signing in as the given user first.
export function printLinks(baseURL: string, links: Array<{ label: string; email: string; password: string; path: string }>): void {
  for (const l of links) {
    const url = `${baseURL}/dev-login?${new URLSearchParams({ email: l.email, password: l.password, next: l.path })}`;
    console.log(`${l.label} (${l.email}): ${url}`);
  }
}

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
      // Odd players have a phone (WhatsApp link), even ones email only.
      phone: i % 2 ? `+34600000${String(i).padStart(3, '0')}` : '',
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
  if (stageName === 'blank') return { ...emptyCtx(api), stage: 'blank' };
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

export const MAILPIT_SMTP_PORT = 1025;
export const MAILPIT_URL = 'http://localhost:8025';
// Node resolves localhost to ::1 first; Mailpit listens on IPv4 only.
export const MAILPIT_API = 'http://127.0.0.1:8025';

// Points the server's SMTP at a local Mailpit (`make mail`).
export async function enableMailpit(api: ScenarioApi): Promise<void> {
  await enableSmtp(api, MAILPIT_SMTP_PORT);
}

// Recipient addresses (To) of every message Mailpit holds, via its REST API.
export async function mailpitRecipients(): Promise<string[]> {
  const resp = await fetch(`${MAILPIT_API}/api/v1/messages?limit=500`);
  if (!resp.ok) throw new Error(`Mailpit /api/v1/messages: ${resp.status}`);
  const body = (await resp.json()) as { messages: Array<{ To: Array<{ Address: string }> }> };
  return body.messages.flatMap((m) => m.To.map((t) => t.Address));
}

export async function mailpitReachable(): Promise<boolean> {
  try {
    return (await fetch(`${MAILPIT_API}/api/v1/info`)).ok;
  } catch {
    return false;
  }
}

export type ChatSchedule = 'none' | 'proposed' | 'confirmed';

export interface ChatMatch {
  competitionId: string;
  competitionName: string;
  matchID: string;
  players: Array<{ id: string; email: string }>;
  pairs: Array<{ id: string; name: string; player1Idx: number; player2Idx: number }>;
  // pair1 / pair2 of the match, as seeded (p01 and p02 may be on either side).
  pair1: { id: string; name: string; player1Idx: number; player2Idx: number };
  pair2: { id: string; name: string; player1Idx: number; player2Idx: number };
}

async function playerToken(baseURL: string, email: string): Promise<string> {
  const resp = await fetch(`${baseURL}/api/collections/users/auth-with-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identity: email, password: PLAYER_PASSWORD }),
  });
  if (!resp.ok) throw new Error(`login ${email}: ${resp.status}`);
  return (await resp.json()).token;
}

// playerPost submits a form to an app route as a player, like the browser does.
export async function playerPost(api: ScenarioApi, email: string, path: string, form: Record<string, string>): Promise<void> {
  const resp = await fetch(`${api.baseURL}${path}`, {
    method: 'POST',
    headers: {
      Cookie: `pb_auth=${await playerToken(api.baseURL, email)}`,
      'HX-Request': 'true',
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: new URLSearchParams(form).toString(),
    redirect: 'manual',
  });
  if (resp.status >= 400) throw new Error(`POST ${path}: ${resp.status} ${await resp.text()}`);
}

// seedChatMatch builds one competition with one match between two pairs of
// players (p01 + p02 against p03 + p04; p04 is unverified, so gets no email)
// and, through the real routes, leaves its date in the given state.
export async function seedChatMatch(api: ScenarioApi, title: string, schedule: ChatSchedule): Promise<ChatMatch> {
  const suffix = uniqueSuffix();
  const players = await createPlayers(api, 4, suffix);
  await apiPatch(api, `/api/collections/users/records/${players[3].id}`, { verified: false });
  const pairs = await createPairs(api, players, suffix);
  const competitionName = `${title} ${suffix}`;
  const competitionId = await createCompetition(api, competitionName, 0, 0);
  for (const pair of pairs) await addPairToCompetition(api, competitionId, pair.id);
  await generateAssignments(api, competitionId);
  await publishCalendar(api, competitionId);
  const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
    `competition='${competitionId}'`)}&perPage=5`);
  if (list.totalItems !== 1) throw new Error(`expected one match, got ${list.totalItems}`);
  const match = list.items[0];
  const pairOf = (id: string) => pairs.find((p) => p.id === id)!;
  const pair1 = pairOf(match.pair1);
  const pair2 = pairOf(match.pair2);

  if (schedule !== 'none') {
    await playerPost(api, players[pair1.player1Idx].email, `/match/${match.id}/thread/proposal`,
      { date: '2027-09-15', time: '18:30', venue_text: 'Padel 360' });
    const pending = await apiGet(api, `/api/collections/match_messages/records?filter=${encodeURIComponent(
      `match='${match.id}' && type='scheduling_proposal' && proposal_status='pending'`)}&perPage=5`);
    if (pending.totalItems !== 1) throw new Error(`expected one pending proposal, got ${pending.totalItems}`);
    if (schedule === 'confirmed') {
      await playerPost(api, players[pair2.player1Idx].email,
        `/match/${match.id}/thread/proposal/${pending.items[0].id}/respond`, { action: 'accept' });
    }
  }
  return { competitionId, competitionName, matchID: match.id, players, pairs, pair1, pair2 };
}

// printChatMatch lists the seeded match for a person to open: page, inbox,
// players and one-click sign-in links.
export function printChatMatch(baseURL: string, m: ChatMatch): void {
  const matchPath = `/match/${m.matchID}`;
  console.log(`\nMatch page: ${baseURL}${matchPath}`);
  console.log(`Mailpit inbox: ${MAILPIT_URL} (start it with: make mail)`);
  console.log(`Match: ${m.pair1.name} vs ${m.pair2.name}`);
  for (const pair of m.pairs) {
    for (const idx of [pair.player1Idx, pair.player2Idx]) {
      console.log(`  ${pair.name}  ${m.players[idx].email}  password ${PLAYER_PASSWORD}  verified ${idx === 3 ? 'no' : 'yes'}`);
    }
  }
  printLinks(baseURL, m.pairs.flatMap((pair) => [pair.player1Idx, pair.player2Idx].map((idx) => ({
    label: `Sign in as ${pair.name} / ${m.players[idx].email}`,
    email: m.players[idx].email, password: PLAYER_PASSWORD, path: matchPath,
  }))));
}

// htmlOf returns the decoded text/html part of a captured MIME message.
export function htmlOf(raw: string): string {
  const parts = raw.split(/\r?\n--[^\r\n]+/);
  const part = parts.find((p) => /content-type:\s*text\/html/i.test(p));
  if (!part) throw new Error('no text/html part in message');
  const [head, ...rest] = part.split(/\r?\n\r?\n/);
  const body = rest.join('\n\n');
  if (/content-transfer-encoding:\s*base64/i.test(head)) return Buffer.from(body, 'base64').toString('utf-8');
  if (/content-transfer-encoding:\s*quoted-printable/i.test(head)) {
    const bytes = body.replace(/=\r?\n/g, '').replace(/=([0-9A-F]{2})/gi, (_, h) => String.fromCharCode(parseInt(h, 16)));
    return Buffer.from(bytes, 'latin1').toString('utf-8');
  }
  return body;
}

// adminCookie logs the test admin in through the HTML form and returns the
// session cookie header value.
export async function adminCookie(baseURL: string): Promise<string> {
  const resp = await fetch(`${baseURL}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: `email=${ADMIN_EMAIL}&password=${ADMIN_PASSWORD}`,
    redirect: 'manual',
  });
  return (resp.headers.getSetCookie?.() || []).join('; ');
}
