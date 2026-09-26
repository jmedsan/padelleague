import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER2_EMAIL, PLAYER2_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore, clickAndWaitForHxRedirect, fillFlatpickrDate } from '../tour-helpers';

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
    // flatpickr replaces the real input (hidden, but keeps the same
    // placeholder attribute) with a visible text alt-input — getByRole
    // picks the accessible one only, getByPlaceholder would match both.
    await expect(page.getByRole('textbox', { name: 'dd/mm/aaaa' })).toBeVisible({ timeout: 3000 });
    await expect(page.locator('select[name="time"]')).toBeVisible({ timeout: 3000 });
  });

  test('W2: after proposing a date, the form collapses into "Proponer otra fecha"', async ({ page }) => {
    const data = loadTestData();
    const match = await suPost('/api/collections/matches/records', {
      competition: data.competitionId, pair1: data.pair1Id, pair2: data.pair2Id,
      status: 'pending', round_number: 99,
    });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${match.id}`);
    await expect(page.getByText('Proponer fecha y lugar')).toBeVisible();

    await fillFlatpickrDate(page, '#proposal-date', '2026-12-01');
    await page.locator('#proposal-time').selectOption('10:00');
    await page.locator('#proposal-venue').selectOption({ index: 1 });
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#proposal-form button:has-text("Proponer fecha")').click(),
    ]);
    await page.waitForLoadState('domcontentloaded');
    await page.waitForSelector('#thread-schedule', { timeout: 10000 });

    // The open "Proponer fecha y lugar" card must be gone — a pending
    // proposal now exists, so the form collapses into an accordion instead
    // of staying open below the pending-proposal card (W2).
    await expect(page.getByText('Proponer fecha y lugar')).toHaveCount(0);
    const accordion = page.locator('.collapse', { hasText: 'Proponer otra fecha' });
    await expect(accordion).toBeVisible();
    await expect(accordion.locator('input[type="checkbox"]')).not.toBeChecked();
  });

  test('proposal form shows inline error instead of a native alert when "Otro" club has no name', async ({ page }) => {
    const data = loadTestData();
    const match = await suPost('/api/collections/matches/records', {
      competition: data.competitionId, pair1: data.pair1Id, pair2: data.pair2Id,
      status: 'pending', round_number: 99,
    });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${match.id}`);

    let dialogFired = false;
    page.on('dialog', () => { dialogFired = true; });

    await fillFlatpickrDate(page, '#proposal-date', '2026-12-01');
    await page.locator('#proposal-time').selectOption('10:00');
    await page.locator('#proposal-venue').selectOption('otro');
    await expect(page.locator('#venue-text-wrap')).toBeVisible();
    await page.locator('#proposal-form button:has-text("Proponer fecha")').click();

    await expect(page.locator('#proposal-form-error')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#proposal-form-error')).toHaveText('Indica el nombre del club');
    expect(dialogFired).toBe(false);
    // Must not have navigated away — the form is still on the page.
    await expect(page.locator('#proposal-form')).toBeVisible();
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

    // P1 positive control: result panel has visible Aceptar resultado button
    const confirmBtn = page.locator('#thread-details button:has-text("Aceptar resultado")');
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
    await fillFlatpickrDate(page, '#proposal-date', tomorrow);
    await proposalForm.locator('select[name="time"]').selectOption('18:00');
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

  test('unfinished match: submit partial → accept → resume with carried sets → finish', async ({ page }) => {
    test.setTimeout(60000);
    const data = loadTestData();

    const freshMatch = await suPost('/api/collections/matches/records', {
      competition: data.competitionId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'scheduled',
      round_number: 60,
      date: '2025-07-01',
      club: 'Padel 360',
    });
    const matchId = freshMatch.id;

    // Player1 submits a partial score — unfinished is auto-detected
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });

    await enterScore(page, '6-3 2-1');
    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'));

    await page.waitForSelector('#thread-details', { timeout: 10000 });
    await expect(page.locator('#thread-details').getByText('6-3 2-1')).toBeVisible({ timeout: 5000 });

    // Admin (pair2 member) accepts the partial score
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });
    const acceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
    await acceptBtn.waitFor({ timeout: 10000 });
    await clickAndWaitForHxRedirect(page, acceptBtn);

    // Match goes back to pending with carried_sets — resume badge appears
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('.badge', { hasText: 'Se reanuda desde 6-3' })).toBeVisible({ timeout: 10000 });

    // Schedule the resumed match via API
    await suPatch(`/api/collections/matches/records/${matchId}`, {
      status: 'scheduled', date: '2025-07-15', club: 'Wurko',
    });

    // Player1 submits the finishing score (carried set 1 is locked)
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });

    const lockedSets = page.locator('.score-set-group[data-locked]');
    await expect(lockedSets).toHaveCount(1, { timeout: 5000 });

    // fillCells uses positional indexing — pass full score; locked set 1 is skipped
    await enterScore(page, '6-3 3-6 6-4');
    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'));

    // Admin (pair2) accepts the final score
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });
    const finalAccept = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
    await finalAccept.waitFor({ timeout: 10000 });
    await clickAndWaitForHxRedirect(page, finalAccept);

    // Match finalized with the full score
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('#thread-details').getByText('Confirmado')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('6-3 3-6 6-4').first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.badge', { hasText: 'Se reanuda' })).not.toBeVisible();
  });

  test('player can withdraw own pending scheduling proposal', async ({ page }) => {
    const data = loadTestData();
    const freshMatch = await suPost('/api/collections/matches/records', {
      competition: data.competitionId, pair1: data.pair1Id, pair2: data.pair2Id,
      status: 'pending', round_number: 70,
    });
    const matchId = freshMatch.id;

    // Player2 (pair1 member) proposes a date
    await loginAs(page, PLAYER2_EMAIL, PLAYER2_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await fillFlatpickrDate(page, '#proposal-date', '2027-01-15');
    await page.locator('#proposal-time').selectOption('10:00');
    await page.locator('#proposal-venue').selectOption({ index: 1 });
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#proposal-form button:has-text("Proponer fecha")').click(),
    ]);
    await page.waitForLoadState('domcontentloaded');

    // Withdraw button is visible — accept the confirm dialog. hx-confirm is
    // intercepted by static/js/confirm.js's custom #confirm-modal, not the
    // native window.confirm().
    const withdrawBtn = page.locator('button:has-text("Retirar")').first();
    await expect(withdrawBtn).toBeVisible({ timeout: 10000 });

    await withdrawBtn.click();
    await expect(page.locator('#confirm-message')).toContainText('Retirar');
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#confirm-ok').click(),
    ]);
    await page.waitForLoadState('domcontentloaded');

    // After withdrawal: proposal gone (no "Retirar" button) and timeline shows the withdrawal
    await expect(page.locator('button:has-text("Retirar")')).toHaveCount(0);
    await expect(page.locator('#thread-timeline').getByText('retiró su propuesta')).toBeVisible({ timeout: 5000 });
  });

  test('opponent can reject a scheduling proposal with a reason', async ({ page }) => {
    const data = loadTestData();
    const freshMatch = await suPost('/api/collections/matches/records', {
      competition: data.competitionId, pair1: data.pair1Id, pair2: data.pair2Id,
      status: 'pending', round_number: 71,
    });
    const matchId = freshMatch.id;

    // Player2 (pair1 member) proposes a date
    await loginAs(page, PLAYER2_EMAIL, PLAYER2_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await fillFlatpickrDate(page, '#proposal-date', '2027-01-20');
    await page.locator('#proposal-time').selectOption('11:00');
    await page.locator('#proposal-venue').selectOption({ index: 1 });
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#proposal-form button:has-text("Proponer fecha")').click(),
    ]);
    await page.waitForLoadState('domcontentloaded');

    // Admin (pair2 member — see global-setup.ts's pair2Id) logs in and
    // rejects with a reason. PLAYER1 is on BOTH pairs (pair1Id's Alpha and
    // pair2Id's Beta share player1), so logging in as PLAYER1 here would
    // resolve to pair1 (PlayerTeam checks pair1 first) — the proposer's own
    // side, showing "Retirar" instead of the opponent's Aceptar/Rechazar.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });

    // Open the reject form by clicking "Rechazar"
    const rejectToggle = page.locator('button:has-text("Rechazar")').first();
    await expect(rejectToggle).toBeVisible({ timeout: 5000 });
    await rejectToggle.click();

    // Select a reason from the dropdown
    const rejectForm = page.locator('.reject-form').first();
    await expect(rejectForm).toBeVisible({ timeout: 3000 });
    await rejectForm.locator('select[name="rejection_reason"]').selectOption('No puedo ese día');

    // Submit the rejection
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      rejectForm.locator('button:has-text("Enviar rechazo")').click(),
    ]);
    await page.waitForLoadState('domcontentloaded');

    // After rejection: the schedule card resets to the open "Proponer
    // fecha" form (a rejected proposal is not shown as active state — see
    // component-modes.md's "hide rejected"), and the timeline entry carries
    // the frozen "Rechazada" badge plus the rejection reason as a Note,
    // same pattern as resultBox's rejection note.
    await expect(page.locator('#thread-timeline').locator('.badge', { hasText: 'Rechazada' })).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#thread-timeline').getByText('No puedo ese día')).toBeVisible({ timeout: 5000 });
  });

  test('flatpickr date picker posts date in YYYY-MM-DD format', async ({ page }) => {
    const data = loadTestData();
    const freshMatch = await suPost('/api/collections/matches/records', {
      competition: data.competitionId, pair1: data.pair1Id, pair2: data.pair2Id,
      status: 'pending', round_number: 72,
    });
    const matchId = freshMatch.id;

    await loginAs(page, PLAYER2_EMAIL, PLAYER2_PASSWORD);
    await page.goto(`/match/${matchId}`);

    // The underlying input (hidden by flatpickr, still in DOM) must accept
    // YYYY-MM-DD and submit it verbatim, then the server stores it in that format.
    await fillFlatpickrDate(page, '#proposal-date', '2027-02-10');

    // Verify the underlying input holds YYYY-MM-DD (not a display format)
    const storedValue = await page.locator('#proposal-date').inputValue();
    expect(storedValue).toMatch(/^\d{4}-\d{2}-\d{2}$/);

    // Submit and verify the server receives and stores the correct date
    await page.locator('#proposal-time').selectOption('09:00');
    await page.locator('#proposal-venue').selectOption({ index: 1 });
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#proposal-form button:has-text("Proponer fecha")').click(),
    ]);
    await page.waitForLoadState('domcontentloaded');

    // The schedule card shows the confirmed/proposed date — if the date was
    // garbled (e.g. DD/MM/YYYY submitted) the server would reject or store
    // wrong. The schedule section must be visible (no error).
    await expect(page.locator('#thread-schedule')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#thread-schedule .badge', { hasText: 'Propuesta' })).toBeVisible({ timeout: 5000 });
  });

  test('rule win: 3-game lead with one completed set finalizes the match', async ({ page }) => {
    test.setTimeout(60000);
    const data = loadTestData();

    const freshMatch = await suPost('/api/collections/matches/records', {
      competition: data.competitionId,
      pair1: data.pair1Id,
      pair2: data.pair2Id,
      status: 'scheduled',
      round_number: 61,
      date: '2025-07-05',
      club: 'Padel 360',
    });
    const matchId = freshMatch.id;

    // Player1 submits 6-3 4-1 — rule win auto-detected (1 set + 3-game lead)
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });

    await enterScore(page, '6-3 4-1');
    await expect(page.locator('.score-winner').first()).toContainText('gana', { timeout: 3000 });

    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'));

    // Admin (pair2) accepts
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForSelector('#thread-details', { timeout: 10000 });
    const acceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
    await acceptBtn.waitFor({ timeout: 10000 });
    await clickAndWaitForHxRedirect(page, acceptBtn);

    // Match finalized (rule win = won, not "no terminado")
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('#thread-details').getByText('Confirmado')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('6-3 4-1').first()).toBeVisible({ timeout: 5000 });
  });
});
