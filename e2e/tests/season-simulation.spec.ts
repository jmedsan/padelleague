import type { Page, APIRequestContext } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import { loginAs, asPlayerOn, loadTestData, suGet, suPost, suPatch, apiListRecords, isMobile, clickAction, leagueDate, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { enterScore, fillFlatpickrDate, clickAndWaitForHxRedirect, clickConfirmAndWaitForHxRedirect, cellByHeader, expectRedirectedTo } from '../tour-helpers';
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
let suToken = '';
let fixtures: MatchFixture[] = [];

const LABEL_TO_INDEX: Record<PairId, number> = { A: 0, B: 1, C: 2, D: 3 };

// SeasonPairs is one set of the four pairs A-D: their competition, their pair
// ids (index 0 = A) and their players' emails (indexed like PLAYERS).
interface SeasonPairs {
  competitionId: string;
  pairIds: string[];
  emails: string[];
}

// The league the serial quarters build through the UI; filled in by Q1.
const league: SeasonPairs = { competitionId: '', pairIds: [], emails: PLAYERS.map(p => p.email) };

interface MatchFixture {
  id: string;
  pair1: string;
  pair2: string;
  pair1Label: PairId;
  pair2Label: PairId;
  planned: PlannedMatch;
  orientedScore: string;
}

test.describe('season simulation', { tag: '@standings' }, () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'season simulation is DB-mutating; runs desktop-only');
  });

  test.describe.configure({ retries: 0 });

  // Split into serial quarters so a failure is diagnosable and each test is
  // lighter. State (league, suToken, fixtures) is shared via module-level
  // variables — set by Q1, consumed by Q2-Q4. Serial so a Q1 failure skips
  // them instead of running them on empty state.
  test.describe.serial('league season', () => {
    test('Q1: build season and play matches 0-2 (scheduling proposal flow)', { tag: '@smoke' }, async ({ page }) => {
      test.setTimeout(180000);
      await buildSeason(page);
      fixtures = await mapFixturesToScores(page.request);
      await playMatchRange(page, fixtures, 0, 2);
    });

    test('Q2: play matches 3-5 (API-set date + submit/confirm flow)', async ({ page }) => {
      test.setTimeout(120000);
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await playMatchRange(page, fixtures, 3, 5);
    });

    test('Q3: play matches 6-8', async ({ page }) => {
      test.setTimeout(120000);
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await playMatchRange(page, fixtures, 6, 8);
    });

    test('Q4: play matches 9-11, assert standings, apply penalty', async ({ page }) => {
      test.setTimeout(180000);
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await playMatchRange(page, fixtures, 9, 11);

      // Phase A: assert standings without penalty
      await assertStandings(page, computeExpected(SCORE_MATRIX, {}), false);

      // Phase B: apply penalty to Pair A, mark one pair paid, re-assert
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await applyPenalty(page, league.competitionId, league.pairIds[0]);
      await togglePayment(page, league.competitionId, league.pairIds[1]);
      await assertStandings(page, computeExpected(SCORE_MATRIX, PENALTIES), true);
    });
  });

  // The unfinished-match and playoff tests build their own pairs through the
  // API (seedSeasonPairs), so each runs alone — `make e2e-failed` or -g —
  // with nothing from the quarters above.
  test.describe('on their own pairs', () => {
    test('unfinished match: partial score creates carried-sets state, resumed match finalizes correctly', async ({ page }) => {
      test.setTimeout(120000);
      suToken = loadTestData().adminToken;
      const season = await seedSeasonPairs(page.request);

      // Create a fresh match — avoid round_number collision with the league fixtures (1-6).
      const match = await suPost(page.request, suToken, '/api/collections/matches/records', {
          competition: season.competitionId,
          pair1: season.pairIds[0],  // Pair A
          pair2: season.pairIds[1],  // Pair B
          status: 'scheduled',
          round_number: 99,
          date: '2025-08-01',
          club: 'Padel 360',
      });
      const matchId = match.id;

      // Step 1: Pair A's player submits a partial score — 6-4 2-6 3-4 (open last set).
      const submitterEmail = playerEmailForPair(season.emails, 'A', 0);
      const confirmerEmail = playerEmailForPair(season.emails, 'B', 0);

      await asPlayerOn(page, season.competitionId, submitterEmail, PLAYER_PASSWORD);
      await page.goto(`/match/${matchId}`);
      await page.waitForSelector('#thread-details', { timeout: 20000 });
      await enterScore(page, '6-4 2-6 3-4');
      await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'), `/match/${matchId}`);

      // Step 2: Pair B accepts the partial score.
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await page.goto(`/match/${matchId}`);
      await page.waitForSelector('#thread-details', { timeout: 20000 });
      const acceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
      await acceptBtn.waitFor({ timeout: 20000 });
      await clickAndWaitForHxRedirect(page, acceptBtn, `/match/${matchId}`);

      // Step 3: Verify the match shows the "Se reanuda desde" carried-sets badge.
      await page.goto(`/match/${matchId}`);
      await page.waitForLoadState('domcontentloaded');
      await expect(page.locator('.badge', { hasText: 'Se reanuda desde' })).toBeVisible({ timeout: 20000 });

      // Step 4: Schedule the resumed match via API.
      await suPatch(page.request, suToken, `/api/collections/matches/records/${matchId}`, { status: 'scheduled', date: '2025-08-15', club: 'Wurko' });

      // Step 5: Pair A submits the finishing score. Every completed set is
      // carried (6-4 and 2-6), so two sets render locked and only set 3 is open.
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await page.goto(`/match/${matchId}`);
      await page.waitForSelector('#thread-details', { timeout: 20000 });

      const lockedSets = page.locator('.score-set-group[data-locked]');
      await expect(lockedSets).toHaveCount(2, { timeout: 15000 });
      await expect(page.locator('select[name="s3a"]')).toBeEnabled();

      // Submit the full score — the two locked sets are skipped, set 3 is filled.
      await enterScore(page, '6-4 2-6 6-3');
      await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'), `/match/${matchId}`);

      // Step 6: Pair B accepts the final score.
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await page.goto(`/match/${matchId}`);
      await page.waitForSelector('#thread-details', { timeout: 20000 });
      const finalAcceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
      await finalAcceptBtn.waitFor({ timeout: 20000 });
      await clickAndWaitForHxRedirect(page, finalAcceptBtn, `/match/${matchId}`);

      // Step 7: Match finalized — Confirmado badge, full score visible, no reanuda badge.
      await page.goto(`/match/${matchId}`);
      await page.waitForLoadState('networkidle');
      await expect(page.locator('#thread-details').getByText('Confirmado')).toBeVisible({ timeout: 20000 });
      // The full score 6-4 2-6 6-3 (Pair A wins 2-1)
      await expect(page.getByText('6-4 2-6 6-3').locator('visible=true').first()).toBeVisible({ timeout: 15000 });
      await expect(page.locator('.badge', { hasText: 'Se reanuda' })).not.toBeVisible();
    });

    test('playoff seeds from the league, advances, and crowns the expected champion', async ({ page }) => {
      test.setTimeout(240000);
      suToken = loadTestData().adminToken;
      const season = await seedSeasonPairs(page.request);
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

      const playoffId = await createPlayoffCompetition(page);
      // Seed A=1, B=2, C=3, D=4 — the league's finish without penalty.
      for (let i = 0; i < 4; i++) {
        await addPairToCompetition(page, playoffId, season.pairIds[i], i + 1);
      }
      await generateFixtures(page, playoffId);

      // Round 1: seed1 v seed4 and seed2 v seed3 → {A,D} and {B,C}.
      const r1 = await getRoundMatches(page.request, playoffId, 1);
      expect(r1.length).toBe(2);
      const r1Pairings = r1.map(m => [idToLabel(season.pairIds, m.pair1), idToLabel(season.pairIds, m.pair2)].sort().join(''));
      expect(r1Pairings).toContain('AD');
      expect(r1Pairings).toContain('BC');

      // Play the semis: A beats D, B beats C.
      for (const m of r1) {
        const labels = [idToLabel(season.pairIds, m.pair1), idToLabel(season.pairIds, m.pair2)];
        if (labels.includes('A')) await playPlayoffMatch(page, season, m, 'A', '6-3 6-4');
        else await playPlayoffMatch(page, season, m, 'B', '6-4 6-3');
      }

      // Auto-advance (R-50/R-51): round 2 populates with the two winners in slot order.
      const r2 = await getRoundMatches(page.request, playoffId, 2);
      expect(r2.length).toBe(1);
      const final = r2[0];
      expect(idToLabel(season.pairIds, final.pair1)).toBe('A');
      expect(idToLabel(season.pairIds, final.pair2)).toBe('B');

      // Final: A beats B → champion A.
      await playPlayoffMatch(page, season, final, 'A', '6-2 6-3');
      const finalDone = await getMatchById(page.request, final.id);
      expect(idToLabel(season.pairIds, finalDone.winner)).toBe('A');
    });
  });
});

