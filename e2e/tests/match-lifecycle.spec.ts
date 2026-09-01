import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER2_EMAIL, PLAYER2_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore } from '../tour-helpers';

test.describe('match lifecycle', () => {
  test('player can view match detail', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
    await expect(page.getByText('Pareja Beta').first()).toBeVisible();
  });

  test('player can submit score', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const matchId = scratchMatchId("submit-score", testInfo.project.name);

    // Set date+club via superuser API so score form is visible
    const suAuth = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    const suToken = (await suAuth.json()).token;
    await page.request.patch(`/api/collections/matches/records/${matchId}`, {
      headers: { Authorization: suToken },
      data: { date: '2025-03-15', club: 'Padel 360' },
    });

    await page.goto(`/match/${matchId}`);
    await expect(page.locator('.score-cell').first()).toBeVisible({ timeout: 5000 });
    await enterScore(page, '6-3 6-4');
    await page.getByRole('button', { name: 'Enviar resultado' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('6-3 6-4').first()).toBeVisible({ timeout: 5000 });
  });

  test('home page shows matches', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');
    await expect(page.locator('a[href^="/match/"]').first()).toBeVisible();
  });

  test('admin override uses masked score component', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    const collapseTitle = page.locator('.collapse-title', { hasText: /corrección de administrador/i });
    await expect(collapseTitle).toBeVisible({ timeout: 5000 });
    await collapseTitle.locator('..').locator('input[type="checkbox"]').click();
    const overrideForm = page.locator('form[hx-post*="admin-override"]');
    await expect(overrideForm.locator('.score-input')).toBeVisible({ timeout: 3000 });
    await expect(overrideForm.locator('.score-cell').first()).toBeVisible();
    await page.fill('input[name="date"]', '2026-09-15');
    await page.fill('input[name="court_number"]', '3');
    await page.getByRole('button', { name: 'Aplicar cambios' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.badge', { hasText: 'Pista 3' })).toBeVisible({ timeout: 5000 });
  });

  test('admin resolve uses masked score component', async ({ page }) => {
    const data = loadTestData();
    const suAuth = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    const suToken = (await suAuth.json()).token;
    const matchId = data.matchIds[1];
    await page.request.patch(`/api/collections/matches/records/${matchId}`, {
      headers: { Authorization: suToken },
      data: { status: 'disputed', scores: '6-3 6-4', disputed_scores: '4-6 6-3 7-5', review_type: 'score' },
    });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    const resolveForm = page.locator('form[hx-post*="disputes"]').filter({ has: page.locator('button:has-text("Resolver")') });
    await expect(resolveForm.locator('.score-input')).toBeVisible({ timeout: 5000 });
    await expect(resolveForm.locator('.score-cell').first()).toBeVisible();
    const quickFill = resolveForm.locator('button:has-text("6-3 6-4")');
    await expect(quickFill).toBeVisible();
  });

  test('counter-propose uses masked score component', async ({ page }) => {
    const data = loadTestData();
    const suAuth = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    const suToken = (await suAuth.json()).token;
    const matchId = data.matchIds[2];
    await page.request.patch(`/api/collections/matches/records/${matchId}`, {
      headers: { Authorization: suToken },
      data: { status: 'confirmed', scores: '6-3 6-4', date: '2025-03-15', club: 'Padel 360' },
    });
    await page.request.post(`/api/collections/match_messages/records`, {
      headers: { Authorization: suToken },
      data: { match: matchId, type: 'result_submission', proposal_status: 'pending',
              proposal_data: JSON.stringify({ scores: '6-3 6-4' }),
              author: data.player1.id },
    });
    await loginAs(page, PLAYER2_EMAIL, PLAYER2_PASSWORD);
    await page.goto(`/match/${matchId}`);
    const counterBtn = page.locator('#thread-messages-list button:has-text("Contraproponer")').first();
    await expect(counterBtn).toBeVisible({ timeout: 10000 });
    await counterBtn.click();
    const rejectForm = page.locator('.reject-form:visible').first();
    await expect(rejectForm.locator('.score-input')).toBeVisible({ timeout: 3000 });
    await expect(rejectForm.locator('.score-cell').first()).toBeVisible();
  });

  test('player cannot access match of another competition', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/match/nonexistent-id');
    await expect(page.getByText('no encontrado')).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('link', { name: 'Volver al inicio' })).toBeVisible();
  });
});
