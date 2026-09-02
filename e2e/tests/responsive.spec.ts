import { test, expect, APIRequestContext } from '@playwright/test';
import { loginAs, loadTestData, isMobile, openDrawer, navViaDrawer, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

const MOBILE = { width: 375, height: 812 };

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
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
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
    await expect(standingsCard.getByText('Pareja Alpha').first()).toBeVisible();
  });

  test('admin pairs', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/pairs');
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
    // 390px — below sm the page must show a card per player instead, with
    // the "Editar"/"Regenerar enlace" actions reachable without scrolling
    // sideways (see review-principles.md).
    await expect(page.locator('table#players-table')).toBeHidden();
    const cards = page.locator('#players-cards');
    await expect(cards.getByText('Test Player', { exact: true })).toBeVisible();
    await expect(cards.getByText('Test Player 2', { exact: true })).toBeVisible();
    const firstCard = cards.locator('> div').first();
    await expect(firstCard).toBeVisible();
    await expect(firstCard.getByText('Editar')).toBeVisible();
    await expect(firstCard.getByText('Regenerar enlace')).toBeVisible();
  });

  test('admin venues', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/venues');
    await checkNoOverflow(page);
    await expect(page.getByText('Pista Central')).toBeVisible();
  });

  test('admin invitations (inline on competition detail)', async ({ page }) => {
    // Invitations moved from a standalone /admin/invitations page into the
    // competition detail page, like Documentos — reached via a real click.
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.locator('.card-title', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);
    await expect(page.getByRole('heading', { name: 'Invitaciones' })).toBeVisible();

    // R-review: Usos/Expira/Enlace/Revocar columns are off-screen at 390px,
    // hiding the page's main action (Copiar). Below sm, a card per invitation
    // must keep "Copiar" reachable without a sideways scroll.
    await page.locator('button:has-text("Nueva invitación")').click();
    await page.locator('#modal-create-invite input[name="email"]').fill(`resp-mobile-${Date.now()}@example.com`);
    await page.locator('#modal-create-invite button[type="submit"]').click();
    await page.waitForLoadState('networkidle');

    const table = page.locator('table').filter({ hasText: 'Destinatario' });
    await expect(table).toBeHidden();
    const card = page.locator('.sm\\:hidden.divide-y > div').first();
    await expect(card).toBeVisible();
    await expect(card.getByText('Copiar')).toBeVisible();
  });

  test('admin disputes', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navViaDrawer(page, '/admin/disputes');
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
    await expect(page.locator('text=Competiciones activas')).toBeVisible();
  });

  test('R-review: pair/player history renders as cards, not a table, at 390px', async ({ page }) => {
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
    // Click through via a real affordance (the pair link on the competition page).
    await page.locator(`a[href="/pair/${data.pair1Id}"]`).first().click();
    await page.waitForLoadState('domcontentloaded');
    await checkNoOverflow(page);

    await expect(page.getByRole('heading', { name: 'Últimos partidos' })).toBeVisible();
    // Below sm, resultHistoryRow's table must be hidden — the score wraps
    // onto multiple lines inside a <td> and clips the V/D column off-screen
    // at 390px otherwise (see review-principles.md).
    const table = page.locator('table.table-sm').filter({ hasText: '6-3' });
    await expect(table).toBeHidden();
    const card = page.locator('.sm\\:hidden.space-y-2 > a').first();
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
    // every alert row is redundant at any width, and wastes space at 390px.
    await expect(alertsCard.getByText('Alertas Sin Redundancia E2E')).toHaveCount(0);

    // Cleanup
    const matches = await apiListRecords(page.request, 'matches', `competition='${compId}'`);
    for (const m of matches) await apiDeleteRecord(page.request, 'matches', m.id);
    await apiDeleteRecord(page.request, 'competitions', compId);
  });
});
