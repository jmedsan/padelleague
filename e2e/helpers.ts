import { Page } from '@playwright/test';

export async function loginAs(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.fill('#email', email);
  await page.fill('#password', password);
  await page.getByRole('button', { name: 'Entrar' }).click();
  await page.waitForURL('/', { timeout: 15000, waitUntil: 'domcontentloaded' });
}

export const ADMIN_EMAIL = process.env.ADMIN_EMAIL || 'admin@test.com';
export const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD || 'testpass123456';
export const PLAYER_EMAIL = process.env.PLAYER_EMAIL || 'player@test.com';
export const PLAYER_PASSWORD = process.env.PLAYER_PASSWORD || 'testpass123456';
