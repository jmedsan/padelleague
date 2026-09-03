import { test, expect, Page } from '@playwright/test';
import { loginAs, isMobile, openDrawer, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

async function navToAdmin(page: Page, href: string): Promise<void> {
  if (isMobile(page)) {
    await openDrawer(page);
    await page.locator(`.drawer-side a[href="${href}"]`).click();
  } else {
    await page.locator('summary:has-text("Gestión")').click();
    await page.waitForTimeout(100);
    await page.locator(`.menu-horizontal a[href="${href}"]`).evaluate(el => (el as HTMLAnchorElement).click());
  }
  await page.waitForLoadState('domcontentloaded');
}

test.describe('admin settings', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/settings');
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
    // Sample-data boxes default to checked and enabled (all dependencies satisfied).
    await expect(page.locator('#chk-players')).toBeChecked();
    await expect(page.locator('#chk-pairs')).toBeChecked();
    await expect(page.locator('#chk-pairs')).toBeEnabled();

    // Uncheck players -> pairs and everything downstream become disabled and unchecked
    await page.locator('#chk-players').uncheck();
    await expect(page.locator('#chk-pairs')).toBeDisabled();
    await expect(page.locator('#chk-pairs')).not.toBeChecked();
    await expect(page.locator('#chk-competitions')).toBeDisabled();
    await expect(page.locator('#chk-competitions')).not.toBeChecked();

    // Re-check players -> pairs re-enabled but not auto-checked; competitions stays disabled
    await page.locator('#chk-players').check();
    await expect(page.locator('#chk-pairs')).toBeEnabled();
    await expect(page.locator('#chk-pairs')).not.toBeChecked();
    await expect(page.locator('#chk-competitions')).toBeDisabled();

    // Check pairs -> competitions becomes enabled
    await page.locator('#chk-pairs').check();
    await expect(page.locator('#chk-competitions')).toBeEnabled();
  });

  test('reset from scratch wipes to an empty database', async ({ page }) => {
    // Boxes default to checked; unchecking players cascades all sample-data off,
    // so an empty selection means "from scratch".
    await page.locator('#chk-players').uncheck();
    await expect(page.locator('#chk-pairs')).not.toBeChecked();
    await expect(page.locator('#chk-matches')).not.toBeChecked();
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

    // Sample data seeds each finalized match's proposer/confirmer with a
    // read=true notification (createSampleNotifications in seed/sample.go).
    // Log in as the first sample player and confirm the bell dropdown shows
    // at least one already-read entry, not just unread ones.
    await loginAs(page, 'sample-p1@padelleague.com', 'padel1234');
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');

    const bellButton = isMobile(page)
      ? page.locator('.dropdown:has(#notif-badge-mobile) button[aria-label="notificaciones"]')
      : page.locator('.dropdown:has(#notif-dropdown) button[aria-label="notificaciones"]');
    await bellButton.click();

    const dropdown = isMobile(page)
      ? page.locator('.dropdown:has(#notif-badge-mobile) .dropdown-content')
      : page.locator('#notif-dropdown');
    await expect(dropdown.locator('a')).not.toHaveCount(0);

    // Unread rows carry bg-neutral/5 font-medium; a read row has neither class.
    const readRow = dropdown.locator('a:not(.font-medium)').first();
    await expect(readRow).toBeVisible();
  });
});
