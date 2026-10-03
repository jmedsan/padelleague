import { test, expect } from '@playwright/test';
import { loginAs } from '../helpers';
import {
  ScenarioApi, ScenarioData, apiGet, apiPatch, loadCtx, printLogin,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

async function playerMatchID(): Promise<string> {
  const pairID = ctx.pairs[0].id;
  const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
    `competition='${ctx.competitionId}' && status='pending' && (pair1='${pairID}' || pair2='${pairID}')`)}&perPage=1`);
  return list.items[0].id;
}

test.describe('match ready for the result input', () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(() => {
    ctx = loadCtx();
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
  });

  test('00 ready: a match with date and place, nothing played', async () => {
    const matchID = await playerMatchID();
    await apiPatch(api, `/api/collections/matches/records/${matchID}`, {
      date: '2025-03-15', club: 'Padel 360',
    });
    printLogin(ctx.baseURL, ctx.players[ctx.pairs[0].player1Idx].email, `/match/${matchID}`);
    console.log('Click "Enviar resultado" without entering a score.');
  });

  test('01 the submit button is disabled while nothing is entered', async ({ page }) => {
    const matchID = await playerMatchID();
    await loginAs(page, ctx.players[ctx.pairs[0].player1Idx].email, PLAYER_PASSWORD);
    await page.goto(`/match/${matchID}`);
    await page.waitForSelector('#result-panel', { timeout: 10000 });
    await expect(page.locator('.score-input').first().locator('.score-cell').first()).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('button', { name: 'Enviar resultado' })).toBeDisabled();
  });

  test('02 forcing the click stores no proposal', async ({ page }) => {
    const matchID = await playerMatchID();
    await loginAs(page, ctx.players[ctx.pairs[0].player1Idx].email, PLAYER_PASSWORD);
    await page.goto(`/match/${matchID}`);
    await page.waitForSelector('#result-panel', { timeout: 10000 });
    await page.getByRole('button', { name: 'Enviar resultado' }).click({ force: true });
    await page.waitForTimeout(1000);
    const proposals = await apiGet(api, `/api/collections/match_messages/records?filter=${encodeURIComponent(
      `match='${matchID}' && type='result_submission'`)}&perPage=50`);
    expect(proposals.totalItems).toBe(0);
    await expect(page.getByText('Resultado propuesto')).toHaveCount(0);
  });
});
