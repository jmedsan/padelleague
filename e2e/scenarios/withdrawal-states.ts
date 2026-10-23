import {
  ScenarioApi, ScenarioCtx, apiGet, formPostData, playerPost,
} from '../scenario-helpers';

// Inside the 2036 season (setCompetitionDates); every scheduled state proposes this slot.
const SLOT = { date: '2036-10-15', time: '18:30', venue: 'Padel 360' };
const SCORE = '6-3 6-4';

// One match of the pair under test, driven into a state through the live routes.
export type MatchState =
  | 'assigned'
  | 'dateProposed'
  | 'scheduled'
  | 'scheduledResult'
  | 'final'
  | 'disputed'
  | 'arbitrated'
  | 'adminFinal';

const EXPECTED_STATUS: Record<MatchState, string> = {
  assigned: 'pending',
  dateProposed: 'pending',
  scheduled: 'scheduled',
  scheduledResult: 'scheduled',
  final: 'final',
  disputed: 'disputed',
  arbitrated: 'pending',
  adminFinal: 'final',
};

// The arbitration category each arbitrated state leaves on the match.
const EXPECTED_ARBITRATION: Partial<Record<MatchState, string>> = {
  disputed: 'result',
  arbitrated: 'scheduling',
};

// The two players who act on a match: the pair under test proposes, the rival answers.
export interface Duel {
  matchId: string;
  proposerEmail: string;
  rivalEmail: string;
}

export function duelOf(ctx: ScenarioCtx, match: any, mine: ScenarioCtx['pairs'][number]): Duel {
  const rivalID = match.pair1 === mine.id ? match.pair2 : match.pair1;
  const rival = ctx.pairs.find((p) => p.id === rivalID);
  if (!rival) throw new Error(`match ${match.id}: rival pair ${rivalID} is not in the scenario`);
  return {
    matchId: match.id,
    proposerEmail: ctx.players[mine.player1Idx].email,
    rivalEmail: ctx.players[rival.player1Idx].email,
  };
}

// pairMatches lists every match of one pair in the competition, in round order.
export async function pairMatches(api: ScenarioApi, competitionId: string, pairId: string): Promise<any[]> {
  const filter = `competition='${competitionId}' && (pair1='${pairId}' || pair2='${pairId}')`;
  const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(filter)}&sort=round_number&perPage=500`);
  return list.items;
}

// expectStatus reads the match back and throws with the match ID when it is not in the state's status.
export async function expectStatus(api: ScenarioApi, matchId: string, state: MatchState): Promise<void> {
  const m = await apiGet(api, `/api/collections/matches/records/${matchId}`);
  if (m.status !== EXPECTED_STATUS[state]) {
    throw new Error(`match ${matchId} (${state}): expected status ${EXPECTED_STATUS[state]}, got ${m.status}`);
  }
  const arbitration = EXPECTED_ARBITRATION[state] ?? '';
  if ((m.arbitration || '') !== arbitration) {
    throw new Error(`match ${matchId} (${state}): expected arbitration "${arbitration}", got "${m.arbitration || ''}"`);
  }
}

async function pendingProposalID(api: ScenarioApi, matchId: string, type: string): Promise<string> {
  const filter = `match='${matchId}' && type='${type}' && proposal_status='pending'`;
  const list = await apiGet(api, `/api/collections/match_messages/records?filter=${encodeURIComponent(filter)}&perPage=5`);
  if (list.totalItems !== 1) throw new Error(`match ${matchId}: expected one pending ${type}, got ${list.totalItems}`);
  return list.items[0].id;
}

async function proposeDate(api: ScenarioApi, d: Duel): Promise<void> {
  await playerPost(api, d.proposerEmail, `/match/${d.matchId}/thread/proposal`,
    { date: SLOT.date, time: SLOT.time, venue_text: SLOT.venue });
}

async function acceptDate(api: ScenarioApi, d: Duel): Promise<void> {
  await proposeDate(api, d);
  const id = await pendingProposalID(api, d.matchId, 'scheduling_proposal');
  await playerPost(api, d.rivalEmail, `/match/${d.matchId}/thread/proposal/${id}/respond`, { action: 'accept' });
}

async function submitResult(api: ScenarioApi, d: Duel): Promise<void> {
  await playerPost(api, d.proposerEmail, `/match/${d.matchId}/submit`, { scores: SCORE });
}

async function acceptResult(api: ScenarioApi, d: Duel): Promise<void> {
  await submitResult(api, d);
  const id = await pendingProposalID(api, d.matchId, 'result_submission');
  await playerPost(api, d.rivalEmail, `/match/${d.matchId}/thread/proposal/${id}/respond`, { action: 'accept' });
}

async function requestArbitration(api: ScenarioApi, d: Duel, category: string, notes: string): Promise<void> {
  await playerPost(api, d.proposerEmail, `/match/${d.matchId}/arbitration`, { category, notes });
}

// driveTo leaves the match in the state and throws unless it reads back as that state.
export async function driveTo(api: ScenarioApi, d: Duel, state: MatchState): Promise<void> {
  switch (state) {
    case 'assigned':
      break;
    case 'dateProposed':
      await proposeDate(api, d);
      await pendingProposalID(api, d.matchId, 'scheduling_proposal');
      break;
    case 'scheduled':
      await acceptDate(api, d);
      break;
    case 'scheduledResult':
      await acceptDate(api, d);
      await submitResult(api, d);
      await pendingProposalID(api, d.matchId, 'result_submission');
      break;
    case 'final':
      await acceptDate(api, d);
      await acceptResult(api, d);
      break;
    case 'disputed':
      await requestArbitration(api, d, 'result', 'El resultado no coincide');
      break;
    case 'arbitrated':
      await requestArbitration(api, d, 'scheduling', 'No logramos acordar la fecha');
      break;
    case 'adminFinal':
      await formPostData(api, `/match/${d.matchId}/admin-override`, { scores: '6-0 6-0' });
      break;
  }
  await expectStatus(api, d.matchId, state);
}
