import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { clickAndWaitForHxRedirect } from '../tour-helpers';
import {
  ScenarioApi, ScenarioData, apiGet,
  loadCtx, saveCtx, ensureStage, assertAssignmentInvariants,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

test.describe('leveled-16 pre-generation', () => {
  test.describe.configure({ mode: 'serial' });
  test.describe.configure({ retries: 0 });

  // 00 is the entry state this scenario serves: dates already set, nothing
  // generated. `make scenario-serve SCENARIO=leveled-16-pregen` only runs
  // tests matching "00 ", so this step must be a pure assertion — no
  // Generar/Publicar click — or a served scenario would already have
  // matches by the time the owner opens the page.
  test('00 dated — dates shown, no matches yet', async ({ page }) => {
    const raw = loadCtx();
    // ensureStage (not requireStage) so this spec works both standalone
    // (setup already built to 'dated') and chained after leveled-16-nodates
    // (which leaves the scenario at 'created' — this advances it here).
    ctx = await ensureStage(
      { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie },
      'dated',
    );
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    await expect(page.locator('#edit-comp-start')).toHaveValue('2036-09-29');
    await expect(page.locator('#edit-comp-end')).toHaveValue('2036-12-10');
    await expect(page.locator('button:has-text("Generar calendario")')).toBeVisible({ timeout: 10000 });

    const data = await apiGet(api,
      `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=1`);
    expect(data.totalItems).toBe(0);
  });

  test('01 generate calendar — matches created as draft', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    const genBtn = page.locator('button:has-text("Generar calendario")');
    await expect(genBtn).toBeVisible({ timeout: 10000 });
    await clickAndWaitForHxRedirect(page, genBtn);

    await expect(page.locator('button:has-text("Publicar calendario")')).toBeVisible({ timeout: 10000 });

    const data = await apiGet(api,
      `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`);
    expect(data.totalItems).toBeGreaterThanOrEqual(20);
  });

  test('02 publish calendar — Jornada groups visible', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    const pubBtn = page.locator('button:has-text("Publicar calendario")');
    await expect(pubBtn).toBeVisible({ timeout: 10000 });
    await clickAndWaitForHxRedirect(page, pubBtn);

    await page.waitForLoadState('domcontentloaded');
    for (let s = 1; s <= ctx.open; s++) {
      await expect(
        page.locator(`.collapse-title:has-text("Jornada ${s}")`).first(),
      ).toBeVisible({ timeout: 10000 });
    }
    await expect(page.locator('.collapse-title:has-text("Jornada 0")')).toHaveCount(0);

    await assertAssignmentInvariants(api, ctx);

    saveCtx({ ...ctx, stage: 'assigned' });
  });
});
