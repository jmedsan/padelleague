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
  await page.getByRole('button', { name: 'Entrar' }).click();
  // Admins land on /admin/competitions (server-side redirect from /); players
  // stay on /.
  await page.waitForURL(/\/(admin\/competitions)?$/, { timeout: 10000 });
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
