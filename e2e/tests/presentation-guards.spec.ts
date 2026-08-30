import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test.describe('R-178: presentation quality guards', () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name === 'mobile', 'presentation guards use desktop nav');
  });

  test('dark-mode legibility: key containers and text are visible', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

    // Switch to dark theme
    await page.evaluate(() => {
      document.documentElement.setAttribute('data-theme', 'dark');
      localStorage.setItem('theme', 'dark');
    });

    // Navigate via click to admin competitions
    await page.locator('a:has-text("Inicio")').first().click();
    await page.waitForLoadState('networkidle');

    // Admin banner should be visible and not hot-pink (#f87272)
    const banner = page.locator('text=Modo administrador');
    await expect(banner).toBeVisible();
    const bannerBg = await banner.evaluate(el => {
      const div = el.closest('div')!;
      return getComputedStyle(div).backgroundColor;
    });
    expect(bannerBg).not.toContain('rgb(248, 114, 114)');

    // Theme attribute is 'dark' (not 'night')
    const theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'));
    expect(theme).toBe('dark');

    // Navigate to competitions via navbar
    await page.locator('a:has-text("Competiciones")').first().click();
    await page.waitForLoadState('networkidle');

    // Key headings and text are visible
    await expect(page.locator('h1:has-text("Panel de administración")')).toBeVisible();
    await expect(page.locator('text=Competiciones activas')).toBeVisible();

    // Click into a competition detail
    const compLink = page.locator('a[href^="/admin/competitions/"]').first();
    if (await compLink.count() > 0) {
      await compLink.click();
      await page.waitForLoadState('networkidle');
      // Section headings should be visible in dark mode
      const headings = page.locator('h2');
      const count = await headings.count();
      expect(count).toBeGreaterThan(0);
      for (let i = 0; i < Math.min(count, 5); i++) {
        await expect(headings.nth(i)).toBeVisible();
      }
    }
  });

  test('label-language sweep: no English leaks in visible page text', async ({ page }) => {
    // Known English words that should NOT appear in the Spanish UI
    const englishLeaks = [
      /\bSubmit\b/i, /\bCancel\b/i, /\bDelete\b/i, /\bSave\b/i,
      /\bSettings\b/i, /\bProfile\b/i, /\bPassword\b/i,
      /\bHome\b(?!page)/i, /\bSearch\b/i, /\bLogout\b/i,
      /\bLogin\b(?!As)/i, /\bPlayers\b/i, /\bMatches\b/i,
      /\bStandings\b/i, /\bSchedule\b/i, /\bResults\b/i,
      /\bNotifications\b/i, /\bDocuments\b/i, /\bVenues\b/i,
    ];

    // Check as player (most common user)
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a:has-text("Inicio")').first().click();
    await page.waitForLoadState('networkidle');

    const bodyText = await page.evaluate(() => document.body.innerText);

    for (const pattern of englishLeaks) {
      const match = bodyText.match(pattern);
      expect(match, `English leak found: "${match?.[0]}" in page text`).toBeNull();
    }

    // Check admin page too
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.locator('a:has-text("Competiciones")').first().click();
    await page.waitForLoadState('networkidle');

    const adminText = await page.evaluate(() => document.body.innerText);
    for (const pattern of englishLeaks) {
      const match = adminText.match(pattern);
      expect(match, `English leak on admin page: "${match?.[0]}"`).toBeNull();
    }
  });

  test('grep-gate: tour-guided has no goto(url) to driven actions', async () => {
    const src = readFileSync(join(__dirname, 'tour-guided.spec.ts'), 'utf-8');
    // Extract all goto() calls
    const gotoPattern = /\.goto\(['"`]([^'"`]+)['"`]\)/g;
    let match: RegExpExecArray | null;
    const violations: string[] = [];

    while ((match = gotoPattern.exec(src)) !== null) {
      const url = match[1];
      // Allowed: '/' (home entry), login, and template-literal entry points
      if (url === '/' || url === '/login') continue;
      // Dynamic competition/match URLs used for doc-gate or API-backed actions
      // are acceptable if they can't be reached via click (e.g. first-time player
      // entering a competition). Flag any static admin/player page URL.
      if (/^\/(admin|match|competition|players|pairs|venues|invitations|disputes)$/.test(url)) {
        violations.push(url);
      }
    }

    expect(violations, `tour-guided should not goto() static pages: ${violations.join(', ')}`).toHaveLength(0);
  });

  test('non-empty panels: urgent tasks and standings render content', async ({ page }) => {
    // Login as player who has match data in seed
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a:has-text("Inicio")').first().click();
    await page.waitForLoadState('networkidle');

    // Check that "Mis competiciones" section has at least one card
    const compCards = page.locator('.card').filter({ hasText: /Liga|Playoff|competici/i });
    const cardCount = await compCards.count();
    expect(cardCount, 'player home should show at least one competition card').toBeGreaterThan(0);

    // Navigate to a competition via click and check standings table is non-empty
    const compLink = page.locator('a[href^="/competition/"]').first();
    if (await compLink.count() > 0) {
      await compLink.click();
      await page.waitForLoadState('networkidle');

      // If docs gate appears, accept it
      const docsGate = page.getByRole('heading', { name: 'Documentos obligatorios' });
      if (await docsGate.isVisible().catch(() => false)) {
        // Accept all docs
        const acceptBtns = page.locator('button:has-text("He leído")');
        const btnCount = await acceptBtns.count();
        for (let i = 0; i < btnCount; i++) {
          await acceptBtns.nth(i).click();
          await page.waitForTimeout(300);
        }
        const confirmBtn = page.locator('button:has-text("Confirmar")');
        if (await confirmBtn.isVisible().catch(() => false)) {
          await confirmBtn.click();
          await page.waitForLoadState('networkidle');
        }
      }

      // Click "Clasificación" tab to reveal standings
      const standingsTab = page.locator('input[aria-label="Clasificación"]');
      if (await standingsTab.count() > 0) {
        await standingsTab.click();
        await page.waitForTimeout(300);
        const standingsRows = page.locator('table tbody tr');
        expect(await standingsRows.count(), 'standings table should have rows').toBeGreaterThan(0);
        await expect(standingsRows.first()).toBeVisible();
      }
    }

    // Admin: check competitions list is non-empty
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.locator('a:has-text("Competiciones")').first().click();
    await page.waitForLoadState('networkidle');

    const adminCards = page.locator('a[href^="/admin/competitions/"]');
    expect(await adminCards.count(), 'admin should see at least one competition').toBeGreaterThan(0);
  });

  test('R-164: date-format guard — no raw ISO dates in visible text', async ({ page }) => {
    // ISO date patterns that should NEVER appear in rendered UI text
    const isoLeaks = [
      /\d{4}-\d{2}-\d{2}T/,                  // RFC3339 with T separator
      /00:00:00\.000Z/,                        // PB midnight timestamp suffix
      /\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/, // raw PB datetime
    ];
    // Spanish date format: DD/MM/YYYY or DD/MM/YYYY HH:MM
    const spanishDate = /\d{2}\/\d{2}\/\d{4}/;

    // Check player home (has dates in next match, proposed dates, etc.)
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a:has-text("Inicio")').first().click();
    await page.waitForLoadState('networkidle');

    let bodyText = await page.evaluate(() => document.body.innerText);

    for (const pattern of isoLeaks) {
      const match = bodyText.match(pattern);
      expect(match, `ISO date leak on player home: "${match?.[0]}"`).toBeNull();
    }

    // Navigate to a competition and check dates there
    const compLink = page.locator('a[href^="/competition/"]').first();
    if (await compLink.count() > 0) {
      await compLink.click();
      await page.waitForLoadState('networkidle');

      // Accept docs gate if present
      const docsGate = page.getByRole('heading', { name: 'Documentos obligatorios' });
      if (await docsGate.isVisible().catch(() => false)) {
        const acceptBtns = page.locator('button:has-text("He leído")');
        const btnCount = await acceptBtns.count();
        for (let i = 0; i < btnCount; i++) {
          await acceptBtns.nth(i).click();
          await page.waitForTimeout(300);
        }
        const confirmBtn = page.locator('button:has-text("Confirmar")');
        if (await confirmBtn.isVisible().catch(() => false)) {
          await confirmBtn.click();
          await page.waitForLoadState('networkidle');
        }
      }

      bodyText = await page.evaluate(() => document.body.innerText);
      for (const pattern of isoLeaks) {
        const match = bodyText.match(pattern);
        expect(match, `ISO date leak on competition page: "${match?.[0]}"`).toBeNull();
      }
    }

    // Check admin competition detail (has round dates, match dates)
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.locator('a:has-text("Competiciones")').first().click();
    await page.waitForLoadState('networkidle');
    const adminComp = page.locator('a[href^="/admin/competitions/"]').first();
    if (await adminComp.count() > 0) {
      await adminComp.click();
      await page.waitForLoadState('networkidle');
      bodyText = await page.evaluate(() => document.body.innerText);
      for (const pattern of isoLeaks) {
        const match = bodyText.match(pattern);
        expect(match, `ISO date leak on admin detail: "${match?.[0]}"`).toBeNull();
      }
    }
  });
});
