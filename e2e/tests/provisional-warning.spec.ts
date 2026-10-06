import { test, expect } from '../overflow-guard';
import type { APIRequestContext, Page } from '@playwright/test';
import {
  asPlayerOn, isMobile, navViaDrawer, loadTestData, openMatchFromHome, loginAs, ADMIN_EMAIL, ADMIN_PASSWORD,
  apiCreateRecord, apiDeleteRecord,
} from '../helpers';

const PASSWORD = 'testpass123456';
const WARNING = '[data-testid="provisional-warning"]';

interface Fixture {
  suffix: string;
  email: string;
  userId: string;
  rivalId: string;
  pairAId: string;
  pairAName: string;
  compId: string;
  otherCompId: string;
  proposedMatchId: string;
  nextMatchId: string;
  cleanup: () => Promise<void>;
}

// seed builds a competition where pair A proposed 6-3 6-4 against pair B and
// B has not answered, plus a second, still-unplayed match between the same
// pairs (its Precedentes strip must count the proposal) and a second empty
// competition so home renders the "Mis competiciones" grid.
async function seed(request: APIRequestContext, project: string, pairAName?: string): Promise<Fixture> {
  const token = loadTestData().adminToken;
  // Short: a long unbreakable competition title overflows the 360px page header.
  const suffix = `${Date.now() % 1_000_000}${project[0]}`;
  const made: Array<[string, string]> = [];
  const create = async (collection: string, data: Record<string, unknown>) => {
    const id = await apiCreateRecord(request, token, collection, data);
    made.unshift([collection, id]);
    return id;
  };
  const user = (tag: string) => create('users', {
    email: `pw-${tag}-${suffix}@test.local`, password: PASSWORD, passwordConfirm: PASSWORD,
    display_name: `Pw ${tag} ${suffix}`, roles: ['player'], verified: true, gender: 'male',
  });
  const a1 = await user('a1');
  const a2 = await user('a2');
  const b1 = await user('b1');
  const b2 = await user('b2');
  const nameA = pairAName ?? `Pw Pareja A ${suffix}`;
  const pairAId = await create('pairs', { name: nameA, player1: a1, player2: a2 });
  const pairBId = await create('pairs', { name: `Pw Pareja B ${suffix}`, player1: b1, player2: b2 });
  const compId = await create('competitions', {
    name: `Pw Comp ${suffix}`, type: 'league', active: true, pairs: [pairAId, pairBId],
  });
  const otherCompId = await create('competitions', {
    name: `Pw Otra ${suffix}`, type: 'league', active: true, pairs: [pairAId, pairBId],
  });
  const proposedMatchId = await create('matches', {
    competition: compId, pair1: pairAId, pair2: pairBId, status: 'scheduled', round_number: 1,
    date: '2026-01-10 10:00:00.000Z',
  });
  const nextMatchId = await create('matches', {
    competition: compId, pair1: pairBId, pair2: pairAId, status: 'scheduled', round_number: 2,
  });
  await create('match_messages', {
    match: proposedMatchId, author: a1, type: 'result_submission', proposal_status: 'pending',
    content: '6-3 6-4', proposal_data: JSON.stringify({ scores: '6-3 6-4' }),
  });
  return {
    suffix, email: `pw-a1-${suffix}@test.local`, userId: a1, rivalId: b1, pairAId, pairAName: nameA,
    compId, otherCompId, proposedMatchId, nextMatchId,
    cleanup: async () => {
      for (const [collection, id] of made) await apiDeleteRecord(request, token, collection, id);
    },
  };
}

async function playerHome(page: Page, f: Fixture): Promise<void> {
  await asPlayerOn(page, f.compId, f.email, PASSWORD);
  await page.waitForLoadState('networkidle');
}

async function openProfile(page: Page, f: Fixture): Promise<void> {
  if (isMobile(page)) {
    await navViaDrawer(page, `/player/${f.userId}`);
  } else {
    await page.locator(`.navbar a[href="/player/${f.userId}"]`).first().click();
  }
  await page.waitForURL(`**/player/${f.userId}`);
  await page.waitForLoadState('networkidle');
}

async function openStandings(page: Page, f: Fixture): Promise<void> {
  await page.locator(`a[href^="/competition/${f.compId}"]`).first().click();
  await page.waitForURL(`**/competition/${f.compId}**`);
  await page.locator('input[aria-label^="Clasificación"]').click();
}

