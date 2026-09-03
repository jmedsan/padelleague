#!/usr/bin/env node
// Generates a realistic sample SQLite database by booting the app,
// creating a full season through the UI (hooks fire, notifications created),
// playing half the matches, and copying the resulting DB.
//
// Usage: node seed/generate-sample-db.mjs
// Requires: the Go binary to be built first (make build)

import { execSync, spawn } from 'child_process';
import { cpSync, mkdtempSync, rmSync, existsSync } from 'fs';
import { tmpdir } from 'os';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const BINARY = '/tmp/padelleague-sample-gen';
const PORT = 8098;
const BASE = `http://localhost:${PORT}`;
const OUTPUT = join(__dirname, 'sample.db');

const ADMIN_EMAIL = 'admin@dalefuerte.com';
const ADMIN_PASSWORD = 'Admin123456!';
const ADMIN_NAME = 'Administrador';
const PLAYER_PASSWORD = 'Player123456!';

const PLAYERS = [
  { name: 'Luis García', email: 'luis@dalefuerte.com', gender: 'male' },
  { name: 'María López', email: 'maria@dalefuerte.com', gender: 'female' },
  { name: 'Carlos Ruiz', email: 'carlos@dalefuerte.com', gender: 'male' },
  { name: 'Ana Martínez', email: 'ana@dalefuerte.com', gender: 'female' },
  { name: 'Pedro Sánchez', email: 'pedro@dalefuerte.com', gender: 'male' },
  { name: 'Laura Fernández', email: 'laura@dalefuerte.com', gender: 'female' },
  { name: 'Javier Moreno', email: 'javier@dalefuerte.com', gender: 'male' },
  { name: 'Elena Torres', email: 'elena@dalefuerte.com', gender: 'female' },
];

const PAIRS = [
  { name: 'Los Ases', p1: 0, p2: 1 },
  { name: 'Fuego Cruzado', p1: 2, p2: 3 },
  { name: 'Sin Piedad', p1: 4, p2: 5 },
  { name: 'Dale Caña', p1: 6, p2: 7 },
];

// First leg scores (6 matches in a round-robin of 4 pairs)
const FIRST_LEG = [
  { home: 0, away: 3, scores: '6-3 6-3' },   // Los Ases vs Dale Caña
  { home: 1, away: 2, scores: '6-2 6-2' },   // Fuego Cruzado vs Sin Piedad
  { home: 0, away: 2, scores: '6-3 6-3' },   // Los Ases vs Sin Piedad
  { home: 3, away: 1, scores: '6-4 6-4' },   // Dale Caña vs Fuego Cruzado
  { home: 0, away: 1, scores: '6-3 6-3' },   // Los Ases vs Fuego Cruzado
  { home: 2, away: 3, scores: '6-3 6-4' },   // Sin Piedad vs Dale Caña
];

let suToken = '';
let playerIds = [];
let pairIds = [];
let competitionId = '';
let cookies = '';

async function main() {
  console.log('Building binary...');
  execSync(`go build -o ${BINARY} .`, { cwd: ROOT, stdio: 'inherit' });

  const dataDir = mkdtempSync(join(tmpdir(), 'padelleague-sample-'));
  console.log(`Temp DB dir: ${dataDir}`);

  const server = spawn(BINARY, ['serve', `--http=0.0.0.0:${PORT}`, `--dir=${dataDir}`], {
    env: {
      PATH: process.env.PATH || '',
      HOME: process.env.HOME || '',
      PB_ADMIN_EMAIL: ADMIN_EMAIL,
      PB_ADMIN_PASSWORD: ADMIN_PASSWORD,
      APP_ADMIN1_EMAIL: ADMIN_EMAIL,
      APP_ADMIN1_PASSWORD: ADMIN_PASSWORD,
      APP_ADMIN1_NAME: ADMIN_NAME,
      APP_ENV: 'dev',
    },
    stdio: ['ignore', 'pipe', 'inherit'],
  });
  server.stdout.resume();

  try {
    await waitForServer();
    console.log('Server ready');

    await authenticate();
    await createPlayers();
    await createPairsAndCompetition();
    await generateFixtures();
    await playFirstLeg();

    // Copy the DB
    const srcDb = join(dataDir, 'data.db');
    if (!existsSync(srcDb)) throw new Error(`DB not found at ${srcDb}`);
    cpSync(srcDb, OUTPUT);
    console.log(`\nSample DB saved to: ${OUTPUT}`);
    console.log('Done!');
  } finally {
    server.kill('SIGTERM');
    await new Promise(r => setTimeout(r, 1000));
    try { rmSync(dataDir, { recursive: true, force: true }); } catch {}
  }
}

async function waitForServer() {
  const start = Date.now();
  while (Date.now() - start < 30000) {
    try {
      const r = await fetch(`${BASE}/login`);
      if (r.ok) return;
    } catch {}
    await sleep(500);
  }
  throw new Error('Server did not start');
}