async function buildSeason(page: Page) {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

  // Get superuser token first — needed for API lookups and password setting
  suToken = loadTestData().adminToken;

  // Step 1: Pre-create 8 players via admin UI
  playerIds = [];
  for (const player of PLAYERS) {
    await preCreatePlayer(page, player.email, player.name);
  }

  // Look up all player IDs and set passwords via superuser API
  for (const player of PLAYERS) {
    const id = (await apiListRecords(page.request, suToken, 'users', `email='${player.email}'`))[0]?.id;
    if (!id) throw new Error(`Player not found after pre-create: ${player.email}`);
    playerIds.push(id);
    await setPlayerPassword(page.request, suToken, id, PLAYER_PASSWORD);
  }

  // Step 2: Create league competition with play_twice
  league.competitionId = await createLeagueCompetition(page);

  // Step 3: Create 4 pairs and add each to the competition
  league.pairIds = [];
  for (const pair of PAIRS) {
    const pairId = await createPair(page, pair.name, playerIds[pair.p1], playerIds[pair.p2], suToken);
    league.pairIds.push(pairId);
    await addPairToCompetition(page, league.competitionId, pairId);
  }

  // Step 4: Generate fixtures
  await generateFixtures(page, league.competitionId);
}

async function preCreatePlayer(page: Page, email: string, displayName: string): Promise<void> {
  await page.goto('/admin/players');
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
    throw new Error(`Pre-create failed with status ${createResp.status()} for ${email}`);
  }

  // HTMX hx-target="body" replaces the page with the success alert
  await expect(page.getByText('Usuario creado').locator('visible=true').first()).toBeVisible({ timeout: 5000 });
}

