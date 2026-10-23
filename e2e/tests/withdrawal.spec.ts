import type { Page } from '@playwright/test';
import { test, expect } from '../overflow-guard';
import {
  loginAs, asPlayerOn, isMobile, clickAction, clickAndWaitForHxRedirect, waitForHxRedirect, loadTestData,
  ADMIN_EMAIL, ADMIN_PASSWORD, PLAYER1_EMAIL, PLAYER1_PASSWORD, PLAYER5_EMAIL, PLAYER5_PASSWORD,
  apiCreateRecord, apiDeleteRecord, apiGetRecord, apiListRecords, openMatchFromHome, suPatch,
} from '../helpers';
import { enterScore } from '../tour-helpers';

const WITHDRAWN = '[data-testid="withdrawn-indicator"]';
const WITHDRAWN_VISIBLE = `${WITHDRAWN}:visible`;
// withdrawnNameOf is the pair-name element (name + indicator) of the named withdrawn pair.
const withdrawnNameOf = (page: Page, name: string) =>
  page.locator(WITHDRAWN_VISIBLE).locator('xpath=..').filter({ hasText: name });

const WITHDRAW_ACTION = (pairId: string) =>
  `form[hx-post$="/withdraw-pair"]:has(input[name="pair_id"][value="${pairId}"]) button`;

// withdrawFromDetail retires pairId from the admin competition detail page through its real
// action: open the dialog, type the confirmation word, press OK, then wait for the redirect.
async function withdrawFromDetail(page: Page, compId: string, pairId: string) {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto(`/admin/competitions/${compId}`);
  await waitForHxRedirect(page, `/admin/competitions/${compId}`, async () => {
    await clickAction(page, WITHDRAW_ACTION(pairId), 'Retirar de la competición');
    await page.locator('#confirm-typed').fill('RETIRAR');
    await page.locator('#confirm-ok').click();
  });
}

// openWithdrawDialog opens the withdraw confirmation for pairId and leaves it open.
async function openWithdrawDialog(page: Page, compId: string, pairId: string) {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await page.goto(`/admin/competitions/${compId}`);
  await clickAction(page, WITHDRAW_ACTION(pairId), 'Retirar de la competición');
}

// createLeague makes a league of the given pairs with a published calendar (players see its
// standings only then), and one pending match per pair of pairs. extra is merged into the
// competition record (e.g. target_matches for a leveled league).
async function createLeague(page: Page, name: string, pairIds: string[], extra: Record<string, unknown> = {}): Promise<string> {
  const token = loadTestData().adminToken;
  const compId = await apiCreateRecord(page.request, token, 'competitions', {
    name, type: 'league', active: true, pairs: pairIds, rounds: 1, calendar_status: 'published', ...extra,
  });
  await apiCreateRecord(page.request, token, 'matches', {
    competition: compId, pair1: pairIds[0], pair2: pairIds[1], status: 'pending', round_number: 1,
  });
  await apiCreateRecord(page.request, token, 'matches', {
    competition: compId, pair1: pairIds[0], pair2: pairIds[2], status: 'pending', round_number: 1,
  });
  return compId;
}

// expandRound opens the Jornadas round whose title starts with title; rounds start collapsed.
// The round's checkbox covers its title, so the checkbox is what gets clicked.
async function expandRound(page: Page, title: string) {
  await page.locator('.collapse', { has: page.locator('.collapse-title', { hasText: title }) })
    .first().locator('input[type="checkbox"]').check();
}

// openFromHome clicks the competition's card on the player's home page.
async function openFromHome(page: Page, compId: string) {
  await page.goto('/');
  await page.locator(`a[href="/competition/${compId}"]`).first().click();
  await page.waitForURL(`**/competition/${compId}`);
}

