import { randomUUID } from 'crypto';

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
    const record = await apiPost(api, '/api/collections/pairs/records', {
      name,
      player1: players[player1Idx].id,
      player2: players[player2Idx].id,
      captain: players[player1Idx].id,
    });
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
  });
  return record.id;
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

export async function playMatchFastForward(
  api: ScenarioApi,
  matchId: string,
  score: string,
  winnerId: string,
): Promise<void> {
  // Date and club are required before a final result can be set.
  await apiPatch(api, `/api/collections/matches/records/${matchId}`, {
    date: '2025-06-01T12:00:00.000Z',
    club: 'Test Club',
  });
  // Setting status=final fires the OnRecordAfterUpdateSuccess hook → TopUpAssignments.
  await apiPatch(api, `/api/collections/matches/records/${matchId}`, {
    status: 'final',
    scores: score,
    winner: winnerId,
  });
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

  // 3. Pending per pair <= open_assignments
  const pendingCount = new Map<string, number>();
  for (const m of matches) {
    if (m.status === 'pending') {
      pendingCount.set(m.pair1, (pendingCount.get(m.pair1) ?? 0) + 1);
      pendingCount.set(m.pair2, (pendingCount.get(m.pair2) ?? 0) + 1);
    }
  }
  for (const [pairId, count] of pendingCount) {
    if (count > ctx.open) {
      throw new Error(`Pair ${pairId} has ${count} pending matches, exceeds open_assignments ${ctx.open}`);
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

const STAGE_ORDER = ['created', 'assigned', 'mid', 'end'] as const;

async function stageCreated(api: ScenarioApi, ctx: ScenarioCtx): Promise<ScenarioCtx> {
  const suffix = uniqueSuffix();
  const target = 6;
  const open = 3;
  const players = await createPlayers(api, 32, suffix);
  const pairs = await createPairs(api, players, suffix);
  const competitionId = await createCompetition(api, `Leveled-16-${suffix}`, target, open);
  for (const pair of pairs) {
    await addPairToCompetition(api, competitionId, pair.id);
  }
  return { ...ctx, competitionId, players, pairs, target, open, stage: 'created' };
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

export async function buildToStage(api: ScenarioApi, stageName: string): Promise<ScenarioCtx> {
  const target = STAGE_ORDER.indexOf(stageName as typeof STAGE_ORDER[number]);
  if (target === -1) throw new Error(`Unknown stage: ${stageName}`);
  let ctx = emptyCtx(api);
  for (let i = 0; i <= target; i++) {
    ctx = await STAGES[STAGE_ORDER[i]](api, ctx);
    await assertAssignmentInvariants(api, ctx);
  }
  return ctx;
}
