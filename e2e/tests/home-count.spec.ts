import { test, expect } from '../overflow-guard';
import { asPlayerOn, loadTestData, apiCreateRecord } from '../helpers';

// The home count is the player's matches still to play (pending or
// scheduled). A disputed match was already played, so a player whose only
// match is in dispute has nothing left to play, and the card says so in
// those words: "pendientes" read as wrong next to the open dispute.
test('home card: a player whose only match is disputed has none to play', { tag: '@competitions' }, async ({ page }, testInfo) => {
  const suffix = `${testInfo.project.name}-${Date.now() % 1000000}`;
  const token = loadTestData().adminToken;
  const req = page.request;
  const password = 'testpass123456';
  const makePlayer = (tag: string) => apiCreateRecord(req, token, 'users', {
    email: `count-${tag}-${suffix}@test.local`, password, passwordConfirm: password,
    display_name: `Count ${tag} ${suffix}`, roles: ['player'], verified: true, gender: 'male',
  });
  const [a1, a2, b1, b2] = await Promise.all(['a1', 'a2', 'b1', 'b2'].map(makePlayer));
  const pairA = await apiCreateRecord(req, token, 'pairs', { name: `Count A ${suffix}`, player1: a1, player2: a2 });
  const pairB = await apiCreateRecord(req, token, 'pairs', { name: `Count B ${suffix}`, player1: b1, player2: b2 });
  const compId = await apiCreateRecord(req, token, 'competitions', {
    name: `Count ${suffix}`, type: 'league', active: true, pairs: [pairA, pairB], rounds: 1,
  });
  await apiCreateRecord(req, token, 'matches', {
    competition: compId, pair1: pairA, pair2: pairB, round_number: 1,
    status: 'disputed', scores: '6-4 6-4', submitted_by: a1,
    disputed_scores: '4-6 4-6', disputed_by: b1,
  });

  await asPlayerOn(page, compId, `count-a1-${suffix}@test.local`, password);
  // Into the competition and back home through the navbar, as a player would.
  await page.getByTestId('single-comp-entry').click();
  await expect(page).toHaveURL(new RegExp(`/competition/${compId}`));
  await page.locator('.navbar a[href="/"]').first().click();
  await expect(page).toHaveURL(/\/$/);

  await expect(page.getByTestId('single-comp-entry')).toContainText('Sin partidos por jugar');
});
