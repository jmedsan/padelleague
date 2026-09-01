import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, isMobile, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

async function createNotification(page: import('@playwright/test').Page, userId: string, adminToken: string, title: string) {
  const resp = await page.request.post('/api/collections/notifications/records', {
    headers: { Authorization: adminToken },
    data: { user: userId, title, type: 'general', read: false },
  });
  if (!resp.ok()) throw new Error(`create notification: ${resp.status()} ${await resp.text()}`);
  return (await resp.json()).id as string;
}

// Delete every notification for the user, repeating until none remain. A
// preceding test (e.g. mobile-lifecycle's accept → NotifResultConfirmed to
// player1) can write a notification asynchronously that lands just after a
// single delete pass; polling to empty flushes that in-flight leak so this
// test starts from a genuinely clean baseline and its absolute count holds.
async function deleteAllExisting(page: import('@playwright/test').Page, userId: string, adminToken: string) {
  for (let attempt = 0; attempt < 6; attempt++) {
    const resp = await page.request.get(`/api/collections/notifications/records?filter=user="${userId}"&perPage=200`, {
      headers: { Authorization: adminToken },
    });
    if (!resp.ok()) return;
    const items = (await resp.json()).items || [];
    if (items.length === 0) return;
    for (const item of items) {
      await page.request.delete(`/api/collections/notifications/records/${item.id}`, {
        headers: { Authorization: adminToken },
      });
    }
    // brief wait so any in-flight cross-test notification settles before re-checking
    await page.waitForTimeout(300);
  }
}

test.describe('notification dismiss and history', () => {
  test('desktop: dismiss via bell, badge decrements, history shows all', async ({ page }) => {
    if (isMobile(page)) { test.skip(); return; }

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
    const bellButton = page.locator('.dropdown:has(#notif-dropdown) button[aria-label="notificaciones"]');
    await bellButton.click();

    const dropdown = page.locator('#notif-dropdown');
    const dismissRow = dropdown.locator(`#notif-row-${dismissId}`);
    await expect(dismissRow).toBeVisible({ timeout: 5000 });

    // Dismiss via × button ("marcar leída")
    await dismissRow.locator('button[aria-label="marcar leída"]').click();

    // Row removed from the bell
    await expect(dismissRow).not.toBeAttached({ timeout: 5000 });

    // The "×" marks the notification read (deterministic contract, immune to any
    // concurrent notification perturbing the global badge count).
    await expect.poll(async () => {
      const r = await page.request.get(`/api/collections/notifications/records/${dismissId}`,
        { headers: { Authorization: data.adminToken } });
      return r.ok() ? (await r.json()).read : null;
    }, { timeout: 5000 }).toBe(true);

    // Re-open dropdown, kept row still present
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
    await expect(page.locator('button[aria-label="marcar leída"]')).toHaveCount(0);
    await expect(page.locator('button[aria-label="descartar"]')).toHaveCount(0);
  });

  test('mobile: dismiss via bell, badge decrements', async ({ page }) => {
    if (!isMobile(page)) { test.skip(); return; }

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
    const mobileBell = mobileDropdownContainer.locator('button[aria-label="notificaciones"]');
    await mobileBell.click();

    // Wait for dropdown to load
    const dismissRow = mobileDropdownContainer.locator(`#notif-row-${dismissId}`);
    await expect(dismissRow).toBeVisible({ timeout: 5000 });

    // Dismiss — click and wait for the HTMX request to complete
    const dismissBtn = dismissRow.locator('button[aria-label="marcar leída"]');
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/dismiss') && resp.status() === 200),
      dismissBtn.click(),
    ]);

    // The "×" marks the notification read (deterministic contract, immune to a
    // concurrent notification perturbing the global badge count).
    await expect.poll(async () => {
      const r = await page.request.get(`/api/collections/notifications/records/${dismissId}`,
        { headers: { Authorization: data.adminToken } });
      return r.ok() ? (await r.json()).read : null;
    }, { timeout: 5000 }).toBe(true);

    // Re-open dropdown to verify row is gone
    await mobileBell.click();
    await expect(mobileDropdownContainer.locator(`#notif-row-${dismissId}`)).not.toBeAttached({ timeout: 5000 });

    // History page shows both
    await page.goto('/notifications/history');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('E2E Mobile Dismiss')).toBeVisible();
    await expect(page.getByText('E2E Mobile Keep')).toBeVisible();
  });
});