async function createLeagueCompetition(page: Page): Promise<string> {
  await page.goto('/admin/competitions');
  await page.getByRole('button', { name: /crear competición/i }).first().click();
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(COMP_NAME);
  await dialog.locator('select[name="type"]').selectOption('league');
  await dialog.locator('input[name="play_twice"]').check();
  await dialog.locator('input[name="active"]').check();

  // admin_competitions.go's create handler redirects to /admin/competitions/{id}.
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'), /^\/admin\/competitions\/[^/]+$/);

  await page.goto('/admin/competitions');
  await expect(page.getByText(COMP_NAME).locator('visible=true').first()).toBeVisible({ timeout: 10000 });

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

  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'), '/admin/pairs');

  const id = (await apiListRecords(page.request, token, 'pairs', `name='${name}'`))[0]?.id;
  if (!id) throw new Error(`Failed to find created pair: ${name}`);
  return id;
}

async function addPairToCompetition(page: Page, compId: string, pairId: string, seed?: number) {
  await page.goto(`/admin/competitions/${compId}`);
  // aria-label disambiguates from the leveled-league "Filtrar por pareja" select.
  await page.selectOption('select[aria-label^="Pareja"]', pairId);
  if (seed !== undefined) {
    await page.fill('input[name="seed"]', String(seed));
  }
  // AddPair returns redirectHX (204 → window.location); await the redirect so it
  // does not race the next navigation.
  await clickAndWaitForHxRedirect(page, page.getByTestId('section-add-pairs').locator('button:has-text("Añadir")'), `/admin/competitions/${compId}`);
}

