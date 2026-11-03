import type { Browser, BrowserContext, Locator, Page } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, isMobile, openDrawer, suGet, suPatch, suDelete, leagueDate, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';
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

// ---- Player card ----------------------------------------------------------

interface Offer { id: string; title: string }

const EXTERNAL = 'https://stub.example/oferta';

// setOffers makes the given offers the only current ones: every existing offer
// is switched off through the superuser API, then each wanted offer is created
// through the live admin form (multipart POST), the same flow the admin uses.
async function setOffers(
  browser: Browser,
  baseURL: string | undefined,
  wanted: Array<{ title: string; url?: string; description?: string; expiresOn?: string; image?: boolean; active?: boolean }>,
): Promise<Offer[]> {
  const admin = await browser.newContext({ baseURL });
  const page = await admin.newPage();
  try {
    const token = loadTestData().adminToken;
    const all = await suGet(page.request, token, '/api/collections/promotions/records?perPage=200');
    for (const rec of all.items) {
      await suPatch(page.request, token, `/api/collections/promotions/records/${rec.id}`, { active: false });
    }
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const made: Offer[] = [];
    for (const w of wanted) {
      const multipart: Record<string, string | { name: string; mimeType: string; buffer: Buffer }> = {
        title: w.title,
        description: w.description ?? `Descripción de ${w.title}`,
        url: w.url ?? EXTERNAL,
      };
      if (w.expiresOn) multipart.expires_on = w.expiresOn;
      if (w.image) multipart.image = { name: 'promo.jpg', mimeType: 'image/jpeg', buffer: PROMO_JPEG };
      // raw-request: the admin multipart form post, 204 asserted below
      const resp = await page.request.post('/admin/promotions', { multipart, headers: { 'HX-Request': 'true' } });
      expect(resp.status(), `create ${w.title}`).toBe(204);
      const rec = (await suGet(page.request, token,
        `/api/collections/promotions/records?filter=${encodeURIComponent(`title="${w.title}"&&active=true`)}`)).items[0];
      if (w.active === false) {
        await suPatch(page.request, token, `/api/collections/promotions/records/${rec.id}`, { active: false });
      }
      made.push({ id: rec.id, title: w.title });
    }
    return made;
  } finally {
    await admin.close();
  }
}

async function promoRecord(page: Page, id: string): Promise<any> {
  return suGet(page.request, loadTestData().adminToken, `/api/collections/promotions/records/${id}`);
}

async function stubExternal(context: BrowserContext): Promise<void> {
  await context.route('https://stub.example/**', route =>
    route.fulfill({ status: 200, contentType: 'text/html', body: '<title>stub</title>' }));
}

// playerHome logs in as a player and waits for the card request of the home page.
async function playerHome(page: Page): Promise<void> {
  await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
  await page.goto('/');
  await page.waitForLoadState('networkidle');
}

// Asserts, in every theme, that the card is set apart from the page: its background differs, it has
// a border, and the tinted lime edge keeps at least 1.5:1 contrast with the body background. The
// edge is a brand tint, not a neutral line, so it does not reach WCAG 1.4.11's 3:1; the lime
// surface, the Patrocinado pill and the primary button carry the identification. The measured
// ratio is attached to the test report.
async function expectSeparatedFromPage(page: Page, target: Locator, label: string): Promise<void> {
  for (const theme of ['dark', 'padel']) {
    if (await page.locator('html').getAttribute('data-theme') !== theme) {
      await page.getByRole('button', { name: 'cambiar tema' }).click();
    }
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await page.waitForTimeout(400);
    const m = await target.evaluate(el => {
      const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true })!;
      const rgb = (c: string) => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); const [r, g, b, a] = ctx.getImageData(0, 0, 1, 1).data; return [r, g, b, a / 255]; };
      const lum = (c: number[]) => { const [r, g, b] = c.slice(0, 3).map(v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
      const over = (fg: number[], bg: number[]) => { const a = fg[3] ?? 1; return [0, 1, 2].map(i => fg[i] * a + bg[i] * (1 - a)); };
      const cs = getComputedStyle(el);
      const body = rgb(getComputedStyle(document.body).backgroundColor);
      const border = over(rgb(cs.borderTopColor), body);
      const [hi, lo] = [lum(border), lum(body)].sort((x, y) => y - x);
      return { bg: cs.backgroundColor, bodyBg: getComputedStyle(document.body).backgroundColor, width: cs.borderTopWidth, ratio: (hi + 0.05) / (lo + 0.05) };
    });
    test.info().annotations.push({ type: 'edge-contrast', description: `${label} ${theme}: ${m.ratio.toFixed(2)}` });
    expect(m.bg, `${label} (${theme}) background differs from the page`).not.toBe(m.bodyBg);
    expect(parseFloat(m.width), `${label} (${theme}) has a border`).toBeGreaterThan(0);
    expect(m.ratio, `${label} (${theme}) border contrast vs page`).toBeGreaterThanOrEqual(1.5);
  }
}

