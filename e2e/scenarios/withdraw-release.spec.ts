import { test, expect, type Page } from '@playwright/test';
import { loginAs, leagueDate, clickAction, waitForHxRedirect, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  PLAYER_PASSWORD, ScenarioApi, ScenarioData, addPairToCompetition, apiGet, apiPatch, assertAssignmentInvariants,
  createCompetition, createPairs, createPlayers, generateAssignments, loadCtx, publishCalendar,
  saveCtx, setCompetitionDates, uniqueSuffix,
} from '../scenario-helpers';
import { driveTo, duelOf, pairMatches } from './withdrawal-states';

// Leveled: 10 pairs, target 3, open 3. Pair W has a final match (vs B), a proposed result (vs C) and
// one open match. The admin withdraws W (flag only, nothing changes), then releases W-B and W-C.
// Target 3 = open 3 makes re-pairing observable: after W-B is final, B sits at its target with a free
// open slot, so releasing W-B (and W-C) gives B and C one new opponent each, never W.
const PAIRS = 10;
const TARGET = 3;
const OPEN = 3;

// Reads one pair's Clasificación row (desktop table): played matches (PJ) and points.
async function standingsRow(page: Page, competitionId: string, pairName: string) {
  await page.goto(`/admin/competitions/${competitionId}`);
  const rows = await page.locator('table.table-zebra').filter({ has: page.locator('th', { hasText: /^PJ$/ }) }).locator('tbody tr').evaluateAll((trs) =>
    trs.map((tr) => {
      const tds = Array.from(tr.querySelectorAll('td'));
      return {
        name: (tds[1]?.textContent ?? '').replace(/\s+/g, ' ').trim(),
        pj: Number((tds[2]?.textContent ?? '').trim()),
        pts: Number((tr.querySelector('td.text-lg.font-bold')?.textContent ?? '').trim()),
      };
    }));
  const row = rows.find((r) => r.name.startsWith(pairName) && !/^\d/.test(r.name.slice(pairName.length).trim()));
  if (!row) throw new Error(`no Clasificación row for ${pairName}: ${JSON.stringify(rows)}`);
  return row;
}

