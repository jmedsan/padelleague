import { test, expect } from '@playwright/test';
import { loginAs, PLAYER_EMAIL, PLAYER_PASSWORD } from '../helpers';

test('player can view match thread', async ({ page }) => {
  await loginAs(page, PLAYER_EMAIL, PLAYER_PASSWORD);
  await page.goto('/');
  const matchLink = page.locator('a[href^="/match/"]').first();
  if (await matchLink.isVisible()) {
    await matchLink.click();
    await expect(page.locator('[name="content"]')).toBeVisible();
  }
});

test('player can post a message in thread', async ({ page }) => {
  await loginAs(page, PLAYER_EMAIL, PLAYER_PASSWORD);
  await page.goto('/');
  const matchLink = page.locator('a[href^="/match/"]').first();
  if (await matchLink.isVisible()) {
    await matchLink.click();
    const input = page.locator('input[name="content"]');
    if (await input.isVisible()) {
      const msg = `E2E test message ${Date.now()}`;
      await input.fill(msg);
      await page.getByRole('button', { name: 'Enviar' }).first().click();
      await expect(page.getByText(msg)).toBeVisible({ timeout: 5000 });
    }
  }
});
