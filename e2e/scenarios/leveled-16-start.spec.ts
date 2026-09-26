import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  ScenarioApi, ScenarioData, apiGet, loadCtx, ensureStage, assertAssignmentInvariants, PLAYER_PASSWORD,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

// This is the continuation point of leveled-16-pregen: calendar generated AND
// published, ready to play the first matches. `make scenario-serve
// SCENARIO=leveled-16-start` only runs tests matching "00 ", so this step
// must be a pure assertion — no score submission, no top-up — or a served
// scenario would already have finals by the time the owner opens the page.
test.describe('leveled-16 start', () => {
  test.describe.configure({ mode: 'serial' });
  test.describe.configure({ retries: 0 });

  test('00 published, ready to play — 24 pending, 0 finals', async ({ page }) => {
    const raw = loadCtx();
    // ensureStage (not requireStage) so this spec works both standalone
    // (setup builds straight to 'assigned' via buildToStage) and chained
    // after leveled-16-pregen (which already left the scenario at 'assigned'
    // — this is then a no-op).
    ctx = await ensureStage(
      { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie },
      'assigned',
    );
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };

    const data = await apiGet(api, `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`);
    const matches: any[] = data.items;
    const pending = matches.filter(m => m.status === 'pending');
    const finals = matches.filter(m => m.status === 'final');

    // 16 pairs × open(3)/2 = 24 nominal; accept [20, 32]. Nothing played yet.
    expect(pending.length).toBeGreaterThanOrEqual(20);
    expect(pending.length).toBeLessThanOrEqual(32);
    expect(finals.length).toBe(0);

    await assertAssignmentInvariants(api, ctx);

    // Calendar is published: a player (not just the admin) can see it.
    const comp = await apiGet(api, `/api/collections/competitions/records/${ctx.competitionId}`);
    expect(comp.calendar_status).toBe('published');

    const player1Email = ctx.players[ctx.pairs[0].player1Idx].email;
    await loginAs(page, player1Email, PLAYER_PASSWORD);
    await page.goto(`/competition/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await page.locator('input[aria-label="Partidos"]').click();
    await page.waitForLoadState('domcontentloaded');
    for (let s = 1; s <= ctx.open; s++) {
      await expect(page.locator(`.collapse-title:has-text("Jornada ${s}")`).first()).toBeVisible({ timeout: 10000 });
    }
    await expect(page.locator('.collapse-title:has-text("Jornada 0")')).toHaveCount(0);

    // Admin can also reach the competition; nothing else has happened.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/competition/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page).toHaveURL(new RegExp(`/competition/${ctx.competitionId}`));
  });
});
