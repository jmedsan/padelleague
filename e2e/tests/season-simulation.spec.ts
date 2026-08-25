import { test, expect, Page } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { setPlayerPassword, uniqueSuffix } from '../season-helpers';

const RUN_ID = uniqueSuffix();
const PLAYER_PASSWORD = 'TestPass123456';

const PLAYERS = [
  { name: `Ana ${RUN_ID}`,    email: `ana-${RUN_ID}@test.local` },
  { name: `Bruno ${RUN_ID}`,  email: `bruno-${RUN_ID}@test.local` },
  { name: `Carla ${RUN_ID}`,  email: `carla-${RUN_ID}@test.local` },
  { name: `David ${RUN_ID}`,  email: `david-${RUN_ID}@test.local` },
  { name: `Elena ${RUN_ID}`,  email: `elena-${RUN_ID}@test.local` },
  { name: `Felix ${RUN_ID}`,  email: `felix-${RUN_ID}@test.local` },
  { name: `Gloria ${RUN_ID}`, email: `gloria-${RUN_ID}@test.local` },
  { name: `Hugo ${RUN_ID}`,   email: `hugo-${RUN_ID}@test.local` },
];

const PAIRS = [
  { name: `Pair A ${RUN_ID}`, p1: 0, p2: 1 },
  { name: `Pair B ${RUN_ID}`, p1: 2, p2: 3 },
  { name: `Pair C ${RUN_ID}`, p1: 4, p2: 5 },
  { name: `Pair D ${RUN_ID}`, p1: 6, p2: 7 },
];

const COMP_NAME = `Season Sim ${RUN_ID}`;

let playerIds: string[] = [];
let pairIds: string[] = [];
let competitionId = '';

test.describe('season simulation', () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'season simulation is DB-mutating; runs desktop-only');
  });

  test.describe.configure({ retries: 0 });

  test('league standings are exact after a full ida y vuelta', async ({ page }) => {
    test.setTimeout(240000);

    await buildSeason(page);

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${competitionId}`);
    for (const pair of PAIRS) {
      await expect(page.getByText(pair.name).first()).toBeVisible({ timeout: 5000 });
    }
    await page.goto(`/competition/${competitionId}`);
    const matchLinks = page.locator('a[href^="/match/"]');
    await expect(matchLinks).toHaveCount(12, { timeout: 10000 });

    // TODO: Task 3 — play all 12 matches and assert standings
  });

  test('playoff seeds from the league, advances, and crowns the expected champion', async ({ page }) => {
    test.setTimeout(240000);
    test.skip(true, 'not implemented yet');
  });
});

async function buildSeason(page: Page) {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

  // Get superuser token first — needed for API lookups and password setting
  const authResp = await page.request.post('/api/collections/_superusers/auth-with-password', {
    data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
  });
  if (!authResp.ok()) throw new Error(`Superuser auth failed: ${authResp.status()}`);
  const { token: superuserToken } = await authResp.json();

  // Step 1: Pre-create 8 players via admin UI
  playerIds = [];
  for (const player of PLAYERS) {
    await preCreatePlayer(page, player.email, player.name);
  }

  // Look up all player IDs and set passwords via superuser API
  for (const player of PLAYERS) {
    const apiResp = await page.request.get(
      `/api/collections/users/records?filter=email='${player.email}'`,
      { headers: { Authorization: superuserToken } },
    );
    const data = await apiResp.json();
    const id = data.items?.[0]?.id;
    if (!id) throw new Error(`Player not found after pre-create: ${player.email}`);
    playerIds.push(id);
    await setPlayerPassword(page.request, superuserToken, id, PLAYER_PASSWORD);
  }

  // Step 2: Create league competition with play_twice
  competitionId = await createLeagueCompetition(page);

  // Step 3: Create 4 pairs and add each to the competition
  pairIds = [];
  for (const pair of PAIRS) {
    const pairId = await createPair(page, pair.name, playerIds[pair.p1], playerIds[pair.p2], superuserToken);
    pairIds.push(pairId);
    await addPairToCompetition(page, competitionId, pairId);
  }

  // Step 4: Generate fixtures
  await generateFixtures(page, competitionId);
}

async function preCreatePlayer(page: Page, email: string, displayName: string): Promise<void> {
  await page.goto('/admin/players');
  await page.locator('label[for="precreate-modal"]').first().click();
  const modal = page.locator('.modal[role="dialog"]').filter({ hasText: 'Pre-crear usuario' });
  await modal.locator('input[name="email"]').fill(email);
  await modal.locator('input[name="display_name"]').fill(displayName);

  const responsePromise = page.waitForResponse(
    resp => resp.url().includes('/admin/players/pre-create'),
  );
  await modal.locator('button[type="submit"]').click();
  const createResp = await responsePromise;

  if (createResp.status() >= 300) {
    throw new Error(`Pre-create failed with status ${createResp.status()} for ${email}`);
  }

  // HTMX hx-target="body" replaces the page with the success alert
  await expect(page.getByText('Usuario creado').first()).toBeVisible({ timeout: 5000 });
}

async function createLeagueCompetition(page: Page): Promise<string> {
  await page.goto('/admin/competitions');
  await page.getByRole('button', { name: /crear competición/i }).first().click();
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(COMP_NAME);
  await dialog.locator('select[name="type"]').selectOption('league');
  await dialog.locator('input[name="play_twice"]').check();
  await dialog.locator('input[name="active"]').check();

  await Promise.all([
    page.waitForResponse(resp => resp.url().includes('/admin/competitions') && resp.status() < 400),
    dialog.locator('button[type="submit"]').click(),
  ]);

  await page.goto('/admin/competitions');
  await expect(page.getByText(COMP_NAME).first()).toBeVisible({ timeout: 10000 });

  const compLink = page.locator(`a:has-text("${COMP_NAME}")`).first();
  const href = await compLink.getAttribute('href');
  if (!href) throw new Error('Competition link not found');
  const id = href.split('/').pop();
  if (!id) throw new Error('Competition ID not found in href');
  return id;
}

async function createPair(page: Page, name: string, player1Id: string, player2Id: string, token: string): Promise<string> {
  await page.goto('/admin/pairs');
  await page.evaluate(() => {
    (document.getElementById('modal-create') as HTMLDialogElement)?.showModal();
  });
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="player1"]').selectOption(player1Id);
  await dialog.locator('select[name="player2"]').selectOption(player2Id);

  await Promise.all([
    page.waitForResponse(resp => resp.url().includes('/admin/pairs') && resp.status() < 400),
    dialog.locator('button[type="submit"]').click(),
  ]);
  await page.waitForLoadState('load');

  const resp = await page.request.get(`/api/collections/pairs/records?filter=name='${name}'`, {
    headers: { Authorization: token },
  });
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Failed to find created pair: ${name}`);
  return id;
}

async function addPairToCompetition(page: Page, compId: string, pairId: string) {
  await page.goto(`/admin/competitions/${compId}`);
  await page.selectOption('select[name="pair"]', pairId);

  await Promise.all([
    page.waitForResponse(resp => resp.url().includes(`/admin/competitions/${compId}/pairs`) && resp.status() < 400),
    page.locator('button:has-text("Añadir")').click(),
  ]);
}

async function generateFixtures(page: Page, compId: string) {
  await page.goto(`/admin/competitions/${compId}`);

  await Promise.all([
    page.waitForResponse(resp => resp.url().includes(`/admin/competitions/${compId}/generate`) && resp.status() < 400),
    page.locator('button:has-text("Generar calendario")').click(),
  ]);
}
