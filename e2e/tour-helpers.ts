import { Page, expect, APIRequestContext, Locator } from '@playwright/test';
import { ExpectedRow, PairId } from './season-helpers';
import { loginAs, isMobile, suGet, suPatch, apiGetRecord, apiListRecords, expectingHxRedirect, waitForHxRedirect, clickAndWaitForHxRedirect, clickConfirmAndWaitForHxRedirect } from './helpers';

// Re-exported for backward compatibility: the HTMX redirect helpers live in
// helpers.ts (a dependency-free file overflow-guard.ts and helpers.ts's own
// loginViaForm both need) to avoid a circular import between this file and
// helpers.ts. See helpers.ts for the implementation and design notes.
export { expectingHxRedirect, waitForHxRedirect, clickAndWaitForHxRedirect, clickConfirmAndWaitForHxRedirect };

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
  await expect(page.getByText('Usuario creado').locator('visible=true').first()).toBeVisible({ timeout: 5000 });
}

export async function createCompetition(
  page: Page,
  name: string,
  type: 'league' | 'playoff',
  options?: { playTwice?: boolean },
): Promise<string> {
  // Callers are on the Competiciones page (navTo) when they call this.
  await page.getByRole('button', { name: /crear competición/i }).first().click();
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="type"]').selectOption(type);
  if (options?.playTwice) {
    await dialog.locator('input[name="play_twice"]').check();
  }
  await dialog.locator('input[name="active"]').check();
  // admin_competitions.go's create handler redirects to /admin/competitions/{id}.
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'), /^\/admin\/competitions\/[^/]+$/);
  // The redirect wait above has landed on /admin/competitions/{id}.
  const id = new URL(page.url()).pathname.split('/')[3];
  if (!id) throw new Error(`Competition id missing from ${page.url()} after creating ${name}`);
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
  // admin_pairs.go's create handler redirects to /admin/pairs.
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'), '/admin/pairs');
  const id = (await apiListRecords(page.request, suToken, 'pairs', `name='${name}'`))[0]?.id;
  if (!id) throw new Error(`Failed to find created pair: ${name}`);
  return id;
}

export async function addPairToCompetition(page: Page, pairId: string, seed?: number): Promise<void> {
  // aria-label disambiguates from the leveled-league "Filtrar por pareja" select.
  const compId = page.url().split('/admin/competitions/')[1];
  await page.selectOption('select[aria-label^="Pareja"]', pairId);
  if (seed !== undefined) {
    await page.fill('input[name="seed"]', String(seed));
  }
  // competition_pairs.go's add handler redirects to /admin/competitions/{compId}.
  await clickAndWaitForHxRedirect(
    page,
    page.getByTestId('section-add-pairs').locator('button:has-text("Añadir")'),
    compId ? `/admin/competitions/${compId}` : /^\/admin\/competitions\/[^/]+$/,
  );
}

export async function generateFixtures(page: Page): Promise<void> {
  const compId = page.url().split('/admin/competitions/')[1];
  // fixtures.go's generate handler redirects to /admin/competitions/{compId}.
  await clickAndWaitForHxRedirect(
    page,
    page.locator('button:has-text("Generar calendario")'),
    compId ? `/admin/competitions/${compId}` : /^\/admin\/competitions\/[^/]+$/,
  );
}

export async function setDates(page: Page, startDate: string, endDate: string): Promise<void> {
  await page.locator('label[for="edit-modal"]').first().click();
  await page.waitForTimeout(500);
  await page.evaluate(({ s, e }) => {
    const startEl = document.querySelector<HTMLInputElement>('input[name="start_date"]')!;
    const endEl = document.querySelector<HTMLInputElement>('input[name="end_date"]')!;
    if ((startEl as any)._flatpickr) {
      (startEl as any)._flatpickr.setDate(s, true);
      (endEl as any)._flatpickr.setDate(e, true);
    } else {
      startEl.value = s;
      endEl.value = e;
      startEl.dispatchEvent(new Event('change', { bubbles: true }));
      endEl.dispatchEvent(new Event('change', { bubbles: true }));
    }
  }, { s: startDate, e: endDate });
  // admin_competitions.go's Update handler redirects to /admin/competitions/{id}.
  const compId = page.url().split('/admin/competitions/')[1];
  await clickAndWaitForHxRedirect(
    page,
    page.locator('.modal button:has-text("Guardar")'),
    compId ? `/admin/competitions/${compId}` : /^\/admin\/competitions\/[^/]+$/,
  );
}