async function dismissed(page: Page): Promise<string | null> {
  return page.evaluate(() => localStorage.getItem('promo-dismissed'));
}

const reloadAndSettle = async (page: Page) => {
  await Promise.all([page.waitForResponse(r => r.url().includes('/promo/card')), page.reload()]);
};

test.describe.serial('promociones player card', { tag: '@promotions' }, () => {
  test('P1: Inicio shows the card with badge, title, description and image', async ({ browser, baseURL, page }) => {
    const [offer] = await setOffers(browser, baseURL, [{ title: 'Oferta P1', description: 'Descripción P1', image: true }]);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('.navbar a[href="/"]').first().click();
    const card = page.getByTestId('promo-card');
    await expect(card).toHaveAttribute('data-promo-id', offer.id);
    await expect(card.getByText('Patrocinado')).toBeVisible();
    await expect(card.getByRole('link', { name: 'Oferta P1' })).toBeVisible();
    await expect(card.getByText('Descripción P1')).toBeVisible();
    await expect(card.locator('img')).toBeVisible();
  });

  test('P2: the card shows on the competition page and not on a match page', async ({ browser, baseURL, page }) => {
    const [offer] = await setOffers(browser, baseURL, [{ title: 'Oferta P2' }]);
    const data = loadTestData();
    await playerHome(page);
    await page.locator(`a[href^="/competition/${data.competitionId}"]`).first().click();
    await page.waitForURL(`**/competition/${data.competitionId}**`);
    await expect(page.getByTestId('promo-card')).toHaveAttribute('data-promo-id', offer.id);
    const link = page.locator(`a[href="/match/${data.matchIds[0]}"]`).first();
    const round = page.locator('.collapse', { has: link }).locator('> input');
    if (await round.count()) await round.check();
    await link.click();
    await page.waitForURL(`**/match/${data.matchIds[0]}`);
    await expect(page.getByTestId('promo-slot')).toHaveCount(0);
  });

  test('P3: a dismissed offer never shows again while another is current', async ({ browser, baseURL, page }) => {
    const [a, b] = await setOffers(browser, baseURL, [{ title: 'Oferta P3A' }, { title: 'Oferta P3B' }]);
    await page.addInitScript(id => localStorage.setItem('promo-dismissed', JSON.stringify([id])), a.id);
    await playerHome(page);
    for (let i = 0; i < 10; i++) {
      await expect(page.getByTestId('promo-card')).toHaveAttribute('data-promo-id', b.id);
      await reloadAndSettle(page);
    }
  });

  test('P4: the X removes the card, it stays gone after a reload, and the ID is stored once', async ({ browser, baseURL, page }) => {
    const [offer] = await setOffers(browser, baseURL, [{ title: 'Oferta P4' }]);
    await playerHome(page);
    await page.getByRole('button', { name: 'descartar promoción' }).click();
    await expect(page.getByTestId('promo-card')).toHaveCount(0);
    expect(await dismissed(page)).toBe(JSON.stringify([offer.id]));
    await reloadAndSettle(page);
    await expect(page.getByTestId('promo-card')).toHaveCount(0);
    expect(await dismissed(page)).toBe(JSON.stringify([offer.id]));
  });

  test('P5: clicking the card opens the offer in a new tab, dismisses it and counts one click', async ({ browser, baseURL, page, context }) => {
    const [offer] = await setOffers(browser, baseURL, [{ title: 'Oferta P5' }]);
    await stubExternal(context);
    await playerHome(page);
    const [popup] = await Promise.all([
      context.waitForEvent('page'),
      page.getByTestId('promo-card').getByRole('link', { name: 'Oferta P5' }).click(),
    ]);
    await popup.waitForURL(EXTERNAL);
    await expect(page.getByTestId('promo-card')).toHaveCount(0);
    expect(await dismissed(page)).toBe(JSON.stringify([offer.id]));
    expect((await promoRecord(page, offer.id)).clicks).toBe(1);
  });

  test('P6: one reload adds exactly one view to the shown offer and none to the other', async ({ browser, baseURL, page }) => {
    const [a, b] = await setOffers(browser, baseURL, [{ title: 'Oferta P6A' }, { title: 'Oferta P6B' }]);
    await playerHome(page);
    const before = { [a.id]: (await promoRecord(page, a.id)).views, [b.id]: (await promoRecord(page, b.id)).views };
    await reloadAndSettle(page);
    const shown = await page.getByTestId('promo-card').getAttribute('data-promo-id');
    expect([a.id, b.id]).toContain(shown);
    for (const id of [a.id, b.id]) {
      expect((await promoRecord(page, id)).views, id).toBe(before[id] + (id === shown ? 1 : 0));
    }
  });

  test('P7: with no current offer the slot is empty and has no height', async ({ browser, baseURL, page }) => {
    await setOffers(browser, baseURL, []);
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await reloadAndSettle(page);
    const slot = page.getByTestId('promo-slot');
    await expect(slot).toBeEmpty();
    expect((await slot.boundingBox())?.height ?? 0).toBe(0);
  });

  test('P11: inactive and expired offers never show; one expiring today does', async ({ browser, baseURL, page }) => {
    const [off, old, today] = await setOffers(browser, baseURL, [
      { title: 'Oferta P11 apagada', active: false },
      { title: 'Oferta P11 vieja', expiresOn: leagueDate(-1) },
      { title: 'Oferta P11 hoy', expiresOn: leagueDate(0) },
    ]);
    await playerHome(page);
    for (let i = 0; i < 5; i++) {
      await expect(page.getByTestId('promo-card')).toHaveAttribute('data-promo-id', today.id);
      await reloadAndSettle(page);
    }
    expect((await promoRecord(page, off.id)).views).toBe(0);
    expect((await promoRecord(page, old.id)).views).toBe(0);

    // The admin sees both states in the list.
    await page.context().clearCookies();
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await openAdminPromotions(page);
    const offRow = page.locator(`[data-promo-id="${off.id}"]`);
    const oldRow = page.locator(`[data-promo-id="${old.id}"]`);
    await expect(offRow.getByText('Inactiva')).toBeVisible();
    await expect(oldRow.getByText('Expirada')).toBeVisible();
    await expect(page.locator(`[data-promo-id="${today.id}"]`).getByText('Expirada')).toHaveCount(0);
  });

  for (const stored of ['{', '"x"', '{"a":1}']) {
    test(`P13: invalid stored value ${stored} still shows the card without page errors`, async ({ browser, baseURL, page }) => {
      await setOffers(browser, baseURL, [{ title: 'Oferta P13' }]);
      const errors: string[] = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.addInitScript(v => localStorage.setItem('promo-dismissed', v), stored);
      await playerHome(page);
      await expect(page.getByTestId('promo-card')).toBeVisible();
      expect(errors).toEqual([]);
    });
  }

  test('P13: blocked localStorage still shows the card, and the X removes it for this page', async ({ browser, baseURL, page }) => {
    await setOffers(browser, baseURL, [{ title: 'Oferta P13 bloqueada' }]);
    const errors: string[] = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.addInitScript(() => {
      Object.defineProperty(window, 'localStorage', { get() { throw new DOMException('denied', 'SecurityError'); } });
    });
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/');
    await expect(page.getByTestId('promo-card')).toBeVisible();
    await page.getByRole('button', { name: 'descartar promoción' }).click();
    await expect(page.getByTestId('promo-card')).toHaveCount(0);
    expect(errors).toEqual([]);
  });

  test('P14: navigating home and back keeps one slot and one card, and a click stores the ID once', async ({ browser, baseURL, page, context }) => {
    const [offer] = await setOffers(browser, baseURL, [{ title: 'Oferta P14' }]);
    await stubExternal(context);
    const data = loadTestData();
    await playerHome(page);
    for (let i = 0; i < 2; i++) {
      await page.locator(`a[href^="/competition/${data.competitionId}"]`).first().click();
      await page.waitForURL(`**/competition/${data.competitionId}**`);
      await expect(page.getByTestId('promo-slot')).toHaveCount(1);
      await expect(page.getByTestId('promo-card')).toHaveCount(1);
      await page.getByRole('link', { name: '← Inicio' }).click();
      await page.waitForURL(/\/$/);
      await expect(page.getByTestId('promo-slot')).toHaveCount(1);
      await expect(page.getByTestId('promo-card')).toHaveCount(1);
    }
    const [popup] = await Promise.all([
      context.waitForEvent('page'),
      page.getByTestId('promo-card').getByRole('link', { name: 'Oferta P14' }).click(),
    ]);
    await popup.waitForURL(EXTERNAL);
    expect(await dismissed(page)).toBe(JSON.stringify([offer.id]));
  });

  test('P10c: a right click on an offer neither dismisses nor hides it', async ({ browser, baseURL, page, context }) => {
    const [a] = await setOffers(browser, baseURL, [{ title: 'Oferta P10C' }]);
    await stubExternal(context);
    await playerHome(page);
    const card = page.getByTestId('promo-card');
    await expect(card).toHaveAttribute('data-promo-id', a.id);
    await card.getByRole('link', { name: 'Oferta P10C' }).click({ button: 'right' });
    await expect(card).toBeVisible();
    expect(await dismissed(page)).toBeNull();
  });

  test('P10d: a middle click on an offer dismisses it and removes the card', async ({ browser, baseURL, page, context }) => {
    const [a] = await setOffers(browser, baseURL, [{ title: 'Oferta P10D' }]);
    await stubExternal(context);
    await playerHome(page);
    const card = page.getByTestId('promo-card');
    await expect(card).toHaveAttribute('data-promo-id', a.id);
    await card.getByRole('link', { name: 'Oferta P10D' }).click({ button: 'middle' });
    await expect(page.getByTestId('promo-card')).toHaveCount(0);
    expect(await dismissed(page)).toContain(a.id);
  });

  test('P15: a 100-character title and 300-character description fit at 360px in both themes', async ({ browser, baseURL, page }) => {
    await setOffers(browser, baseURL, [{ title: 'T'.repeat(100), description: 'D'.repeat(300), image: true }]);
    await page.setViewportSize({ width: 360, height: 780 });
    await playerHome(page);
    await expect(page.getByTestId('promo-card')).toBeVisible();
    for (const theme of ['dark', 'padel']) {
      if (await page.locator('html').getAttribute('data-theme') !== theme) {
        await page.getByRole('button', { name: 'cambiar tema' }).click();
      }
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow, theme).toBeLessThanOrEqual(0);
      const box = await page.getByTestId('promo-card').boundingBox();
      expect(box!.x + box!.width, theme).toBeLessThanOrEqual(360);
    }
  });
});

