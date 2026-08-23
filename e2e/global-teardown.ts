import { rmSync } from 'fs';

export default async function globalTeardown() {
  const server = (globalThis as any).__E2E_SERVER;
  if (server && !server.killed) {
    server.kill('SIGTERM');
    await new Promise<void>(resolve => {
      server.on('close', resolve);
      setTimeout(resolve, 5000);
    });
  }

  const dataDir = (globalThis as any).__E2E_DATA_DIR;
  if (dataDir) {
    try {
      rmSync(dataDir, { recursive: true, force: true });
    } catch {
      // best effort cleanup
    }
  }
}
