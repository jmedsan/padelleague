import { Page, expect, APIRequestContext } from '@playwright/test';
import { ExpectedRow, PairId } from './season-helpers';
import { loginAs } from './helpers';

// ---------------------------------------------------------------------------
// referenceFallback — tracks non-affordance navigations for the guided spec
// ---------------------------------------------------------------------------

interface Fallback {
  reason: string;
  url: string;
}

const fallbacks: Fallback[] = [];

export async function referenceFallback(page: Page, reason: string, url: string): Promise<void> {
  fallbacks.push({ reason, url });
  await page.goto(url);
}

export function collectFallbacks(): Fallback[] {
  return [...fallbacks];
}

export function resetFallbacks(): void {
  fallbacks.length = 0;
}

export function assertFallbacksMatch(actual: Fallback[], expected: string[]): void {
  const reasons = actual.map(f => f.reason);
  expect(reasons.sort()).toEqual([...expected].sort());
}

// ---------------------------------------------------------------------------
// HTMX redirect helper (shared by action helpers)
// ---------------------------------------------------------------------------

export async function clickAndWaitForHxRedirect(page: Page, locator: ReturnType<Page['locator']>): Promise<void> {
  const navPromise = page.waitForEvent('framenavigated', { timeout: 15000 });
  await locator.click();
  await navPromise;
  await page.waitForLoadState('domcontentloaded');
}

// ---------------------------------------------------------------------------
// Action helpers — assume the page is already at the right place
// ---------------------------------------------------------------------------

export async function createPlayer(page: Page, email: string, displayName: string): Promise<void> {
  await page.locator('label[for="precreate-modal"]').first().click();
  const modal = page.locator('.modal[role="dialog"]').filter({ hasText: 'Crear jugador' });
  await modal.locator('input[name="email"]').fill(email);
  await modal.locator('input[name="display_name"]').fill(displayName);
  await modal.locator('select[name="gender"]').selectOption('male');
  const responsePromise = page.waitForResponse(
    resp => resp.url().includes('/admin/players/pre-create'),
  );
  await modal.locator('button[type="submit"]').click();
  const createResp = await responsePromise;
  if (createResp.status() >= 300) {
    throw new Error(`Pre-create failed: ${createResp.status()} for ${email}`);
  }
  await expect(page.getByText('Usuario creado').first()).toBeVisible({ timeout: 5000 });
}

export async function createCompetition(
  page: Page,
  name: string,
  type: 'league' | 'playoff',
  options?: { playTwice?: boolean; suToken?: string },
): Promise<string> {
  const btn = page.getByRole('button', { name: /crear competición/i }).first();
  if (!await btn.isVisible().catch(() => false)) {
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
  }
  await btn.click();
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="type"]').selectOption(type);
  if (options?.playTwice) {
    await dialog.locator('input[name="play_twice"]').check();
  }
  await dialog.locator('input[name="active"]').check();
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));
  const match = page.url().match(/\/admin\/competitions\/([^/]+)/);
  if (match) return match[1];
  const headers: Record<string, string> = {};
  if (options?.suToken) headers['Authorization'] = options.suToken;
  const resp = await page.request.get(
    `/api/collections/competitions/records?filter=name='${name}'&perPage=1`,
    { headers },
  );
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Competition not found after create: ${name}`);
  return id;
}

export async function createPair(
  page: Page,
  name: string,
  player1Id: string,
  player2Id: string,
  suToken: string,
): Promise<string> {
  await page.evaluate(() => {
    (document.getElementById('modal-create') as HTMLDialogElement)?.showModal();
  });
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="player1"]').selectOption(player1Id);
  await dialog.locator('select[name="player2"]').selectOption(player2Id);
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));
  const resp = await page.request.get(`/api/collections/pairs/records?filter=name='${name}'`, {
    headers: { Authorization: suToken },
  });
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Failed to find created pair: ${name}`);
  return id;
}

