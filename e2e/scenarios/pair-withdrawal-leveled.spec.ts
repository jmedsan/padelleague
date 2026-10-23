import { test } from '@playwright/test';
import { ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  ScenarioApi, ScenarioData, addPairToCompetition, assertAssignmentInvariants, createCompetition, createPairs,
  createPlayers, generateAssignments, loadCtx, printLinks, printLogin, publishCalendar, saveCtx,
  setCompetitionDates, uniqueSuffix,
} from '../scenario-helpers';
import { MatchState, driveTo, duelOf, pairMatches } from './withdrawal-states';

// Leveled: target is total matches per pair, open is the concurrent non-final matches the top-up keeps.
const PAIRS = 8;
const TARGET = 6;
const OPEN = 5;

// Each open match of the pair under test gets one state; the top-up then refills the open set.
const STATES: MatchState[] = ['assigned', 'dateProposed', 'scheduled', 'scheduledResult', 'final'];

test('00 seed: eight pairs, leveled, the pair under test with five open matches in five states', async () => {
  const raw = loadCtx();
  const api: ScenarioApi = { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie };
  const suffix = uniqueSuffix();

  const players = await createPlayers(api, PAIRS * 2, suffix);
  const pairs = await createPairs(api, players, suffix);
  const competitionId = await createCompetition(api, `Retirada nivelada ${suffix}`, TARGET, OPEN);
  for (const p of pairs) await addPairToCompetition(api, competitionId, p.id);
  await setCompetitionDates(api, competitionId);
  await generateAssignments(api, competitionId);
  await publishCalendar(api, competitionId);

  const ctx: ScenarioData = { ...raw, api, competitionId, players, pairs, target: TARGET, open: OPEN, stage: 'assigned' };
  await assertAssignmentInvariants(api, ctx);

  const mine = pairs[0];
  const initial = await pairMatches(api, competitionId, mine.id);
  if (initial.length !== OPEN || initial.some((m) => m.status !== 'pending')) {
    throw new Error(`${mine.name}: expected ${OPEN} pending matches, got ${initial.length}`);
  }

  const rows: Array<{ label: string; match: any }> = [];
  for (let i = 0; i < STATES.length; i++) {
    await driveTo(api, duelOf(ctx, initial[i], mine), STATES[i]);
    rows.push({ label: STATES[i], match: initial[i] });
  }

  // The final match frees a slot, so the top-up adds exactly one match for the pair.
  const after = await pairMatches(api, competitionId, mine.id);
  const initialIDs = new Set(initial.map((m) => m.id));
  const added = after.filter((m) => !initialIDs.has(m.id));
  if (after.length !== TARGET || added.length !== 1) {
    throw new Error(`${mine.name}: expected ${TARGET} matches with one new, got ${after.length} with ${added.length} new`);
  }
  const open = after.filter((m) => m.status !== 'final');
  if (open.length !== OPEN) {
    throw new Error(`${mine.name}: expected ${OPEN} open matches after top-up, got ${open.length}`);
  }
  rows.push({ label: 'top-up', match: added[0] });

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
  printLogin(ctx.baseURL, players[mine.player1Idx].email, `/match/${initial[0].id}`);
  console.log('\nChecklist, after withdrawing the pair (typed RETIRAR):');
  console.log('  1. Assigned, date-proposed and scheduled matches are released: deleted with their threads.');
  console.log('  2. The proposed-result match and the players\' 6-3 6-4 final end final with the rival winning 6-0 6-0.');
  console.log('  3. Each released or changed match sends the rival one notice that links to the competition page.');
  console.log('  4. The top-up may give a freed rival a new opponent; the withdrawn pair gets none.');
  console.log('  5. The pair name shows a red withdrawn icon in this competition; hovering it shows "Pareja retirada".');
  console.log('  6. The withdrawn pair gets one notice, then no more competition notices.');
});
