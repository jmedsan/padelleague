import { test, expect } from '@playwright/test';
import { loginAs, loginViaForm, isMobile, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test('unauthenticated access redirects to login', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});

test('login with valid credentials via form', async ({ page }) => {
  await loginViaForm(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await expect(page).toHaveURL('/');
  await expect(page.locator('.navbar')).toBeVisible();
  if (isMobile(page)) {
    await expect(page.locator('[aria-label="cambiar vista"]')).toBeVisible();
  } else {
    await expect(page.locator('details:has(a[href="/view/player"]) summary')).toBeVisible();
  }
});

test('login with invalid credentials shows error', async ({ page }) => {
  await page.goto('/login');
  await page.fill('#email', 'wrong@test.com');
  await page.fill('#password', 'wrongpassword');
  await page.getByRole('button', { name: 'Entrar' }).click();
  await expect(page.locator('.alert-error')).toBeVisible({ timeout: 5000 });
});

test('player cannot access admin page', async ({ page }) => {
  await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
  await page.goto('/admin');
  await expect(page.getByRole('heading', { name: 'Competiciones' })).not.toBeVisible({ timeout: 3000 });
});

test('logout redirects to login', async ({ page }) => {
  await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
  await page.request.post('/logout');
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});
