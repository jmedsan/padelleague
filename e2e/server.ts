import { execSync, spawn, ChildProcess } from 'child_process';
import { mkdtempSync, openSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';

export interface ServerHandle {
  process: ChildProcess;
  dataDir: string;
  port: number;
  baseURL: string;
}

export async function spawnServer(
  port: number,
  opts?: { extraEnv?: Record<string, string>; keepAlive?: boolean },
): Promise<ServerHandle> {
  const binary = join(mkdtempSync(join(tmpdir(), 'pl-')), 'padelleague');
  execSync(`go build -o ${binary} .`, {
    cwd: join(__dirname, '..'),
    stdio: 'inherit',
  });

  const dataDir = mkdtempSync(join(tmpdir(), 'padelleague-test-'));
  const baseURL = `http://localhost:${port}`;
  const keepAlive = opts?.keepAlive ?? false;

  let stdio: any;
  if (keepAlive) {
    const logFile = join(dataDir, 'server.log');
    const logFd = openSync(logFile, 'w');
    stdio = ['ignore', logFd, logFd];
  } else {
    stdio = ['ignore', 'pipe', 'inherit'];
  }

  const proc = spawn(binary, ['serve', `--http=0.0.0.0:${port}`, `--dir=${dataDir}`], {
    env: { PATH: process.env.PATH || '', HOME: process.env.HOME || '', ...opts?.extraEnv },
    stdio,
    detached: keepAlive,
  });

  if (!keepAlive) {
    proc.stdout?.resume();
  }

  await waitForServer(`${baseURL}/login`, 60_000);

  return { process: proc, dataDir, port, baseURL };
}

export async function superuserLogin(
  baseURL: string,
  email: string,
  password: string,
): Promise<string> {
  const resp = await fetch(`${baseURL}/api/collections/_superusers/auth-with-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identity: email, password }),
  });
  const data = await resp.json();
  if (!data.token) throw new Error(`Superuser auth failed: ${JSON.stringify(data)}`);
  return data.token;
}

export function killServer(handle: ServerHandle): void {
  if (handle.process && !handle.process.killed) {
    handle.process.kill('SIGTERM');
  }
}

async function waitForServer(url: string, timeoutMs: number): Promise<void> {
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
