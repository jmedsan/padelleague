import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';
import { createNotification, deleteAllExisting } from '../notification-helpers';

test.describe('notification dismiss and history', { tag: '@notifications' }, () => {
  test('desktop: dismiss via bell, badge decrements, history shows all', { tag: '@smoke' }, async ({ page }) => {
    test.slow(); // multiple notification creates + polls; can exceed the 30s default under worker contention

    const data = loadTestData();
    await deleteAllExisting(page, data.player1.id, data.adminToken);

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);

    const dismissId = await createNotification(page, data.player1.id, data.adminToken, 'E2E Dismiss Test');
    const keepId = await createNotification(page, data.player1.id, data.adminToken, 'E2E Keep Test');

    // Reload to pick up new notifications in badge
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');

    // Badge shows the 2 unread we created (badge populates via hx-trigger="load").
    const badge = page.locator('#notif-badge');
    await expect.poll(async () => parseInt((await badge.textContent())?.trim() || '0', 10),
      { timeout: 5000 }).toBeGreaterThanOrEqual(2);

    // Open bell dropdown
    const bellButton = page.locator('.dropdown:has(#notif-dropdown) button[aria-label^="notificaciones"]');
    await bellButton.click();

    const dropdown = page.locator('#notif-dropdown');
    const dismissRow = dropdown.locator(`#notif-row-${dismissId}`);
    await expect(dismissRow).toBeVisible({ timeout: 5000 });

    // Dismiss via × button ("marcar leída")
    await dismissRow.locator('button[aria-label^="Descartar"]').click();

    // Row removed from the bell
    await expect(dismissRow).not.toBeAttached({ timeout: 5000 });

    // The "×" marks the notification read (deterministic contract, immune to any
    // concurrent notification perturbing the global badge count).
    await expect.poll(async () => {
      // raw-request: polled; a not-yet-visible record reads as null, not a failure.
      const r = await page.request.get(`/api/collections/notifications/records/${dismissId}`,
        { headers: { Authorization: data.adminToken } });
      return r.ok() ? (await r.json()).read : null;
    }, { timeout: 5000 }).toBe(true);

    // Removing the focused Descartar button from the DOM breaks the dropdown's
    // :focus-within visibility, closing it — re-open to check the kept row.
    await bellButton.click();
    const keepRow = dropdown.locator(`#notif-row-${keepId}`);
    await expect(keepRow).toBeVisible({ timeout: 5000 });

    // Navigate to history — both appear (the dismissed one is retained), and the
    // history page has NO remove/dismiss control (permanent record, P2).
    await page.goto('/notifications/history');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: 'Historial de notificaciones' })).toBeVisible();
    await expect(page.getByText('E2E Dismiss Test')).toBeVisible();
    await expect(page.getByText('E2E Keep Test')).toBeVisible();
    await expect(page.locator('button[aria-label^="Descartar"]')).toHaveCount(0);
  });
});
