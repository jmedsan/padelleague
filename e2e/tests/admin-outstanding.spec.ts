import type { Page, APIRequestContext } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import {
  loginAs, ADMIN_EMAIL, ADMIN_PASSWORD, loadTestData,
  apiCreateRecord as apiCreateRecordBase, apiListRecords as apiListRecordsBase,
  apiDeleteRecord as apiDeleteRecordBase,
} from '../helpers';

let suToken = '';

test.describe('admin outstanding matches', { tag: '@admin' }, () => {
  test.describe.configure({ retries: 0 });

  test('admin sees a seeded pending match on the outstanding-matches page', async ({ page }) => {
    test.setTimeout(120000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Outstanding E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
    });

    await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'pending',
      round_number: 1,
    });

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/outstanding');

    const panel = page.locator('[data-testid="outstanding-list"]');
    await expect(panel).toBeVisible();
    await expect(panel).toContainText('Outstanding E2E');

    // Cleanup
    const matches = await apiListRecords(page.request, 'matches', `competition='${compId}'`);
    for (const m of matches) await apiDeleteRecord(page.request, 'matches', m.id);
    await apiDeleteRecord(page.request, 'competitions', compId);
  });
});

// --- API helpers ---

async function getSuperuserToken(page: Page) {
  if (suToken) return;
  for (let attempt = 0; attempt < 5; attempt++) {
    const resp = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    if (resp.status() === 429) {
      await new Promise(r => setTimeout(r, 15000));
      continue;
    }
    if (!resp.ok()) throw new Error(`Superuser auth failed: ${resp.status()}`);
    suToken = (await resp.json()).token;
    return;
  }
  throw new Error('Superuser auth failed after 5 attempts (rate limited)');
}

async function apiCreateRecord(request: APIRequestContext, collection: string, data: Record<string, any>): Promise<string> {
  return apiCreateRecordBase(request, suToken, collection, data);
}

async function apiListRecords(request: APIRequestContext, collection: string, filter: string): Promise<any[]> {
  return apiListRecordsBase(request, suToken, collection, filter);
}

async function apiDeleteRecord(request: APIRequestContext, collection: string, id: string): Promise<void> {
  await apiDeleteRecordBase(request, suToken, collection, id);
}