// ---- Navbar and Promociones page ------------------------------------------

test.describe.serial('promociones navbar and page', { tag: '@promotions' }, () => {
  test('P8: the player reaches Promociones from the navbar (icon on phone, link on desktop)', async ({ browser, baseURL, page }) => {
    await setOffers(browser, baseURL, [{ title: 'Oferta P8' }]);
    await playerHome(page);
    if (isMobile(page)) {
      await page.setViewportSize({ width: 360, height: 780 });
      const icon = page.getByRole('link', { name: 'promociones' });
      const box = await icon.boundingBox();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      const blocks = await page.evaluate(() =>
        [...document.querySelector('.navbar')!.children]
          .filter(d => (d as HTMLElement).offsetParent !== null)
          .map(d => { const r = d.getBoundingClientRect(); return [r.left, r.right]; }));
      for (let i = 1; i < blocks.length; i++) {
        expect(blocks[i][0], `block ${i} starts after block ${i - 1}`).toBeGreaterThanOrEqual(blocks[i - 1][1] - 1);
      }
      expect(blocks[blocks.length - 1][1]).toBeLessThanOrEqual(360);
      await icon.click();
    } else {
      await page.locator('.navbar').getByRole('link', { name: 'Promociones', exact: true }).click();
    }
    await page.waitForURL('**/promotions');
    await expect(page.getByRole('heading', { name: 'Promociones' })).toBeVisible();
    await expect(page.getByTestId('promo-card').getByRole('link', { name: 'Oferta P8' })).toBeVisible();
  });

  test('P22: the player card stands out from the page background in every theme, on Inicio and Promociones', async ({ browser, baseURL, page }) => {
    await setOffers(browser, baseURL, [{ title: 'Oferta P22' }]);
    await playerHome(page);
    await expectSeparatedFromPage(page, page.getByTestId('promo-card'), 'Inicio card');
    if (isMobile(page)) {
      await page.setViewportSize({ width: 360, height: 780 });
      await page.getByRole('link', { name: 'promociones' }).click();
    } else {
      await page.locator('.navbar').getByRole('link', { name: 'Promociones', exact: true }).click();
    }
    await page.waitForURL('**/promotions');
    await expectSeparatedFromPage(page, page.getByTestId('promo-card').first(), 'Promociones card');
  });

  test('P23: the card has the Patrocinado pill, a 64px tile and a primary CTA that counts a click', async ({ browser, baseURL, page, context }) => {
    const [offer] = await setOffers(browser, baseURL, [{ title: 'Oferta P23' }]);
    await stubExternal(context);
    await playerHome(page);
    const card = page.getByTestId('promo-card');
    const pill = card.getByTestId('promo-label');
    await expect(pill).toHaveText('Patrocinado');
    expect(parseFloat(await pill.evaluate(el => getComputedStyle(el).borderTopWidth))).toBeGreaterThan(0);
    expect(await pill.evaluate(el => getComputedStyle(el).borderRadius)).not.toBe('0px');
    const tile = await card.getByTestId('promo-tile').locator('> *').first().boundingBox();
    expect(Math.round(tile!.width)).toBe(64);
    expect(Math.round(tile!.height)).toBe(64);
    const cta = card.getByTestId('promo-cta');
    await expect(cta).toHaveText('Ver oferta →');
    await expect(cta).toHaveClass(/btn-primary/);
    await expect(cta).toHaveAttribute('href', new RegExp(`/promo/${offer.id}$`));
    expect((await cta.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    const [popup] = await Promise.all([context.waitForEvent('page'), cta.click()]);
    await popup.waitForURL(EXTERNAL);
    expect((await promoRecord(page, offer.id)).clicks).toBe(1);
  });

  test('P19: with no current offer the player has no Promociones entry in the navbar', async ({ browser, baseURL, page }) => {
    await setOffers(browser, baseURL, []);
    await playerHome(page);
    if (isMobile(page)) await page.setViewportSize({ width: 360, height: 780 });
    await expect(page.locator('.navbar')).toBeVisible();
    await expect(page.locator('.navbar a[href="/promotions"]')).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'promociones' })).toHaveCount(0);
    await setOffers(browser, baseURL, [{ title: 'Oferta P19' }]);
    await reloadAndSettle(page);
    await expect(page.locator('.navbar a[href="/promotions"]:visible')).toHaveCount(1);
  });

  test('P9: the admin view has no player entry, no card and no card request', async ({ browser, baseURL, page }) => {
    await setOffers(browser, baseURL, [{ title: 'Oferta P9' }]);
    const requests: string[] = [];
    page.on('request', r => { if (r.url().includes('/promo/card')) requests.push(r.url()); });
    const data = loadTestData();
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page.locator('.navbar a[href="/promotions"], .navbar a[aria-label^="promociones"]')).toHaveCount(0);
    await page.locator(`a[href="/admin/competitions/${data.competitionId}"]`).first().click();
    await page.locator(`a[href="/competition/${data.competitionId}"]:visible`).first().click();
    await expect(page).toHaveURL(new RegExp(`/competition/${data.competitionId}$`));
    await page.waitForLoadState('networkidle');
    await expect(page.getByTestId('promo-card')).toHaveCount(0);
    expect(requests).toEqual([]);
  });

  test('P10: a dismissed offer is still listed without X; opening one there hides it on Inicio', async ({ browser, baseURL, page, context }) => {
    const [a, b] = await setOffers(browser, baseURL, [{ title: 'Oferta P10A' }, { title: 'Oferta P10B' }]);
    await stubExternal(context);
    await page.addInitScript(id => { if (!localStorage.getItem('promo-dismissed')) localStorage.setItem('promo-dismissed', JSON.stringify([id])); }, a.id);
    await playerHome(page);
    if (isMobile(page)) {
      await page.getByRole('link', { name: 'promociones' }).click();
    } else {
      await page.locator('.navbar').getByRole('link', { name: 'Promociones', exact: true }).click();
    }
    await page.waitForURL('**/promotions');
    const cardA = page.locator(`[data-promo-id="${a.id}"]`);
    await expect(cardA).toBeVisible();
    await expect(page.getByRole('button', { name: 'descartar promoción' })).toHaveCount(0);
    const [popup] = await Promise.all([
      context.waitForEvent('page'),
      page.locator(`[data-promo-id="${b.id}"]`).getByRole('link', { name: 'Oferta P10B' }).click(),
    ]);
    await popup.waitForURL(EXTERNAL);
    expect(JSON.parse((await dismissed(page))!)).toEqual([a.id, b.id]);
    await expect(page.locator(`[data-promo-id="${b.id}"]`)).toBeVisible();
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await expect(page.getByTestId('promo-card')).toHaveCount(0);
  });

  test('P15: long title and description fit on the Promociones page at 360px', async ({ browser, baseURL, page }) => {
    await setOffers(browser, baseURL, [{ title: 'T'.repeat(100), description: 'D'.repeat(300), image: true }]);
    await page.setViewportSize({ width: 360, height: 780 });
    await playerHome(page);
    await page.getByRole('link', { name: 'promociones' }).click();
    await page.waitForURL('**/promotions');
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test('P16: a deleted offer left in localStorage is harmless and its old link says it is unavailable', async ({ browser, baseURL, page }, testInfo) => {
    testInfo.annotations.push({ type: 'expected-http-error', description: '404 /promo/' });
    const [gone, kept] = await setOffers(browser, baseURL, [{ title: 'Oferta P16 borrada' }, { title: 'Oferta P16 vigente' }]);
    await suDelete(page.request, loadTestData().adminToken, `/api/collections/promotions/records/${gone.id}`);
    await page.addInitScript(id => localStorage.setItem('promo-dismissed', JSON.stringify([id])), gone.id);
    await playerHome(page);
    await expect(page.getByTestId('promo-card')).toHaveAttribute('data-promo-id', kept.id);
    const resp = await page.goto(`/promo/${gone.id}`);
    expect(resp!.status()).toBe(404);
    await expect(page.getByText('Promoción no disponible')).toBeVisible();
  });
});

