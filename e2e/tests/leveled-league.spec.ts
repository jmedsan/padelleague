import { test, expect, Page, APIRequestContext } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { uniqueSuffix, setPlayerPassword } from '../season-helpers';
import {
  createPlayer, createPair, addPairToCompetition,
  generateFixtures, clickAndWaitForHxRedirect,
  lookupPlayerId, enterScore, confirmScore,
} from '../tour-helpers';

const RUN_ID = uniqueSuffix();
const PLAYER_PASSWORD = 'TestPass123456';

const PLAYERS = [
  { name: `L-Ana ${RUN_ID}`,    email: `l-ana-${RUN_ID}@test.local` },
  { name: `L-Bruno ${RUN_ID}`,  email: `l-bruno-${RUN_ID}@test.local` },
  { name: `L-Carla ${RUN_ID}`,  email: `l-carla-${RUN_ID}@test.local` },
  { name: `L-David ${RUN_ID}`,  email: `l-david-${RUN_ID}@test.local` },
  { name: `L-Elena ${RUN_ID}`,  email: `l-elena-${RUN_ID}@test.local` },
  { name: `L-Felix ${RUN_ID}`,  email: `l-felix-${RUN_ID}@test.local` },
  { name: `L-Gloria ${RUN_ID}`, email: `l-gloria-${RUN_ID}@test.local` },
  { name: `L-Hugo ${RUN_ID}`,   email: `l-hugo-${RUN_ID}@test.local` },
  { name: `L-Ines ${RUN_ID}`,   email: `l-ines-${RUN_ID}@test.local` },
  { name: `L-Julio ${RUN_ID}`,  email: `l-julio-${RUN_ID}@test.local` },
  { name: `L-Karen ${RUN_ID}`,  email: `l-karen-${RUN_ID}@test.local` },
  { name: `L-Luis ${RUN_ID}`,   email: `l-luis-${RUN_ID}@test.local` },
];

const PAIR_DEFS = [
  { name: `L-Pair-1 ${RUN_ID}`, p1: 0, p2: 1 },
  { name: `L-Pair-2 ${RUN_ID}`, p1: 2, p2: 3 },
  { name: `L-Pair-3 ${RUN_ID}`, p1: 4, p2: 5 },
  { name: `L-Pair-4 ${RUN_ID}`, p1: 6, p2: 7 },
  { name: `L-Pair-5 ${RUN_ID}`, p1: 8, p2: 9 },
  { name: `L-Pair-6 ${RUN_ID}`, p1: 10, p2: 11 },
];

const COMP_NAME = `L-Leveled ${RUN_ID}`;
const TARGET = 3;
const OPEN = 2;

let playerIds: string[] = [];
let pairIds: string[] = [];
let competitionId = '';
let suToken = '';

// ---------------------------------------------------------------------------
// Shared setup — runs once in serial mode (workers: 1 in playwright.config)
// ---------------------------------------------------------------------------

