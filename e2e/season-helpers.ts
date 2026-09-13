import { APIRequestContext } from '@playwright/test';
import { randomUUID } from 'crypto';

export type PairId = 'A' | 'B' | 'C' | 'D';

export interface ExpectedRow {
  pair: PairId;
  position: number;
  played: number;
  wins: number;
  losses: number;
  setsWon: number;
  setsLost: number;
  gamesWon: number;
  gamesLost: number;
  penalty: number;
  points: number;
}

export interface PlannedMatch {
  home: PairId;
  away: PairId;
  score: string;
}

// Win matrix: A-B 1-1, A-C 1-1, A-D 1-1, B-C 2-0, B-D 0-2, C-D 2-0.
// Every pair finishes 3-3 (9 pts before penalty).
// Order without penalty: A>B (set diff), B>C (game diff), C>D (H2H).
//
// Set formats — A always wins 2-0 and loses 1-2; the B/C/D internal ring
// (B→C, D→B, C→D) is all 2-0.  Game margins tuned so that
// B game-diff (+1) > C game-diff (-5) = D game-diff (-5).
//
// Fixtures listed in RoundRobin generation order for 4 pairs [A,B,C,D]
// with double=true (rounds 1-3 first leg, 4-6 return leg swapped).
export const SCORE_MATRIX: PlannedMatch[] = [
  { home: 'A', away: 'D', score: '6-3 6-3' },       // 0:  A wins 2-0
  { home: 'B', away: 'C', score: '6-2 6-2' },       // 1:  B wins 2-0
  { home: 'A', away: 'C', score: '6-3 6-3' },       // 2:  A wins 2-0
  { home: 'D', away: 'B', score: '6-4 6-4' },       // 3:  D wins 2-0
  { home: 'A', away: 'B', score: '6-3 4-1' },       // 4:  A wins via rule (1 set + 3-game lead)
  { home: 'C', away: 'D', score: '6-3 6-4' },       // 5:  C wins 2-0
  { home: 'D', away: 'A', score: '4-6 6-3 6-4' },   // 6:  D wins 2-1
  { home: 'C', away: 'B', score: '4-6 4-6' },       // 7:  B wins 2-0
  { home: 'C', away: 'A', score: '4-6 6-3 6-4' },   // 8:  C wins 2-1
  { home: 'B', away: 'D', score: '4-6 4-6' },       // 9:  D wins 2-0
  { home: 'B', away: 'A', score: '4-6 6-3 6-4' },   // 10: B wins 2-1
  { home: 'D', away: 'C', score: '4-6 3-6' },       // 11: C wins 2-0
];

export const PENALTIES: Partial<Record<PairId, number>> = { A: 3 };

export function uniqueSuffix(): string {
  return randomUUID().slice(0, 8);
}

export async function setPlayerPassword(
  request: APIRequestContext,
  superuserToken: string,
  userId: string,
  password: string,
): Promise<void> {
  const resp = await request.patch(
    `/api/collections/users/records/${userId}`,
    {
      headers: { Authorization: superuserToken },
      data: { password, passwordConfirm: password },
    },
  );
  if (!resp.ok()) {
    throw new Error(`setPlayerPassword failed: ${resp.status()} ${await resp.text()}`);
  }
}

interface ParsedScore {
  sets1: number;
  sets2: number;
  games1: number;
  games2: number;
}

function isValidSet(a: number, b: number): boolean {
  if (a === b) return false;
  const hi = Math.max(a, b), lo = Math.min(a, b);
  if (hi === 6 && lo <= 4) return true;
  if (hi === 7 && (lo === 5 || lo === 6)) return true;
  return false;
}

function parseScore(score: string): ParsedScore {
  const sets = score.trim().split(/\s+/);
  let sets1 = 0, sets2 = 0, games1 = 0, games2 = 0;
  for (const s of sets) {
    const [g1, g2] = s.split('-').map(Number);
    games1 += g1;
    games2 += g2;
    if (isValidSet(g1, g2)) {
      if (g1 > g2) sets1++;
      else sets2++;
    } else {
      // Open set: award to leader (same as TallyScore)
      if (g1 > g2) sets1++;
      else if (g2 > g1) sets2++;
    }
  }
  return { sets1, sets2, games1, games2 };
}

function determineWinner(m: PlannedMatch): PairId {
  const s = parseScore(m.score);
  return s.sets1 > s.sets2 ? m.home : m.away;
}

