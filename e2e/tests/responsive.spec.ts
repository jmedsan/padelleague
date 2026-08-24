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
});
