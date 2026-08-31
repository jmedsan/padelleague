import { test, expect, Page, APIRequestContext } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  setPlayerPassword, uniqueSuffix, SCORE_MATRIX, PENALTIES,
  computeExpected, PlannedMatch, PairId,
} from '../season-helpers';
import {
  createPlayer, createPair, addPairToCompetition, markAllPairsPaid,
  generateFixtures, setDates, submitScore, confirmScore, disputeScore,
  resolveDispute, createDocument, attachDocumentToCompetition, acceptDocsGate,
  clickAndWaitForHxRedirect,
  assertFinalStandings, assertPlayoffChampion,
  lookupPlayerId, getRoundMatches, getMatchById,
  referenceFallback, collectFallbacks, resetFallbacks, assertFallbacksMatch,
} from '../tour-helpers';

const RUN_ID = uniqueSuffix();
const PLAYER_PASSWORD = 'TestPass123456';

const PLAYERS = [
  { name: `G-Ana ${RUN_ID}`,    email: `g-ana-${RUN_ID}@test.local` },
  { name: `G-Bruno ${RUN_ID}`,  email: `g-bruno-${RUN_ID}@test.local` },
  { name: `G-Carla ${RUN_ID}`,  email: `g-carla-${RUN_ID}@test.local` },
  { name: `G-David ${RUN_ID}`,  email: `g-david-${RUN_ID}@test.local` },
  { name: `G-Elena ${RUN_ID}`,  email: `g-elena-${RUN_ID}@test.local` },
  { name: `G-Felix ${RUN_ID}`,  email: `g-felix-${RUN_ID}@test.local` },
  { name: `G-Gloria ${RUN_ID}`, email: `g-gloria-${RUN_ID}@test.local` },
  { name: `G-Hugo ${RUN_ID}`,   email: `g-hugo-${RUN_ID}@test.local` },
];

const PAIRS: { name: string; label: PairId; p1: number; p2: number }[] = [
  { name: `G-Pair A ${RUN_ID}`, label: 'A', p1: 0, p2: 1 },
  { name: `G-Pair B ${RUN_ID}`, label: 'B', p1: 2, p2: 3 },
  { name: `G-Pair C ${RUN_ID}`, label: 'C', p1: 4, p2: 5 },
  { name: `G-Pair D ${RUN_ID}`, label: 'D', p1: 6, p2: 7 },
];

const COMP_NAME = `G-League ${RUN_ID}`;
const PLAYOFF_NAME = `G-Playoff ${RUN_ID}`;
const EXPECTED_FALLBACKS: string[] = ['no Documentos quick-link on home'];

let playerIds: string[] = [];
let pairIds: string[] = [];
let competitionId = '';
let playoffId = '';
let suToken = '';

const LABEL_TO_INDEX: Record<PairId, number> = { A: 0, B: 1, C: 2, D: 3 };

const pairNames: Record<PairId, string> = {
  A: PAIRS[0].name, B: PAIRS[1].name, C: PAIRS[2].name, D: PAIRS[3].name,
};

// ---------------------------------------------------------------------------
// Score orientation helpers
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
  pair1: string;
  pair2: string;
  pair1Label: PairId;
  pair2Label: PairId;
  orientedScore: string;
}

// ---------------------------------------------------------------------------
// Home-affordance navigation helpers
// ---------------------------------------------------------------------------

async function goHome(page: Page): Promise<void> {
  await page.goto('/');
  await page.waitForLoadState('domcontentloaded');
}

async function clickAdminQuickLink(page: Page, label: string): Promise<void> {
  const link = page.locator(`a:has-text("${label}")`).filter({ has: page.locator('svg') });
  await link.last().click();
  await page.waitForLoadState('domcontentloaded');
}

async function gotoMatchViaNextMatch(page: Page): Promise<string> {
  await goHome(page);
  const verPartido = page.getByRole('link', { name: 'Ver partido' }).first();
  await verPartido.click();
  await page.waitForLoadState('domcontentloaded');
  const match = page.url().match(/\/match\/([^/?]+)/);
  if (!match) throw new Error('Failed to extract match ID from URL after clicking Ver partido');
  return match[1];
}

async function gotoMatchViaPendingAction(page: Page): Promise<void> {
  await goHome(page);
  const action = page.locator('a[href^="/match/"]').filter({ hasText: 'Confirmar resultado' }).first();
  await action.click();
  await page.waitForLoadState('domcontentloaded');
}

async function gotoMatchViaCompCard(page: Page, compId: string, matchId: string): Promise<void> {
  await goHome(page);
  await page.locator(`a[href="/competition/${compId}"]`).first().click();
  await page.waitForLoadState('domcontentloaded');
  if (await page.getByRole('heading', { name: 'Documentos obligatorios' }).isVisible().catch(() => false)) {
    await acceptDocsGate(page);
  }
  await page.locator(`a[href="/match/${matchId}"]`).first().click();
  await page.waitForLoadState('domcontentloaded');
}

