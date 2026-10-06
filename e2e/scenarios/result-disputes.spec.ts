import { test, expect, type Page } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  PLAYER_PASSWORD, ScenarioApi, ScenarioData, apiGet, apiPatch, apiPost, loadCtx, printLogin, printLinks,
} from '../scenario-helpers';

const WARNING = '[data-testid="provisional-warning"]';

let ctx: ScenarioData;
let api: ScenarioApi;

interface Case {
  matchId: string;
  rivalPairId: string;
  rivalPlayerEmail: string;
  extraMatchId: string; // a later, unplayed match against the same rival: its Precedentes strip reads this pair's history
}
interface Seeded { control: Case; disputed: Case }

const tokens = new Map<string, string>();

// playerToken logs in once per player: PocketBase rate limits its auth endpoint,
// so every later request reuses the token.
async function playerToken(email: string): Promise<string> {
  const cached = tokens.get(email);
  if (cached) return cached;
  const resp = await fetch(`${ctx.baseURL}/api/collections/users/auth-with-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identity: email, password: PLAYER_PASSWORD }),
  });
  if (!resp.ok) throw new Error(`login ${email}: ${resp.status} ${await resp.text()}`);
  const token: string = (await resp.json()).token;
  tokens.set(email, token);
  return token;
}

// formAs posts a live-handler form as the given player, like the browser does.
async function formAs(email: string, path: string, fields: Record<string, string>): Promise<void> {
  const resp = await fetch(`${ctx.baseURL}${path}`, {
    method: 'POST',
    headers: {
      Cookie: `pb_auth=${await playerToken(email)}`,
      'HX-Request': 'true',
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: new URLSearchParams(fields).toString(),
    redirect: 'manual',
  });
  if (resp.status >= 400) throw new Error(`${path}: ${resp.status} ${await resp.text()}`);
}

const submit = (email: string, matchId: string, scores: string) =>
  formAs(email, `/match/${matchId}/submit`, { scores });

const mine = () => ctx.pairs[0];
const mineEmail = () => ctx.players[mine().player1Idx].email;

// seeded rebuilds the two cases from the data the 00 step left behind: the
// pair's first two matches, in id order, are control / disputed.
async function seeded(): Promise<Seeded> {
  const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
    `competition='${ctx.competitionId}' && round_number<90 && (pair1='${mine().id}' || pair2='${mine().id}')`)}&sort=id&perPage=2`);
  const extras = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
    `competition='${ctx.competitionId}' && round_number>=90 && (pair1='${mine().id}' || pair2='${mine().id}')`)}&perPage=10`);
  const build = (m: any): Case => {
    const rivalPairId = m.pair1 === mine().id ? m.pair2 : m.pair1;
    const rival = ctx.pairs.find(p => p.id === rivalPairId)!;
    const extra = extras.items.find((x: any) => x.pair1 === rivalPairId || x.pair2 === rivalPairId);
    return {
      matchId: m.id, rivalPairId, rivalPlayerEmail: ctx.players[rival.player1Idx].email, extraMatchId: extra.id,
    };
  };
  const [control, disputed] = list.items.map(build);
  return { control, disputed };
}

async function asMine(page: Page): Promise<void> {
  await loginAs(page, mineEmail(), PLAYER_PASSWORD);
  await page.goto('/');
  await page.waitForLoadState('domcontentloaded');
}

async function openStandings(page: Page): Promise<void> {
  await page.locator(`a[href^="/competition/${ctx.competitionId}"]`).first().click();
  await page.waitForURL(`**/competition/${ctx.competitionId}**`);
  await page.locator('input[aria-label^="Clasificación"]').click();
}

// openMatch reaches a match the way a player does: home, the competition,
// its Partidos tab, the round, the match link.
async function openMatch(page: Page, matchId: string): Promise<void> {
  await page.locator('.navbar a[href="/"]').first().click();
  await page.waitForURL(/\/$/);
  await page.locator(`a[href^="/competition/${ctx.competitionId}"]`).first().click();
  await page.waitForURL(`**/competition/${ctx.competitionId}**`);
  await page.locator('input[aria-label^="Jornadas"], input[aria-label^="Partidos"]').click();
  const link = page.locator(`a[href="/match/${matchId}"]`).first();
  const round = page.locator('.collapse', { has: link }).locator('> input');
  if (await round.count()) await round.check();
  await link.click();
  await page.waitForURL(`**/match/${matchId}`);
}

test.describe('a disputed result is not counted', () => {
  test.describe.configure({ mode: 'serial', retries: 0 });

  test.beforeAll(() => {
    ctx = loadCtx();
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
  });

  test('00 ready: one match with a lone proposal, one disputed', async () => {
    const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
      `competition='${ctx.competitionId}' && status='pending' && (pair1='${mine().id}' || pair2='${mine().id}')`)}&sort=id&perPage=2`);
    if (list.items.length < 2) throw new Error(`pair needs 2 open matches, has ${list.items.length}`);
    for (const m of list.items) {
      await apiPatch(api, `/api/collections/matches/records/${m.id}`, { date: '2025-03-15', club: 'Padel 360' });
      // A later match against the same rival, so the Precedentes strip has a page to live on.
      await apiPost(api, '/api/collections/matches/records', {
        competition: ctx.competitionId, pair1: m.pair1, pair2: m.pair2, status: 'pending', round_number: 90,
      });
    }
    const { control, disputed } = await seeded();

    // Control: a lone pending proposal.
    await submit(mineEmail(), control.matchId, '6-3 6-4');

    // Disputed: proposal, then the rival asks for arbitration over the result.
    await submit(mineEmail(), disputed.matchId, '6-3 6-4');
    await formAs(disputed.rivalPlayerEmail, `/match/${disputed.matchId}/arbitration`, {
      category: 'result', notes: 'No jugamos así',
    });

    printLogin(ctx.baseURL, mineEmail(), `/match/${disputed.matchId}`);
    const comp = `/competition/${ctx.competitionId}`;
    printLinks(ctx.baseURL, [
      { label: 'Calendar / Jornadas (the lone proposal row shows 6-3 6-4 + Propuesta, the disputed row no score)', email: mineEmail(), password: PLAYER_PASSWORD, path: comp },
      { label: 'Match with the lone proposal', email: mineEmail(), password: PLAYER_PASSWORD, path: `/match/${control.matchId}` },
      { label: 'Disputed match (no score)', email: mineEmail(), password: PLAYER_PASSWORD, path: `/match/${disputed.matchId}` },
      { label: 'Admin rounds page', email: ADMIN_EMAIL, password: ADMIN_PASSWORD, path: `/admin/competitions/${ctx.competitionId}` },
    ]);
    console.log('Two matches of this pair: (1) a lone proposal 6-3 6-4 — counts, with the warning; (2) disputed — counts nowhere. Check Clasificación, your profile, the "Precedentes" strip on the later match against each rival, and the admin counters.');
  });

  test('01 standings: only the lone proposal counts, and only it warns', async ({ page }) => {
    const { control, disputed } = await seeded();
    await asMine(page);
    await openStandings(page);
    const row = (pairId: string) => page.locator('table.table-zebra tbody tr', { has: page.locator(`a[href="/pair/${pairId}"]`) });
    const played = (pairId: string) => row(pairId).locator('td').nth(2);

    await expect(played(mine().id)).toHaveText('1');
    await expect(row(mine().id).locator(WARNING)).toHaveCount(1);
    await expect(played(control.rivalPairId)).toHaveText('1');
    await expect(row(control.rivalPairId).locator(WARNING)).toHaveCount(1);
    await expect(played(disputed.rivalPairId)).toHaveText('0');
    await expect(row(disputed.rivalPairId).locator(WARNING)).toHaveCount(0);
  });

  test('02 stats tiles: the player profile counts one match, with the warning', async ({ page }) => {
    await asMine(page);
    await page.locator(`.navbar a[href="/player/${ctx.players[mine().player1Idx].id}"]`).first().click();
    await page.waitForLoadState('domcontentloaded');
    const tile = page.locator('.stat', { hasText: 'Partidos' }).first();
    await expect(tile.locator('.stat-value')).toHaveText('1');
    await expect(tile.locator(WARNING)).toBeVisible();
  });

  test('03 Precedentes: only the lone proposal is a precedent', async ({ page }) => {
    const { control, disputed } = await seeded();
    await asMine(page);
    await openMatch(page, control.extraMatchId);
    const strip = page.locator('.card', { hasText: 'Precedentes' }).last();
    await expect(strip).toContainText('1 · 0');
    await expect(strip.locator(WARNING)).toBeVisible();

    await openMatch(page, disputed.extraMatchId);
    await expect(page.locator('.card', { hasText: 'Precedentes' }), 'no precedent exists, so no strip').toHaveCount(0);
  });

  test('05 calendar: the lone proposal shows its score with Propuesta, the disputed match shows none', async ({ page }) => {
    const { control, disputed } = await seeded();
    await asMine(page);
    await page.locator(`a[href^="/competition/${ctx.competitionId}"]`).first().click();
    await page.waitForURL(`**/competition/${ctx.competitionId}**`);
    await page.locator('input[aria-label^="Jornadas"], input[aria-label^="Partidos"]').click();
    const rowOf = async (matchId: string) => {
      const link = page.locator(`a[href="/match/${matchId}"]`).first();
      const round = page.locator('.collapse', { has: link }).locator('> input');
      if (await round.count()) await round.check();
      return link;
    };
    const proposed = await rowOf(control.matchId);
    await expect(proposed).toContainText('6-3 6-4');
    await expect(proposed.locator('.badge', { hasText: 'Propuesta' })).toBeVisible();
    await expect(proposed.locator(WARNING)).toHaveCount(0);
    const dispute = await rowOf(disputed.matchId);
    await expect(dispute, 'a disputed result is not a result: no score in the row').not.toContainText(/(^|\s)[0-7]-[0-7](\s|$)/); // a set score, not the pair-name suffix
  });

  test('04 admin card counter: played counts the lone proposal only, with the warning', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const total = (await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
      `competition='${ctx.competitionId}'`)}&perPage=1`)).totalItems;
    const card = page.locator(`a[href="/admin/competitions/${ctx.competitionId}"]`).first();
    await expect(card).toContainText(`1/${total} partidos`);
    await expect(card.locator(WARNING)).toBeVisible();
  });
});
