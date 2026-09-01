import { test, expect, Page } from '@playwright/test';
import { loginAs, loadTestData, isMobile, openDrawer, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

async function navToAdmin(page: Page, href: string): Promise<void> {
  if (isMobile(page)) {
    await openDrawer(page);
    await page.locator(`.drawer-side a[href="${href}"]`).click();
  } else {
    await page.locator('summary:has-text("Gestión")').click();
    await page.waitForTimeout(100);
    await page.locator(`.menu-horizontal a[href="${href}"]`).evaluate(el => (el as HTMLAnchorElement).click());
  }
  await page.waitForLoadState('domcontentloaded');
}

test.describe('competition lifecycle', () => {
  test('admin can view dashboard with competitions', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin');
    await expect(page.getByText('Panel de administración')).toBeVisible();
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
  });

  test('admin can create a new competition', async ({ page, }, testInfo) => {
    const name = `Liga Nueva ${testInfo.project.name} ${Date.now()}`;
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin');
    await page.getByRole('button', { name: /crear competición/i }).first().click();
    await page.fill('input[name="name"]', name);
    await page.selectOption('select[name="type"]', 'league');
    await page.locator('dialog button[type="submit"]').click();
    await page.waitForURL(/\/admin/, { timeout: 10000 });
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText(name)).toBeVisible({ timeout: 10000 });
  });

  test('player can view competition standings', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    await page.locator('input[aria-label="Clasificación"]').click();
    const standingsTable = page.locator('table.table-zebra');
    await expect(standingsTable).toBeVisible({ timeout: 5000 });
    await expect(standingsTable.locator('td', { hasText: 'Pareja Alpha' })).toBeVisible();
    await expect(standingsTable.locator('td', { hasText: 'Pareja Beta' })).toBeVisible();
  });

  test('competition page shows match fixtures with round labels', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('input[aria-label="Jornadas"]')).toBeVisible();
    await expect(page.getByText(/Jornada \d/).first()).toBeVisible();
    const matchLinks = page.locator('a[href^="/match/"]');
    const count = await matchLinks.count();
    expect(count).toBeGreaterThan(0);
    await expect(matchLinks.first()).toContainText(/Pareja/);
  });

  test('admin can view competition detail', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin');
    await page.locator('a.card', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    const body = await page.textContent('body');
    expect(body).toContain('Pareja Alpha');
  });
});
