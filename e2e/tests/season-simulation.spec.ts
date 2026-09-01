import { test, expect, Page, APIRequestContext } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore } from '../tour-helpers';
import {
  setPlayerPassword, uniqueSuffix, SCORE_MATRIX, PENALTIES,
  computeExpected, PlannedMatch, PairId,
} from '../season-helpers';

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
let suToken = '';

const LABEL_TO_INDEX: Record<PairId, number> = { A: 0, B: 1, C: 2, D: 3 };

interface MatchFixture {
  id: string;
  pair1: string;
  pair2: string;
  pair1Label: PairId;
  pair2Label: PairId;
  planned: PlannedMatch;
  orientedScore: string;
}

test.describe('season simulation', () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'season simulation is DB-mutating; runs desktop-only');
  });

  test.describe.configure({ retries: 0 });

  test('league standings are exact after a full ida y vuelta', async ({ page }) => {
    test.setTimeout(240000);

    await buildSeason(page);

    // Step A: map fixtures to scores
    const fixtures = await mapFixturesToScores(page.request);

    // Step B: play all 12 matches with varied interactions
    await playAllMatches(page, fixtures);

    // Step C: Phase A assertion — no penalty yet
    await assertStandings(page, computeExpected(SCORE_MATRIX, {}), false);

    // Step D: Phase B — apply penalty to Pair A, mark one pair paid
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await applyPenalty(page, competitionId, pairIds[0]);
    await togglePayment(page, competitionId, pairIds[1]);
    await assertStandings(page, computeExpected(SCORE_MATRIX, PENALTIES), true);
  });

  test('playoff seeds from the league, advances, and crowns the expected champion', async ({ page }) => {
    test.setTimeout(240000);
    // Reuses the players/pairs created by test 1 (same worker + DB). Re-auth superuser
    // in case the token from test 1 has aged.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const authResp = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    suToken = (await authResp.json()).token;

    const playoffId = await createPlayoffCompetition(page);
    // Seed by league finish (no penalty): A=1, B=2, C=3, D=4.
    for (let i = 0; i < 4; i++) {
      await addPairToCompetition(page, playoffId, pairIds[i], i + 1);
    }
    await generateFixtures(page, playoffId);

    // Round 1: seed1 v seed4 and seed2 v seed3 → {A,D} and {B,C}.
    const r1 = await getRoundMatches(page.request, playoffId, 1);
    expect(r1.length).toBe(2);
    const r1Pairings = r1.map(m => [idToLabel(m.pair1), idToLabel(m.pair2)].sort().join(''));
    expect(r1Pairings).toContain('AD');
    expect(r1Pairings).toContain('BC');

    // Play the semis: A beats D, B beats C.
    for (const m of r1) {
      const labels = [idToLabel(m.pair1), idToLabel(m.pair2)];
      if (labels.includes('A')) await playPlayoffMatch(page, m, 'A', '6-3 6-4');
      else await playPlayoffMatch(page, m, 'B', '6-4 6-3');
    }

    // Auto-advance (R-50/R-51): round 2 populates with the two winners in slot order.
    const r2 = await getRoundMatches(page.request, playoffId, 2);
    expect(r2.length).toBe(1);
    const final = r2[0];
    expect(idToLabel(final.pair1)).toBe('A');
    expect(idToLabel(final.pair2)).toBe('B');

    // Final: A beats B → champion A.
    await playPlayoffMatch(page, final, 'A', '6-2 6-3');
    const finalDone = await getMatchById(page.request, final.id);
    expect(idToLabel(finalDone.winner)).toBe('A');
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
  suToken = superuserToken;

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
  await modal.locator('select[name="gender"]').selectOption('male');

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

  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));

  const resp = await page.request.get(`/api/collections/pairs/records?filter=name='${name}'`, {
    headers: { Authorization: token },
  });
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Failed to find created pair: ${name}`);
  return id;
}

async function addPairToCompetition(page: Page, compId: string, pairId: string, seed?: number) {
  await page.goto(`/admin/competitions/${compId}`);
  await page.selectOption('select[name="pair"]', pairId);
  if (seed !== undefined) {
    await page.fill('input[name="seed"]', String(seed));
  }
  // AddPair returns redirectHX (204 → window.location); await the redirect so it
  // does not race the next navigation.
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Añadir")'));
}

async function createPlayoffCompetition(page: Page): Promise<string> {
  const name = `Playoff ${RUN_ID}`;
  await page.goto('/admin/competitions');
  await page.getByRole('button', { name: /crear competición/i }).first().click();
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="type"]').selectOption('playoff');
  await dialog.locator('input[name="active"]').check();
  await Promise.all([
    page.waitForResponse(resp => resp.url().includes('/admin/competitions') && resp.status() < 400),
    dialog.locator('button[type="submit"]').click(),
  ]);
  await page.goto('/admin/competitions');
  await expect(page.getByText(name).first()).toBeVisible({ timeout: 10000 });
  const href = await page.locator(`a:has-text("${name}")`).first().getAttribute('href');
  if (!href) throw new Error('Playoff competition link not found');
  const id = href.split('/').pop();
  if (!id) throw new Error('Playoff competition id not found');
  return id;
}

async function getRoundMatches(request: APIRequestContext, compId: string, round: number): Promise<any[]> {
  const resp = await request.get(
    `/api/collections/matches/records?filter=competition='${compId}'&sort=created&perPage=50`,
    { headers: { Authorization: suToken } },
  );
  const items = (await resp.json()).items;
  return items.filter((m: any) => Number(m.round_number) === round);
}

async function getMatchById(request: APIRequestContext, id: string): Promise<any> {
  const resp = await request.get(`/api/collections/matches/records/${id}`, { headers: { Authorization: suToken } });
  return await resp.json();
}

async function setDateAndClub(request: APIRequestContext, matchId: string): Promise<void> {
  await request.patch(
    `/api/collections/matches/records/${matchId}`,
    {
      headers: { Authorization: suToken },
      data: { date: '2025-03-15', club: 'Padel 360' },
    },
  );
}

function playerEmailForPairId(pairId: string, idx: 0 | 1): string {
  return playerEmailForPair(idToLabel(pairId), idx);
}

// Play a playoff match: pair1's player submits the winner-oriented score, pair2's player accepts.
async function playPlayoffMatch(page: Page, match: any, winnerLabel: PairId, winnerScore: string) {
  const p1Label = idToLabel(match.pair1);
  const oriented = p1Label === winnerLabel ? winnerScore : orientScore(winnerScore, true);
  await setDateAndClub(page.request, match.id);
  await loginAs(page, playerEmailForPairId(match.pair1, 0), PLAYER_PASSWORD);
  await submitScore(page, match.id, oriented);
  await loginAs(page, playerEmailForPairId(match.pair2, 0), PLAYER_PASSWORD);
  await confirmScore(page, match.id);
}

async function generateFixtures(page: Page, compId: string) {
  await page.goto(`/admin/competitions/${compId}`);
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Generar calendario")'));
}

// HTMX + redirectHX: click triggers XHR → 204 + HX-Redirect → window.location.href.
// Must wait for the full page load after the redirect completes.
async function clickAndWaitForHxRedirect(page: Page, locator: ReturnType<Page['locator']>) {
  const navPromise = page.waitForEvent('framenavigated', { timeout: 15000 });
  await locator.click();
  await navPromise;
  await page.waitForLoadState('domcontentloaded');
}

// --- T3: Map fixtures to scores ---

function idToLabel(pairId: string): PairId {
  const idx = pairIds.indexOf(pairId);
  if (idx < 0) throw new Error(`Unknown pair ID: ${pairId}`);
  return (['A', 'B', 'C', 'D'] as PairId[])[idx];
}

function orientScore(score: string, flip: boolean): string {
  if (!flip) return score;
  return score.split(/\s+/).map(s => {
    const [a, b] = s.split('-');
    return `${b}-${a}`;
  }).join(' ');
}

async function mapFixturesToScores(request: APIRequestContext): Promise<MatchFixture[]> {
  const resp = await request.get(
    `/api/collections/matches/records?filter=competition='${competitionId}'&perPage=50&sort=round_number,created`,
    { headers: { Authorization: suToken } },
  );
  const data = await resp.json();
  if (data.items.length !== 12) {
    throw new Error(`Expected 12 matches, got ${data.items.length}`);
  }

  const fixtures: MatchFixture[] = [];
  for (const m of data.items) {
    const p1Label = idToLabel(m.pair1);
    const p2Label = idToLabel(m.pair2);
    const planned = SCORE_MATRIX.find(
      s => s.home === p1Label && s.away === p2Label,
    );
    const plannedFlipped = SCORE_MATRIX.find(
      s => s.home === p2Label && s.away === p1Label,
    );
    if (!planned && !plannedFlipped) {
      throw new Error(`No SCORE_MATRIX entry for ${p1Label} vs ${p2Label}`);
    }
    const flip = !planned;
    const entry = (planned || plannedFlipped)!;
    fixtures.push({
      id: m.id,
      pair1: m.pair1,
      pair2: m.pair2,
      pair1Label: p1Label,
      pair2Label: p2Label,
      planned: entry,
      orientedScore: orientScore(entry.score, flip),
    });
  }
  return fixtures;
}

// --- T3: Play matches ---

function playerEmailForPair(pairLabel: PairId, playerIndex: 0 | 1): string {
  const pairIdx = LABEL_TO_INDEX[pairLabel];
  const playerGlobalIdx = PAIRS[pairIdx][playerIndex === 0 ? 'p1' : 'p2'];
  return PLAYERS[playerGlobalIdx].email;
}

async function submitScore(page: Page, matchId: string, score: string) {
  await page.goto(`/match/${matchId}`);
  await enterScore(page, score);
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'));
}

async function confirmScore(page: Page, matchId: string) {
  await page.goto(`/match/${matchId}`);
  await page.waitForSelector('#thread-details', { timeout: 5000 });
  const acceptBtn = page.locator('#thread-details button:has-text("Confirmar")').first();
  await acceptBtn.waitFor({ timeout: 5000 });
  await clickAndWaitForHxRedirect(page, acceptBtn);
}

// Thread actions must run on the FULL match page: the `/match/{id}/thread`
// fragment is served without the HTMX runtime (it lives only in layout.html),
// so its forms never fire when that URL is opened directly. On `/match/{id}`
// the thread lazy-loads via hx-get (match.html) with HTMX active.
async function gotoMatchThread(page: Page, matchId: string) {
  await page.goto(`/match/${matchId}`);
  await page.locator('form[hx-post$="/thread/message"]').waitFor({ state: 'visible', timeout: 15000 });
}

async function postProposal(page: Page, matchId: string) {
  await gotoMatchThread(page, matchId);
  await page.locator('form[hx-post$="/thread/message"] input[name="content"]').fill('Shall we play?');
  await clickAndWaitForHxRedirect(page, page.locator('form[hx-post$="/thread/message"] button[type="submit"]'));

  await gotoMatchThread(page, matchId);
  // When no date+place is set, the form renders as an open card (no collapse).
  // When already scheduled, it's inside a collapse that must be expanded.
  const collapse = page.locator('.collapse', { has: page.locator('.collapse-title:has-text("Proponer fecha")') });
  if (await collapse.count() > 0) {
    await collapse.locator('input[type="checkbox"]').check();
  }
  const tomorrow = new Date(Date.now() + 86400000).toISOString().slice(0, 10);
  await page.fill('#proposal-form input[name="date"]', tomorrow);
  await page.fill('#proposal-form input[name="time"]', '18:00');
  await page.selectOption('#proposal-form select[name="venue_id"]', 'otro');
  await page.fill('#proposal-form input[name="venue_text"]', 'Test Club');
  await clickAndWaitForHxRedirect(page, page.locator('#proposal-form button:has-text("Proponer fecha")'));
}

async function acceptProposal(page: Page, matchId: string) {
  await page.goto(`/match/${matchId}`);
  const acceptBtn = page.locator('button:has-text("Aceptar")').first();
  await acceptBtn.waitFor({ state: 'visible', timeout: 15000 });
  await clickAndWaitForHxRedirect(page, acceptBtn);
}

async function rejectProposal(page: Page, matchId: string) {
  await page.goto(`/match/${matchId}`);
  const rejectBtn = page.locator('button:has-text("Rechazar")').first();
  await rejectBtn.waitFor({ state: 'visible', timeout: 15000 });
  await rejectBtn.click();
  await page.locator('select[name="rejection_reason"]').selectOption({ index: 1 });
  await clickAndWaitForHxRedirect(page, page.locator('form.reject-form button[type="submit"]'));
}

async function playAllMatches(page: Page, fixtures: MatchFixture[]) {
  for (let i = 0; i < fixtures.length; i++) {
    const f = fixtures[i];
    const submitterEmail = playerEmailForPair(f.pair1Label, 0);
    const confirmerEmail = playerEmailForPair(f.pair2Label, 0);

    if (i < 4) {
      // Matches 0-3: scheduling proposal + accept, then submit + confirm
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await postProposal(page, f.id);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await acceptProposal(page, f.id);
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await submitScore(page, f.id, f.orientedScore);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await confirmScore(page, f.id);
    } else if (i >= 4 && i <= 7) {
      // Matches 4-7: set date+club via API, then submit + accept
      await setDateAndClub(page.request, f.id);
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await submitScore(page, f.id, f.orientedScore);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await confirmScore(page, f.id);
    } else if (i === 8 || i === 9) {
      // Matches 8-9: set date+club, submit + accept
      await setDateAndClub(page.request, f.id);
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await submitScore(page, f.id, f.orientedScore);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await confirmScore(page, f.id);
    } else if (i === 10) {
      // Match 10: proposal rejected, second proposal accepted, then submit + confirm
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await postProposal(page, f.id);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await rejectProposal(page, f.id);
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await postProposal(page, f.id);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await acceptProposal(page, f.id);
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await submitScore(page, f.id, f.orientedScore);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await confirmScore(page, f.id);
    } else {
      // Match 11: set date+club, submit + accept
      await setDateAndClub(page.request, f.id);
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await submitScore(page, f.id, f.orientedScore);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await confirmScore(page, f.id);
    }

    // Verify match reached final status
    const matchResp = await page.request.get(
      `/api/collections/matches/records/${f.id}`,
      { headers: { Authorization: suToken } },
    );
    const matchData = await matchResp.json();
    if (matchData.status !== 'final') {
      throw new Error(`Match ${i} (${f.pair1Label} vs ${f.pair2Label}) status is '${matchData.status}', expected 'final'`);
    }
  }
}

// --- T3: Standings assertions ---

async function assertStandings(
  page: Page,
  expected: ReturnType<typeof computeExpected>,
  hasPenalties: boolean,
) {
  await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
  await page.goto(`/competition/${competitionId}`);
  // Click the Clasificación tab
  await page.locator('input[aria-label="Clasificación"]').click();
  await page.waitForSelector('table.table-zebra tbody tr', { timeout: 5000 });

  const rows = page.locator('table.table-zebra tbody tr');
  const count = await rows.count();
  if (count !== 4) throw new Error(`Expected 4 standings rows, got ${count}`);

  for (let i = 0; i < expected.length; i++) {
    const row = rows.nth(i);
    const cells = row.locator('td');
    const exp = expected[i];
    const pairName = PAIRS[LABEL_TO_INDEX[exp.pair]].name;

    await expect(cells.nth(0)).toContainText(String(exp.position));
    await expect(cells.nth(1)).toContainText(pairName);
    await expect(cells.nth(2)).toContainText(String(exp.played));
    await expect(cells.nth(3)).toContainText(String(exp.wins));
    await expect(cells.nth(4)).toContainText(String(exp.losses));
    await expect(cells.nth(5)).toContainText(`${exp.setsWon}/${exp.setsLost}`);
    await expect(cells.nth(6)).toContainText(`${exp.gamesWon}/${exp.gamesLost}`);
    await expect(cells.nth(7)).toContainText(String(exp.points));

    if (hasPenalties) {
      if (exp.penalty > 0) {
        await expect(cells.nth(8)).toContainText(`-${exp.penalty}`);
      }
    }
  }
}

// --- T3: Penalty and payment ---

async function applyPenalty(page: Page, compId: string, pairId: string) {
	await page.goto(`/admin/competitions/${compId}`);
	const modal = page.locator(`#penalty-modal-${pairId} + .modal`);
	await page.locator(`label[for="penalty-modal-${pairId}"]:has-text("Penalizar")`).click();
	await modal.locator('textarea[name="reason"]').fill('Ajuste de clasificación');
	await clickAndWaitForHxRedirect(page, modal.locator('button:has-text("Confirmar penalización")'));
}

async function togglePayment(page: Page, compId: string, pairId: string) {
  await page.goto(`/admin/competitions/${compId}`);
  const checkbox = page.locator(`tr:has(input[value="${pairId}"]) input[type="checkbox"]`).first();
  await clickAndWaitForHxRedirect(page, checkbox);
}
