import type { APIRequestContext } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, isMobile, openDrawer, navViaDrawer, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

// Samsung Galaxy S23 (owner's real device) — matches the mobile project's
// default viewport in playwright.config.ts. Kept as an explicit constant
// here (rather than relying on the project default) because these tests
// also run on the desktop project and need to force the narrow viewport.
const MOBILE = { width: 360, height: 780 };

let suToken = '';

async function getSuperuserToken(page: import('@playwright/test').Page) {
  if (suToken) return;
  const resp = await page.request.post('/api/collections/_superusers/auth-with-password', {
    data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
  });
  if (!resp.ok()) throw new Error(`Superuser auth failed: ${resp.status()}`);
  suToken = (await resp.json()).token;
}

async function apiCreateRecord(request: APIRequestContext, collection: string, data: Record<string, any>): Promise<string> {
  const resp = await request.post(`/api/collections/${collection}/records`, {
    headers: { Authorization: suToken, 'Content-Type': 'application/json' },
    data,
  });
  if (!resp.ok()) throw new Error(`Create ${collection} failed: ${resp.status()} ${await resp.text()}`);
  return (await resp.json()).id;
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

async function checkNoOverflow(page: import('@playwright/test').Page) {
  const overflow = await page.evaluate(() => {
    return document.documentElement.scrollWidth > window.innerWidth;
  });
  expect(overflow, 'page should not have horizontal overflow').toBe(false);
}

test.describe('responsive - no horizontal overflow', () => {
  test('login page', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await page.goto('/login');
    await checkNoOverflow(page);
  });

  test('home page', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await checkNoOverflow(page);
    // Depending on test ordering, player may be in 1 or more competitions
    const singleEntry = page.locator('[data-testid="single-comp-entry"]');
    const multiHeading = page.locator('[data-testid="player-competitions-heading"]');
    const hasSingle = await singleEntry.isVisible().catch(() => false);
    if (hasSingle) {
      await expect(singleEntry).toContainText('Liga E2E Test');
    } else {
      await expect(multiHeading).toBeVisible();
    }
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
  });

  test('admin dashboard', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: 'Competiciones', exact: true })).toBeVisible();
    await expect(page.locator('.card-title', { hasText: 'Liga E2E Test' }).first()).toBeVisible();
  });

  test('competition page', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator(`a[href^="/competition/"]`, { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    await expect(page.locator('input[aria-label="Jornadas"]')).toBeVisible();
    await page.locator('input[aria-label="Jornadas"]').click();
    // getByText also matches the pair-filter <select>'s <option> (never
    // "visible" per Playwright) — scope to the visible match-row text.
    await expect(page.getByText('Pareja Alpha').locator('visible=true').first()).toBeVisible();
  });

  test('match detail', async ({ page }) => {
    const data = loadTestData();
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await checkNoOverflow(page);
  });

  test('match thread', async ({ page }) => {
    const data = loadTestData();
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await checkNoOverflow(page);
  });

  test('admin competition detail', async ({ page }) => {
    const data = loadTestData();
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${data.competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    const standingsCard = page.locator('.card', { has: page.getByRole('heading', { name: 'Clasificación' }) });
    await expect(standingsCard).toBeVisible();
    // Mobile card list and desktop table both render every pair name (CSS-
    // toggled per breakpoint, both in the DOM at once) — scope to visible.
    await expect(standingsCard.getByText('Pareja Alpha').locator('visible=true').first()).toBeVisible();
  });

  test('W13: long pair names in a jornada match row wrap instead of overflowing at 360px', async ({ page }) => {
    await getSuperuserToken(page);
    const suffix = `w13-${Date.now()}`;
    const p1a = await apiCreateRecord(page.request, 'users', {
      email: `${suffix}-a1@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Alejandro Fernandez', roles: ['player'], verified: true, gender: 'male',
    });
    const p1b = await apiCreateRecord(page.request, 'users', {
      email: `${suffix}-a2@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Bartolome Gutierrez', roles: ['player'], verified: true, gender: 'male',
    });
    const p2a = await apiCreateRecord(page.request, 'users', {
      email: `${suffix}-b1@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Cristobal Rodriguez', roles: ['player'], verified: true, gender: 'male',
    });
    const p2b = await apiCreateRecord(page.request, 'users', {
      email: `${suffix}-b2@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Domingo Hernandez', roles: ['player'], verified: true, gender: 'male',
    });
    const pairA = await apiCreateRecord(page.request, 'pairs', {
      name: 'Alejandro Fernandez / Bartolome Gutierrez', player1: p1a, player2: p1b,
    });
    const pairB = await apiCreateRecord(page.request, 'pairs', {
      name: 'Cristobal Rodriguez / Domingo Hernandez', player1: p2a, player2: p2b,
    });
    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: `W13 Long Names ${suffix}`, type: 'league', active: true, pairs: [pairA, pairB],
    });
    await apiCreateRecord(page.request, 'matches', {
      competition: compId, pair1: pairA, pair2: pairB, status: 'pending', round_number: 1,
    });

    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);

    // Names must never truncate — they wrap onto their own line instead, so
    // the full pair name stays readable (see matchRow's stacked mobile
    // layout, `p.break-words`, distinct from the hidden-on-mobile desktop
    // `span.break-words`). Scope to the match row itself (not the Parejas
    // section or penalty modal, which repeat the same pair names elsewhere).
    const matchRow = page.locator('a[href^="/match/"]').first();
    const pairLines = matchRow.locator('p.break-words');
    const lineCount = await pairLines.count();
    expect(lineCount, 'both pair-name lines must be present').toBe(2);
    await expect(pairLines.nth(0)).toContainText('Alejandro Fernandez / Bartolome Gutierrez');
    await expect(pairLines.nth(1)).toContainText('Cristobal Rodriguez / Domingo Hernandez');
    for (let i = 0; i < lineCount; i++) {
      const isTruncating = await pairLines.nth(i).evaluate(el => el.scrollWidth > el.clientWidth);
      expect(isTruncating, `pair-name line ${i} must wrap in full, never truncate`).toBe(false);
    }

    await apiDeleteRecord(page.request, 'competitions', compId);
    await apiDeleteRecord(page.request, 'pairs', pairA);
    await apiDeleteRecord(page.request, 'pairs', pairB);
    for (const uid of [p1a, p1b, p2a, p2b]) await apiDeleteRecord(page.request, 'users', uid);
  });

  test('H1: competition tab strip wraps instead of overflowing at 360px', async ({ page }) => {
    await getSuperuserToken(page);
    const suffix = `h1-${Date.now()}`;
    const p1 = await apiCreateRecord(page.request, 'users', {
      email: `${suffix}-a1@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Elena Torres', roles: ['player'], verified: true, gender: 'female',
    });
    const p2 = await apiCreateRecord(page.request, 'users', {
      email: `${suffix}-a2@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Fatima Ruiz', roles: ['player'], verified: true, gender: 'female',
    });
    const p3 = await apiCreateRecord(page.request, 'users', {
      email: `${suffix}-b1@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Gema Ortiz', roles: ['player'], verified: true, gender: 'female',
    });
    const p4 = await apiCreateRecord(page.request, 'users', {
      email: `${suffix}-b2@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Hortensia Vega', roles: ['player'], verified: true, gender: 'female',
    });
    const pairA = await apiCreateRecord(page.request, 'pairs', {
      name: 'Pareja H1 A', player1: p1, player2: p2,
    });
    const pairB = await apiCreateRecord(page.request, 'pairs', {
      name: 'Pareja H1 B', player1: p3, player2: p4,
    });
    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: `H1 Tabs Overflow ${suffix}`, type: 'league', active: true, pairs: [pairA, pairB],
      calendar_status: 'published',
    });
    // The Clasificación tab itself is always visible once the calendar is
    // published (an empty-state renders otherwise); a finalized match makes
    // the standings table render here instead. calendar_status must be
    // published — a draft calendar still hides the tab from any non-admin
    // viewer, same as matchVisibleTo does for match pages.
    await apiCreateRecord(page.request, 'matches', {
      competition: compId, pair1: pairA, pair2: pairB, status: 'final',
      round_number: 1, scores: '6-3 6-4', winner: pairA,
    });
    const docId = await apiCreateRecord(page.request, 'documents', {
      title: `Reglamento ${suffix}`, url: 'https://example.com/reglamento',
    });
    await page.request.patch(`/api/collections/competitions/records/${compId}`, {
      headers: { Authorization: suToken, 'Content-Type': 'application/json' },
      data: { documents: [docId] },
    });
    const adminID = (await apiListRecords(page.request, 'users', `email = "${ADMIN_EMAIL}"`))[0]?.id;
    await apiCreateRecord(page.request, 'announcements', {
      competition: compId, title: `Aviso ${suffix}`, body: 'Aviso de prueba', created_by: adminID,
    });

    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${compId}`);
    await page.waitForLoadState('domcontentloaded');
    // No manual checkNoOverflow call here: the shared overflowGuard fixture
    // (../overflow-guard.ts) now asserts no horizontal overflow on every
    // mobile-project test automatically, including this page. This test's
    // own assertions below stay scoped to the tab strip.

    const tablist = page.locator('div[role="tablist"]').first();
    await expect(tablist).toBeVisible();
    const box = await tablist.boundingBox();
    expect(box?.width, 'tablist must not exceed the 360px viewport').toBeLessThanOrEqual(360);

    for (const label of ['Jornadas', 'Avisos', 'Documentos', 'Clasificación']) {
      await expect(page.locator(`input[aria-label="${label}"]`)).toBeVisible();
    }

    await apiDeleteRecord(page.request, 'competitions', compId);
    await apiDeleteRecord(page.request, 'pairs', pairA);
    await apiDeleteRecord(page.request, 'pairs', pairB);
    await apiDeleteRecord(page.request, 'documents', docId);
    for (const uid of [p1, p2, p3, p4]) await apiDeleteRecord(page.request, 'users', uid);
  });

  // H1-class sweep: every admin page header pairing a title (+ optional
  // count badge) with an action button must wrap instead of overflowing —
  // same bug as the tab strip above, found across five pages (admin
  // invitations, players, pairs, health) whose header row was missing
  // flex-wrap. Measured via bounding box at 360px, not by eyeballing text
  // length. outstanding.html and competition-detail.html's Documentos
  // header have no adjacent button (title + count only) — not this class,
  // excluded here.
  test('H1-sweep: admin page headers wrap instead of overflowing at 360px', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

    const pages: Array<{ url: string; header: string }> = [
      { url: '/admin/invitations', header: '.card-body > .flex.items-center.justify-between' },
      { url: '/admin/players', header: '.card-body > .flex.items-center.justify-between' },
      { url: '/admin/pairs', header: '.flex.justify-between.items-center' },
      { url: '/admin/venues', header: '.card-body > .flex.items-center.justify-between' },
      { url: '/admin/health', header: '.flex.items-end.justify-between' },
    ];
    for (const { url, header } of pages) {
      await page.goto(url);
      await page.waitForLoadState('domcontentloaded');
      await checkNoOverflow(page);
      const box = await page.locator(header).first().boundingBox();
      expect(box?.width, `${url} header must not exceed the 360px viewport`).toBeLessThanOrEqual(360);
    }
  });

  test('admin pairs', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/pairs');
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    await expect(page.getByText('Pareja Alpha')).toBeVisible();
    await expect(page.getByText('Pareja Beta')).toBeVisible();
  });

  test('admin players', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/players');
    await checkNoOverflow(page);
    // R-review: the table's Email/Género/actions columns are off-screen at
    // 360px — below sm the page must show a card per player instead, with
    // the "Editar"/"Regenerar enlace" actions reachable without scrolling
    // sideways (see review-principles.md).
    await expect(page.locator('table#players-table')).toBeHidden();
    const cards = page.locator('#players-cards');
    await expect(cards.getByText('Test Player', { exact: true })).toBeVisible();
    await expect(cards.getByText('Test Player 2', { exact: true })).toBeVisible();
    const firstCard = cards.locator('> li').first();
    await expect(firstCard).toBeVisible();
    await firstCard.getByLabel('Más acciones').click();
    await expect(firstCard.getByText('Editar')).toBeVisible();
    await expect(firstCard.getByText('Regenerar contraseña')).toBeVisible();
  });

  test('admin venues', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/venues');
    await checkNoOverflow(page);
    // Mobile card list and desktop table both render every venue name.
    await expect(page.getByText('Pista Central').locator('visible=true').first()).toBeVisible();
  });

  test('admin invitations', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/invitations');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: 'Invitaciones' })).toBeVisible();

    // R-review: Usos/Expira/Enlace/Revocar columns are off-screen at 360px,
    // hiding the page's main action (Copiar). Below sm, a card per invitation
    // must keep "Copiar" reachable without a sideways scroll.
    const invEmail = `resp-mobile-${Date.now()}@example.com`;
    await page.locator('button:has-text("Nueva invitación")').click();
    await page.locator('#modal-create-invite input[name="email"]').fill(invEmail);
    await page.locator('#modal-create-invite button[type="submit"]').click();
    await page.waitForLoadState('networkidle');

    const table = page.locator('table').filter({ hasText: 'Destinatario' });
    await expect(table).toBeHidden();
    // Mobile card list: sm:hidden > ul.divide-y > li (not the stale
    // .lg\:hidden div guess) — Copiar lives in the row's "Más acciones"
    // dropdown, reachable with one tap, no sideways scroll.
    const card = page.locator('.sm\\:hidden .divide-y > li').filter({ hasText: invEmail });
    await expect(card).toBeVisible();
    await card.getByLabel('Más acciones').click();
    await expect(card.getByText('Copiar enlace')).toBeVisible();
  });

  test('admin disputes', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/disputes');
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: 'Disputas' })).toBeVisible();
  });

  test('player profile', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openDrawer(page);
    await page.locator('.drawer-side a:has-text("Mi perfil")').click();
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: 'Test Player' })).toBeVisible();
  });

  test('notification prefs', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await navViaDrawer(page, '/profile/notifications');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: /Preferencias de notificaciones/i })).toBeVisible();
  });

  test('R-165: competition card badges stay within card bounds', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    // loginAs already navigates to / (home)
    await page.waitForLoadState('networkidle');

    const cards = page.locator('.card.overflow-hidden');
    const count = await cards.count();
    for (let i = 0; i < count; i++) {
      const card = cards.nth(i);
      const cardBox = await card.boundingBox();
      if (!cardBox) continue;
      const badges = card.locator('.badge');
      const badgeCount = await badges.count();
      for (let j = 0; j < badgeCount; j++) {
        const badgeBox = await badges.nth(j).boundingBox();
        if (!badgeBox) continue;
        expect(badgeBox.x + badgeBox.width, `badge ${j} in card ${i} right edge`).toBeLessThanOrEqual(cardBox.x + cardBox.width + 1);
      }
    }
  });

  test('R-170: dark mode renders readable text and admin indicator', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.evaluate(() => {
      document.documentElement.setAttribute('data-theme', 'dark');
      localStorage.setItem('theme', 'dark');
    });
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForLoadState('networkidle');

    // Admin mode indicator (top-bar pill/dropdown) should be visible
    if (isMobile(page)) {
      await expect(page.locator('[aria-label="cambiar vista"]')).toBeVisible();
    } else {
      await expect(page.locator('details:has(a[href="/view/player"]) summary')).toBeVisible();
    }

    // Theme should be 'dark', not 'night'
    const theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'));
    expect(theme).toBe('dark');

    // Key text elements should be visible (not invisible due to low opacity)
    await expect(page.locator('h1:has-text("Competiciones")')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Competiciones activas' })).toBeVisible();
  });

  test('R-review: pair/player history renders as cards, not a table, at 360px', async ({ page }) => {
    test.setTimeout(60000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Historial Móvil E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      rounds: 1,
    });
    await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'final',
      round_number: 1,
      scores: '6-3 6-4',
      winner: data.pair1Id,
      date: new Date().toISOString().slice(0, 10),
    });

    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    await page.waitForLoadState('domcontentloaded');
    // Click through via a real affordance (the pair link on the competition
    // page). Mobile card list and desktop table both render this link —
    // .first() alone could pick the CSS-hidden desktop one.
    await page.locator(`a[href="/pair/${data.pair1Id}"]`).locator('visible=true').first().click();
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);

    await expect(page.getByRole('heading', { name: 'Últimos partidos' })).toBeVisible();
    // Below sm, resultHistoryRow's table must be hidden — the score wraps
    // onto multiple lines inside a <td> and clips the V/D column off-screen
    // at 360px otherwise (see review-principles.md).
    const table = page.locator('table.table-sm').filter({ hasText: '6-3' });
    await expect(table).toBeHidden();
    const card = page.locator('.sm\\:hidden.space-y-3 > a').first();
    await expect(card).toBeVisible();
    await expect(card.getByText('6-3')).toBeVisible();
    await expect(card.locator('.badge')).toBeVisible();

    // Cleanup
    const matches = await apiListRecords(page.request, 'matches', `competition='${compId}'`);
    for (const m of matches) await apiDeleteRecord(page.request, 'matches', m.id);
    await apiDeleteRecord(page.request, 'competitions', compId);
  });

  test('R-review: competition-detail alert row omits the redundant competition name', async ({ page }) => {
    test.setTimeout(60000);
    await getSuperuserToken(page);
    const data = loadTestData();

    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: 'Alertas Sin Redundancia E2E',
      type: 'league',
      active: true,
      pairs: [data.pair1Id, data.pair2Id],
      rounds: 1,
    });
    await apiCreateRecord(page.request, 'matches', {
      competition: compId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'disputed',
      round_number: 1,
      scores: '6-3 6-4',
      dispute_notes: 'Test dispute for HideCompetition',
    });

    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    await page.waitForLoadState('domcontentloaded');

    const alertsCard = page.locator('div.card', { has: page.getByRole('heading', { level: 2, name: 'Alertas' }) });
    await expect(alertsCard).toBeVisible();
    // The competition name is already in the page header — repeating it on
    // every alert row is redundant at any width, and wastes space at 360px.
    await expect(alertsCard.getByText('Alertas Sin Redundancia E2E')).toHaveCount(0);

    // Cleanup
    const matches = await apiListRecords(page.request, 'matches', `competition='${compId}'`);
    for (const m of matches) await apiDeleteRecord(page.request, 'matches', m.id);
    await apiDeleteRecord(page.request, 'competitions', compId);
  });

  test('F1: bulk "marcar bolas entregadas" button label wraps inside the button at 360px, no spillover', async ({ page }) => {
    await getSuperuserToken(page);
    const data = loadTestData();

    // A pair with balls unset (default) triggers HasNoBolas, which renders
    // the bulk "Marcar todas las bolas como entregadas" button.
    const compId = await apiCreateRecord(page.request, 'competitions', {
      name: `F1 Bolas Overflow ${Date.now()}`, type: 'league', active: true,
      pairs: [data.pair1Id, data.pair2Id],
    });

    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    await page.waitForLoadState('domcontentloaded');

    const bolasBtn = page.getByRole('button', { name: 'Marcar todas las bolas como entregadas' });
    await expect(bolasBtn).toBeVisible();
    // The button must grow to fit its (possibly two-line) label — content
    // taller than the button's own box means the label text is spilling
    // outside the button's visual bounds instead of being contained by it.
    const overflowsOwnBox = await bolasBtn.evaluate(el => el.scrollHeight > el.clientHeight + 1);
    expect(overflowsOwnBox, 'button label must not spill outside the button').toBe(false);

    // Cleanup
    await apiDeleteRecord(page.request, 'competitions', compId);
  });
});
