import { test, expect, Page } from '@playwright/test';
import { loginAs, isMobile, openDrawer, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

let suToken = '';

async function getSuperuserToken(page: Page) {
  if (suToken) return;
  const resp = await page.request.post('/api/collections/_superusers/auth-with-password', {
    data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
  });
  if (!resp.ok()) throw new Error(`Superuser auth failed: ${resp.status()}`);
  suToken = (await resp.json()).token;
}

const NAV_LABELS: Record<string, string> = {
  '/admin/competitions': 'Competiciones',
  '/admin/health': 'Salud',
  '/admin/players': 'Usuarios',
  '/admin/venues': 'Clubes',
  '/admin/invitations': 'Invitaciones',
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
    // Mobile card list and desktop table both render every venue name; only
    // one is visible per viewport (see the venue-creation test below).
    await expect(page.getByText('Pista Central').locator('visible=true').first()).toBeVisible();
  });

  test('admin can view invitations page', async ({ page }) => {
    // Invitations is a standalone global admin page (not competition-scoped
    // — InvitationsList has no competition filter), reached via the
    // "Invitaciones" nav link.
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/invitations');
    await expect(page.getByRole('heading', { name: 'Invitaciones' })).toBeVisible();
    await expect(page.getByRole('button', { name: /nueva invitaci[oó]n/i })).toBeVisible();
  });

  test('admin can view disputes page', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/disputes');
    await expect(page.getByRole('heading', { name: 'Disputas' })).toBeVisible();
  });

  test('admin can create invitation', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page, '/admin/invitations');
    await page.getByRole('button', { name: /nueva invitaci[oó]n/i }).click();
    const invEmail = `inv-${Date.now()}@test.com`;
    await page.locator('#modal-create-invite input[name="email"]').fill(invEmail);
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#modal-create-invite button[type="submit"]').click(),
    ]);
    await expect(page.getByText(invEmail).locator('visible=true').first()).toBeVisible({ timeout: 5000 });
  });

  test('admin can upload a competition logo', async ({ page }) => {
    // Competition-scoped invitations (and the register-hero-shows-logo path
    // this test used to exercise end-to-end) were removed in fb675f6:
    // invitations are global-only now, no admin UI sets invitations.competition
    // — that branch in auth.go is only reachable via a raw API write or
    // pre-refactor data, and is covered at the handler level by
    // TestRegisterPage_ShowsCompetitionName/ShowsCompetitionLogo. This test
    // keeps only the still-live part: the logo upload itself.
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
    // Known rough edge: LogoUpload responds with redirectHX (HX-Redirect),
    // which htmx turns into a full window.location navigation — this resets
    // the whole DOM and closes the edit modal (DaisyUI's checkbox-toggle
    // modal is opacity:0 when closed, not display:none, so a naive
    // toBeVisible before re-opening it would hang against a technically-
    // present-but-invisible element). Re-open the modal after the redirect
    // to see the uploaded logo.
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.setInputFiles('#comp-logo-input', {
        name: 'logo.jpg',
        mimeType: 'image/jpeg',
        buffer: logoJpeg,
      }),
    ]);
    await page.locator('label[for="edit-modal"]', { hasText: 'Editar' }).click();
    await expect(page.locator('img[src*="/logo/competition/"]').first()).toBeVisible({ timeout: 10000 });
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
    // Mobile card list and desktop table both render every venue name at
    // once (CSS-toggled per breakpoint, both present in the DOM) — same
    // dual-render pattern as the Penalizar trigger below.
    await expect(page.getByText(name).locator('visible=true').first()).toBeVisible({ timeout: 5000 });
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

    // Verify penalty modal is reachable from inside collapsed Parejas. The
    // desktop table's trigger is icon-only (aria-label, no text node); the
    // mobile dropdown's copy of the same label has visible text instead —
    // :visible picks whichever markup the current viewport actually shows
    // (see R-178, e2e/tests/presentation-guards.spec.ts).
    await parejas.locator('> input[type="checkbox"]').check({ force: true });
    await page.waitForTimeout(300);
    const penalizeBtn = parejas.locator('label[for^="penalty-modal-"]:visible').first();
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

  test('admin sees admin notification toggles', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/profile/notifications');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('Administrador')).toBeVisible();
    await expect(page.locator('input[name="match_progress"]')).toBeVisible();
    await expect(page.locator('input[name="admin_message"]')).toBeVisible();
    await expect(page.locator('input[name="user_joined"]')).toBeVisible();
    await expect(page.locator('input[name="message"]')).toBeVisible();
  });

  test('F2: activity log shows a verb phrase and Spanish labels for enum field changes', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');

    // Create via the real UI form — gender_type defaults to "free" here,
    // same as CompetitionHandler.Create, so the only field this test
    // changes afterward is gender_type.
    const name = `F2 Activity Log ${Date.now()}`;
    await page.getByRole('button', { name: 'Crear competición' }).first().click();
    await page.locator('#create-comp-name').fill(name);
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('#modal-create button[type="submit"]').click(),
    ]);
    await expect(page.locator('h1, h2', { hasText: name })).toBeVisible({ timeout: 10000 });

    await page.locator('label[for="edit-modal"]', { hasText: 'Editar' }).click();
    await page.waitForSelector('#edit-comp-gender', { state: 'visible' });
    await page.locator('#edit-comp-gender').selectOption('male');
    await Promise.all([
      page.waitForEvent('load', { timeout: 10000 }),
      page.locator('.modal-action button[type="submit"]').click(),
    ]);

    // The redirect may land back on the competition detail page or on the
    // list (see the redirect-target bug tracked separately) — navigate to
    // the competition explicitly so this test doesn't depend on which.
    await page.locator('.card-title, h1, h2', { hasText: name }).first().click().catch(() => {});
    await page.waitForLoadState('domcontentloaded');
    if (!page.url().includes('/admin/competitions/')) {
      await page.goto('/admin/competitions');
      await page.waitForLoadState('domcontentloaded');
      await page.locator('.card-title', { hasText: name }).first().click();
      await page.waitForLoadState('domcontentloaded');
    }

    const activityCard = page.locator('div.card', { has: page.getByRole('heading', { level: 2, name: 'Actividad' }) });
    await expect(activityCard).toBeVisible();
    // Verb phrase (not a bare field list) and the Spanish label "Masculina"
    // (not the raw stored value "male"), exactly as the edit form's own
    // <option> text reads.
    await expect(activityCard.getByText(/actualizó la configuración:.*Género: Libre → Masculina/)).toBeVisible();

    // Cleanup — the competitions collection's REST API is superuser-only
    // (see migrations/1725900000_lock_api_rules.go for the same lockdown
    // pattern on other collections), so the admin's own cookie session
    // can't delete it; use the same superuser-token pattern responsive.
    // spec.ts uses for its own teardown.
    const compId = page.url().split('/admin/competitions/')[1];
    if (compId) {
      await getSuperuserToken(page);
      await page.request.delete(`/api/collections/competitions/records/${compId}`, {
        headers: { Authorization: suToken },
      });
    }
  });

  test('admin can toggle admin notifications off and save', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/profile/notifications');
    await page.waitForLoadState('domcontentloaded');
    const toggle = page.locator('input[name="user_joined"]');
    await expect(toggle).toBeChecked();
    await toggle.uncheck();
    await page.click('button:has-text("Guardar")');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('input[name="user_joined"]')).not.toBeChecked();
    await toggle.check();
    await page.click('button:has-text("Guardar")');
  });
});