// ---------------------------------------------------------------------------
// Test
// ---------------------------------------------------------------------------

test.describe('guided navigation tour', () => {
  test.describe.configure({ retries: 0 });

  test('complete league + playoff via home affordances (P4, P6)', async ({ page }) => {
    test.setTimeout(420000);
    resetFallbacks();

    // --- Auth superuser for API lookups ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const authResp = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    if (!authResp.ok()) throw new Error(`Superuser auth failed: ${authResp.status()}`);
    suToken = (await authResp.json()).token;

    // =======================================================================
    // Phase 1: Admin setup via home affordances
    // =======================================================================

    // 1a. Competiciones quick-link → /admin/competitions → create league (inactive)
    // (Bootstrap card only shows with zero competitions; seed data creates one.)
    await goHome(page);
    await clickAdminQuickLink(page, 'Competiciones');
    competitionId = await createCompInactive(page, COMP_NAME, 'league', true);

    // 1b. Jugadores quick-link → create 8 players
    playerIds = [];
    for (const player of PLAYERS) {
      await goHome(page);
      await clickAdminQuickLink(page, 'Jugadores');
      await createPlayer(page, player.email, player.name);
    }
    for (const player of PLAYERS) {
      const id = await lookupPlayerId(page.request, suToken, player.email);
      playerIds.push(id);
      await setPlayerPassword(page.request, suToken, id, PLAYER_PASSWORD);
    }

    // 1c. Parejas quick-link → create 4 pairs
    pairIds = [];
    for (const pair of PAIRS) {
      await goHome(page);
      await clickAdminQuickLink(page, 'Parejas');
      const id = await createPair(page, pair.name, playerIds[pair.p1], playerIds[pair.p2], suToken);
      pairIds.push(id);
    }

    // 1d. Configurar → competition detail → add pairs + generate fixtures + set dates
    await goHome(page);
    await page.getByTestId('setup-configure').click();
    await page.waitForLoadState('domcontentloaded');
    for (const pairId of pairIds) {
      await addPairToCompetition(page, pairId);
    }
    await generateFixtures(page);
    const startDate = new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10);
    const endDate = new Date(Date.now() + 30 * 86400000).toISOString().slice(0, 10);
    await setDates(page, startDate, endDate);
    // A pair can't play without paying — mark all pairs paid before activating.
    await markAllPairsPaid(page);

    // 1e. Configurar → activate via toggle
    await goHome(page);
    await page.getByTestId('setup-configure').click();
    await page.waitForLoadState('domcontentloaded');
    await clickAndWaitForHxRedirect(page, page.locator('.toggle.toggle-success'));

    // =======================================================================
    // Phase 1f: Admin creates mandatory doc + player passes gate
    // =======================================================================

    // Admin creates a mandatory document via Documentos panel
    await goHome(page);
    await referenceFallback(page, 'no Documentos quick-link on home', '/admin/documents');
    await createDocument(page, 'Reglamento de prueba', 'https://example.com/reglamento', {
      mandatory: true,
    });

    // Admin attaches it to the competition
    await goHome(page);
    await clickAdminQuickLink(page, 'Competiciones');
    await page.getByRole('link', { name: COMP_NAME }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await attachDocumentToCompetition(page, 'Reglamento de prueba');

    // Player hits the gate on first competition entry
    await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
    await page.goto(`/competition/${competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    await acceptDocsGate(page);

    // After accepting, player sees normal competition page
    await expect(page.locator('input[aria-label="Jornadas"]')).toBeVisible({ timeout: 5000 });

    // =======================================================================
    // Phase 2: Play all 12 league matches via home affordances
    // =======================================================================

    const fixtures = await mapFixturesToScores(page.request);

    for (let i = 0; i < fixtures.length; i++) {
      const f = fixtures[i];
      const submitterEmail = playerEmailForPair(f.pair1Label, 0);
      const confirmerEmail = playerEmailForPair(f.pair2Label, 0);

      if (i === 8 || i === 9) {
        // Dispute flow: submit → dispute → admin resolve via Disputas quick-link
        await loginAs(page, submitterEmail, PLAYER_PASSWORD);
        await gotoMatchViaCompCard(page, competitionId, f.id);
        await submitScore(page, f.orientedScore);

        await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
        await gotoMatchViaPendingAction(page);
        await disputeScore(page);

        await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
        await goHome(page);
        await clickAdminQuickLink(page, 'Disputas');
        await resolveDispute(page, f.id, f.orientedScore);
      } else {
        // Standard flow: submit via home → confirm via PendingActions
        await loginAs(page, submitterEmail, PLAYER_PASSWORD);
        await gotoMatchViaCompCard(page, competitionId, f.id);

        if (i === 0) {
          // R-174: proposal accordion visible on pending match
          const proposalTitle = page.locator('.collapse-title:has-text("Proponer fecha")');
          await expect(proposalTitle).toBeVisible({ timeout: 3000 });

          // R-174: no availability buttons on thread page
          await expect(page.locator('button:has-text("Estoy libre")')).toHaveCount(0);
          await expect(page.locator('button:has-text("No puedo")')).toHaveCount(0);

          // Expand proposal accordion to check venue select
          const proposalCollapse = page.locator('.collapse:has(select[name="venue_id"])');
          const proposalCheckbox = proposalCollapse.locator('> input[type="checkbox"]');
          if (!(await proposalCheckbox.isChecked())) {
            await proposalCheckbox.check({ force: true });
            await page.waitForTimeout(300);
          }

          // R-171: venue select must not pre-select any option
          const venueSelect = page.locator('select[name="venue_id"]');
          await expect(venueSelect).toBeVisible({ timeout: 3000 });
          const selectedOptions = venueSelect.locator('option[selected]');
          await expect(selectedOptions).toHaveCount(0);
        }

        await submitScore(page, f.orientedScore);

        await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
        await gotoMatchViaPendingAction(page);
        await confirmScore(page);

        // R-169: after confirming, the share button must include the match URL
        if (i === 0) {
          const shareBtn = page.locator('button:has-text("Compartir")');
          await expect(shareBtn).toBeVisible({ timeout: 3000 });
          const onclick = await shareBtn.getAttribute('onclick') ?? '';
          expect(onclick).toMatch(new RegExp(`match.*${f.id}`));

          // R-164: no raw ISO date strings visible on the match detail page
          const bodyText = await page.locator('body').innerText();
          expect(bodyText).not.toMatch(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}Z/);

          // R-174: no scheduling controls on a played match
          await page.goto(`/match/${f.id}`);
          await page.waitForLoadState('domcontentloaded');
          await page.waitForSelector('#match-thread', { timeout: 5000 });
          await expect(page.locator('button:has-text("Estoy libre")')).toHaveCount(0);
          await expect(page.locator('button:has-text("No puedo")')).toHaveCount(0);
          await expect(page.locator('text=Proponer fecha')).toHaveCount(0);
        }
      }

      // Verify match reached final
      const matchData = await getMatchById(page.request, suToken, f.id);
      if (matchData.status !== 'final') {
        throw new Error(`Match ${i} (${f.pair1Label} vs ${f.pair2Label}) status '${matchData.status}', expected 'final'`);
      }
    }

    // =======================================================================
    // Phase 3: Assert standings (no penalty)
    // =======================================================================

    const expectedNoPenalty = computeExpected(SCORE_MATRIX, {});
    await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
    await assertFinalStandings(page, competitionId, pairNames, expectedNoPenalty, false);

    // =======================================================================
    // Phase 4: Apply penalty via Competiciones quick-link
    // =======================================================================

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await goHome(page);
    await clickAdminQuickLink(page, 'Competiciones');
    await page.getByRole('link', { name: COMP_NAME }).first().click();
    await page.waitForLoadState('domcontentloaded');
    // Expand Parejas accordion (collapsed when started + all paid)
    const parejasSection = page.locator('[data-testid="section-parejas"]');
    const parejasCheckbox = parejasSection.locator('> input[type="checkbox"]');
    if (!(await parejasCheckbox.isChecked())) {
      await parejasCheckbox.check({ force: true });
      await page.waitForTimeout(300);
    }
    const penaltyModal = page.locator(`#penalty-modal-${pairIds[0]} + .modal`);
    await page.locator(`label[for="penalty-modal-${pairIds[0]}"]:has-text("Penalizar")`).click();
    await penaltyModal.locator('textarea[name="reason"]').fill('Ajuste de clasificación');
    await clickAndWaitForHxRedirect(page, penaltyModal.locator('button:has-text("Confirmar penalización")'));

    // Assert standings with penalty
    const expectedWithPenalty = computeExpected(SCORE_MATRIX, PENALTIES);
    await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
    await assertFinalStandings(page, competitionId, pairNames, expectedWithPenalty, true);

    // =======================================================================
    // Phase 5: Finalize league via Competiciones quick-link
    // =======================================================================

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await goHome(page);
    await clickAdminQuickLink(page, 'Competiciones');
    await page.getByRole('link', { name: COMP_NAME }).first().click();
    await page.waitForLoadState('domcontentloaded');
    page.once('dialog', dialog => dialog.accept());
    await clickAndWaitForHxRedirect(page, page.getByTestId('finalize-league'));

    // =======================================================================
    // Phase 6: Playoff creation via playoff-prompt card
    // =======================================================================

    await goHome(page);
    // Playoff-prompt card appears only when no other active playoff exists.
    // In the full suite, other specs may have created one, so fall back to the
    // Competiciones quick-link (still a home affordance).
    const promptVisible = await page.getByTestId('playoff-prompt').isVisible().catch(() => false);
    if (promptVisible) {
      await page.getByTestId('playoff-prompt').locator('a').click();
    } else {
      await clickAdminQuickLink(page, 'Competiciones');
    }
    await page.waitForLoadState('domcontentloaded');
    playoffId = await createCompInactive(page, PLAYOFF_NAME, 'playoff', false);

    // Configurar → add pairs (seeded by league finish: A=1, B=2, C=3, D=4)
    await goHome(page);
    await page.locator(`a[href="/admin/competitions/${playoffId}"][data-testid="setup-configure"]`).click();
    await page.waitForLoadState('domcontentloaded');
    for (let i = 0; i < 4; i++) {
      await addPairToCompetition(page, pairIds[i], i + 1);
    }
    await generateFixtures(page);

    // Activate playoff via toggle
    await clickAndWaitForHxRedirect(page, page.locator('.toggle.toggle-success'));

    // =======================================================================
    // Phase 7: Play playoff via home affordances
    // =======================================================================

    // Semis: A beats D, B beats C
    const r1 = await getRoundMatches(page.request, suToken, playoffId, 1);
    expect(r1.length).toBe(2);

    for (const m of r1) {
      const labels = [idToLabel(m.pair1), idToLabel(m.pair2)];
      const winnerLabel: PairId = labels.includes('A') ? 'A' : 'B';
      const winnerScore = labels.includes('A') ? '6-3 6-4' : '6-4 6-3';
      const p1Label = idToLabel(m.pair1);
      const oriented = p1Label === winnerLabel ? winnerScore : orientScore(winnerScore, true);

      const submitterEmail = playerEmailForPair(p1Label, 0);
      const confirmerEmail = playerEmailForPair(idToLabel(m.pair2), 0);

      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await gotoMatchViaCompCard(page, playoffId, m.id);
      await submitScore(page, oriented);

      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await gotoMatchViaPendingAction(page);
      await confirmScore(page);
    }

    // Final: A beats B
    const r2 = await getRoundMatches(page.request, suToken, playoffId, 2);
    expect(r2.length).toBe(1);
    const final = r2[0];
    expect(idToLabel(final.pair1)).toBe('A');
    expect(idToLabel(final.pair2)).toBe('B');

    const finalOriented = idToLabel(final.pair1) === 'A' ? '6-2 6-3' : orientScore('6-2 6-3', true);
    await loginAs(page, playerEmailForPair(idToLabel(final.pair1), 0), PLAYER_PASSWORD);
    await gotoMatchViaCompCard(page, playoffId, final.id);
    await submitScore(page, finalOriented);

    await loginAs(page, playerEmailForPair(idToLabel(final.pair2), 0), PLAYER_PASSWORD);
    await gotoMatchViaPendingAction(page);
    await confirmScore(page);

    const finalDone = await getMatchById(page.request, suToken, final.id);
    expect(idToLabel(finalDone.winner)).toBe('A');

    // =======================================================================
    // Phase 8: Assert standings + champion (P6)
    // =======================================================================

    await assertPlayoffChampion(page, playoffId, PAIRS[0].name);

    // =======================================================================
    // Teardown: assert fallback set matches expected (P4)
    // =======================================================================

    assertFallbacksMatch(collectFallbacks(), EXPECTED_FALLBACKS);
  });
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function createCompInactive(
  page: Page,
  name: string,
  type: 'league' | 'playoff',
  playTwice: boolean,
): Promise<string> {
  await page.getByRole('button', { name: /crear competición/i }).first().click();
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="type"]').selectOption(type);
  if (playTwice) {
    await dialog.locator('input[name="play_twice"]').check();
  }
  // Uncheck active (checked by default in the template)
  await dialog.locator('input[name="active"]').uncheck();
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));

  // Look up the competition ID via API
  const resp = await page.request.get(
    `/api/collections/competitions/records?filter=name='${name}'&perPage=1`,
    { headers: { Authorization: suToken } },
  );
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Competition not found after create: ${name}`);
  return id;
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
      pair1: m.pair1,
      pair2: m.pair2,
      pair1Label: p1Label,
      pair2Label: p2Label,
      orientedScore: orientScore(entry.score, flip),
    });
  }
  return fixtures;
}