test.describe.serial('withdraw-release', () => {
  test('00 seed: W has a final (vs B) and a proposed-result (vs C) match', async () => {
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

    // Deterministic pick from the met-graph, never a re-run: the first pair W (in creation order) with two
    // rivals B and C that have not met each other. After both releases B and C are then the only pairs
    // short of the target and the only unmet partners left for each other.
    const rivalIdOf = (m: any, p: string) => (m.pair1 === p ? m.pair2 : m.pair1);
    const matchesOf = new Map<string, any[]>();
    for (const p of pairs) matchesOf.set(p.id, await pairMatches(api, competitionId, p.id));
    const met = (x: string, y: string) => matchesOf.get(x)!.some((m) => m.pair1 === y || m.pair2 === y);
    let chosen: { w: (typeof pairs)[number]; wb: any; wc: any; wo: any } | undefined;
    for (const cand of pairs) {
      const ms = matchesOf.get(cand.id)!;
      if (ms.length !== 3) throw new Error(`${cand.name}: expected 3 matches`);
      for (const [i, j] of [[0, 1], [0, 2], [1, 2]]) {
        if (!met(rivalIdOf(ms[i], cand.id), rivalIdOf(ms[j], cand.id))) { chosen = { w: cand, wb: ms[i], wc: ms[j], wo: ms[3 - i - j] }; break; }
      }
      if (chosen) break;
    }
    if (!chosen) throw new Error('met-graph: no pair has two rivals that have not met each other');
    const { w, wb, wc, wo } = chosen;
    await driveTo(api, duelOf(ctx, wb, w), 'final');
    await driveTo(api, duelOf(ctx, wc, w), 'scheduledResult');

    // A final that stays: B against its first other opponent. It keeps the Clasificación non-empty after
    // W-B and W-C are released, so the test can assert exact rows.
    const rivalOf0 = (m: any) => pairs.find((p) => p.id === (m.pair1 === w.id ? m.pair2 : m.pair1))!;
    const bPair = rivalOf0(wb);
    const bx = (await pairMatches(api, competitionId, bPair.id)).find((m) => m.id !== wb.id)!;
    await driveTo(api, duelOf(ctx, bx, bPair), 'final');

    const rivalOf = (m: any) => pairs.find((p) => p.id === (m.pair1 === w.id ? m.pair2 : m.pair1))!;
    const b = rivalOf(wb);
    const c = rivalOf(wc);

    // Case 2: 4 pairs, target 2 = a 4-cycle W-B-Y-X-W. After W is withdrawn and only W-B is released, B has
    // met Y and W, and its only other pair X is already at its target: B has no placeable opponent left.
    const players2 = await createPlayers(api, 8, `${suffix}s`);
    const pairs2 = await createPairs(api, players2, `${suffix}s`);
    const competition2 = await createCompetition(api, `Retirada sin rival ${suffix}`, 2, 2);
    for (const p of pairs2) await addPairToCompetition(api, competition2, p.id);
    await setCompetitionDates(api, competition2);
    await generateAssignments(api, competition2);
    await publishCalendar(api, competition2);
    const w2 = pairs2[0];
    const w2Matches = await pairMatches(api, competition2, w2.id);
    if (w2Matches.length !== 2) throw new Error(`${w2.name}: expected 2 matches in the small competition`);
    const wb2 = w2Matches[0];
    const b2 = pairs2.find((p) => p.id === (wb2.pair1 === w2.id ? wb2.pair2 : wb2.pair1))!;

    saveCtx({ ...ctx, stage: 'assigned', withdrawRelease: { w: w.id, b: b.id, c: c.id, wb: wb.id, wc: wc.id, wo: wo.id, bx: bx.id },
      noRival: { competition: competition2, w: w2.id, b: b2.id, wb: wb2.id, players: players2, pairs: pairs2 } } as ScenarioData);

    const base = ctx.baseURL;
    const comp = `/competition/${competitionId}`;
    const email = (p: { player1Idx: number }) => players[p.player1Idx].email;
    const url = (who: string, pw: string, path: string) =>
      `${base}/dev-login?${new URLSearchParams({ email: who, password: pw, next: path })}`;
    const admin = (path: string) => url(ADMIN_EMAIL, ADMIN_PASSWORD, path);
    const as = (p: { player1Idx: number }, path: string) => url(email(p), PLAYER_PASSWORD, path);
    const as2 = (p: { player1Idx: number }, path: string) => url(players2[p.player1Idx].email, PLAYER_PASSWORD, path);
    const steps: Array<[string, string, string]> = [
      ['Player of B', as(b, '/'), 'Note your matches: W-B and another match are final (both count in your stats), plus your open matches.'],
      ['Player of C', as(c, `/match/${wc.id}`), 'W-C shows the proposed result waiting for C.'],
      ['Admin', admin(`/admin/competitions/${competitionId}`), 'Retirar W (type RETIRAR): the flash says "<pair> retirada"; the withdrawn icon shows beside W.'],
      ['Admin', admin(`/match/${wo.id}`), "W's open match is unchanged: still open, no 6-0 6-0."],
      ['Player of W', as(w, '/'), 'The withdrawal notice is there; no new match appears now or later.'],
      ['Player of W', as(w, comp), 'The withdrawn icon shows beside the pair name.'],
      ['Admin', admin(`/match/${wb.id}`), 'Liberar W-B (it has a final result): read the dialog and confirm.'],
      ['Admin', admin(`/match/${wc.id}`), 'Liberar W-C (proposed result): confirm.'],
      ['Admin', admin(`/admin/competitions/${competitionId}`), 'Open "Liberados": W-B and W-C are listed; open one: read-only, "Liberado" badge, history kept.'],
      ['Player of B', as(b, `/match/${wb.id}`), 'W-B now returns 404.'],
      ['Player of B', as(b, comp), 'W-B is gone, B has ONE NEW match against a new opponent (not W; open the later Jornada sections in Partidos), and the W-B points are gone from the standings (the other final stays).'],
      ['Player of C', as(c, `/match/${wc.id}`), 'W-C now returns 404.'],
      ['Player of C', as(c, comp), 'W-C is gone and C has ONE NEW match against a new opponent (not W; open the later Jornada sections).'],
      ['Player of W', as(w, comp), 'W still has only its open match: no new assignment.'],
      ['Admin', admin(`/admin/competitions/${competitionId}`), 'Case 3: open Clasificación and the stats of B and C: the W-B and W-C results no longer count anywhere.'],
      ['Admin', admin(`/admin/competitions/${competition2}`), 'Case 2 (second competition, 4 pairs, target 2): Retirar W2 (type RETIRAR).'],
      ['Admin', admin(`/match/${wb2.id}`), 'Liberar W2-B2 (the match against B2).'],
      ['Player of B2', as2(b2, `/competition/${competition2}`), 'B2 has NO new match (the only pair left is already at its target) and no "Revancha" anywhere; B2 stays one match short.'],
      ['Admin', admin(`/admin/competitions/${competition2}`), 'The shortfall notice names B2 as ending below the target.'],
      ['Admin', admin(`/admin/competitions/${competition2}`), 'Case 4 (test 03 drives B2-Y2 to final, moves the end date into the past and fires the close): the league is closed; B2 has no penalty, Y2 and X2 have -1 each, W2 none.'],
    ];
    console.log(`\nW = ${w.name} (withdrawn), B = ${b.name}, C = ${c.name}`);
    console.log(`Admin ${ADMIN_EMAIL}; W ${email(w)}; B ${email(b)}; C ${email(c)}\n`);
    steps.forEach(([who, link, check], i) => console.log(`${i + 1}. ${who}: ${link}\n   ${check}`));
  });

  test('01 withdraw W, release W-B and W-C through the UI: B and C get one new opponent each', async ({ page }) => {
    const ctx = loadCtx() as any;
    const { w, b, c, wb, wc, wo, bx } = ctx.withdrawRelease;
    const api: ScenarioApi = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
    const live = async (pair: string) =>
      (await pairMatches(api, ctx.competitionId, pair)).filter((m) => !['final', 'released'].includes(m.status));
    const wBefore = (await pairMatches(api, ctx.competitionId, w)).map((m) => `${m.id}:${m.status}`).sort();
    const opponentsBefore = async (pair: string) =>
      (await pairMatches(api, ctx.competitionId, pair)).map((m) => (m.pair1 === pair ? m.pair2 : m.pair1));
    const metBefore = { b: await opponentsBefore(b), c: await opponentsBefore(c) };
    const liveBefore = { b: (await live(b)).map((m) => m.id), c: (await live(c)).map((m) => m.id) };

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${ctx.competitionId}`);
    await waitForHxRedirect(page, `/admin/competitions/${ctx.competitionId}`, async () => {
      await clickAction(page, `form[hx-post$="/withdraw-pair"]:has(input[name="pair_id"][value="${w}"]) button`, 'Retirar de la competición');
      await page.locator('#confirm-typed').fill('RETIRAR');
      await page.locator('#confirm-ok').click();
    });
    expect((await pairMatches(api, ctx.competitionId, w)).map((m) => `${m.id}:${m.status}`).sort()).toEqual(wBefore);

    // Case 3: the released results drop out of the Clasificación (W-B final, W-C proposed result).
    const nameOf = (id: string) => ctx.pairs.find((p: any) => p.id === id).name as string;
    // Expected rows from independent facts: W-B and bx are final (winner stored), W-C holds a proposed
    // result that counts as played (proposal = played) and the proposer's score 6-3 6-4 is pair1's win.
    const rec = async (id: string) => apiGet(api, `/api/collections/matches/records/${id}`);
    const counted = [await rec(wb), await rec(bx), { ...(await rec(wc)), winner: (await rec(wc)).pair1 }];
    const expectRow = (pair: string, ms: any[]) => {
      const mine = ms.filter((m) => m.pair1 === pair || m.pair2 === pair);
      return { pj: mine.length, pts: 3 * mine.filter((m) => m.winner === pair).length };
    };
    const rowOf = async (pair: string) => {
      const { pj, pts } = await standingsRow(page, ctx.competitionId, nameOf(pair));
      return { pj, pts };
    };
    expect(await rowOf(w)).toEqual(expectRow(w, counted));
    expect(await rowOf(b)).toEqual(expectRow(b, counted));
    expect(await rowOf(c)).toEqual(expectRow(c, counted));
    expect(expectRow(w, counted).pj).toBe(2);

    for (const id of [wb, wc]) {
      await page.goto(`/match/${id}`);
      await waitForHxRedirect(page, /^\/admin\/competitions\/[^/]+$/, async () => {
        await page.getByRole('button', { name: 'Liberar partido' }).first().click();
        await expect(page.locator('#confirm-message')).toContainText('Liberar');
        await page.locator('#confirm-ok').click();
      });
    }
    await expect(page.locator(`#released-matches a[href="/match/${wb}"]`)).toHaveCount(1);
    await expect(page.locator(`#released-matches a[href="/match/${wc}"]`)).toHaveCount(1);

    // The top-up runs after the response: poll until B and C each have exactly one new match, never vs W.
    const fresh = async (pair: string, before: string[]) => (await live(pair)).filter((m) => !before.includes(m.id) && m.id !== wc);
    await expect.poll(async () => [(await fresh(b, liveBefore.b)).length, (await fresh(c, liveBefore.c)).length], { timeout: 20_000 }).toEqual([1, 1]);
    for (const [pair, before] of [[b, metBefore.b], [c, metBefore.c]] as const) {
      for (const m of await fresh(pair, liveBefore[pair === b ? 'b' : 'c'])) {
        expect([m.pair1, m.pair2]).not.toContain(w);
        // Never a repeat: the new opponent is a pair this one has never been assigned to.
        expect(before).not.toContain(m.pair1 === pair ? m.pair2 : m.pair1);
      }
    }
    expect((await live(w)).map((m) => m.id)).toEqual([wo]);

    // After the release only bx counts: W (withdrawn, nothing else played) is at 0/0.
    const remaining = [counted[1]];
    expect(await rowOf(w)).toEqual({ pj: 0, pts: 0 });
    expect(await rowOf(b)).toEqual(expectRow(b, remaining));
    expect(await rowOf(c)).toEqual(expectRow(c, remaining));

    const bPlayer = ctx.players[ctx.pairs.find((p: any) => p.id === b).player1Idx].email;
    await loginAs(page, bPlayer, PLAYER_PASSWORD);
    expect((await page.request.get(`/match/${wb}`)).status()).toBe(404);
    const newB = (await fresh(b, liveBefore.b))[0];
    for (const path of ['/', `/competition/${ctx.competitionId}`]) {
      await page.goto(path);
      await expect(page.locator(`a[href="/match/${wb}"]`)).toHaveCount(0);
    }
    await page.goto(`/competition/${ctx.competitionId}`);
    await expect(page.locator(`a[href="/match/${newB.id}"]`)).toHaveCount(1); // inside a closed Jornada accordion
  });
  test('02 withdraw W2, release W2-B2: B2 stays short, no new match, no Revancha', async ({ page }) => {
    const ctx = loadCtx() as any;
    const { competition, w, b, wb, players, pairs } = ctx.noRival;
    const nameOfB2 = pairs.find((p: any) => p.id === b).name as string;
    const api: ScenarioApi = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
    const live = async () => (await pairMatches(api, competition, b)).filter((m) => !['final', 'released'].includes(m.status));
    const before = (await live()).map((m) => m.id);
    expect(before).toHaveLength(2); // B2 has W2-B2 (about to be released) and its match against Y2

    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto(`/admin/competitions/${competition}`);
    await waitForHxRedirect(page, `/admin/competitions/${competition}`, async () => {
      await clickAction(page, `form[hx-post$="/withdraw-pair"]:has(input[name="pair_id"][value="${w}"]) button`, 'Retirar de la competición');
      await page.locator('#confirm-typed').fill('RETIRAR');
      await page.locator('#confirm-ok').click();
    });
    await page.goto(`/match/${wb}`);
    await waitForHxRedirect(page, /^\/admin\/competitions\/[^/]+$/, async () => {
      await page.getByRole('button', { name: 'Liberar partido' }).first().click();
      await page.locator('#confirm-ok').click();
    });

    // The top-up ran with the release: B2's only unmet pair is full, so nothing new may appear.
    await page.waitForTimeout(3000);
    expect((await live()).map((m) => m.id)).toEqual(before.filter((id) => id !== wb));
    await expect(page.locator('[data-testid="leveled-shortfall-notice"]')).toContainText('quedará con');
    await expect(page.locator('[data-testid="leveled-shortfall-notice"]')).toContainText(nameOfB2);

    // B2's own view: no new match and no Revancha on its competition page or its match pages.
    await loginAs(page, players[pairs.find((p: any) => p.id === b).player1Idx].email, PLAYER_PASSWORD);
    await page.goto(`/competition/${competition}`);
    await expect(page.getByText('Revancha')).toHaveCount(0);
    for (const id of before.filter((x) => x !== wb)) {
      await page.goto(`/match/${id}`);
      await expect(page.getByText('Revancha')).toHaveCount(0);
    }
    expect((await page.request.get(`/match/${wb}`)).status()).toBe(404);
  });

  test('03 league close: a pair short only for lack of an unmet opponent gets no penalty', async () => {
    const ctx = loadCtx() as any;
    const { competition, w, b, players, pairs } = ctx.noRival;
    const api: ScenarioApi = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
    const ctx2 = { ...ctx, api, competitionId: competition, players, pairs };
    const other = (m: any, p: string) => (m.pair1 === p ? m.pair2 : m.pair1);

    // 4-cycle W2-B2-Y2-X2-W2: W2-B2 is released. B2's other match is vs Y2.
    const bMatches = (await pairMatches(api, competition, b)).filter((m) => m.status !== 'released');
    expect(bMatches).toHaveLength(1);
    const y = other(bMatches[0], b);
    await driveTo(api, duelOf(ctx2, bMatches[0], pairs.find((p: any) => p.id === b)), 'final');
    const yMatches = (await pairMatches(api, competition, y)).filter((m) => m.status !== 'released' && m.status !== 'final');
    expect(yMatches).toHaveLength(1); // Y2-X2, assigned and unplayed
    const x = other(yMatches[0], y);
    expect((await pairMatches(api, competition, x)).filter((m) => m.status !== 'final')).toHaveLength(2); // X2-Y2 and X2-W2 (W2 withdrawn)

    // Move the window into the past (past the recovery week) so the daily cron closes the league.
    const stamp = (ago: number) => `${leagueDate(-ago)} 00:00:00.000Z`;
    await apiPatch(api, `/api/collections/competitions/records/${competition}`, {
      start_date: stamp(120), end_date: stamp(60), max_pending_matches: 1,
    });
    const resp = await fetch(`${ctx.baseURL}/api/crons/pending-match-penalties`, { method: 'POST', headers: { Authorization: ctx.suToken } });
    expect(resp.ok).toBe(true);

    await expect.poll(async () => (await apiGet(api, `/api/collections/competitions/records/${competition}`)).finalized, { timeout: 20_000 }).toBe(true);
    const rows = (await apiGet(api, `/api/collections/penalties/records?perPage=200&filter=${encodeURIComponent(`competition="${competition}"`)}`)).items;
    const totals = new Map<string, number>();
    for (const r of rows) if (!r.voided) totals.set(r.pair, (totals.get(r.pair) ?? 0) + r.amount);
    expect(totals.get(b) ?? 0).toBe(0); // short only because no unmet opponent existed
    expect(totals.get(w) ?? 0).toBe(0); // withdrawn
    expect(totals.get(y)).toBe(1); // one assigned unplayed match
    expect(totals.get(x)).toBe(1); // X2-Y2 counts, X2-W2 does not (W2 withdrawn)
  });
});
