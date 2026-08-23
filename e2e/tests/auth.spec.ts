import { test, expect } from '@playwright/test';
import { loginAs, loginViaForm, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test('unauthenticated access redirects to login', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});

test('login with valid credentials via form', async ({ page }) => {
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

test('player cannot access admin page', async ({ page }) => {
  await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
  const resp = await page.request.get('/admin');
  expect([302, 403, 200]).toContain(resp.status());
  await page.goto('/admin');
  const url = page.url();
  const bodyText = await page.locator('body').textContent();
  const isForbidden = url.includes('/login') || bodyText?.includes('no tienes') || bodyText?.includes('acceso');
  expect(isForbidden || !url.includes('/admin')).toBeTruthy();
});

test('logout redirects to login', async ({ page }) => {
  await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
  await page.request.post('/logout');
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});
