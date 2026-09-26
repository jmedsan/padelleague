import { test, expect } from '@playwright/test';
import { loginAs, isMobile, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

// The mobile navbar must never overlap its own blocks: at 360px the role
// switch is icon-only so the "Liga Dale Fuerte" wordmark keeps its full
// width next to the hamburger and the bell.
test('navbar blocks do not overlap at 360px on admin pages', async ({ page }) => {
  test.skip(!isMobile(page), 'phone-only guard');
  await page.setViewportSize({ width: 360, height: 780 });
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto('/admin/competitions');
  await page.waitForLoadState('domcontentloaded');

  const info = await page.evaluate(() => {
    const nav = document.querySelector('.navbar')!;
    const wordmark = nav.querySelector('a[href="/"] span')!.getBoundingClientRect();
    const blocks = [...nav.querySelectorAll(':scope > div')]
      .filter(d => (d as HTMLElement).offsetParent !== null)
      .map(d => d.getBoundingClientRect())
      .map(r => [r.left, r.right]);
    return { wordmark: [wordmark.left, wordmark.right], blocks, width: window.innerWidth };
  });
  // Every visible block ends before the next one starts.
  for (let i = 1; i < info.blocks.length; i++) {
    expect(info.blocks[i][0], `block ${i} starts after block ${i - 1} ends`).toBeGreaterThanOrEqual(info.blocks[i - 1][1] - 1);
  }
  expect(info.wordmark[1]).toBeLessThanOrEqual(info.blocks[1][1]);
  expect(info.wordmark[1]).toBeLessThan(info.width);
  await expect(page.locator('.navbar button[aria-label^="cambiar vista"]')).toBeVisible();
  await expect(page.locator('.navbar button[aria-label^="cambiar vista"] span.hidden')).toHaveCount(1);
});
