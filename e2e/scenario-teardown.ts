import { writeFileSync } from 'fs';
import { join } from 'path';
import { ServerHandle, killServer } from './server';
import { ADMIN_EMAIL, ADMIN_PASSWORD } from './global-setup';

export default async function scenarioTeardown() {
  const handle = (globalThis as any).__SCENARIO_SERVER as ServerHandle;
  if (process.env.E2E_KEEP === '1') {
    writeFileSync(join(__dirname, '.test-data/scenario.pid'), String(handle.process.pid));
    writeFileSync(join(__dirname, '.test-data/scenario.dir'), handle.dataDir);
    console.log(`\n=== Scenario server kept alive ===`);
    console.log(`URL:       ${handle.baseURL}`);
    console.log(`Admin:     ${ADMIN_EMAIL} / ${ADMIN_PASSWORD}`);
    console.log(`Data dir:  ${handle.dataDir}`);
    console.log(`PID:       ${handle.process.pid}`);
    console.log(`Stop with: make e2e-scenario-stop`);
    console.log(`Note: crons (quorum 5min, leveled top-up 00:30) keep running.`);
    return;
  }
  killServer(handle);
}
