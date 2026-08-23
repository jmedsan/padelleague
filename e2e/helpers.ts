import { Page, expect } from '@playwright/test';

export async function loginAs(page: Page, email: string, password: string) {
  for (let attempt = 0; attempt < 5; attempt++) {
    const resp = await page.request.post('/login', {
      form: { email, password },
      maxRedirects: 0,
    });
    if (resp.status() === 429) {
      await page.waitForTimeout(12000);
      continue;
    }
    const headers = resp.headersArray();
    const setCookies = headers.filter(h => h.name.toLowerCase() === 'set-cookie');
    for (const sc of setCookies) {
      const match = sc.value.match(/pb_auth=([^;]+)/);
      if (match) {
        await page.context().addCookies([{
          name: 'pb_auth',
          value: match[1],
          domain: 'localhost',
          path: '/',
          httpOnly: true,
        }]);
      }
    }
    await page.goto('/');
    await expect(page).toHaveURL('/', { timeout: 5000 });
    return;
  }
  throw new Error('Login failed after 5 attempts (rate limited)');
}

export const ADMIN_EMAIL = process.env.ADMIN_EMAIL || 'admin@test.com';
export const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD || 'testpass123456';
export const PLAYER_EMAIL = process.env.PLAYER_EMAIL || 'player@test.com';
export const PLAYER_PASSWORD = process.env.PLAYER_PASSWORD || 'testpass123456';
