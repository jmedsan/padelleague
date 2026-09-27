import { execSync, spawn, ChildProcess } from 'child_process';
import { mkdtempSync, openSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';
import { runDataDir } from './run-dir';

// pidFileName is written into every spawned server's data dir so a later
// process (this run's own startup sweep, or a one-off cleanup) can tell
// whether the server that owned a given /tmp/padelleague-test-* dir or
// e2e/.test-data/<port>/ dir is still alive, without guessing from directory
// age. Same file name in both locations so sweepStaleTestDirs can reuse one
// liveness check for either.
const PID_FILE = '.server.pid';

export interface ServerHandle {
  process: ChildProcess;
  dataDir: string;
  port: number;
  baseURL: string;
  stderr: { tail: () => string };
}

// stderrRingBuffer captures a server's stderr so a crash mid-test can be
// attributed to it — plain `inherit` interleaves unlabeled output from every
// concurrent worker's own server into one stream, which is how a real crash
// (panic, OOM) surfaced only as an unexplained `SocketError` on the client
// side with no way to tell which process died or why.
function stderrRingBuffer(maxLines: number): { write: (chunk: Buffer) => void; tail: () => string } {
  const lines: string[] = [];
  let carry = '';
  return {
    write(chunk: Buffer) {
      const parts = (carry + chunk.toString('utf8')).split('\n');
      carry = parts.pop() ?? '';
      lines.push(...parts);
      if (lines.length > maxLines) lines.splice(0, lines.length - maxLines);
    },
    tail: () => (carry ? [...lines, carry] : lines).join('\n'),
  };
}

export async function spawnServer(
  port: number,
  opts?: { extraEnv?: Record<string, string>; keepAlive?: boolean; binary?: string },
): Promise<ServerHandle> {
  // A pre-built binary (buildBinary, called once from globalSetup) skips a
  // ~6s go build per call — the difference between one build and N when N
  // Playwright workers each spawn their own server.
  const binary = opts?.binary ?? await buildBinary();

  const dataDir = mkdtempSync(join(tmpdir(), 'padelleague-test-'));
  const baseURL = `http://localhost:${port}`;
  const keepAlive = opts?.keepAlive ?? false;

  let stdio: any;
  if (keepAlive) {
    const logFile = join(dataDir, 'server.log');
    const logFd = openSync(logFile, 'w');
    stdio = ['ignore', logFd, logFd];
  } else {
    stdio = ['ignore', 'pipe', 'pipe'];
  }

  const proc = spawn(binary, ['serve', `--http=0.0.0.0:${port}`, `--dir=${dataDir}`], {
    env: { PATH: process.env.PATH || '', HOME: process.env.HOME || '', ...opts?.extraEnv },
    stdio,
    detached: keepAlive,
  });

  const stderr = stderrRingBuffer(200);
  if (!keepAlive) {
    proc.stdout?.resume();
    proc.stderr?.on('data', chunk => stderr.write(chunk));
  }

  if (proc.pid !== undefined) {
    try {
      writeFileSync(join(dataDir, PID_FILE), String(proc.pid));
    } catch {
      // best effort: sweepStaleTestDirs falls back to treating this dir as
      // stale (no readable PID) rather than never cleaning it
    }
    // Every caller (globalSetup's single server, scenarioSetup, the
    // per-worker fixture) reaches spawnServer with the port it's binding, so
    // writing this here — rather than in each call site — is the one place
    // that can never forget it. Without it, e2e/.test-data/<port>/ has no
    // liveness marker of its own and a stale sweep there would have nothing
    // but directory age to go on, which is wrong for a long-lived kept-alive
    // server (make scenario-serve) or a run that legitimately takes over an
    // hour.
    try {
      writeFileSync(join(runDataDir(port), PID_FILE), String(proc.pid));
    } catch {
      // best effort: same fallback as above
    }
  }

  const handle: ServerHandle = { process: proc, dataDir, port, baseURL, stderr };
  await waitForServer(`${baseURL}/login`, 60_000, handle);

  return handle;
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

// buildBinary compiles the server once into a fresh temp dir and returns its
// path. Call once (e.g. from globalSetup) and pass the result as
// spawnServer's `binary` option when spawning more than one server, so a
// multi-worker run pays the build cost once instead of once per worker.
export async function buildBinary(): Promise<string> {
  const dir = mkdtempSync(join(tmpdir(), 'pl-'));
  const binary = join(dir, 'padelleague');
  try {
    execSync(`go build -o ${binary} .`, {
      cwd: join(__dirname, '..'),
      stdio: 'inherit',
    });
  } catch (err) {
    // A build failure leaves this mkdtemp'd dir with nothing in it to clean
    // up — every retry against a broken tree leaked one more /tmp/pl-*
    // (119 seen from one bad stretch). Remove it before rethrowing so a
    // failed build costs one dir, not one forever.
    rmSync(dir, { recursive: true, force: true });
    throw err;
  }
  return binary;
}

// killServerAndWait sends SIGTERM and waits for the process to actually exit
// before returning, escalating to SIGKILL if it doesn't within timeoutMs.
// Callers that then delete dataDir need this: PocketBase's SQLite WAL files
// are still open (and on Linux, unlinking a file a process holds open just
// detaches the name — the disk usage isn't reclaimed until the fd closes),
// so a delete raced against a SIGTERM that hasn't been processed yet can
// leave orphaned inodes even though the directory looks removed.
async function waitForExit(handle: ServerHandle, timeoutMs: number): Promise<boolean> {
  if (handle.process.exitCode !== null || handle.process.signalCode !== null) return true;
  return new Promise(resolve => {
    const timer = setTimeout(() => resolve(false), timeoutMs);
    handle.process.once('exit', () => {
      clearTimeout(timer);
      resolve(true);
    });
  });
}

export async function killServerAndWait(handle: ServerHandle, timeoutMs = 5000): Promise<void> {
  if (!handle.process || handle.process.killed) return;
  handle.process.kill('SIGTERM');
  const exited = await waitForExit(handle, timeoutMs);
  if (!exited) {
    handle.process.kill('SIGKILL');
    await waitForExit(handle, timeoutMs);
  }
}

// cleanupServer kills the server (waiting for exit) and removes both the
// binary copy and the data dir it was spawned with. binaryDir is the
// mkdtempSync parent of the binary path (join(binaryDir, 'padelleague')) —
// pass it only when this handle owns a per-server binary build; a shared
// binary (E2E_BINARY, or worker-server.ts's sharedBinary) is reused across
// specs/workers and must outlive any single server's teardown.
export async function cleanupServer(handle: ServerHandle, binaryDir?: string): Promise<void> {
  await killServerAndWait(handle);
  try {
    rmSync(handle.dataDir, { recursive: true, force: true });
  } catch {
    // best effort cleanup
  }
  if (binaryDir) {
    try {
      rmSync(binaryDir, { recursive: true, force: true });
    } catch {
      // best effort cleanup
    }
  }
}

function pidIsAlive(pid: number): boolean {
  try {
    // Signal 0 sends nothing; it only checks whether the PID exists and is
    // ours to signal, per kill(2).
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

// isStaleTmpDataDir applies /tmp/padelleague-test-*'s liveness rule: trust a
// readable, valid PID file over age (no cap, so a long-running kept-alive
// server's dir is never swept out from under it); fall back to a one-hour
// age cap when there's no valid PID file to check. That fallback stays
// needed here because dirs from before PID-file tracking shipped can still
// be on disk.
function isStaleTmpDataDir(full: string, mtimeMs: number): boolean {
  const pid = readPid(full);
  if (pid !== undefined) return !pidIsAlive(pid);
  return Date.now() - mtimeMs >= 60 * 60 * 1000;
}

// isStaleTestDataDir applies e2e/.test-data/<port>/'s liveness rule: every
// writer (spawnServer, for every caller — globalSetup, scenarioSetup, the
// per-worker fixture) now unconditionally records its PID, so age is never
// consulted — a dir is stale only when its recorded PID names a dead
// process. An age fallback here would delete a still-running
// `make scenario-serve` server's scenario.json/pid after an hour (breaking
// `make scenario-stop`), or a concurrent run's seed.json mid-run if it
// happens to take over an hour. A dir with no PID file at all (legacy, from
// before this fix) is left alone — not swept — since there's nothing to
// prove it's dead.
function isStaleTestDataDir(full: string): boolean {
  const pid = readPid(full);
  return pid !== undefined && !pidIsAlive(pid);
}

function readPid(dir: string): number | undefined {
  try {
    const raw = readFileSync(join(dir, PID_FILE), 'utf8').trim();
    const parsed = Number(raw);
    // pid 0 targets the current process's own group in kill(2), not a real
    // server — reject it along with NaN/negative so a truncated or empty PID
    // file (e.g. a write that raced a crash) can't read as permanently "alive".
    return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined;
  } catch {
    return undefined;
  }
}

// sweepStaleTestDirs removes /tmp/pl-* (binary copies), /tmp/padelleague-test-*
// (data dirs), and e2e/.test-data/<port>/ (per-worker artifact dirs) left
// behind by a previous run that never reached its own teardown — a run
// killed with SIGKILL, a closed terminal, or a crash between spawnServer and
// cleanupServer all skip the normal cleanup path. Each such dir is orphaned
// forever otherwise, since nothing else ever revisits it (the /tmp leak grew
// to ~1100 dirs / ~30GB on this machine; e2e/.test-data/ separately grew to
// ~745 dirs since worker-server.ts's per-worker ports never repeat and
// nothing removed the old ones after normal teardown either). Safe to call
// at the start of any run — concurrent runs on this machine are never
// touched, and neither is a `make scenario-serve` server a user has
// deliberately left running for a long manual session (E2E_KEEP=1, no time
// limit): see isStaleTmpDataDir/isStaleTestDataDir above for each tree's
// exact liveness rule.
//   - a binary dir has no owning long-lived process to check at all — the
//     `go build` that created it has already exited by the time spawnServer
//     returns, and the binary itself may then be `spawn()`ed repeatedly
//     across a whole run — so it relies on the one-hour age window
//     unconditionally.
export function sweepStaleTestDirs(): void {
  const dir = tmpdir();
  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    entries = [];
  }
  for (const name of entries) {
    const isBinaryDir = name.startsWith('pl-');
    const isDataDir = name.startsWith('padelleague-test-');
    if (!isBinaryDir && !isDataDir) continue;
    const full = join(dir, name);
    try {
      const st = statSync(full);
      if (!st.isDirectory()) continue;
      const stale = isDataDir ? isStaleTmpDataDir(full, st.mtimeMs) : Date.now() - st.mtimeMs >= 60 * 60 * 1000;
      if (!stale) continue;
      rmSync(full, { recursive: true, force: true });
    } catch {
      // best effort: another process may be racing us to remove/use it
    }
  }

  const testDataRoot = join(__dirname, '.test-data');
  let ports: string[];
  try {
    ports = readdirSync(testDataRoot);
  } catch {
    return;
  }
  for (const name of ports) {
    const full = join(testDataRoot, name);
    try {
      const st = statSync(full);
      if (!st.isDirectory()) continue;
      if (!isStaleTestDataDir(full)) continue;
      rmSync(full, { recursive: true, force: true });
    } catch {
      // best effort: another process may be racing us to remove/use it
    }
  }
}

async function waitForServer(url: string, timeoutMs: number, handle: ServerHandle): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (await hasExited(handle, 0)) {
      throw new Error(await processDiedMessage(handle, 'before it became ready'));
    }
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

// hasExited waits up to graceMs for the process's 'exit' event before
// answering, because a socket the server held can close (surfacing as
// `fetch failed`/`SocketError` at the caller) slightly before Node delivers
// 'exit' — checking exitCode/signalCode immediately after a failed fetch can
// read as "still alive" for a process that is, in fact, already gone.
async function hasExited(handle: ServerHandle, graceMs: number): Promise<boolean> {
  const { process: proc } = handle;
  if (proc.exitCode !== null || proc.signalCode !== null) return true;
  if (graceMs === 0) return false;
  return new Promise(resolve => {
    const timer = setTimeout(() => resolve(false), graceMs);
    proc.once('exit', () => {
      clearTimeout(timer);
      resolve(true);
    });
  });
}

// processDiedMessage attributes a server outcome to its actual exit
// code/signal and stderr tail (dead), or reports it as still alive (the
// signature of a *different* server having answered/failed a request) — a
// crash surfaces as "server exited with X, stderr: Y" and a live-but-wedged
// server as "server still alive", instead of an unattributed `fetch
// failed`/`SocketError` at the call site that happened to be mid-request.
export async function processDiedMessage(handle: ServerHandle, when: string): Promise<string> {
  if (!(await hasExited(handle, 2000))) {
    return `server (pid ${handle.process.pid}, port ${handle.port}) is still alive ${when}`;
  }
  const { exitCode, signalCode } = handle.process;
  const reason = signalCode ? `signal ${signalCode}` : `exit code ${exitCode}`;
  const tail = handle.stderr.tail();
  return `server (pid ${handle.process.pid}, port ${handle.port}) died ${when}: ${reason}` +
    (tail ? `\nstderr tail:\n${tail}` : '\n(no stderr captured)');
}
