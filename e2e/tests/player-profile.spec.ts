import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, isMobile, openDrawer, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test.describe('player profile and stats', () => {
  test('player can view own profile with stats', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await page.locator('input[aria-label="Clasificación"]').click();
    const pairLink = page.locator('table.table-zebra a[href^="/pair/"]').first();
    await expect(pairLink).toBeVisible({ timeout: 5000 });
    await pairLink.click();
    await page.waitForLoadState('domcontentloaded');
    const playerLink = page.locator('table a[href^="/player/"]').first();
    await expect(playerLink).toBeVisible();
    await playerLink.click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: /Test Player/ })).toBeVisible();
    await expect(page.locator('.stat-title', { hasText: 'Partidos' })).toBeVisible();
    await expect(page.locator('.stat-title', { hasText: '% Victorias' })).toBeVisible();
    await expect(page.getByText('Parejas')).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
  });

  test('player can view H2H via standings compare', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await page.selectOption('select[name="p1"]', data.pair1Id);
    await page.selectOption('select[name="p2"]', data.pair2Id);
    await page.getByRole('button', { name: 'Comparar' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Cara a cara' })).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
    await expect(page.getByText('Pareja Beta').first()).toBeVisible();
  });

  test('player can view notification preferences', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    if (isMobile(page)) {
      await openDrawer(page);
      await page.locator('.drawer-side a[href="/profile/notifications"]').click();
    } else {
      await page.goto('/profile/notifications');
    }
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: /Preferencias de notificaciones/i })).toBeVisible();
  });

  test('notification count loads', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await expect(page.locator('[hx-get="/notifications/count"]').first()).toBeAttached({ timeout: 5000 });
  });
});
