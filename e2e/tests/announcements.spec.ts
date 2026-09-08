import { test, expect } from '@playwright/test';
import { loadTestData, loginAs, isMobile, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test.describe('announcements', () => {
  test('admin composes announcement, player reaches it via bell notification', async ({ page }) => {
    const data = loadTestData();
    const title = `E2E Anuncio ${Date.now()}`;
    const body = 'Este es el cuerpo del anuncio de prueba E2E.';

    // Admin composes the announcement on the competition detail page.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${data.competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    const composeForm = page.locator('form[hx-post*="/broadcast"]');
    await composeForm.locator('input[name="title"]').fill(title);
    await composeForm.locator('textarea[name="body"]').fill(body);
    await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/broadcast') && resp.status() === 204),
      composeForm.locator('button[type="submit"]').click(),
    ]);
    await page.waitForLoadState('domcontentloaded');

    // Clear toast confirms the broadcast was sent.
    await expect(page.locator('#flash-msg')).toContainText('Anuncio enviado');

    // Admin sees it listed in the Anuncios card.
    const adminCard = page.locator('[data-testid="announcement-card"]', { hasText: title });
    await expect(adminCard).toBeVisible({ timeout: 5000 });
    await expect(adminCard).toContainText(body);

    // Player logs in, opens the bell, clicks the announcement notification.
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');

    const mobile = isMobile(page);
    const bellButton = mobile
      ? page.locator('.lg\\:hidden .dropdown button[aria-label="notificaciones"]')
      : page.locator('.dropdown:has(#notif-dropdown) button[aria-label="notificaciones"]');
    await bellButton.click();

    const dropdown = mobile
      ? page.locator('.lg\\:hidden .dropdown')
      : page.locator('#notif-dropdown');
    const notifRow = dropdown.locator('a', { hasText: title });
    await expect(notifRow).toBeVisible({ timeout: 5000 });
    await notifRow.click();

    // Player lands on the competition page with the Anuncios tab checked.
    await page.waitForLoadState('domcontentloaded');
    await expect(page).toHaveURL(new RegExp(`/competition/${data.competitionId}#anuncios$`));
    const anunciosTab = page.locator('#tab-anuncios');
    await expect(anunciosTab).toBeChecked();

    // Player sees the announcement body.
    const playerCard = page.locator('[data-testid="announcement-card"]', { hasText: title });
    await expect(playerCard).toBeVisible({ timeout: 5000 });
    await expect(playerCard).toContainText(body);
  });
});
