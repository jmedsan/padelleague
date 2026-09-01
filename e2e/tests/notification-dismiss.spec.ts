import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, isMobile, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

async function createNotification(page: import('@playwright/test').Page, userId: string, adminToken: string, title: string) {
  const resp = await page.request.post('/api/collections/notifications/records', {
    headers: { Authorization: adminToken },
    data: { user: userId, title, type: 'general', read: false, dismissed: false },
  });
  if (!resp.ok()) throw new Error(`create notification: ${resp.status()} ${await resp.text()}`);
  return (await resp.json()).id as string;
}

test.describe('notification dismiss and history', () => {
  test('dismiss removes notification from bell, history shows all', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);

    const dismissId = await createNotification(page, data.player1.id, data.adminToken, 'E2E Dismiss Test');
    const keepId = await createNotification(page, data.player1.id, data.adminToken, 'E2E Keep Test');

    if (isMobile(page)) {
      await page.goto('/notifications/history');
      await page.waitForLoadState('domcontentloaded');
      await expect(page.getByText('E2E Dismiss Test')).toBeVisible();
      await expect(page.getByText('E2E Keep Test')).toBeVisible();
      return;
    }

    // Desktop: open the bell dropdown
    const bellButton = page.locator('.dropdown:has(#notif-dropdown) button[aria-label="notificaciones"]');
    await bellButton.click();

    const dropdown = page.locator('#notif-dropdown');
    await expect(dropdown).toBeVisible();

    // Wait for HTMX to load notifications, then verify our rows
    const dismissRow = dropdown.locator(`#notif-row-${dismissId}`);
    const keepRow = dropdown.locator(`#notif-row-${keepId}`);
    await expect(dismissRow).toBeVisible({ timeout: 5000 });
    await expect(keepRow).toBeVisible();

    // Dismiss via × button
    await dismissRow.locator('button[aria-label="descartar"]').click();

    // Dismissed row removed from DOM
    await expect(dismissRow).not.toBeAttached({ timeout: 5000 });

    // Re-open dropdown (dismiss click may close it)
    await bellButton.click();
    await expect(dropdown.locator('[id^="notif-row-"]').first()).toBeVisible({ timeout: 5000 });

    // Keep row still present, dismiss row gone
    await expect(keepRow).toBeVisible();
    await expect(dismissRow).not.toBeAttached();

    // Navigate to history — both appear (dismissed one faded)
    await page.goto('/notifications/history');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: 'Historial de notificaciones' })).toBeVisible();
    await expect(page.getByText('E2E Dismiss Test')).toBeVisible();
    await expect(page.getByText('E2E Keep Test')).toBeVisible();
  });
});
