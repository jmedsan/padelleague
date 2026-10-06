import { test, expect } from '@playwright/test';
import { loginAs } from '../helpers';
import {
  PLAYER_PASSWORD, ScenarioApi, ScenarioData, apiGet, apiPatch, loadCtx, printLogin,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

async function playerToken(email: string): Promise<string> {
  const resp = await fetch(`${ctx.baseURL}/api/collections/users/auth-with-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identity: email, password: PLAYER_PASSWORD }),
  });
  if (!resp.ok) throw new Error(`login ${email}: ${resp.status}`);
  return (await resp.json()).token;
}

test.describe('a result proposed and not yet confirmed', () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(() => {
    ctx = loadCtx();
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
  });

  test('00 ready: one pair proposed 6-3 6-4 and the rival has not answered', async () => {
    const mine = ctx.pairs[0];
    const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
      `competition='${ctx.competitionId}' && status='pending' && (pair1='${mine.id}' || pair2='${mine.id}')`)}&perPage=1`);
    const match = list.items[0];
    await apiPatch(api, `/api/collections/matches/records/${match.id}`, {
      date: '2025-03-15', club: 'Padel 360',
    });

    const proposer = ctx.players[mine.player1Idx];
    const resp = await fetch(`${ctx.baseURL}/match/${match.id}/submit`, {
      method: 'POST',
      headers: {
        Cookie: `pb_auth=${await playerToken(proposer.email)}`,
        'HX-Request': 'true',
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({ scores: '6-3 6-4' }).toString(),
      redirect: 'manual',
    });
    if (resp.status >= 400) throw new Error(`submit: ${resp.status} ${await resp.text()}`);

    printLogin(ctx.baseURL, proposer.email, `/match/${match.id}`);
    console.log('Check the warning icon ("Incluye resultados sin confirmar") on: Clasificación, home (Mis últimos partidos and the competition card), your profile and pair stats, and the admin round badge.');
  });

  test('01 calendar: the proposed match row shows 6-3 6-4 and Propuesta, no warning icon', async ({ page }) => {
    const mine = ctx.pairs[0];
    const proposer = ctx.players[mine.player1Idx];
    const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
      `competition='${ctx.competitionId}' && date='2025-03-15 00:00:00.000Z' && (pair1='${mine.id}' || pair2='${mine.id}')`)}&perPage=1`);
    const matchId = list.items[0].id;
    await loginAs(page, proposer.email, PLAYER_PASSWORD);
    await page.goto('/');
    await page.locator(`a[href^="/competition/${ctx.competitionId}"]`).first().click();
    await page.waitForURL(`**/competition/${ctx.competitionId}**`);
    await page.locator('input[aria-label^="Jornadas"], input[aria-label^="Partidos"]').click();
    const row = page.locator(`a[href="/match/${matchId}"]`).first();
    for (const box of await page.locator('.collapse', { has: row }).all()) {
      const input = box.locator('> input');
      if (await input.count() && !(await input.isChecked())) await input.check();
    }
    await expect(row).toContainText('6-3 6-4');
    await expect(row.locator('xpath=ancestor::div[contains(concat(" ",@class," ")," collapse ")][1]').locator('.collapse-title').first(),
      'a proposed result counts as played').toContainText('Jugados');
    await expect(page.locator('.collapse-title', { hasText: 'Sin asignar' }), 'every live match has a Jornada slot').toHaveCount(0);
    await expect(row.locator('.badge', { hasText: 'Propuesta' })).toBeVisible();
    await expect(row.locator('[data-testid="provisional-warning"]')).toHaveCount(0);
  });
});