// fillFlatpickrDate sets a flatpickr-backed date input's value directly, the
// same way setDates does for the competition edit form: flatpickr hides the
// real <input> (type="hidden") and shows a separate visible alt-input, so
// Playwright's .fill()/waitForSelector (both visibility-gated by default)
// never see it — set .value and dispatch 'change' instead, and wait for the
// element with { state: 'attached' } rather than the default 'visible'.
export async function fillFlatpickrDate(page: Page, selector: string, value: string): Promise<void> {
  await page.waitForSelector(selector, { state: 'attached', timeout: 10000 });
  await page.locator(selector).evaluate((el: HTMLInputElement, v: string) => {
    el.value = v;
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }, value);
}

// markAllPairsPaid marks every pair in the current competition as paid via the
// "Marcar todos como pagado" control on the competition detail page. Reloads
// first so the payment section reflects the pairs just added over HTMX.
export async function markAllPairsPaid(page: Page): Promise<void> {
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  const btn = page.getByRole('button', { name: /marcar todos como pagado/i });
  // Callers have just added unpaid pairs, so the control must be there.
  await expect(btn).toBeVisible();

  // The button carries hx-confirm, intercepted by static/js/confirm.js's
  // custom #confirm-modal — the request only fires once #confirm-ok is
  // clicked (see scheduling-walkover.spec.ts for the same pattern).
  // competition_payments.go's TogglePaymentAll redirects to /admin/competitions/{id}.
  const compId = page.url().split('/admin/competitions/')[1];
  await clickConfirmAndWaitForHxRedirect(
    page,
    btn.first(),
    compId ? `/admin/competitions/${compId}` : /^\/admin\/competitions\/[^/]+$/,
  );
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
  // handlers/match.go's MatchSubmit redirects to /match/{id}.
  const matchId = page.url().split('/match/')[1]?.split(/[?#]/)[0];
  await clickAndWaitForHxRedirect(
    page,
    page.locator('button:has-text("Enviar resultado")'),
    matchId ? `/match/${matchId}` : /^\/match\/[^/?#]+$/,
  );
}

export async function confirmScore(page: Page): Promise<void> {
  await page.waitForSelector('#thread-details', { timeout: 15000 });
  const acceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
  await acceptBtn.waitFor({ timeout: 10000 });
  // handlers/thread.go's RespondProposal redirects to /match/{id}?scroll=mensajes.
  const matchId = page.url().split('/match/')[1]?.split(/[?#]/)[0];
  await clickAndWaitForHxRedirect(
    page,
    acceptBtn,
    matchId ? `/match/${matchId}` : /^\/match\/[^/?#]+$/,
  );
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
  // admin_documents.go's create handler redirects to /admin/documents.
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'), '/admin/documents');
}

export async function attachDocumentToCompetition(
  page: Page,
  docTitle: string,
): Promise<void> {
  // Scoped to the form: the sponsors section has an identical "Adjuntar"
  // button for its own attach form, so an unscoped locator hits strict mode.
  const form = page.locator('form:has(select[name="document"])');
  await form.locator('select[name="document"]').selectOption({ label: docTitle });
  // admin_competitions.go's AttachDocument redirects to /admin/competitions/{id}.
  const compId = page.url().split('/admin/competitions/')[1];
  await clickAndWaitForHxRedirect(
    page,
    form.locator('button:has-text("Adjuntar")'),
    compId ? `/admin/competitions/${compId}` : /^\/admin\/competitions\/[^/]+$/,
  );
}

// ackDocsOnce clears a mandatory-document gate the first time a player
// meets it, as a real player does: open the competition, read the documents,
// accept. `acked` records who already did, so the gate is expected exactly
// once per player (handlers/respond.go checkDocGate blocks the match page and
// every thread action until then).
export async function ackDocsOnce(page: Page, email: string, compId: string, acked: Set<string>): Promise<void> {
  if (acked.has(email)) return;
  // Callers have just logged in, so the player is on home: enter by its card.
  await page.locator(`a[href^="/competition/${compId}"]`).first().click();
  await page.waitForURL(`**/competition/${compId}**`);
  await acceptDocsGate(page);
  acked.add(email);
}

export async function acceptDocsGate(page: Page): Promise<void> {
  await expect(page.getByRole('heading', { name: 'Documentos obligatorios' })).toBeVisible({ timeout: 5000 });
  const mandatoryLinks = page.locator('[data-mandatory-id]');
  const count = await mandatoryLinks.count();
  for (let i = 0; i < count; i++) {
    await mandatoryLinks.nth(i).click();
  }
  // public_competition.go's AcceptDocs redirects to /competition/{id}.
  const compId = page.url().split('/competition/')[1]?.split(/[?#]/)[0];
  await clickAndWaitForHxRedirect(
    page,
    page.locator('#accept-btn'),
    compId ? `/competition/${compId}` : /^\/competition\/[^/?#]+$/,
  );
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// cellByHeader resolves a row's cell by the table's header text (trimmed)
// instead of a hardcoded column index, so assertions survive a table adding
// or reordering columns (e.g. standings-table.html's Pen/Aj. only render
// when the competition has any penalty/adjustment, and the mobile table
// orders columns differently from the desktop one so it fits without
// scrolling). `table` is the ancestor carrying `thead th`; `row` is the
// `tr` (a descendant of `table`) whose `td` at that header's position is
// returned.
export async function cellByHeader(table: Locator, row: Locator, header: string): Promise<Locator> {
  const headers = await table.locator('thead th').allInnerTexts();
  const index = headers.findIndex(h => h.trim() === header);
  if (index === -1) throw new Error(`header "${header}" not found among [${headers.map(h => h.trim()).join(', ')}]`);
  return row.locator('td').nth(index);
}

export async function assertFinalStandings(
  page: Page,
  compId: string,
  pairNames: Record<PairId, string>,
  expected: ExpectedRow[],
  hasPenalties: boolean,
): Promise<void> {
  await page.goto(`/competition/${compId}`);
  await page.locator('input[aria-label^="Clasificación"]').click();
  // standings-table.html renders two separate <table> elements — table-zebra
  // (desktop) and table-sm (mobile) — CSS-hidden at the other breakpoint.
  const table = isMobile(page) ? page.locator('table.table-sm') : page.locator('table.table-zebra');
  await table.locator('tbody tr').first().waitFor({ timeout: 5000 });

  const rows = table.locator('tbody tr');
  const count = await rows.count();
  expect(count).toBe(expected.length);

  for (let i = 0; i < expected.length; i++) {
    const row = rows.nth(i);
    const exp = expected[i];
    const name = pairNames[exp.pair];

    const setDiff = exp.setsWon - exp.setsLost;
    const gameDiff = exp.gamesWon - exp.gamesLost;

    await expect(await cellByHeader(table, row, '#')).toContainText(String(exp.position));
    await expect(await cellByHeader(table, row, 'Pareja')).toContainText(name);
    await expect(await cellByHeader(table, row, 'PJ')).toContainText(String(exp.played));
    await expect(await cellByHeader(table, row, 'PG')).toContainText(String(exp.wins));
    await expect(await cellByHeader(table, row, 'PP')).toContainText(String(exp.losses));
    await expect(await cellByHeader(table, row, 'DS')).toContainText(setDiff >= 0 ? `+${setDiff}` : String(setDiff));
    await expect(await cellByHeader(table, row, 'DJ')).toContainText(gameDiff >= 0 ? `+${gameDiff}` : String(gameDiff));
    await expect(await cellByHeader(table, row, 'Pts')).toContainText(String(exp.points));

    if (hasPenalties && exp.penalty > 0) {
      await expect(await cellByHeader(table, row, 'Pen')).toContainText(`-${exp.penalty}`);
    }
  }
}

// expectSelected asserts a <select>'s chosen option by its visible label
// rather than its value — needed for nullable selects like "Sin clasificar"
// whose value is the empty string, which toHaveValue('') can't distinguish
// from "no option selected yet".
export async function expectSelected(select: Locator, label: string): Promise<void> {
  const selected = select.locator('option:checked');
  await expect(selected).toHaveText(label);
}

// expectRedirectedTo asserts the page ended up at `url` after an admin
// mutation — HTMX mutations in this app redirect via hx-redirect/Location
// on success, so landing anywhere else (the form re-rendered with an error,
// a 404) is a real failure the caller's next assertions would otherwise
// paper over by asserting only on page content.
export async function expectRedirectedTo(page: Page, url: string | RegExp): Promise<void> {
  await expect(page).toHaveURL(url);
}

export async function assertPlayoffChampion(
  page: Page,
  playoffCompId: string,
  expectedPairName: string,
): Promise<void> {
  await page.goto(`/competition/${playoffCompId}`);
  // The bracket's last round column ("Final") contains the final match.
  // The winner has `font-bold text-accent` styling (matchBracketCell).
  const bracketColumns = page.locator('.bracket > .round');
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
  const id = (await apiListRecords(request, suToken, 'users', `email='${email}'`))[0]?.id;
  if (!id) throw new Error(`Player not found: ${email}`);
  return id;
}

export async function getRoundMatches(
  request: APIRequestContext,
  suToken: string,
  compId: string,
  round: number,
): Promise<any[]> {
  const { items } = await suGet(request, suToken, `/api/collections/matches/records?filter=competition='${compId}'&sort=created&perPage=50`);
  return items.filter((m: any) => Number(m.round_number) === round);
}

export async function getMatchById(
  request: APIRequestContext,
  suToken: string,
  id: string,
): Promise<any> {
  return apiGetRecord(request, suToken, 'matches', id);
}

export async function setMatchDateAndClub(
  request: APIRequestContext,
  suToken: string,
  matchId: string,
  date: string,
  club: string,
): Promise<void> {
  await suPatch(request, suToken, `/api/collections/matches/records/${matchId}`, { date, club });
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
// "organize" action needing a date proposed. Both players first clear the
// competition's mandatory-document gate if they haven't (ackDocsOnce).
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
  docsAcked: Set<string>,
): Promise<void> {
  const match = await apiGetRecord(page.request, suToken, 'matches', matchId);
  const authorPairID = await pairIDForPlayer(page.request, suToken, authorUserId);
  const accepterPairID = match.pair1 === authorPairID ? match.pair2 : match.pair1;
  const accepterUserID = await firstPlayerOfPair(page.request, suToken, accepterPairID);

  const authorEmail = await emailForUser(page.request, suToken, authorUserId);
  const accepterEmail = await emailForUser(page.request, suToken, accepterUserID);

  await loginAs(page, authorEmail, TOUR_PLAYER_PASSWORD);
  await ackDocsOnce(page, authorEmail, match.competition, docsAcked);
  // raw-request: a thread form POST (cookie auth); status, body and redirect are checked below.
  const proposeResp = await page.request.post(`/match/${matchId}/thread/proposal`, {
    form: { date, time, venue_id: '', venue_text: venueName },
    maxRedirects: 0,
  });
  const proposeBody = await proposeResp.text();
  const proposeRedirect = proposeResp.headers()['location'] ?? proposeResp.headers()['hx-redirect'];
  if (!proposeResp.ok() || proposeBody.includes('alert-error') || proposeRedirect?.startsWith('/competition/')) {
    throw new Error(`acceptScheduleProposal: propose failed for ${authorEmail}: ${proposeResp.status()} ${proposeBody} (redirect=${proposeRedirect})`);
  }

  const pending = await apiListRecords(page.request, suToken, 'match_messages',
    `match='${matchId}' && type='scheduling_proposal' && proposal_status='pending'`);
  if (pending.length !== 1) {
    throw new Error(`acceptScheduleProposal: expected 1 pending proposal for match ${matchId} after posting, got ${pending.length}`);
  }
  const proposalId = pending[0].id;

  await loginAs(page, accepterEmail, TOUR_PLAYER_PASSWORD);
  await ackDocsOnce(page, accepterEmail, match.competition, docsAcked);
  // raw-request: a thread form POST (cookie auth); status and body are checked below.
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
  const id = (await apiListRecords(request, suToken, 'pairs', `player1='${userID}' || player2='${userID}'`))[0]?.id;
  if (!id) throw new Error(`pairIDForPlayer: no pair found for user ${userID}`);
  return id;
}

async function firstPlayerOfPair(request: APIRequestContext, suToken: string, pairID: string): Promise<string> {
  const body = await apiGetRecord(request, suToken, 'pairs', pairID);
  if (!body.player1) throw new Error(`firstPlayerOfPair: pair ${pairID} has no player1`);
  return body.player1;
}

async function emailForUser(request: APIRequestContext, suToken: string, userID: string): Promise<string> {
  const body = await apiGetRecord(request, suToken, 'users', userID);
  if (!body.email) throw new Error(`emailForUser: user ${userID} has no email`);
  return body.email;
}
