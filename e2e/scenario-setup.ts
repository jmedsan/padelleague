import { mkdirSync, writeFileSync } from 'fs';
import { join } from 'path';
import { spawnServer, superuserLogin } from './server';
import { ADMIN_EMAIL, ADMIN_PASSWORD, ADMIN_NAME } from './global-setup';
import { buildToStage, ScenarioApi } from './scenario-helpers';
import { SCENARIOS } from './scenario-registry';

const PORT = process.env.E2E_PORT ? Number(process.env.E2E_PORT) : 8098;
const SCENARIO = process.env.SCENARIO ?? '';
const TEST_DATA_DIR = join(__dirname, '.test-data');

export default async function scenarioSetup() {
  const scenario = SCENARIOS[SCENARIO];
  if (!scenario) throw new Error(`Unknown scenario: ${SCENARIO}`);

  const keepAlive = process.env.E2E_KEEP === '1';

  const handle = await spawnServer(PORT, {
    keepAlive,
    extraEnv: {
      PB_ADMIN_EMAIL: ADMIN_EMAIL,
      PB_ADMIN_PASSWORD: ADMIN_PASSWORD,
      APP_ADMIN1_EMAIL: ADMIN_EMAIL,
      APP_ADMIN1_PASSWORD: ADMIN_PASSWORD,
      APP_ADMIN1_NAME: ADMIN_NAME,
      APP_ENV: 'dev',
      APP_DEV_TOOLS: 'true',
    },
  });

  (globalThis as any).__SCENARIO_SERVER = handle;

  const suToken = await superuserLogin(handle.baseURL, ADMIN_EMAIL, ADMIN_PASSWORD);
  const adminCookie = await getAdminCookie(handle.baseURL);

  const api: ScenarioApi = { baseURL: handle.baseURL, suToken, adminCookie };
  const ctx = await buildToStage(api, scenario.startStage);

  mkdirSync(TEST_DATA_DIR, { recursive: true });

  writeFileSync(
    join(TEST_DATA_DIR, 'scenario.json'),
    JSON.stringify({
      baseURL: handle.baseURL,
      suToken,
      adminCookie,
      competitionId: ctx.competitionId,
      players: ctx.players,
      pairs: ctx.pairs,
      target: ctx.target,
      open: ctx.open,
      stage: ctx.stage,
    }, null, 2),
  );
}

async function getAdminCookie(baseURL: string): Promise<string> {
  const resp = await fetch(`${baseURL}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: `email=${ADMIN_EMAIL}&password=${ADMIN_PASSWORD}`,
    redirect: 'manual',
  });
  const cookies = resp.headers.getSetCookie?.() || [];
  return cookies.join('; ');
}
