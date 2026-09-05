import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER2_EMAIL, PLAYER2_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore } from '../tour-helpers';

const BASE = `http://localhost:${process.env.E2E_PORT || 8099}`;

function suToken(): string {
  return loadTestData().adminToken;
}

async function suPatch(path: string, data: Record<string, unknown>): Promise<void> {
  const resp = await fetch(`${BASE}${path}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', Authorization: suToken() },
    body: JSON.stringify(data),
  });
  if (!resp.ok) {
    throw new Error(`suPatch ${path}: ${resp.status} ${await resp.text()}`);
  }
}

async function suPost(path: string, data: Record<string, unknown>): Promise<any> {
  const resp = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: suToken() },
    body: JSON.stringify(data),
  });
  if (!resp.ok) {
    throw new Error(`suPost ${path}: ${resp.status} ${await resp.text()}`);
  }
  return resp.json();
}

async function suGet(path: string): Promise<any> {
  const resp = await fetch(`${BASE}${path}`, {
    headers: { Authorization: suToken() },
  });
  return resp.json();
}

test.describe('match lifecycle', () => {
  test('player can view match detail', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
    await expect(page.getByText('Pareja Beta').first()).toBeVisible();
    // Competition + round live in the breadcrumb; the match card must NOT repeat
    // them on the full match page (kills the .Mode.Full gate if reverted).
    await expect(page.locator('.breadcrumbs').getByText('Jornada', { exact: false })).toBeVisible();
    await expect(page.locator('.card').getByText(/Jornada \d/)).toHaveCount(0);
  });

  test('player can submit score', async ({ page }, testInfo) => {
    const matchId = scratchMatchId("submit-score", testInfo.project.name);
    await suPatch(`/api/collections/matches/records/${matchId}`, { date: '2025-03-15', club: 'Padel 360' });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.score-cell').first()).toBeVisible({ timeout: 5000 });
    await enterScore(page, '6-3 6-4');
    await page.getByRole('button', { name: 'Enviar resultado' }).click();
    await page.waitForURL(`**/match/${matchId}`, { timeout: 10000 });
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('6-3 6-4').first()).toBeVisible({ timeout: 5000 });
  });

  test('home page shows matches', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');
    await expect(page.locator('a[href^="/match/"]').first()).toBeVisible();
  });

  test('admin override uses masked score component', async ({ page }, testInfo) => {
    const matchId = scratchMatchId('lifecycle-ui', testInfo.project.name);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    // The admin override lives in the single result panel (lazy thread fragment).
    await page.waitForSelector('#result-panel', { timeout: 10000 });
    const collapseTitle = page.locator('.collapse-title', { hasText: /corrección de administrador/i });
    await expect(collapseTitle).toBeVisible({ timeout: 5000 });
    await collapseTitle.locator('..').locator('input[type="checkbox"]').click();
    const overrideForm = page.locator('form[hx-post*="admin-override"]');
    await expect(overrideForm.locator('.score-input')).toBeVisible({ timeout: 3000 });
    await expect(overrideForm.locator('.score-cell').first()).toBeVisible();
  });

  test('counter-propose uses masked score component', async ({ page }, testInfo) => {
    const matchId = scratchMatchId('lifecycle-ui', testInfo.project.name);
    const adminId = (await suGet(`/api/collections/users/records?filter=email='${ADMIN_EMAIL}'`)).items[0].id;
    await suPatch(`/api/collections/matches/records/${matchId}`, {
      status: 'scheduled', submitted_by: adminId, date: '2025-03-15', club: 'Padel 360',
    });
    await suPost('/api/collections/match_messages/records', {
      match: matchId, type: 'result_submission', proposal_status: 'pending',
      proposal_data: JSON.stringify({ scores: '6-3 6-4' }),
      author: adminId,
    });
    await loginAs(page, PLAYER2_EMAIL, PLAYER2_PASSWORD);
    await page.goto(`/match/${matchId}`);
    const counterBtn = page.locator('#thread-details button:has-text("Contraproponer")').first();
    await expect(counterBtn).toBeVisible({ timeout: 10000 });
    await counterBtn.click();
    const counterForm = page.locator('.counter-form:visible').first();
    await expect(counterForm.locator('.score-input')).toBeVisible({ timeout: 3000 });
    await expect(counterForm.locator('.score-cell').first()).toBeVisible();
  });

  test('admin resolve uses masked score component', async ({ page }, testInfo) => {
    const matchId = scratchMatchId('lifecycle-ui', testInfo.project.name);
    await suPatch(`/api/collections/matches/records/${matchId}`, {
      status: 'disputed', scores: '6-3 6-4', disputed_scores: '4-6 6-3 7-5', review_type: 'score',
    });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    const resolveForm = page.locator('form[hx-post*="disputes"]').filter({ has: page.locator('button:has-text("Resolver")') });
    await expect(resolveForm.locator('.score-input')).toBeVisible({ timeout: 5000 });
    await expect(resolveForm.locator('.score-cell').first()).toBeVisible();
    // The proposed results are clickable boxes (not tiny relabeled buttons);
    // clicking one fills the resolve score input with that box's score.
    const boxes = page.locator('.dispute-resolve button.card');
    await expect(boxes).toHaveCount(2);
    await boxes.first().click();
    const si = resolveForm.locator('.score-input');
    await expect(si.locator('[name="s1a"]')).toHaveValue('6');
    await expect(si.locator('[name="s1b"]')).toHaveValue('3');
  });

  test('match page shows precedentes strip from a prior meeting between the same pairs', async ({ page }, testInfo) => {
    // Uses its own competition + pairs, not the shared seeded pair1/pair2 —
    // Precedents() picks the most recent 'final' meeting between two pairs,
    // so if another spec (e.g. the mobile tour) creates a later final match
    // between the shared pairs, it silently outranks this test's fixture.
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const comp = await suPost('/api/collections/competitions/records', {
      name: `Precedentes Test ${suffix}`, type: 'league', active: true,
    });
    const pA1 = await suPost('/api/collections/users/records', {
      email: `prec-a1-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Prec A1 ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const pA2 = await suPost('/api/collections/users/records', {
      email: `prec-a2-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Prec A2 ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const pB1 = await suPost('/api/collections/users/records', {
      email: `prec-b1-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Prec B1 ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const pB2 = await suPost('/api/collections/users/records', {
      email: `prec-b2-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Prec B2 ${suffix}`, roles: ['player'], verified: true, gender: 'male',
    });
    const pairA = await suPost('/api/collections/pairs/records', {
      name: `Prec Pareja A ${suffix}`, player1: pA1.id, player2: pA2.id,
    });
    const pairB = await suPost('/api/collections/pairs/records', {
      name: `Prec Pareja B ${suffix}`, player1: pB1.id, player2: pB2.id,
    });
    await suPatch(`/api/collections/competitions/records/${comp.id}`, {
      pairs: [pairA.id, pairB.id],
    });

    // Finalize a prior meeting between the two dedicated pairs.
    const prior = await suPost('/api/collections/matches/records', {
      competition: comp.id,
      pair1: pairA.id,
      pair2: pairB.id,
      status: 'final',
      scores: '6-2 6-1',
      winner: pairA.id,
      round_number: 98,
    });

    // A second, pending match between the same pairs — the one we view.
    const current = await suPost('/api/collections/matches/records', {
      competition: comp.id,
      pair1: pairA.id,
      pair2: pairB.id,
      status: 'pending',
      round_number: 97,
    });

    await loginAs(page, pA1.email, 'testpass123456');
    await page.goto(`/match/${current.id}`);
    await expect(page.getByRole('heading', { name: 'Precedentes' })).toBeVisible();
    const strip = page.locator('.card', { has: page.getByRole('heading', { name: 'Precedentes' }) });
    await expect(strip.getByText('6-2 6-1')).toBeVisible();
    await strip.locator('a[href^="/match/"]').click();
    await expect(page).toHaveURL(new RegExp(`/match/${prior.id}$`));
  });

  test('player cannot access match of another competition', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/match/nonexistent-id');
    await expect(page.getByText('no encontrado')).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('link', { name: 'Volver al inicio' })).toBeVisible();
  });
});
