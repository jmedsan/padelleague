import { test, expect } from '../overflow-guard';
import { readFileSync } from 'fs';
import { join } from 'path';
import {
  loginAs, asPlayerOn, scratchMatchId, isMobile, navViaDrawer, loadTestData, suPatch, apiGetRecord, clickAndWaitForHxRedirect, switchView, openMatchFromHome,
  apiCreateRecord as apiCreateRecordBase, apiDeleteRecord as apiDeleteRecordBase,
  ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER3_EMAIL, PLAYER3_PASSWORD,
} from '../helpers';
import { submitScore, confirmScore } from '../tour-helpers';
import type { APIRequestContext } from '@playwright/test';

async function goToPage(page: import('@playwright/test').Page, href: string, label: string): Promise<void> {
  if (isMobile(page)) {
    await navViaDrawer(page, href);
  } else {
    await page.locator(`a:has-text("${label}")`).first().click();
    await page.waitForLoadState('networkidle');
  }
}

// openAdminPage follows the admin menu: the drawer on mobile, the navbar's
// "Gestión" dropdown on desktop.
async function openAdminPage(page: import('@playwright/test').Page, href: string): Promise<void> {
  if (isMobile(page)) {
    await navViaDrawer(page, href);
    return;
  }
  const menu = page.locator(`.menu-horizontal details:has(a[href="${href}"])`);
  await menu.locator('summary').click();
  await menu.locator(`a[href="${href}"]`).click();
  await page.waitForURL(`**${href}`);
}

