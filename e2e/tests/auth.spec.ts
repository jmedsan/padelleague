import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test('unauthenticated access redirects to login', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});

test('login with valid credentials', async ({ page }) => {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await expect(page).toHaveURL('/');
  await expect(page.locator('.navbar')).toBeVisible();
});

test('login with invalid credentials shows error', async ({ page }) => {
  await page.goto('/login');
  await page.fill('#email', 'wrong@test.com');
  await page.fill('#password', 'wrongpassword');
  await page.getByRole('button', { name: 'Entrar' }).click();
  await expect(page.locator('#error')).toContainText(/error|incorrecto/i);
});
