import { test as base } from '@playwright/test';
import { seedEnv, seedTestData } from './global-setup';
import { setWorkerPort } from './run-dir';
import { buildBinary, killServer, spawnServer, type ServerHandle } from './server';
import { rmSync } from 'fs';

// Base port for the first Playwright worker; each subsequent worker gets
// basePort + parallelIndex, so N workers never collide on a port or a
// database.
export const WORKER_BASE_PORT = 8200;

interface WorkerServerFixtures {
  workerServer: ServerHandle;
}

// workerServer is a worker-scoped fixture: Playwright creates it once per
// worker process (not once per test) and tears it down when the worker
// exits. Each worker gets its own server + database + seed.json, so workers
// run fully independent tests in parallel without sharing state — the same
// isolation `make e2e`'s single server previously got from workers=1.
export const test = base.extend<{}, WorkerServerFixtures>({
  workerServer: [async ({}, use, workerInfo) => {
    const port = WORKER_BASE_PORT + workerInfo.parallelIndex;
    setWorkerPort(port);

    const binary = await sharedBinary();
    const handle = await spawnServer(port, { extraEnv: seedEnv(), binary });
    await seedTestData(handle.baseURL, port);

    await use(handle);

    killServer(handle);
    try {
      rmSync(handle.dataDir, { recursive: true, force: true });
    } catch {
      // best effort cleanup
    }
  }, { scope: 'worker', auto: true }],

  // Overrides Playwright's built-in baseURL fixture (normally a static
  // string from playwright.config.ts's `use`) with this worker's own
  // server, resolved after workerServer has spawned it.
  baseURL: async ({ workerServer }, use) => {
    await use(workerServer.baseURL);
  },
});

export { expect } from '@playwright/test';

// Building the Go binary takes ~6s and is identical for every worker.
// Playwright workers are separate OS processes, so a module-level cache
// only de-dupes retries within one worker — `make e2e` sets E2E_BINARY to a
// path it builds once up front, which every worker then reuses instead of
// rebuilding. Falls back to a per-worker build when E2E_BINARY is unset
// (e.g. running `npx playwright test` directly).
let binaryPromise: Promise<string> | undefined;
function sharedBinary(): Promise<string> {
  if (process.env.E2E_BINARY) return Promise.resolve(process.env.E2E_BINARY);
  if (!binaryPromise) binaryPromise = buildBinary();
  return binaryPromise;
}
