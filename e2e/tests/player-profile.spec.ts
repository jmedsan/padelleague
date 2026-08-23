import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test.describe('player profile and stats', () => {
  test('player can view own profile', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/player/${data.player1.id}`);
    await expect(page.getByRole('heading', { name: 'Javi' })).toBeVisible();
  });

  test('player can view another player profile', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/player/${data.player2.id}`);
    await expect(page.getByRole('heading', { name: 'Carlos' })).toBeVisible();
  });

  test('player can view H2H page', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/h2h');
    await expect(page.locator('body')).toBeVisible();
  });

  test('player can view notification preferences', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/profile/notifications');
    await expect(page.locator('body')).toBeVisible();
  });

  test('notification count loads', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');
    await expect(page.locator('[hx-get="/notifications/count"]').first()).toBeAttached({ timeout: 5000 });
  });
});
