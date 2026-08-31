import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, PLAYER1_EMAIL, PLAYER1_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test.describe('mobile match lifecycle', () => {
  test.beforeEach(({ }, testInfo) => {
    test.skip(testInfo.project.name !== 'mobile', 'mobile-only lifecycle test');
  });

  test('submit → confirm → final on mobile viewport', async ({ page }, testInfo) => {
    const matchId = scratchMatchId('mobile-lifecycle', testInfo.project.name);

    // Step 1: Player1 (pair1) submits a score
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.score-btn').first()).toBeVisible({ timeout: 5000 });

    for (const [f, v] of [['s1a', '6'], ['s1b', '2'], ['s2a', '7'], ['s2b', '5']] as const) {
      await page.$eval(`input[name="${f}"]`, (el, val) => { (el as HTMLInputElement).value = val; }, v);
    }
    await page.getByRole('button', { name: 'Enviar resultado' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('6-2 7-5').first()).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/esperando confirmaci[oó]n/i).first()).toBeVisible({ timeout: 5000 });

    // Step 2: Admin (pair2 member, opponent) confirms the score.
    // Switch to player view so the confirm button shows instead of admin override.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/view/player');
    await page.waitForLoadState('networkidle');
    await page.goto(`/match/${matchId}`);
    await page.getByRole('button', { name: /confirmar/i }).click();
    await page.waitForLoadState('networkidle');

    // Step 3: Verify final state — score visible, no pending actions
    await expect(page.getByText('6-2 7-5').first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.badge:has-text("Final")')).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('button', { name: 'Enviar resultado' })).not.toBeVisible();
  });
});
