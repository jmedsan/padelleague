import { Page, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';

export const ADMIN_EMAIL = 'admin@test.com';
export const ADMIN_PASSWORD = 'testpass123456';
export const PLAYER1_EMAIL = 'player@test.com';
export const PLAYER1_PASSWORD = 'testpass123456';
export const PLAYER2_EMAIL = 'player2@test.com';
export const PLAYER2_PASSWORD = 'testpass123456';

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
    const raw = readFileSync(join(__dirname, '.test-data/seed.json'), 'utf-8');
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
};

export function scratchMatchId(purpose: keyof typeof SCRATCH_SLOTS | string, projectName: string): string {
  const data = loadTestData();
  const slot = SCRATCH_SLOTS[purpose];
  if (slot === undefined) throw new Error(`unknown scratch purpose: ${purpose}`);
  // seed appends scratch matches after the generated fixtures
  const base = data.matchIds.length - 4;
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

async function setAuthCookie(page: Page, token: string) {
  await page.context().addCookies([{
    name: 'pb_auth',
    value: token,
    domain: 'localhost',
    path: '/',
    httpOnly: true,
  }]);
  await page.goto('/');
  await expect(page).toHaveURL('/', { timeout: 5000 });
}

export async function loginViaForm(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.fill('#email', email);
  await page.fill('#password', password);
  await page.getByRole('button', { name: 'Entrar' }).click();
  await page.waitForURL('/', { timeout: 10000 });
}