async function authenticate() {
  // Superuser token
  const r = await post('/api/collections/_superusers/auth-with-password', {
    identity: ADMIN_EMAIL, password: ADMIN_PASSWORD,
  });
  suToken = r.token;

  // HTML login for cookie-based form submissions
  const loginResp = await fetch(`${BASE}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: `email=${ADMIN_EMAIL}&password=${ADMIN_PASSWORD}`,
    redirect: 'manual',
  });
  cookies = (loginResp.headers.getSetCookie?.() || []).join('; ');
}

async function createPlayers() {
  console.log('Creating players...');
  for (const p of PLAYERS) {
    const r = await post('/api/collections/users/records', {
      email: p.email, password: PLAYER_PASSWORD, passwordConfirm: PLAYER_PASSWORD,
      display_name: p.name, roles: ['player'], gender: p.gender, verified: true,
    });
    playerIds.push(r.id);
    console.log(`  ${p.name} (${r.id})`);
  }
}

async function createPairsAndCompetition() {
  console.log('Creating pairs...');
  for (const p of PAIRS) {
    const r = await post('/api/collections/pairs/records', {
      name: p.name, player1: playerIds[p.p1], player2: playerIds[p.p2],
    });
    pairIds.push(r.id);
    console.log(`  ${p.name} (${r.id})`);
  }

  console.log('Creating competition...');
  const now = new Date();
  const start = new Date(now.getTime() - 20 * 86400000).toISOString().slice(0, 10);
  const end = new Date(now.getTime() + 20 * 86400000).toISOString().slice(0, 10);
  const comp = await post('/api/collections/competitions/records', {
    name: 'Liga Otoño 2026', type: 'league', active: true, play_twice: true,
    gender_type: 'free', start_date: start, end_date: end,
  });
  competitionId = comp.id;
  console.log(`  Competition: ${comp.id}`);

  for (const pid of pairIds) {
    const existing = await get(`/api/collections/competitions/records/${competitionId}`);
    const pairs = existing.pairs || [];
    pairs.push(pid);
    await patch(`/api/collections/competitions/records/${competitionId}`, { pairs });
  }
}

async function generateFixtures() {
  console.log('Generating fixtures...');
  await fetch(`${BASE}/admin/competitions/${competitionId}/generate`, {
    method: 'POST',
    headers: { Cookie: cookies, 'HX-Request': 'true' },
  });
  // Wait for generation
  await sleep(1000);
  const matches = await get(`/api/collections/matches/records?filter=competition='${competitionId}'&sort=round_number`);
  console.log(`  ${matches.items.length} matches generated`);
}

async function playFirstLeg() {
  console.log('Playing first leg (6 matches)...');
  const allMatches = await get(`/api/collections/matches/records?filter=competition='${competitionId}'&sort=round_number&perPage=50`);
  const matches = allMatches.items;

  for (let i = 0; i < Math.min(FIRST_LEG.length, matches.length); i++) {
    const plan = FIRST_LEG[i];
    const match = matches[i];
    if (!match) break;

    const submitterPairId = pairIds[plan.home];
    const confirmerPairId = pairIds[plan.away];

    // Login as submitter (player1 of home pair)
    const submitterEmail = PLAYERS[PAIRS[plan.home].p1].email;
    await loginAsPlayer(submitterEmail);

    // Submit score via form
    await submitScore(match.id, plan.scores);
    await sleep(300);

    // Login as confirmer (player1 of away pair)
    const confirmerEmail = PLAYERS[PAIRS[plan.away].p1].email;
    await loginAsPlayer(confirmerEmail);

    // Confirm the score
    await confirmScore(match.id);
    await sleep(300);

    console.log(`  Match ${i + 1}: ${PAIRS[plan.home].name} vs ${PAIRS[plan.away].name} — ${plan.scores}`);
  }

  // Re-login as admin
  await authenticate();
}

async function loginAsPlayer(email) {
  const resp = await fetch(`${BASE}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: `email=${email}&password=${PLAYER_PASSWORD}`,
    redirect: 'manual',
  });
  cookies = (resp.headers.getSetCookie?.() || []).join('; ');
}

async function submitScore(matchId, scores) {
  const sets = scores.split(' ');
  const body = new URLSearchParams();
  for (let i = 0; i < 3; i++) {
    if (sets[i]) {
      const [g1, g2] = sets[i].split('-');
      body.set(`set${i + 1}_1`, g1);
      body.set(`set${i + 1}_2`, g2);
    } else {
      body.set(`set${i + 1}_1`, '');
      body.set(`set${i + 1}_2`, '');
    }
  }
  await fetch(`${BASE}/match/${matchId}/submit-score`, {
    method: 'POST',
    headers: { Cookie: cookies, 'Content-Type': 'application/x-www-form-urlencoded', 'HX-Request': 'true' },
    body: body.toString(),
    redirect: 'manual',
  });
}

async function confirmScore(matchId) {
  await fetch(`${BASE}/match/${matchId}/confirm-score`, {
    method: 'POST',
    headers: { Cookie: cookies, 'HX-Request': 'true' },
    redirect: 'manual',
  });
}

// HTTP helpers
async function post(path, data) {
  const r = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: suToken },
    body: JSON.stringify(data),
  });
  if (!r.ok) throw new Error(`POST ${path}: ${r.status} ${await r.text()}`);
  return r.json();
}

async function get(path) {
  const r = await fetch(`${BASE}${path}`, {
    headers: { Authorization: suToken },
  });
  if (!r.ok) throw new Error(`GET ${path}: ${r.status}`);
  return r.json();
}

async function patch(path, data) {
  const r = await fetch(`${BASE}${path}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', Authorization: suToken },
    body: JSON.stringify(data),
  });
  if (!r.ok) throw new Error(`PATCH ${path}: ${r.status}`);
  return r.json();
}

function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

main().catch(err => { console.error(err); process.exit(1); });
