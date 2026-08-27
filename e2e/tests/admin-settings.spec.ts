import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test.describe('admin settings', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/settings');
  });

  test('shows reset form with checkboxes', async ({ page }) => {
    await expect(page.getByText('Zona de peligro')).toBeVisible();
    await expect(page.locator('#chk-players')).toBeVisible();
    await expect(page.locator('#chk-pairs')).toBeVisible();
    await expect(page.locator('#chk-competitions')).toBeVisible();
    await expect(page.locator('#chk-matches')).toBeVisible();
  });

  test('DELETE gate rejects wrong confirmation', async ({ page }) => {
    await page.locator('#chk-players').check();
    await page.locator('#confirm-input').fill('WRONG');
    // Button should be disabled with wrong text
    await expect(page.locator('#reset-btn')).toBeDisabled();
  });

  test('checkbox dependency chain', async ({ page }) => {
    // Initially pairs is disabled
    await expect(page.locator('#chk-pairs')).toBeDisabled();

    // Check players -> pairs becomes enabled
    await page.locator('#chk-players').check();
    await expect(page.locator('#chk-pairs')).toBeEnabled();
    await expect(page.locator('#chk-competitions')).toBeDisabled();

    // Check pairs -> competitions becomes enabled
    await page.locator('#chk-pairs').check();
    await expect(page.locator('#chk-competitions')).toBeEnabled();

    // Uncheck players -> pairs becomes disabled and unchecked
    await page.locator('#chk-players').uncheck();
    await expect(page.locator('#chk-pairs')).toBeDisabled();
    await expect(page.locator('#chk-pairs')).not.toBeChecked();
    await expect(page.locator('#chk-competitions')).toBeDisabled();
  });

  test('mode section visible only when all checked', async ({ page }) => {
    await expect(page.locator('#mode-section')).toBeHidden();

    await page.locator('#chk-players').check();
    await page.locator('#chk-pairs').check();
    await page.locator('#chk-competitions').check();
    await page.locator('#chk-matches').check();

    await expect(page.locator('#mode-section')).toBeVisible();
  });

  test('players-only reset succeeds', async ({ page }) => {
    page.on('dialog', (dialog) => dialog.accept());

    await page.locator('#chk-players').check();
    await page.locator('#confirm-input').fill('DELETE');
    await page.locator('#reset-btn').click();

    await expect(page.locator('#reset-result')).toContainText('reiniciada');
  });

  test('full reset with sample league', async ({ page }) => {
    page.on('dialog', (dialog) => dialog.accept());

    await page.locator('#chk-players').check();
    await page.locator('#chk-pairs').check();
    await page.locator('#chk-competitions').check();
    await page.locator('#chk-matches').check();
    await page.locator('#confirm-input').fill('DELETE');
    await page.locator('input[name="mode"][value="sample"]').check();
    await page.locator('#reset-btn').click();

    await expect(page.locator('#reset-result')).toContainText('Liga de ejemplo creada');
  });

  test('dependency validation server-side', async ({ page }) => {
    page.on('dialog', (dialog) => dialog.accept());

    // Force-enable pairs checkbox via JS to bypass client-side dependency
    await page.locator('#confirm-input').fill('DELETE');
    await page.evaluate(() => {
      const chk = document.getElementById('chk-pairs') as HTMLInputElement;
      chk.disabled = false;
      chk.checked = true;
      const btn = document.getElementById('reset-btn') as HTMLButtonElement;
      btn.disabled = false;
    });
    await page.locator('#reset-btn').click();

    await expect(page.locator('#reset-result')).toContainText('borrar parejas requiere borrar jugadores');
  });
});