interface PairStats {
  played: number;
  wins: number;
  losses: number;
  setsWon: number;
  setsLost: number;
  gamesWon: number;
  gamesLost: number;
}

export function computeExpected(
  matches: PlannedMatch[],
  penalties: Partial<Record<PairId, number>>,
): ExpectedRow[] {
  const pairs: PairId[] = ['A', 'B', 'C', 'D'];
  const stats = Object.fromEntries(
    pairs.map(p => [p, { played: 0, wins: 0, losses: 0, setsWon: 0, setsLost: 0, gamesWon: 0, gamesLost: 0 }]),
  ) as Record<PairId, PairStats>;

  for (const m of matches) {
    const s = parseScore(m.score);
    const winner = determineWinner(m);
    const loser = winner === m.home ? m.away : m.home;

    stats[m.home].played++;
    stats[m.away].played++;
    stats[winner].wins++;
    stats[loser].losses++;

    stats[m.home].setsWon += s.sets1;
    stats[m.home].setsLost += s.sets2;
    stats[m.away].setsWon += s.sets2;
    stats[m.away].setsLost += s.sets1;

    stats[m.home].gamesWon += s.games1;
    stats[m.home].gamesLost += s.games2;
    stats[m.away].gamesWon += s.games2;
    stats[m.away].gamesLost += s.games1;
  }

  const rows: ExpectedRow[] = pairs.map(p => {
    const pen = penalties[p] ?? 0;
    return {
      pair: p,
      position: 0,
      played: stats[p].played,
      wins: stats[p].wins,
      losses: stats[p].losses,
      setsWon: stats[p].setsWon,
      setsLost: stats[p].setsLost,
      gamesWon: stats[p].gamesWon,
      gamesLost: stats[p].gamesLost,
      penalty: pen,
      points: stats[p].wins * 3 - pen,
    };
  });

  // Sort mirrors sortStandings + resolveMiniLeague in league/standings.go.
  // Points first; then tie groups resolved by the Liga Dale Fuerte chain.
  rows.sort((a, b) => b.points - a.points);

  // Find tie groups and resolve each.
  let i = 0;
  while (i < rows.length) {
    let j = i + 1;
    while (j < rows.length && rows[j].points === rows[i].points) j++;
    const group = rows.slice(i, j);
    if (group.length > 1) {
      resolveGroup(group, matches);
      for (let k = 0; k < group.length; k++) rows[i + k] = group[k];
    }
    i = j;
  }

  rows.forEach((r, idx) => { r.position = idx + 1; });
  return rows;
}

// resolveGroup orders a tie group in place using the Liga tiebreaker chain.
// For 2 pairs: played → head-to-head → mutual set diff → mutual game diff → overall stats → name.
// For 3+ pairs: pre-step by played, then recursive mini-league partition (§3.3.10 P2).
function resolveGroup(group: ExpectedRow[], matches: PlannedMatch[]): void {
  if (group.length <= 1) return;
  if (group.length === 2) {
    resolveTwoWay(group, matches);
    return;
  }
  resolveMiniLeaguePrestep(group, matches);
}

function resolveTwoWay(group: ExpectedRow[], matches: PlannedMatch[]): void {
  const pairIds = group.map(r => r.pair);
  const mutual = mutualStats(pairIds, matches);
  group.sort((x, y) => {
    if (x.played !== y.played) return y.played - x.played;
    const sx = mutual[x.pair], sy = mutual[y.pair];
    if (sx.wins !== sy.wins) return sy.wins - sx.wins;
    const setDiffX = sx.setsWon - sx.setsLost, setDiffY = sy.setsWon - sy.setsLost;
    if (setDiffX !== setDiffY) return setDiffY - setDiffX;
    const gameDiffX = sx.gamesWon - sx.gamesLost, gameDiffY = sy.gamesWon - sy.gamesLost;
    if (gameDiffX !== gameDiffY) return gameDiffY - gameDiffX;
    return lessByOverallThenName(x, y) ? -1 : 1;
  });
}

