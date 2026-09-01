import { execSync, spawn, ChildProcess } from 'child_process';
import { mkdtempSync, writeFileSync, mkdirSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';

const PORT = 8099;
const BASE_URL = `http://localhost:${PORT}`;
const BINARY = '/tmp/padelleague-test';

export const ADMIN_EMAIL = 'admin@test.com';
export const ADMIN_PASSWORD = 'testpass123456';
export const ADMIN_NAME = 'Test Admin';
export const PLAYER1_EMAIL = 'player@test.com';
export const PLAYER1_PASSWORD = 'testpass123456';
export const PLAYER1_NAME = 'Test Player';
export const PLAYER2_EMAIL = 'player2@test.com';
export const PLAYER2_PASSWORD = 'testpass123456';
export const PLAYER2_NAME = 'Test Player 2';
export const PLAYER3_EMAIL = 'player3@test.com';
export const PLAYER3_PASSWORD = 'testpass123456';
export const PLAYER3_NAME = 'Test Player 3';
export const PLAYER4_EMAIL = 'player4@test.com';
export const PLAYER4_PASSWORD = 'testpass123456';
export const PLAYER4_NAME = 'Test Player 4';

let serverProcess: ChildProcess;

export default async function globalSetup() {
  execSync(`go build -o ${BINARY} .`, {
    cwd: join(__dirname, '..'),
    stdio: 'inherit',
  });

  const dataDir = mkdtempSync(join(tmpdir(), 'padelleague-test-'));

  serverProcess = spawn(BINARY, ['serve', `--http=0.0.0.0:${PORT}`, `--dir=${dataDir}`], {
    env: {
      PATH: process.env.PATH || '',
      HOME: process.env.HOME || '',
      PB_ADMIN_EMAIL: ADMIN_EMAIL,
      PB_ADMIN_PASSWORD: ADMIN_PASSWORD,
      APP_ADMIN1_EMAIL: ADMIN_EMAIL,
      APP_ADMIN1_PASSWORD: ADMIN_PASSWORD,
      APP_ADMIN1_NAME: ADMIN_NAME,
      APP_PLAYER_EMAIL: PLAYER1_EMAIL,
      APP_PLAYER_PASSWORD: PLAYER1_PASSWORD,
      APP_PLAYER_NAME: PLAYER1_NAME,
      APP_PLAYER2_EMAIL: PLAYER2_EMAIL,
      APP_PLAYER2_PASSWORD: PLAYER2_PASSWORD,
      APP_PLAYER2_NAME: PLAYER2_NAME,
      APP_ENV: 'dev',
      APP_DEV_TOOLS: 'true',
    },
    stdio: ['ignore', 'pipe', 'inherit'],
  });

  serverProcess.stdout?.resume();

  (globalThis as any).__E2E_SERVER = serverProcess;
  (globalThis as any).__E2E_DATA_DIR = dataDir;

  await waitForServer(BASE_URL + '/login', 60_000);
  await seedTestData();
}

async function waitForServer(url: string, timeoutMs: number) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(url);
      if (res.ok || res.status === 200) return;
    } catch {
      // server not ready yet
    }
    await new Promise(r => setTimeout(r, 500));
  }
  throw new Error(`Server did not start within ${timeoutMs}ms`);
}

