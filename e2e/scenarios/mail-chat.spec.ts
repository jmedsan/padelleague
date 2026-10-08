import { test, expect } from '@playwright/test';
import {
  MAILPIT_URL, PLAYER_PASSWORD, ScenarioApi, ScenarioData, addPairToCompetition, apiGet, apiPatch,
  createCompetition, createPairs, createPlayers, generateAssignments, loadCtx, printLinks,
  publishCalendar, saveCtx, uniqueSuffix,
} from '../scenario-helpers';

let ctx: ScenarioData;

async function seedMatchID(api: ScenarioApi, compId: string): Promise<string> {
  const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
    `competition='${compId}'`)}&perPage=5`);
  expect(list.totalItems).toBe(1);
  return list.items[0].id;
}

test.describe('chat email to the match players via Mailpit', () => {
  test.describe.configure({ retries: 0 });

  test('00 seed: one competition, one match, four players, SMTP -> Mailpit', async () => {
    const raw = loadCtx();
    const api: ScenarioApi = { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie };
    const suffix = uniqueSuffix();
    const players = await createPlayers(api, 4, suffix);
    await apiPatch(api, `/api/collections/users/records/${players[3].id}`, { verified: false });
    const pairs = await createPairs(api, players, suffix);
    const competitionId = await createCompetition(api, `Chat por email ${suffix}`, 0, 0);
    for (const pair of pairs) await addPairToCompetition(api, competitionId, pair.id);
    await generateAssignments(api, competitionId);
    await publishCalendar(api, competitionId);
    const matchID = await seedMatchID(api, competitionId);

    ctx = { ...raw, competitionId, players, pairs, stage: 'blank' };
    saveCtx(ctx);

    const [home, away] = await Promise.all(pairs.map((p) => apiGet(api, `/api/collections/pairs/records/${p.id}`)));
    const matchPath = `/match/${matchID}`;
    console.log(`\nMatch page: ${ctx.baseURL}${matchPath}`);
    console.log(`Mailpit inbox: ${MAILPIT_URL} (start it with: make mail)`);
    console.log(`Match: ${home.name} vs ${away.name}`);
    for (const pair of pairs) {
      for (const idx of [pair.player1Idx, pair.player2Idx]) {
        console.log(`  ${pair.name}  ${players[idx].email}  password ${PLAYER_PASSWORD}  verified ${idx === 3 ? 'no' : 'yes'}`);
      }
    }
    printLinks(ctx.baseURL, pairs.flatMap((pair) => [pair.player1Idx, pair.player2Idx].map((idx) => ({
      label: `Sign in as ${pair.name} / ${players[idx].email}`,
      email: players[idx].email, password: PLAYER_PASSWORD, path: matchPath,
    }))));
  });
});
