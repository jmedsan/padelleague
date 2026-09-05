import { test, expect } from '@playwright/test';
import { loginAs, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test.describe('PWA offline support', () => {
  test('service worker precaches the app shell and offline fallback page', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.waitForFunction(() => navigator.serviceWorker.controller !== null, { timeout: 15000 });

    const cachedPaths = await page.evaluate(async () => {
      const names = await caches.keys();
      const paths: string[] = [];
      for (const n of names) {
        const cache = await caches.open(n);
        const reqs = await cache.keys();
        paths.push(...reqs.map((r) => new URL(r.url).pathname));
      }
      return paths;
    });

    for (const path of [
      '/static/css/styles.css',
      '/static/js/htmx.min.js',
      '/static/js/score-input.js',
      '/static/img/icon-192.png',
      '/static/img/icon-512.png',
      '/static/offline.html',
    ]) {
      expect(cachedPaths).toContain(path);
    }
  });

  test('offline navigation shows the Sin conexión fallback instead of a browser error', async ({ page, context }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.waitForFunction(() => navigator.serviceWorker.controller !== null, { timeout: 15000 });

    await context.setOffline(true);
    await page.goto('/player/does-not-matter').catch(() => {});
    await expect(page.getByText('Sin conexión', { exact: false })).toBeVisible({ timeout: 5000 });
    await context.setOffline(false);
  });
});
