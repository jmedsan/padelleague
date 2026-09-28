import type { Locator, Page } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import {
  loginAs, asPlayerOn, isMobile, openDrawer, clickAndWaitForHxRedirect, loadTestData, suPost, suGet,
  ADMIN_EMAIL, ADMIN_PASSWORD,
} from '../helpers';

// Pixel baselines for the six key screens, at 360 and 1280 (both projects)
// and in both themes. Every screen shows only this test's own fixture —
// fixed names, fixed 2036 dates, fixed scores — so the pixels never depend
// on the shared seed or on what other specs left behind. Emails carry a
// suffix (users must be unique); nothing that renders does.

const PASSWORD = 'TestPass123456';
const COMP_NAME = 'Liga Capturas';
const THEMES = ['dark', 'padel'] as const;

interface Fixture {
  compId: string;
  disputedId: string;
  playerEmail: string;
}

async function buildFixture(page: Page, suffix: string): Promise<Fixture> {
  const token = loadTestData().adminToken;
  const req = page.request;
  const names = ['Ana Norte', 'Bea Norte', 'Carla Sur', 'Dora Sur'];
  const players = await Promise.all(names.map((name, i) => suPost(req, token, '/api/collections/users/records', {
    email: `screens-${i}-${suffix}@test.local`,
    display_name: name,
    gender: 'female', roles: ['player'],
    password: PASSWORD, passwordConfirm: PASSWORD,
    verified: true,
  })));
  const north = await suPost(req, token, '/api/collections/pairs/records', {
    name: 'Pareja Norte', player1: players[0].id, player2: players[1].id, captain: players[0].id,
  });
  const south = await suPost(req, token, '/api/collections/pairs/records', {
    name: 'Pareja Sur', player1: players[2].id, player2: players[3].id, captain: players[2].id,
  });
  const doc = await suPost(req, token, '/api/collections/documents/records', {
    title: 'Reglamento de la liga', url: 'https://example.com/reglamento',
  });
  const comp = await suPost(req, token, '/api/collections/competitions/records', {
    name: COMP_NAME, type: 'league', active: true, rounds: 2,
    pairs: [north.id, south.id], documents: [doc.id],
    start_date: '2036-03-01 00:00:00.000Z', end_date: '2036-06-30 00:00:00.000Z',
  });
  await suPost(req, token, '/api/collections/matches/records', {
    competition: comp.id, pair1: north.id, pair2: south.id, round_number: 1,
    status: 'final', scores: '6-3 6-4', winner: north.id,
    date: '2036-03-10', club: 'Padel 360',
  });
  // A dispute as the live RequestArbitration(category=result) flow leaves it
  // (same fields as seed/sample.go disputeSampleMatch): Sur submitted a
  // score, Norte objected with a counter-score. Scores are pair1-first.
  const disputed = await suPost(req, token, '/api/collections/matches/records', {
    competition: comp.id, pair1: south.id, pair2: north.id, round_number: 2,
    status: 'disputed', date: '2036-03-17', club: 'Wurko',
    scores: '6-4 4-6 7-5', submitted_by: players[2].id,
    disputed_scores: '6-4 4-6 5-7', disputed_by: players[0].id,
    dispute_notes: 'No estoy de acuerdo: el tercer set fue 5-7, no 7-5.',
    arbitration: 'result', arbitration_by: players[0].id,
  });
  const admin = (await suGet(req, token, `/api/collections/users/records?filter=email='${ADMIN_EMAIL}'`)).items[0];
  await suPost(req, token, '/api/collections/announcements/records', {
    competition: comp.id, title: 'Horarios de pista', body: 'Las pistas abren a las 9:00.', created_by: admin.id,
  });
  return { compId: comp.id, disputedId: disputed.id, playerEmail: `screens-0-${suffix}@test.local` };
}

// Relative times ("hace 2 min"), relative dates and avatars change between
// runs; everything else on these screens is fixed by the fixture.
function masks(page: Page): Locator[] {
  return [page.locator('time'), page.locator('span[title]'), page.locator('.avatar')];
}

