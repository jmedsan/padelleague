import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  ScenarioApi, ScenarioData, apiGet, loadCtx, requireStage,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

test.describe('leveled-16 no dates', () => {
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
});
