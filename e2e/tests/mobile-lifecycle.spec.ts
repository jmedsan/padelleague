import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, PLAYER1_EMAIL, PLAYER1_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore } from '../tour-helpers';

test.describe('mobile match lifecycle', () => {
  test.beforeEach(({ }, testInfo) => {
    test.skip(testInfo.project.name !== 'mobile', 'mobile-only lifecycle test');
  });

  test('submit → accept → final on mobile viewport', async ({ page }, testInfo) => {
    const matchId = scratchMatchId('mobile-lifecycle', testInfo.project.name);

    // Set date+club via superuser API so score submission is enabled
    const suAuth = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    const suToken = (await suAuth.json()).token;
    await page.request.patch(`/api/collections/matches/records/${matchId}`, {
      headers: { Authorization: suToken },
      data: { date: '2025-03-15', club: 'Padel 360' },
    });

    // Step 1: Player1 (pair1) submits a score
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.score-cell').first()).toBeVisible({ timeout: 5000 });
    await enterScore(page, '6-2 7-5');
    await page.getByRole('button', { name: 'Enviar resultado' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('6-2 7-5').first()).toBeVisible({ timeout: 5000 });

    // Step 2: Admin (pair2 member, opponent) accepts the result proposal via thread.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/view/player');
    await page.waitForLoadState('networkidle');
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 5000 });
    const acceptBtn = page.locator('#thread-details button:has-text("Confirmar")').first();
    await acceptBtn.waitFor({ timeout: 5000 });
    await acceptBtn.click();
    await page.waitForLoadState('networkidle');

    // Step 3: Verify final state — score visible, no pending actions
    await expect(page.getByText('6-2 7-5').first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#thread-details .badge:has-text("Confirmado")')).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('button', { name: 'Enviar resultado' })).not.toBeVisible();
  });
});
