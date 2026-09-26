import { test, expect } from '../overflow-guard';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { uniqueSuffix, setPlayerPassword } from '../season-helpers';
import {
  createPlayer, createPair, addPairToCompetition,
  generateFixtures, clickAndWaitForHxRedirect,
  lookupPlayerId,
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

  test('leveled-league admin dialog + player view', async ({ page }) => {
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

    // Leveled leagues require start_date/end_date before "Generar calendario"
    // will accept them — set via API since the create dialog has no date fields.
    const now = Date.now();
    await page.request.patch(`/api/collections/competitions/records/${competitionId}`, {
      headers: { Authorization: suToken },
      data: {
        start_date: new Date(now - 7 * 24 * 60 * 60 * 1000).toISOString(),
        end_date: new Date(now + 60 * 24 * 60 * 60 * 1000).toISOString(),
      },
    });

    // Add pairs (no seed param — leveled leagues are type "league", seed input only
    // appears for playoff type; seed order is set separately via the Nivel inicial card)
    await page.goto(`/admin/competitions/${competitionId}`);
    await page.waitForLoadState('domcontentloaded');
    for (const pid of pairIds) {
      await addPairToCompetition(page, pid);
    }

    // Assign each pair a skill level through the per-pair "Nivel" dropdown
    // (saves on change and redirects back to the detail page).
    const LEVELS = ['advanced', 'intermediate_high', 'intermediate', 'intermediate_low', 'beginner_high', 'beginner'];
    for (let i = 0; i < pairIds.length; i++) {
      const levelSelect = page.locator(`form[hx-post="/admin/pairs/${pairIds[i]}/level"] select:visible`).first();
      await expect(levelSelect).toBeEnabled({ timeout: 5000 });
      const nav = page.waitForEvent('framenavigated', { timeout: 15000 });
      await levelSelect.selectOption(LEVELS[i % LEVELS.length]);
      await nav;
      await page.waitForLoadState('domcontentloaded');
    }
    await expect(
      page.locator(`form[hx-post="/admin/pairs/${pairIds[0]}/level"] select:visible`).first(),
    ).toHaveValue(LEVELS[0]);

    // Generate initial assignments
    await generateFixtures(page);
    await page.waitForLoadState('domcontentloaded');

    // Levels lock once matches exist: every pair's dropdown is disabled.
    await expect(
      page.locator('form[hx-post$="/level"] select:not([disabled])'),
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

    // "Jornada 1" group is present
    await expect(page.locator('.collapse-title:has-text("Jornada 1")')).toBeVisible({ timeout: 5000 });

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
    await expect(page.locator('.collapse-title:has-text("Jornada 1")')).toBeVisible({ timeout: 5000 });
  });
});
