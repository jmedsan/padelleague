import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER_EMAIL, PLAYER_PASSWORD } from '../helpers';

const SCREENSHOTS_DIR = 'test-results/styles';

async function checkNoHorizontalOverflow(page: import('@playwright/test').Page) {
  const overflow = await page.evaluate(() => {
    return document.documentElement.scrollWidth > window.innerWidth;
  });
  expect(overflow, 'page should not have horizontal overflow').toBe(false);
}

test.describe('Login page styles', () => {
  test('desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goto('/login');
    await expect(page.locator('body')).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOTS_DIR}/login-desktop.png`, fullPage: true });
  });

  test('mobile - no overflow', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/login');
    await expect(page.locator('body')).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOTS_DIR}/login-mobile.png`, fullPage: true });
    await checkNoHorizontalOverflow(page);
  });
});

test.describe('Home page styles (authenticated)', () => {
  test('desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await loginAs(page, PLAYER_EMAIL, PLAYER_PASSWORD);
    await page.screenshot({ path: `${SCREENSHOTS_DIR}/home-desktop.png`, fullPage: true });
  });

  test('mobile - no overflow', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await loginAs(page, PLAYER_EMAIL, PLAYER_PASSWORD);
    await page.screenshot({ path: `${SCREENSHOTS_DIR}/home-mobile.png`, fullPage: true });
    await checkNoHorizontalOverflow(page);
  });
});

test.describe('Admin dashboard styles', () => {
  test('desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin');
    await expect(page.locator('body')).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOTS_DIR}/admin-desktop.png`, fullPage: true });
  });

  test('mobile - no overflow', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin');
    await expect(page.locator('body')).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOTS_DIR}/admin-mobile.png`, fullPage: true });
    await checkNoHorizontalOverflow(page);
  });
});

test.describe('Competition page styles', () => {
  test('desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await expect(page.locator('body')).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOTS_DIR}/competitions-desktop.png`, fullPage: true });
  });

  test('mobile - no overflow', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await expect(page.locator('body')).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOTS_DIR}/competitions-mobile.png`, fullPage: true });
    await checkNoHorizontalOverflow(page);
  });
});

test.describe('Notification dropdown styles', () => {
  test('dropdown opens', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await loginAs(page, PLAYER_EMAIL, PLAYER_PASSWORD);
    const bell = page.locator('[hx-get="/notifications/list"]').first();
    if (await bell.isVisible()) {
      await bell.click();
      await page.waitForTimeout(500);
      await page.screenshot({ path: `${SCREENSHOTS_DIR}/notification-dropdown.png` });
    } else {
      await page.screenshot({ path: `${SCREENSHOTS_DIR}/notification-no-bell.png` });
    }
  });
});
