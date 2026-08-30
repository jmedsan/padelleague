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

  test('accent-folded search matches', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'notificacion');

    await expect(results.locator('a .text-sm.font-medium', { hasText: 'Notificaciones' })).toBeVisible({ timeout: 10000 });
  });

  test('player does not see admin-only entries', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'Pistas');

    await expect(results).toBeVisible({ timeout: 10000 });
    await expect(results.locator('a', { hasText: 'Pistas' })).not.toBeVisible({ timeout: 3000 });
    await expect(results.getByText('No se encontraron resultados')).toBeVisible();
  });

  test('admin sees admin-only entries', async ({ page }, testInfo) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'Pistas');

    await expect(results.locator('a', { hasText: 'Pistas' })).toBeVisible({ timeout: 10000 });
  });

  test('search result link resolves', async ({ page }, testInfo) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const results = await openSearchAndType(page, testInfo, 'Mi perfil');

    const link = results.locator('a[href*="/player/"]').first();
    await expect(link).toBeVisible({ timeout: 10000 });
    await link.click();
    await expect(page).toHaveURL(/\/player\//, { timeout: 10000 });
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
    await expect(results.locator('a', { hasText: 'Jugadores' })).toBeVisible();
  });
});
