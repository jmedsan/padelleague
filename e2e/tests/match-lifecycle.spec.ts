import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER2_EMAIL, PLAYER2_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore } from '../tour-helpers';

const BASE = 'http://localhost:8099';

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
    const quickFill = resolveForm.locator('button:has-text("6-3 6-4")');
    await expect(quickFill).toBeVisible();
  });

  test('player cannot access match of another competition', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/match/nonexistent-id');
    await expect(page.getByText('no encontrado')).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('link', { name: 'Volver al inicio' })).toBeVisible();
  });
});
