import { test, expect, Page, APIRequestContext } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD, loadTestData } from '../helpers';

let suToken = '';

test.describe('scheduling, walkover & bracket', () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name === 'destructive', 'skip destructive project');
  });

  test.describe.configure({ retries: 0 });

  test('urgent task cards show on player home with warning badges', async ({ page }) => {
    test.setTimeout(120000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const now = new Date();
    const startDate = new Date(now.getTime() - 14 * 86400000).toISOString().slice(0, 10);
    const endDate = new Date(now.getTime() - 5 * 86400000).toISOString().slice(0, 10);

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Urgentes E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      start_date: startDate,
      end_date: endDate,
      arrange_grace_days: 3,
      rounds: 1,
    });

    await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'pending',
      round_number: 1,
    });

    await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'disputed',
      round_number: 1,
      scores: '6-3 6-4',
      dispute_notes: 'Test dispute',
    });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');

    const actions = page.locator('[data-testid="home-actions"]');
    await expect(actions).toBeVisible({ timeout: 10000 });

    // Dispute action with error accent (filter to the test competition)
    const disputeAction = actions.locator('a').filter({ hasText: 'Urgentes E2E' }).filter({ hasText: 'Disputa abierta' });
    await expect(disputeAction).toBeVisible();
    await expect(disputeAction).toHaveClass(/bg-error/);

    // Organize action with recovery badge (end_date 5 days ago, within 14-day recovery)
    const organizeAction = actions.locator('a').filter({ hasText: /Organiza antes del/ });
    await expect(organizeAction).toBeVisible({ timeout: 5000 });
    await expect(organizeAction.locator('[data-testid="recovery-badge"]')).toBeVisible();

    // Cleanup
    const matches = await apiListRecords(page.request, 'matches', `competition='${compId}'`);
    for (const m of matches) await apiDeleteRecord(page.request, 'matches', m.id);
    await apiDeleteRecord(page.request, 'competitions', compId);
  });

  test('walkover: report unplayed → admin approves → final with penalty', async ({ page }) => {
    test.setTimeout(120000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Walkover E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      walkover_score: '6-0 6-0',
      default_penalty: 5,
      rounds: 1,
    });

    const matchId = await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'pending',
      round_number: 1,
    });

    // Player reports the match as unplayed via the real UI form.
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.getByRole('button', { name: 'Reportar partido no jugado' }).click();
    const dialog = page.locator(`dialog#walkover-modal-${matchId}`);
    await expect(dialog).toBeVisible({ timeout: 3000 });
    await dialog.locator('textarea[name="reason"]').fill('El rival no se presentó.');
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes(`/match/${matchId}/report-unplayed`)),
      dialog.locator('button:has-text("Reportar no jugado")').click(),
    ]);
    await page.waitForLoadState('networkidle');

    const matchAfterReport = await apiGetRecord(page.request, 'matches', matchId);
    expect(matchAfterReport.status).toBe('disputed');
    expect(matchAfterReport.review_type).toBe('walkover');
    expect(matchAfterReport.walkover_requested_by).toBe(data.player1.id);

    // Admin approves via disputes page
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/disputes');
    await expect(page.getByText('Solicitud de incomparecencia')).toBeVisible({ timeout: 10000 });

    const woForm = page.locator(`form[hx-post*="/admin/disputes/${matchId}/walkover-approve"]`);
    await woForm.locator('select[name="winner"]').selectOption(data.pair1Id);
    await woForm.locator('button:has-text("Aprobar incomparecencia")').click();
    await expect(woForm).not.toBeVisible({ timeout: 10000 });

    // Verify final state
    const matchFinal = await apiGetRecord(page.request, 'matches', matchId);
    expect(matchFinal.status).toBe('final');
    expect(matchFinal.scores).toBe('6-0 6-0');
    expect(matchFinal.winner).toBe(data.pair1Id);

    // Check standings show penalty
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${compId}`);
    const standingsTab = page.locator('input[aria-label="Clasificación"]');
    await standingsTab.click();
    await page.waitForSelector('table.table-zebra tbody tr', { timeout: 5000 });
    await expect(page.locator('table.table-zebra .text-error').filter({ hasText: '-5' })).toBeVisible();

    await page.screenshot({ path: '/tmp/claude-1000/-mnt-data-Dev-PadelLeague/1bb535f8-6b3f-49b6-85d1-278927d6a279/scratchpad/walkover-standings.png', fullPage: true });

    // Cleanup
    await apiDeleteRecord(page.request, 'matches', matchId);
    await apiDeleteRecord(page.request, 'competitions', compId);
  });

  test('playoff bracket renders at mobile viewport with Spanish round names', async ({ page, browser }) => {
    test.setTimeout(120000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const pair3Id = await apiCreateRecord(page.request, 'pairs', {
      name: 'Pareja Gamma',
      player1: data.player1.id,
      player2: data.player2.id,
    });
    const pair4Id = await apiCreateRecord(page.request, 'pairs', {
      name: 'Pareja Delta',
      player1: data.player1.id,
      player2: data.player2.id,
    });

    // Create playoff via API with pairs and seeding
    const allPairs = [data.pair1Id, data.pair2Id, pair3Id, pair4Id];
    const seeding: Record<string, number> = {};
    allPairs.forEach((id, i) => { seeding[id] = i + 1; });

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Playoff E2E',
      type: 'playoff',
      active: true,
      pairs: allPairs,
      seeding: JSON.stringify(seeding),
    });

    // Generate fixtures via admin UI
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    const genNav = page.waitForEvent('framenavigated', { timeout: 15000 });
    await page.locator('button:has-text("Generar calendario")').click();
    await genNav;
    await page.waitForLoadState('domcontentloaded');

    // Open at 375px mobile viewport
    const mobileContext = await browser.newContext({ viewport: { width: 375, height: 812 } });
    const mobilePage = await mobileContext.newPage();
    await loginAs(mobilePage, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await mobilePage.goto(`/competition/${compId}`);

    await expect(mobilePage.getByText('Cuadro')).toBeVisible({ timeout: 10000 });
    await expect(mobilePage.getByText('Semifinal')).toBeVisible();
    await expect(mobilePage.getByText('Final').first()).toBeVisible();

    // Bracket cards: 2 semis + 1 final = 3 match cards
    const bracketCards = mobilePage.locator('.card.shadow-sm.border');
    const cardCount = await bracketCards.count();
    expect(cardCount).toBeGreaterThanOrEqual(3);

    await mobilePage.screenshot({ path: '/tmp/claude-1000/-mnt-data-Dev-PadelLeague/1bb535f8-6b3f-49b6-85d1-278927d6a279/scratchpad/bracket-mobile-375.png', fullPage: true });
    await mobileContext.close();

    // Cleanup
    const matches = await apiListRecords(page.request, 'matches', `competition='${compId}'`);
    for (const m of matches) await apiDeleteRecord(page.request, 'matches', m.id);
    await apiDeleteRecord(page.request, 'competitions', compId);
    await apiDeleteRecord(page.request, 'pairs', pair3Id);
    await apiDeleteRecord(page.request, 'pairs', pair4Id);
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
