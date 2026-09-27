import type { Page, APIRequestContext } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import {
  loginAs, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD, loadTestData, leagueDate,
  apiCreateRecord as apiCreateRecordBase, apiGetRecord as apiGetRecordBase,
  apiListRecords as apiListRecordsBase, apiDeleteRecord as apiDeleteRecordBase,
} from '../helpers';

let suToken = '';

test.describe('end-of-league recovery window', () => {
  test.describe.configure({ retries: 0 });

  test('pending match shows the recovery badge during the recovery window', { tag: '@smoke' }, async ({ page }) => {
    test.setTimeout(120000);
    await getSuperuserToken(page);
    const data = loadTestData();

    // end_date 5 days ago, default recovery_days (14) -> still in recovery.
    const endDate = leagueDate(-5);
    const startDate = leagueDate(-30);

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Recovery Badge E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      start_date: startDate,
      end_date: endDate,
      rounds: 1,
      calendar_status: 'published',
    });

    await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'pending',
      round_number: 1,
    });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');
    await expect(page.locator('[data-testid="recovery-badge"]').first()).toBeVisible({ timeout: 10000 });

    // Cleanup
    const matches = await apiListRecords(page.request, 'matches', `competition='${compId}'`);
    for (const m of matches) await apiDeleteRecord(page.request, 'matches', m.id);
    await apiDeleteRecord(page.request, 'competitions', compId);
  });

  test('admin can finalize a league early, ending its recovery window', async ({ page }) => {
    test.setTimeout(120000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const endDate = leagueDate(-5);
    const startDate = leagueDate(-30);

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Finalize E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      start_date: startDate,
      end_date: endDate,
      rounds: 1,
      calendar_status: 'published',
    });

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    await expect(page.getByText('En recuperación')).toBeVisible({ timeout: 10000 });

    // hx-confirm is intercepted by static/js/confirm.js's custom
    // #confirm-modal, not the native window.confirm() — the request only
    // fires once #confirm-ok is clicked.
    await page.locator('[data-testid="finalize-league"]').click();
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes(`/admin/competitions/${compId}/finalize`)),
      page.locator('#confirm-ok').click(),
    ]);
    await page.waitForLoadState('networkidle');

    const comp = await apiGetRecord(page.request, 'competitions', compId);
    expect(comp.finalized).toBe(true);
    await expect(page.locator('[data-testid="finalize-league"]')).not.toBeVisible();
    await expect(page.getByText('Finalizada', { exact: true })).toBeVisible();

    // Cleanup
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

async function apiGetRecord(request: APIRequestContext, collection: string, id: string): Promise<any> {
  return apiGetRecordBase(request, suToken, collection, id);
}

async function apiListRecords(request: APIRequestContext, collection: string, filter: string): Promise<any[]> {
  return apiListRecordsBase(request, suToken, collection, filter);
}

async function apiDeleteRecord(request: APIRequestContext, collection: string, id: string): Promise<void> {
  await apiDeleteRecordBase(request, suToken, collection, id);
}