export async function addPairToCompetition(page: Page, pairId: string, seed?: number): Promise<void> {
  await page.selectOption('select[name="pair"]', pairId);
  if (seed !== undefined) {
    await page.fill('input[name="seed"]', String(seed));
  }
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Añadir")'));
}

export async function generateFixtures(page: Page): Promise<void> {
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Generar calendario")'));
}

export async function setDates(page: Page, startDate: string, endDate: string): Promise<void> {
  // Open the edit modal to set competition start/end dates
  await page.locator('label[for="edit-modal"]').first().click();
  await page.waitForTimeout(300);
  await page.fill('input[name="start_date"]', startDate);
  await page.fill('input[name="end_date"]', endDate);
  await clickAndWaitForHxRedirect(page, page.locator('.modal button:has-text("Guardar")'));
}

export async function activate(page: Page): Promise<void> {
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Activar")'));
}

// markAllPairsPaid marks every pair in the current competition as paid via the
// "Marcar todos como pagado" control on the competition detail page. Reloads
// first so the payment section reflects the pairs just added over HTMX.
export async function markAllPairsPaid(page: Page): Promise<void> {
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  const btn = page.getByRole('button', { name: /marcar todos como pagado/i });
  if (await btn.count() === 0) return;
  await Promise.all([
    page.waitForResponse((r) => r.url().includes('/payment-all')),
    btn.first().click(),
  ]);
  await expect(btn).toHaveCount(0);
}

export async function enterScore(page: Page, score: string, opts?: { suffix?: string }): Promise<void> {
  const root = opts?.suffix
    ? page.locator(`.score-input[data-suffix="${opts.suffix}"]`)
    : page.locator('.score-input').first();
  await root.evaluate((el, s) => {
    (window as any).fillCells(el, s);
  }, score);
}

/** @deprecated Use enterScore instead */
export async function submitScore(page: Page, score: string): Promise<void> {
  await enterScore(page, score);
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'));
}

export async function confirmScore(page: Page): Promise<void> {
  await page.waitForSelector('#thread-details', { timeout: 15000 });
  const acceptBtn = page.locator('#thread-details button:has-text("Confirmar")').first();
  await acceptBtn.waitFor({ timeout: 10000 });
  await clickAndWaitForHxRedirect(page, acceptBtn);
}

export async function disputeScore(page: Page, counterScore = '6-4 4-6 5-7'): Promise<void> {
  await page.waitForSelector('#thread-details', { timeout: 5000 });
  const counterBtn = page.locator('#thread-details button:has-text("Contraproponer")').first();
  await counterBtn.click();
  const counterForm = page.locator('.counter-form:visible').first();
  const scoreInput = counterForm.locator('.score-input').first();
  await scoreInput.waitFor({ state: 'visible', timeout: 3000 });
  const suffix = await scoreInput.getAttribute('data-suffix') ?? undefined;
  await enterScore(page, counterScore, { suffix });
  await clickAndWaitForHxRedirect(page, counterForm.locator('button[type="submit"]'));
}

export async function resolveDispute(page: Page, matchId: string, score: string): Promise<void> {
  const row = page.locator(`form[hx-post*="/admin/disputes/${matchId}/resolve"]`).first();
  const scoreInput = row.locator('.score-input').first();
  await scoreInput.waitFor({ state: 'visible', timeout: 3000 });
  const suffix = await scoreInput.getAttribute('data-suffix') ?? undefined;
  await enterScore(page, score, { suffix });
  page.once('dialog', d => d.accept());
  await clickAndWaitForHxRedirect(page, row.locator('button:has-text("Resolver")'));
}

export async function approveWalkover(page: Page, matchId: string): Promise<void> {
  const row = page.locator(`form[hx-post*="/admin/disputes/${matchId}/walkover-approve"]`).first();
  page.once('dialog', d => d.accept());
  await clickAndWaitForHxRedirect(page, row.locator('button:has-text("Aprobar")'));
}

export async function setPlayoffDate(page: Page, matchId: string, date: string): Promise<void> {
  const row = page.locator(`tr:has(a[href="/match/${matchId}"])`).first();
  await row.locator('input[name="date"]').fill(date);
  await clickAndWaitForHxRedirect(page, row.locator('button:has-text("Guardar")'));
}

