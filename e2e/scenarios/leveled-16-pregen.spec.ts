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

  test('00 dated — generate calendar — matches created as draft', async ({ page }) => {
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

    const genBtn = page.locator('button:has-text("Generar calendario")');
    await expect(genBtn).toBeVisible({ timeout: 10000 });
    await clickAndWaitForHxRedirect(page, genBtn);

    await expect(page.locator('button:has-text("Publicar calendario")')).toBeVisible({ timeout: 10000 });

    const data = await apiGet(api,
      `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`);
    expect(data.totalItems).toBeGreaterThanOrEqual(20);
  });

  test('01 publish calendar — Jornada groups visible', async ({ page }) => {
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
