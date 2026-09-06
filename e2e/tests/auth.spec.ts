import { test, expect } from '@playwright/test';
import { loginAs, loginViaForm, isMobile, loadTestData, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

const BASE = `http://localhost:${process.env.E2E_PORT || 8099}`;
const UNVERIFIED_PASSWORD = 'testpass123456';

async function createUnverifiedPlayer(label: string): Promise<{ email: string; password: string }> {
  const email = `unverified-${label}-${Date.now()}@test.local`;
  const resp = await fetch(`${BASE}/api/collections/users/records`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: loadTestData().adminToken },
    body: JSON.stringify({
      email, display_name: `Sin Verificar ${label}`,
      gender: 'male', roles: ['player'],
      password: UNVERIFIED_PASSWORD, passwordConfirm: UNVERIFIED_PASSWORD,
      verified: false,
    }),
  });
  if (!resp.ok) throw new Error(`create unverified player: ${resp.status} ${await resp.text()}`);
  return { email, password: UNVERIFIED_PASSWORD };
}

test('unauthenticated access redirects to login', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});

test('login with valid credentials via form', async ({ page }) => {
  await loginViaForm(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  // Admin GET / redirects to /admin/competitions, the single admin landing page.
  await expect(page).toHaveURL(/\/admin\/competitions$/);
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
  await expect(page.getByRole('heading', { name: 'Competiciones', exact: true })).not.toBeVisible({ timeout: 3000 });
});

test('unverified-email banner can be dismissed and stays hidden for the session', async ({ page }) => {
  const player = await createUnverifiedPlayer(isMobile(page) ? 'mobile' : 'desktop');
  await loginAs(page, player.email, player.password);
  await page.goto('/');

  const banner = page.locator('#verify-banner');
  await expect(banner).toBeVisible({ timeout: 10000 });

  await banner.locator('button[aria-label="Descartar aviso"]').click();
  await expect(banner).toHaveCount(0);

  // Same session (sessionStorage persists) — a reload must not bring it back.
  await page.reload();
  await expect(page.locator('#verify-banner')).toHaveCount(0);
});

test('logout redirects to login', async ({ page }) => {
  await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
  await page.request.post('/logout');
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});