async function seedTestData() {
  // Login as superuser via PocketBase API
  const authResp = await fetch(`${BASE_URL}/api/collections/_superusers/auth-with-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identity: ADMIN_EMAIL, password: ADMIN_PASSWORD }),
  });
  const authData = await authResp.json();
  if (!authData.token) {
    throw new Error(`Superuser auth failed: ${JSON.stringify(authData)}`);
  }
  const adminToken = authData.token;

  // Get player IDs
  const player1 = await getUser(PLAYER1_EMAIL, adminToken);
  const player2 = await getUser(PLAYER2_EMAIL, adminToken);
  const player3 = await createPlayer(PLAYER3_EMAIL, PLAYER3_PASSWORD, PLAYER3_NAME, adminToken);
  const player4 = await createPlayer(PLAYER4_EMAIL, PLAYER4_PASSWORD, PLAYER4_NAME, adminToken);

  // Create pair 1 (player1 + player2) and pair 2 (admin + player1) via admin form
  const pair1Id = await createPair('Pareja Alpha', player1.id, player2.id, adminToken);
  const pair2Id = await createPair('Pareja Beta', player1.id, (await getUser(ADMIN_EMAIL, adminToken)).id, adminToken);
  // pair3: no overlap with pair1/pair2 players — used for admin-notif scratch matches
  // so the admin is NOT a participant and receives match-progress notifications
  const pair3Id = await createPair('Pareja Gamma', player3.id, player4.id, adminToken);

  // Create competition
  const compId = await createCompetition('Liga E2E Test', 'league', adminToken);

  // Add pairs to competition
  await addPairToCompetition(compId, pair1Id, adminToken);
  await addPairToCompetition(compId, pair2Id, adminToken);

  // Generate fixtures
  await generateFixtures(compId, adminToken);

  // Get match IDs
  const matchesResp = await fetch(`${BASE_URL}/api/collections/matches/records?filter=competition='${compId}'`, {
    headers: { 'Authorization': adminToken },
  });
  const matchesData = await matchesResp.json();
  const matches = matchesData.items || [];

  // Two pairs produce exactly one round-robin match, and the desktop and
  // mobile projects share one database. Tests that submit a score or accept a
  // proposal mutate that match, so whichever project ran second found it
  // already played and failed. Add one scratch match per project so those
  // tests get an untouched match each. Two mutating tests across two projects
  // means four, since a test that confirms a match leaves nothing pending for
  // the next one.
  // Slots 0-1 use pair1 vs pair2; slot 2 (admin-notif) uses pair1 vs pair3
  // so the admin is NOT a match participant and receives the notification;
  // slot 3 (mobile-lifecycle) uses pair1 vs pair2.
  // pair3 is NOT added to the competition to avoid changing fixture generation.
  for (let i = 0; i < 10; i++) {
    const usePair3 = i >= 4 && i < 6; // slots 0-1 = indices 0-3, slot 2 = indices 4-5
    const extra = await fetch(`${BASE_URL}/api/collections/matches/records`, {
      method: 'POST',
      headers: { 'Authorization': adminToken, 'Content-Type': 'application/json' },
      body: JSON.stringify({
        competition: compId,
        pair1: pair1Id,
        pair2: usePair3 ? pair3Id : pair2Id,
        status: 'pending',
        round_number: i + 2,
      }),
    });
    if (!extra.ok) throw new Error(`scratch match ${i}: ${extra.status} ${await extra.text()}`);
    matches.push(await extra.json());
  }

  // Create a venue
  const venueId = await createVenue('Pista Central', adminToken);

  // Save test data for specs
  const testData = {
    adminToken,
    player1: { id: player1.id, email: PLAYER1_EMAIL },
    player2: { id: player2.id, email: PLAYER2_EMAIL },
    pair1Id,
    pair2Id,
    competitionId: compId,
    matchIds: matches.map((m: any) => m.id),
    venueId,
  };

  mkdirSync(join(__dirname, '.test-data'), { recursive: true });
  writeFileSync(join(__dirname, '.test-data/seed.json'), JSON.stringify(testData, null, 2));
}

async function getUser(email: string, token: string) {
  const resp = await fetch(`${BASE_URL}/api/collections/users/records?filter=email='${email}'`, {
    headers: { 'Authorization': token },
  });
  const data = await resp.json();
  return data.items[0];
}

async function createPlayer(email: string, password: string, name: string, token: string): Promise<{ id: string; email: string }> {
  const resp = await fetch(`${BASE_URL}/api/collections/users/records`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': token },
    body: JSON.stringify({ email, password, passwordConfirm: password, display_name: name, roles: ['player'], verified: true }),
  });
  if (!resp.ok) throw new Error(`createPlayer: ${resp.status} ${await resp.text()}`);
  const data = await resp.json();
  return { id: data.id, email };
}

async function createPair(name: string, player1Id: string, player2Id: string, token: string): Promise<string> {
  const resp = await fetch(`${BASE_URL}/api/collections/pairs/records`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': token },
    body: JSON.stringify({ name, player1: player1Id, player2: player2Id }),
  });
  const data = await resp.json();
  return data.id;
}

async function createCompetition(name: string, type: string, token: string): Promise<string> {
  const resp = await fetch(`${BASE_URL}/api/collections/competitions/records`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': token },
    body: JSON.stringify({ name, type, active: true }),
  });
  const data = await resp.json();
  return data.id;
}

async function addPairToCompetition(compId: string, pairId: string, token: string) {
  const resp = await fetch(`${BASE_URL}/api/collections/competitions/records/${compId}`, {
    headers: { 'Authorization': token },
  });
  const comp = await resp.json();
  const pairs = comp.pairs || [];
  pairs.push(pairId);
  await fetch(`${BASE_URL}/api/collections/competitions/records/${compId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'Authorization': token },
    body: JSON.stringify({ pairs }),
  });
}

async function generateFixtures(compId: string, token: string) {
  // Use the admin HTML endpoint with cookie-based auth
  const loginResp = await fetch(`${BASE_URL}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: `email=${ADMIN_EMAIL}&password=${ADMIN_PASSWORD}`,
    redirect: 'manual',
  });
  const cookies = loginResp.headers.getSetCookie?.() || [];
  const cookieStr = cookies.join('; ');

  await fetch(`${BASE_URL}/admin/competitions/${compId}/generate`, {
    method: 'POST',
    headers: {
      'Cookie': cookieStr,
      'HX-Request': 'true',
    },
  });
}

async function createVenue(name: string, token: string): Promise<string> {
  const resp = await fetch(`${BASE_URL}/api/collections/venues/records`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': token },
    body: JSON.stringify({ name, address: 'Calle Test 1' }),
  });
  const data = await resp.json();
  return data.id;
}