// resolveMiniLeaguePrestep sorts by overall played, then calls miniLeaguePartition
// on each sub-group still tied on played (mirrors resolveMiniLeague in Go).
function resolveMiniLeaguePrestep(group: ExpectedRow[], matches: PlannedMatch[]): void {
  group.sort((a, b) => b.played - a.played);
  let start = 0;
  while (start < group.length) {
    let end = start + 1;
    while (end < group.length && group[end].played === group[start].played) end++;
    const sub = group.slice(start, end);
    if (sub.length >= 2) {
      miniLeaguePartition(sub, matches);
      for (let k = 0; k < sub.length; k++) group[start + k] = sub[k];
    }
    start = end;
  }
}

// miniLeaguePartition recomputes mutual stats fresh for the current group at
// each call (§3.3.10 P2) and tries mini criteria in order. If a criterion
// separates pairs, resolved sub-groups recurse from the top (fresh stats).
// Groups unseparated after all three mini criteria fall to overall stats.
function miniLeaguePartition(group: ExpectedRow[], matches: PlannedMatch[]): void {
  if (group.length <= 1) return;
  if (group.length === 2) {
    resolveTwoWay(group, matches);
    return;
  }

  const pairIds = group.map(r => r.pair);
  const mini = mutualStats(pairIds, matches);

  const miniCriteria: Array<(r: ExpectedRow) => number> = [
    r => mini[r.pair].wins * 3,
    r => mini[r.pair].setsWon - mini[r.pair].setsLost,
    r => mini[r.pair].gamesWon - mini[r.pair].gamesLost,
  ];

  for (const score of miniCriteria) {
    if (partitionBy(group, score, sub => miniLeaguePartition(sub, matches))) return;
  }

  // All mini criteria exhausted with no separation — fall to overall.
  group.sort((a, b) => lessByOverallThenName(a, b) ? -1 : 1);
}

// partitionBy sorts group descending by score. If the criterion separates at
// least one pair (not all equal), calls resolve on each tied sub-group of
// size ≥2 and returns true. Returns false without calling resolve when all
// scores are equal (prevents infinite recursion; caller tries next criterion).
function partitionBy(
  group: ExpectedRow[],
  score: (r: ExpectedRow) => number,
  resolve: (sub: ExpectedRow[]) => void,
): boolean {
  group.sort((a, b) => score(b) - score(a));
  if (score(group[0]) === score(group[group.length - 1])) return false;

  let start = 0;
  while (start < group.length) {
    let end = start + 1;
    while (end < group.length && score(group[end]) === score(group[start])) end++;
    const sub = group.slice(start, end);
    if (sub.length >= 2) {
      resolve(sub);
      for (let k = 0; k < sub.length; k++) group[start + k] = sub[k];
    }
    start = end;
  }
  return true;
}

function lessByOverallThenName(a: ExpectedRow, b: ExpectedRow): boolean {
  const setDiffA = a.setsWon - a.setsLost, setDiffB = b.setsWon - b.setsLost;
  if (setDiffA !== setDiffB) return setDiffA > setDiffB;
  const gameDiffA = a.gamesWon - a.gamesLost, gameDiffB = b.gamesWon - b.gamesLost;
  if (gameDiffA !== gameDiffB) return gameDiffA > gameDiffB;
  return a.pair < b.pair;
}

// mutualStats computes match stats for each pair restricted to matches where
// both participants are in pairIds (mirrors matchesBetween + tallyMatchStats in Go).
function mutualStats(pairIds: PairId[], matches: PlannedMatch[]): Record<PairId, PairStats> {
  const inGroup = new Set<PairId>(pairIds);
  const result = Object.fromEntries(
    pairIds.map(p => [p, { played: 0, wins: 0, losses: 0, setsWon: 0, setsLost: 0, gamesWon: 0, gamesLost: 0 }]),
  ) as Record<PairId, PairStats>;

  for (const m of matches) {
    if (!inGroup.has(m.home) || !inGroup.has(m.away)) continue;
    const s = parseScore(m.score);
    const winner = determineWinner(m);
    const loser = winner === m.home ? m.away : m.home;

    result[m.home].played++;
    result[m.away].played++;
    result[winner].wins++;
    result[loser].losses++;

    result[m.home].setsWon += s.sets1;
    result[m.home].setsLost += s.sets2;
    result[m.away].setsWon += s.sets2;
    result[m.away].setsLost += s.sets1;

    result[m.home].gamesWon += s.games1;
    result[m.home].gamesLost += s.games2;
    result[m.away].gamesWon += s.games2;
    result[m.away].gamesLost += s.games1;
  }
  return result;
}