// ---------------------------------------------------------------------------
// Document helpers
// ---------------------------------------------------------------------------

export async function createDocument(
  page: Page,
  title: string,
  url: string,
  options?: { mandatory?: boolean; isDefault?: boolean },
): Promise<void> {
  await page.locator('button:has-text("Nuevo documento")').click();
  const dialog = page.locator('dialog#modal-create-doc');
  await dialog.locator('input[name="title"]').fill(title);
  await dialog.locator('input[name="url"]').fill(url);
  if (options?.mandatory) {
    await dialog.locator('input[name="is_mandatory"]').check();
  }
  if (options?.isDefault) {
    await dialog.locator('input[name="is_default"]').check();
  }
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));
}

export async function attachDocumentToCompetition(
  page: Page,
  docTitle: string,
): Promise<void> {
  // Scoped to the form: the sponsors section has an identical "Adjuntar"
  // button for its own attach form, so an unscoped locator hits strict mode.
  const form = page.locator('form:has(select[name="document"])');
  await form.locator('select[name="document"]').selectOption({ label: docTitle });
  await clickAndWaitForHxRedirect(page, form.locator('button:has-text("Adjuntar")'));
}

export async function acceptDocsGate(page: Page): Promise<void> {
  await expect(page.getByRole('heading', { name: 'Documentos obligatorios' })).toBeVisible({ timeout: 5000 });
  const mandatoryLinks = page.locator('[data-mandatory-id]');
  const count = await mandatoryLinks.count();
  for (let i = 0; i < count; i++) {
    await mandatoryLinks.nth(i).click();
  }
  await clickAndWaitForHxRedirect(page, page.locator('#accept-btn'));
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

export async function assertFinalStandings(
  page: Page,
  compId: string,
  pairNames: Record<PairId, string>,
  expected: ExpectedRow[],
  hasPenalties: boolean,
): Promise<void> {
  await page.goto(`/competition/${compId}`);
  await page.locator('input[aria-label="Clasificación"]').click();
  await page.waitForSelector('table.table-zebra tbody tr', { timeout: 5000 });

  const rows = page.locator('table.table-zebra tbody tr');
  const count = await rows.count();
  expect(count).toBe(expected.length);

  for (let i = 0; i < expected.length; i++) {
    const row = rows.nth(i);
    const cells = row.locator('td');
    const exp = expected[i];
    const name = pairNames[exp.pair];

    const setDiff = exp.setsWon - exp.setsLost;
    const gameDiff = exp.gamesWon - exp.gamesLost;

    await expect(cells.nth(0)).toContainText(String(exp.position));
    await expect(cells.nth(1)).toContainText(name);
    await expect(cells.nth(2)).toContainText(String(exp.played));
    await expect(cells.nth(3)).toContainText(String(exp.wins));
    await expect(cells.nth(4)).toContainText(String(exp.losses));
    await expect(cells.nth(5)).toContainText(setDiff >= 0 ? `+${setDiff}` : String(setDiff));
    await expect(cells.nth(6)).toContainText(gameDiff >= 0 ? `+${gameDiff}` : String(gameDiff));
    await expect(cells.nth(7)).toContainText(String(exp.points));

    if (hasPenalties && exp.penalty > 0) {
      await expect(cells.nth(8)).toContainText(`-${exp.penalty}`);
    }
  }
}

export async function assertPlayoffChampion(
  page: Page,
  playoffCompId: string,
  expectedPairName: string,
): Promise<void> {
  await page.goto(`/competition/${playoffCompId}`);
  // The bracket's last round column contains the final match.
  // The winner has `font-bold text-accent` styling.
  const bracketColumns = page.locator('.flex.gap-4 > .flex.flex-col');
  const lastColumn = bracketColumns.last();
  const winner = lastColumn.locator('.font-bold.text-accent').first();
  await expect(winner).toContainText(expectedPairName);
}

// ---------------------------------------------------------------------------
// API helpers (superuser-level, used during setup)
// ---------------------------------------------------------------------------

export async function lookupPlayerId(
  request: APIRequestContext,
  suToken: string,
  email: string,
): Promise<string> {
  const resp = await request.get(
    `/api/collections/users/records?filter=email='${email}'`,
    { headers: { Authorization: suToken } },
  );
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Player not found: ${email}`);
  return id;
}

export async function getRoundMatches(
  request: APIRequestContext,
  suToken: string,
  compId: string,
  round: number,
): Promise<any[]> {
  const resp = await request.get(
    `/api/collections/matches/records?filter=competition='${compId}'&sort=created&perPage=50`,
    { headers: { Authorization: suToken } },
  );
  const items = (await resp.json()).items;
  return items.filter((m: any) => Number(m.round_number) === round);
}

export async function getMatchById(
  request: APIRequestContext,
  suToken: string,
  id: string,
): Promise<any> {
  const resp = await request.get(
    `/api/collections/matches/records/${id}`,
    { headers: { Authorization: suToken } },
  );
  return await resp.json();
}

export async function setMatchDateAndClub(
  request: APIRequestContext,
  suToken: string,
  matchId: string,
  date: string,
  club: string,
): Promise<void> {
  await request.patch(
    `/api/collections/matches/records/${matchId}`,
    {
      headers: { Authorization: suToken },
      data: { date, club },
    },
  );
}

// acceptScheduleProposal drives the real propose+accept flow through the
// thread handler endpoints — POST /match/{id}/thread/proposal as the
// opponent (authorUserId), then POST .../respond with action=accept as a
// player from matchId's other pair — instead of fabricating an already-
// accepted match_messages record directly. Matches what a real UI flow
// leaves behind: ScheduleStatus becomes "confirmed"
// (handlers/public.go:applyProposalToNextMatch, via matches.status =
// "scheduled" set by the accept handler), which is what makes the match
// count as "upcoming" (home.html's Próximos partidos) instead of an
// "organize" action needing a date proposed.
//
// Takes `page` (not a bare APIRequestContext, the prior signature) because
// it needs two real logins — one per participant — which only a Page can
// do (loginAs sets a cookie on page.context()). It only receives a user ID
// for the proposer, not credentials, so it resolves both participants'
// emails via the API and logs each in with the standard tour player
// password. page ends up logged in as the accepter when this returns;
// callers that need a specific session afterward should loginAs again.
//
// date must be today or later — parseProposalForm in handlers/thread.go
// rejects past dates, unlike the raw record write this replaces.
export async function acceptScheduleProposal(
  page: Page,
  suToken: string,
  matchId: string,
  authorUserId: string,
  date: string,
  time: string,
  venueName: string,
): Promise<void> {
  const matchResp = await page.request.get(`/api/collections/matches/records/${matchId}`, {
    headers: { Authorization: suToken },
  });
  if (!matchResp.ok()) {
    throw new Error(`acceptScheduleProposal: match lookup failed: ${matchResp.status()} ${await matchResp.text()}`);
  }
  const match = await matchResp.json();
  const authorPairID = await pairIDForPlayer(page.request, suToken, authorUserId);
  const accepterPairID = match.pair1 === authorPairID ? match.pair2 : match.pair1;
  const accepterUserID = await firstPlayerOfPair(page.request, suToken, accepterPairID);

  const authorEmail = await emailForUser(page.request, suToken, authorUserId);
  const accepterEmail = await emailForUser(page.request, suToken, accepterUserID);

  await loginAs(page, authorEmail, TOUR_PLAYER_PASSWORD);
  await clearDocGateIfPresent(page, matchId);
  const proposeResp = await page.request.post(`/match/${matchId}/thread/proposal`, {
    form: { date, time, venue_id: '', venue_text: venueName },
    maxRedirects: 0,
  });
  const proposeBody = await proposeResp.text();
  const proposeRedirect = proposeResp.headers()['location'] ?? proposeResp.headers()['hx-redirect'];
  if (!proposeResp.ok() || proposeBody.includes('alert-error') || proposeRedirect?.startsWith('/competition/')) {
    throw new Error(`acceptScheduleProposal: propose failed for ${authorEmail}: ${proposeResp.status()} ${proposeBody} (redirect=${proposeRedirect})`);
  }

  // page.request only carries the pb_auth cookie, which the app's
  // CookieAuth middleware deliberately does not copy to an Authorization
  // header for /api/ paths (see middleware/auth.go) — so a raw REST call
  // here needs an explicit Authorization header. The superuser token
  // already in scope is sufficient for this read-only lookup.
  const filter = `match='${matchId}' && type='scheduling_proposal' && proposal_status='pending'`;
  const listResp = await page.request.get(
    `/api/collections/match_messages/records?filter=${encodeURIComponent(filter)}&sort=-created&perPage=1`,
    { headers: { Authorization: suToken } },
  );
  const listBody = await listResp.json();
  const proposalId = listBody.items?.[0]?.id;
  if (!proposalId) {
    throw new Error(`acceptScheduleProposal: no pending proposal found for match ${matchId} after posting (status=${listResp.status()}, body=${JSON.stringify(listBody)})`);
  }

  await loginAs(page, accepterEmail, TOUR_PLAYER_PASSWORD);
  await clearDocGateIfPresent(page, matchId);
  const acceptResp = await page.request.post(
    `/match/${matchId}/thread/proposal/${proposalId}/respond`,
    { form: { action: 'accept' } },
  );
  const acceptBody = await acceptResp.text();
  if (!acceptResp.ok() || acceptBody.includes('alert-error')) {
    throw new Error(`acceptScheduleProposal: accept failed for ${accepterEmail}: ${acceptResp.status()} ${acceptBody}`);
  }
}

// TOUR_PLAYER_PASSWORD is the password every tour-created player is given
// (see createPlayer/setPlayerPassword in the tour specs) — acceptScheduleProposal
// needs it to log the proposer/accepter in for real, since it only receives
// user IDs from its callers, not credentials.
const TOUR_PLAYER_PASSWORD = 'TestPass123456';

async function pairIDForPlayer(request: APIRequestContext, suToken: string, userID: string): Promise<string> {
  const resp = await request.get(
    `/api/collections/pairs/records?filter=${encodeURIComponent(`player1='${userID}' || player2='${userID}'`)}&perPage=1`,
    { headers: { Authorization: suToken } },
  );
  const body = await resp.json();
  const id = body.items?.[0]?.id;
  if (!id) throw new Error(`pairIDForPlayer: no pair found for user ${userID}`);
  return id;
}

async function firstPlayerOfPair(request: APIRequestContext, suToken: string, pairID: string): Promise<string> {
  const resp = await request.get(`/api/collections/pairs/records/${pairID}`, {
    headers: { Authorization: suToken },
  });
  const body = await resp.json();
  if (!body.player1) throw new Error(`firstPlayerOfPair: pair ${pairID} has no player1`);
  return body.player1;
}

async function emailForUser(request: APIRequestContext, suToken: string, userID: string): Promise<string> {
  const resp = await request.get(`/api/collections/users/records/${userID}`, {
    headers: { Authorization: suToken },
  });
  const body = await resp.json();
  if (!body.email) throw new Error(`emailForUser: user ${userID} has no email`);
  return body.email;
}

// clearDocGateIfPresent navigates to the match page and accepts any pending
// mandatory-document gate for the current session — a raw form POST to a
// thread endpoint doesn't trigger the gate's own redirect handling the way
// a real page navigation + submit does (see gotoMatchViaCompetition in the
// tour specs), so callers driving handler endpoints directly must clear it
// first or the POST silently redirects instead of performing the action.
async function clearDocGateIfPresent(page: Page, matchId: string): Promise<void> {
  await page.goto(`/match/${matchId}`);
  await page.waitForLoadState('domcontentloaded');
  if (await page.getByRole('heading', { name: 'Documentos obligatorios' }).isVisible().catch(() => false)) {
    await acceptDocsGate(page);
  }
}
