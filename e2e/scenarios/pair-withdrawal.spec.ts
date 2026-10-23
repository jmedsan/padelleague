import { test } from '@playwright/test';
import { ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  ScenarioApi, ScenarioData, addPairToCompetition, apiPatch, createCompetition, createPairs, createPlayers,
  generateAssignments, loadCtx, printLinks, printLogin, publishCalendar, saveCtx, setCompetitionDates, uniqueSuffix,
} from '../scenario-helpers';
import { MatchState, driveTo, duelOf, expectStatus, pairMatches } from './withdrawal-states';

// Six pairs, play twice: every pair plays five opponents twice, so ten matches each.
const PAIRS = 6;
const MATCHES_PER_PAIR = 10;

// One match per state, in the order they are driven; the rest stay assigned as spares.
const STATES: MatchState[] = [
  'assigned', 'dateProposed', 'scheduled', 'scheduledResult',
  'final', 'disputed', 'arbitrated', 'adminFinal',
];

test('00 seed: six pairs, play twice, the pair under test in eight states and spares', async () => {
  const raw = loadCtx();
  const api: ScenarioApi = { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie };
  const suffix = uniqueSuffix();

  const players = await createPlayers(api, PAIRS * 2, suffix);
  const pairs = await createPairs(api, players, suffix);
  const competitionId = await createCompetition(api, `Retirada ${suffix}`, 0, 0);
  for (const p of pairs) await addPairToCompetition(api, competitionId, p.id);
  await apiPatch(api, `/api/collections/competitions/records/${competitionId}`, { play_twice: true });
  await setCompetitionDates(api, competitionId);
  await generateAssignments(api, competitionId);
  await publishCalendar(api, competitionId);

  const ctx: ScenarioData = { ...raw, api, competitionId, players, pairs, target: 0, open: 0, stage: 'assigned' };
  const mine = pairs[0];
  const matches = await pairMatches(api, competitionId, mine.id);
  if (matches.length !== MATCHES_PER_PAIR) {
    throw new Error(`${mine.name}: expected ${MATCHES_PER_PAIR} matches, got ${matches.length}`);
  }

  const rows: Array<{ label: string; match: any }> = [];
  for (let i = 0; i < STATES.length; i++) {
    await driveTo(api, duelOf(ctx, matches[i], mine), STATES[i]);
    rows.push({ label: STATES[i], match: matches[i] });
  }
  for (const spare of matches.slice(STATES.length)) {
    await expectStatus(api, spare.id, 'assigned');
    rows.push({ label: 'spare', match: spare });
  }

  saveCtx({ ...ctx, stage: 'assigned' });

  console.log(`\nPair under test: ${mine.name}`);
  for (const { label, match } of rows) {
    const rivalID = match.pair1 === mine.id ? match.pair2 : match.pair1;
    const rival = pairs.find((p) => p.id === rivalID)!;
    console.log(`  ${label.padEnd(16)} vs ${rival.name.padEnd(18)} ${ctx.baseURL}/match/${match.id}`);
  }
  printLinks(ctx.baseURL, [
    { label: 'Admin, competition', email: ADMIN_EMAIL, password: ADMIN_PASSWORD, path: `/admin/competitions/${competitionId}` },
  ]);
  printLogin(ctx.baseURL, players[mine.player1Idx].email, `/match/${matches[0].id}`);
  console.log('\nChecklist, after withdrawing the pair (typed RETIRAR):');
  console.log('  1. Every match of the pair ends final with the rival winning 6-0 6-0: pending, scheduled,');
  console.log('     proposed result, disputed, the players\' 6-3 6-4 final, and the spare.');
  console.log('  2. The match an admin set to 6-0 6-0 is unchanged: no timeline entry, no notice.');
  console.log('  3. The disputed and arbitrated matches have no open arbitration request.');
  console.log('  4. The pair name is gray in this competition, with the tooltip "Pareja retirada de esta competición".');
  console.log('  5. The withdrawn pair gets one notice, then no more competition notices.');
  console.log('  6. Each rival gets one notice per changed match.');
});
