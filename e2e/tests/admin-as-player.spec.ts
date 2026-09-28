import { test, expect } from '../overflow-guard';
import { loginAs, loadTestData, apiCreateRecord, switchView, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

// An admin who also plays sees the app as the view they chose, not as their
// role: in the player view an admin-only action (releasing a leveled
// assignment) and every admin entry are hidden, and switching back to the admin view shows it.

test.describe('admin as player', { tag: '@admin' }, () => {
  test('"Liberar partido" follows the view, not the role', async ({ page }) => {
    const data = loadTestData();
    const token = data.adminToken;
    const suffix = `release-view-${Date.now() % 100000}`;
    // Four pairs with target_matches 2 < pairs-1: a leveled league
    // (league.IsLeveled), where the admin may release an open assignment.
    const extraPairs: string[] = [];
    for (const side of ['x', 'y']) {
      const players: string[] = [];
      for (const n of [1, 2]) {
        players.push(await apiCreateRecord(page.request, token, 'users', {
          email: `${suffix}-${side}${n}@test.local`, password: 'TestPass123456', passwordConfirm: 'TestPass123456',
          display_name: `Nivel ${side.toUpperCase()}${n}`, gender: 'male', roles: ['player'], verified: true,
        }));
      }
      extraPairs.push(await apiCreateRecord(page.request, token, 'pairs', {
        name: `Pareja Nivel ${side.toUpperCase()} ${suffix}`, player1: players[0], player2: players[1],
      }));
    }
    const compId = await apiCreateRecord(page.request, token, 'competitions', {
      name: `Liga Nivelada ${suffix}`, type: 'league', active: true,
      pairs: [data.adminPairId, data.pair3Id, ...extraPairs],
      target_matches: 2, open_assignments: 1,
      // Players (the admin in player view included) see matches only on a
      // published calendar (handlers/public_competition.go).
      calendar_status: 'published',
    });
    const matchId = await apiCreateRecord(page.request, token, 'matches', {
      competition: compId, pair1: data.adminPairId, pair2: data.pair3Id,
      status: 'pending', round_number: 0,
    });

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await switchView(page, 'player');
    await page.locator(`a[href="/competition/${compId}"]`).locator('visible=true').first().click();
    await page.locator(`a[href="/match/${matchId}"]`).locator('visible=true').first().click();
    await page.waitForURL(`**/match/${matchId}`);
    const release = page.getByRole('button', { name: 'Liberar partido' });
    await expect(page.locator('#thread-details')).toBeVisible();
    await expect(release).toHaveCount(0);
    // No admin entry anywhere: neither the "Gestión" menu nor any /admin link
    // (desktop menu, mobile drawer or page body).
    const adminLinks = page.locator('a[href^="/admin"]');
    await expect(page.locator('summary', { hasText: 'Gestión' })).toHaveCount(0);
    await expect(adminLinks).toHaveCount(0);

    // The view switch returns to the page it was made from (handlers/view.go).
    await switchView(page, 'admin');
    await page.waitForURL(`**/match/${matchId}`);
    await expect(release).toBeVisible();
    await expect(adminLinks.first()).toBeAttached();
  });
});
