import { readFileSync } from 'fs';
import { join } from 'path';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

// route-census.json is the single source of truth shared with
// routes/route_census_test.go (Go side asserts it matches the router's
// registered GET routes exactly). This spec is the consumer: visit every
// non-skipped entry as both an admin and a player, at whatever viewport the
// running project uses (desktop 1280x720, mobile 360x780 — see
// playwright.config.ts), and assert 200 + pageGuards (imported test already
// wires that up for every test automatically).
interface CensusRoute {
  path: string;
  auth: 'none' | 'admin' | 'player' | 'both';
  param?: string;
  skipCensus?: string;
}

function loadCensus(): CensusRoute[] {
  const raw = readFileSync(join(__dirname, '..', 'route-census.json'), 'utf-8');
  return (JSON.parse(raw).routes as CensusRoute[]).filter(r => !r.skipCensus);
}

let suToken = '';

async function getSuperuserToken(request: APIRequestContext): Promise<void> {
  if (suToken) return;
  const resp = await request.post('/api/collections/_superusers/auth-with-password', {
    data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
  });
  if (!resp.ok()) throw new Error(`Superuser auth failed: ${resp.status()} ${await resp.text()}`);
  suToken = (await resp.json()).token;
}

// ensureDocumentId returns an existing documents record id, creating one
// (a link-type reference item — no file upload needed) if the seeded DB has
// none yet.
async function ensureDocumentId(request: APIRequestContext): Promise<string> {
  await getSuperuserToken(request);
  const list = await request.get('/api/collections/documents/records?perPage=1', {
    headers: { Authorization: suToken },
  });
  const items = (await list.json()).items ?? [];
  if (items.length > 0) return items[0].id;

  const created = await request.post('/api/collections/documents/records', {
    headers: { Authorization: suToken, 'Content-Type': 'application/json' },
    data: { title: 'Reglamento (route-census)', url: 'https://example.com/reglamento.pdf' },
  });
  if (!created.ok()) throw new Error(`Create document failed: ${created.status()} ${await created.text()}`);
  return (await created.json()).id;
}

// ensureSponsorId returns an existing sponsors record id, creating one via
// multipart/form-data (the `logo` FileField is required — a plain JSON POST
// can't satisfy that) if the seeded DB has none yet.
async function ensureSponsorId(request: APIRequestContext): Promise<string> {
  await getSuperuserToken(request);
  const list = await request.get('/api/collections/sponsors/records?perPage=1', {
    headers: { Authorization: suToken },
  });
  const items = (await list.json()).items ?? [];
  if (items.length > 0) return items[0].id;

  const created = await request.post('/api/collections/sponsors/records', {
    headers: { Authorization: suToken },
    multipart: {
      name: 'Route Census Sponsor',
      logo: {
        name: 'logo.png',
        mimeType: 'image/png',
        // 1x1 transparent PNG — smallest valid file for the FileField.
        buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=', 'base64'),
      },
    },
  });
  if (!created.ok()) throw new Error(`Create sponsor failed: ${created.status()} ${await created.text()}`);
  return (await created.json()).id;
}

// resolveParam maps a census entry's declared `param` name to an actual
// value from the seeded test data (or a freshly-created record for the two
// entity types global-setup doesn't seed).
async function resolveParam(request: APIRequestContext, name: string): Promise<string> {
  const data = loadTestData();
  switch (name) {
    case 'competitionId': return data.competitionId;
    case 'matchId': return data.matchIds[0];
    case 'pair1Id': return data.pair1Id;
    case 'player1Id': return data.player1.id;
    case 'venueId': return data.venueId;
    case 'documentId': return ensureDocumentId(request);
    case 'sponsorId': return ensureSponsorId(request);
    default: throw new Error(`route-census.json: unknown param "${name}" — add a case in resolveParam`);
  }
}

async function resolvePath(request: APIRequestContext, route: CensusRoute): Promise<string> {
  // PocketBase's {$} means "this exact path, nothing after" (net/http's
  // ServeMux root-only pattern) — it isn't a real URL segment to substitute,
  // routes.go registers it only for "/".
  if (route.path === '/{$}') return '/';
  if (!route.param) return route.path;
  const value = await resolveParam(request, route.param);
  // Every parametrized census path has exactly one {name} or {name...}
  // placeholder (routes.go never registers two dynamic segments on one GET
  // page route) — replace it regardless of its exact placeholder name.
  return route.path.replace(/\{[^}]+\}/, value);
}

async function visitAndAssert(page: Page, path: string): Promise<void> {
  const response = await page.goto(path);
  expect(response?.status(), `GET ${path} should return 200`).toBe(200);
  await page.waitForLoadState('domcontentloaded');
}

const census = loadCensus();

test.describe('route census', () => {
  for (const route of census) {
    if (route.auth === 'none') {
      test(`anonymous: ${route.path}`, async ({ page }) => {
        const path = await resolvePath(page.request, route);
        await visitAndAssert(page, path);
      });
      continue;
    }
    if (route.auth === 'admin' || route.auth === 'both') {
      test(`admin: ${route.path}`, async ({ page }) => {
        const path = await resolvePath(page.request, route);
        await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
        await visitAndAssert(page, path);
      });
    }
    if (route.auth === 'player' || route.auth === 'both') {
      test(`player: ${route.path}`, async ({ page }) => {
        const path = await resolvePath(page.request, route);
        await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
        await visitAndAssert(page, path);
      });
    }
  }
});
