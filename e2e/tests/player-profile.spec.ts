import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, isMobile, openDrawer, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

const BASE = `http://localhost:${process.env.E2E_PORT || 8099}`;
const FRESH_PLAYER_PASSWORD = 'TestPass123456';

function suToken(): string {
  return loadTestData().adminToken;
}

async function suPost(path: string, data: Record<string, unknown>): Promise<any> {
  const resp = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: suToken() },
    body: JSON.stringify(data),
  });
  if (!resp.ok) throw new Error(`suPost ${path}: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

// createFreshPlayer makes a player who has never uploaded an avatar, so the
// "placeholder" assertion below is not racing the mobile tour project (which
// runs first in the shared suite and uploads an avatar for PLAYER1).
async function createFreshPlayer(label: string): Promise<{ email: string; password: string }> {
  const email = `avatar-${label}-${Date.now()}@test.local`;
  await suPost('/api/collections/users/records', {
    email, display_name: `Foto Test ${label}`,
    gender: 'male', roles: ['player'],
    password: FRESH_PLAYER_PASSWORD, passwordConfirm: FRESH_PLAYER_PASSWORD,
    verified: true,
  });
  return { email, password: FRESH_PLAYER_PASSWORD };
}

test.describe('player profile and stats', () => {
  test('player can view own profile with stats', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await page.locator('input[aria-label="Clasificación"]').click();
    // standingsTable.html renders a desktop table.table-zebra and a mobile
    // table.table-sm, each hidden at the other breakpoint via CSS.
    const standingsTableClass = isMobile(page) ? 'table.table-sm' : 'table.table-zebra';
    const pairLink = page.locator(`${standingsTableClass} a[href^="/pair/"]`).first();
    await expect(pairLink).toBeVisible({ timeout: 5000 });
    await pairLink.click();
    await page.waitForLoadState('domcontentloaded');
    const playerLink = page.locator('table a[href^="/player/"]').first();
    await expect(playerLink).toBeVisible();
    await playerLink.click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: /Test Player/ })).toBeVisible();
    await expect(page.locator('.stat-title', { hasText: 'Partidos' })).toBeVisible();
    await expect(page.locator('.stat-title', { hasText: '% Victorias' })).toBeVisible();
    await expect(page.getByText('Parejas')).toBeVisible();
    await expect(page.getByText('Pareja Alpha').first()).toBeVisible();
  });

  test('player sees notification prefs with message toggle but no admin section', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    if (isMobile(page)) {
      await openDrawer(page);
      await page.locator('.drawer-side a[href="/profile/notifications"]').click();
    } else {
      await page.goto('/profile/notifications');
    }
    await page.waitForLoadState('domcontentloaded');
    await expect(page.getByRole('heading', { name: /Preferencias de notificaciones/i })).toBeVisible();
    await expect(page.locator('input[name="message"]')).toBeVisible();
    await expect(page.getByText('Administrador')).not.toBeVisible();
  });

  test('player can toggle message notifications off and save', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto('/profile/notifications');
    await page.waitForLoadState('domcontentloaded');
    const toggle = page.locator('input[name="message"]');
    await expect(toggle).toBeChecked();
    await toggle.uncheck();
    await page.click('button:has-text("Guardar")');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('input[name="message"]')).not.toBeChecked();
    await toggle.check();
    await page.click('button:has-text("Guardar")');
  });

  test('notification count loads', async ({ page }) => {
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await expect(page.locator('[hx-get="/notifications/count"]').first()).toBeAttached({ timeout: 5000 });
  });

  test('player can upload their own avatar photo', async ({ page }) => {
    // Fresh player: the shared seed's PLAYER1 already has an avatar by the
    // time mobile runs before desktop (mobile tour uploads one), so the
    // "starts as placeholder" assertion needs a player this test fully
    // controls, per season-simulation.spec.ts's pattern of not trusting
    // shared seed state.
    const player = await createFreshPlayer('upload');
    await loginAs(page, player.email, player.password);
    if (isMobile(page)) {
      await openDrawer(page);
      await page.locator('.drawer-side a', { hasText: 'Mi perfil' }).click();
    } else {
      // Desktop nav links the display name in the navbar to the player's
      // own profile (views/layout.html) — click it as a real affordance.
      // loginAs already lands on / (helpers.ts setAuthCookie navigates and
      // asserts the URL), so no extra goto is needed here.
      await page.getByRole('link', { name: 'Foto Test upload', exact: true }).click();
    }
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('#avatar-identity')).toBeVisible();
    await expect(page.locator('#avatar-identity .avatar.placeholder')).toBeVisible();

    // 4x4 red JPEG, built in-memory — no fixture file needed on disk.
    const redJpeg = Buffer.from(
      '/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSj/2wBDAQcHBwoIChMKChMoGhYaKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCj/wAARCAAEAAQDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDk6KKK8I/Vj//Z',
      'base64'
    );
    await page.setInputFiles('#avatar-file-input', {
      name: 'avatar.jpg',
      mimeType: 'image/jpeg',
      buffer: redJpeg,
    });

    await expect(page.locator('#avatar-identity img')).toBeVisible({ timeout: 10000 });
    const src = await page.locator('#avatar-identity img').getAttribute('src');
    expect(src).toMatch(/^\/api\/files\/users\//);

    await page.reload();
    await expect(page.locator('#avatar-identity img')).toBeVisible({ timeout: 10000 });
  });

  test('avatar upload control is hidden when viewing another player', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/player/${data.player2.id}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('#avatar-identity')).toBeVisible();
    await expect(page.locator('#avatar-file-input')).toHaveCount(0);
  });

  test('player can change their own display name from Mi cuenta', async ({ page }) => {
    const player = await createFreshPlayer('rename');
    await loginAs(page, player.email, player.password);
    if (isMobile(page)) {
      await openDrawer(page);
      await page.locator('.drawer-side a', { hasText: 'Mi perfil' }).click();
    } else {
      await page.getByRole('link', { name: 'Foto Test rename', exact: true }).click();
    }
    await page.waitForLoadState('domcontentloaded');

    const account = page.locator('[data-testid="my-account"]');
    await expect(account).toBeVisible();
    await account.locator('#display_name').fill('Renombrado E2E');
    await account.getByRole('button', { name: 'Guardar nombre' }).click();
    await page.waitForURL(/\/player\//);
    await page.waitForLoadState('domcontentloaded');

    await expect(page.getByText('Nombre actualizado')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Renombrado E2E' })).toBeVisible();
    await expect(account.locator('#display_name')).toHaveValue('Renombrado E2E');
  });

  test('Mi cuenta section is hidden when viewing another player', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/player/${data.player2.id}`);
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('[data-testid="my-account"]')).toHaveCount(0);
  });

  test('wrong current password shows an error and does not change the password', async ({ page }) => {
    const player = await createFreshPlayer('badpass');
    await loginAs(page, player.email, player.password);
    if (isMobile(page)) {
      await openDrawer(page);
      await page.locator('.drawer-side a', { hasText: 'Mi perfil' }).click();
    } else {
      await page.getByRole('link', { name: 'Foto Test badpass', exact: true }).click();
    }
    await page.waitForLoadState('domcontentloaded');

    const account = page.locator('[data-testid="my-account"]');
    await account.locator('#current_password').fill('wrong-password');
    await account.locator('#new_password').fill('newpassword12345');
    await account.locator('#new_password_confirm').fill('newpassword12345');
    await account.getByRole('button', { name: 'Cambiar contraseña' }).click();

    await expect(page.getByText('La contraseña actual no es correcta')).toBeVisible({ timeout: 5000 });

    // The old password must still be valid — proves the rejected attempt
    // didn't change it. Bypasses loginAs's token cache, which would mask a
    // regression by reusing the token from the first successful login above.
    // Retries on 429 like loginAs does — the auth endpoint is rate-limited
    // and this suite logs in frequently.
    let authResp;
    for (let attempt = 0; attempt < 5; attempt++) {
      authResp = await page.request.post('/api/collections/users/auth-with-password', {
        data: { identity: player.email, password: player.password },
      });
      if (authResp.status() !== 429) break;
      await new Promise(r => setTimeout(r, 15000));
    }
    expect(authResp!.ok()).toBe(true);
  });
});
