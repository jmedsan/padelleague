import { test, expect, Page, APIRequestContext } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  setPlayerPassword, uniqueSuffix, SCORE_MATRIX, PENALTIES,
  computeExpected, PlannedMatch, PairId,
} from '../season-helpers';
import {
  createPlayer, createCompetition, createPair, addPairToCompetition, markAllPairsPaid,
  generateFixtures, submitScore, confirmScore, disputeScore, resolveDispute,
  createDocument, attachDocumentToCompetition, acceptDocsGate,
  clickAndWaitForHxRedirect,
  assertFinalStandings, assertPlayoffChampion,
  lookupPlayerId, getRoundMatches, getMatchById,
} from '../tour-helpers';

const RUN_ID = uniqueSuffix();
const PLAYER_PASSWORD = 'TestPass123456';

const PLAYERS = [
  { name: `R-Ana ${RUN_ID}`,    email: `r-ana-${RUN_ID}@test.local` },
  { name: `R-Bruno ${RUN_ID}`,  email: `r-bruno-${RUN_ID}@test.local` },
  { name: `R-Carla ${RUN_ID}`,  email: `r-carla-${RUN_ID}@test.local` },
  { name: `R-David ${RUN_ID}`,  email: `r-david-${RUN_ID}@test.local` },
  { name: `R-Elena ${RUN_ID}`,  email: `r-elena-${RUN_ID}@test.local` },
  { name: `R-Felix ${RUN_ID}`,  email: `r-felix-${RUN_ID}@test.local` },
  { name: `R-Gloria ${RUN_ID}`, email: `r-gloria-${RUN_ID}@test.local` },
  { name: `R-Hugo ${RUN_ID}`,   email: `r-hugo-${RUN_ID}@test.local` },
];

const PAIRS: { name: string; label: PairId; p1: number; p2: number }[] = [
  { name: `R-Pair A ${RUN_ID}`, label: 'A', p1: 0, p2: 1 },
  { name: `R-Pair B ${RUN_ID}`, label: 'B', p1: 2, p2: 3 },
  { name: `R-Pair C ${RUN_ID}`, label: 'C', p1: 4, p2: 5 },
  { name: `R-Pair D ${RUN_ID}`, label: 'D', p1: 6, p2: 7 },
];

const COMP_NAME = `R-League ${RUN_ID}`;
const PLAYOFF_NAME = `R-Playoff ${RUN_ID}`;

let playerIds: string[] = [];
let pairIds: string[] = [];
let competitionId = '';
let suToken = '';

const LABEL_TO_INDEX: Record<PairId, number> = { A: 0, B: 1, C: 2, D: 3 };

const pairNames: Record<PairId, string> = {
  A: PAIRS[0].name, B: PAIRS[1].name, C: PAIRS[2].name, D: PAIRS[3].name,
};

// ---------------------------------------------------------------------------
// Nav-menu helper — opens the desktop Gestión dropdown and clicks the link
// ---------------------------------------------------------------------------

async function navTo(page: Page, label: string): Promise<void> {
  // Ensure the layout (with nav menu) is present — HTMX body replacements
  // and HX-Redirects can leave the page without the nav chrome.
  if (!await page.locator('summary:has-text("Gestión")').isVisible().catch(() => false)) {
    await page.goto('/');
  }
  await page.locator('summary:has-text("Gestión")').click();
  const link = page.locator(`.menu-horizontal a:has-text("${label}")`);
  await link.click();
  await page.waitForLoadState('domcontentloaded');
}

// ---------------------------------------------------------------------------
// Score orientation helpers (same logic as season-simulation)
// ---------------------------------------------------------------------------

function orientScore(score: string, flip: boolean): string {
  if (!flip) return score;
  return score.split(/\s+/).map(s => {
    const [a, b] = s.split('-');
    return `${b}-${a}`;
  }).join(' ');
}

