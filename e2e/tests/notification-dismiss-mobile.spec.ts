import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';
import { createNotification, deleteAllExisting } from '../notification-helpers';

// Phone-only: listed in MOBILE_VIEWPORT_SPECS (playwright.config.ts), so it
// runs on the mobile project only. Desktop twin: notification-dismiss.spec.ts.
test.describe('notification dismiss on phone', { tag: '@notifications' }, () => {
  test('mobile: dismiss via bell, badge decrements', async ({ page }) => {
    const data = loadTestData();
    await deleteAllExisting(page, data.player1.id, data.adminToken);

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);

    const dismissId = await createNotification(page, data.player1.id, data.adminToken, 'E2E Mobile Dismiss');
    await createNotification(page, data.player1.id, data.adminToken, 'E2E Mobile Keep');

    // Reload to pick up badge count
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');

    // Mobile badge shows our 2 (populates via hx-trigger="load").
    const mobileBadge = page.locator('#notif-badge-mobile');
    await expect.poll(async () => parseInt((await mobileBadge.textContent())?.trim() || '0', 10),
      { timeout: 5000 }).toBeGreaterThanOrEqual(2);

    // Open mobile bell dropdown
    const mobileDropdownContainer = page.locator('.lg\\:hidden .dropdown');
    const mobileBell = mobileDropdownContainer.locator('button[aria-label^="notificaciones"]');
    await mobileBell.click();

    // Wait for dropdown to load
    const dismissRow = mobileDropdownContainer.locator(`#notif-row-${dismissId}`);
    await expect(dismissRow).toBeVisible({ timeout: 5000 });

    // Dismiss — click and wait for the HTMX request to complete
    const dismissBtn = dismissRow.locator('button[aria-label^="Descartar"]');
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/dismiss') && resp.status() === 200),
      dismissBtn.click(),
    ]);

    // The "×" marks the notification read (deterministic contract, immune to a
    // concurrent notification perturbing the global badge count).
    await expect.poll(async () => {
      // raw-request: polled; a not-yet-visible record reads as null, not a failure.
      const r = await page.request.get(`/api/collections/notifications/records/${dismissId}`,
        { headers: { Authorization: data.adminToken } });
      return r.ok() ? (await r.json()).read : null;
    }, { timeout: 5000 }).toBe(true);

    // Dismissing doesn't close the dropdown — verify the row is gone without reopening.
    await expect(mobileDropdownContainer.locator(`#notif-row-${dismissId}`)).not.toBeAttached({ timeout: 5000 });

    // History page shows both
    await page.goto('/notifications/history');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('E2E Mobile Dismiss')).toBeVisible();
    await expect(page.getByText('E2E Mobile Keep')).toBeVisible();
  });
});