async function createPlayoffCompetition(page: Page): Promise<string> {
  const name = `Playoff ${RUN_ID}`;
  await page.goto('/admin/competitions');
  await page.getByRole('button', { name: /crear competición/i }).first().click();
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="type"]').selectOption('playoff');
  await dialog.locator('input[name="active"]').check();
  // admin_competitions.go's create handler redirects to /admin/competitions/{id}.
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'), /^\/admin\/competitions\/[^/]+$/);
  await page.goto('/admin/competitions');
  await expect(page.getByText(name).locator('visible=true').first()).toBeVisible({ timeout: 10000 });
  const href = await page.locator(`a:has-text("${name}")`).first().getAttribute('href');
  if (!href) throw new Error('Playoff competition link not found');
  const id = href.split('/').pop();
  if (!id) throw new Error('Playoff competition id not found');
  return id;
}

async function getRoundMatches(request: APIRequestContext, compId: string, round: number): Promise<any[]> {
  const { items } = await suGet(request, suToken, `/api/collections/matches/records?filter=competition='${compId}'&sort=created&perPage=50`);
  return items.filter((m: any) => Number(m.round_number) === round);
}

async function getMatchById(request: APIRequestContext, id: string): Promise<any> {
  return suGet(request, suToken, `/api/collections/matches/records/${id}`);
}

async function setDateAndClub(request: APIRequestContext, matchId: string): Promise<void> {
  await suPatch(request, suToken, `/api/collections/matches/records/${matchId}`, { date: '2025-03-15', club: 'Padel 360' });
}

function playerEmailForPairId(season: SeasonPairs, pairId: string, idx: 0 | 1): string {
  return playerEmailForPair(season.emails, idToLabel(season.pairIds, pairId), idx);
}

// seedSeasonPairs builds a fresh A-D set through the API: eight players with
// unique emails, the four pairs, and a league holding them.
async function seedSeasonPairs(request: APIRequestContext): Promise<SeasonPairs> {
  const suffix = uniqueSuffix();
  const emails: string[] = [];
  const ids: string[] = [];
  for (const player of PLAYERS) {
    const email = player.email.replace('@', `-${suffix}@`);
    const user = await suPost(request, suToken, '/api/collections/users/records', {
      email, display_name: `${player.name} ${suffix}`, gender: 'male', roles: ['player'],
      password: PLAYER_PASSWORD, passwordConfirm: PLAYER_PASSWORD, verified: true,
    });
    emails.push(email);
    ids.push(user.id);
  }
  const pairIds: string[] = [];
  for (const pair of PAIRS) {
    const rec = await suPost(request, suToken, '/api/collections/pairs/records', {
      name: `${pair.name} ${suffix}`, player1: ids[pair.p1], player2: ids[pair.p2], captain: ids[pair.p1],
    });
    pairIds.push(rec.id);
  }
  const comp = await suPost(request, suToken, '/api/collections/competitions/records', {
    name: `${COMP_NAME} ${suffix}`, type: 'league', active: true, pairs: pairIds,
  });
  return { competitionId: comp.id, pairIds, emails };
}

// Play a playoff match: pair1's player submits the winner-oriented score, pair2's player accepts.
async function playPlayoffMatch(page: Page, season: SeasonPairs, match: any, winnerLabel: PairId, winnerScore: string) {
  const p1Label = idToLabel(season.pairIds, match.pair1);
  const oriented = p1Label === winnerLabel ? winnerScore : orientScore(winnerScore, true);
  await setDateAndClub(page.request, match.id);
  await loginAs(page, playerEmailForPairId(season, match.pair1, 0), PLAYER_PASSWORD);
  await submitScore(page, match.id, oriented);
  await loginAs(page, playerEmailForPairId(season, match.pair2, 0), PLAYER_PASSWORD);
  await confirmScore(page, match.id);
}