// Chromium's capture-beyond-viewport path (fullPage, locator screenshots)
// switches touch emulation off for the page's life, so mobile would be
// captured without the (hover: none) 44px floor. Instead the viewport grows
// to the page height and a plain viewport capture (clipped for an element)
// is taken; the viewport is then restored and touch checked still on.
async function snap(page: Page, name: string, target?: Locator): Promise<void> {
  const base = page.viewportSize();
  if (!base) throw new Error('screens: the project must set a viewport');
  // The navbar is sticky: a page left scrolled (after a click lower down)
  // draws it offset, so every capture starts at the top.
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
  // Grow the viewport until it holds the whole page: the height is re-read
  // after each resize, so content that settles late (or a layout that grows
  // with the viewport) is captured whole and the clip is measured on the
  // final layout.
  await expect.poll(async () => {
    const want = Math.max(await page.evaluate(() => document.documentElement.scrollHeight), base.height);
    if (page.viewportSize()?.height !== want) {
      await page.setViewportSize({ width: base.width, height: want });
      return false;
    }
    return (await page.evaluate(() => window.innerHeight)) === want;
  }, { intervals: [250] }).toBe(true);
  await page.evaluate(() => document.fonts.ready);
  const opts = { maxDiffPixelRatio: 0.01, mask: masks(page) };
  if (target) {
    const box = await target.boundingBox();
    if (!box) throw new Error(`screens: ${name} target is not rendered`);
    // Whole pixels, with the size rounded on its own: floor/ceil of the
    // edges gave 162 or 163px for the same row as its offset's fraction moved.
    const clip = { x: Math.round(box.x), y: Math.round(box.y), width: Math.round(box.width), height: Math.round(box.height) };
    await expect(page).toHaveScreenshot(name, { ...opts, clip });
  } else {
    await expect(page).toHaveScreenshot(name, opts);
  }
  await page.setViewportSize(base);
  await expect.poll(() => page.evaluate(() => window.innerHeight)).toBe(base.height);
  if (isMobile(page)) {
    await expect.poll(() => page.evaluate(() => matchMedia('(hover: none)').matches)).toBe(true);
  }
}

async function setTheme(page: Page, theme: string): Promise<void> {
  const html = page.locator('html');
  if ((await html.getAttribute('data-theme')) !== theme) {
    await page.locator('.navbar').getByRole('button', { name: 'cambiar tema' }).click();
  }
  await expect(html).toHaveAttribute('data-theme', theme);
}

test.describe('screen baselines', { tag: '@screens' }, () => {
  for (const theme of THEMES) {
    test(`player screens and login (${theme})`, async ({ page }, testInfo) => {
      test.slow(); // six full-page captures, each waiting for a stable frame
      const fx = await buildFixture(page, `${theme}-${testInfo.project.name}-${Date.now() % 100000}`);

      await asPlayerOn(page, fx.compId, fx.playerEmail, PASSWORD);
      await setTheme(page, theme);
      await snap(page, `home-${theme}.png`);

      await page.locator(`a[href="/competition/${fx.compId}"]`).locator('visible=true').first().click();
      await expect(page.getByText(COMP_NAME).locator('visible=true').first()).toBeVisible();
      for (const tab of ['Jornadas', 'Clasificación', 'Avisos', 'Documentos']) {
        await page.locator(`input[aria-label^="${tab}"]`).click();
        await snap(page, `competition-${tab.toLowerCase()}-${theme}.png`);
      }

      await page.locator('input[aria-label^="Jornadas"]').click();
      await page.locator(`a[href="/match/${fx.disputedId}"]`).locator('visible=true').first().click();
      await expect(page.locator('#thread-details').getByText('6-4 4-6 5-7')).toBeVisible();
      await snap(page, `match-dispute-${theme}.png`);

      // The theme lives in localStorage, so the login page after logout
      // renders in the theme chosen above.
      if (isMobile(page)) await openDrawer(page);
      await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Salir")').locator('visible=true').first(), '/login');
      await snap(page, `login-${theme}.png`);
    });

    test(`admin competitions (${theme})`, async ({ page }, testInfo) => {
      const fx = await buildFixture(page, `admin-${theme}-${testInfo.project.name}-${Date.now() % 100000}`);
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await setTheme(page, theme);
      if (isMobile(page)) {
        await openDrawer(page);
        await page.locator('.drawer-side a[href="/admin/competitions"]').click();
      } else {
        await page.locator('summary:has-text("Gestión")').click();
        await page.locator('.menu-horizontal a[href="/admin/competitions"]').click();
      }
      await page.waitForURL('**/admin/competitions');
      // The list holds every competition in the shared database, so only
      // this fixture's row is pixel-stable. Its sibling rows are hidden too:
      // otherwise the row's offset (and its sub-pixel text rendering) depends
      // on how many competitions other specs left above it.
      const row = page.locator(`a[href="/admin/competitions/${fx.compId}"]`).locator('visible=true').first()
        .locator('xpath=ancestor-or-self::*[self::tr or contains(concat(" ", @class, " "), " card ")][1]');
      await row.evaluate(el => {
        for (const sib of Array.from(el.parentElement?.children ?? [])) {
          if (sib !== el) (sib as HTMLElement).style.display = 'none';
        }
      });
      await snap(page, `admin-competitions-row-${theme}.png`, row);
    });
  }
});
