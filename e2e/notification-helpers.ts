import { apiCreateRecord, apiListRecords, apiDeleteRecord } from './helpers';

export async function createNotification(page: import('@playwright/test').Page, userId: string, adminToken: string, title: string) {
  return apiCreateRecord(page.request, adminToken, 'notifications', { user: userId, title, type: 'general', read: false });
}

// Delete every notification for the user, repeating until none remain. A
// preceding test (e.g. mobile-lifecycle's accept → NotifResultConfirmed to
// player1) can write a notification asynchronously that lands just after a
// single delete pass; polling to empty flushes that in-flight leak so this
// test starts from a genuinely clean baseline and its absolute count holds.
export async function deleteAllExisting(page: import('@playwright/test').Page, userId: string, adminToken: string) {
  for (let attempt = 0; attempt < 6; attempt++) {
    const items = await apiListRecords(page.request, adminToken, 'notifications', `user="${userId}"`);
    if (items.length === 0) return;
    for (const item of items) {
      await apiDeleteRecord(page.request, adminToken, 'notifications', item.id);
    }
    // brief wait so any in-flight cross-test notification settles before re-checking
    await page.waitForTimeout(300);
  }
}
