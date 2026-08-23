import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test('admin can view competitions page', async ({ page }) => {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto('/admin');
  await expect(page.getByText('Panel de administración')).toBeVisible();
});

test('admin can create a competition', async ({ page, browserName }, testInfo) => {
  const name = `E2E League ${testInfo.project.name} ${Date.now()}`;
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto('/admin');
  await page.getByRole('button', { name: /crear competición/i }).first().click();
  await page.fill('input[name="name"]', name);
  await page.selectOption('select[name="type"]', 'league');
  await page.locator('dialog button[type="submit"]').click();
  await page.waitForTimeout(2000);
  await page.goto('/admin');
  await expect(page.getByText(name)).toBeVisible({ timeout: 10000 });
});
