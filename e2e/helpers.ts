import { Page, expect } from '@playwright/test';

export async function loginAs(page: Page, email: string, password: string) {
  for (let attempt = 0; attempt < 5; attempt++) {
    const resp = await page.request.post('/api/collections/users/auth-with-password', {
      data: { identity: email, password },
    });
    if (resp.status() === 429) {
      await page.waitForTimeout(15000);
      continue;
    }
    if (!resp.ok()) {
      throw new Error(`Login failed: ${resp.status()} ${await resp.text()}`);
    }
    const body = await resp.json();
    await page.context().addCookies([{
      name: 'pb_auth',
      value: body.token,
      domain: 'localhost',
      path: '/',
      httpOnly: true,
    }]);
    await page.goto('/');
    await expect(page).toHaveURL('/', { timeout: 5000 });
    return;
  }
  throw new Error('Login failed after 5 attempts (rate limited)');
}

export async function loginViaForm(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.fill('#email', email);
  await page.fill('#password', password);
  await page.getByRole('button', { name: 'Entrar' }).click();
  await page.waitForURL('/', { timeout: 10000 });
}

export const ADMIN_EMAIL = process.env.ADMIN_EMAIL || 'admin@test.com';
export const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD || 'testpass123456';
export const PLAYER_EMAIL = process.env.PLAYER_EMAIL || 'player@test.com';
export const PLAYER_PASSWORD = process.env.PLAYER_PASSWORD || 'testpass123456';
