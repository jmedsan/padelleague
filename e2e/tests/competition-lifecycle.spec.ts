import { test, expect, Page, APIRequestContext } from '@playwright/test';
import { loginAs, loadTestData, isMobile, openDrawer, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

let suToken = '';

async function getSuToken(request: APIRequestContext): Promise<string> {
  if (suToken) return suToken;
  const resp = await request.post('/api/collections/_superusers/auth-with-password', {
    data: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
  });
  suToken = (await resp.json()).token;
  return suToken;
}

async function navToAdmin(page: Page, href: string): Promise<void> {
  if (isMobile(page)) {
    await openDrawer(page);
    await page.locator(`.drawer-side a[href="${href}"]`).click();
  } else {
    await page.locator('summary:has-text("Gestión")').click();
    await page.waitForTimeout(100);
    await page.locator(`.menu-horizontal a[href="${href}"]`).evaluate(el => (el as HTMLAnchorElement).click());
  }
  await page.waitForLoadState('domcontentloaded');
}

test.describe('competition lifecycle', () => {
  test('admin single-active entry redirects to competition detail', async ({ page }) => {
    const token = await getSuToken(page.request);
    // Deactivate any extra active competitions (left by prior tests)
    const resp = await page.request.get('/api/collections/competitions/records?filter=active%3Dtrue&perPage=50', {
      headers: { Authorization: token },
    });
    const activeComps = (await resp.json()).items || [];
    const extras = activeComps.filter((c: any) => c.name !== 'Liga E2E Test');
    for (const c of extras) {
      await page.request.patch(`/api/collections/competitions/records/${c.id}`, {
        headers: { Authorization: token, 'Content-Type': 'application/json' },
        data: { active: false },
      });
    }

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    // N=1 active: /admin redirects to competition detail
    await navToAdmin(page, '/admin');
    await page.waitForLoadState('domcontentloaded');
    await expect(page).toHaveURL(/\/admin\/competitions\//);
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();

    // Restore deactivated competitions
    for (const c of extras) {
      await page.request.patch(`/api/collections/competitions/records/${c.id}`, {
        headers: { Authorization: token, 'Content-Type': 'application/json' },
        data: { active: true },
      });
    }
  });

  test('admin dashboard shows title and competitions', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: 'Competiciones' })).toBeVisible();
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
  });

  test('admin can create a new competition', async ({ page, }, testInfo) => {
    const name = `Liga Nueva ${testInfo.project.name} ${Date.now()}`;
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.getByRole('button', { name: /crear competición/i }).first().click();
    await page.fill('input[name="name"]', name);
    await page.selectOption('select[name="type"]', 'league');
    await page.locator('dialog button[type="submit"]').click();
    await page.waitForURL(/\/admin/, { timeout: 10000 });
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText(name)).toBeVisible({ timeout: 10000 });
  });

  test('player can view competition standings', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    await page.locator('input[aria-label="Clasificación"]').click();
    const standingsTable = page.locator('table.table-zebra');
    await expect(standingsTable).toBeVisible({ timeout: 5000 });
    await expect(standingsTable.locator('td', { hasText: 'Pareja Alpha' })).toBeVisible();
    await expect(standingsTable.locator('td', { hasText: 'Pareja Beta' })).toBeVisible();
  });

  test('competition page shows match fixtures with mine-only default', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('input[aria-label="Jornadas"]')).toBeVisible();
    await expect(page.getByText(/Jornada \d/).first()).toBeVisible();
    // Mine-only: player's own matches visible
    const matchLinks = page.locator('a[href^="/match/"]');
    const count = await matchLinks.count();
    expect(count).toBeGreaterThan(0);
    await expect(matchLinks.first()).toContainText('Pareja Alpha');
    // Toggle shows mine-only/all selector
    const toggle = page.locator('[data-testid="mine-only-toggle"]');
    await expect(toggle).toBeVisible();
    await expect(toggle.getByText('Mis partidos')).toBeVisible();
    await expect(toggle.getByText('Todos')).toBeVisible();
  });

  test('admin can view competition detail', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/competitions');
    await page.waitForLoadState('domcontentloaded');
    await page.locator('a.card', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByText('Liga E2E Test').first()).toBeVisible();
    const body = await page.textContent('body');
    expect(body).toContain('Pareja Alpha');
  });
});
