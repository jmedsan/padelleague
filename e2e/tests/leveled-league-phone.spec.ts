import { test, expect } from '../overflow-guard';
import { loadTestData, apiCreateRecord, asPlayerOn, loginAs, leagueDate, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';
import { generateFixtures } from '../tour-helpers';

// Phone-only: listed in MOBILE_VIEWPORT_SPECS (playwright.config.ts), so it
// runs on the mobile project only. Desktop twin: leveled-league.spec.ts.
test.describe('leveled league on phone', { tag: '@leveled' }, () => {
  test('leveled competition page renders correctly on phone', async ({ page }) => {
    const data = loadTestData();
    const token = data.adminToken;
    // Four pairs with target_matches 2 < pairs-1: a leveled league (league.IsLeveled).
    const compId = await apiCreateRecord(page.request, token, 'competitions', {
      name: `Liga Nivelada Movil ${Date.now() % 100000}`, type: 'league', active: true,
      pairs: [data.pair1Id, data.pair2Id, data.pair3Id, data.adminPairId],
      target_matches: 2, open_assignments: 1,
      start_date: leagueDate(-7), end_date: leagueDate(60),
    });
    // The admin generates the calendar: the top-up assigns the opponents and
    // writes each match's slot, the same path the main leveled test takes.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    await page.waitForLoadState('domcontentloaded');
    await generateFixtures(page);

    await asPlayerOn(page, compId, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator(`a[href="/competition/${compId}"]`).locator('visible=true').first().click();

    await expect(page.locator('input[aria-label^="Partidos"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.collapse-title:has-text("Jornada 1")')).toBeVisible({ timeout: 5000 });
  });
});
