import { test, expect } from '@playwright/test';
import { loginAs, PLAYER_EMAIL, PLAYER_PASSWORD } from '../helpers';

test.use({ viewport: { width: 375, height: 812 } });

test('no horizontal overflow on home page', async ({ page }) => {
  await loginAs(page, PLAYER_EMAIL, PLAYER_PASSWORD);
  await page.goto('/');
  const hasOverflow = await page.evaluate(() => {
    return document.documentElement.scrollWidth > window.innerWidth;
  });
  expect(hasOverflow).toBe(false);
});

test('no horizontal overflow on login page', async ({ page }) => {
  await page.goto('/login');
  const hasOverflow = await page.evaluate(() => {
    return document.documentElement.scrollWidth > window.innerWidth;
  });
  expect(hasOverflow).toBe(false);
});
