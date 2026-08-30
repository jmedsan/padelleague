import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test.describe('competition lifecycle', () => {
  test('admin can view dashboard with competitions', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin');
    await expect(page.getByText('Panel de administración')).toBeVisible();
    await expect(page.getByText('Liga E2E Test')).toBeVisible();
  });

  test('admin can create a new competition', async ({ page, }, testInfo) => {
    const name = `Liga Nueva ${testInfo.project.name} ${Date.now()}`;
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin');
    await page.getByRole('button', { name: /crear competición/i }).first().click();
    await page.fill('input[name="name"]', name);
    await page.selectOption('select[name="type"]', 'league');
    await page.locator('dialog button[type="submit"]').click();
    await page.waitForURL(/\/admin/, { timeout: 10000 });
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText(name)).toBeVisible({ timeout: 10000 });
  });

  test('player can view competition standings', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${data.competitionId}`);
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
    await expect(page.getByText('Pareja Beta').first()).toBeVisible();
  });

  test('competition page shows match fixtures', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${data.competitionId}`);
    const matchLinks = page.locator('a[href^="/match/"]');
    const count = await matchLinks.count();
    expect(count).toBeGreaterThan(0);
  });

  test('h2h comparison form navigates with pair params', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${data.competitionId}`);
    await page.selectOption('select[name="p1"]', data.pair1Id);
    await page.selectOption('select[name="p2"]', data.pair2Id);
    await page.getByRole('button', { name: 'Comparar' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page).toHaveURL(/\/h2h\?p1=.*&p2=.*/);
    await expect(page.getByRole('heading', { name: 'Cara a cara' })).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
    await expect(page.getByText('Pareja Beta').first()).toBeVisible();
  });

  test('admin can view competition detail', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${data.competitionId}`);
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
  });
});
