import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { clickAndWaitForHxRedirect, setDates } from '../tour-helpers';
import {
  ScenarioApi, ScenarioData, apiGet,
  loadCtx, saveCtx, requireStage, assertAssignmentInvariants,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

test.describe('leveled-16 pre-generation', () => {
  test.describe.configure({ mode: 'serial' });
  test.describe.configure({ retries: 0 });

  test('00 created — no dates, generate refused', async ({ page }) => {
    ctx = loadCtx();
    requireStage(ctx, 'created');
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };

    const data = await apiGet(api,
      `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=1`);
    expect(data.totalItems).toBe(0);

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');

    const compLink = page.locator(`a[href="/admin/competitions/${ctx.competitionId}"]`).first();
    await compLink.waitFor({ timeout: 10000 });
    await compLink.click();
    await page.waitForLoadState('domcontentloaded');

    // Generate button is present
    const genBtn = page.locator('button:has-text("Generar calendario")');
    await expect(genBtn).toBeVisible({ timeout: 10000 });

    // Click generate — should fail because no dates (error retargets to #flash)
    await genBtn.click();
    await expect(
      page.locator('#flash .alert:has-text("necesitan fecha de inicio y de fin")'),
    ).toBeVisible({ timeout: 10000 });

    // No matches created
    const after = await apiGet(api,
      `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=1`);
    expect(after.totalItems).toBe(0);
  });

  test('01 add dates via settings form', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    const startDate = '2026-09-28';
    const endDate = '2026-12-06';
    await setDates(page, startDate, endDate);

    // Verify dates were saved by checking the competition record
    const comp = await apiGet(api,
      `/api/collections/competitions/records/${ctx.competitionId}`);
    expect(comp.start_date).toBeTruthy();
    expect(comp.end_date).toBeTruthy();

    saveCtx({ ...ctx, stage: 'dated' });
  });

  test('02 generate calendar — matches created as draft', async ({ page }) => {
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

  test('03 publish calendar — Jornada groups visible', async ({ page }) => {
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
