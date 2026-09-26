import { mkdirSync } from 'fs';
import { join } from 'path';

// runDataDir returns this run's private artifact directory,
// e2e/.test-data/<port>/, so concurrent runs (each on its own E2E_PORT)
// never read each other's seed.json or scenario files. defaultPort must
// match the resolution in the Playwright config that drives the run.
export function runDataDir(defaultPort: number): string {
  const port = process.env.E2E_PORT ? Number(process.env.E2E_PORT) : defaultPort;
  const dir = join(__dirname, '.test-data', String(port));
  mkdirSync(dir, { recursive: true });
  return dir;
}
