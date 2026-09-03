import { test, expect, Page } from '@playwright/test';
import { loginAs, isMobile, openDrawer, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

async function navToAdmin(page: Page): Promise<void> {
  if (isMobile(page)) {
    await openDrawer(page);
    await page.locator('.drawer-side a[href="/admin/competitions"]').click();
  } else {
    await page.locator('summary:has-text("Gestión")').click();
    await page.waitForTimeout(100);
    await page.locator('.menu-horizontal a[href="/admin/competitions"]').evaluate(
      el => (el as HTMLAnchorElement).click()
    );
  }
  await page.waitForLoadState('domcontentloaded');
}

test.describe('gender enforcement', () => {
  test('same-gender pair rejected from mixed competition', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await navToAdmin(page);

    // Create a mixed competition via the admin UI
    const compName = `Test Mixta ${Date.now()}`;
    await page.getByRole('button', { name: /crear competición/i }).first().click();
    const modal = page.locator('#modal-create');
    await expect(modal).toBeVisible();
    await modal.locator('input[name="name"]').fill(compName);
    await modal.locator('select[name="gender_type"]').selectOption('mixed');
    await modal.locator('button[type="submit"]').click();
    // Create redirects straight to the new competition's detail page.
    await page.waitForURL(/\/admin\/competitions\/[^/]+$/, { timeout: 10000 });
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: compName })).toBeVisible({ timeout: 5000 });

    // Expand "Añadir parejas" accordion
    const addSection = page.locator('[data-testid="section-add-pairs"]');
    await addSection.locator('input[type="checkbox"]').check({ force: true });
    await page.waitForTimeout(300);

    // Select a pair (seed players are all male — rejected in a mixed competition)
    const pairSelect = addSection.locator('select[name="pair"]');
    await expect(pairSelect).toBeVisible();
    const options = await pairSelect.locator('option:not([value=""])').all();
    expect(options.length).toBeGreaterThan(0);
    const firstValue = await options[0].getAttribute('value');
    await pairSelect.selectOption(firstValue!);

    // Click "Añadir pareja"
    await addSection.locator('button:has-text("Añadir")').click();

    // AddPair's rejection is a flash alert (HX-Retarget swaps #flash, not the
    // form's own hx-target), so the message lands in the global flash slot.
    const result = page.locator('#flash');
    await expect(result).toContainText('parejas mixtas deben tener un jugador y una jugadora', { timeout: 5000 });
  });
});
