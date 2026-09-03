import { test, expect, Page, APIRequestContext } from '@playwright/test';
import { loginAs, isMobile, navViaDrawer, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  setPlayerPassword, uniqueSuffix, SCORE_MATRIX, PENALTIES,
  computeExpected, PlannedMatch, PairId,
} from '../season-helpers';
import {
  createPlayer, createCompetition, createPair, addPairToCompetition, markAllPairsPaid,
  generateFixtures, submitScore, confirmScore,
  createDocument, attachDocumentToCompetition, acceptDocsGate,
  clickAndWaitForHxRedirect,
  assertFinalStandings, assertPlayoffChampion,
  lookupPlayerId, getRoundMatches, getMatchById, setMatchDateAndClub, acceptScheduleProposal,
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
// Nav-menu helper — viewport-aware (desktop Gestión dropdown / mobile drawer)
// ---------------------------------------------------------------------------

// Parejas and Documentos were removed from the Gestión menu (menu
// simplification: pairs/documents are managed from inside a competition's
// detail page now, not from a standalone admin list) — /admin/pairs and
// /admin/documents still exist as routes but are no longer nav-reachable,
// so those two are goto()'d directly rather than clicked. See
// .claude/steering/e2e-timing.md.
const NAV_HREFS: Record<string, string> = {
  'Competiciones': '/admin/competitions',
  'Jugadores': '/admin/players',
};

async function navTo(page: Page, label: string): Promise<void> {
  if (isMobile(page)) {
    const href = NAV_HREFS[label];
    if (!href) throw new Error(`navTo: unknown label "${label}"`);
    await navViaDrawer(page, href);
    return;
  }
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
  test.describe.configure({ retries: 0 });

  test('complete league + playoff via nav-menu navigation', async ({ page }) => {
    test.setTimeout(420000);

    // --- Dark mode is the default theme, before any login or toggle ---
    await page.goto('/login');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

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

    // --- Step 2: Create pairs — /admin/pairs has no nav-menu entry anymore
    // (pairs are managed from inside a competition's detail page); goto()
    // directly, the route itself is unchanged.
    pairIds = [];
    for (const pair of PAIRS) {
      await page.goto('/admin/pairs');
      await page.waitForLoadState('domcontentloaded');
      const pairId = await createPair(page, pair.name, playerIds[pair.p1], playerIds[pair.p2], suToken);
      pairIds.push(pairId);
    }

    // --- Step 3: Create league competition via Panel ---
    await navTo(page, 'Competiciones');
    competitionId = await createCompetition(page, COMP_NAME, 'league', { playTwice: true, suToken });

    // --- Step 4: Add pairs to competition ---
    // After createCompetition, navigate to the competition detail page.
    await navTo(page, 'Competiciones');
    await page.locator(`a:has-text("${COMP_NAME}")`).first().click();
    await page.waitForLoadState('domcontentloaded');

    // Before upload: no logo image anywhere in the header (red state).
    await expect(page.locator('img[src*="/api/files/competitions/"]')).toHaveCount(0);

    // Admin uploads a competition logo via the edit modal (4x4 red JPEG,
    // built in-memory — same approach as the avatar upload test, no fixture
    // file needed on disk).
    const logoJpeg = Buffer.from(
      '/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSj/2wBDAQcHBwoIChMKChMoGhYaKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCj/wAARCAAEAAQDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDk6KKK8I/Vj//Z',
      'base64'
    );
    await page.locator('label[for="edit-modal"]', { hasText: 'Editar' }).click();
    await page.waitForSelector('#comp-logo-input', { state: 'visible' });
    await page.setInputFiles('#comp-logo-input', {
      name: 'logo.jpg',
      mimeType: 'image/jpeg',
      buffer: logoJpeg,
    });
    // The upload's HX-Redirect triggers a full page load (not a Playwright
    // "navigation" event in the SPA sense) — wait for the resulting img
    // directly instead of an intermediate load-state signal.

    // After upload: the competition header shows the logo image (green state).
    await expect(page.locator('img[src*="/api/files/competitions/"]').first()).toBeVisible({ timeout: 10000 });

    for (const pairId of pairIds) {
      await addPairToCompetition(page, pairId);
    }

    // --- Step 5: Generate fixtures ---
    await generateFixtures(page);

    // A pair can't play without paying — mark all pairs paid.
    await markAllPairsPaid(page);

    // --- Step 5b: Admin creates + attaches mandatory doc, player passes gate ---
    // /admin/documents (the library) has no nav-menu entry anymore either —
    // goto() directly, same reasoning as Parejas above.
    await page.goto('/admin/documents');
    await page.waitForLoadState('domcontentloaded');
    await createDocument(page, 'Reglamento de prueba', 'https://example.com/reglamento', {
      mandatory: true,
    });

    // Doc-card actions must anchor to the title line and stack on mobile, not
    // vertically center on the card's variable height (a mandatory badge adds a row).
    // Assert the layout contract directly (deterministic): the card uses
    // sm:items-start + flex-col, which reverting to `items-center` removes.
    const docCard = page.locator('[data-testid="document-card"]', { hasText: 'Reglamento de prueba' }).first();
    await expect(docCard).toBeVisible();
    const cls = await docCard.getAttribute('class');
    expect(cls).toContain('sm:items-start'); // top-anchored, not items-center
    expect(cls).toContain('flex-col'); // stacks on mobile (title not truncated by inline actions)

    // Attach it to the league competition
    await navTo(page, 'Competiciones');
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

    // Pre-schedule (propose+accept, driven through the real thread handler
    // endpoints — see acceptScheduleProposal) pair A's first fixture so the
    // "Próximos partidos" section has a confirmed match to display.
    const pairAFixtures = fixtures.filter(f => f.pair1Label === 'A' || f.pair2Label === 'A');
    const scheduledFixture = pairAFixtures[0];
    const scheduledOtherLabel = scheduledFixture.pair1Label === 'A' ? scheduledFixture.pair2Label : scheduledFixture.pair1Label;
    const scheduledOtherEmail = playerEmailForPair(scheduledOtherLabel, 0);
    const scheduledOtherId = await lookupPlayerId(page.request, suToken, scheduledOtherEmail);
    // Real propose+accept flow, so the date must be today or later
    // (parseProposalForm in handlers/thread.go rejects past dates).
    const scheduleDate = new Date(Date.now() + 86400000).toISOString().slice(0, 10);
    const scheduleDateDisplay = scheduleDate.split('-').reverse().join('/');
    await acceptScheduleProposal(page, suToken, scheduledFixture.id, scheduledOtherId, scheduleDate, '18:00', 'Padel 360');

    // --- Notification shows the competition name ---
    // scheduledOtherEmail is the proposal author; after the accept above they
    // get a "Propuesta aceptada" notification carrying CompName. Reach the
    // history page the way a real user would (bell dropdown → "Ver todas"),
    // and assert the comp_name line inside that specific notification row,
    // not just page-wide text (which could match unrelated content).
    await loginAs(page, scheduledOtherEmail, PLAYER_PASSWORD);
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
    // Both mobile and desktop bell buttons exist in the DOM (breakpoint
    // classes just hide one); :visible picks whichever this viewport shows.
    await page.locator('button[aria-label="notificaciones"]:visible').click();
    await page.locator('a:has-text("Ver todas"):visible').click();
    await page.waitForLoadState('domcontentloaded');
    expect(page.url()).toContain('/notifications/history');

    const proposalAcceptedRow = page.locator('a', { hasText: 'Propuesta aceptada' }).first();
    await expect(proposalAcceptedRow).toBeVisible();
    await expect(proposalAcceptedRow).toContainText(COMP_NAME);

    // --- Upcoming matches section on player home ---
    await loginAs(page, PLAYERS[0].email, PLAYER_PASSWORD);
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
    const upcomingSection = page.locator('[data-testid="upcoming-matches"]');
    await expect(upcomingSection).toBeVisible();
    const upcomingRow = upcomingSection.locator('[data-testid="upcoming-match"]').first();
    await expect(upcomingRow).toBeVisible();
    await expect(upcomingRow).toContainText(scheduleDateDisplay);
    // The competition logo uploaded above must also render on the player's
    // home upcoming-match card, not just the admin competition header.
    await expect(upcomingRow.locator('img[src*="/api/files/competitions/"]')).toBeVisible();
    await upcomingRow.click();
    await page.waitForLoadState('domcontentloaded');
    expect(page.url()).toContain(`/match/${scheduledFixture.id}`);

    for (const f of fixtures) {
      const submitterEmail = playerEmailForPair(f.pair1Label, 0);
      const confirmerEmail = playerEmailForPair(f.pair2Label, 0);

      if (f.id !== scheduledFixture.id) {
        // Set date+club so score submission is enabled. The fixture
        // scheduled above already has date/club set by the real accept
        // handler (handlers/thread.go) — overwriting them here would
        // desync matches.date from the accepted proposal's date.
        await setMatchDateAndClub(page.request, suToken, f.id, '2025-03-15', 'Padel 360');
      }

      // Submitter logs in, navigates to match page
      await loginAs(page, submitterEmail, PLAYER_PASSWORD);
      await gotoMatchViaCompetition(page, f.id);
      await submitScore(page, f.orientedScore);

      // Opponent accepts the result proposal
      await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
      await gotoMatchViaCompetition(page, f.id);
      await confirmScore(page);

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
    await navTo(page, 'Competiciones');
    await page.locator(`a:has-text("${COMP_NAME}")`).first().click();
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

    // --- Step 9: Create playoff via Panel ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navTo(page, 'Competiciones');
    const playoffId = await createCompetition(page, PLAYOFF_NAME, 'playoff', { suToken });

    // Navigate to playoff detail page
    await navTo(page, 'Competiciones');
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
    // Multiple links to the competition now exist (stats table + one per
    // history row via competitionIdentity) — any one confirms the link.
    await expect(page.locator(`a[href="/competition/${competitionId}"]`).first()).toBeVisible();

    // Click player link → verify player page
    await page.locator(`table a[href="/player/${playerIds[0]}"]`).click();
    await page.waitForLoadState('domcontentloaded');
    expect(page.url()).toContain(`/player/${playerIds[0]}`);
    await expect(page.locator('h1')).toContainText(PLAYERS[0].name);

    // Level tile: this player has 6 finalized league matches (>= the 3-match
    // minimum, MinMatchesForLevel in league/stats.go), so the radial must
    // show a real number, not be hidden as "not enough data".
    const levelTile = page.locator('[data-testid="level-tile"]');
    await expect(levelTile).toBeVisible();
    const levelText = await page.locator('[data-testid="level-value"]').textContent();
    expect(levelText).toMatch(/^\d+(\.\d+)?$/);
    await expect(page.locator('body')).not.toContainText('Sin nivel');

    // Stats tiles heading distinguishes the aggregate stats from the
    // per-competition breakdown table below it.
    await expect(page.getByText('Todas las competiciones')).toBeVisible();

    // Match history table shows a "Competición" column so a player with
    // matches across multiple competitions can tell them apart (the
    // per-competition stats breakdown table has one too, hence .first()).
    await expect(page.locator('table th', { hasText: 'Competición' }).first()).toBeVisible();

    // --- Step 13: Double-role (R-150) — admin+player view switcher ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');

    if (isMobile(page)) {
      // Mobile: admin pill button visible
      const pill = page.locator('[aria-label="cambiar vista"]');
      await expect(pill).toBeVisible();

      // Switch to player view
      await pill.click();
      await page.locator('.dropdown-content a[href="/view/player"]').click();
      await page.waitForLoadState('domcontentloaded');
      await expect(page.locator('h1, h2').first()).toBeVisible();

      // Switch back to admin view
      const pillPlayer = page.locator('[aria-label="cambiar vista"]');
      await pillPlayer.click();
      await page.locator('.dropdown-content a[href="/view/admin"]').click();
      await page.waitForLoadState('domcontentloaded');
    } else {
      // Desktop: view-switcher dropdown visible
      const viewSwitcher = page.locator('.menu-horizontal details:has(a[href="/view/player"]) summary');
      await expect(viewSwitcher).toBeVisible();
      await expect(page.locator('summary:has-text("Gestión")')).toBeVisible();

      // Switch to player view
      const desktopNav = page.locator('.menu-horizontal');
      await viewSwitcher.click();
      await desktopNav.locator('a[href="/view/player"]').click();
      await page.waitForLoadState('domcontentloaded');
      await expect(page.locator('summary:has-text("Gestión")')).not.toBeVisible();
      await expect(page.locator('h1, h2').first()).toBeVisible();

      // Switch back to admin view
      const viewSwitcherPlayer = page.locator('.menu-horizontal details:has(a[href="/view/admin"]) summary');
      await viewSwitcherPlayer.click();
      await desktopNav.locator('a[href="/view/admin"]').click();
      await page.waitForLoadState('domcontentloaded');
      await expect(page.locator('summary:has-text("Gestión")')).toBeVisible();
    }
  });
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function gotoMatchViaCompetition(page: Page, matchId: string): Promise<void> {
  const url = `/match/${matchId}`;
  await page.goto(url);
  await page.waitForLoadState('domcontentloaded');
  // Doc gate may redirect to the competition page — accept and re-navigate
  if (await page.getByRole('heading', { name: 'Documentos obligatorios' }).isVisible().catch(() => false)) {
    await acceptDocsGate(page);
    await page.goto(url);
    await page.waitForLoadState('domcontentloaded');
  }
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

  await setMatchDateAndClub(page.request, suToken, match.id, '2025-03-15', 'Padel 360');

  const submitterEmail = playerEmailForPair(p1Label, 0);
  const confirmerEmail = playerEmailForPair(idToLabel(match.pair2), 0);

  await loginAs(page, submitterEmail, PLAYER_PASSWORD);
  await gotoMatchViaCompetition(page, match.id);
  await submitScore(page, oriented);

  await loginAs(page, confirmerEmail, PLAYER_PASSWORD);
  await gotoMatchViaCompetition(page, match.id);
  await confirmScore(page);
}
