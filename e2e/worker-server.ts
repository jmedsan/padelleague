import { test as base } from '@playwright/test';
import { seedEnv, seedTestData } from './global-setup';
import { setWorkerPort } from './run-dir';
import { buildBinary, cleanupServer, sweepStaleTestDirs, spawnServer, processDiedMessage, type ServerHandle } from './server';
import { dirname } from 'path';
import { createServer } from 'net';

// Runs once per worker process (module load, before any fixture), which is
// as close to "once per `make e2e` invocation" as this file gets — workers
// are separate OS processes with no shared state to dedupe against, but the
// sweep is idempotent (it only removes dirs whose owning PID is dead / that
// are stale by age) so running it N times for N workers costs nothing beyond
// a few extra readdir calls.
sweepStaleTestDirs();

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
    // Only this worker's own per-worker build (not a shared E2E_BINARY path,
    // built once up front by `make e2e` and reused by every worker) is this
    // teardown's to remove.
    const binaryDir = process.env.E2E_BINARY ? undefined : dirname(binary);
    const handle = await spawnServer(port, { extraEnv: seedEnv(), binary });
    try {
      await seedTestData(handle.baseURL, port);
    } catch (err) {
      // Surface why the server is unreachable — dead (exit code/signal +
      // stderr) or still alive (the signature of a *different* server having
      // answered) — instead of letting the bare fetch error (e.g.
      // `SocketError: other side closed`) stand unattributed.
      const detail = await processDiedMessage(handle, 'during seedTestData');
      // A failed seed never runs `use(handle)`, so nothing else will call
      // cleanupServer for this handle — clean it up here or it leaks an
      // orphan server process + dataDir.
      await cleanupServer(handle, binaryDir);
      throw new Error(`${detail}\ncaused by: ${err}`);
    }

    await use(handle);

    await cleanupServer(handle, binaryDir);
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
