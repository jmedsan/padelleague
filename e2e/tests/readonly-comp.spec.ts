import { test, expect } from '@playwright/test';
import { loginAs, loadTestData, ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test.describe('R-172: finalized competition is read-only for players', () => {
  test('player cannot see submit/propose controls on finalized comp match', async ({ page }) => {
    const data = loadTestData();
    const compId = data.competitionId;
    const matchId = data.matchIds[0];

    // Finalize the competition via API
    const resp = await page.request.patch(
      `/api/collections/competitions/records/${compId}`,
      {
        headers: { Authorization: data.adminToken },
        data: { finalized: true },
      },
    );
    expect(resp.ok()).toBeTruthy();

    try {
      // Login as player and visit match
      await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
      await page.goto(`/match/${matchId}`);
      await page.waitForLoadState('networkidle');

      // Submit form should NOT be visible
      await expect(page.getByText('Registrar resultado')).not.toBeVisible();
      await expect(page.getByText('Enviar resultado')).not.toBeVisible();

      // Thread scheduling should not show proposal form
      const thread = page.locator('#thread-container');
      await expect(thread.getByText('Proponer fecha')).not.toBeVisible();

      // Admin should still see override controls
      await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
      await page.goto(`/match/${matchId}?admin=1`);
      await page.waitForLoadState('networkidle');
      await expect(page.getByText('Corrección de administrador')).toBeVisible();
    } finally {
      // Un-finalize to avoid breaking other tests
      await page.request.patch(
        `/api/collections/competitions/records/${compId}`,
        {
          headers: { Authorization: data.adminToken },
          data: { finalized: false },
        },
      );
    }
  });
});