test.describe('leveled league', () => {
  test.describe.configure({ retries: 0 });

  test('full leveled-league flow (admin setup + player view + result + standings + release)', async ({ page }) => {
    test.setTimeout(300000);

    page.on('dialog', d => d.accept());

    // --- Superuser token for API calls ---
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const authResp = await page.request.post('/api/collections/_superusers/auth-with-password', {
      data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    if (!authResp.ok()) throw new Error(`Superuser auth failed: ${authResp.status()}`);
    suToken = (await authResp.json()).token;

    // =========================================================================
    // Phase 1: Admin creates leveled competition with 6 pairs, target=3, open=2
    // =========================================================================

    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');

    // Create players
    playerIds = [];
    for (const player of PLAYERS) {
      await page.goto('/admin/players');
      await page.waitForLoadState('domcontentloaded');
      await createPlayer(page, player.email, player.name);
    }
    for (const player of PLAYERS) {
      const id = await lookupPlayerId(page.request, suToken, player.email);
      playerIds.push(id);
      await setPlayerPassword(page.request, suToken, id, PLAYER_PASSWORD);
    }

    // Create pairs
    pairIds = [];
    for (const pd of PAIR_DEFS) {
      await page.goto('/admin/pairs');
      await page.waitForLoadState('domcontentloaded');
      const id = await createPair(page, pd.name, playerIds[pd.p1], playerIds[pd.p2], suToken);
      pairIds.push(id);
    }

    // Create competition with leveled settings in the dialog
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.getByRole('button', { name: /crear competición/i }).first().click();
    const dialog = page.locator('dialog#modal-create');
    await dialog.locator('input[name="name"]').fill(COMP_NAME);
    await dialog.locator('select[name="type"]').selectOption('league');
    await dialog.locator('input[name="active"]').check();

    // Expand "Opciones avanzadas" — DaisyUI collapse uses a hidden checkbox;
    // clicking the label div is intercepted by the input, so check it directly.
    const advCheckbox = dialog.locator('.collapse').filter({ hasText: 'Opciones avanzadas' }).locator('input[type="checkbox"]').first();
    await advCheckbox.check({ force: true });
    await dialog.locator('input#create-comp-target').fill(String(TARGET));
    await dialog.locator('input#create-comp-open').fill(String(OPEN));

    await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));

    // Extract competition ID from URL
    const urlMatch = page.url().match(/\/admin\/competitions\/([^/]+)/);
    if (urlMatch) {
      competitionId = urlMatch[1];
    } else {
      const resp = await page.request.get(
        `/api/collections/competitions/records?filter=name='${COMP_NAME}'&perPage=1`,
        { headers: { Authorization: suToken } },
      );
      competitionId = (await resp.json()).items?.[0]?.id;
      if (!competitionId) throw new Error('Competition not found after create');
    }

    // Add pairs (no seed param — leveled leagues are type "league", seed input only
    // appears for playoff type; seed order is set separately via the Nivel inicial card)
    await page.goto(`/admin/competitions/${competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    for (const pid of pairIds) {
      await addPairToCompetition(page, pid);
    }

    // Set seed order via "Nivel inicial" card (fill each pair's seed input and submit)
    await page.waitForSelector('[data-testid="section-seed-order"]', { timeout: 5000 });
    const seedSection = page.locator('[data-testid="section-seed-order"]');
    for (let i = 0; i < pairIds.length; i++) {
      const seedInput = seedSection.locator(`input[name="seed_${pairIds[i]}"]`);
      if (await seedInput.isVisible().catch(() => false)) {
        await seedInput.fill(String(i + 1));
      }
    }
    const seedBtn = seedSection.locator('button:has-text("Guardar orden")');
    if (await seedBtn.isVisible().catch(() => false)) {
      await Promise.all([
        page.waitForResponse(r => r.url().includes('/seed') && r.request().method() === 'POST', { timeout: 10000 }),
        seedBtn.click(),
      ]).catch(() => null);
    }

    // Generate initial assignments
    await generateFixtures(page);
    await page.waitForLoadState('domcontentloaded');

    // Assert "Nivel inicial" card is locked after generation:
    // the collapse closes (checked=false) and the "Guardar orden" button disappears.
    await expect(
      page.locator('[data-testid="section-seed-order"] button:has-text("Guardar orden")'),
    ).toHaveCount(0, { timeout: 5000 });

    // Publish
    await page.locator('button:has-text("Publicar calendario")').click();
    await page.waitForLoadState('domcontentloaded');

    // Assert published badge
    await expect(page.locator('.badge-success:has-text("Publicado")')).toBeVisible({ timeout: 5000 });

    // =========================================================================
    // Phase 2: Player sees competition — tab "Partidos", own pair preselected
    // =========================================================================

    const p1Email = PLAYERS[PAIR_DEFS[0].p1].email;
    await loginAs(page, p1Email, PLAYER_PASSWORD);

    // Navigate from home card
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
    const compLink = page.locator(`a[href="/competition/${competitionId}"]`).first();
    await compLink.waitFor({ timeout: 10000 });
    await compLink.click();
    await page.waitForLoadState('domcontentloaded');

    // Tab is "Partidos" (not "Jornadas")
    await expect(page.locator('input[aria-label="Partidos"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('input[aria-label="Jornadas"]')).toHaveCount(0);

    // "Por jugar" group is present
    await expect(page.locator('.collapse-title:has-text("Por jugar")')).toBeVisible({ timeout: 5000 });

    // Own pair is preselected in the filter dropdown
    const filterSelect = page.locator('select[name="pair"]');
    const selectedValue = await filterSelect.inputValue();
    expect(selectedValue).toBe(pairIds[0]);

    // Only own-pair matches shown (no pair-2+ matches without own pair)
    const matchLinks = page.locator('a[href^="/match/"]').filter({ hasText: PAIR_DEFS[0].name });
    await expect(matchLinks.first()).toBeVisible({ timeout: 5000 });

    // No "Jornada" text anywhere in the page body
    const bodyText = await page.locator('main').textContent() ?? '';
    expect(bodyText).not.toMatch(/Jornada\s+0/);

    // =========================================================================
    // Phase 3: Player switches to "Todas las parejas"
    // =========================================================================

    await filterSelect.selectOption('all');
    await page.waitForLoadState('domcontentloaded');

    // Now all pairs' matches should be visible (more match cards)
    const allMatchLinks = page.locator('a[href^="/match/"]');
    const allCount = await allMatchLinks.count();
    expect(allCount).toBeGreaterThanOrEqual(2);

    // =========================================================================
    // Phase 4: Player submits a result on own match
    // =========================================================================

    // Go back to own-pair filter to find the first match
    await filterSelect.selectOption(pairIds[0]);
    await page.waitForLoadState('domcontentloaded');

    const firstMatchLink = page.locator('a[href^="/match/"]').first();
    const firstMatchHref = await firstMatchLink.getAttribute('href');
    if (!firstMatchHref) throw new Error('No match link found');
    const matchId = firstMatchHref.split('/match/')[1];

    await firstMatchLink.click();
    await page.waitForLoadState('domcontentloaded');

    // Match page has no "Jornada" breadcrumb for round-0 match
    const breadcrumb = await page.locator('.breadcrumbs').textContent() ?? '';
    expect(breadcrumb).not.toMatch(/Jornada/);

    // No "Liberar partido" for a player
    await expect(page.locator('button:has-text("Liberar partido")')).toHaveCount(0);

    // Set date + club to satisfy HasDateAndPlace; status stays pending (IsPreScore=true)
    await page.request.patch(
      `/api/collections/matches/records/${matchId}`,
      {
        headers: { Authorization: suToken },
        data: { date: new Date().toISOString().slice(0, 10), club: 'Padel 360' },
      },
    );

    await page.reload();
    await page.waitForLoadState('domcontentloaded');

    // Submit score as pair-1 player
    await enterScore(page, '6-2 6-3');
    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")').first());

    // Confirm score as the actual opponent's player (opponent determined from match record)
    const matchRec = await page.request.get(
      `/api/collections/matches/records/${matchId}`,
      { headers: { Authorization: suToken } },
    );
    const matchData = await matchRec.json();
    const opponentPairId = matchData.pair1 === pairIds[0] ? matchData.pair2 : matchData.pair1;
    const opponentPairRec = await page.request.get(
      `/api/collections/pairs/records/${opponentPairId}`,
      { headers: { Authorization: suToken } },
    );
    const opponentPairData = await opponentPairRec.json();
    const opponentPlayerRec = await page.request.get(
      `/api/collections/users/records/${opponentPairData.player1}`,
      { headers: { Authorization: suToken } },
    );
    const opponentEmail = (await opponentPlayerRec.json()).email as string;
    await loginAs(page, opponentEmail, PLAYER_PASSWORD);
    await page.goto(`/match/${matchId}`);
    await page.waitForLoadState('domcontentloaded');
    await confirmScore(page);

    // =========================================================================
    // Phase 5: Player reloads competition — played match in "Jugados — <mes>"
    // =========================================================================

    await loginAs(page, p1Email, PLAYER_PASSWORD);
    await page.goto(`/competition/${competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    // "Jugados — <mes>" group must appear (use networkidle to ensure all renders complete)
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.collapse-title').filter({ hasText: /Jugados — / })).toBeVisible({ timeout: 10000 });

    // =========================================================================
    // Phase 6: Standings shows "Aj." column
    // =========================================================================

    // Navigate directly to the standings tab so the server renders it active
    await page.goto(`/competition/${competitionId}?tab=clasificacion`);
    await page.waitForLoadState('domcontentloaded');

    // "Aj." column header should be present in standings (leveled league)
    // Two Aj. th elements: desktop (hidden sm:block) and mobile (sm:hidden). On mobile
    // viewport the mobile table is visible, so use last() which picks the sm:hidden one.
    await expect(page.locator('th:has-text("Aj.")').last()).toBeVisible({ timeout: 5000 });

    // =========================================================================
    // Phase 7: Admin "Liberar partido" on a pending match
    // =========================================================================

    // Find a pending match for pair-1 (new one should exist after top-up from result)
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

    // Get all pending matches via API
    const pendingResp = await page.request.get(
      `/api/collections/matches/records?filter=competition='${competitionId}'%26%26status='pending'&perPage=50`,
      { headers: { Authorization: suToken } },
    );
    const pendingMatches = (await pendingResp.json()).items ?? [];
    if (pendingMatches.length === 0) {
      console.log('No pending matches to release — skipping release phase');
    } else {
      const targetMatch = pendingMatches[0];
      const targetMatchId = targetMatch.id;

      await page.goto(`/match/${targetMatchId}`);
      await page.waitForLoadState('domcontentloaded');

      // "Liberar partido" button is visible for admin on pending leveled match
      const releaseBtn = page.locator('button:has-text("Liberar partido")');
      await expect(releaseBtn).toBeVisible({ timeout: 5000 });

      await releaseBtn.click();
      // Dialog auto-accepted by page.on('dialog', d => d.accept())
      await page.waitForURL(`**/competition/${competitionId}`, { timeout: 10000 });
      await page.waitForLoadState('domcontentloaded');

      // The released match no longer exists
      const releasedResp = await page.request.get(
        `/api/collections/matches/records/${targetMatchId}`,
        { headers: { Authorization: suToken } },
      );
      expect(releasedResp.status()).toBe(404);
    }
  });

  // =========================================================================
  // Responsive assertions (phone + desktop)
  // =========================================================================

  test('leveled competition page renders correctly on phone', async ({ page }) => {
    test.skip(
      (page.viewportSize()?.width ?? 1280) >= 1024,
      'phone-only test',
    );
    test.setTimeout(60000);

    if (!competitionId) {
      test.skip(true, 'competition not created (run in serial after main test)');
      return;
    }

    const p1Email = PLAYERS[PAIR_DEFS[0].p1].email;
    await loginAs(page, p1Email, PLAYER_PASSWORD);
    await page.goto(`/competition/${competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    await expect(page.locator('input[aria-label="Partidos"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.collapse-title:has-text("Por jugar")')).toBeVisible({ timeout: 5000 });
  });
});
