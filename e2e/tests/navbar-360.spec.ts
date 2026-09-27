import { test, expect } from '../worker-server';
import { loginAs, isMobile, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

// The mobile navbar must never overlap its own blocks: at 360px the role
// switch is icon-only and the "Liga Dale Fuerte" wordmark wraps to two lines
// when it doesn't fit, so hamburger, wordmark, role switch, bell and theme
// toggle all keep their own room.
test('navbar blocks do not overlap at 360px on admin pages', { tag: '@presentation' }, async ({ page }) => {
  test.skip(!isMobile(page), 'phone-only guard');
  await page.setViewportSize({ width: 360, height: 780 });
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto('/admin/competitions');
  await page.waitForLoadState('domcontentloaded');

  const info = await page.evaluate(() => {
    const nav = document.querySelector('.navbar')!;
    const wordmark = nav.querySelector('a[href="/"] span')!.getBoundingClientRect();
    const roleSwitch = nav.querySelector('button[aria-label^="cambiar vista"]')!.getBoundingClientRect();
    const blocks = [...nav.children]
      .filter(d => (d as HTMLElement).offsetParent !== null)
      .map(d => d.getBoundingClientRect())
      .map(r => [r.left, r.right]);
    return { wordmark: [wordmark.left, wordmark.right], roleSwitchLeft: roleSwitch.left, blocks, width: window.innerWidth };
  });
  // Every visible block ends before the next one starts.
  for (let i = 1; i < info.blocks.length; i++) {
    expect(info.blocks[i][0], `block ${i} starts after block ${i - 1} ends`).toBeGreaterThanOrEqual(info.blocks[i - 1][1] - 1);
  }
  expect(info.wordmark[1]).toBeLessThanOrEqual(info.blocks[1][1]);
  expect(info.wordmark[1], 'wordmark text ends before the role switch').toBeLessThanOrEqual(info.roleSwitchLeft);
  expect(info.blocks[info.blocks.length - 1][1]).toBeLessThanOrEqual(info.width);
  await expect(page.locator('.navbar button[aria-label^="cambiar vista"]')).toBeVisible();
  await expect(page.locator('.navbar button[aria-label^="cambiar vista"] span.hidden')).toHaveCount(1);
});

// The theme toggle is one control in the top bar at every width; the drawer
// no longer carries a second "Cambiar tema" entry.
test('navbar theme button flips the theme at 360px; drawer has no theme entry', { tag: '@presentation' }, async ({ page }) => {
  test.skip(!isMobile(page), 'phone-only guard');
  await page.setViewportSize({ width: 360, height: 780 });
  await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
  await page.goto('/');
  await page.waitForLoadState('domcontentloaded');

  const html = page.locator('html');
  const before = await html.getAttribute('data-theme');
  const toggle = page.locator('.navbar button[aria-label="cambiar tema"]');
  await expect(toggle).toBeVisible();
  await toggle.tap();
  await expect(html).toHaveAttribute('data-theme', before === 'dark' ? 'padel' : 'dark');
  await toggle.tap();
  await expect(html).toHaveAttribute('data-theme', before!);

  await page.getByLabel('abrir menú').click();
  const drawer = page.locator('.drawer-side');
  await expect(drawer.getByRole('link', { name: 'Mi perfil' })).toBeVisible();
  await expect(drawer.getByText('Cambiar tema')).toHaveCount(0);
});
