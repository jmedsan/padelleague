import { test, expect, checkAll } from '../overflow-guard';
import {
  loginAs, loadTestData, clickAndWaitForHxRedirect,
  apiListRecords, apiDeleteRecord,
  ADMIN_EMAIL, ADMIN_PASSWORD,
} from '../helpers';

let suToken = '';

// Samsung Galaxy S23 (owner's real device) — the target mobile viewport for
// every admin edit form. Each of these forms is opened via an HTMX/JS swap
// into an existing modal, not a fresh page navigation, so the pageGuards
// auto-fixture's load/framenavigated listeners never see it — checkAll must
// be called explicitly once the modal is actually open, same idiom as
// scheduling-walkover.spec.ts's manual-context bracket check.
const MOBILE = { width: 360, height: 780 };

// 4x4 red JPEG, built in-memory — same fixture as admin-management.spec.ts's
// M10 sponsor-creation test, no file needed on disk. The sponsors collection
// requires a logo, so the create form (unlike the other entities here) must
// be driven through the UI rather than a plain API POST.
const SPONSOR_JPEG = Buffer.from(
  '/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSj/2wBDAQcHBwoIChMKChMoGhYaKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCj/wAARCAAEAAQDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDk6KKK8I/Vj//Z',
  'base64'
);

test.describe('admin edit forms at 360px', { tag: '@presentation' }, () => {
  test('venue edit form', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/venues');
    await page.waitForLoadState('domcontentloaded');

    const row = page.locator('li', { hasText: 'Pista Central' });
    await row.getByLabel('Más acciones').click();
    await row.getByText('Editar').click();
    const modal = page.locator('#edit-venue-modal');
    await expect(modal.getByRole('heading', { name: 'Editar club' })).toBeVisible();
    await expect(modal.locator('#edit-venue-name')).toHaveValue('Pista Central');
    const violations = await checkAll(page, true);
    expect(violations, 'venue edit form guard violations').toEqual([]);
  });

  test('player edit form', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/players');
    await page.waitForLoadState('domcontentloaded');

    const firstCard = page.locator('#players-cards > li').first();
    await firstCard.getByLabel('Más acciones').click();
    await firstCard.getByText('Editar').click();
    const modal = page.locator('#edit-player-modal');
    await expect(modal.getByRole('heading', { name: 'Editar jugador' })).toBeVisible();
    const violations = await checkAll(page, true);
    expect(violations, 'player edit form guard violations').toEqual([]);
  });

  test('pair edit form', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/pairs');
    await page.waitForLoadState('domcontentloaded');

    await page.locator('.card', { hasText: 'Pareja Alpha' }).getByLabel('Editar').click();
    const modal = page.locator('#modal-edit');
    await expect(modal.getByRole('heading', { name: 'Editar pareja' })).toBeVisible();
    const violations = await checkAll(page, true);
    expect(violations, 'pair edit form guard violations').toEqual([]);
  });

  test('sponsor edit form', async ({ page }) => {
    suToken = loadTestData().adminToken;
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/sponsors');
    await page.waitForLoadState('domcontentloaded');

    const sponsorName = `Sponsor Edit ${Date.now()}`;
    await page.getByRole('button', { name: /nuevo patrocinador/i }).click();
    await page.locator('#modal-create-sponsor input[name="name"]').fill(sponsorName);
    await page.setInputFiles('#modal-create-sponsor input[name="logo"]', {
      name: 'sponsor.jpg', mimeType: 'image/jpeg', buffer: SPONSOR_JPEG,
    });
    await page.locator('#modal-create-sponsor input[name="url"]').fill('https://example.com');
    await clickAndWaitForHxRedirect(page, page.locator('#modal-create-sponsor button[type="submit"]'), '/admin/sponsors');

    const sponsorCard = page.locator('[data-testid="sponsor-card"]', { hasText: sponsorName });
    await expect(sponsorCard).toBeVisible({ timeout: 5000 });

    await page.setViewportSize(MOBILE);
    await sponsorCard.getByLabel('Más acciones').click();
    await sponsorCard.getByText('Editar').click();
    const modal = page.locator('#edit-sponsor-modal');
    await expect(modal.getByRole('heading', { name: 'Editar patrocinador' })).toBeVisible();
    const violations = await checkAll(page, true);
    expect(violations, 'sponsor edit form guard violations').toEqual([]);

    const [sponsor] = await apiListRecords(page.request, suToken, 'sponsors', `name = "${sponsorName}"`);
    await apiDeleteRecord(page.request, suToken, 'sponsors', sponsor.id);
  });

  test('competition settings edit form', async ({ page }) => {
    const data = loadTestData();
    await page.setViewportSize(MOBILE);
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${data.competitionId}`);
    await page.waitForLoadState('domcontentloaded');

    await page.getByText('Editar', { exact: true }).locator('visible=true').first().click();
    await expect(page.getByRole('heading', { name: 'Editar competición' })).toBeVisible();
    const violations = await checkAll(page, true);
    expect(violations, 'competition settings edit form guard violations').toEqual([]);
  });
});
