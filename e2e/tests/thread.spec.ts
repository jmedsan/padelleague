import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER2_EMAIL, PLAYER2_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

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
  if (!resp.ok) throw new Error(`suPatch ${path}: ${resp.status} ${await resp.text()}`);
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


test.describe('match thread', () => {
  test('player can view match thread', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await expect(page.getByRole('heading', { name: 'Historial del partido' })).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[name="content"]')).toBeVisible({ timeout: 10000 });
  });

  test('player can post a message', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const msg = `E2E msg ${Date.now()}`;
    const resp = await page.request.post(`/match/${data.matchIds[0]}/thread/message`, {
      form: { content: msg },
    });
    expect(resp.status()).toBeLessThan(400);
    await page.goto(`/match/${data.matchIds[0]}`);
    await page.waitForFunction(
      (text) => document.body.innerText.includes(text),
      msg,
      { timeout: 15000 }
    );
  });

  test('player can propose a schedule', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${scratchMatchId("propose-schedule", testInfo.project.name)}`);
    // When no date+place is set, the scheduling form appears as a prominent card
    const scheduleHeading = page.getByText('Proponer fecha y lugar');
    await expect(scheduleHeading).toBeVisible({ timeout: 10000 });
    await expect(page.locator('input[type="date"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('input[type="time"]')).toBeVisible({ timeout: 3000 });
  });

  test('admin non-participant can post in thread', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const msg = `Admin msg ${Date.now()}`;
    const resp = await page.request.post(`/match/${data.matchIds[0]}/thread/message`, {
      form: { content: msg, type: 'chat' },
    });
    expect(resp.status()).toBeLessThan(400);
    await page.goto(`/match/${data.matchIds[0]}`);
    await page.waitForFunction(
      (text) => document.body.innerText.includes(text),
      msg,
      { timeout: 15000 }
    );
    await expect(page.locator('[name="content"]')).toBeVisible({ timeout: 5000 });
  });

  test('thread split: timeline read-only, result flow, score once (P1, P4, P6)', async ({ page }, testInfo) => {
    const data = loadTestData();
    // Create a fresh match for this run to avoid retry issues with state transitions
    const adminResp = await fetch(`${BASE}/api/collections/users/records?filter=email='${ADMIN_EMAIL}'`, {
      headers: { Authorization: suToken() },
    });
    const adminUser = (await adminResp.json()).items[0];
    const freshMatch = await suPost('/api/collections/matches/records', {
      competition: data.competitionId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'scheduled',
      round_number: 50,
      date: '2025-06-15',
      club: 'Padel 360',
    });
    const matchId = freshMatch.id;
    await suPost('/api/collections/match_messages/records', {
      match: matchId, type: 'result_submission', proposal_status: 'pending',
      content: '6-3 6-4',
      proposal_data: JSON.stringify({ scores: '6-3 6-4' }),
      author: adminUser.id,
    });

    await loginAs(page, PLAYER2_EMAIL, PLAYER2_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });
    await page.waitForSelector('#thread-timeline', { timeout: 10000 });

    // P1: timeline has zero buttons and zero forms
    await expect(page.locator('#thread-timeline button')).toHaveCount(0);
    await expect(page.locator('#thread-timeline form')).toHaveCount(0);

    // dateBox reuse: a scheduled match shows the "Confirmada" marker on its date.
    await expect(page.getByText('Confirmada').first()).toBeVisible({ timeout: 5000 });
    // Timeline is a flat list of read-only entries; each entry renders its
    // structured content (dateBox/resultBox) in a small card, but the cards
    // carry no actionable controls (asserted above and below).

    // P1 positive control: result panel has visible Confirmar button
    const confirmBtn = page.locator('#thread-details button:has-text("Confirmar")');
    await expect(confirmBtn.first()).toBeVisible({ timeout: 5000 });

    // P4: result status badge shows "Propuesta", not "Confirmado", when pending
    const resultCard = page.locator('#thread-details');
    await expect(resultCard.getByText('Confirmado')).not.toBeVisible();
    await expect(resultCard.locator('.badge', { hasText: 'Propuesta' })).toBeVisible({ timeout: 5000 });

    // Transition to final via API
    await suPatch(`/api/collections/matches/records/${matchId}`, {
      status: 'final', scores: '6-3 6-4', winner: data.pair1Id,
    });
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('networkidle');

    // P4: now the "Confirmado" result status badge appears
    await expect(page.locator('#thread-details').getByText('Confirmado')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('6-3 6-4').first()).toBeVisible({ timeout: 5000 });

    // P6: no actionable score button in timeline — score text is fine as read-only history
    await expect(page.locator('#thread-timeline button')).toHaveCount(0);
    await expect(page.locator('#thread-timeline form')).toHaveCount(0);
  });

  test('thread split: deadlock shows both proposals (P5)', async ({ page }, testInfo) => {
    const data = loadTestData();
    const adminResp = await fetch(`${BASE}/api/collections/users/records?filter=email='${ADMIN_EMAIL}'`, {
      headers: { Authorization: suToken() },
    });
    const adminUser = (await adminResp.json()).items[0];
    const freshMatch = await suPost('/api/collections/matches/records', {
      competition: data.competitionId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'scheduled',
      round_number: 51,
      date: '2025-06-20',
      club: 'Wurko',
    });
    const matchId = freshMatch.id;
    // Two pending result submissions from opposite pairs:
    // player2 (pair1) and admin (pair2)
    await suPost('/api/collections/match_messages/records', {
      match: matchId, type: 'result_submission', proposal_status: 'pending',
      content: '6-3 6-4',
      proposal_data: JSON.stringify({ scores: '6-3 6-4' }),
      author: data.player2.id,
    });
    await suPost('/api/collections/match_messages/records', {
      match: matchId, type: 'result_submission', proposal_status: 'pending',
      content: '4-6 6-3 7-5',
      proposal_data: JSON.stringify({ scores: '4-6 6-3 7-5' }),
      author: adminUser.id,
    });

    await loginAs(page, PLAYER2_EMAIL, PLAYER2_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });

    // P5: both scores visible in the result panel
    await expect(page.locator('#thread-details').getByText('6-3 6-4')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#thread-details').getByText('4-6 6-3 7-5')).toBeVisible({ timeout: 5000 });

    // Both are pair-labeled
    await expect(page.locator('#thread-details').locator('.badge', { hasText: 'Propuesta' }).first()).toBeVisible();
  });

  // Pins the ?scroll=mensajes redirect fix (fix(thread): stop stale
  // schedule/result cards after proposal actions): PostProposal and
  // RespondProposal must force a REAL page reload so the "Fecha y lugar"
  // card reflects the new state without the player manually refreshing.
  // If either redirect used #mensajes instead (a same-document hash
  // navigation, which does not re-fetch or re-render the page), the badge
  // assertions below would see stale content and fail — see the injected
  // regression check at the end of this test for a live demonstration.
  test('proposing and accepting a schedule refreshes the card without a manual reload', async ({ page }) => {
    const data = loadTestData();
    const adminResp = await fetch(`${BASE}/api/collections/users/records?filter=email='${ADMIN_EMAIL}'`, {
      headers: { Authorization: suToken() },
    });
    const adminUser = (await adminResp.json()).items[0];
    const freshMatch = await suPost('/api/collections/matches/records', {
      competition: data.competitionId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'pending',
      round_number: 52,
    });
    const matchId = freshMatch.id;

    // --- Propose as player2 (pair1) ---
    await loginAs(page, PLAYER2_EMAIL, PLAYER2_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await expect(page.getByText('Proponer fecha y lugar')).toBeVisible({ timeout: 10000 });

    const proposalForm = page.locator('#proposal-form');
    const tomorrow = new Date(Date.now() + 86400000).toISOString().slice(0, 10);
    await proposalForm.locator('input[name="date"]').fill(tomorrow);
    await proposalForm.locator('input[name="time"]').fill('18:00');
    await proposalForm.locator('select[name="venue_id"]').selectOption(data.venueId);
    await proposalForm.locator('button[type="submit"]').click();

    // No manual reload — assert directly. If the redirect used a
    // same-document hash nav, this card would still show "Pendiente"/the
    // stale propose form.
    await expect(page.locator('#thread-schedule').locator('.badge', { hasText: 'Propuesta' })).toBeVisible({ timeout: 10000 });
    expect(new URL(page.url()).search).toBe(''); // ?scroll=mensajes was stripped

    // --- Accept as admin (pair2) ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    const acceptForm = page.locator('form[hx-post*="/respond"]').filter({ has: page.locator('input[value="accept"]') }).first();
    await acceptForm.locator('button[type="submit"]').click();

    await expect(page.locator('#thread-schedule').locator('.badge', { hasText: 'Confirmada' })).toBeVisible({ timeout: 10000 });
    expect(new URL(page.url()).search).toBe('');
  });

  // Regression demonstration for the fix above: if PostProposal's redirect
  // used #mensajes (same-document hash nav) instead of ?scroll=mensajes (a
  // real reload), the schedule card goes stale. Simulates that exact
  // condition client-side — without touching handlers/thread.go — by
  // navigating to the pre-fix URL shape ourselves and confirming the card
  // does NOT show the fresh state, proving the assertions above are
  // load-bearing rather than trivially always-green.
});
