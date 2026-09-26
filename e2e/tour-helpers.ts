import { Page, expect, APIRequestContext, Locator } from '@playwright/test';
import { ExpectedRow, PairId } from './season-helpers';
import { loginAs, isMobile } from './helpers';

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

// Both helpers require the caller's expected redirect target (pathname match
// against the server's own HX-Redirect header), then wait for the redirect to
// actually land, then 'load' if it was a real document navigation. Earlier
// signals were each racy or vacuous on their own:
//   - 'framenavigated' has no frame filter, so it also fires for child-frame
//     or same-document navigations unrelated to the HX-Redirect — it can
//     resolve before the real target document request even starts.
//   - a bare URL match (the old expectRedirectedTo-after-click pattern) is a
//     no-op when the action redirects back to the SAME url the page was
//     already on (e.g. the penalty-confirm step on /admin/competitions/{id}
//     redirects to /admin/competitions/{id}) — toHaveURL is already
//     satisfied, so it asserts nothing and returns immediately, before the
//     navigation lands, AND proves nothing even if it later does: a server
//     bug that redirected somewhere else first, then bounced back, would
//     still pass.
//   - registering the navigation waiter AFTER awaiting the htmx POST response
//     is too late: htmx sets location.href the instant that response lands,
//     so the target document request can arrive and be missed before the
//     waiter is even registered, timing out. The listener has to be attached
//     before act() fires the click.
//   - matching on recorded main-frame navigation responses breaks for any
//     HX-Redirect whose target differs from the current URL only by its hash
//     (e.g. PostMessage's redirectHX(e, "/match/"+id+"#mensajes") from that
//     same match page): per the HTML spec that's a same-document fragment
//     navigation — no network request, no 'load' event — so nothing ever
//     lands and the wait times out. A first fix special-cased "same
//     pathname+search" as fragment-only, but that also wrongly covers a
//     target that is BYTE-FOR-BYTE identical to the current URL (e.g.
//     createPair's redirectHX(e, "/admin/pairs") called from a page already
//     there, no query, no hash): the spec forces a full reload for an
//     identical location.href assignment, same as any other cross-document
//     navigation, so treating it as fragment-only returned before that reload
//     even started and the caller's next page.goto raced it ("Navigation …
//     is interrupted", season-simulation.spec.ts's addPairToCompetition right
//     after createPair). Telling these apart by comparing URL strings needs a
//     special case per shape; the browser already knows which one it did.
// A document-identity marker asks the browser directly instead of inferring
// it from URL comparisons: stamp `window.__hxNav` on the page before act()
// runs. A same-document fragment navigation leaves the marker in place; any
// actual document load — including a reload to the identical URL — tears
// down the old window and the marker goes with it. That single signal covers
// a real reload, a same-URL reload, and a fragment-only change without
// naming any of them individually, and it drops the navigation-response
// listener entirely.
//   - even with the marker distinguishing same-document from a real reload
//     correctly, polling page.url() for the target hash still hangs for
//     #mensajes specifically: views/thread.html's onMensajesHash() strips the
//     hash straight back off via history.replaceState the instant it sees
//     #mensajes (on load AND on 'hashchange'), right after triggering the
//     thread's own refresh. htmx's location.href assignment does fire a real
//     hashchange, but the app's own listener consumes it and rewrites the URL
//     before Playwright's page.url() is ever read — so the hash the caller is
//     polling for can never be observed there, by design, regardless of poll
//     frequency. Do NOT "fix" this by asserting on page.url() again; register
//     a hashchange listener BEFORE act() runs instead, so it captures the
//     hash from the event itself (event.newURL) rather than the URL bar
//     after the app's own listener has already run.
async function waitForHxRedirectTarget(page: Page, expectedTarget: string | RegExp, act: () => Promise<void>): Promise<void> {
  await page.evaluate(() => {
    (window as unknown as { __hxNav?: boolean }).__hxNav = true;
    (window as unknown as { __hxHash?: string }).__hxHash = window.location.hash;
    window.addEventListener('hashchange', (e: HashChangeEvent) => {
      (window as unknown as { __hxHash?: string }).__hxHash = new URL(e.newURL).hash;
    });
  });
  const beforeHash = new URL(page.url()).hash;
  const [hxResponse] = await Promise.all([
    page.waitForResponse(
      r => r.request().method() !== 'GET' && r.frame() === page.mainFrame() && r.request().headers()['hx-request'] === 'true',
      { timeout: 15000 },
    ),
    act(),
  ]);
  const target = await hxResponse.headerValue('hx-redirect');
  if (!target) throw new Error(`expected HX-Redirect header, got ${hxResponse.status()} ${hxResponse.url()}`);
  const targetUrl = new URL(target, page.url());
  const targetPath = targetUrl.pathname;
  const matches = typeof expectedTarget === 'string' ? targetPath === expectedTarget : expectedTarget.test(targetPath);
  if (!matches) throw new Error(`HX-Redirect target "${targetPath}" did not match expected ${expectedTarget}`);
  // htmx does `location.href = target` verbatim (static/js/htmx.min.js).
  // Either the marker survives (same document — a fragment-only change; poll
  // the hashchange listener's captured value, not page.url() — see above) or
  // it's gone (a real document load; wait for 'load'). page.evaluate throws
  // mid-navigation while the old document is torn down — that failure IS the
  // "marker gone" signal, not an error.
  const markerSurvived = async () => {
    try {
      return await page.evaluate(() => (window as unknown as { __hxNav?: boolean }).__hxNav === true);
    } catch {
      return false;
    }
  };
  if (targetUrl.hash !== beforeHash && await markerSurvived()) {
    await expect.poll(
      () => page.evaluate(() => (window as unknown as { __hxHash?: string }).__hxHash).catch(() => undefined),
      { timeout: 15000 },
    ).toBe(targetUrl.hash);
  } else {
    await expect.poll(markerSurvived, { timeout: 15000 }).toBe(false);
    await page.waitForLoadState('load');
  }
}

