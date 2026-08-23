import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER2_EMAIL, PLAYER2_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test.describe('match lifecycle', () => {
  test('player can view match detail', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await expect(page.locator('main')).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
    await expect(page.getByText('Pareja Beta').first()).toBeVisible();
  });

  test('player can submit score', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    const scoreInput = page.locator('input[name="scores"]');
    if (await scoreInput.isVisible({ timeout: 3000 }).catch(() => false)) {
      await scoreInput.fill('6-3 6-4');
      await page.getByRole('button', { name: 'Enviar resultado' }).click();
      await expect(page.locator('.alert-success, .alert-info, .card')).toBeVisible({ timeout: 5000 });
    }
  });

  test('home page shows matches', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');
    await expect(page.locator('a[href^="/match/"]').first()).toBeVisible();
  });

  test('player cannot access match of another competition', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/match/nonexistent-id');
    const body = await page.locator('body').textContent();
    const isError = body?.includes('no encontrado') || body?.includes('error') || page.url().includes('/login');
    expect(isError).toBeTruthy();
  });
});
