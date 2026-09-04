import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, isMobile, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

const BASE = 'http://localhost:8099';

function suToken(): string {
  return loadTestData().adminToken;
}

async function suPost(path: string, data: Record<string, unknown>): Promise<any> {
  const resp = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: suToken() },
    body: JSON.stringify(data),
  });
  if (!resp.ok) throw new Error(`suPost ${path}: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

async function suPatch(path: string, data: Record<string, unknown>): Promise<void> {
  const resp = await fetch(`${BASE}${path}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', Authorization: suToken() },
    body: JSON.stringify(data),
  });
  if (!resp.ok) throw new Error(`suPatch ${path}: ${resp.status} ${await resp.text()}`);
}

test.describe('competition lifecycle', () => {
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
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
  });

  test('admin can create a new competition', async ({ page, }, testInfo) => {
    const name = `Liga Nueva ${testInfo.project.name} ${Date.now()}`;
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.getByRole('button', { name: /crear competición/i }).first().click();
    await page.fill('input[name="name"]', name);
    await page.selectOption('select[name="type"]', 'league');
    await page.locator('dialog button[type="submit"]').click();
    await page.waitForURL(/\/admin/, { timeout: 10000 });
    await page.waitForLoadState('domcontentloaded');
    // Scoped to the heading: the site footer also renders this competition's
    // name (single active competition, out-of-context promotion), so the
    // unscoped getByText matches both and violates Playwright's strict mode.
    await expect(page.getByRole('heading', { name })).toBeVisible({ timeout: 10000 });
  });

  test('player can view competition standings', async ({ page }, testInfo) => {
    // Self-contained competition + pairs + one played match: the shared seed
    // (data.competitionId / Pareja Alpha) accumulates matches across the
    // whole suite (mobile tour plays some before this runs), so its Forma
    // dot count is not a fixed "1" — asserting on it needs a fresh
    // competition this test fully controls, per season-simulation.spec.ts's
    // pattern of building its own fixtures rather than trusting shared state.
    const suffix = `${testInfo.project.name}-${Date.now()}`;
    const compName = `Clasificación Test ${suffix}`;
    const makePlayer = async (label: string) => suPost('/api/collections/users/records', {
      email: `clasif-${label}-${suffix}@test.local`,
      display_name: `Clasif ${label} ${suffix}`,
      gender: 'male', roles: ['player'],
      password: 'TestPass123456', passwordConfirm: 'TestPass123456',
      verified: true,
    });
    const [p1, p2, p3, p4] = await Promise.all(['1', '2', '3', '4'].map(makePlayer));
    const comp = await suPost('/api/collections/competitions/records', {
      name: compName, type: 'league', active: true,
    });
    const pairAlpha = await suPost('/api/collections/pairs/records', {
      name: `Pareja Clasif A ${suffix}`,
      player1: p1.id, player2: p2.id,
    });
    const pairBeta = await suPost('/api/collections/pairs/records', {
      name: `Pareja Clasif B ${suffix}`,
      player1: p3.id, player2: p4.id,
    });
    await suPatch(`/api/collections/competitions/records/${comp.id}`, {
      pairs: [pairAlpha.id, pairBeta.id],
    });
    await suPost('/api/collections/matches/records', {
      competition: comp.id, pair1: pairAlpha.id, pair2: pairBeta.id,
      status: 'final', round_number: 1, scores: '6-3 6-4', winner: pairAlpha.id,
    });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/competition/${comp.id}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText(compName).first()).toBeVisible();
    await page.locator('input[aria-label="Clasificación"]').click();
    const standingsTable = page.locator('table.table-zebra');
    await expect(standingsTable).toBeVisible({ timeout: 5000 });
    await expect(standingsTable.locator('td', { hasText: pairAlpha.name })).toBeVisible();
    await expect(standingsTable.locator('td', { hasText: pairBeta.name })).toBeVisible();

    // Forma column: hidden on phones (hidden sm:table-cell), visible on
    // desktop. This competition has exactly one played match, so the
    // winning pair's row shows one green dot and the loser's shows one red.
    if (!isMobile(page)) {
      await expect(standingsTable.locator('th', { hasText: 'Forma' })).toBeVisible();
      const alphaRow = standingsTable.locator('tr', { has: page.locator('td', { hasText: pairAlpha.name }) });
      await expect(alphaRow.locator('span.bg-success')).toHaveCount(1);
      const betaRow = standingsTable.locator('tr', { has: page.locator('td', { hasText: pairBeta.name }) });
      await expect(betaRow.locator('span.bg-error')).toHaveCount(1);
    }
  });

  test('competition page shows match fixtures with mine-only default', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('input[aria-label="Jornadas"]')).toBeVisible();
    await page.locator('input[aria-label="Jornadas"]').click();
    await expect(page.getByText(/Jornada \d/).first()).toBeVisible();
    // Mine-only: player's own matches visible
    const matchLinks = page.locator('a[href^="/match/"]');
    const count = await matchLinks.count();
    expect(count).toBeGreaterThan(0);
    await expect(matchLinks.first()).toContainText('Pareja Alpha');
    // Toggle shows mine-only/all selector
    const toggle = page.locator('[data-testid="mine-only-toggle"]');
    await expect(toggle).toBeVisible();
    await expect(toggle.getByText('Mis partidos')).toBeVisible();
    await expect(toggle.getByText('Todos')).toBeVisible();
  });

  test('admin can view competition detail', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.locator('a.card', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    const body = await page.textContent('body');
    expect(body).toContain('Pareja Alpha');
  });
});
