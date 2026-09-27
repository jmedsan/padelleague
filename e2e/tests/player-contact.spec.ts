import type { APIRequestContext } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import {
  loginAs, loadTestData, isMobile, suPatch as suPatchBase, apiCreateRecord, apiDeleteRecord,
  PLAYER1_EMAIL, PLAYER1_PASSWORD,
} from '../helpers';
import { PLAYER2_NAME } from '../global-setup';

function suToken(): string {
  return loadTestData().adminToken;
}

async function suPatch(request: APIRequestContext, path: string, data: Record<string, unknown>): Promise<void> {
  await suPatchBase(request, suToken(), path, data);
}

test.describe('player contact', { tag: '@profile' }, () => {
  test.afterAll(async ({ request }) => {
    const data = loadTestData();
    await suPatch(request, `/api/collections/users/records/${data.player2.id}`, { phone: '' });
  });

  test('player reaches another player\'s profile and sees icon contact links labeled with the number/address', async ({ page, request }) => {
    const data = loadTestData();
    await suPatch(request, `/api/collections/users/records/${data.player2.id}`, { phone: '+34612345678' });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.locator('a[href^="/competition/"]', { hasText: 'Liga E2E Test' }).first().click();
    await page.waitForLoadState('domcontentloaded');
    await page.locator('input[aria-label^="Clasificación"]').click();
    const standingsTableClass = isMobile(page) ? 'table.table-sm' : 'table.table-zebra';
    await page.locator(`${standingsTableClass} a[href="/pair/${data.pair1Id}"]`).click();
    await page.waitForURL(`**/pair/${data.pair1Id}`);
    await page.locator(`table a[href="/player/${data.player2.id}"]`).click();
    await page.waitForURL(`**/player/${data.player2.id}`);

    const contact = page.locator('[data-testid="player-contact"]');
    const whatsappLabel = 'WhatsApp: +34612345678';
    const emailLabel = `Email: ${data.player2.email}`;
    const whatsapp = contact.getByRole('link', { name: whatsappLabel, exact: true });
    const email = contact.getByRole('link', { name: emailLabel, exact: true });
    await expect(whatsapp).toHaveAttribute('href', 'https://wa.me/34612345678');
    await expect(email).toHaveAttribute('href', `mailto:${data.player2.email}`);
    // Icon-only: the SVG is the visible content, no text label.
    await expect(whatsapp).toHaveText('');
    await expect(email).toHaveText('');
    await expect(whatsapp.locator('svg')).toBeVisible();
    // Default (dark) theme: WhatsApp brand green.
    await expect(whatsapp.locator('svg')).toHaveCSS('color', 'rgb(37, 211, 102)');
    // Light theme swaps in WhatsApp's darker green so the icon keeps 3:1 contrast.
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'padel'));
    await expect(whatsapp.locator('svg')).toHaveCSS('color', 'rgb(18, 140, 126)');
    await expect(email.locator('svg')).toBeVisible();
    // The icons speak for themselves: no hover tooltip (owner, 2026-09-27).
    await expect(contact.locator('.tooltip')).toHaveCount(0);
  });

  test('match page rival contacts fit a 360px phone, for a long name and a short one, each with both icons', async ({ page, request }, testInfo) => {
    test.skip(!isMobile(page), 'phone-width layout check');
    const data = loadTestData();
    const token = suToken();
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const longName = `Maximiliano Fernández-Villaverde ${suffix}`;
    const rival1 = await apiCreateRecord(request, token, 'users', {
      email: `maximiliano-fernandez-villaverde-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: longName, roles: ['player'], verified: true, phone: '+34612345678',
    });
    const rival2 = await apiCreateRecord(request, token, 'users', {
      email: `ana-ruiz-${suffix}@test.local`, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: 'Ana Ruiz', roles: ['player'], verified: true, phone: '+34699887766',
    });
    const rivalPairId = await apiCreateRecord(request, token, 'pairs', {
      name: `Pareja Ancha E2E ${suffix}`, player1: rival1, player2: rival2, captain: rival1,
    });
    const matchId = await apiCreateRecord(request, token, 'matches', {
      competition: data.competitionId, pair1: data.pair1Id, pair2: rivalPairId, status: 'pending',
    });

    try {
      await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
      await page.goto('/');
      await page.locator(`a[href="/match/${matchId}"]`).first().click();
      await page.waitForURL(`**/match/${matchId}`);

      const rivals = page.locator('[data-testid="rival-contacts"]');
      await expect(rivals.getByText(longName)).toBeVisible();
      await expect(rivals.getByRole('link', { name: 'WhatsApp: +34612345678', exact: true })).toBeVisible();
      await expect(rivals.getByRole('link', { name: /^Email: maximiliano-/ })).toBeVisible();
      // The short name keeps its icons on the name's line, mid-card: where a
      // hidden hover tooltip carrying the full email address pushed the page
      // sideways on 481a3d0. The long name wraps its icons below.
      await expect(rivals.getByText('Ana Ruiz')).toBeVisible();
      await expect(rivals.getByRole('link', { name: 'WhatsApp: +34699887766', exact: true })).toBeVisible();
      const card = await rivals.evaluate(el => ({ scroll: el.scrollWidth, client: el.clientWidth }));
      expect(card.scroll).toBeLessThanOrEqual(card.client);
      const doc = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }));
      expect(doc.scroll).toBeLessThanOrEqual(doc.client);
    } finally {
      await apiDeleteRecord(request, token, 'matches', matchId);
      await apiDeleteRecord(request, token, 'pairs', rivalPairId);
      await apiDeleteRecord(request, token, 'users', rival1);
      await apiDeleteRecord(request, token, 'users', rival2);
    }
  });

  test('participant sees exactly the rival pair\'s contacts, never the partner\'s or their own', async ({ page, request }, testInfo) => {
    const data = loadTestData();
    const token = suToken();
    const suffix = `${Date.now()}-${testInfo.project.name}`;
    const rival1Email = `rival1-contact-${suffix}@test.com`;
    const rival2Email = `rival2-contact-${suffix}@test.com`;

    // A pair genuinely disjoint from data.pair1Id (viewer's pair) — two fresh
    // players, not the seeded player1/player2/admin, so nobody here overlaps
    // pair1 and the test exercises the real 2-rival case, not the
    // both-pairs-share-player1 shortcut. Emails/names carry a per-run suffix
    // so a failed cleanup never collides with the next run.
    const rival1 = await apiCreateRecord(request, token, 'users', {
      email: rival1Email, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Rival Uno ${suffix}`, roles: ['player'], verified: true, phone: '+34612345678',
    });
    const rival2 = await apiCreateRecord(request, token, 'users', {
      email: rival2Email, password: 'testpass123456', passwordConfirm: 'testpass123456',
      display_name: `Rival Dos ${suffix}`, roles: ['player'], verified: true,
    });
    const rivalPairId = await apiCreateRecord(request, token, 'pairs', {
      name: `Pareja Rival E2E ${suffix}`, player1: rival1, player2: rival2, captain: rival1,
    });
    const matchId = await apiCreateRecord(request, token, 'matches', {
      competition: data.competitionId, pair1: data.pair1Id, pair2: rivalPairId, status: 'pending',
    });

    try {
      await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
      await page.goto('/');
      await page.locator(`a[href="/match/${matchId}"]`).first().click();
      await page.waitForURL(`**/match/${matchId}`);
      await page.waitForLoadState('domcontentloaded');

      const rivals = page.locator('[data-testid="rival-contacts"]');
      await expect(rivals).toBeVisible();
      await expect(rivals.getByText(`Rival Uno ${suffix}`)).toBeVisible();
      await expect(rivals.getByText(`Rival Dos ${suffix}`)).toBeVisible();
      await expect(rivals.locator('a[href="https://wa.me/34612345678"]')).toBeVisible();
      await expect(rivals.locator(`a[href="mailto:${rival1Email}"]`)).toBeVisible();
      // Rival Dos has no phone: email-only (Story 2 AC), no stray wa.me link for them.
      await expect(rivals.locator(`a[href="mailto:${rival2Email}"]`)).toBeVisible();
      await expect(rivals.locator('a[href^="https://wa.me/"]')).toHaveCount(1);

      // Partner (player2) and the viewer's own contact never appear in the card.
      await expect(rivals.getByText(PLAYER2_NAME)).toHaveCount(0);
      await expect(rivals.locator(`a[href="mailto:${data.player2.email}"]`)).toHaveCount(0);
      await expect(rivals.locator(`a[href="mailto:${PLAYER1_EMAIL}"]`)).toHaveCount(0);
    } finally {
      await apiDeleteRecord(request, token, 'matches', matchId);
      await apiDeleteRecord(request, token, 'pairs', rivalPairId);
      await apiDeleteRecord(request, token, 'users', rival1);
      await apiDeleteRecord(request, token, 'users', rival2);
    }
  });
});
