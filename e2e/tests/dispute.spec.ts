import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test('admin can view disputes page', async ({ page }) => {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto('/admin/disputes');
  await expect(page).toHaveURL('/admin/disputes');
});

test('dispute page shows disputed matches or empty state', async ({ page }) => {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto('/admin/disputes');
  const content = page.locator('main, .drawer-content');
  await expect(content).toBeVisible();
});
