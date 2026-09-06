import { test, expect, Page, TestInfo } from '@playwright/test';
import { loginAs, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

async function openSearchAndType(page: Page, testInfo: TestInfo, query: string) {
  const isMobile = testInfo.project.name === 'mobile';
  const searchInput = isMobile
    ? page.locator('.drawer-side input[name="q"]')
    : page.locator('#global-search');
  const results = isMobile
    ? page.locator('#search-results-mobile #search-results')
    : page.locator('#search-results-dropdown #search-results');

  if (isMobile) {
    await page.getByLabel('abrir menú').click();
    await expect(searchInput).toBeVisible({ timeout: 5000 });
  }

  await searchInput.click();
  await searchInput.pressSequentially(query, { delay: 30 });
  await expect(results).toBeVisible({ timeout: 10000 });

  return results;
}

test.describe('global search', () => {
  test('typo search finds correct result and screenshot', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'clasif');

    await expect(results.locator('.text-sm.font-medium', { hasText: 'Clasificación' })).toBeVisible({ timeout: 10000 });

    await page.screenshot({
      path: `screenshots/search-typo-${testInfo.project.name}.png`,
    });
  });

  test('type badge does not repeat per row — group heading is the only type label', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'clasif');

    const row = results.locator('a', { hasText: 'Clasificación' });
    await expect(row).toBeVisible({ timeout: 10000 });
    // The group heading above already names the type; the row itself must
    // carry no separate type badge.
    await expect(row.locator('.badge')).toHaveCount(0);
  });

  test('accent-folded search matches', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'notificacion');

    await expect(results.locator('a .text-sm.font-medium', { hasText: 'Preferencias' })).toBeVisible({ timeout: 10000 });
  });

  test('player does not see admin-only entries', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'Configuración');

    await expect(results).toBeVisible({ timeout: 10000 });
    await expect(results.locator('a', { hasText: 'Configuración' })).not.toBeVisible({ timeout: 3000 });
    await expect(results.getByText('No se encontraron resultados')).toBeVisible();
  });

  test('admin sees admin-only entries', async ({ page }, testInfo) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'Configuración');

    // Match by URL: a competition can legitimately be named with "configuración"
    // in its own label (e.g. "... — configuración pendiente"), which also
    // matches a loose hasText/name filter alongside the actual settings-page link.
    await expect(results.locator('a[href="/admin/settings"]')).toBeVisible({ timeout: 10000 });
  });

  test('search result link resolves', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'Mi perfil');

    const link = results.locator('a[href*="/player/"]').first();
    await expect(link).toBeVisible({ timeout: 10000 });
    await link.click();
    await expect(page).toHaveURL(/\/player\//, { timeout: 10000 });
  });

  test('clicking a recent search loads results into the dropdown, not a bare page', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);

    // First search records the query in search_history.
    const firstResults = await openSearchAndType(page, testInfo, 'clasif');
    await expect(firstResults.locator('.text-sm.font-medium', { hasText: 'Clasificación' })).toBeVisible({ timeout: 10000 });

    // Re-open the search box at zero query to see it listed as recent.
    const isMobile = testInfo.project.name === 'mobile';
    const searchInput = isMobile
      ? page.locator('.drawer-side input[name="q"]')
      : page.locator('#global-search');
    const results = isMobile
      ? page.locator('#search-results-mobile #search-results')
      : page.locator('#search-results-dropdown #search-results');

    await searchInput.fill('');
    await searchInput.blur();
    await searchInput.click();
    await expect(results).toBeVisible({ timeout: 10000 });
    await expect(results.getByText('Búsquedas recientes')).toBeVisible({ timeout: 10000 });

    const recentButton = results.locator('button', { hasText: 'clasif' });
    await expect(recentButton).toBeVisible();
    await recentButton.click();

    // Clicking a recent search must populate the dropdown in place — the URL
    // must not navigate to the bare, unstyled /search partial.
    await expect(results.locator('.text-sm.font-medium', { hasText: 'Clasificación' })).toBeVisible({ timeout: 10000 });
    expect(page.url()).not.toContain('/search?q=');
  });

  test('zero-query panel shows quick-nav with links', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const isMobile = testInfo.project.name === 'mobile';
    const searchInput = isMobile
      ? page.locator('.drawer-side input[name="q"]')
      : page.locator('#global-search');
    const results = isMobile
      ? page.locator('#search-results-mobile #search-results')
      : page.locator('#search-results-dropdown #search-results');

    if (isMobile) {
      await page.getByLabel('abrir menú').click();
      await expect(searchInput).toBeVisible({ timeout: 5000 });
    }

    await searchInput.click();
    await expect(results).toBeVisible({ timeout: 10000 });

    await expect(results.getByText('Ir a')).toBeVisible();
    await expect(results.locator('a', { hasText: 'Inicio' })).toBeVisible();
    await expect(results.locator('a', { hasText: 'Mi perfil' })).toBeVisible();
  });

  test('zero-query admin panel shows admin nav links', async ({ page }, testInfo) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const isMobile = testInfo.project.name === 'mobile';
    const searchInput = isMobile
      ? page.locator('.drawer-side input[name="q"]')
      : page.locator('#global-search');
    const results = isMobile
      ? page.locator('#search-results-mobile #search-results')
      : page.locator('#search-results-dropdown #search-results');

    if (isMobile) {
      await page.getByLabel('abrir menú').click();
      await expect(searchInput).toBeVisible({ timeout: 5000 });
    }

    await searchInput.click();
    await expect(results).toBeVisible({ timeout: 10000 });

    await expect(results.locator('a', { hasText: 'Disputas' })).toBeVisible();
    await expect(results.locator('a', { hasText: 'Usuarios' })).toBeVisible();
  });
});
