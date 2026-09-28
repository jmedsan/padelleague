import { test, expect, checkAll } from '../overflow-guard';

// Chromium turns touch emulation off for the page's life after a screenshot
// that captures beyond the viewport, so `(hover: none)` stops matching and
// input.css's 44px floor with it. The guard must name that cause instead of
// reporting every compliant control as a small tap target (the screens spec's
// mobile captures did exactly that). Its own touch context, as a real phone.
test('tap-target guard reports lost touch emulation, not the controls', { tag: '@presentation' }, async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 360, height: 780 }, isMobile: true, hasTouch: true });
  const page = await context.newPage();
  await page.goto('/login');
  await page.waitForLoadState('domcontentloaded');
  expect(await checkAll(page, true), 'login page under touch emulation').toEqual([]);

  await page.screenshot({ fullPage: true });

  const url = page.url();
  expect(await checkAll(page, true)).toEqual([
    `[no-touch-emulation] ${url}: (hover: none) doesn't match, so the 44px floor isn't active; a screenshot beyond the viewport turns touch emulation off`,
  ]);
  await context.close();
});
