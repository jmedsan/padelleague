import { mkdirSync } from 'fs';
import { join } from 'path';

// Set once per worker process by worker-server.ts's fixture, before any test
// in that worker runs. Each Playwright worker process gets its own module
// instance of run-dir.ts, so this is not shared across workers.
let workerPort: number | undefined;

// setWorkerPort records this worker's server port so runDataDir resolves to
// that worker's own run directory instead of the single-run default.
export function setWorkerPort(port: number): void {
  workerPort = port;
}

// runDataDir returns this run's private artifact directory,
// e2e/.test-data/<port>/, so concurrent runs (each on its own port) never
// read each other's seed.json or scenario files. Resolution order: a port
// set via setWorkerPort (parallel e2e, one server per worker) > E2E_PORT
// (make e2e, scenario runs — single shared server) > defaultPort, which
// must match the resolution in the Playwright config that drives the run.
export function runDataDir(defaultPort: number): string {
  const port = workerPort ?? (process.env.E2E_PORT ? Number(process.env.E2E_PORT) : defaultPort);
  const dir = join(__dirname, '.test-data', String(port));
  mkdirSync(dir, { recursive: true });
  return dir;
}
