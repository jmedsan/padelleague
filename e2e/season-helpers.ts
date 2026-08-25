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
  { home: 'A', away: 'B', score: '6-3 6-3' },       // 4:  A wins 2-0
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

function parseScore(score: string): ParsedScore {
  const sets = score.trim().split(/\s+/);
  let sets1 = 0, sets2 = 0, games1 = 0, games2 = 0;
  for (const s of sets) {
    const [g1, g2] = s.split('-').map(Number);
    games1 += g1;
    games2 += g2;
    if (g1 > g2) sets1++;
    else sets2++;
  }
  return { sets1, sets2, games1, games2 };
}

function determineWinner(m: PlannedMatch): PairId {
  const s = parseScore(m.score);
  return s.sets1 > s.sets2 ? m.home : m.away;
}

export function computeExpected(
  matches: PlannedMatch[],
  penalties: Partial<Record<PairId, number>>,
): ExpectedRow[] {
  const pairs: PairId[] = ['A', 'B', 'C', 'D'];
  const stats = Object.fromEntries(
    pairs.map(p => [p, { played: 0, wins: 0, losses: 0, setsWon: 0, setsLost: 0, gamesWon: 0, gamesLost: 0 }]),
  ) as Record<PairId, { played: number; wins: number; losses: number; setsWon: number; setsLost: number; gamesWon: number; gamesLost: number }>;

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

  rows.sort((a, b) => {
    if (a.points !== b.points) return b.points - a.points;
    const setDiffA = a.setsWon - a.setsLost;
    const setDiffB = b.setsWon - b.setsLost;
    if (setDiffA !== setDiffB) return setDiffB - setDiffA;
    const gameDiffA = a.gamesWon - a.gamesLost;
    const gameDiffB = b.gamesWon - b.gamesLost;
    if (gameDiffA !== gameDiffB) return gameDiffB - gameDiffA;
    return headToHead(a.pair, b.pair, matches) ? -1 : 1;
  });

  rows.forEach((r, i) => { r.position = i + 1; });
  return rows;
}

function headToHead(pairA: PairId, pairB: PairId, matches: PlannedMatch[]): boolean {
  let winsA = 0, winsB = 0;
  for (const m of matches) {
    if ((m.home === pairA && m.away === pairB) || (m.home === pairB && m.away === pairA)) {
      const winner = determineWinner(m);
      if (winner === pairA) winsA++;
      else if (winner === pairB) winsB++;
    }
  }
  return winsA > winsB;
}
