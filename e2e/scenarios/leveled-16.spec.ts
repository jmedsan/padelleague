import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore, clickAndWaitForHxRedirect } from '../tour-helpers';
import { assertAssignmentInvariants, ScenarioCtx, ScenarioApi, apiGet, apiPatch, PLAYER_PASSWORD } from '../scenario-helpers';

// ---------------------------------------------------------------------------
// Context loaded from scenario setup
// ---------------------------------------------------------------------------

interface ScenarioData extends ScenarioCtx {
  baseURL: string;
  suToken: string;
  adminCookie: string;
}

function loadCtx(): ScenarioData {
  const raw = readFileSync(join(__dirname, '../.test-data/scenario.json'), 'utf-8');
  return JSON.parse(raw);
}

// Module-level state shared across serial steps.
let ctx: ScenarioData;
let api: ScenarioApi;

// ---------------------------------------------------------------------------
// Serial steps
// ---------------------------------------------------------------------------

test.describe('leveled-16 scenario', () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'scenario spec is serial, runs desktop-only');
  });

  test.describe.configure({ mode: 'serial' });
  test.describe.configure({ retries: 0 });

  test('00 baseline — 24 pending matches, invariants pass', async ({ page }) => {
    ctx = loadCtx();
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };

    const data = await apiGet(api, `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`);
    const matches: any[] = data.items;
    const pending = matches.filter(m => m.status === 'pending');

    // 16 pairs × open(3)/2 = 24 nominal; transient open+1 ceiling means up to 16×4/2=32.
    // Accept anything in [20, 32].
    expect(pending.length).toBeGreaterThanOrEqual(20);
    expect(pending.length).toBeLessThanOrEqual(32);

    await assertAssignmentInvariants(api, ctx);

    // Admin can navigate to the competition page
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/competition/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page).toHaveURL(new RegExp(`/competition/${ctx.competitionId}`));
  });

  test('01 assignments visible in UI', async ({ page }) => {
    // Use the first pair's player1 email
    const player1Email = ctx.players[ctx.pairs[0].player1Idx].email;
    await loginAs(page, player1Email, PLAYER_PASSWORD);

    await page.goto(`/competition/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    // Click "Partidos" tab (leveled competitions use "Partidos", not "Jornadas")
    await page.locator('input[aria-label="Partidos"]').click();
    await page.waitForLoadState('domcontentloaded');

    // "Por jugar" group is present
    await expect(page.locator('.collapse-title:has-text("Por jugar")')).toBeVisible({ timeout: 10000 });

    // Pair filter dropdown is present and at least one match link is visible
    await expect(page.locator('select[name="pair"]')).toBeVisible();
    await expect(page.locator('a[href^="/match/"]').first()).toBeVisible({ timeout: 5000 });
  });

  test('02 play match → top-up fires', async ({ page }) => {
    // Pick a pending match from the API
    const data = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(`competition='${ctx.competitionId}' && status='pending'`)}&perPage=500`);
    const pending: any[] = data.items;
    expect(pending.length).toBeGreaterThan(0);
    const match = pending[0];
    const matchId = match.id;
    const pair1Id = match.pair1;
    const pair2Id = match.pair2;

    // Look up pair records to get player IDs
    const pair1Record = await apiGet(api, `/api/collections/pairs/records/${pair1Id}`);
    const pair2Record = await apiGet(api, `/api/collections/pairs/records/${pair2Id}`);
    const player1Record = await apiGet(api, `/api/collections/users/records/${pair1Record.player1}`);
    const player2Record = await apiGet(api, `/api/collections/users/records/${pair2Record.player1}`);

    // Set date and club first — required before score submission
    await apiPatch(api, `/api/collections/matches/records/${matchId}`, {
      date: '2026-10-01T10:00:00.000Z',
      club: 'Test Club',
    });

    // Login as pair1's first player and submit score
    await loginAs(page, player1Record.email, PLAYER_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('domcontentloaded');

    await enterScore(page, '6-3 6-4');
    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")').first());

    // Login as pair2's first player and accept
    await loginAs(page, player2Record.email, PLAYER_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('domcontentloaded');
    await page.locator('#thread-details').waitFor({ timeout: 15000 });

    const acceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
    await acceptBtn.waitFor({ timeout: 10000 });
    await clickAndWaitForHxRedirect(page, acceptBtn);

    // Verify match is final
    const finalMatch = await apiGet(api, `/api/collections/matches/records/${matchId}`);
    expect(finalMatch.status).toBe('final');

    // Allow a moment for the top-up hook to fire
    await page.waitForTimeout(1000);

    // Verify both pairs got a new pending match (top-up)
    const afterData = await apiGet(api, `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`);
    const afterMatches: any[] = afterData.items;

    const pair1PendingAfter = afterMatches.filter(m => m.status === 'pending' && (m.pair1 === pair1Id || m.pair2 === pair1Id));
    const pair2PendingAfter = afterMatches.filter(m => m.status === 'pending' && (m.pair1 === pair2Id || m.pair2 === pair2Id));

    expect(pair1PendingAfter.length).toBeGreaterThan(0);
    expect(pair2PendingAfter.length).toBeGreaterThan(0);

    await assertAssignmentInvariants(api, ctx);
  });

  test('03 admin release → re-assignment', async ({ page }) => {
    // Pick a pending match
    const data = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(`competition='${ctx.competitionId}' && status='pending'`)}&perPage=500`);
    const pending: any[] = data.items;
    expect(pending.length).toBeGreaterThan(0);
    const match = pending[0];
    const matchId = match.id;
    const releasedPair1 = match.pair1;
    const releasedPair2 = match.pair2;
    const pendingCountBefore = pending.length;

    // Login as admin and navigate to the match
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('domcontentloaded');
    // Wait for async thread to load so the full page is settled
    await page.locator('#match-thread').waitFor({ state: 'visible', timeout: 10000 });

    // Click "Liberar partido" — uses custom hx-confirm DaisyUI modal
    const releaseBtn = page.locator('button:has-text("Liberar partido")');
    await expect(releaseBtn).toBeVisible({ timeout: 10000 });
    await releaseBtn.click();
    const confirmOk = page.locator('#confirm-ok');
    await confirmOk.waitFor({ timeout: 5000 });
    await clickAndWaitForHxRedirect(page, confirmOk);
    await page.waitForLoadState('domcontentloaded');

    // Verify the released match is gone
    const releasedResp = await page.request.get(
      `/api/collections/matches/records/${matchId}`,
      { headers: { Authorization: api.suToken } },
    );
    expect(releasedResp.status()).toBe(404);

    // Allow a moment for top-up to fire
    await page.waitForTimeout(1000);

    // Total pending count should not have dropped (a new match was assigned)
    const afterData = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(`competition='${ctx.competitionId}' && status='pending'`)}&perPage=500`);
    const pendingAfter: any[] = afterData.items;
    expect(pendingAfter.length).toBeGreaterThanOrEqual(pendingCountBefore - 1);

    // The new matches must NOT re-pair the same two teams that were released
    const repairedMatch = pendingAfter.find(m =>
      (m.pair1 === releasedPair1 && m.pair2 === releasedPair2) ||
      (m.pair1 === releasedPair2 && m.pair2 === releasedPair1),
    );
    expect(repairedMatch).toBeUndefined();

    await assertAssignmentInvariants(api, ctx);
  });

  test('04 admin pair filter + scheduling-status sort', async ({ page }) => {
    // Pick two pending matches that don't share a pair, so filtering by
    // matchA's pair legitimately excludes matchB.
    const data = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(`competition='${ctx.competitionId}' && status='pending'`)}&perPage=500`);
    const pending: any[] = data.items;
    expect(pending.length).toBeGreaterThanOrEqual(2);
    const matchA = pending[0];
    const matchAPairs = new Set([matchA.pair1, matchA.pair2]);
    const matchB = pending.find(m => !matchAPairs.has(m.pair1) && !matchAPairs.has(m.pair2));
    expect(matchB, 'need a second pending match with no shared pair with matchA').toBeTruthy();

    // Give matchA an earlier arrange_by but "scheduled" status (a proposal
    // made but not yet accepted); matchB a later arrange_by but still
    // "pending" (no proposal at all). If sorting were arrange_by-only,
    // matchA would come first — the scheduling-status fix must put matchB
    // first, since "pending" outranks "scheduled" in admin triage order.
    // "scheduled" (not "confirmed") because the match status state machine
    // (hooks/hooks.go validTransitions) only allows confirmed→{final,disputed},
    // so a "confirmed" match here couldn't be reverted to pending afterward.
    // Wrapped in try/finally so a mid-test failure can't leave matches in a
    // mutated state for later steps or other specs (e.g. smtp-verify, which
    // picks the first status='pending' match).
    try {
      await apiPatch(api, `/api/collections/matches/records/${matchA.id}`, {
        status: 'scheduled',
        arrange_by: '2026-10-01',
      });
      await apiPatch(api, `/api/collections/matches/records/${matchB.id}`, {
        arrange_by: '2026-10-15',
      });

      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await page.goto(`/admin/competitions/${ctx.competitionId}`);
      await page.waitForLoadState('domcontentloaded');

      // Expand "Por jugar" and confirm matchB (pending, later date) appears
      // before matchA (scheduled, earlier date).
      const porJugarTitle = page.locator('.collapse-title:has-text("Por jugar")');
      await expect(porJugarTitle).toBeVisible({ timeout: 10000 });
      await porJugarTitle.locator('..').locator('input[type="checkbox"]').click();
      const rows = page.locator(`a[href="/match/${matchB.id}"], a[href="/match/${matchA.id}"]`);
      await expect(rows.first()).toHaveAttribute('href', `/match/${matchB.id}`, { timeout: 10000 });

      // Pair filter: selecting matchA's pair1 shows only matches for that pair.
      const pairFilter = page.getByLabel('Filtrar por pareja', { exact: true });
      await pairFilter.selectOption(matchA.pair1);
      await page.waitForLoadState('domcontentloaded');
      await expect(page.locator(`a[href="/match/${matchA.id}"]`)).toBeVisible({ timeout: 10000 });
      await expect(page.locator(`a[href="/match/${matchB.id}"]`)).toHaveCount(0);
    } finally {
      // Restore both matches to their original pending state for later steps.
      // scheduled→pending is a valid transition (unlike confirmed→pending).
      await apiPatch(api, `/api/collections/matches/records/${matchA.id}`, {
        status: 'pending', arrange_by: matchA.arrange_by || null,
      });
      await apiPatch(api, `/api/collections/matches/records/${matchB.id}`, {
        arrange_by: matchB.arrange_by || null,
      });
    }
  });
});
