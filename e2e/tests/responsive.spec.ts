import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

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
  });

  test('admin dashboard', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin');
    await checkNoOverflow(page);
  });

  test('competition page', async ({ page }) => {
    const data = loadTestData();
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${data.competitionId}`);
    await checkNoOverflow(page);
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
    await page.goto('/admin/pairs');
    await checkNoOverflow(page);
  });

  test('admin players', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/players');
    await checkNoOverflow(page);
  });

  test('admin venues', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/venues');
    await checkNoOverflow(page);
  });

  test('admin invitations', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/invitations');
    await checkNoOverflow(page);
  });

  test('admin disputes', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/disputes');
    await checkNoOverflow(page);
  });

  test('player profile', async ({ page }) => {
    const data = loadTestData();
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/player/${data.player1.id}`);
    await checkNoOverflow(page);
  });

  test('notification prefs', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/profile/notifications');
    await checkNoOverflow(page);
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
    await page.goto('/admin/competitions');
    await page.waitForLoadState('networkidle');

    // Admin mode indicator (top-bar pill/dropdown) should be visible
    const indicator = page.locator('[aria-label="cambiar vista"], details:has(a[href="/view/player"]) summary').first();
    await expect(indicator).toBeVisible();

    // Theme should be 'dark', not 'night'
    const theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'));
    expect(theme).toBe('dark');

    // Key text elements should be visible (not invisible due to low opacity)
    await expect(page.locator('h1:has-text("Panel de administración")')).toBeVisible();
    await expect(page.locator('text=Competiciones activas')).toBeVisible();
  });
});
