import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test.describe('admin settings', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/settings');
  });

  test('shows reset form with example checkboxes', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Reiniciar base de datos' })).toBeVisible();
    await expect(page.getByText('Datos de ejemplo a cargar', { exact: true })).toBeVisible();
    await expect(page.locator('#chk-players')).toBeVisible();
    await expect(page.locator('#chk-pairs')).toBeVisible();
    await expect(page.locator('#chk-competitions')).toBeVisible();
    await expect(page.locator('#chk-matches')).toBeVisible();
    // Blocking overlay + spinner present but hidden until a reset runs.
    await expect(page.locator('#reset-overlay')).toBeAttached();
    await expect(page.locator('#reset-overlay')).toBeHidden();
    await expect(page.locator('#reset-overlay .loading-spinner')).toBeAttached();
  });

  test('DELETE gate: enabled on DELETE alone (from scratch), disabled otherwise', async ({ page }) => {
    await expect(page.locator('#reset-btn')).toBeDisabled();
    await page.locator('#confirm-input').fill('WRONG');
    await expect(page.locator('#reset-btn')).toBeDisabled();
    // No checkbox required — an empty selection means "from scratch".
    await page.locator('#confirm-input').fill('DELETE');
    await expect(page.locator('#reset-btn')).toBeEnabled();
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

  test('reset from scratch wipes to an empty database', async ({ page }) => {
    await page.locator('#confirm-input').fill('DELETE');
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/admin/settings/reset')),
      page.locator('#reset-btn').click(),
    ]);
    await expect(page.locator('#reset-result')).toContainText('vacía');
  });

  test('reset and load the full example league', async ({ page }) => {
    await page.locator('#chk-players').check();
    await page.locator('#chk-pairs').check();
    await page.locator('#chk-competitions').check();
    await page.locator('#chk-matches').check();
    await page.locator('#confirm-input').fill('DELETE');
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/admin/settings/reset')),
      page.locator('#reset-btn').click(),
    ]);
    await expect(page.locator('#reset-result')).toContainText('ejemplo');
  });
});
