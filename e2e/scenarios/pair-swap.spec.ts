import { test, expect } from '@playwright/test';
import { ADMIN_EMAIL, ADMIN_PASSWORD, loginAs } from '../helpers';
import {
  PLAYER_PASSWORD, ScenarioApi, ScenarioData, apiGet, apiPost, loadCtx, uniqueSuffix,
} from '../scenario-helpers';

let ctx: ScenarioData;
let api: ScenarioApi;

// The auth endpoint is rate limited, so each player signs in once and the
// token is reused for every route call.
const tokens = new Map<string, string>();

async function playerPost(email: string, path: string, form: Record<string, string>): Promise<void> {
  let token = tokens.get(email);
  if (!token) {
    const login = await fetch(`${ctx.baseURL}/api/collections/users/auth-with-password`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ identity: email, password: PLAYER_PASSWORD }),
    });
    if (!login.ok) throw new Error(`login ${email}: ${login.status}`);
    token = (await login.json()).token as string;
    tokens.set(email, token);
  }
  const resp = await fetch(`${ctx.baseURL}${path}`, {
    method: 'POST',
    headers: {
      Cookie: `pb_auth=${token}`,
      'HX-Request': 'true',
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: new URLSearchParams(form).toString(),
    redirect: 'manual',
  });
  if (resp.status >= 400) throw new Error(`POST ${path}: ${resp.status} ${await resp.text()}`);
}

async function pairMatch() {
  const pairID = ctx.pairs[0].id;
  const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
    `competition='${ctx.competitionId}' && (pair1='${pairID}' || pair2='${pairID}')`)}&perPage=1`);
  return list.items[0];
}

test.describe('timeline after a pair member is swapped', () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(() => {
    ctx = loadCtx();
    api = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
  });

  test('00 ready: a played match, a spare player, swap Player 1 in the admin', async () => {
    const match = await pairMatch();
    const mine = ctx.pairs[0];
    const rival = ctx.pairs.find((p) => p.id === (match.pair1 === mine.id ? match.pair2 : match.pair1))!;
    const proposer = ctx.players[mine.player1Idx];
    const rivalPlayer = ctx.players[rival.player1Idx];

    // Confirmed date and place, through the thread routes.
    await playerPost(proposer.email, `/match/${match.id}/thread/proposal`,
      { date: '2027-09-15', time: '18:30', venue_text: 'Padel 360' });
    const dates = await apiGet(api, `/api/collections/match_messages/records?filter=${encodeURIComponent(
      `match='${match.id}' && type='scheduling_proposal' && proposal_status='pending'`)}&perPage=5`);
    if (dates.totalItems !== 1) throw new Error(`expected one pending date proposal, got ${dates.totalItems}`);
    await playerPost(rivalPlayer.email,
      `/match/${match.id}/thread/proposal/${dates.items[0].id}/respond`, { action: 'accept' });

    // A chat message and a result from the proposer, confirmed by the rival.
    await playerPost(proposer.email, `/match/${match.id}/thread/message`, { content: 'Nos vemos en la pista' });
    await playerPost(proposer.email, `/match/${match.id}/submit`, { scores: '6-3 6-4' });
    const results = await apiGet(api, `/api/collections/match_messages/records?filter=${encodeURIComponent(
      `match='${match.id}' && type='result_submission' && proposal_status='pending'`)}&perPage=5`);
    if (results.totalItems !== 1) throw new Error(`expected one pending result, got ${results.totalItems}`);
    await playerPost(rivalPlayer.email,
      `/match/${match.id}/thread/proposal/${results.items[0].id}/respond`, { action: 'accept' });

    // The spare player belongs to no pair.
    const spareEmail = `spare-${uniqueSuffix()}@test.local`;
    await apiPost(api, '/api/collections/users/records', {
      email: spareEmail, password: PLAYER_PASSWORD, passwordConfirm: PLAYER_PASSWORD,
      display_name: 'Spare Player', roles: ['player'], verified: true, gender: 'male',
    });

    const adminLink = `${ctx.baseURL}/dev-login?${new URLSearchParams({
      email: ADMIN_EMAIL, password: ADMIN_PASSWORD, next: '/admin/pairs' })}`;
    console.log(`\nAdmin pairs page (one click): ${adminLink}`);
    console.log(`Spare player: ${spareEmail} / ${PLAYER_PASSWORD}`);
    console.log(`Match page: ${ctx.baseURL}/match/${match.id}`);
    console.log(`Edit ${mine.name}, replace Player 1 with Spare Player, pick a captain, then check the timeline still shows "Player 1 (${mine.name})".`);
  });

  test('01 the seeded timeline labels both pairs', async ({ page }) => {
    const match = await pairMatch();
    const mine = ctx.pairs[0];
    await loginAs(page, ctx.players[mine.player1Idx].email, PLAYER_PASSWORD);
    await page.goto(`/match/${match.id}`);
    await expect(page.getByText(`Player 1 (${mine.name})`).locator('visible=true').first()).toBeVisible({ timeout: 10000 });
  });
});
