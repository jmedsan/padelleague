import { test, expect } from '@playwright/test';
import {
  MAILPIT_URL, PLAYER_PASSWORD, ScenarioApi, ScenarioData, addPairToCompetition, apiGet, apiPatch,
  createCompetition, createPairs, createPlayers, generateAssignments, loadCtx, printLinks,
  publishCalendar, saveCtx, uniqueSuffix,
} from '../scenario-helpers';

let ctx: ScenarioData;

async function seedMatchID(api: ScenarioApi, compId: string): Promise<string> {
  const list = await apiGet(api, `/api/collections/matches/records?filter=${encodeURIComponent(
    `competition='${compId}'`)}&perPage=5`);
  expect(list.totalItems).toBe(1);
  return list.items[0].id;
}

async function playerToken(baseURL: string, email: string): Promise<string> {
  const resp = await fetch(`${baseURL}/api/collections/users/auth-with-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identity: email, password: PLAYER_PASSWORD }),
  });
  if (!resp.ok) throw new Error(`login ${email}: ${resp.status}`);
  return (await resp.json()).token;
}

// proposeSchedule runs the real flow up to the proposal: a player proposes a
// date, time and venue and the rival does not answer.
async function proposeSchedule(api: ScenarioApi, matchID: string, proposerEmail: string): Promise<void> {
  const post = async (email: string, path: string, form: Record<string, string>): Promise<void> => {
    const resp = await fetch(`${api.baseURL}${path}`, {
      method: 'POST',
      headers: {
        Cookie: `pb_auth=${await playerToken(api.baseURL, email)}`,
        'HX-Request': 'true',
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams(form).toString(),
      redirect: 'manual',
    });
    if (resp.status >= 400) throw new Error(`POST ${path}: ${resp.status} ${await resp.text()}`);
  };
  await post(proposerEmail, `/match/${matchID}/thread/proposal`, { date: '2027-09-15', time: '18:30', venue_text: 'Padel 360' });
  const pending = await apiGet(api, `/api/collections/match_messages/records?filter=${encodeURIComponent(
    `match='${matchID}' && type='scheduling_proposal' && proposal_status='pending'`)}&perPage=5`);
  expect(pending.totalItems).toBe(1);
}

test.describe('chat email for a match with a pending date and place proposal via Mailpit', () => {
  test.describe.configure({ retries: 0 });

  test('00 seed: as mail-chat, plus a date, time and venue proposed by p01 and not accepted', async () => {
    const raw = loadCtx();
    const api: ScenarioApi = { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie };
    const suffix = uniqueSuffix();
    const players = await createPlayers(api, 4, suffix);
    await apiPatch(api, `/api/collections/users/records/${players[3].id}`, { verified: false });
    const pairs = await createPairs(api, players, suffix);
    const competitionId = await createCompetition(api, `Chat con propuesta ${suffix}`, 0, 0);
    for (const pair of pairs) await addPairToCompetition(api, competitionId, pair.id);
    await generateAssignments(api, competitionId);
    await publishCalendar(api, competitionId);
    const matchID = await seedMatchID(api, competitionId);
    const match = await apiGet(api, `/api/collections/matches/records/${matchID}`);
    const pairOf = (id: string) => pairs.find((p) => p.id === id)!;
    await proposeSchedule(api, matchID, players[pairOf(match.pair1).player1Idx].email);

    ctx = { ...raw, competitionId, players, pairs, stage: 'blank' };
    saveCtx(ctx);

    const [home, away] = await Promise.all(pairs.map((p) => apiGet(api, `/api/collections/pairs/records/${p.id}`)));
    const matchPath = `/match/${matchID}`;
    console.log(`\nMatch page: ${ctx.baseURL}${matchPath}`);
    console.log(`Mailpit inbox: ${MAILPIT_URL} (start it with: make mail)`);
    console.log(`Match: ${home.name} vs ${away.name}`);
    for (const pair of pairs) {
      for (const idx of [pair.player1Idx, pair.player2Idx]) {
        console.log(`  ${pair.name}  ${players[idx].email}  password ${PLAYER_PASSWORD}  verified ${idx === 3 ? 'no' : 'yes'}`);
      }
    }
    printLinks(ctx.baseURL, pairs.flatMap((pair) => [pair.player1Idx, pair.player2Idx].map((idx) => ({
      label: `Sign in as ${pair.name} / ${players[idx].email}`,
      email: players[idx].email, password: PLAYER_PASSWORD, path: matchPath,
    }))));
  });
});
