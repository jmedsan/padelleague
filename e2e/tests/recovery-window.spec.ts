import { test, expect, Page, APIRequestContext } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD, loadTestData } from '../helpers';

let suToken = '';

test.describe('end-of-league recovery window', () => {
  test.describe.configure({ retries: 0 });

  test('pending match shows the recovery badge during the recovery window', async ({ page }) => {
    test.setTimeout(120000);
    await getSuperuserToken(page);
    const data = loadTestData();

    // end_date 5 days ago, default recovery_days (14) -> still in recovery.
    const endDate = new Date(Date.now() - 5 * 86400000).toISOString().slice(0, 10);
    const startDate = new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10);

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Recovery Badge E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      start_date: startDate,
      end_date: endDate,
      rounds: 1,
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

    const endDate = new Date(Date.now() - 5 * 86400000).toISOString().slice(0, 10);
    const startDate = new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10);

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Finalize E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      start_date: startDate,
      end_date: endDate,
      rounds: 1,
    });

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    await expect(page.getByText('En recuperación')).toBeVisible({ timeout: 10000 });

    page.once('dialog', dialog => dialog.accept());
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes(`/admin/competitions/${compId}/finalize`)),
      page.locator('[data-testid="finalize-league"]').click(),
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
  const resp = await request.post(`/api/collections/${collection}/records`, {
    headers: { Authorization: suToken, 'Content-Type': 'application/json' },
    data,
  });
  if (!resp.ok()) throw new Error(`Create ${collection} failed: ${resp.status()} ${await resp.text()}`);
  return (await resp.json()).id;
}

async function apiGetRecord(request: APIRequestContext, collection: string, id: string): Promise<any> {
  const resp = await request.get(`/api/collections/${collection}/records/${id}`, {
    headers: { Authorization: suToken },
  });
  if (!resp.ok()) throw new Error(`Get ${collection}/${id} failed: ${resp.status()}`);
  return await resp.json();
}

async function apiListRecords(request: APIRequestContext, collection: string, filter: string): Promise<any[]> {
  const resp = await request.get(`/api/collections/${collection}/records?filter=${encodeURIComponent(filter)}&perPage=50`, {
    headers: { Authorization: suToken },
  });
  if (!resp.ok()) throw new Error(`List ${collection} failed: ${resp.status()}`);
  return (await resp.json()).items || [];
}

async function apiDeleteRecord(request: APIRequestContext, collection: string, id: string) {
  await request.delete(`/api/collections/${collection}/records/${id}`, {
    headers: { Authorization: suToken },
  });
}
