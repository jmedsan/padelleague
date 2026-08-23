import { execSync, spawn, ChildProcess } from 'child_process';
import { mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';

const PORT = 8099;
const BASE_URL = `http://localhost:${PORT}`;
const BINARY = '/tmp/padelleague-test';

let serverProcess: ChildProcess;

export default async function globalSetup() {
  execSync(`go build -o ${BINARY} .`, {
    cwd: join(__dirname, '..'),
    stdio: 'inherit',
  });

  const dataDir = mkdtempSync(join(tmpdir(), 'padelleague-test-'));

  serverProcess = spawn(BINARY, ['serve', `--http=0.0.0.0:${PORT}`, `--dir=${dataDir}`], {
    env: {
      ...process.env,
      PB_ADMIN_EMAIL: 'admin@test.com',
      PB_ADMIN_PASSWORD: 'testpass123456',
      APP_ADMIN_EMAIL: 'admin@test.com',
      APP_ADMIN_PASSWORD: 'testpass123456',
      APP_PLAYER_EMAIL: 'player@test.com',
      APP_PLAYER_PASSWORD: 'testpass123456',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  (globalThis as any).__E2E_SERVER = serverProcess;
  (globalThis as any).__E2E_DATA_DIR = dataDir;

  await waitForServer(BASE_URL + '/login', 30_000);
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
