import { test, expect, Page } from '@playwright/test';
import { loginAs, isMobile, openDrawer, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

const NAV_LABELS: Record<string, string> = {
  '/admin/competitions': 'Competiciones',
  '/admin/health': 'Salud',
  '/admin/players': 'Jugadores',
  '/admin/venues': 'Clubes',
};

async function navToAdmin(page: Page, href: string): Promise<void> {
  if (NAV_LABELS[href]) {
    if (isMobile(page)) {
      await openDrawer(page);
      await page.locator(`.drawer-side a[href="${href}"]`).click();
    } else {
      await page.locator('summary:has-text("Gestión")').click();
      await page.waitForTimeout(100);
      const link = page.locator(`.menu-horizontal a:has-text("${NAV_LABELS[href]}")`);
      await link.evaluate(el => (el as HTMLAnchorElement).click());
    }
  } else {
    await page.goto(href);
  }
  await page.waitForLoadState('domcontentloaded');
}

test.describe('admin management', () => {
  test('admin can view pairs page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/pairs');
    await expect(page.getByText('Pareja Alpha')).toBeVisible();
    await expect(page.getByText('Pareja Beta')).toBeVisible();
  });

  test('admin can view players page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/players');
    await expect(page.getByText('Test Player', { exact: true }).locator('visible=true').first()).toBeVisible();
    await expect(page.getByText('Test Player 2', { exact: true }).locator('visible=true').first()).toBeVisible();
  });

  test('admin can view venues page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/venues');
    await expect(page.getByText('Pista Central')).toBeVisible();
  });

  test('admin can view invitations inline on competition detail', async ({ page }) => {
    // Invitations moved from a standalone /admin/invitations page into the
    // competition detail page (like Documentos) — reached via a real click
    // on a competition card, not goto(url).
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.locator('.card-title', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: 'Invitaciones' })).toBeVisible();
    await expect(page.getByRole('button', { name: /nueva invitaci[oó]n/i })).toBeVisible();
  });

  test('admin can view disputes page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/disputes');
    await expect(page.getByRole('heading', { name: 'Disputas' })).toBeVisible();
  });

  test('admin can create invitation from competition detail', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.locator('.card-title', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await page.getByRole('button', { name: /nueva invitaci[oó]n/i }).click();
    const invEmail = `inv-${Date.now()}@test.com`;
    await page.locator('#modal-create-invite input[name="email"]').fill(invEmail);
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#modal-create-invite button[type="submit"]').click(),
    ]);
    await expect(page.getByText(invEmail).locator('visible=true').first()).toBeVisible({ timeout: 5000 });
  });

  test('register hero shows competition name and logo from an invitation link', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.locator('.card-title', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');

    await page.locator('label[for="edit-modal"]', { hasText: 'Editar' }).click();
    await page.waitForSelector('#comp-logo-input', { state: 'visible' });

    // 4x4 red JPEG, built in-memory — same approach as the logo upload tour
    // assertion, no fixture file needed on disk.
    const logoJpeg = Buffer.from(
      '/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSj/2wBDAQcHBwoIChMKChMoGhYaKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCj/wAARCAAEAAQDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDk6KKK8I/Vj//Z',
      'base64'
    );
    await page.setInputFiles('#comp-logo-input', {
      name: 'logo.jpg',
      mimeType: 'image/jpeg',
      buffer: logoJpeg,
    });
    await expect(page.locator('img[src*="/api/files/competitions/"]').first()).toBeVisible({ timeout: 10000 });

    // Create an invitation and read its register link straight off the
    // "Copiar" button's onclick attribute (copyInviteLink(token, this)) —
    // no clipboard permission dance needed.
    const invEmail = `inv-hero-${Date.now()}@test.com`;
    await page.getByRole('button', { name: /nueva invitaci[oó]n/i }).click();
    await page.locator('#modal-create-invite input[name="email"]').fill(invEmail);
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#modal-create-invite button[type="submit"]').click(),
    ]);
    const inviteRow = page.locator('tr, div.py-3', { hasText: invEmail }).locator('visible=true').first();
    await expect(inviteRow).toBeVisible({ timeout: 5000 });
    const copyBtn = inviteRow.locator('button[title="Copiar enlace"]');
    const onclick = await copyBtn.getAttribute('onclick');
    const token = onclick?.match(/copyInviteLink\('([^']+)'/)?.[1];
    expect(token).toBeTruthy();

    // Open the actual register link, as an invited (logged-out) player would.
    // Clear the session directly rather than clicking "Salir" — on mobile
    // that button lives in the off-canvas drawer (not display:none, so
    // :visible can't disambiguate it from the desktop navbar's copy), and
    // this test only needs a logged-out browser, not to exercise logout UI.
    await page.context().clearCookies();
    await page.goto(`/register?token=${token}`);
    await page.waitForLoadState('domcontentloaded');

    await expect(page.getByText('Liga E2E Test te invita a')).toBeVisible();
    // Scoped to main: the site footer now also renders this competition's
    // logo (single active competition, out-of-context promotion), so the
    // unscoped selector matches both and violates Playwright's strict mode.
    await expect(page.locator('main img[src*="/api/files/competitions/"]')).toBeVisible();
  });

  test('admin can create venue', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/venues');
    const name = `Club ${Date.now()}`;
    await page.getByRole('button', { name: /nuevo club/i }).click();
    await page.locator('#modal-create-venue input[name="name"]').fill(name);
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#modal-create-venue button[type="submit"]').click(),
    ]);
    await expect(page.getByText(name)).toBeVisible({ timeout: 5000 });
  });

  test('R-168: competition detail sections are collapsed accordions when started', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    // Navigate to dashboard, then click into the first competition
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.locator('a[href^="/admin/competitions/"]').first().click();
    await page.waitForLoadState('networkidle');

    // Verify accordion sections exist with collapse class
    const parejas = page.locator('[data-testid="section-parejas"]');
    await expect(parejas).toBeVisible();
    await expect(parejas).toHaveClass(/collapse/);

    const addPairs = page.locator('[data-testid="section-add-pairs"]');
    await expect(addPairs).toBeVisible();
    await expect(addPairs).toHaveClass(/collapse/);

    // Add pairs and round dates should be collapsed (not checked) when started
    const addPairsCheckbox = addPairs.locator('> input[type="checkbox"]');
    await expect(addPairsCheckbox).not.toBeChecked();

    // Verify penalty modal is reachable from inside collapsed Parejas
    await parejas.locator('> input[type="checkbox"]').check({ force: true });
    await page.waitForTimeout(300);
    const penalizeBtn = parejas.locator('label:has-text("Penalizar")').first();
    if (await penalizeBtn.count() > 0) {
      const modalId = await penalizeBtn.getAttribute('for');
      await penalizeBtn.click();
      await page.waitForTimeout(300);
      const toggle = page.locator(`#${modalId}`);
      await expect(toggle).toBeChecked();
    }
  });

  test('R-226: gender_type field is a dropdown with Spanish labels', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.getByRole('button', { name: /crear competici[oó]n/i }).first().click();
    const dialog = page.locator('dialog#modal-create');
    await expect(dialog).toBeVisible();
    const select = dialog.locator('select[name="gender_type"]');
    await expect(select).toBeVisible();
    const options = await select.locator('option').allTextContents();
    expect(options).toContain('Libre');
    expect(options).toContain('Mixta');
    expect(options).toContain('Masculina');
    expect(options).toContain('Femenina');
  });

  test('M10: sponsor created, attached, and shown in the scoped competition footer', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);

    // 1. Create a sponsor via the admin/sponsors page.
    await page.goto('/admin/sponsors');
    await page.waitForLoadState('domcontentloaded');
    const sponsorName = `Test Sponsor ${Date.now()}`;
    await page.getByRole('button', { name: /nuevo patrocinador/i }).click();
    await page.locator('#modal-create-sponsor input[name="name"]').fill(sponsorName);

    // 4x4 red JPEG, built in-memory — same approach as the competition logo
    // upload tour assertion above, no fixture file needed on disk.
    const sponsorJpeg = Buffer.from(
      '/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSj/2wBDAQcHBwoIChMKChMoGhYaKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCj/wAARCAAEAAQDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDk6KKK8I/Vj//Z',
      'base64'
    );
    await page.setInputFiles('#modal-create-sponsor input[name="logo"]', {
      name: 'sponsor.jpg',
      mimeType: 'image/jpeg',
      buffer: sponsorJpeg,
    });
    await page.locator('#modal-create-sponsor input[name="url"]').fill('https://example.com');
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#modal-create-sponsor button[type="submit"]').click(),
    ]);

    // 2. The sponsor row appears in the library list.
    const sponsorCard = page.locator('[data-testid="sponsor-card"]', { hasText: sponsorName });
    await expect(sponsorCard).toBeVisible({ timeout: 5000 });

    // 3. Navigate to the competition detail page and attach the sponsor.
    await page.goto('/admin/competitions');
    await page.locator('.card-title', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    const attachSponsorForm = page.locator('form:has(select[name="sponsor"])');
    await attachSponsorForm.locator('select[name="sponsor"]').selectOption({ label: sponsorName });
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      attachSponsorForm.getByRole('button', { name: /adjuntar/i }).click(),
    ]);
    await expect(
      page.locator('[data-testid="sponsor-attach-card"]', { hasText: sponsorName })
    ).toBeVisible({ timeout: 5000 });

    // 4. Reload the competition detail page — its footer is scoped to this
    // competition (FooterCompetitionID), so the assertions below fail if
    // either the attach or the footer rendering regresses.
    await page.reload();
    await page.waitForLoadState('domcontentloaded');

    // 5. The footer shows the sponsor as a linked, alt-labeled image.
    const sponsorLink = page.locator('footer a', { has: page.locator(`img[alt="${sponsorName}"]`) });
    await expect(sponsorLink).toBeVisible();
    await expect(sponsorLink).toHaveAttribute('href', 'https://example.com');

    // 6. The footer shows the competition identity (logo + name), not just
    // the ambient "active competitions" list — proof the footer is scoped.
    const compInFooter = page.locator('footer a', { has: page.locator('img[alt="Liga E2E Test"], :text("Liga E2E Test")') });
    await expect(compInFooter.first()).toBeVisible();

    // 7. A 404-style page (unmatched record) still renders the footer, but
    // without this competition's identity — the out-of-context shape.
    await page.goto('/match/does-not-exist');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('footer')).toBeVisible();
    await expect(page.locator('footer', { hasText: sponsorName })).toHaveCount(0);
  });
});