test.describe('promociones admin list layout', { tag: '@promotions' }, () => {
  test('P17: row menu items line up, titles line up with and without image, counters use singular', async ({ browser, baseURL, page }) => {
    const [plain, pictured] = await setOffers(browser, baseURL, [{ title: 'Oferta P17 sin imagen' }, { title: 'Oferta P17 con imagen', image: true }]);
    const su = loadTestData().adminToken;
    await suPatch(page.request, su, `/api/collections/promotions/records/${plain.id}`, { views: 1, clicks: 1 });
    await suPatch(page.request, su, `/api/collections/promotions/records/${pictured.id}`, { views: 2, clicks: 0 });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await openAdminPromotions(page);
    const rowPlain = page.locator(`[data-promo-id="${plain.id}"]`);
    const rowPictured = page.locator(`[data-promo-id="${pictured.id}"]`);

    // Counters: correct singular and plural.
    await expect(rowPlain).toContainText('1 vista');
    await expect(rowPlain).not.toContainText('1 vistas');
    await expect(rowPlain).toContainText('1 clic');
    await expect(rowPlain).not.toContainText('1 clics');
    await expect(rowPictured).toContainText('2 vistas');
    await expect(rowPictured).toContainText('0 clics');

    // Titles start at the same x whether or not the row has a thumbnail.
    const titleLeft = (row: typeof rowPlain) => row.locator('p.font-medium').evaluate(el => el.getBoundingClientRect().left);
    expect(await titleLeft(rowPlain)).toBe(await titleLeft(rowPictured));

    // Menu items: same icon x and same text x for every entry, all left-aligned.
    await rowPlain.getByLabel('Más acciones').click();
    const items = rowPlain.locator('.dropdown-content li > button, .dropdown-content li > form > button');
    await expect(items).toHaveCount(3);
    const geo = await items.evaluateAll(btns => btns.map(b => {
      const menu = b.closest('.dropdown-content')!.getBoundingClientRect();
      const text = [...b.childNodes].filter(n => n.nodeType === Node.TEXT_NODE && n.textContent!.trim()).pop()!;
      const range = document.createRange();
      range.selectNodeContents(text);
      return {
        label: text.textContent!.trim(),
        iconLeft: b.querySelector('svg')!.getBoundingClientRect().left,
        textLeft: range.getBoundingClientRect().left,
        menuLeft: menu.left,
      };
    }));
    expect(geo.map(g => g.label)).toEqual(['Pausar', 'Editar', 'Eliminar']);
    expect(new Set(geo.map(g => Math.round(g.iconLeft))).size, JSON.stringify(geo)).toBe(1);
    expect(new Set(geo.map(g => Math.round(g.textLeft))).size, JSON.stringify(geo)).toBe(1);
  });
  test('P18: a long URL wraps inside the admin row at 360px instead of being clipped', async ({ browser, baseURL, page }) => {
    const longUrl = 'https://www.playtomic.io/tenants/0123456789abcdef/courts/padel-pista-central-cubierta?utm_source=liga-dale-fuerte&utm_campaign=promociones';
    const [offer] = await setOffers(browser, baseURL, [{ title: 'Oferta P18', url: longUrl }]);
    await page.setViewportSize({ width: 360, height: 780 });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await openAdminPromotions(page);
    const row = page.locator(`[data-promo-id="${offer.id}"]`);
    const link = row.locator(`a[href="${longUrl}"]`);
    await expect(link).toHaveText(longUrl);
    const m = await link.evaluate(el => ({
      clipped: el.scrollWidth > el.clientWidth,
      right: el.getBoundingClientRect().right,
      lines: Math.round(el.getBoundingClientRect().height / parseFloat(getComputedStyle(el).lineHeight)),
      page: document.documentElement.scrollWidth > document.documentElement.clientWidth,
    }));
    expect(m.clipped, 'URL text is not clipped').toBe(false);
    expect(m.right, 'URL ends inside the 360px viewport').toBeLessThanOrEqual(360);
    expect(m.lines, 'URL wraps onto several lines').toBeGreaterThan(1);
    expect(m.page, 'page has no horizontal overflow').toBe(false);
  });

  test('P20: the views and clicks counters stay on one line at 360px', async ({ browser, baseURL, page }) => {
    const [offer] = await setOffers(browser, baseURL, [{ title: 'Oferta P20 con título largo para ocupar el ancho' }]);
    await page.setViewportSize({ width: 360, height: 780 });
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await openAdminPromotions(page);
    const labels = page.locator(`[data-promo-id="${offer.id}"] [data-testid="promo-counters"] > span`);
    await expect(labels).toHaveText(['0 vistas', '0 clics']);
    const tops = await labels.evaluateAll(els => els.map(el => Math.round(el.getBoundingClientRect().top)));
    expect(tops[0], 'both counters share one row').toBe(tops[1]);
  });
});