test.describe('R-178: presentation quality guards', { tag: '@presentation' }, () => {
  test('dark-mode legibility: key containers and text are visible', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

    // Switch to dark theme
    await page.evaluate(() => {
      document.documentElement.setAttribute('data-theme', 'dark');
      localStorage.setItem('theme', 'dark');
    });

    // Reload so the stored theme is applied by the page itself
    await page.reload();
    await page.waitForLoadState('networkidle');

    // Admin mode indicator (top-bar pill/dropdown) should be visible
    if (isMobile(page)) {
      await expect(page.locator('[aria-label^="cambiar vista"]')).toBeVisible();
    } else {
      await expect(page.locator('details:has(a[href="/view/player"]) summary')).toBeVisible();
    }

    // Theme attribute is 'dark' (not 'night')
    const theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'));
    expect(theme).toBe('dark');

    // Navigate to admin competitions
    await expect(page, 'admins land on the competitions list').toHaveURL(/\/admin\/competitions$/);
    await page.waitForLoadState('networkidle');

    // Key headings and text are visible
    await expect(page.locator('h1:has-text("Competiciones")')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Competiciones activas' })).toBeVisible();

    // Click into a competition detail
    const compLink = page.locator(`a[href="/admin/competitions/${loadTestData().competitionId}"]`).first();
    await expect(compLink).toBeVisible();
    await compLink.click();
    await page.waitForLoadState('networkidle');
    // The league detail's section headings are visible in dark mode
    for (const name of ['Calendario', 'Documentos', 'Patrocinadores', 'Clasificación', 'Avisos', 'Actividad']) {
      await expect(page.getByRole('heading', { level: 2, name: new RegExp(`^${name}`) })).toBeVisible();
    }
  });

  test('label-language sweep: no English leaks in visible page text', async ({ page }) => {
    // Known English words that should NOT appear in the Spanish UI
    const englishLeaks = [
      /\bSubmit\b/i, /\bCancel\b/i, /\bDelete\b/i, /\bSave\b/i,
      /\bSettings\b/i, /\bProfile\b/i, /\bPassword\b/i,
      /\bHome\b(?!page)/i, /\bSearch\b/i, /\bLogout\b/i,
      /\bLogin\b(?!As)/i, /\bPlayers\b/i, /\bMatches\b/i,
      /\bStandings\b/i, /\bSchedule\b/i, /\bResults\b/i,
      /\bNotifications\b/i, /\bDocuments\b/i, /\bVenues\b/i,
    ];

    // Check as player (most common user)
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.waitForLoadState('networkidle');

    const bodyText = await page.evaluate(() => document.body.innerText);

    for (const pattern of englishLeaks) {
      const match = bodyText.match(pattern);
      expect(match, `English leak found: "${match?.[0]}" in page text`).toBeNull();
    }

    // Check admin page too
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page, 'admins land on the competitions list').toHaveURL(/\/admin\/competitions$/);
    await page.waitForLoadState('networkidle');

    const adminText = await page.evaluate(() => document.body.innerText);
    for (const pattern of englishLeaks) {
      const match = adminText.match(pattern);
      expect(match, `English leak on admin page: "${match?.[0]}"`).toBeNull();
    }
  });

  test('grep-gate: tour-guided has no goto(url) to driven actions', async () => {
    const src = readFileSync(join(__dirname, 'tour-guided.spec.ts'), 'utf-8');
    // Extract all goto() calls
    const gotoPattern = /\.goto\(['"`]([^'"`]+)['"`]\)/g;
    let match: RegExpExecArray | null;
    const violations: string[] = [];

    while ((match = gotoPattern.exec(src)) !== null) {
      const url = match[1];
      // Allowed: '/' (home entry), login, and template-literal entry points
      if (url === '/' || url === '/login') continue;
      // Dynamic competition/match URLs used for doc-gate or API-backed actions
      // are acceptable if they can't be reached via click (e.g. first-time player
      // entering a competition). Flag any static admin/player page URL.
      if (/^\/(admin|match|competition|players|pairs|venues|invitations|disputes)$/.test(url)) {
        violations.push(url);
      }
    }

    expect(violations, `tour-guided should not goto() static pages: ${violations.join(', ')}`).toHaveLength(0);
  });

  test('non-empty panels: urgent tasks and standings render content', { tag: '@smoke' }, async ({ page }) => {
    // Login as player who has match data in seed
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.waitForLoadState('networkidle');

    // Check that "Mis competiciones" section has at least one card
    const compCards = page.locator('.card').filter({ hasText: /Liga|Playoff|competici/i });
    const cardCount = await compCards.count();
    expect(cardCount, 'player home should show at least one competition card').toBeGreaterThan(0);

    // Enter the seeded competition (it has a played match, and no mandatory
    // document, so no gate) and check the standings table is non-empty.
    const data = loadTestData();
    const compLink = page.locator(`a[href="/competition/${data.competitionId}"]`).first();
    await expect(compLink).toBeVisible();
    await compLink.click();
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Documentos obligatorios' })).toHaveCount(0);

    await page.getByRole('tab', { name: 'Clasificación', exact: true }).click();
    // standingsTable.html renders a desktop table.table-zebra and a
    // mobile table.table-sm, each hidden at the other breakpoint via
    // CSS — scope to whichever one is visible for this viewport.
    const standingsTableClass = isMobile(page) ? 'table.table-sm' : 'table.table-zebra';
    const standingsRows = page.locator(`${standingsTableClass} tbody tr`);
    const comp = await apiGetRecord(page.request, data.adminToken, 'competitions', data.competitionId);
    await expect(standingsRows, 'one standings row per pair').toHaveCount(comp.pairs.length);
    await expect(standingsRows.first()).toBeVisible();

    // Admin: the competitions list shows the seeded competition
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page, 'admins land on the competitions list').toHaveURL(/\/admin\/competitions$/);
    await expect(page.locator(`a[href="/admin/competitions/${data.competitionId}"]`).first(), 'admin list shows the seeded competition').toBeVisible();
  });

  test('R-231: home pending-actions panel renders when player has actions', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.waitForLoadState('networkidle');

    const actions = page.locator('[data-testid="home-actions"]');
    await expect(actions, 'pending-actions panel should be visible').toBeVisible({ timeout: 5000 });
    const actionLinks = actions.locator('a');
    expect(await actionLinks.count(), 'should have at least one action').toBeGreaterThan(0);
    await expect(actionLinks.first()).toBeVisible();
  });

  test('R-231: home recent-results panel renders finalized matches', async ({ page }, testInfo) => {
    const suToken = loadTestData().adminToken;
    // Fresh players and pairs, so the panel's five-entry cap and the shared
    // seed competition's finals can't show a result in place of this one.
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const makePlayer = (tag: string) => apiCreate(page.request, suToken, 'users', {
      email: `r231-${tag}-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `R231 ${tag} ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const [a1, a2, b1, b2] = await Promise.all(['a1', 'a2', 'b1', 'b2'].map(makePlayer));
    const pairA = await apiCreate(page.request, suToken, 'pairs', { name: `R231 Pareja A ${suffix}`, player1: a1, player2: a2 });
    const pairBName = `R231 Pareja B ${suffix}`;
    const pairB = await apiCreate(page.request, suToken, 'pairs', { name: pairBName, player1: b1, player2: b2 });
    const compId = await apiCreate(page.request, suToken, 'competitions', {
      name: `R231 Results ${suffix}`, type: 'league', active: true,
      pairs: [pairA, pairB], rounds: 1,
    });
    const matchId = await apiCreate(page.request, suToken, 'matches', {
      competition: compId, pair1: pairA, pair2: pairB,
      status: 'final', round_number: 1, scores: '6-3 6-4', winner: pairA,
    });

    try {
      await asPlayerOn(page, compId, `r231-a1-${suffix}@test.local`, 'testpass123456');
      await page.waitForLoadState('networkidle');

      const heading = page.getByRole('heading', { name: 'Mis últimos partidos' });
      await expect(heading, 'recent-results heading should be visible').toBeVisible({ timeout: 5000 });
      // The list is the heading's next sibling (home.html); its parent is the
      // whole page, which would also match links outside the panel.
      const resultsList = heading.locator('xpath=following-sibling::div[1]');
      const entry = resultsList.locator(`a[href="/match/${matchId}"]`);
      await expect(entry, 'the finalized match must be listed').toHaveCount(1);
      await expect(entry).toContainText(pairBName);
      await expect(entry).toContainText('6-3 6-4');
    } finally {
      await apiDelete(page.request, suToken, 'matches', matchId);
      await apiDelete(page.request, suToken, 'competitions', compId);
      await apiDelete(page.request, suToken, 'pairs', pairA);
      await apiDelete(page.request, suToken, 'pairs', pairB);
      for (const uid of [a1, a2, b1, b2]) await apiDelete(page.request, suToken, 'users', uid);
    }
  });

  test('W12: home keeps "Mis últimos partidos" heading and shows an empty-state message when there are no recent results', async ({ page }, testInfo) => {
    const suToken = loadTestData().adminToken;
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const email = `w12-${suffix}@test.local`;
    const playerId = await apiCreate(page.request, suToken, 'users', {
      email, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `W12 Player ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const partnerId = await apiCreate(page.request, suToken, 'users', {
      email: `w12-partner-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `W12 Partner ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const pairId = await apiCreate(page.request, suToken, 'pairs', {
      name: `W12 Pareja ${suffix}`, player1: playerId, player2: partnerId,
    });
    const compId = await apiCreate(page.request, suToken, 'competitions', {
      name: `W12 Comp ${suffix}`, type: 'league', active: true, pairs: [pairId],
    });

    try {
      await asPlayerOn(page, compId, email, 'testpass123456');
      await page.waitForLoadState('networkidle');

      const heading = page.getByRole('heading', { name: 'Mis últimos partidos' });
      await expect(heading, 'heading must stay visible with zero recent results').toBeVisible({ timeout: 5000 });
      await expect(page.getByText('No hay resultados recientes'), 'empty-state message must show below the heading').toBeVisible();
    } finally {
      await apiDelete(page.request, suToken, 'competitions', compId);
      await apiDelete(page.request, suToken, 'pairs', pairId);
      await apiDelete(page.request, suToken, 'users', playerId);
      await apiDelete(page.request, suToken, 'users', partnerId);
    }
  });

  test('bug: published league with zero played matches still shows the Clasificación tab with an empty state', async ({ page }, testInfo) => {
    const suToken = loadTestData().adminToken;
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const email = `clasif-empty-${suffix}@test.local`;
    const playerId = await apiCreate(page.request, suToken, 'users', {
      email, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Clasif Player ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const partnerId = await apiCreate(page.request, suToken, 'users', {
      email: `clasif-empty-partner-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Clasif Partner ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const pairId = await apiCreate(page.request, suToken, 'pairs', {
      name: `Clasif Pareja ${suffix}`, player1: playerId, player2: partnerId,
    });
    const compName = `Clasif Empty Comp ${suffix}`;
    const compId = await apiCreate(page.request, suToken, 'competitions', {
      name: compName, type: 'league', active: true, pairs: [pairId],
    });

    await asPlayerOn(page, compId, email, 'testpass123456');
    await page.waitForLoadState('networkidle');

    // Reach the competition by clicking its home card, not goto(url).
    await page.locator('a', { hasText: compName }).first().click();
    await page.waitForLoadState('networkidle');

    const standingsTab = page.locator('input[aria-label^="Clasificación"]');
    await expect(standingsTab, 'Clasificación tab must be visible for a published league with zero played matches').toBeVisible();
    await standingsTab.click();
    await expect(page.getByText('No hay datos de clasificación todavía'), 'empty-state text must render inside the tab').toBeVisible();

    await apiDelete(page.request, suToken, 'competitions', compId);
    await apiDelete(page.request, suToken, 'pairs', pairId);
    await apiDelete(page.request, suToken, 'users', playerId);
    await apiDelete(page.request, suToken, 'users', partnerId);
  });

  test('bug: competition with zero announcements still shows the Avisos tab with an empty state', async ({ page }, testInfo) => {
    const suToken = loadTestData().adminToken;
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const email = `avisos-empty-${suffix}@test.local`;
    const playerId = await apiCreate(page.request, suToken, 'users', {
      email, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Avisos Player ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const partnerId = await apiCreate(page.request, suToken, 'users', {
      email: `avisos-empty-partner-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Avisos Partner ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const pairId = await apiCreate(page.request, suToken, 'pairs', {
      name: `Avisos Pareja ${suffix}`, player1: playerId, player2: partnerId,
    });
    const compName = `Avisos Empty Comp ${suffix}`;
    const compId = await apiCreate(page.request, suToken, 'competitions', {
      name: compName, type: 'league', active: true, pairs: [pairId],
    });

    await asPlayerOn(page, compId, email, 'testpass123456');
    await page.waitForLoadState('networkidle');

    // Reach the competition by clicking its home card, not goto(url).
    await page.locator('a', { hasText: compName }).first().click();
    await page.waitForLoadState('networkidle');

    const announcementsTab = page.locator('input[aria-label^="Avisos"]');
    await expect(announcementsTab, 'Avisos tab must be visible with zero announcements').toBeVisible();
    await announcementsTab.click();
    await expect(page.getByText('No hay avisos todavía'), 'empty-state text must render inside the tab').toBeVisible();

    await apiDelete(page.request, suToken, 'competitions', compId);
    await apiDelete(page.request, suToken, 'pairs', pairId);
    await apiDelete(page.request, suToken, 'users', playerId);
    await apiDelete(page.request, suToken, 'users', partnerId);
  });

  test('feature: a pending (unconfirmed) result proposal counts in the Clasificación with a tooltip', async ({ page }, testInfo) => {
    const suToken = loadTestData().adminToken;
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const p1u1 = await apiCreate(page.request, suToken, 'users', {
      email: `prov-a1-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Prov A1 ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const p1u2 = await apiCreate(page.request, suToken, 'users', {
      email: `prov-a2-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Prov A2 ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const p2u1 = await apiCreate(page.request, suToken, 'users', {
      email: `prov-b1-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Prov B1 ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const p2u2 = await apiCreate(page.request, suToken, 'users', {
      email: `prov-b2-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Prov B2 ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const pairAName = `Prov Pareja A ${suffix}`;
    const pairAId = await apiCreate(page.request, suToken, 'pairs', { name: pairAName, player1: p1u1, player2: p1u2 });
    const pairBId = await apiCreate(page.request, suToken, 'pairs', { name: `Prov Pareja B ${suffix}`, player1: p2u1, player2: p2u2 });
    const compName = `Prov Comp ${suffix}`;
    const compId = await apiCreate(page.request, suToken, 'competitions', {
      name: compName, type: 'league', active: true, pairs: [pairAId, pairBId],
    });
    const matchId = await apiCreate(page.request, suToken, 'matches', {
      competition: compId, pair1: pairAId, pair2: pairBId, status: 'scheduled', round_number: 1,
    });
    const proposalId = await apiCreate(page.request, suToken, 'match_messages', {
      match: matchId, author: p1u1, type: 'result_submission', proposal_status: 'pending',
      content: '6-3 6-4', proposal_data: JSON.stringify({ scores: '6-3 6-4' }),
    });

    await asPlayerOn(page, compId, `prov-a1-${suffix}@test.local`, 'testpass123456');
    await page.waitForLoadState('networkidle');

    // Reach the competition by clicking its home card, not goto(url). This
    // fixture's player belongs to exactly one competition, so home renders
    // the single-featured-card layout (data-testid="single-comp-entry")
    // instead of the "Mis competiciones" grid — and also has a pending
    // "propose a date" home action whose Detail text contains the
    // competition name too, so a plain hasText <a> locator would be
    // ambiguous.
    await page.locator('[data-testid="single-comp-entry"]').click();
    await page.waitForLoadState('networkidle');

    await page.locator('input[aria-label^="Clasificación"]').click();
    // standingsTable renders both a desktop table and a mobile table, only
    // one visible per breakpoint via CSS — scope to whichever is visible.
    const standingsTableClass = isMobile(page) ? 'table.table-sm' : 'table.table-zebra';
    const row = page.locator(`${standingsTableClass} tbody tr`, { hasText: pairAName }).first();
    await expect(row, 'the pending proposal must count as a win right away').toBeVisible();
    const tooltip = row.locator('[data-tip="Incluye resultados sin confirmar"]');
    await expect(tooltip, 'the row must show the unconfirmed-result tooltip').toBeVisible();

    await apiDelete(page.request, suToken, 'match_messages', proposalId);
    await apiDelete(page.request, suToken, 'matches', matchId);
    await apiDelete(page.request, suToken, 'competitions', compId);
    await apiDelete(page.request, suToken, 'pairs', pairAId);
    await apiDelete(page.request, suToken, 'pairs', pairBId);
    for (const uid of [p1u1, p1u2, p2u1, p2u2]) await apiDelete(page.request, suToken, 'users', uid);
  });

  test('R-231: notifications dropdown shows entries when notifications exist', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.waitForLoadState('networkidle');

    const bell = page.locator('button[aria-label^="notificaciones"]:visible');
    await bell.click();
    await page.waitForTimeout(500);

    const dropdown = isMobile(page)
      ? bell.locator('xpath=..').locator('.dropdown-content')
      : page.locator('#notif-dropdown');
    const entries = dropdown.locator('a[href^="/match/"], a[href^="/notification"]');
    expect(await entries.count(), 'notification dropdown should have entries').toBeGreaterThan(0);
  });

  test('R-231: admin disputes page shows dispute rows when disputes exist', async ({ page }) => {
    const data = loadTestData();
    const suToken = loadTestData().adminToken;

    const compId = await apiCreate(page.request, suToken, 'competitions', {
      name: 'R231 Disputes', type: 'league', active: true,
      pairs: [data.pair1Id, data.pair2Id], rounds: 1,
    });
    const matchId = await apiCreate(page.request, suToken, 'matches', {
      competition: compId, pair1: data.pair1Id, pair2: data.pair2Id,
      status: 'disputed', round_number: 1, scores: '6-3 6-4',
      dispute_notes: 'R-231 test dispute',
    });

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    // Admins land on /admin/competitions; its dispute count links to the list.
    await page.locator('a[href="/admin/disputes"]').click();
    await page.waitForURL('**/admin/disputes');
    await page.waitForLoadState('networkidle');

    // /admin/disputes renders each item via the shared healthItemRow partial
    // (an <a>, not a .card) — no score is shown there, only pair names, the
    // category badge, and a link into the match for detail. Filter on this
    // test's own matchId, not just pair names — the seeded pairs (Pareja
    // Alpha/Beta) are shared across specs, so other disputes accumulated in
    // the same run also match "Pareja Alpha" and .first() picked whichever
    // rendered first, not necessarily this test's row.
    const disputeRow = page.locator(`[data-testid="health-item-row"][href="/match/${matchId}"]`);
    await expect(disputeRow).toBeVisible({ timeout: 5000 });
    await expect(disputeRow.getByText('Pareja Alpha')).toBeVisible();
    await expect(disputeRow.getByText('Pareja Beta')).toBeVisible();
    await expect(disputeRow.getByText('Disputa', { exact: true })).toBeVisible();
    await expect(disputeRow.getByText('6-3 6-4')).toHaveCount(0);

    await apiDelete(page.request, suToken, 'matches', matchId);
    await apiDelete(page.request, suToken, 'competitions', compId);
  });

  test('penalty log: no "Quitar" text button in the penalty form; Penalizar is a distinct action', async ({ page }) => {
    // The penalty column's per-entry remove is a de-emphasized × icon inside the
    // penalty <form>, NOT a "Quitar" text button woven into the log (P4). Reverting
    // T6 brings the "Quitar" text button back → this fails. "Penalizar" renders as
    // a distinct control. (Asserted on a seeded competition that renders pairs.)
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page, 'admins land on the competitions list').toHaveURL(/\/admin\/competitions$/);
    await page.waitForLoadState('domcontentloaded');
    await page.locator('a[href^="/admin/competitions/"]').first().click();
    await page.waitForLoadState('networkidle');
    const parejas = page.locator('[data-testid="section-parejas"]');
    await parejas.locator('> input[type="checkbox"]').check({ force: true });
    await page.waitForTimeout(300);

    // No "Quitar" text button inside a penalty form (the old woven-in control).
    await expect(parejas.locator('form[hx-post*="/penalty"] button:has-text("Quitar")')).toHaveCount(0);
    // "Penalizar" renders as a distinct action for each pair row. The
    // desktop table's trigger is icon-only (aria-label, no text node) and
    // always visible; on mobile it lives inside the "Más acciones" dropdown,
    // closed by default — open it first (same pattern as tour-reference.spec.ts).
    let penalizeLabel = parejas.locator('label[for^="penalty-modal-"][aria-label^="Penalizar"]').first();
    if (isMobile(page)) {
      const dropdown = parejas.locator('.dropdown:has(label[for^="penalty-modal-"])').first();
      await dropdown.locator('button[aria-label^="Más acciones"]').click();
      penalizeLabel = dropdown.locator('label[for^="penalty-modal-"]');
    }
    await expect(penalizeLabel).toBeVisible();
  });

  test('E: leveled league shows Revancha badge and admin shortfall notice', async ({ page }) => {
    const suToken = loadTestData().adminToken;
    const suffix = `e-shortfall-${Date.now()}`;

    // A third pair beyond the shared data.pair1Id/pair2Id, so a 3-pair,
    // target=1 competition has an odd total need (1) that no pairing can
    // supply — league.LeveledShortfall must report it.
    const p3a = await apiCreate(page.request, suToken, 'users', {
      email: `${suffix}-a@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Shortfall A', roles: ['player'], verified: true, gender: 'male',
    });
    const p3b = await apiCreate(page.request, suToken, 'users', {
      email: `${suffix}-b@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Shortfall B', roles: ['player'], verified: true, gender: 'male',
    });
    const pair3Id = await apiCreate(page.request, suToken, 'pairs', {
      name: `Pareja Shortfall ${suffix}`, player1: p3a, player2: p3b,
    });

    const data = loadTestData();
    const compName = `E Shortfall ${suffix}`;
    const compId = await apiCreate(page.request, suToken, 'competitions', {
      name: compName, type: 'league', active: true,
      pairs: [data.pair1Id, data.pair2Id, pair3Id],
      target_matches: 1, open_assignments: 1,
    });
    // pair1-pair2 already has their one match (rematch: true, exercising
    // the shared matchCard/matchRow badge); pair3 has none, and neither of
    // the others can supply it — the unavoidable shortfall.
    const matchId = await apiCreate(page.request, suToken, 'matches', {
      competition: compId, pair1: data.pair1Id, pair2: data.pair2Id,
      status: 'pending', round_number: 0, rematch: true,
    });

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page, 'admins land on the competitions list').toHaveURL(/\/admin\/competitions$/);
    await page.waitForLoadState('domcontentloaded');
    // Reached by clicking the competition's own card, not goto(url).
    await page.getByRole('link', { name: compName }).first().click();
    await page.waitForLoadState('networkidle');

    await expect(page.getByText('Revancha').locator('visible=true').first()).toBeVisible({ timeout: 5000 });
    const notice = page.getByTestId('leveled-shortfall-notice');
    await expect(notice).toBeVisible({ timeout: 5000 });
    await expect(notice).toContainText('quedará con 0 partidos en vez de 1');

    await apiDelete(page.request, suToken, 'matches', matchId);
    await apiDelete(page.request, suToken, 'competitions', compId);
    await apiDelete(page.request, suToken, 'pairs', pair3Id);
    await apiDelete(page.request, suToken, 'users', p3a);
    await apiDelete(page.request, suToken, 'users', p3b);
  });

  test('R-164: date-format guard — no raw ISO dates in visible text', async ({ page }) => {
    // ISO date patterns that should NEVER appear in rendered UI text
    const isoLeaks = [
      /\d{4}-\d{2}-\d{2}T/,                  // RFC3339 with T separator
      /00:00:00\.000Z/,                        // PB midnight timestamp suffix
      /\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/, // raw PB datetime
    ];
    // Spanish date format: DD/MM/YYYY or DD/MM/YYYY HH:MM
    const spanishDate = /\d{2}\/\d{2}\/\d{4}/;

    // Check player home (has dates in next match, proposed dates, etc.)
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.waitForLoadState('networkidle');

    let bodyText = await page.evaluate(() => document.body.innerText);

    for (const pattern of isoLeaks) {
      const match = bodyText.match(pattern);
      expect(match, `ISO date leak on player home: "${match?.[0]}"`).toBeNull();
    }

    // Enter the seeded competition (no mandatory document, so no gate) and check dates there
    const compLink = page.locator(`a[href="/competition/${loadTestData().competitionId}"]`).first();
    await expect(compLink).toBeVisible();
    await compLink.click();
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Documentos obligatorios' })).toHaveCount(0);

    bodyText = await page.evaluate(() => document.body.innerText);
    for (const pattern of isoLeaks) {
      const match = bodyText.match(pattern);
      expect(match, `ISO date leak on competition page: "${match?.[0]}"`).toBeNull();
    }

    // Check admin competition detail (has round dates, match dates)
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page, 'admins land on the competitions list').toHaveURL(/\/admin\/competitions$/);
    await page.waitForLoadState('networkidle');
    const adminComp = page.locator(`a[href="/admin/competitions/${loadTestData().competitionId}"]`).first();
    await expect(adminComp).toBeVisible();
    await adminComp.click();
    await page.waitForLoadState('networkidle');
    bodyText = await page.evaluate(() => document.body.innerText);
    for (const pattern of isoLeaks) {
      const match = bodyText.match(pattern);
      expect(match, `ISO date leak on admin detail: "${match?.[0]}"`).toBeNull();
    }
  });

  test('R-173: admin match-progress notification — submit + accept triggers bell entry', async ({ page }, testInfo) => {
    const matchId = scratchMatchId('admin-notif', testInfo.project.name);

    // Set date+club via superuser API so score submission is enabled
    await suPatch(page.request, loadTestData().adminToken, `/api/collections/matches/records/${matchId}`, { date: '2025-03-15', club: 'Padel 360' });

    // Player1 submits a score
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openMatchFromHome(page, matchId);
    await page.waitForLoadState('networkidle');
    await submitScore(page, '6-3 6-4');

    // Player3 confirms (on pair3, opposite team — admin is not a participant)
    await loginAs(page, PLAYER3_EMAIL, PLAYER3_PASSWORD);
    // pair3 is outside the competition (global-setup), so player3 has no
    // competition card; they reach the match from the proposal notification.
    await page.locator('button[aria-label^="notificaciones"]:visible').click();
    await clickAndWaitForHxRedirect(page, page.locator(`a[href="/match/${matchId}"]:visible`).first(), `/match/${matchId}`);
    await page.waitForLoadState('networkidle');
    await confirmScore(page);

    // Admin clicks the notification bell and sees the match-progress entry
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.waitForLoadState('networkidle');

    // Click the bell dropdown to load notifications
    const bell = page.locator('button[aria-label^="notificaciones"]:visible');
    await bell.click();
    await page.waitForTimeout(500);

    // The dropdown should contain a match-progress notification (mobile has no #notif-dropdown id)
    const dropdown = isMobile(page)
      ? bell.locator('xpath=..').locator('.dropdown-content')
      : page.locator('#notif-dropdown');
    await expect(dropdown.locator('text=Progreso de partido').first()).toBeVisible({ timeout: 5000 });
  });

  test('R-175: mode-driven home — admin GET / redirects to the admin dashboard, not player content; player view shows the opposite', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

    // A fresh context has no view_as cookie, so the admin is in the admin view.
    await page.waitForLoadState('networkidle');

    // Admin GET / redirects to /admin/competitions, the single admin landing
    // page — it never renders player home content.
    await expect(page).toHaveURL(/\/admin\/competitions$/);
    await expect(page.getByRole('heading', { name: 'Competiciones', exact: true })).toBeVisible();
    await expect(page.locator('text=Gestión').first()).toBeAttached();
    const bodyText = await page.evaluate(() => document.body.innerText);
    expect(bodyText).not.toContain('Administración');

    // Flip to player view via the switcher
    await switchView(page, 'player');

    // Switching to the player view from an admin page lands on home (view.go).

    // Player view must stay on / (no redirect) and show no admin content.
    await expect(page).toHaveURL(/\/$/);
    await expect(page.locator('a[href="/admin/competitions"]')).toHaveCount(0);
    const playerBody = await page.evaluate(() => document.body.innerText);
    expect(playerBody).not.toContain('Preparar competiciones');

    // Switch back to admin view for other tests
    await switchView(page, 'admin');
  });

  test('R-35: captain badge appears next to captain player on pair detail page', async ({ page }) => {
    // pair1 (Pareja Alpha) has player1 as captain per global-setup (captain: player1Id).
    // Navigate to the pair page from standings — click affordance, not goto(url).
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.waitForLoadState('networkidle');

    // Enter the seeded competition
    const compLink = page.locator(`a[href="/competition/${data.competitionId}"]`).first();
    await expect(compLink).toBeVisible({ timeout: 5000 });
    await compLink.click();
    await page.waitForLoadState('domcontentloaded');

    // The seeded competition has no mandatory document, so no gate.
    await expect(page.getByRole('heading', { name: 'Documentos obligatorios' })).toHaveCount(0);

    // Click Clasificación tab to reveal pair links
    const standingsTab = page.locator('input[aria-label^="Clasificación"]');
    await expect(standingsTab).toBeVisible({ timeout: 5000 });
    await standingsTab.click();
    await page.waitForTimeout(300);

    // Click the pair1 link from the visible standings table (mobile and desktop
    // each render a separate table, hidden via CSS at the other breakpoint).
    const standingsTableClass = isMobile(page) ? 'table.table-sm' : 'table.table-zebra';
    const pairLink = page.locator(`${standingsTableClass} a[href="/pair/${data.pair1Id}"]`).first();
    await expect(pairLink).toBeVisible({ timeout: 5000 });
    await pairLink.click();
    await page.waitForLoadState('domcontentloaded');

    expect(page.url()).toContain(`/pair/${data.pair1Id}`);
    // Captain badge must appear — reverting playerLinkCaptain wiring removes it
    await expect(page.locator('.badge[title="Capitán"]')).toBeVisible({ timeout: 5000 });
  });

  test('R-35: CSS grep-gate — HTMX loading spinner rule present in compiled stylesheet', async () => {
    // Verifies the loading spinner CSS (form.htmx-request button[type="submit"]::after)
    // is compiled into styles.css. Reverts to input.css removal would fail this.
    const cssPath = join(__dirname, '../../static/css/styles.css');
    const css = readFileSync(cssPath, 'utf-8');
    expect(css, 'styles.css must contain htmx-request spinner rule').toContain('htmx-request');
  });

  test('R-209: pairs create form disables selected player in sibling dropdown', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await openAdminPage(page, '/admin/pairs');
    await page.waitForLoadState('networkidle');

    await page.evaluate(() => {
      (document.getElementById('modal-create') as HTMLDialogElement)?.showModal();
    });
    const dialog = page.locator('dialog#modal-create');
    const player1 = dialog.locator('select[name="player1"]');
    const player2 = dialog.locator('select[name="player2"]');

    await player1.waitFor({ state: 'visible', timeout: 5000 });
    const options = await player1.locator('option:not([value=""])').all();
    expect(options.length, 'should have player options').toBeGreaterThan(0);

    const firstValue = await options[0].getAttribute('value');
    await player1.selectOption(firstValue!);

    const disabled = await player2.locator(`option[value="${firstValue}"]`).getAttribute('disabled');
    expect(disabled, 'selected player1 should be disabled in player2 dropdown').not.toBeNull();
  });
});

async function apiCreate(request: APIRequestContext, token: string, collection: string, data: Record<string, any>): Promise<string> {
  return apiCreateRecordBase(request, token, collection, data);
}

async function apiDelete(request: APIRequestContext, token: string, collection: string, id: string): Promise<void> {
  await apiDeleteRecordBase(request, token, collection, id);
}
