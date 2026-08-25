import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER2_EMAIL, PLAYER2_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test.describe('match lifecycle', () => {
  test('player can view match detail', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
    await expect(page.getByText('Pareja Beta').first()).toBeVisible();
  });

  test('player can submit score', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${scratchMatchId("submit-score", testInfo.project.name)}`);
    await expect(page.locator('select[name="s1a"]')).toBeVisible({ timeout: 5000 });
    await page.selectOption('select[name="s1a"]', '6');
    await page.selectOption('select[name="s1b"]', '3');
    await page.selectOption('select[name="s2a"]', '6');
    await page.selectOption('select[name="s2b"]', '4');
    await page.getByRole('button', { name: 'Enviar resultado' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('6-3 6-4')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/esperando confirmaci[oó]n/i).first()).toBeVisible({ timeout: 5000 });
  });

  test('home page shows matches', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');
    await expect(page.locator('a[href^="/match/"]').first()).toBeVisible();
  });

  test('admin can set court number via override', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    const collapseTitle = page.locator('.collapse-title', { hasText: /corrección de administrador/i });
    await expect(collapseTitle).toBeVisible({ timeout: 5000 });
    await collapseTitle.locator('..').locator('input[type="checkbox"]').click();
    await page.fill('input[name="date"]', '2026-09-15');
    await page.fill('input[name="court_number"]', '3');
    await page.getByRole('button', { name: 'Aplicar cambios' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.badge', { hasText: 'Pista 3' })).toBeVisible({ timeout: 5000 });
  });

  test('player cannot access match of another competition', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/match/nonexistent-id');
    await expect(page.getByText('no encontrado')).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('link', { name: 'Volver al inicio' })).toBeVisible();
  });
});
