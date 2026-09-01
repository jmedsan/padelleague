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

async function deleteAllExisting(page: import('@playwright/test').Page, userId: string, adminToken: string) {
  const resp = await page.request.get(`/api/collections/notifications/records?filter=user="${userId}"&perPage=200`, {
    headers: { Authorization: adminToken },
  });
  if (!resp.ok()) return;
  const body = await resp.json();
  for (const item of body.items || []) {
    await page.request.delete(`/api/collections/notifications/records/${item.id}`, {
      headers: { Authorization: adminToken },
    });
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

    // Badge shows count (2 unread)
    const badge = page.locator('#notif-badge');
    await expect(badge).toContainText('2', { timeout: 5000 });

    // Open bell dropdown
    const bellButton = page.locator('.dropdown:has(#notif-dropdown) button[aria-label="notificaciones"]');
    await bellButton.click();

    const dropdown = page.locator('#notif-dropdown');
    const dismissRow = dropdown.locator(`#notif-row-${dismissId}`);
    await expect(dismissRow).toBeVisible({ timeout: 5000 });

    // Dismiss via × button
    await dismissRow.locator('button[aria-label="descartar"]').click();

    // Row removed
    await expect(dismissRow).not.toBeAttached({ timeout: 5000 });

    // Badge decremented to 1
    await expect(badge).toContainText('1', { timeout: 5000 });

    // Re-open dropdown, kept row still present
    await bellButton.click();
    const keepRow = dropdown.locator(`#notif-row-${keepId}`);
    await expect(keepRow).toBeVisible({ timeout: 5000 });

    // Navigate to history — both appear
    await page.goto('/notifications/history');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: 'Historial de notificaciones' })).toBeVisible();
    await expect(page.getByText('E2E Dismiss Test')).toBeVisible();
    await expect(page.getByText('E2E Keep Test')).toBeVisible();
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

    // Mobile badge shows count
    const mobileBadge = page.locator('#notif-badge-mobile');
    await expect(mobileBadge).toContainText('2', { timeout: 5000 });

    // Open mobile bell dropdown
    const mobileDropdownContainer = page.locator('.lg\\:hidden .dropdown');
    const mobileBell = mobileDropdownContainer.locator('button[aria-label="notificaciones"]');
    await mobileBell.click();

    // Wait for dropdown to load
    const dismissRow = mobileDropdownContainer.locator(`#notif-row-${dismissId}`);
    await expect(dismissRow).toBeVisible({ timeout: 5000 });

    // Dismiss — click and wait for the HTMX request to complete
    const dismissBtn = dismissRow.locator('button[aria-label="descartar"]');
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/dismiss') && resp.status() === 200),
      dismissBtn.click(),
    ]);

    // Wait for OOB swap to update badge
    await expect(mobileBadge).toContainText('1', { timeout: 10000 });

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
