import { Page, APIRequestContext, Request, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { runDataDir } from './run-dir';

export const ADMIN_EMAIL = 'admin@test.com';
export const ADMIN_PASSWORD = 'testpass123456';
export const PLAYER1_EMAIL = 'player@test.com';
export const PLAYER1_PASSWORD = 'testpass123456';
export const PLAYER2_EMAIL = 'player2@test.com';
export const PLAYER2_PASSWORD = 'testpass123456';
export const PLAYER3_EMAIL = 'player3@test.com';
export const PLAYER3_PASSWORD = 'testpass123456';

export interface TestData {
  adminToken: string;
  player1: { id: string; email: string };
  player2: { id: string; email: string };
  pair1Id: string;
  pair2Id: string;
  competitionId: string;
  matchIds: string[];
  venueId: string;
}

let _cachedData: TestData | null = null;

export function loadTestData(): TestData {
  if (!_cachedData) {
    const raw = readFileSync(join(runDataDir(8099), 'seed.json'), 'utf-8');
    _cachedData = JSON.parse(raw);
  }
  return _cachedData!;
}

// Auth tokens are cached per email for the lifetime of the worker process.
// The app rate-limits login to 5 requests a minute (R-15), so logging in once
// per test made every test after the fifth sleep 15 seconds waiting out a 429.
// The token is what the browser actually needs; re-authenticating to obtain an
// identical one bought nothing and cost roughly ten minutes a run.
const _tokenCache = new Map<string, string>();

// Tests that change a match's state need one nobody else will touch. The
// desktop and mobile projects share a database, and a test that submits a
// score leaves the match confirmed, so a later test needing a pending match
// finds none. Each mutating test therefore gets its own, keyed by purpose and
// project rather than by project alone.
const SCRATCH_SLOTS: Record<string, number> = {
  'submit-score': 0,
  'propose-schedule': 1,
  'admin-notif': 2,
  'mobile-lifecycle': 3,
  'lifecycle-ui': 4,
  'invalid-nonlast-set': 5,
};

export function scratchMatchId(purpose: keyof typeof SCRATCH_SLOTS | string, projectName: string): string {
  const data = loadTestData();
  const slot = SCRATCH_SLOTS[purpose];
  if (slot === undefined) throw new Error(`unknown scratch purpose: ${purpose}`);
  // seed appends scratch matches after the generated fixtures
  const base = data.matchIds.length - Object.keys(SCRATCH_SLOTS).length * 2;
  const idx = base + slot * 2 + (projectName === 'mobile' ? 1 : 0);
  const id = data.matchIds[idx];
  if (!id) throw new Error(`no scratch match at ${idx}; seed provides ${data.matchIds.length}`);
  return id;
}

export async function loginAs(page: Page, email: string, password: string) {
  const cached = _tokenCache.get(email);
  if (cached) {
    await setAuthCookie(page, cached);
    return;
  }
  for (let attempt = 0; attempt < 5; attempt++) {
    const resp = await page.request.post('/api/collections/users/auth-with-password', {
      data: { identity: email, password },
    });
    if (resp.status() === 429) {
      await new Promise(r => setTimeout(r, 15000));
      continue;
    }
    if (!resp.ok()) {
      throw new Error(`Login failed: ${resp.status()} ${await resp.text()}`);
    }
    const body = await resp.json();
    _tokenCache.set(email, body.token);
    await setAuthCookie(page, body.token);
    return;
  }
  throw new Error('Login failed after 5 attempts (rate limited)');
}

// Tracks whether a page has an in-flight main-frame navigation request —
// e.g. an HX-Redirect's location.href assignment that fired but whose
// document response hasn't landed yet. Installed once per page on first
// use. setAuthCookie's page.goto('/') below must never fire while one of
// these is pending, or Playwright throws the opaque "Navigation to '/' is
// interrupted by another navigation to '/x'" instead of naming the real
// cause: a caller that redirected without waiting for it to land.
const _trackedPages = new WeakSet<Page>();
const _navCounts = new WeakMap<Page, number>();

function trackNavigations(page: Page): void {
  if (_trackedPages.has(page)) return;
  _trackedPages.add(page);
  _navCounts.set(page, 0);
  const isMainFrameNav = (req: Request) => req.isNavigationRequest() && req.frame() === page.mainFrame();
  page.on('request', (req) => {
    if (isMainFrameNav(req)) _navCounts.set(page, (_navCounts.get(page) ?? 0) + 1);
  });
  const settle = (req: Request) => {
    if (isMainFrameNav(req)) _navCounts.set(page, Math.max(0, (_navCounts.get(page) ?? 1) - 1));
  };
  page.on('requestfinished', settle);
  page.on('requestfailed', settle);
}

async function setAuthCookie(page: Page, token: string) {
  trackNavigations(page);
  await page.context().addCookies([{
    name: 'pb_auth',
    value: token,
    domain: 'localhost',
    path: '/',
    httpOnly: true,
  }]);
  // Guard against a caller that just navigated (e.g. clickAndWaitForHxRedirect
  // landing on /match/{id}, whose inline <script> is still running right
  // after domcontentloaded — see tour-helpers.ts). Waiting for the previous
  // page to fully settle before firing this goto avoids "Navigation to '/'
  // is interrupted by another navigation" when the two land within
  // milliseconds of each other.
  await page.waitForLoadState('load');
  // Invariant: if a main-frame navigation is still in flight here, some
  // earlier click in this test redirected the page (HX-Redirect or a plain
  // <a>) without waiting for that navigation to land before calling loginAs.
  // Fail with the real cause instead of letting page.goto('/') below throw
  // Playwright's generic "Navigation to '/' is interrupted by another
  // navigation to '/x'" error.
  const pending = _navCounts.get(page) ?? 0;
  if (pending > 0) {
    throw new Error(
      `loginAs called while ${pending} main-frame navigation(s) still in flight on ${page.url()} — ` +
      `an earlier click redirected without waiting for it to land (wrap it in clickAndWaitForHxRedirect ` +
      `or await page.waitForLoadState('load') before loginAs)`,
    );
  }
  await page.goto('/');
  // Admins land on /admin/competitions (server-side redirect from /); players
  // stay on /. Either confirms the auth cookie took; a login/error page would
  // match neither.
  await expect(page).toHaveURL(/\/(admin\/competitions)?$/, { timeout: 5000 });
}

// LEAGUE_TIMEZONE must track league/scheduling.go's defaultTimezone — the
// value app_settings.league_timezone is seeded to by
// migrations/1734700000_league_timezone.go and nothing in seed/ or the e2e
// setup ever overrides. It is not a second independent guess at the
// timezone; it is the same default, copied here because a .ts test can't
// import a Go constant.
const LEAGUE_TIMEZONE = 'Atlantic/Canary';

// leagueDate returns the calendar date (YYYY-MM-DD), in the league's display
// timezone, offsetDays from today. Use this instead of
// `new Date(Date.now() + N * 86400000).toISOString().slice(0, 10)`: that
// idiom takes the offset in UTC milliseconds, then reads the date back out in
// UTC — silently wrong for a league whose timezone is ahead of UTC (Atlantic/
// Canary is UTC+1 in summer/WEST) during the hour(s) after local midnight,
// when the UTC calendar day hasn't rolled over yet. "Tomorrow" computed in
// UTC during that window is still "today" in the league, so a value meant to
// be tomorrow's date is actually today's — every test asserting a
// date-relative label (e.g. "mañana") against it fails, deterministically,
// every night in that window.
//
// Compute in calendar days on the LOCAL date, not by adding milliseconds:
// adding 24h in UTC always lands on the right UTC-calendar-day but the WRONG
// league-calendar-day near a DST transition or near local midnight.
export function leagueDate(offsetDays: number): string {
  const fmt = new Intl.DateTimeFormat('en-CA', { timeZone: LEAGUE_TIMEZONE, year: 'numeric', month: '2-digit', day: '2-digit' });
  const parts = fmt.formatToParts(new Date());
  const y = Number(parts.find(p => p.type === 'year')!.value);
  const m = Number(parts.find(p => p.type === 'month')!.value);
  const d = Number(parts.find(p => p.type === 'day')!.value);
  // Construct at UTC noon on the league's current calendar day, then add
  // whole days — noon avoids any DST-edge wraparound when offsetDays crosses
  // a transition, and the result is read back out as a calendar date only,
  // never as an instant.
  const base = new Date(Date.UTC(y, m - 1, d, 12, 0, 0));
  base.setUTCDate(base.getUTCDate() + offsetDays);
  return `${base.getUTCFullYear()}-${String(base.getUTCMonth() + 1).padStart(2, '0')}-${String(base.getUTCDate()).padStart(2, '0')}`;
}

// ---------------------------------------------------------------------------
// HTMX redirect helper (shared by action helpers)
// ---------------------------------------------------------------------------

// A grep for "Publicar calendario"-style click sites only finds the one
// sample reported, not the class: ANY click whose response carries
// HX-Redirect but isn't awaited via waitForHxRedirectTarget races the same
// way (tour-guided.spec.ts's playoff-publish button raced a later
// clickConfirmAndWaitForHxRedirect call this way — 100% reproducible, see
// git history). expectingHxRedirect marks a page as "a helper is currently
// awaiting a redirect on it" from just before act() fires until the redirect
// lands (or the wait throws); overflow-guard.ts's pageGuards fixture checks
// this flag on every response carrying an hx-redirect header and fails the
// test if it's not set — an unhelpered site fails the very next time its
// button is clicked in any test, not just when someone happens to grep for
// its label.
export const expectingHxRedirect = new WeakMap<Page, boolean>();

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
// Exported directly (not just via the click wrappers below) for action sites
// that trigger the htmx request some way other than a plain click — a file
// input's change event, a select's change event, etc. — where the caller
// supplies its own act().
export async function waitForHxRedirect(page: Page, expectedTarget: string | RegExp, act: () => Promise<void>): Promise<void> {
  return waitForHxRedirectTarget(page, expectedTarget, act);
}

async function waitForHxRedirectTarget(page: Page, expectedTarget: string | RegExp, act: () => Promise<void>): Promise<void> {
  expectingHxRedirect.set(page, true);
  try {
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
  } finally {
    expectingHxRedirect.set(page, false);
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

export function isMobile(page: Page): boolean {
  const vp = page.viewportSize();
  return !!vp && vp.width < 1024;
}

// clickAction clicks a per-row action that the desktop table renders icon-only
// (aria-label, no text) and the mobile card list hides inside a "Más acciones"
// dropdown. target is a selector matching the action in both markups (e.g.
// `label[for="penalty-modal-abc"]`), label the desktop aria-label prefix.
export async function clickAction(page: Page, target: string, label: string): Promise<void> {
  if (isMobile(page)) {
    const dropdown = page.locator(`.dropdown:has(${target})`).first();
    await dropdown.locator('button[aria-label^="Más acciones"]').click();
    await dropdown.locator(target).click();
    return;
  }
  await page.locator(`${target}[aria-label^="${label}"]:visible`).first().click();
}

export async function openDrawer(page: Page): Promise<void> {
  const toggle = page.locator('label[for="main-drawer"]').first();
  if (!await toggle.isVisible().catch(() => false)) {
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
  }
  await toggle.click();
  await page.locator('.drawer-side .menu').waitFor({ state: 'visible', timeout: 3000 });
}

export async function navViaDrawer(page: Page, href: string): Promise<void> {
  await openDrawer(page);
  await page.locator(`.drawer-side a[href="${href}"]`).click();
  await page.waitForLoadState('domcontentloaded');
}

export async function loginViaForm(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.fill('#email', email);
  await page.fill('#password', password);
  // Admins land on /admin/competitions (server-side redirect from /); players
  // stay on /.
  await clickAndWaitForHxRedirect(page, page.getByRole('button', { name: 'Entrar' }), /^\/(admin\/competitions)?$/);
}

// ---------------------------------------------------------------------------
// Authenticated API helpers
// ---------------------------------------------------------------------------
//
// Specs used to hand-roll their own suPost/suPatch/apiCreateRecord/etc., each
// repeating the same `if (!resp.ok()) throw` body — and a few (suGet in
// thread.spec.ts and match-lifecycle.spec.ts, apiDeleteRecord everywhere)
// dropped the check entirely, so a failed lookup or cleanup delete passed
// silently instead of failing the test that depended on it. One core here,
// named wrappers for the common verbs, so every spec gets the check for
// free instead of re-deriving it.

async function apiRequest(
  request: APIRequestContext,
  method: 'get' | 'post' | 'patch' | 'delete',
  path: string,
  token: string,
  data?: Record<string, unknown>,
): Promise<any> {
  const resp = await request[method](path, {
    headers: { Authorization: token, 'Content-Type': 'application/json' },
    ...(data !== undefined ? { data } : {}),
  });
  if (!resp.ok()) {
    throw new Error(`${method.toUpperCase()} ${path}: ${resp.status()} ${await resp.text()}`);
  }
  return resp;
}

export async function suGet(request: APIRequestContext, token: string, path: string): Promise<any> {
  const resp = await apiRequest(request, 'get', path, token);
  return resp.json();
}

export async function suPost(request: APIRequestContext, token: string, path: string, data: Record<string, unknown>): Promise<any> {
  const resp = await apiRequest(request, 'post', path, token, data);
  return resp.json();
}

export async function suPatch(request: APIRequestContext, token: string, path: string, data: Record<string, unknown>): Promise<void> {
  await apiRequest(request, 'patch', path, token, data);
}

export async function suDelete(request: APIRequestContext, token: string, path: string): Promise<void> {
  await apiRequest(request, 'delete', path, token);
}

export async function apiCreateRecord(request: APIRequestContext, token: string, collection: string, data: Record<string, unknown>): Promise<string> {
  const body = await suPost(request, token, `/api/collections/${collection}/records`, data);
  return body.id;
}

export async function apiGetRecord(request: APIRequestContext, token: string, collection: string, id: string): Promise<any> {
  return suGet(request, token, `/api/collections/${collection}/records/${id}`);
}

export async function apiListRecords(request: APIRequestContext, token: string, collection: string, filter: string): Promise<any[]> {
  const body = await suGet(request, token, `/api/collections/${collection}/records?filter=${encodeURIComponent(filter)}&perPage=50`);
  return body.items || [];
}

export async function apiDeleteRecord(request: APIRequestContext, token: string, collection: string, id: string): Promise<void> {
  await suDelete(request, token, `/api/collections/${collection}/records/${id}`);
}
