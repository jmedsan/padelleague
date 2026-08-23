import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test('admin can view competitions page', async ({ page }) => {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto('/admin/competitions');
  await expect(page.getByText('Competiciones')).toBeVisible();
});

test('admin can create a competition', async ({ page }) => {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto('/admin/competitions/new');
  await page.fill('input[name="name"]', 'E2E Test League');
  await page.selectOption('select[name="type"]', 'league');
  await page.getByRole('button', { name: /crear/i }).click();
  await expect(page.getByText('E2E Test League')).toBeVisible();
});
