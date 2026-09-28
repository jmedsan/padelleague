import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, suPatch, clickAndWaitForHxRedirect, PLAYER3_EMAIL, PLAYER3_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore } from '../tour-helpers';

test.describe('mobile match lifecycle', { tag: '@scoring' }, () => {
  test.beforeEach(({ }, testInfo) => {
    test.skip(testInfo.project.name !== 'mobile', 'mobile-only lifecycle test');
  });

  test('submit → accept → final on mobile viewport', async ({ page }) => {
    // The admin-as-player match: Pareja Admin (admin + player7) vs pair3.
    const matchId = loadTestData().adminMatchId;

    // Set date+club via superuser API so score submission is enabled
    await suPatch(page.request, loadTestData().adminToken, `/api/collections/matches/records/${matchId}`, { date: '2025-03-15', club: 'Padel 360' });

    // Step 1: player3 (pair3) submits a score
    await loginAs(page, PLAYER3_EMAIL, PLAYER3_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.score-cell').first()).toBeVisible({ timeout: 5000 });
    await enterScore(page, '6-2 7-5');
    await clickAndWaitForHxRedirect(page, page.getByRole('button', { name: 'Enviar resultado' }), `/match/${matchId}`);
    await expect(page.getByText('6-2 7-5').locator('visible=true').first()).toBeVisible({ timeout: 5000 });

    // Step 2: the admin, playing in the opposing pair, switches to the player
    // view and accepts the result proposal via the thread.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/view/player');
    await page.waitForLoadState('networkidle');
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 5000 });
    const acceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
    await acceptBtn.waitFor({ timeout: 5000 });
    await clickAndWaitForHxRedirect(page, acceptBtn, `/match/${matchId}`);

    // Step 3: Verify final state — score visible, no pending actions
    await expect(page.getByText('6-2 7-5').locator('visible=true').first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#thread-details .badge:has-text("Confirmado")')).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('button', { name: 'Enviar resultado' })).not.toBeVisible();
  });
});
