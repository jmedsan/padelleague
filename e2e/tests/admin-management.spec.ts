import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test.describe('admin management', () => {
  test('admin can view pairs page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/pairs');
    await expect(page.getByText('Pareja Alpha')).toBeVisible();
    await expect(page.getByText('Pareja Beta')).toBeVisible();
  });

  test('admin can view players page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/players');
    await expect(page.getByText('Test Player', { exact: true })).toBeVisible();
    await expect(page.getByText('Test Player 2', { exact: true })).toBeVisible();
  });

  test('admin can view venues page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/venues');
    await expect(page.getByText('Pista Central')).toBeVisible();
  });

  test('admin can view invitations page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/invitations');
    await expect(page.getByRole('heading', { name: 'Invitaciones' })).toBeVisible();
    await expect(page.getByRole('button', { name: /nueva invitaci[oó]n/i })).toBeVisible();
  });

  test('admin can view disputes page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/disputes');
    await expect(page.getByRole('heading', { name: 'Disputas pendientes' })).toBeVisible();
  });

  test('admin can create invitation', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/invitations');
    await page.getByRole('button', { name: /nueva invitaci[oó]n/i }).click();
    const invEmail = `inv-${Date.now()}@test.com`;
    await page.locator('#modal-create input[name="email"]').fill(invEmail);
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#modal-create button[type="submit"]').click(),
    ]);
    await page.goto('/admin/invitations');
    await expect(page.getByText(invEmail).first()).toBeVisible({ timeout: 5000 });
  });

  test('admin can create venue', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/venues');
    const name = `Club ${Date.now()}`;
    await page.getByRole('button', { name: /nuevo club/i }).click();
    await page.locator('#modal-create-venue input[name="name"]').fill(name);
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#modal-create-venue button[type="submit"]').click(),
    ]);
    await page.goto('/admin/venues');
    await expect(page.getByText(name)).toBeVisible({ timeout: 5000 });
  });

  test('R-166: category field is a dropdown with Spanish labels', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.locator('a:has-text("Competiciones")').first().click();
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: /crear competici[oó]n/i }).click();
    const dialog = page.locator('dialog#modal-create');
    await expect(dialog).toBeVisible();
    const select = dialog.locator('select[name="category"]');
    await expect(select).toBeVisible();
    const options = await select.locator('option').allTextContents();
    expect(options).toContain('1ª categoría');
    expect(options).toContain('Mixta');
    expect(options).toContain('Femenina');
  });
});