export async function clickAndWaitForHxRedirect(page: Page, locator: ReturnType<Page['locator']>, expectedTarget: string | RegExp): Promise<void> {
  return waitForHxRedirectTarget(page, expectedTarget, async () => {
    await locator.click();
    // Invariant: an hx-confirm element clicked through the plain (non-Confirm)
    // variant opens #confirm-modal instead of ever submitting — act() above
    // then hangs on the htmx-request wait until the 15s timeout produces an
    // opaque failure, often surfacing at a later, unrelated call site instead
    // of here. Name the real cause immediately instead.
    if (await page.locator('#confirm-modal[open]').count() > 0) {
      throw new Error(
        `#confirm-modal opened after clicking ${await describeLocator(locator)} — this element carries hx-confirm; ` +
        `use clickConfirmAndWaitForHxRedirect instead of clickAndWaitForHxRedirect`,
      );
    }
  });
}

async function describeLocator(locator: ReturnType<Page['locator']>): Promise<string> {
  try {
    return await locator.evaluate((el: Element) => el.outerHTML.slice(0, 200));
  } catch {
    return String(locator);
  }
}

// clickConfirmAndWaitForHxRedirect is clickAndWaitForHxRedirect for a target
// that carries hx-confirm — intercepted by static/js/confirm.js's custom
// #confirm-modal (not window.confirm), so the request only fires once
// #confirm-ok is clicked.
export async function clickConfirmAndWaitForHxRedirect(page: Page, locator: ReturnType<Page['locator']>, expectedTarget: string | RegExp): Promise<void> {
  return waitForHxRedirectTarget(page, expectedTarget, async () => {
    await locator.click();
    await page.locator('#confirm-ok').click();
  });
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
  // admin_competitions.go's create handler redirects to /admin/competitions/{id}.
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'), /^\/admin\/competitions\/[^/]+$/);
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
  // admin_pairs.go's create handler redirects to /admin/pairs.
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'), '/admin/pairs');
  const resp = await page.request.get(`/api/collections/pairs/records?filter=name='${name}'`, {
    headers: { Authorization: suToken },
  });
  const data = await resp.json();
  const id = data.items?.[0]?.id;
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
  if (await btn.count() === 0) return;

  // The button carries hx-confirm, intercepted by static/js/confirm.js's
  // custom #confirm-modal — the request only fires once #confirm-ok is
  // clicked (see scheduling-walkover.spec.ts for the same pattern).
  await btn.first().click();
  await Promise.all([
    page.waitForResponse((r) => r.url().includes('/payment-all')),
    page.locator('#confirm-ok').click(),
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
