import { test, expect } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore, clickAndWaitForHxRedirect } from '../tour-helpers';
import {
  assertAssignmentInvariants, ScenarioApi, ScenarioData,
  apiGet, apiPatch, PLAYER_PASSWORD, loadCtx, ensureStage,
  competitionDates, jornadaTitle, jornadaHiISO,
} from '../scenario-helpers';

// playMatchToFinal drives one leveled match end-to-end through the UI (submit
// → accept), the same flow step 02 uses, and returns the two pairs involved.
// Shared by 02 and 02b so both top-up assertions play through the UI rather
// than finalizing via a raw API PATCH.
async function playMatchToFinal(
  page: import('@playwright/test').Page,
  api: ScenarioApi,
  matchId: string,
): Promise<{ pair1Id: string; pair2Id: string }> {
  const match = await apiGet(api, `/api/collections/matches/records/${matchId}`);
  const pair1Id = match.pair1;
  const pair2Id = match.pair2;

  const pair1Record = await apiGet(api, `/api/collections/pairs/records/${pair1Id}`);
  const pair2Record = await apiGet(api, `/api/collections/pairs/records/${pair2Id}`);
  const player1Record = await apiGet(api, `/api/collections/users/records/${pair1Record.player1}`);
  const player2Record = await apiGet(api, `/api/collections/users/records/${pair2Record.player1}`);

  // Set date and club first — required before score submission
  await apiPatch(api, `/api/collections/matches/records/${matchId}`, {
    date: '2036-10-02T10:00:00.000Z',
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

  const finalMatch = await apiGet(api, `/api/collections/matches/records/${matchId}`);
  expect(finalMatch.status).toBe('final');

  return { pair1Id, pair2Id };
}

let ctx: ScenarioData;
let api: ScenarioApi;
// Pairs that finished a match in step 02 and got topped up to slot 4 — step
// 02b needs one of these specifically (see its comment) rather than any
// slot-4 match, since a slot-4 opponent that hasn't itself exhausted its
// slots 1..open yet does not "want" a slot-5 top-up merely from this match.
let step02GraduatedPairs: string[] = [];

// ---------------------------------------------------------------------------
// Serial steps
// ---------------------------------------------------------------------------

test.describe('leveled-16 scenario', () => {
  test.describe.configure({ mode: 'serial' });
  test.describe.configure({ retries: 0 });

  test('00 baseline — 24 pending matches, invariants pass', async ({ page }) => {
    const raw = loadCtx();
    ctx = await ensureStage(
      { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie },
      'assigned',
    );
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };

    const data = await apiGet(api, `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`);
    const matches: any[] = data.items;
    const pending = matches.filter(m => m.status === 'pending');

    // 16 pairs × open(3)/2 = 24 nominal; accept [20, 32].
    expect(pending.length).toBeGreaterThanOrEqual(20);
    expect(pending.length).toBeLessThanOrEqual(32);

    await assertAssignmentInvariants(api, ctx);

    // Admin can navigate to the competition page
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/competition/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page).toHaveURL(new RegExp(`/competition/${ctx.competitionId}`));

    // The initial batch renders exactly `open` Jornada groups (1..open),
    // never a stray "Jornada 0" — the round-robin fallback title. Admin has
    // no own pair, so resolvePairFilter already defaults to "all".
    await page.locator('input[aria-label="Partidos"]').click();
    await page.waitForLoadState('domcontentloaded');
    for (let s = 1; s <= ctx.open; s++) {
      await expect(page.locator(`.collapse-title:has-text("Jornada ${s}")`).first()).toBeVisible({ timeout: 10000 });
    }
    await expect(page.locator('.collapse-title:has-text("Jornada 0")')).toHaveCount(0);
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

    // "Jornada 1" group is present
    await expect(page.locator('.collapse-title:has-text("Jornada 1")').first()).toBeVisible({ timeout: 10000 });

    // Pair filter dropdown is present and at least one match link is visible
    await expect(page.locator('select[name="pair"]')).toBeVisible();
    await expect(page.locator('a[href^="/match/"]').first()).toBeVisible({ timeout: 5000 });

    // Player info message: this pair hasn't reached target_matches yet, so
    // the "new matches get assigned after you finish" message is visible.
    await expect(page.locator('text=partidos pendientes')).toBeVisible({ timeout: 5000 });
  });

  test('02 play match → top-up fires', async ({ page }) => {
    // Pick a Jornada-1 pending match from the API — the top-up's slot-4
    // landing below is only pinned when the finishing pair's first three
    // slots (1..open) are already occupied, which is guaranteed for slot 1.
    const data = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(`competition='${ctx.competitionId}' && status='pending' && slot=1`)}&perPage=500`);
    const pending: any[] = data.items;
    expect(pending.length).toBeGreaterThan(0);
    const matchId = pending[0].id;

    const { pair1Id, pair2Id } = await playMatchToFinal(page, api, matchId);

    // Allow a moment for the top-up hook to fire
    await page.waitForTimeout(1000);

    // Both finishing pairs get exactly one new pending match, both in slot 4
    // (their next unoccupied Jornada, since 1..3 are already occupied — see
    // league.leveledState.occupy/candidateFor) with arrange_by pinned to the
    // last day of Jornada 4's window.
    const afterData = await apiGet(api, `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`);
    const afterMatches: any[] = afterData.items;

    const { startDate, endDate } = competitionDates();
    const jornada4Deadline = jornadaHiISO(startDate, endDate, ctx.target, 4);

    for (const pairId of [pair1Id, pair2Id]) {
      const pairSlot4After = afterMatches.filter(m => m.status === 'pending' && m.slot === 4 && (m.pair1 === pairId || m.pair2 === pairId));
      expect(pairSlot4After.length).toBe(1);
      expect(pairSlot4After[0].arrange_by.slice(0, 10)).toBe(jornada4Deadline);
    }

    await assertAssignmentInvariants(api, ctx);

    step02GraduatedPairs = [pair1Id, pair2Id];

    // The Jornada 4 group is visible in the UI, titled with its date range.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/competition/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await page.locator('input[aria-label="Partidos"]').click();
    await page.waitForLoadState('domcontentloaded');
    const jornada4Title = jornadaTitle(startDate, endDate, ctx.target, 4);
    await expect(page.locator(`.collapse-title:has-text("${jornada4Title}")`).first()).toBeVisible({ timeout: 10000 });
  });

  test('02b play a Jornada-4 match → next top-up lands in slot 5', async ({ page }) => {
    // A pair only "wants" a new assignment once its pending count drops below
    // `open` (league.leveledState.wants) — its slot-4 opponent from the
    // initial top-up (a third pair whose own slots 1..open are still
    // untouched) does not itself want a slot-5 top-up from this match. Play
    // one of step 02's own graduated pairs' slot-4 match, so at least one
    // side genuinely triggers the next top-up.
    expect(step02GraduatedPairs.length).toBe(2);
    const data = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(`competition='${ctx.competitionId}' && status='pending' && slot=4`)}&perPage=500`);
    const pending: any[] = data.items;
    const match = pending.find(m => step02GraduatedPairs.includes(m.pair1) || step02GraduatedPairs.includes(m.pair2));
    expect(match, 'need the slot-4 match for one of step 02\'s graduated pairs').toBeTruthy();
    const matchId = match.id;
    const graduatedPairId = step02GraduatedPairs.includes(match.pair1) ? match.pair1 : match.pair2;

    await playMatchToFinal(page, api, matchId);

    await page.waitForTimeout(1000);

    const afterData = await apiGet(api, `/api/collections/matches/records?filter=competition='${ctx.competitionId}'&perPage=500`);
    const afterMatches: any[] = afterData.items;

    const { startDate, endDate } = competitionDates();
    const jornada5Deadline = jornadaHiISO(startDate, endDate, ctx.target, 5);

    const pairSlot5After = afterMatches.filter(m => m.status === 'pending' && m.slot === 5 && (m.pair1 === graduatedPairId || m.pair2 === graduatedPairId));
    expect(pairSlot5After.length).toBe(1);
    expect(pairSlot5After[0].arrange_by.slice(0, 10)).toBe(jornada5Deadline);

    // No pair exceeds open+1 pending across the whole competition.
    await assertAssignmentInvariants(api, ctx);

    // The Jornada 5 group is visible in the UI, titled with its date range.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/competition/${ctx.competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await page.locator('input[aria-label="Partidos"]').click();
    await page.waitForLoadState('domcontentloaded');
    const jornada5Title = jornadaTitle(startDate, endDate, ctx.target, 5);
    await expect(page.locator(`.collapse-title:has-text("${jornada5Title}")`).first()).toBeVisible({ timeout: 10000 });
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
    // Pick two pending matches with the same slot (so both land in the same
    // Jornada group and the sort assertion is meaningful) that don't share
    // a pair, so filtering by matchA's pair legitimately excludes matchB.
    const data = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(`competition='${ctx.competitionId}' && status='pending'`)}&perPage=500`);
    const pending: any[] = data.items;
    expect(pending.length).toBeGreaterThanOrEqual(2);
    let matchA: any;
    let matchB: any;
    outer: for (const a of pending) {
      if (a.slot < 1 || a.slot > ctx.open) continue; // must land in a Jornada group
      const aPairs = new Set([a.pair1, a.pair2]);
      for (const b of pending) {
        if (b.id === a.id || b.slot !== a.slot) continue;
        if (aPairs.has(b.pair1) || aPairs.has(b.pair2)) continue;
        matchA = a;
        matchB = b;
        break outer;
      }
    }
    expect(matchB, 'need two same-Jornada pending matches with no shared pair').toBeTruthy();

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
        arrange_by: '2036-10-02',
      });
      await apiPatch(api, `/api/collections/matches/records/${matchB.id}`, {
        arrange_by: '2036-10-16',
      });

      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await page.goto(`/admin/competitions/${ctx.competitionId}`);
      await page.waitForLoadState('domcontentloaded');

      // Expand matchA/matchB's shared Jornada group and confirm matchB
      // (pending, later date) appears before matchA (scheduled, earlier date).
      const jornadaGroup = page.locator(`.collapse-title:has-text("Jornada ${matchA.slot}")`);
      await expect(jornadaGroup).toBeVisible({ timeout: 10000 });
      await jornadaGroup.locator('..').locator('input[type="checkbox"]').click();
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
