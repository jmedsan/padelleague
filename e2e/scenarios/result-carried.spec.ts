import { test } from '@playwright/test';
import {
  ScenarioApi, ScenarioData, apiGet, apiPatch, loadCtx, printLogin,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

test.describe('match resumed with carried sets', () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(() => {
    ctx = loadCtx();
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
  });

  test('00 ready: 6-3 already played, date and place set, ready for the rest of the result', async () => {
    const pair = ctx.pairs[0];
    const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
      `competition='${ctx.competitionId}' && status='pending' && (pair1='${pair.id}' || pair2='${pair.id}')`)}&perPage=1`);
    const matchID = list.items[0].id;
    await apiPatch(api, `/api/collections/matches/records/${matchID}`, {
      carried_sets: '6-3', date: '2025-03-15', club: 'Padel 360',
    });
    printLogin(ctx.baseURL, ctx.players[pair.player1Idx].email, `/match/${matchID}`);
    console.log('Click "Enviar resultado" without entering the rest of the score.');
  });
});
