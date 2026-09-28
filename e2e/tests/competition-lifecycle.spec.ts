import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import type { APIRequestContext } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import {
  loginAs, asPlayerOn, loadTestData, isMobile, apiCreateRecord, apiDeleteRecord, apiListRecords,
  suPost as suPostBase, suPatch as suPatchBase,
  ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD,
} from '../helpers';
import { clickAndWaitForHxRedirect, expectSelected } from '../tour-helpers';

function suToken(): string {
  return loadTestData().adminToken;
}

async function suPost(request: APIRequestContext, path: string, data: Record<string, unknown>): Promise<any> {
  return suPostBase(request, suToken(), path, data);
}

async function suPatch(request: APIRequestContext, path: string, data: Record<string, unknown>): Promise<void> {
  await suPatchBase(request, suToken(), path, data);
}

test.describe('competition lifecycle', { tag: '@competitions' }, () => {
  test('admin entry always redirects to the competitions dashboard', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin');
    await page.waitForLoadState('domcontentloaded');
    await expect(page).toHaveURL(/\/admin\/competitions$/);
    await expect(page.getByRole('heading', { name: 'Competiciones', exact: true })).toBeVisible();
  });

  test('admin dashboard shows title and competitions', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: 'Competiciones', exact: true })).toBeVisible();
    await expect(page.getByText('Liga E2E Test').locator('visible=true').first()).toBeVisible();
  });

  test('admin can create a new competition', async ({ page, }, testInfo) => {
    const name = `Liga Nueva ${testInfo.project.name} ${Date.now()}`;
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.getByRole('button', { name: /crear competición/i }).first().click();
    await page.fill('input[name="name"]', name);
    await page.selectOption('select[name="type"]', 'league');
    await clickAndWaitForHxRedirect(page, page.locator('dialog button[type="submit"]'), /^\/admin\/competitions\/[^/]+$/);
    // Scoped to the heading: the site footer also renders this competition's
    // name (single active competition, out-of-context promotion), so the
    // unscoped getByText matches both and violates Playwright's strict mode.
    await expect(page.getByRole('heading', { name })).toBeVisible({ timeout: 10000 });
  });

  test('player can view competition standings', { tag: '@smoke' }, async ({ page, request }, testInfo) => {
    // Self-contained competition + pairs + one played match: the shared seed
    // (data.competitionId / Pareja Alpha) accumulates matches across the
    // whole suite (mobile tour plays some before this runs), so its Forma
    // dot count is not a fixed "1" — asserting on it needs a fresh
    // competition this test fully controls, per season-simulation.spec.ts's
    // pattern of building its own fixtures rather than trusting shared state.
    const suffix = `${testInfo.project.name.charAt(0)}${Date.now() % 100000}`;
    const compName = `Liga Clasif ${suffix}`;
    const makePlayer = async (label: string) => suPost(request, '/api/collections/users/records', {
      email: `clasif-${label}-${suffix}@test.local`,
      display_name: `Clasif ${label} ${suffix}`,
      gender: 'male', roles: ['player'],
      password: 'TestPass123456', passwordConfirm: 'TestPass123456',
      verified: true,
    });
    const [p1, p2, p3, p4] = await Promise.all(['1', '2', '3', '4'].map(makePlayer));
    const comp = await suPost(request, '/api/collections/competitions/records', {
      name: compName, type: 'league', active: true,
    });
    const pairAlpha = await suPost(request, '/api/collections/pairs/records', {
      name: `Pareja Clasif A ${suffix}`,
      player1: p1.id, player2: p2.id,
    });
    const pairBeta = await suPost(request, '/api/collections/pairs/records', {
      name: `Pareja Clasif B ${suffix}`,
      player1: p3.id, player2: p4.id,
    });
    await suPatch(request, `/api/collections/competitions/records/${comp.id}`, {
      pairs: [pairAlpha.id, pairBeta.id],
    });
    await suPost(request, '/api/collections/matches/records', {
      competition: comp.id, pair1: pairAlpha.id, pair2: pairBeta.id,
      status: 'final', round_number: 1, scores: '6-3 6-4', winner: pairAlpha.id,
    });

    await asPlayerOn(page, comp.id, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${comp.id}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText(compName).locator('visible=true').first()).toBeVisible();
    await page.locator('input[aria-label^="Clasificación"]').click();
    // standingsTable.html renders two <table>s (a desktop table.table-zebra
    // and a mobile table.table-sm), each hidden at the other breakpoint via
    // CSS — assert on whichever one is actually visible for this viewport.
    const standingsTable = isMobile(page)
      ? page.locator('table.table-sm')
      : page.locator('table.table-zebra');
    await expect(standingsTable).toBeVisible({ timeout: 5000 });
    await expect(standingsTable.locator('td', { hasText: pairAlpha.name })).toBeVisible();
    await expect(standingsTable.locator('td', { hasText: pairBeta.name })).toBeVisible();

    // Pts column: bold and rightmost on desktop, bold and third (after #,
    // Pareja) on mobile so it's visible without horizontal scrolling. This
    // competition has exactly one played match, so the winner has 3 points.
    const alphaRow = standingsTable.locator('tr', { has: page.locator('td', { hasText: pairAlpha.name }) });
    await expect(alphaRow.locator('td.font-bold', { hasText: '3' })).toBeVisible();
  });

  test('competition page shows match fixtures with the team filter defaulting to all pairs', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('input[aria-label^="Jornadas"]')).toBeVisible();
    await page.locator('input[aria-label^="Jornadas"]').click();
    await expect(page.getByText(/Jornada \d/).locator('visible=true').first()).toBeVisible();
    // Default: every pair's matches are visible, including the player's own.
    const matchLinks = page.locator('a[href^="/match/"]');
    const count = await matchLinks.count();
    expect(count).toBeGreaterThan(0);
    const allText = await matchLinks.allTextContents();
    expect(allText.some(t => t.includes('Pareja Alpha'))).toBe(true);
    // Team filter select, defaulting to "Todas las parejas".
    const filter = page.locator('select[name="pair"]');
    await expect(filter).toBeVisible();
    await expectSelected(filter, 'Todas las parejas');
  });

  test('admin can view competition detail', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.locator('a.card', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('Liga E2E Test').locator('visible=true').first()).toBeVisible();
    const body = await page.textContent('body');
    expect(body).toContain('Pareja Alpha');
  });

  test('team filter select shows only the chosen pair and checks the Jornadas tab', async ({ page, request }, testInfo) => {
    const suffix = `${testInfo.project.name.charAt(0)}${Date.now() % 100000}`;
    const compName = `Liga Filter ${suffix}`;
    const makePlayer = async (label: string) => suPost(request, '/api/collections/users/records', {
      email: `filter-${label}-${suffix}@test.local`,
      display_name: `Filter ${label} ${suffix}`,
      gender: 'male', roles: ['player'],
      password: 'TestPass123456', passwordConfirm: 'TestPass123456',
      verified: true,
    });
    const [p1, p2, p3, p4, p5, p6] = await Promise.all(['1', '2', '3', '4', '5', '6'].map(makePlayer));
    const comp = await suPost(request, '/api/collections/competitions/records', {
      name: compName, type: 'league', active: true,
    });
    const pairMine = await suPost(request, '/api/collections/pairs/records', {
      name: `Pareja Mine ${suffix}`, player1: p1.id, player2: p2.id,
    });
    const pairOther = await suPost(request, '/api/collections/pairs/records', {
      name: `Pareja Other ${suffix}`, player1: p3.id, player2: p4.id,
    });
    const pairThird = await suPost(request, '/api/collections/pairs/records', {
      name: `Pareja Third ${suffix}`, player1: p5.id, player2: p6.id,
    });
    await suPatch(request, `/api/collections/competitions/records/${comp.id}`, {
      pairs: [pairMine.id, pairOther.id, pairThird.id],
    });
    const matchMine = await suPost(request, '/api/collections/matches/records', {
      competition: comp.id, pair1: pairMine.id, pair2: pairThird.id,
      status: 'pending', round_number: 1,
    });
    const matchOther = await suPost(request, '/api/collections/matches/records', {
      competition: comp.id, pair1: pairOther.id, pair2: pairThird.id,
      status: 'pending', round_number: 2,
    });

    await asPlayerOn(page, comp.id, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    // PLAYER1_EMAIL is not on any of these pairs, so all three show as
    // "other" — filtering by pairOther should show only matchOther's link,
    // since matchMine involves pairMine and pairThird, not pairOther.
    await page.goto(`/competition/${comp.id}`);
    await page.waitForLoadState('domcontentloaded');
    await page.locator('input[aria-label^="Jornadas"]').click();

    const filter = page.locator('select[name="pair"]');
    await expect(filter).toBeVisible();
    console.log('OPTIONS BEFORE SELECT:', await filter.innerHTML());
    await Promise.all([
      page.waitForURL(new RegExp(`pair=${pairOther.id}`)),
      filter.selectOption(pairOther.id),
    ]);
    await page.waitForLoadState('domcontentloaded');

    await expect(page.locator(`a[href="/match/${matchOther.id}"]`)).toBeVisible();
    await expect(page.locator(`a[href="/match/${matchMine.id}"]`)).toHaveCount(0);
    await expect(page.locator('input[aria-label^="Jornadas"]')).toBeChecked();
    await expect(page.locator('select[name="pair"]')).toHaveValue(pairOther.id);
  });

  test('draft calendar is hidden from players until published', async ({ page, request }, testInfo) => {
    const suffix = `${testInfo.project.name.charAt(0)}${Date.now() % 100000}`;
    const compName = `Liga Publish ${suffix}`;
    const makePlayer = async (label: string) => suPost(request, '/api/collections/users/records', {
      email: `publish-${label}-${suffix}@test.local`,
      display_name: `Publish ${label} ${suffix}`,
      gender: 'male', roles: ['player'],
      password: 'TestPass123456', passwordConfirm: 'TestPass123456',
      verified: true,
    });
    const [p1, p2, p3, p4] = await Promise.all(['1', '2', '3', '4'].map(makePlayer));
    const comp = await suPost(request, '/api/collections/competitions/records', {
      name: compName, type: 'league', active: true,
    });
    const pairAlpha = await suPost(request, '/api/collections/pairs/records', {
      name: `Pareja Publish A ${suffix}`, player1: p1.id, player2: p2.id,
    });
    const pairBeta = await suPost(request, '/api/collections/pairs/records', {
      name: `Pareja Publish B ${suffix}`, player1: p3.id, player2: p4.id,
    });
    await suPatch(request, `/api/collections/competitions/records/${comp.id}`, {
      pairs: [pairAlpha.id, pairBeta.id],
    });

    // Admin generates the calendar — it starts as a draft, invisible to players.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${comp.id}`);
    await page.waitForLoadState('domcontentloaded');
    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Generar calendario")'), `/admin/competitions/${comp.id}`);
    await expect(page.locator('.badge', { hasText: 'Borrador' })).toBeVisible();
    const publishButton = page.locator('button:has-text("Publicar calendario")');
    await expect(publishButton).toBeVisible();

    // Player sees the draft-calendar empty state, not the rounds.
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${comp.id}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('El calendario aún no está publicado')).toBeVisible();

    // Admin publishes.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${comp.id}`);
    await page.waitForLoadState('domcontentloaded');
    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Publicar calendario")'), `/admin/competitions/${comp.id}`);
    await expect(page.locator('.badge', { hasText: 'Publicado' })).toBeVisible();

    // A participant (p1 is on Pareja A) clicks the bell notification and
    // lands on the competition with the Jornadas tab showing the rounds.
    await loginAs(page, p1.email, 'TestPass123456');
    await page.waitForLoadState('domcontentloaded');

    const mobile = isMobile(page);
    const bellButton = mobile
      ? page.locator('.lg\\:hidden .dropdown button[aria-label^="notificaciones"]')
      : page.locator('.dropdown:has(#notif-dropdown) button[aria-label^="notificaciones"]');
    await bellButton.click();

    const dropdown = mobile
      ? page.locator('.lg\\:hidden .dropdown')
      : page.locator('#notif-dropdown');
    const notifRow = dropdown.locator('a', { hasText: 'Calendario publicado' });
    await expect(notifRow).toBeVisible({ timeout: 5000 });
    await clickAndWaitForHxRedirect(page, notifRow, `/competition/${comp.id}`);

    await expect(page).toHaveURL(new RegExp(`/competition/${comp.id}$`));
    await expect(page.locator('input[aria-label^="Jornadas"]')).toBeVisible();
    await page.locator('input[aria-label^="Jornadas"]').click();
    await expect(page.getByText(/Jornada \d/).locator('visible=true').first()).toBeVisible();
  });

  // Census #16: each tab's empty state was fixed on its own, one commit per
  // tab. This loops every tab panel in views/competition.html, so a new tab
  // without a row here fails the test instead of shipping unchecked.
  test('every competition tab shows its empty state, and its content when there is some', async ({ page }, testInfo) => {
    const template = readFileSync(join(__dirname, '../../views/competition.html'), 'utf8');
    const panels = [...template.matchAll(/data-tabpanel="([^"]+)"/g)].map(m => m[1]);
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const tabs: Record<string, { label: string; empty: string; filled: string }> = {
      jornadas: { label: 'Jornadas', empty: 'No hay jornadas generadas', filled: `Tabs B ${suffix}` },
      anuncios: { label: 'Avisos', empty: 'No hay avisos todavía', filled: `Aviso ${suffix}` },
      documentos: { label: 'Documentos', empty: 'No hay documentos adjuntos', filled: `Reglamento ${suffix}` },
      clasificacion: { label: 'Clasificación', empty: 'No hay datos de clasificación todavía', filled: `Tabs B ${suffix}` },
    };
    expect(panels.sort(), 'every tab panel in competition.html needs a row here').toEqual(Object.keys(tabs).sort());

    const token = suToken();
    const req = page.request;
    const adminId = (await apiListRecords(req, token, 'users', `email = "${ADMIN_EMAIL}"`))[0].id;
    const makePlayer = (tag: string) => apiCreateRecord(req, token, 'users', {
      email: `tabs-${tag}-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Tabs ${tag} ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const [a1, a2, b1, b2] = await Promise.all(['a1', 'a2', 'b1', 'b2'].map(makePlayer));
    const pairA = await apiCreateRecord(req, token, 'pairs', { name: `Tabs A ${suffix}`, player1: a1, player2: a2 });
    const pairB = await apiCreateRecord(req, token, 'pairs', { name: `Tabs B ${suffix}`, player1: b1, player2: b2 });
    const makeComp = (name: string) => apiCreateRecord(req, token, 'competitions', {
      name: `${name} ${suffix}`, type: 'league', active: true, pairs: [pairA, pairB], rounds: 1,
    });
    const emptyComp = await makeComp('Tabs Vacía');
    const fullComp = await makeComp('Tabs Llena');
    const matchId = await apiCreateRecord(req, token, 'matches', {
      competition: fullComp, pair1: pairA, pair2: pairB, status: 'final',
      round_number: 1, scores: '6-3 6-4', winner: pairA,
    });
    const docId = await apiCreateRecord(req, token, 'documents', {
      title: `Reglamento ${suffix}`, url: 'https://example.com/reglamento',
    });
    await suPatch(req, `/api/collections/competitions/records/${fullComp}`, { documents: [docId] });
    const announcementId = await apiCreateRecord(req, token, 'announcements', {
      competition: fullComp, title: `Aviso ${suffix}`, body: 'Aviso de prueba', created_by: adminId,
    });

    try {
      for (const [compId, filled] of [[emptyComp, false], [fullComp, true]] as const) {
        await asPlayerOn(page, compId, `tabs-a1-${suffix}@test.local`, 'testpass123456');
        const compName = filled ? `Tabs Llena ${suffix}` : `Tabs Vacía ${suffix}`;
        await page.locator('a[href^="/competition/"]', { hasText: compName }).first().click();
        await page.waitForURL(`**/competition/${compId}**`);
        for (const panel of panels) {
          const { label, empty, filled: content } = tabs[panel];
          const tab = page.getByRole('tab', { name: label, exact: true });
          await expect(tab, `${label} tab (${filled ? 'filled' : 'empty'})`).toBeVisible();
          await tab.click();
          const body = page.locator(`[data-tabpanel="${panel}"]`);
          await expect(body).toBeVisible();
          if (filled) {
            await expect(body, `${label} shows its content`).toContainText(content);
            await expect(body.getByText(empty)).toHaveCount(0);
          } else {
            await expect(body.getByText(empty), `${label} shows its empty state`).toBeVisible();
          }
        }
      }
    } finally {
      await apiDeleteRecord(req, token, 'announcements', announcementId);
      await apiDeleteRecord(req, token, 'matches', matchId);
      for (const c of [emptyComp, fullComp]) await apiDeleteRecord(req, token, 'competitions', c);
      await apiDeleteRecord(req, token, 'documents', docId);
      for (const p of [pairA, pairB]) await apiDeleteRecord(req, token, 'pairs', p);
      for (const u of [a1, a2, b1, b2]) await apiDeleteRecord(req, token, 'users', u);
    }
  });
});