test.describe('pair withdrawal @withdrawal', { tag: '@withdrawal' }, () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name === 'destructive', 'skip destructive project');
  });

  test.describe.configure({ retries: 0 });

  const createdComps: string[] = [];

  test.afterEach(async ({ page }) => {
    const token = loadTestData().adminToken;
    for (const id of createdComps) {
      for (const m of await apiListRecords(page.request, token, 'matches', `competition='${id}'`)) {
        await apiDeleteRecord(page.request, token, 'matches', m.id);
      }
      await apiDeleteRecord(page.request, token, 'competitions', id);
    }
    createdComps.length = 0;
  });

  test('admin pairs list shows the withdrawn indicator with its tooltip and no Retirada badge', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada admin ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await withdrawFromDetail(page, compId, data.pair1Id);

    // Indicator in the pairs list, reached by the withdraw click on the same page.
    await expect(withdrawnNameOf(page, 'Pareja Alpha').first()).toBeVisible();
    const indicator = page.locator(WITHDRAWN_VISIBLE).first();
    await expect(indicator.locator('.tooltip')).toHaveAttribute('data-tip', 'Pareja retirada');
    await expect(indicator.locator('.tooltip')).toHaveClass(/\btooltip-error\b/);
    await expect(page.locator('.badge', { hasText: 'Retirada' })).toHaveCount(0);
    // The name wraps instead of truncating, and the icon stays inside the viewport.
    const name = indicator.locator('xpath=preceding-sibling::span[1]');
    await expect(name).toHaveText('Pareja Alpha');
    await expect(name).toHaveClass(/\bbreak-words\b/);
    await expect(name).not.toHaveClass(/\btruncate\b/);
    // The withdrawn name is grayed; only the icon beside it is red.
    await expect(name).toHaveClass(/\bopacity-50\b/);
    const box = (await indicator.boundingBox())!;
    expect(box.x + box.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  });

  test('competition page standings show the withdrawn indicator on the pair, reached from home', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada standings ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await withdrawFromDetail(page, compId, data.pair1Id);
    await asPlayerOn(page, compId, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openFromHome(page, compId);
    await page.locator('input[aria-label^="Clasificación"]').click();

    await expect(withdrawnNameOf(page, 'Pareja Alpha').first()).toBeVisible();
    await expect(page.locator('.badge', { hasText: 'Retirada' })).toHaveCount(0);
  });

  test('the same pair shows no indicator in another competition', async ({ page }, testInfo) => {
    const data = loadTestData();
    const withdrawn = await createLeague(page, `Retirada A ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    const other = await createLeague(page, `Retirada B ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(withdrawn, other);

    await withdrawFromDetail(page, withdrawn, data.pair1Id);
    await asPlayerOn(page, other, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openFromHome(page, other);
    await expandRound(page, 'Jornada 1');

    await expect(page.locator('text=Pareja Alpha >> visible=true').first()).toBeVisible();
    await expect(page.locator(WITHDRAWN)).toHaveCount(0);
  });

  test('a match of a withdrawn pair shows the indicator on its match page, reached from the competition page', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada partido ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await withdrawFromDetail(page, compId, data.pair1Id);
    await asPlayerOn(page, compId, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openFromHome(page, compId);

    const matches = await apiListRecords(page.request, data.adminToken, 'matches', `competition='${compId}' && pair1='${data.pair1Id}'`);
    await openMatchFromHome(page, matches[0].id);

    await expect(withdrawnNameOf(page, 'Pareja Alpha').first()).toBeVisible();
  });

  test('a withdrawn pair player sees the walkover result on the match and cannot act on it', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada accion ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);
    const [match] = await apiListRecords(page.request, data.adminToken, 'matches',
      `competition='${compId}' && pair1='${data.pair1Id}' && pair2='${data.pair2Id}'`);

    await withdrawFromDetail(page, compId, data.pair1Id);

    const walkover = await apiGetRecord(page.request, data.adminToken, 'matches', match.id);
    expect(walkover.status).toBe('final');
    expect(walkover.scores).toBe('0-6 0-6'); // stored pair1 first; pair2 holds the sets
    expect(walkover.winner).toBe(data.pair2Id);
    expect(walkover.review_type).toBe('walkover');

    await asPlayerOn(page, compId, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openFromHome(page, compId);
    await openMatchFromHome(page, match.id);

    await expect(page.locator('#result-panel')).toContainText('0-6 0-6');
    await expect(page.locator('#result-panel form')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Enviar resultado' })).toHaveCount(0);

    // The withdrawn pair's player is refused before the match's own state is checked.
    // raw-request: the refusal is the response under test; the throwing helpers would fail on it.
    const refused = await page.request.post(`/match/${match.id}/submit`, { form: { scores: '6-3 6-4' } });
    expect(await refused.text()).toContain('Tu pareja se ha retirado de esta competición');
    const unchanged = await apiGetRecord(page.request, data.adminToken, 'matches', match.id);
    expect(unchanged.scores).toBe('0-6 0-6');
    expect(unchanged.winner).toBe(data.pair2Id);
  });

  test('the jornadas list shows the indicator on the withdrawn pair in its match rows and not on the others', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada lista ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await withdrawFromDetail(page, compId, data.pair1Id);
    await asPlayerOn(page, compId, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openFromHome(page, compId);
    await expandRound(page, 'Jornada 1');

    await expect(withdrawnNameOf(page, 'Pareja Alpha').first()).toBeVisible();
    await expect(withdrawnNameOf(page, 'Pareja Beta')).toHaveCount(0);
  });

  test('the jornadas pair filter marks the withdrawn pair (retirada) and leaves the others plain', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada filtro ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await withdrawFromDetail(page, compId, data.pair1Id);
    await asPlayerOn(page, compId, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openFromHome(page, compId);
    await page.getByLabel('Jornadas', { exact: true }).click();

    await expect(page.locator('select[name="pair"] option', { hasText: /^(★ )?Pareja Alpha \(retirada\)$/ })).toHaveCount(1);
    await expect(page.locator('select[name="pair"] option', { hasText: /^(★ )?Pareja Beta$/ })).toHaveCount(1);
  });

  test('a playoff match whose pair withdrew stands as a walkover when the opponent withdraws too', async ({ page }, testInfo) => {
    const data = loadTestData();
    const token = data.adminToken;
    const compId = await apiCreateRecord(page.request, token, 'competitions', {
      name: `Retirada playoff ${testInfo.project.name} ${Date.now()}`, type: 'playoff', active: true,
      pairs: [data.pair1Id, data.pair2Id, data.pair3Id, data.adminPairId], rounds: 2,
    });
    createdComps.push(compId);
    const matchId = await apiCreateRecord(page.request, token, 'matches', {
      competition: compId, pair1: data.pair1Id, pair2: data.pair2Id, status: 'pending', round_number: 1,
    });
    await apiCreateRecord(page.request, token, 'matches', {
      competition: compId, pair1: data.pair3Id, pair2: data.adminPairId, status: 'pending', round_number: 1,
    });

    // The first withdrawal walks the opponent over; the second leaves that final match standing.
    await withdrawFromDetail(page, compId, data.pair1Id);
    await withdrawFromDetail(page, compId, data.pair2Id);

    const matchFinal = await apiGetRecord(page.request, token, 'matches', matchId);
    expect(matchFinal.status).toBe('final');
    expect(matchFinal.scores).toBe('0-6 0-6');
    expect(matchFinal.winner).toBe(data.pair2Id);

    await asPlayerOn(page, compId, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openFromHome(page, compId);

    await expect(page.locator(`a[href="/match/${matchId}"]`).first()).toBeVisible();
    await expect(page.locator('span:visible', { hasText: /^Sin resultado$/ })).toHaveCount(0);
  });

  test('the playoff dialog states that finished matches stay', async ({ page }, testInfo) => {
    const data = loadTestData();
    const token = data.adminToken;
    const compId = await apiCreateRecord(page.request, token, 'competitions', {
      name: `Retirada playoff texto ${testInfo.project.name} ${Date.now()}`, type: 'playoff', active: true,
      pairs: [data.pair1Id, data.pair2Id, data.pair3Id, data.adminPairId], rounds: 2,
    });
    createdComps.push(compId);
    await apiCreateRecord(page.request, token, 'matches', {
      competition: compId, pair1: data.pair1Id, pair2: data.pair2Id, status: 'pending', round_number: 1,
    });

    await openWithdrawDialog(page, compId, data.pair1Id);
    await expect(page.locator('#confirm-message')).toContainText('los partidos ya terminados se mantienen');
    await page.locator('[data-confirm="cancel"]').click();
  });

  test('the withdraw confirm stays disabled until RETIRAR is typed, ignoring surrounding spaces and case', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada palabra ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await openWithdrawDialog(page, compId, data.pair1Id);
    const typed = page.locator('#confirm-typed');
    const ok = page.locator('#confirm-ok');
    await expect(typed).toBeVisible();
    await expect(ok).toBeDisabled();

    await typed.fill('RETIRA');
    await expect(ok).toBeDisabled();

    await typed.fill('RETIRARS');
    await expect(ok).toBeDisabled();

    await typed.fill('  retirar  ');
    await expect(ok).toBeEnabled();
    await page.locator('[data-confirm="cancel"]').click();
  });

  test('cancel and Escape change nothing, and reopening clears the typed word', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada cancelar ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await openWithdrawDialog(page, compId, data.pair1Id);
    await page.locator('#confirm-typed').fill('RETIRAR');
    await page.locator('[data-confirm="cancel"]').click();
    await expect(page.locator('#confirm-modal')).not.toHaveAttribute('open', '');
    await expect(page.locator(WITHDRAWN)).toHaveCount(0);

    await clickAction(page, WITHDRAW_ACTION(data.pair1Id), 'Retirar de la competición');
    await expect(page.locator('#confirm-typed')).toHaveValue('');
    await expect(page.locator('#confirm-ok')).toBeDisabled();
    await page.keyboard.press('Escape');
    await expect(page.locator('#confirm-modal')).not.toHaveAttribute('open', '');
    await expect(page.locator(WITHDRAWN)).toHaveCount(0);
  });

  test('the dialog names the pair and states the walkover effect', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada texto ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await openWithdrawDialog(page, compId, data.pair1Id);
    const message = page.locator('#confirm-message');
    await expect(message).toContainText('¿Retirar a Pareja Alpha?');
    await expect(message).toContainText('incomparecencia (6-0 6-0)');
    await expect(message).toContainText('Solo se mantienen los resultados que el administrador ya fijó como incomparecencia a favor del rival.');
    await expect(message).not.toContainText('sin jugar se liberan');
    await page.locator('[data-confirm="cancel"]').click();
  });

  test('the leveled dialog also states that unplayed matches are released', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada nivel ${testInfo.project.name} ${Date.now()}`,
      [data.pair1Id, data.pair2Id, data.pair3Id], { target_matches: 1 });
    createdComps.push(compId);

    await openWithdrawDialog(page, compId, data.pair1Id);
    await expect(page.locator('#confirm-message')).toContainText('sin jugar se liberan');
    await page.locator('[data-confirm="cancel"]').click();
  });

  test('the remove-pair confirm keeps the plain dialog without the typed input', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada otras ${testInfo.project.name} ${Date.now()}`, [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${compId}`);
    await clickAction(page,
      `form[hx-post$="/remove-pair"]:has(input[name="pair_id"][value="${data.pair1Id}"]) button`,
      'Quitar de la competición');

    await expect(page.locator('#confirm-typed')).toBeHidden();
    await expect(page.locator('#confirm-ok')).toBeEnabled();
    await page.locator('[data-confirm="cancel"]').click();
  });

  test('a result proposal by a withdrawn pair names the pair in its frozen timeline author line without the indicator', async ({ page }, testInfo) => {
    const data = loadTestData();
    const compId = await createLeague(page, `Retirada linea ${testInfo.project.name} ${Date.now()}`,
      [data.pair1Id, data.pair2Id, data.pair3Id]);
    createdComps.push(compId);
    const [match] = await apiListRecords(page.request, data.adminToken, 'matches',
      `competition='${compId}' && pair1='${data.pair1Id}' && pair2='${data.pair2Id}'`);

    // The submit form renders only once the match has a date and place.
    await suPatch(page.request, data.adminToken, `/api/collections/matches/records/${match.id}`, { date: '2025-03-15', club: 'Padel 360' });

    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await openMatchFromHome(page, match.id);
    await page.waitForSelector('#result-panel', { timeout: 10000 });
    await expect(page.locator('.score-cell').first()).toBeVisible({ timeout: 5000 });
    await enterScore(page, '6-3 6-4');
    await clickAndWaitForHxRedirect(page, page.getByRole('button', { name: 'Enviar resultado' }), `/match/${match.id}`);

    await withdrawFromDetail(page, compId, data.pair1Id);

    await loginAs(page, PLAYER5_EMAIL, PLAYER5_PASSWORD);
    await page.goto('/');
    await openMatchFromHome(page, match.id);
    const timeline = page.locator('#thread-timeline');
    await expect(timeline.getByText('6-3 6-4').locator('visible=true').first()).toBeVisible({ timeout: 5000 });
    // The proposal's author line is a frozen snapshot: it names the pair but shows no withdrawn indicator.
    const author = timeline.locator('.font-bold', { hasText: '(Pareja Alpha)' });
    await expect(author).toHaveCount(1);
    await expect(author).toContainText('(Pareja Alpha)');
    await expect(author.locator(WITHDRAWN)).toHaveCount(0);
  });
});
