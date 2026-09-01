import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, isMobile, openDrawer, navViaDrawer, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

const MOBILE = { width: 375, height: 812 };

async function checkNoOverflow(page: import('@playwright/test').Page) {
  const overflow = await page.evaluate(() => {
    return document.documentElement.scrollWidth > window.innerWidth;
  });
  expect(overflow, 'page should not have horizontal overflow').toBe(false);
}

test.describe('responsive - no horizontal overflow', () => {
  test('login page', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await page.goto('/login');
    await checkNoOverflow(page);
  });

  test('home page', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await checkNoOverflow(page);
    await expect(page.locator('[data-testid="player-competitions-heading"]')).toContainText('Mis competiciones');
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
  });

  test('admin dashboard', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin');
    await checkNoOverflow(page);
    await expect(page.getByText('Panel de administración')).toBeVisible();
    await expect(page.locator('.card-title', { hasText: 'Liga E2E Test' }).first()).toBeVisible();
  });

  test('competition page', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator(`a[href^="/competition/"]`, { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    await expect(page.locator('input[aria-label="Jornadas"]')).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
  });

  test('match detail', async ({ page }) => {
    const data = loadTestData();
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await checkNoOverflow(page);
  });

  test('match thread', async ({ page }) => {
    const data = loadTestData();
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await checkNoOverflow(page);
  });

  test('admin pairs', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/pairs');
    await checkNoOverflow(page);
    await expect(page.getByText('Pareja Alpha')).toBeVisible();
    await expect(page.getByText('Pareja Beta')).toBeVisible();
  });

  test('admin players', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/players');
    await checkNoOverflow(page);
    await expect(page.getByText('Test Player', { exact: true })).toBeVisible();
    await expect(page.getByText('Test Player 2', { exact: true })).toBeVisible();
  });

  test('admin venues', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/venues');
    await checkNoOverflow(page);
    await expect(page.getByText('Pista Central')).toBeVisible();
  });

  test('admin invitations', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/invitations');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: 'Invitaciones' })).toBeVisible();
  });

  test('admin disputes', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/disputes');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: 'Disputas pendientes' })).toBeVisible();
  });

  test('player profile', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openDrawer(page);
    await page.locator('.drawer-side a:has-text("Mi perfil")').click();
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: 'Test Player' })).toBeVisible();
  });

  test('notification prefs', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await navViaDrawer(page, '/profile/notifications');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: /Preferencias de notificaciones/i })).toBeVisible();
  });

  test('R-165: competition card badges stay within card bounds', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    // loginAs already navigates to / (home)
    await page.waitForLoadState('networkidle');

    const cards = page.locator('.card.overflow-hidden');
    const count = await cards.count();
    for (let i = 0; i < count; i++) {
      const card = cards.nth(i);
      const cardBox = await card.boundingBox();
      if (!cardBox) continue;
      const badges = card.locator('.badge');
      const badgeCount = await badges.count();
      for (let j = 0; j < badgeCount; j++) {
        const badgeBox = await badges.nth(j).boundingBox();
        if (!badgeBox) continue;
        expect(badgeBox.x + badgeBox.width, `badge ${j} in card ${i} right edge`).toBeLessThanOrEqual(cardBox.x + cardBox.width + 1);
      }
    }
  });

  test('R-170: dark mode renders readable text and admin indicator', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.evaluate(() => {
      document.documentElement.setAttribute('data-theme', 'dark');
      localStorage.setItem('theme', 'dark');
    });
    if (isMobile(page)) {
      await navViaDrawer(page, '/admin');
    } else {
      await page.locator('summary:has-text("Gestión")').click();
      await page.waitForTimeout(100);
      await page.locator('.menu-horizontal a[href="/admin"]').evaluate(el => (el as HTMLAnchorElement).click());
    }
    await page.waitForLoadState('networkidle');

    // Admin mode indicator (top-bar pill/dropdown) should be visible
    if (isMobile(page)) {
      await expect(page.locator('[aria-label="cambiar vista"]')).toBeVisible();
    } else {
      await expect(page.locator('details:has(a[href="/view/player"]) summary')).toBeVisible();
    }

    // Theme should be 'dark', not 'night'
    const theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'));
    expect(theme).toBe('dark');

    // Key text elements should be visible (not invisible due to low opacity)
    await expect(page.locator('h1:has-text("Panel de administración")')).toBeVisible();
    await expect(page.locator('text=Competiciones activas')).toBeVisible();
  });
});
