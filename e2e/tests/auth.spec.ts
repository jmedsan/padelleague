import { test, expect } from '@playwright/test';
import { loginViaForm, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test('unauthenticated access redirects to login', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});

test('login with valid credentials', async ({ page }) => {
  await loginViaForm(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await expect(page).toHaveURL('/');
  await expect(page.locator('body')).toBeVisible();
});

test('login with invalid credentials shows error', async ({ page }) => {
  await page.goto('/login');
  await page.fill('#email', 'wrong@test.com');
  await page.fill('#password', 'wrongpassword');
  await page.getByRole('button', { name: 'Entrar' }).click();
  await expect(page.locator('.alert-error')).toBeVisible({ timeout: 5000 });
});