// openCalendarRow reaches a match row the way a player does: the competition
// card on home, its Partidos tab, the round accordion.
async function openCalendarRow(page: Page, f: Fixture, matchId: string) {
  await page.locator(`a[href^="/competition/${f.compId}"]`).first().click();
  await page.waitForURL(`**/competition/${f.compId}**`);
  await page.locator('input[aria-label^="Jornadas"], input[aria-label^="Partidos"]').click();
  const row = page.locator(`a[href="/match/${matchId}"]`).first();
  const round = page.locator('.collapse', { has: row }).locator('> input');
  if (await round.count()) await round.check();
  return row;
}

test.describe('provisional results are counted and warned about everywhere', { tag: '@provisional' }, () => {
  test('home: the recent-results row shows the proposed score with the Propuesta badge, no icon', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await playerHome(page, f);
      const row = page.locator(`h2:has-text("Mis últimos partidos") + div a[href="/match/${f.proposedMatchId}"]`);
      await expect(row.locator('.badge', { hasText: 'Propuesta' }), 'the unconfirmed row says Propuesta').toBeVisible();
      await expect(row, 'the proposed score is shown').toContainText('6-3 6-4');
      await expect(row.locator(WARNING), 'a single match uses the badge, not the aggregate icon').toHaveCount(0);
    } finally {
      await f.cleanup();
    }
  });

  test('calendar: the proposed match row shows its score with the Propuesta badge, no icon', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await playerHome(page, f);
      const row = await openCalendarRow(page, f, f.proposedMatchId);
      await expect(row.locator('.badge', { hasText: 'Propuesta' })).toBeVisible();
      await expect(row, 'the proposed score is shown in the row').toContainText('6-3 6-4');
      await expect(row.locator(WARNING), 'a single match uses the badge, not the aggregate icon').toHaveCount(0);
      const open = await openCalendarRow(page, f, f.nextMatchId);
      await expect(open, 'a match with no proposal shows no score').not.toContainText(/\d-\d/);
    } finally {
      await f.cleanup();
    }
  });

  test('calendar: a round holding only a proposed match is not the one opened by default', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await playerHome(page, f);
      await page.locator(`a[href^="/competition/${f.compId}"]`).first().click();
      await page.waitForURL(`**/competition/${f.compId}**`);
      const roundOf = (id: string) => page.locator('.collapse', { has: page.locator(`a[href="/match/${id}"]`) }).locator('> input');
      await expect(roundOf(f.nextMatchId), 'the first round with an unplayed match opens').toBeChecked();
      await expect(roundOf(f.proposedMatchId), 'a proposed result counts as played').not.toBeChecked();
    } finally {
      await f.cleanup();
    }
  });

  test('home: a deadlock of disagreeing proposals counts nowhere and does not warn', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await apiCreateRecord(page.request, loadTestData().adminToken, 'match_messages', {
        match: f.proposedMatchId, author: f.rivalId, type: 'result_submission', proposal_status: 'pending',
        content: '3-6 4-6', proposal_data: JSON.stringify({ scores: '3-6 4-6' }),
      });
      await playerHome(page, f);
      await expect(
        page.locator(`h2:has-text("Mis últimos partidos") + div a[href="/match/${f.proposedMatchId}"]`),
        'a conflicting result must not appear in recent results',
      ).toHaveCount(0);
      const card = page.locator('[data-testid="player-competitions-heading"] + div > .card', { has: page.locator(`a[href="/competition/${f.compId}"]`) });
      await expect(card.locator('.badge', { hasText: 'pts' })).toBeVisible();
      await expect(card.locator(WARNING), 'a conflicting result must not warn').toHaveCount(0);
    } finally {
      await f.cleanup();
    }
  });

  test('home: the competition card counts the proposed match as played, not por jugar', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await playerHome(page, f);
      const card = page.locator('[data-testid="player-competitions-heading"] + div > .card', { has: page.locator(`a[href="/competition/${f.compId}"]`) });
      await expect(card, 'only the unproposed match is left to play').toContainText('1 partido por jugar');
    } finally {
      await f.cleanup();
    }
  });

  test('home: the competition card standing badge carries the warning', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await playerHome(page, f);
      const card = page.locator('[data-testid="player-competitions-heading"] + div > .card', { has: page.locator(`a[href="/competition/${f.compId}"]`) });
      await expect(card.locator('.badge', { hasText: 'pts' }).locator(WARNING)).toBeVisible();
      const other = page.locator('[data-testid="player-competitions-heading"] + div > .card', { has: page.locator(`a[href="/competition/${f.otherCompId}"]`) });
      await expect(other.locator(WARNING), 'a competition without unconfirmed results must not warn').toHaveCount(0);
    } finally {
      await f.cleanup();
    }
  });

  test('admin: the competition card counts the proposed match as played, with the warning', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      const card = page.locator(`a[href="/admin/competitions/${f.compId}"]`).first();
      await expect(card).toContainText('1/2 partidos');
      await expect(card.locator(WARNING)).toBeVisible();
      const other = page.locator(`a[href="/admin/competitions/${f.otherCompId}"]`).first();
      await expect(other.locator(WARNING)).toHaveCount(0);
    } finally {
      await f.cleanup();
    }
  });

  test('player profile: the Partidos tile and the competition row carry the warning', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await playerHome(page, f);
      await openProfile(page, f);
      const tile = page.locator('.stat', { hasText: 'Partidos' }).first();
      await expect(tile.locator(WARNING)).toBeVisible();
      await expect(tile.locator('.stat-value')).toHaveText('1');
      const row = page.locator('tr', { hasText: `Pw Comp ${f.suffix}` }).first();
      await expect(row.locator(WARNING)).toBeVisible();
    } finally {
      await f.cleanup();
    }
  });

  test('pair page: reached from the standings, the Partidos tile warns', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await playerHome(page, f);
      await openStandings(page, f);
      // Click the start of the link: a truncated name leaves its center under the next cell.
      await page.locator(`a[href="/pair/${f.pairAId}"]:visible`).first().click({ position: { x: 4, y: 4 } });
      await page.waitForURL(`**/pair/${f.pairAId}`);
      const tile = page.locator('.stat', { hasText: 'Partidos' }).first();
      await expect(tile.locator(WARNING)).toBeVisible();
    } finally {
      await f.cleanup();
    }
  });

  test('match page: the Precedentes strip counts the proposal and warns', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await playerHome(page, f);
      await openMatchFromHome(page, f.nextMatchId);
      const strip = page.locator('.card', { hasText: 'Precedentes' }).last();
      await expect(strip).toContainText('0 · 1');
      await expect(strip.locator(WARNING)).toBeVisible();
    } finally {
      await f.cleanup();
    }
  });

  test('admin: the round badge counts the proposed match and warns', async ({ page }, testInfo) => {
    const f = await seed(page.request, testInfo.project.name);
    try {
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await page.locator(`a[href="/admin/competitions/${f.compId}"]`).first().click();
      await page.waitForURL(`**/admin/competitions/${f.compId}`);
      const badge = page.locator('.badge', { hasText: 'partido' }).filter({ has: page.locator(WARNING) }).first();
      await expect(badge).toContainText('1/1 partido');
    } finally {
      await f.cleanup();
    }
  });

  test('standings: the warning stays visible beside a long, truncated pair name', async ({ page }, testInfo) => {
    const longName = 'Pareja con un nombre larguísimo para forzar el truncado';
    const f = await seed(page.request, testInfo.project.name, longName);
    try {
      await playerHome(page, f);
      await openStandings(page, f);
      const table = page.locator(isMobile(page) ? 'table.table-sm' : 'table.table-zebra');
      const warning = table.locator('tbody tr', { hasText: longName.slice(0, 20) }).locator(WARNING).first();
      await expect(warning).toBeVisible();
      const box = (await warning.boundingBox())!;
      const viewport = page.viewportSize()!;
      expect(box.x + box.width, 'the icon must sit inside the viewport, not clipped away').toBeLessThanOrEqual(viewport.width);
      const cellBox = (await warning.locator('xpath=ancestor::td[1]').boundingBox())!;
      expect(box.x, 'the icon must not start left of its cell').toBeGreaterThanOrEqual(cellBox.x);
      expect(box.x + box.width, 'the icon must not be clipped by its cell').toBeLessThanOrEqual(cellBox.x + cellBox.width);
      expect(cellBox.width, 'the Pareja column must stay wide enough to read a name').toBeGreaterThanOrEqual(140);
      const nameLink = table.locator('tbody tr', { hasText: longName.slice(0, 20) }).locator('td').nth(1).locator('a').first();
      const visibleStart = await nameLink.evaluate((el) => {
        const r = el.getBoundingClientRect();
        const cell = el.closest('td')!.getBoundingClientRect();
        return Math.min(r.width, cell.right - r.left);
      });
      expect(visibleStart, 'the visible part of the name must show more than a few letters').toBeGreaterThanOrEqual(80);
      const hit = await warning.evaluate((el) => {
        const r = el.getBoundingClientRect();
        const top = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
        return top !== null && el.contains(top);
      });
      expect(hit, 'the icon must be the element painted at its own center').toBe(true);
    } finally {
      await f.cleanup();
    }
  });
});
