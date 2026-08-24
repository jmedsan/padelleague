import { Page, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';

export const ADMIN_EMAIL = 'admin@test.com';
export const ADMIN_PASSWORD = 'testpass123456';
export const PLAYER1_EMAIL = 'player@test.com';
export const PLAYER1_PASSWORD = 'testpass123456';
export const PLAYER2_EMAIL = 'player2@test.com';
export const PLAYER2_PASSWORD = 'testpass123456';

export interface TestData {
  adminToken: string;
  player1: { id: string; email: string };
  player2: { id: string; email: string };
  pair1Id: string;
  pair2Id: string;
  competitionId: string;
  matchIds: string[];
  venueId: string;
}

let _cachedData: TestData | null = null;

export function loadTestData(): TestData {
  if (!_cachedData) {
    const raw = readFileSync(join(__dirname, '.test-data/seed.json'), 'utf-8');
    _cachedData = JSON.parse(raw);
  }
  return _cachedData!;
}

export async function loginAs(page: Page, email: string, password: string) {
  for (let attempt = 0; attempt < 5; attempt++) {
    const resp = await page.request.post('/api/collections/users/auth-with-password', {
      data: { identity: email, password },
    });
    if (resp.status() === 429) {
      await new Promise(r => setTimeout(r, 15000));
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
