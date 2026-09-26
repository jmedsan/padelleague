import { readFileSync, writeFileSync } from 'fs';
import { join } from 'path';
import { runDataDir } from './run-dir';
import { ServerHandle, killServer } from './server';
import { ADMIN_EMAIL, ADMIN_PASSWORD } from './global-setup';
import { PLAYER_PASSWORD } from './scenario-helpers';

export default async function scenarioTeardown() {
  const handle = (globalThis as any).__SCENARIO_SERVER as ServerHandle;
  if (process.env.E2E_KEEP === '1') {
    const dir = runDataDir(8098);
    writeFileSync(join(dir, 'scenario.pid'), String(handle.process.pid));
    writeFileSync(join(dir, 'scenario.dir'), handle.dataDir);
    console.log(`\n=== Scenario server kept alive ===`);
    console.log(`URL:       ${handle.baseURL}`);
    console.log(`Admin:     ${ADMIN_EMAIL} / ${ADMIN_PASSWORD}`);
    const range = readPlayerRange();
    if (range) {
      console.log(`Players:   ${range} / ${PLAYER_PASSWORD}`);
    }
    console.log(`Data dir:  ${handle.dataDir}`);
    console.log(`PID:       ${handle.process.pid}`);
    console.log(`Stop with: make scenario-stop PORT=${handle.port}`);
    console.log(`Note: crons (quorum 5min, leveled top-up 00:30) keep running.`);
    return;
  }
  killServer(handle);
}

// readPlayerRange reads the players array from scenario.json (written by
// scenario-setup.ts before teardown runs) and formats the full login range,
// e.g. "p01-abc123@test.local .. p32-abc123@test.local" — every player
// shares PLAYER_PASSWORD, so this line is enough to log in as any of them.
function readPlayerRange(): string | undefined {
  try {
    const ctx = JSON.parse(readFileSync(join(runDataDir(8098), 'scenario.json'), 'utf8'));
    const players: Array<{ email: string }> = ctx.players ?? [];
    if (players.length === 0) return undefined;
    if (players.length === 1) return players[0].email;
    return `${players[0].email} .. ${players[players.length - 1].email}`;
  } catch {
    return undefined;
  }
}
