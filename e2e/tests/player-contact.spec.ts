import type { APIRequestContext } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import {
  loginAs, loadTestData, suPatch as suPatchBase, apiCreateRecord, apiDeleteRecord,
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

  test('player sees WhatsApp and email links on another player\'s profile', async ({ page, request }) => {
    const data = loadTestData();
    await suPatch(request, `/api/collections/users/records/${data.player2.id}`, { phone: '+34612345678' });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/player/${data.player2.id}`);
    await page.waitForLoadState('domcontentloaded');

    const contact = page.locator('[data-testid="player-contact"]');
    await expect(contact).toBeVisible();
    const whatsapp = contact.locator('a[href^="https://wa.me/"]');
    await expect(whatsapp).toBeVisible();
    const email = contact.locator('a[href^="mailto:"]');
    await expect(email).toBeVisible();
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
