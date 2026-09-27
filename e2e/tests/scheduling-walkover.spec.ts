import type { Page, APIRequestContext } from '@playwright/test';
import { test, expect, checkAll } from '../overflow-guard';
import {
  loginAs, isMobile, leagueDate, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD, loadTestData,
  apiCreateRecord as apiCreateRecordBase, apiGetRecord as apiGetRecordBase,
  apiListRecords as apiListRecordsBase, apiDeleteRecord as apiDeleteRecordBase, suPatch,
} from '../helpers';
import { clickAndWaitForHxRedirect, clickConfirmAndWaitForHxRedirect, generateFixtures } from '../tour-helpers';

let suToken = '';

test.describe('scheduling, walkover & bracket', { tag: '@scheduling' }, () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name === 'destructive', 'skip destructive project');
  });

  test.describe.configure({ retries: 0 });

  test('urgent task cards show on player home with warning badges', async ({ page }) => {
    test.setTimeout(120000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const startDate = leagueDate(-14);
    const endDate = leagueDate(-5);

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Urgentes E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      start_date: startDate,
      end_date: endDate,
      arrange_grace_days: 3,
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

  test('walkover: request arbitration (no_show) → admin approves → final with penalty', { tag: '@smoke' }, async ({ page }, testInfo) => {
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
      calendar_status: 'published',
    });

    const matchId = await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'pending',
      round_number: 1,
      // A date is required for the arbitration affordance to show (W5: can't
      // request arbitration on a match with no scheduled date at all).
      date: '2026-12-01',
      club: 'Padel 360',
    });

    // Player requests arbitration (category no_show) via the real UI form.
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.locator('button.btn-outline', { hasText: 'Solicitar arbitraje' }).click();
    const dialog = page.locator(`dialog#arbitration-modal-${matchId}`);
    await expect(dialog).toBeVisible({ timeout: 3000 });
    await dialog.locator('select[name="category"]').selectOption('no_show');
    await dialog.locator('textarea[name="notes"]').fill('El rival no se presentó.');
    // RequestArbitration responds with redirectHX (HX-Redirect) to /match/{id}
    // — same match page it's already on. clickAndWaitForHxRedirect asserts
    // the header target and waits for the real document navigation, so the
    // next loginAs's page.goto('/') never races an in-flight redirect.
    await clickAndWaitForHxRedirect(page, dialog.locator('button:has-text("Solicitar arbitraje")'), `/match/${matchId}`);

    const matchAfterReport = await apiGetRecord(page.request, 'matches', matchId);
    expect(matchAfterReport.status).toBe('disputed');
    expect(matchAfterReport.review_type).toBe('walkover');
    expect(matchAfterReport.arbitration).toBe('no_show');
    expect(matchAfterReport.walkover_requested_by).toBe(data.player1.id);

    // no_show sets status=disputed directly, so the walkover-specific alert
    // shows on the match page instead of the generic "Arbitraje solicitado"
    // badge (that badge is reserved for categories that don't change status —
    // see the scheduling-category test below).
    await expect(page.getByText('Solicitud de partido no jugado')).toBeVisible({ timeout: 5000 });

    // Admin approves from the match page — /admin/disputes is now a compact
    // link list (healthItemRow), the walkover-approve form lives on the
    // match detail page it links to.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/disputes');
    await page.locator(`a[href="/match/${matchId}"]`).click();
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Solicitud de partido no jugado')).toBeVisible({ timeout: 10000 });

    const woForm = page.locator(`form[hx-post*="/admin/disputes/${matchId}/walkover-approve"]`);
    await woForm.locator('select[name="winner"]').selectOption(data.pair1Id);
    // hx-confirm is intercepted by static/js/confirm.js's custom #confirm-modal
    // (not the native confirm() dialog), so the request only fires once its
    // #confirm-ok button is clicked. WalkoverApprove also responds with
    // redirectHX — wait for the real navigation before the next loginAs.
    await clickConfirmAndWaitForHxRedirect(page, woForm.locator('button:has-text("Aprobar incomparecencia")'), `/admin/competitions/${compId}`);

    // Verify final state
    const matchFinal = await apiGetRecord(page.request, 'matches', matchId);
    expect(matchFinal.status).toBe('final');
    expect(matchFinal.scores).toBe('6-0 6-0');
    expect(matchFinal.winner).toBe(data.pair1Id);

    // Check standings show penalty
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${compId}`);
    const standingsTab = page.locator('input[aria-label^="Clasificación"]');
    await standingsTab.click();
    // standingsTable.html renders a desktop table.table-zebra and a mobile
    // table.table-sm, each hidden at the other breakpoint via CSS.
    const standingsTableClass = isMobile(page) ? 'table.table-sm' : 'table.table-zebra';
    await page.waitForSelector(`${standingsTableClass} tbody tr`, { timeout: 5000 });
    await expect(page.locator(`${standingsTableClass} .text-error`).filter({ hasText: '-5' })).toBeVisible();

    await page.screenshot({ path: testInfo.outputPath('walkover-standings.png'), fullPage: true });

    // Cleanup — the walkover approval also created a penalty row referencing
    // this competition (required relation), so it must go before the competition.
    const penalties = await apiListRecords(page.request, 'penalties', `competition='${compId}'`);
    for (const p of penalties) await apiDeleteRecord(page.request, 'penalties', p.id);
    await apiDeleteRecord(page.request, 'matches', matchId);
    await apiDeleteRecord(page.request, 'competitions', compId);
  });

  test('arbitration (scheduling category) leaves match pending; admin closes it from the match page', async ({ page }) => {
    test.setTimeout(60000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Arbitraje Scheduling E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      rounds: 1,
      calendar_status: 'published',
    });
    const matchId = await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'pending',
      round_number: 1,
      date: '2026-12-01',
      club: 'Padel 360',
    });

    // KISS complement to the escalation: a plain contact link alongside the
    // open-arbitration panel, not a prefilled WhatsApp message. app_settings
    // has no seeded WhatsApp number, so set one here or the link never
    // renders and the assertion below would be checking nothing.
    const [settings] = await apiListRecords(page.request, 'app_settings', '');
    await suPatch(page.request, suToken, `/api/collections/app_settings/records/${settings.id}`, { contact_whatsapp: '34600111222' });

    // scheduling/abandonment/other categories flag the match for admin review
    // without moving it to disputed, so pairs can keep negotiating.
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.locator('button.btn-outline', { hasText: 'Solicitar arbitraje' }).click();
    const dialog = page.locator(`dialog#arbitration-modal-${matchId}`);
    await expect(dialog).toBeVisible({ timeout: 3000 });
    await dialog.locator('select[name="category"]').selectOption('scheduling');
    await dialog.locator('textarea[name="notes"]').fill('No conseguimos acordar fecha.');
    await clickAndWaitForHxRedirect(page, dialog.locator('button:has-text("Solicitar arbitraje")'), `/match/${matchId}`);

    const matchAfterReport = await apiGetRecord(page.request, 'matches', matchId);
    expect(matchAfterReport.status).toBe('pending');
    expect(matchAfterReport.arbitration).toBe('scheduling');
    await expect(page.getByText('Arbitraje solicitado por')).toBeVisible({ timeout: 5000 });
    const arbitrationPanel = page.getByTestId('arbitration-open-panel');
    await expect(arbitrationPanel.getByRole('link', { name: 'WhatsApp' })).toBeVisible();

    // Admin closes the request from the same match page. The contact line is
    // player-facing only — an admin viewing their own arbitration panel must
    // not be told to contact "the admin".
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await expect(page.getByText('Arbitraje solicitado por')).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId('arbitration-open-panel').getByRole('link', { name: 'WhatsApp' })).not.toBeVisible();
    await clickConfirmAndWaitForHxRedirect(page, page.getByRole('button', { name: 'Cerrar arbitraje' }), `/match/${matchId}`);

    const matchAfterClose = await apiGetRecord(page.request, 'matches', matchId);
    expect(matchAfterClose.arbitration).toBe('');
    expect(matchAfterClose.status).toBe('pending');

    await apiDeleteRecord(page.request, 'matches', matchId);
    await apiDeleteRecord(page.request, 'competitions', compId);
    await suPatch(page.request, suToken, `/api/collections/app_settings/records/${settings.id}`, { contact_whatsapp: '' });
  });

  test('playoff bracket renders at mobile viewport with Spanish round names', async ({ page, browser }, testInfo) => {
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
    await generateFixtures(page);
    // The generated bracket is a draft until published; players see nothing before.
    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Publicar calendario")'), `/admin/competitions/${compId}`);

    // Open at 375px mobile viewport. hasTouch matches the mobile project's
    // own S23 emulation (playwright.config.ts) so `@media (hover: none)`
    // engages here exactly as it would on a real phone — without it, this
    // context renders as a touchless desktop regardless of its narrow
    // viewport, and any tap-target regression on this page goes unnoticed.
    const mobileContext = await browser.newContext({ viewport: { width: 375, height: 812 }, hasTouch: true });
    const mobilePage = await mobileContext.newPage();
    await loginAs(mobilePage, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await mobilePage.goto(`/competition/${compId}`);

    await expect(mobilePage.getByRole('heading', { name: 'Cuadro' })).toBeVisible({ timeout: 10000 });
    await expect(mobilePage.getByText('Semifinal')).toBeVisible();
    await expect(mobilePage.getByText('Final').first()).toBeVisible();

    // Bracket cards: 2 semis + 1 final = 3 match cards
    const bracketCards = mobilePage.locator('.card.shadow-sm.border');
    const cardCount = await bracketCards.count();
    expect(cardCount).toBeGreaterThanOrEqual(3);

    // This page bypasses the pageGuards auto-fixture (it's a manually
    // created context, not the fixture's own `page`), so run the same
    // DOM-snapshot guards explicitly instead of losing coverage silently.
    const violations = await checkAll(mobilePage, true);
    expect(violations, 'page guard violations (mobile bracket context)').toEqual([]);

    await mobilePage.screenshot({ path: testInfo.outputPath('bracket-mobile-375.png'), fullPage: true });
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