// Generating leaves the calendar in draft, which hides every match from
// players; publish it in the same step so the player flows can see them.
async function generateFixtures(page: Page, compId: string) {
  await page.goto(`/admin/competitions/${compId}`);
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Generar calendario")'), `/admin/competitions/${compId}`);
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Publicar calendario")'), `/admin/competitions/${compId}`);
}

// --- T3: Map fixtures to scores ---

function idToLabel(pairIds: string[], pairId: string): PairId {
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
  const data = await suGet(request, suToken, `/api/collections/matches/records?filter=competition='${league.competitionId}'&perPage=50&sort=round_number,created`);
  if (data.items.length !== 12) {
    throw new Error(`Expected 12 matches, got ${data.items.length}`);
  }

  const fixtures: MatchFixture[] = [];
  for (const m of data.items) {
    const p1Label = idToLabel(league.pairIds, m.pair1);
    const p2Label = idToLabel(league.pairIds, m.pair2);
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

function playerEmailForPair(emails: string[], pairLabel: PairId, playerIndex: 0 | 1): string {
  const pairIdx = LABEL_TO_INDEX[pairLabel];
  const playerGlobalIdx = PAIRS[pairIdx][playerIndex === 0 ? 'p1' : 'p2'];
  return emails[playerGlobalIdx];
}

async function submitScore(page: Page, matchId: string, score: string) {
  await page.goto(`/match/${matchId}`);
  await enterScore(page, score);
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'), `/match/${matchId}`);
}

async function confirmScore(page: Page, matchId: string) {
  await page.goto(`/match/${matchId}`);
  await page.waitForSelector('#thread-details', { timeout: 15000 });
  const acceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
  await acceptBtn.waitFor({ timeout: 15000 });
  await clickAndWaitForHxRedirect(page, acceptBtn, `/match/${matchId}`);
}

// Thread actions must run on the FULL match page: the `/match/{id}/thread`
// fragment is served without the HTMX runtime (it lives only in layout.html),
// so its forms never fire when that URL is opened directly. On `/match/{id}`
// the thread lazy-loads via hx-get (match.html) with HTMX active.
async function gotoMatchThread(page: Page, matchId: string) {
  await page.goto(`/match/${matchId}`);
  await page.locator('form[hx-post$="/thread/message"]').waitFor({ state: 'visible', timeout: 30000 });
}

async function postProposal(page: Page, matchId: string) {
  await gotoMatchThread(page, matchId);
  await page.locator('form[hx-post$="/thread/message"] input[name="content"]').fill('Shall we play?');
  await clickAndWaitForHxRedirect(page, page.locator('form[hx-post$="/thread/message"] button[type="submit"]'), `/match/${matchId}`);

  await gotoMatchThread(page, matchId);
  // Every caller proposes on a match with no date+place yet (matches 0-3 and
  // 10; 10's first proposal is rejected), so the form is an open card, not
  // the collapse a scheduled match shows.
  const tomorrow = leagueDate(1);
  await fillFlatpickrDate(page, '#proposal-date', tomorrow);
  await page.locator('#proposal-time').selectOption('18:00');
  await page.selectOption('#proposal-form select[name="venue_id"]', 'otro');
  await page.fill('#proposal-form input[name="venue_text"]', 'Test Club');
  await clickAndWaitForHxRedirect(page, page.locator('#proposal-form button:has-text("Proponer fecha")'), `/match/${matchId}`);
}

async function acceptProposal(page: Page, matchId: string) {
  await page.goto(`/match/${matchId}`);
  const acceptBtn = page.locator('button:has-text("Aceptar")').first();
  await acceptBtn.waitFor({ state: 'visible', timeout: 30000 });
  await clickAndWaitForHxRedirect(page, acceptBtn, `/match/${matchId}`);
}

async function rejectProposal(page: Page, matchId: string) {
  await page.goto(`/match/${matchId}`);
  const rejectBtn = page.locator('button:has-text("Rechazar")').first();
  await rejectBtn.waitFor({ state: 'visible', timeout: 30000 });
  await rejectBtn.click();
  await page.locator('form.reject-form select[name="rejection_reason"]').selectOption({ index: 1 });
  await clickAndWaitForHxRedirect(page, page.locator('form.reject-form button[type="submit"]'), `/match/${matchId}`);
}

// playMatchRange plays fixtures[start..end] (inclusive) using the same varied
// interaction patterns as the original single-test flow — matches 0-3 use the
// scheduling proposal UI, match 10 exercises reject+re-propose, the rest use
// the API date-set shortcut.
async function playMatchRange(page: Page, allFixtures: MatchFixture[], start: number, end: number) {
  for (let i = start; i <= end; i++) {
    const f = allFixtures[i];
    const submitterEmail = playerEmailForPair(league.emails, f.pair1Label, 0);
    const confirmerEmail = playerEmailForPair(league.emails, f.pair2Label, 0);

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
      // All other matches: set date+club via API, then submit + confirm
      await setDateAndClub(page.request, f.id);
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await submitScore(page, f.id, f.orientedScore);
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await confirmScore(page, f.id);
    }

    // Verify match reached final status
    const matchData = await suGet(page.request, suToken, `/api/collections/matches/records/${f.id}`);
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
  await page.locator(`a[href^="/competition/${league.competitionId}"]`).first().click();
  await page.waitForURL(`**/competition/${league.competitionId}**`);
  // Click the Clasificación tab
  await page.locator('input[aria-label^="Clasificación"]').click();
  // standingsTable.html renders a desktop table.table-zebra and a mobile
  // table.table-sm, each hidden at the other breakpoint via CSS — the two
  // tables also order columns differently (mobile puts Pts third so it's
  // visible without scrolling), so cells are looked up by header text via
  // cellByHeader rather than a fixed index.
  const table = isMobile(page) ? page.locator('table.table-sm') : page.locator('table.table-zebra');
  await table.locator('tbody tr').first().waitFor({ timeout: 15000 });

  const rows = table.locator('tbody tr');
  const count = await rows.count();
  if (count !== 4) throw new Error(`Expected 4 standings rows, got ${count}`);

  for (let i = 0; i < expected.length; i++) {
    const row = rows.nth(i);
    const exp = expected[i];
    const pairName = PAIRS[LABEL_TO_INDEX[exp.pair]].name;

    const setDiff = exp.setsWon - exp.setsLost;
    const gameDiff = exp.gamesWon - exp.gamesLost;

    await expect(await cellByHeader(table, row, '#')).toContainText(String(exp.position));
    await expect(await cellByHeader(table, row, 'Pareja')).toContainText(pairName);
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

// --- T3: Penalty and payment ---

async function applyPenalty(page: Page, compId: string, pairId: string) {
	await page.goto(`/admin/competitions/${compId}`);
	const modal = page.locator(`#penalty-modal-${pairId} + .modal`);
	await clickAction(page, `label[for="penalty-modal-${pairId}"]`, 'Penalizar');
	await modal.locator('textarea[name="reason"]').fill('Ajuste de clasificación');
	await clickAndWaitForHxRedirect(page, modal.locator('button:has-text("Confirmar penalización")'), `/admin/competitions/${compId}`);
}

async function togglePayment(page: Page, compId: string, pairId: string) {
  await page.goto(`/admin/competitions/${compId}`);
  // Payment is an icon toggle button behind the custom confirm modal.
  const toggle = page.locator(`form[hx-post$="/payment"]:has(input[value="${pairId}"]) button:visible`).first();
  await clickConfirmAndWaitForHxRedirect(page, toggle, `/admin/competitions/${compId}`);
}