function idToLabel(pairId: string): PairId {
  const idx = pairIds.indexOf(pairId);
  if (idx < 0) throw new Error(`Unknown pair ID: ${pairId}`);
  return (['A', 'B', 'C', 'D'] as PairId[])[idx];
}

function playerEmailForPair(label: PairId, playerIndex: 0 | 1): string {
  const pairIdx = LABEL_TO_INDEX[label];
  const globalIdx = PAIRS[pairIdx][playerIndex === 0 ? 'p1' : 'p2'];
  return PLAYERS[globalIdx].email;
}

interface MatchFixture {
  id: string;
  pair1Label: PairId;
  pair2Label: PairId;
  orientedScore: string;
}

// ---------------------------------------------------------------------------
// Test
// ---------------------------------------------------------------------------

test.describe('reference navigation tour', () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'reference tour runs desktop-only');
  });

  test.describe.configure({ retries: 0 });

  test('complete league + playoff via nav-menu navigation', async ({ page }) => {
    test.setTimeout(420000);

    // --- Auth superuser ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const authResp = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    if (!authResp.ok()) throw new Error(`Superuser auth failed: ${authResp.status()}`);
    suToken = (await authResp.json()).token;

    // --- Step 1: Create players via Jugadores nav link ---
    playerIds = [];
    for (const player of PLAYERS) {
      await navTo(page, 'Jugadores');
      await createPlayer(page, player.email, player.name);
    }
    for (const player of PLAYERS) {
      const id = await lookupPlayerId(page.request, suToken, player.email);
      playerIds.push(id);
      await setPlayerPassword(page.request, suToken, id, PLAYER_PASSWORD);
    }

    // --- Step 2: Create pairs via Parejas nav link ---
    pairIds = [];
    for (const pair of PAIRS) {
      await navTo(page, 'Parejas');
      const pairId = await createPair(page, pair.name, playerIds[pair.p1], playerIds[pair.p2], suToken);
      pairIds.push(pairId);
    }

    // --- Step 3: Create league competition via Panel ---
    await navTo(page, 'Panel');
    competitionId = await createCompetition(page, COMP_NAME, 'league', { playTwice: true, suToken });

    // --- Step 4: Add pairs to competition ---
    // After createCompetition, navigate to the competition detail page.
    await navTo(page, 'Panel');
    await page.locator(`a:has-text("${COMP_NAME}")`).first().click();
    await page.waitForLoadState('domcontentloaded');

    for (const pairId of pairIds) {
      await addPairToCompetition(page, pairId);
    }

    // --- Step 5: Generate fixtures ---
    await generateFixtures(page);

    // A pair can't play without paying — mark all pairs paid.
    await markAllPairsPaid(page);

    // --- Step 5b: Admin creates + attaches mandatory doc, player passes gate ---
    await navTo(page, 'Documentos');
    await createDocument(page, 'Reglamento de prueba', 'https://example.com/reglamento', {
      mandatory: true,
    });

    // Attach it to the league competition
    await navTo(page, 'Panel');
    await page.locator(`a:has-text("${COMP_NAME}")`).first().click();
    await page.waitForLoadState('domcontentloaded');
    await attachDocumentToCompetition(page, 'Reglamento de prueba');

    // Player hits the gate on first entry
    await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
    await page.goto(`/competition/${competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await acceptDocsGate(page);
    await expect(page.locator('input[aria-label="Jornadas"]')).toBeVisible({ timeout: 5000 });

    // Back to admin for the rest
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

    // --- Step 6: Play all 12 matches (submit + confirm) ---
    const fixtures = await mapFixturesToScores(page.request);

    for (const [fixtureIndex, f] of fixtures.entries()) {
      const submitterEmail = playerEmailForPair(f.pair1Label, 0);
      const confirmerEmail = playerEmailForPair(f.pair2Label, 0);

      // Submitter logs in, navigates to match page
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await gotoMatchViaCompetition(page, f.id);
      await submitScore(page, f.orientedScore);

      // Confirmer logs in, navigates to match
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await gotoMatchViaCompetition(page, f.id);
      if (fixtureIndex === 0) {
        await disputeScore(page);
        await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
        await gotoMatchViaCompetition(page, f.id);
        const resolveForm = page.locator(`form[hx-post="/admin/disputes/${f.id}/resolve"]`);
        await expect(resolveForm).toBeVisible();
        await expect(resolveForm.locator('input[name="score"]')).toHaveValue(f.orientedScore);
        await resolveDispute(page, f.id, f.orientedScore);
      } else {
        await confirmScore(page);
      }

      // Verify final
      const matchData = await getMatchById(page.request, suToken, f.id);
      if (matchData.status !== 'final') {
        throw new Error(`Match ${f.pair1Label} vs ${f.pair2Label} status '${matchData.status}', expected 'final'`);
      }
    }

    // --- Step 7: Assert standings (no penalty) ---
    const expectedNoPenalty = computeExpected(SCORE_MATRIX, {});
    await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
    await assertFinalStandings(page, competitionId, pairNames, expectedNoPenalty, false);

    // --- Step 8: Apply penalty via admin competition page ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navTo(page, 'Panel');
    await page.locator(`a:has-text("${COMP_NAME}")`).first().click();
    await page.waitForLoadState('domcontentloaded');
    const penaltyModal = page.locator(`#penalty-modal-${pairIds[0]}`).locator('..');
    await page.locator(`label[for="penalty-modal-${pairIds[0]}"]:has-text("Penalizar")`).click();
    await penaltyModal.locator('textarea[name="reason"]').fill('Ajuste de clasificación');
    await clickAndWaitForHxRedirect(page, penaltyModal.locator('button:has-text("Confirmar penalización")'));

    // Assert standings with penalty
    const expectedWithPenalty = computeExpected(SCORE_MATRIX, PENALTIES);
    await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
    await assertFinalStandings(page, competitionId, pairNames, expectedWithPenalty, true);

    // --- Step 9: Create playoff via Panel ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navTo(page, 'Panel');
    const playoffId = await createCompetition(page, PLAYOFF_NAME, 'playoff', { suToken });

    // Navigate to playoff detail page
    await navTo(page, 'Panel');
    await page.locator(`a:has-text("${PLAYOFF_NAME}")`).first().click();
    await page.waitForLoadState('domcontentloaded');

    // Seed by league finish: A=1, B=2, C=3, D=4
    for (let i = 0; i < 4; i++) {
      await addPairToCompetition(page, pairIds[i], i + 1);
    }
    await generateFixtures(page);

    // --- Step 10: Play playoff ---
    // Semis: A beats D, B beats C
    const r1 = await getRoundMatches(page.request, suToken, playoffId, 1);
    expect(r1.length).toBe(2);

    for (const m of r1) {
      const labels = [idToLabel(m.pair1), idToLabel(m.pair2)];
      const winnerLabel: PairId = labels.includes('A') ? 'A' : 'B';
      const winnerScore = labels.includes('A') ? '6-3 6-4' : '6-4 6-3';
      await playPlayoffMatch(page, m, winnerLabel, winnerScore);
    }

    // Final: A beats B
    const r2 = await getRoundMatches(page.request, suToken, playoffId, 2);
    expect(r2.length).toBe(1);
    const final = r2[0];
    expect(idToLabel(final.pair1)).toBe('A');
    expect(idToLabel(final.pair2)).toBe('B');
    await playPlayoffMatch(page, final, 'A', '6-2 6-3');

    const finalDone = await getMatchById(page.request, suToken, final.id);
    expect(idToLabel(finalDone.winner)).toBe('A');

    // --- Step 11: Assert playoff champion ---
    await assertPlayoffChampion(page, playoffId, PAIRS[0].name);

    // --- Step 12: Entity interlinking — competition → pair → player ---
    await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
    await page.goto(`/competition/${competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    // Switch to Clasificación tab, then click pair link in standings
    await page.locator('input[aria-label="Clasificación"]').click();
    const pairLink = page.locator(`a[href="/pair/${pairIds[0]}"]`).first();
    await expect(pairLink).toBeVisible();
    await pairLink.click();
    await page.waitForLoadState('domcontentloaded');

    // Verify pair page content
    expect(page.url()).toContain(`/pair/${pairIds[0]}`);
    await expect(page.locator('h1')).toContainText(PAIRS[0].name);
    await expect(page.locator(`table a[href="/player/${playerIds[0]}"]`)).toBeVisible();
    await expect(page.locator(`table a[href="/player/${playerIds[1]}"]`)).toBeVisible();
    await expect(page.locator(`a[href="/competition/${competitionId}"]`)).toBeVisible();

    // Click player link → verify player page
    await page.locator(`table a[href="/player/${playerIds[0]}"]`).click();
    await page.waitForLoadState('domcontentloaded');
    expect(page.url()).toContain(`/player/${playerIds[0]}`);
    await expect(page.locator('h1')).toContainText(PLAYERS[0].name);

    // --- Step 13: Double-role (R-150) — admin+player "Ver como" switcher ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');

    // Admin has both roles → "Ver como" switcher visible
    const verComo = page.locator('summary:has-text("Ver como")');
    await expect(verComo).toBeVisible();
    // Default view is admin — Gestión dropdown visible
    await expect(page.locator('summary:has-text("Gestión")')).toBeVisible();

    // Switch to player view via desktop nav
    const desktopNav = page.locator('.menu-horizontal');
    await verComo.click();
    await desktopNav.locator('a[href="/view/player"]').click();
    await page.waitForLoadState('domcontentloaded');
    // In player view: Gestión should be hidden, player home content visible
    await expect(page.locator('summary:has-text("Gestión")')).not.toBeVisible();
    await expect(page.locator('h1, h2').first()).toBeVisible();

    // Switch back to admin view
    const verComoPlayer = page.locator('.menu-horizontal summary:has-text("Ver como")');
    await verComoPlayer.click();
    await desktopNav.locator('a[href="/view/admin"]').click();
    await page.waitForLoadState('domcontentloaded');
    // Gestión visible again
    await expect(page.locator('summary:has-text("Gestión")')).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function gotoMatchViaCompetition(page: Page, matchId: string): Promise<void> {
  await page.goto(`/match/${matchId}`);
  await page.waitForLoadState('domcontentloaded');
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
    const planned = SCORE_MATRIX.find(s => s.home === p1Label && s.away === p2Label);
    const plannedFlipped = SCORE_MATRIX.find(s => s.home === p2Label && s.away === p1Label);
    if (!planned && !plannedFlipped) {
      throw new Error(`No SCORE_MATRIX entry for ${p1Label} vs ${p2Label}`);
    }
    const flip = !planned;
    const entry = (planned || plannedFlipped)!;
    fixtures.push({
      id: m.id,
      pair1Label: p1Label,
      pair2Label: p2Label,
      orientedScore: orientScore(entry.score, flip),
    });
  }
  return fixtures;
}

async function playPlayoffMatch(page: Page, match: any, winnerLabel: PairId, winnerScore: string) {
  const p1Label = idToLabel(match.pair1);
  const oriented = p1Label === winnerLabel ? winnerScore : orientScore(winnerScore, true);

  const submitterEmail = playerEmailForPair(p1Label, 0);
  const confirmerEmail = playerEmailForPair(idToLabel(match.pair2), 0);

  await loginAs(page, submitterEmail, PLAYER_PASSWORD);
  await gotoMatchViaCompetition(page, match.id);
  await submitScore(page, oriented);

  await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
  await gotoMatchViaCompetition(page, match.id);
  await confirmScore(page);
}
