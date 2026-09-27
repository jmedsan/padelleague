import type { APIRequestContext } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, suPatch as suPatchBase, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

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
});
