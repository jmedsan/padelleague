import type { Page } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, isMobile, openDrawer, suGet, suPatch, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import { clickAndWaitForHxRedirect } from '../tour-helpers';
import { clickConfirmAndWaitForHxRedirect } from '../helpers';

// 4x4 red JPEG, built in-memory (no fixture file on disk).
const PROMO_JPEG = Buffer.from(
  '/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSj/2wBDAQcHBwoIChMKChMoGhYaKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCj/wAARCAAEAAQDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDk6KKK8I/Vj//Z',
  'base64'
);

// Reaches Gestión > Promociones by clicking the menu entry (drawer on phones).
async function openAdminPromotions(page: Page): Promise<void> {
  await page.goto('/');
  await page.waitForLoadState('domcontentloaded');
  if (isMobile(page)) {
    await openDrawer(page);
    await page.locator('.drawer-side a[href="/admin/promotions"]').click();
  } else {
    await page.locator('summary:has-text("Gestión")').click();
    await page.waitForTimeout(100);
    await page.locator('.menu-horizontal a[href="/admin/promotions"]').evaluate(el => (el as HTMLAnchorElement).click());
  }
  await page.waitForURL('**/admin/promotions');
  await page.waitForLoadState('domcontentloaded');
}

async function createPromo(page: Page, title: string, url: string): Promise<void> {
  await page.getByRole('button', { name: /nueva promoción/i }).click();
  const modal = page.locator('#modal-create-promotion');
  await modal.locator('input[name="title"]').fill(title);
  await modal.locator('textarea[name="description"]').fill('Descuento para jugadores');
  await modal.locator('input[name="url"]').fill(url);
  await page.setInputFiles('#modal-create-promotion input[name="image"]', {
    name: 'promo.jpg', mimeType: 'image/jpeg', buffer: PROMO_JPEG,
  });
  await modal.locator('button[type="submit"]').click();
}

async function promoByTitle(page: Page, title: string) {
  const items = await suGet(page.request, loadTestData().adminToken,
    `/api/collections/promotions/records?filter=${encodeURIComponent(`title="${title}"`)}`);
  return items.items[0];
}

test.describe.serial('promociones admin', { tag: '@promotions' }, () => {
  test('P12: admin creates with image, edits, toggles; bad link shows the error; counters match the API', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await openAdminPromotions(page);

    // A javascript: link is refused with the error inside the form.
    await createPromo(page, 'Promo E2E mala', 'javascript:alert(1)');
    await expect(page.getByRole('alert').filter({ hasText: 'El enlace' })).toContainText('El enlace debe empezar por http:// o https://');
    await expect(page.locator('[data-testid="promotion-row"]', { hasText: 'Promo E2E mala' })).toHaveCount(0);

    // Valid link: created with its image and listed as active.
    await page.locator('#modal-create-promotion input[name="url"]').fill('https://promo.example/oferta');
    await page.locator('#modal-create-promotion input[name="title"]').fill('Promo E2E');
    await clickAndWaitForHxRedirect(page, page.locator('#modal-create-promotion button[type="submit"]'), '/admin/promotions');
    const row = page.locator('[data-testid="promotion-row"]', { hasText: 'Promo E2E' });
    await expect(row).toBeVisible();
    await expect(row.getByText('Activa', { exact: true })).toBeVisible();
    await expect(row.locator('img')).toBeVisible();
    const rec = await promoByTitle(page, 'Promo E2E');
    expect(rec.image).not.toBe('');
    expect(rec.active).toBe(true);

    // Counters shown are the stored ones.
    await suPatch(page.request, loadTestData().adminToken, `/api/collections/promotions/records/${rec.id}`, { views: 7, clicks: 3 });
    await page.reload();
    await expect(row).toContainText('7 vistas');
    await expect(row).toContainText('3 clics');

    // Edit through the row menu.
    await row.getByLabel('Más acciones').click();
    await row.getByText('Editar').click();
    await page.locator('#edit-promotion-title').fill('Promo E2E editada');
    await clickAndWaitForHxRedirect(page, page.locator('#edit-promotion-modal button[type="submit"]'), '/admin/promotions');
    const edited = page.locator('[data-testid="promotion-row"]', { hasText: 'Promo E2E editada' });
    await expect(edited).toBeVisible();

    // Pause, then resume.
    await edited.getByLabel('Más acciones').click();
    await clickAndWaitForHxRedirect(page, edited.getByText('Pausar'), '/admin/promotions');
    await expect(edited.getByText('Inactiva')).toBeVisible();
    expect((await promoByTitle(page, 'Promo E2E editada')).active).toBe(false);
    await edited.getByLabel('Más acciones').click();
    await clickAndWaitForHxRedirect(page, edited.getByText('Reanudar'), '/admin/promotions');
    await expect(edited.getByText('Activa', { exact: true })).toBeVisible();
  });

  test('P16: admin deletes through the menu; cancel keeps it, confirm removes it', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await openAdminPromotions(page);
    const row = page.locator('[data-testid="promotion-row"]', { hasText: 'Promo E2E editada' });
    await expect(row).toBeVisible();

    await row.getByLabel('Más acciones').click();
    await row.getByText('Eliminar').click();
    await page.locator('#confirm-modal [data-confirm="cancel"]').click();
    await expect(row).toBeVisible();

    await row.getByLabel('Más acciones').click();
    await clickConfirmAndWaitForHxRedirect(page, row.getByText('Eliminar'), '/admin/promotions');
    await expect(row).toHaveCount(0);
    expect(await promoByTitle(page, 'Promo E2E editada')).toBeUndefined();
  });
});
