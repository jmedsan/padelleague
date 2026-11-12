import { test, expect } from '@playwright/test';
import { loginAs, clickAction, waitForHxRedirect, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  PLAYER_PASSWORD, ScenarioApi, ScenarioData, addPairToCompetition, apiGet, assertAssignmentInvariants,
  createCompetition, createPairs, createPlayers, generateAssignments, loadCtx, printLinks, publishCalendar,
  saveCtx, setCompetitionDates, uniqueSuffix,
} from '../scenario-helpers';
import { driveTo, duelOf, pairMatches } from './withdrawal-states';

// Leveled: 10 pairs, target 6, open 5. Pair A has a final match (vs B) and a proposed result (vs C);
// pair W has open matches and is withdrawn by the admin: flag only, nothing else changes.
const PAIRS = 10;
const TARGET = 6;
const OPEN = 5;

test.describe.serial('withdraw-release', () => {
  test('00 seed: A has a final and a proposed-result match, W is ready to be withdrawn', async () => {
    const raw = loadCtx();
    const api: ScenarioApi = { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie };
    const suffix = uniqueSuffix();

    const players = await createPlayers(api, PAIRS * 2, suffix);
    const pairs = await createPairs(api, players, suffix);
    const competitionId = await createCompetition(api, `Retirada y liberados ${suffix}`, TARGET, OPEN);
    for (const p of pairs) await addPairToCompetition(api, competitionId, p.id);
    await setCompetitionDates(api, competitionId);
    await generateAssignments(api, competitionId);
    await publishCalendar(api, competitionId);

    const ctx: ScenarioData = { ...raw, api, competitionId, players, pairs, target: TARGET, open: OPEN, stage: 'assigned' };
    await assertAssignmentInvariants(api, ctx);

    const a = pairs[0];
    const aMatches = await pairMatches(api, competitionId, a.id);
    const [ab, ac] = aMatches;
    await driveTo(api, duelOf(ctx, ab, a), 'final');
    await driveTo(api, duelOf(ctx, ac, a), 'scheduledResult');

    const rivalOf = (m: any) => pairs.find((p) => p.id === (m.pair1 === a.id ? m.pair2 : m.pair1))!;
    const b = rivalOf(ab);
    const c = rivalOf(ac);
    const w = pairs.find((p) => ![a.id, b.id, c.id].includes(p.id))!;
    const wMatches = (await pairMatches(api, competitionId, w.id)).filter((m) => m.status !== 'final');
    if (wMatches.length === 0) throw new Error(`${w.name}: expected open matches`);

    saveCtx({ ...ctx, stage: 'assigned', withdrawRelease: { a: a.id, b: b.id, c: c.id, w: w.id, ab: ab.id, ac: ac.id } } as ScenarioData);

    const base = ctx.baseURL;
    const comp = `/competition/${competitionId}`;
    const email = (p: { player1Idx: number }) => players[p.player1Idx].email;
    console.log(`\nA = ${a.name}, B = ${b.name}, C = ${c.name}, W = ${w.name}`);
    console.log('\n=== Admin ===');
    printLinks(base, [
      { label: 'Competition', email: ADMIN_EMAIL, password: ADMIN_PASSWORD, path: `/admin/competitions/${competitionId}` },
      { label: 'Match A-B (final)', email: ADMIN_EMAIL, password: ADMIN_PASSWORD, path: `/match/${ab.id}` },
      { label: 'Match A-C (proposed result)', email: ADMIN_EMAIL, password: ADMIN_PASSWORD, path: `/match/${ac.id}` },
    ]);
    console.log('  1. Retirar W (type RETIRAR): the flash says "<pair> retirada".');
    console.log('  2. Check W\'s matches are unchanged (still open, none became a walkover).');
    console.log('  3. Liberar A-B and read the dialog.');
    console.log('  4. Liberar A-C.');
    console.log('  5. Open "Liberados": both matches are listed.');
    console.log(`\n=== Player of W (${email(w)}) ===`);
    printLinks(base, [
      { label: 'Home', email: email(w), password: PLAYER_PASSWORD, path: '/' },
      { label: 'Competition', email: email(w), password: PLAYER_PASSWORD, path: comp },
      { label: 'Match', email: email(w), password: PLAYER_PASSWORD, path: `/match/${wMatches[0].id}` },
    ]);
    console.log('  1. Check the notice. 2. No new match was assigned. 3. The withdrawn icon shows beside the pair name.');
    console.log(`\n=== Player of B (${email(b)}) ===`);
    printLinks(base, [
      { label: 'Home', email: email(b), password: PLAYER_PASSWORD, path: '/' },
      { label: 'Competition', email: email(b), password: PLAYER_PASSWORD, path: comp },
      { label: 'Match A-B', email: email(b), password: PLAYER_PASSWORD, path: `/match/${ab.id}` },
    ]);
    console.log('  1. After Liberar, A-B is gone from home, the competition page and stats.');
    console.log('  2. The A-B URL returns 404. 3. The standings dropped the points.');
    console.log('  4. Re-pairing: Liberar frees open slots, so after Liberar A-C the top-up may give A and/or C a new');
    console.log('     opponent (never each other again, never W). Check Home or Competition for the new match; a rival');
    console.log('     with no eligible opponent left simply gets none. W never gets a new match.');
    console.log(`\n=== Rival view of the proposed result: player of C (${email(c)}) ===`);
    printLinks(base, [
      { label: 'Home', email: email(c), password: PLAYER_PASSWORD, path: '/' },
      { label: 'Match A-C (before Liberar)', email: email(c), password: PLAYER_PASSWORD, path: `/match/${ac.id}` },
    ]);
    console.log('  1. Before Liberar, A-C shows the proposed result awaiting C. After Liberar it is gone everywhere');
    console.log('     (home, competition, stats, standings) and its URL returns 404.');
  });

  test('01 withdraw W, then release A-B and A-C through the UI', async ({ page }) => {
    const ctx = loadCtx() as any;
    const { a, w, b, c, ab, ac } = ctx.withdrawRelease;
    const liveIds = async (api: ScenarioApi, pair: string) =>
      (await pairMatches(api, ctx.competitionId, pair)).filter((m) => !['final', 'released'].includes(m.status)).map((m) => m.id);
    const api: ScenarioApi = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
    const openBefore = (await pairMatches(api, ctx.competitionId, w)).map((m) => `${m.id}:${m.status}`);

    const liveBefore = { a: await liveIds(api, a), b: await liveIds(api, b), c: await liveIds(api, c) };
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${ctx.competitionId}`);
    await waitForHxRedirect(page, `/admin/competitions/${ctx.competitionId}`, async () => {
      await clickAction(page, `form[hx-post$="/withdraw-pair"]:has(input[name="pair_id"][value="${w}"]) button`, 'Retirar de la competición');
      await page.locator('#confirm-typed').fill('RETIRAR');
      await page.locator('#confirm-ok').click();
    });
    const openAfter = (await pairMatches(api, ctx.competitionId, w)).map((m) => `${m.id}:${m.status}`);
    expect(openAfter).toEqual(openBefore);

    const fresh = async (pair: string, before: string[]) =>
      (await pairMatches(api, ctx.competitionId, pair)).filter((m) => !before.includes(m.id) && !['final', 'released'].includes(m.status));
    for (const id of [ac, ab]) {
      await page.goto(`/match/${id}`);
      await waitForHxRedirect(page, /^\/admin\/competitions\/[^/]+$/, async () => {
        await page.getByRole('button', { name: 'Liberar partido' }).first().click();
        await expect(page.locator('#confirm-message')).toContainText('Liberar');
        await page.locator('#confirm-ok').click();
      });
      if (id === ac) {
        // The top-up runs after the response, so poll for the new matches.
        // Each rival gets at most one new opponent; at least one does (the other may have no eligible opponent left).
        await expect.poll(async () => (await fresh(a, liveBefore.a)).length + (await fresh(c, liveBefore.c)).length, { timeout: 20_000 }).toBeGreaterThan(0);
        const newA = await fresh(a, liveBefore.a);
        const newC = await fresh(c, liveBefore.c);
        expect(newA.length).toBeLessThanOrEqual(1);
        expect(newC.length).toBeLessThanOrEqual(1);
        for (const m of [...newA, ...newC]) {
          expect([m.pair1, m.pair2].sort()).not.toEqual([a, c].sort());
          expect([m.pair1, m.pair2]).not.toContain(w);
        }
      }
    }
    expect((await liveIds(api, w)).sort()).toEqual(openBefore.map((x) => x.split(':')[0]).sort());
    await expect(page.locator('#released-matches')).toContainText('Liberados');
    await expect(page.locator(`#released-matches a[href="/match/${ab}"]`)).toHaveCount(1);
    await expect(page.locator(`#released-matches a[href="/match/${ac}"]`)).toHaveCount(1);

    const bPlayer = ctx.players[ctx.pairs.find((p: any) => p.id === b).player1Idx].email;
    await loginAs(page, bPlayer, PLAYER_PASSWORD);
    expect((await page.request.get(`/match/${ab}`)).status()).toBe(404);
    for (const path of ['/', `/competition/${ctx.competitionId}`]) {
      await page.goto(path);
      await expect(page.locator(`a[href="/match/${ab}"]`)).toHaveCount(0);
    }
  });
});
