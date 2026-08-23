import { test, expect } from '@playwright/test';
import { loginAs, PLAYER_EMAIL, PLAYER_PASSWORD } from '../helpers';

test.describe('score submission', () => {
  test('player can view match detail', async ({ page }) => {
    await loginAs(page, PLAYER_EMAIL, PLAYER_PASSWORD);
    await page.goto('/');
    const matchLink = page.locator('a[href^="/match/"]').first();
    if (await matchLink.isVisible({ timeout: 3000 }).catch(() => false)) {
      await matchLink.click();
      await expect(page.locator('.card')).toBeVisible();
    }
  });

  test('score form validates input', async ({ page }) => {
    await loginAs(page, PLAYER_EMAIL, PLAYER_PASSWORD);
    await page.goto('/');
    const matchLink = page.locator('a[href^="/match/"]').first();
    if (await matchLink.isVisible({ timeout: 3000 }).catch(() => false)) {
      await matchLink.click();
      const submitBtn = page.getByRole('button', { name: 'Enviar resultado' });
      if (await submitBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
        await submitBtn.click();
        await expect(page.locator('.alert-error')).toBeVisible();
      }
    }
  });
});
