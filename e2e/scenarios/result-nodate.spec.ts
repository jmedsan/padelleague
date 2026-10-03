import { test } from '@playwright/test';
import {
  ScenarioApi, ScenarioData, apiGet, loadCtx, printLogin,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

test.describe('match without a date and place proposal', () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(() => {
    ctx = loadCtx();
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
  });

  test('00 ready: no date or place proposed, the result cannot be entered', async () => {
    const pair = ctx.pairs[0];
    const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
      `competition='${ctx.competitionId}' && status='pending' && (pair1='${pair.id}' || pair2='${pair.id}')`)}&perPage=1`);
    const matchID = list.items[0].id;
    printLogin(ctx.baseURL, ctx.players[pair.player1Idx].email, `/match/${matchID}`);
    console.log('Look at the result card: it should say the date and place must be proposed first.');
  });
});
