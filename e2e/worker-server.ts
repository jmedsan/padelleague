import { test as base } from '@playwright/test';
import { seedEnv, seedTestData } from './global-setup';
import { setWorkerPort } from './run-dir';
import { buildBinary, killServer, spawnServer, type ServerHandle } from './server';
import { rmSync } from 'fs';
import { createServer } from 'net';

interface WorkerServerFixtures {
  workerServer: ServerHandle;
}

// Asks the OS for a free TCP port instead of guessing one, so two Playwright
// processes running at once on this machine (e.g. two workers, or two
// separate `npx playwright test` invocations from different sessions) never
// bind the same port and silently share one server/database. Same approach
// as find-free-port.mjs (used by `make e2e` itself), inlined here as an
// awaitable helper since worker-server.ts needs it in-process per worker.
async function findFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.on('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : undefined;
      server.close(() => {
        if (port === undefined) reject(new Error('findFreePort: no port assigned'));
        else resolve(port);
      });
    });
  });
}

// workerServer is a worker-scoped fixture: Playwright creates it once per
// worker process (not once per test) and tears it down when the worker
// exits. Each worker gets its own server + database + seed.json, so workers
// run fully independent tests in parallel without sharing state — the same
// isolation `make e2e`'s single server previously got from workers=1.
export const test = base.extend<{}, WorkerServerFixtures>({
  workerServer: [async ({}, use) => {
    const port = await findFreePort();
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
