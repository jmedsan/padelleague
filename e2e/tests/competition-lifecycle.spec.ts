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
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/admin') && resp.status() < 400),
      page.locator('dialog button[type="submit"]').click(),
    ]);
    await page.goto('/admin');
    await expect(page.getByText(name)).toBeVisible({ timeout: 10000 });
  });

  test('player can view competition standings', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${data.competitionId}`);
    await expect(page.locator('body')).toBeVisible();
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

  test('admin can view competition detail', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${data.competitionId}`);
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
  });
});
