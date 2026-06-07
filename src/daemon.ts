import { createHash, randomUUID } from "node:crypto";
import { constants, watch, type FSWatcher } from "node:fs";
import { access, chmod, mkdir, readFile, rm, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { execFile, spawn } from "node:child_process";
import { setTimeout as sleep } from "node:timers/promises";
import { promisify } from "node:util";

import type { RuntimeOptions } from "./cli-shared.js";
import { captureTimelineSnapshot } from "./timeline.js";
import { latestTimelineEntry } from "./store.js";
import type { DaemonStatus, DaemonStatusReadResult, TimelineEntry } from "./types.js";

const DAEMON_DIR = "daemon";
const STATUS_FILE = "status.json";
const LOCK_FILE = "lock.json";
const STALE_MS = 15_000;
const DEFAULT_INTERVAL_MS = 5_000;
const DEFAULT_DEBOUNCE_MS = 1_000;
const HEARTBEAT_DURING_CAPTURE_MS = Math.floor(STALE_MS / 3);
const execFileAsync = promisify(execFile);

export interface DaemonStartOptions {
  intervalMs?: number;
  debounceMs?: number;
  identityToken?: string;
  nodeExecPath?: string;
  workerEntryPath?: string;
}

export type DaemonStartReason = "started" | "already-running" | "stale-live";

export interface DaemonStartResult {
  started: boolean;
  reason: DaemonStartReason;
  status: DaemonStatus;
}

export interface DaemonStopResult {
  stopped: boolean;
  status: DaemonStatus;
}

interface DaemonLock {
  runId?: string;
  identityHash?: string;
  pid?: number;
  createdAt?: string;
}

export class DaemonStartError extends Error {
  constructor(
    public readonly code: string,
    public readonly problem: string,
    public readonly cause: string,
    public readonly fix: string,
    public readonly status: DaemonStatus
  ) {
    super(problem);
  }
}

export function daemonPaths(storeDir: string): { dir: string; status: string; lock: string } {
  const dir = path.join(storeDir, DAEMON_DIR);
  return {
    dir,
    status: path.join(dir, STATUS_FILE),
    lock: path.join(dir, LOCK_FILE)
  };
}

export function daemonWatchedPaths(options: RuntimeOptions): string[] {
  return [
    path.join(options.projectPath, ".mcp.json"),
    path.join(options.projectPath, ".claude", "settings.json"),
    path.join(options.homeDir, ".claude", "settings.json"),
    path.join(options.homeDir, ".claude", "skills")
  ];
}

export async function startDaemon(options: RuntimeOptions, startOptions: DaemonStartOptions = {}): Promise<DaemonStartResult> {
  const existing = await readDaemonStatus(options);
  if (existing.ok && existing.status.running) {
    return { started: false, reason: "already-running", status: existing.status };
  }
  if (existing.ok && shouldBlockStartForLivePid(existing.status)) {
    return { started: false, reason: "stale-live", status: blockedStartStatus(existing.status) };
  }

  const runId = randomUUID().replace(/-/g, "").slice(0, 12);
  const identityToken = randomUUID();
  const identityHash = hashIdentity(identityToken);
  await prepareDaemonDir(options.storeDir);
  try {
    await acquireLock(options.storeDir, runId, identityHash);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "EEXIST") {
      throw error;
    }
    const current = await readDaemonStatus(options);
    if (current.ok && current.status.running) {
      return { started: false, reason: "already-running", status: current.status };
    }
    if ((current.ok && shouldBlockStartForLivePid(current.status)) || await lockHasLivePid(options.storeDir)) {
      return { started: false, reason: "stale-live", status: blockedStartStatus(current.status) };
    }
    await rm(daemonPaths(options.storeDir).lock, { force: true });
    await acquireLock(options.storeDir, runId, identityHash);
  }
  const now = new Date().toISOString();
  const status: DaemonStatus = {
    running: true,
    startedAt: now,
    lastHeartbeatAt: now,
    runId,
    identityHash,
    identityVerified: true,
    projectPath: options.projectPath,
    storeDir: options.storeDir,
    watchedPaths: daemonWatchedPaths(options),
    stale: false,
    errors: []
  };
  try {
    const latest = await latestTimelineEntry(options.storeDir, {
      projectPath: options.projectPath,
      agent: options.agent
    });
    if (latest) {
      status.lastEventAt = latest.observedAt;
    } else {
      const baseline = await captureTimelineSnapshot(options, {
        daemonRunId: runId,
        snapshotName: `daemon-baseline-${runId}`
      });
      if (baseline.entry) {
        status.lastEventAt = baseline.entry.observedAt;
      }
    }
  } catch (error) {
    const failed = {
      ...status,
      running: false,
      errors: [readableError(error)]
    };
    await cleanupDaemon(options.storeDir, failed, { removeLock: "owner" });
    throw new DaemonStartError(
      "HEM_DAEMON_BASELINE_FAILED",
      "Daemon baseline capture failed.",
      readableError(error),
      "Fix the reported store or scan error, then run `hem daemon start --json` again.",
      failed
    );
  }

  let child: ReturnType<typeof spawn>;
  try {
    const nodeExecPath = startOptions.nodeExecPath ?? process.execPath;
    const workerEntryPath = startOptions.workerEntryPath ?? process.argv[1] ?? "";
    await access(nodeExecPath, constants.X_OK);
    child = spawn(nodeExecPath, [
      workerEntryPath,
      "daemon",
      "worker",
      "--project",
      options.projectPath,
      "--run-id",
      runId,
      "--identity-token",
      identityToken,
      "--interval-ms",
      String(startOptions.intervalMs ?? DEFAULT_INTERVAL_MS),
      "--debounce-ms",
      String(startOptions.debounceMs ?? DEFAULT_DEBOUNCE_MS),
      ...(options.agent ? ["--agent", options.agent] : [])
    ], {
      detached: true,
      stdio: "ignore",
      env: {
        ...process.env,
        HOME: options.homeDir,
        HEM_STORE: options.storeDir
      }
    });
    if (!child.pid) {
      throw new Error("spawn did not return a child pid");
    }
  } catch (error) {
    const failed = {
      ...status,
      running: false,
      errors: [readableError(error)]
    };
    await cleanupDaemon(options.storeDir, failed, { removeLock: "owner" });
    throw new DaemonStartError(
      "HEM_DAEMON_SPAWN_FAILED",
      "Daemon worker could not be started.",
      readableError(error),
      "Fix the reported process launch error, then run `hem daemon start --json` again.",
      failed
    );
  }

  child.unref();
  status.pid = child.pid;
  status.pidAlive = true;
  await writeDaemonLock(options.storeDir, runId, identityHash, child.pid);
  await writeDaemonStatus(options.storeDir, status);
  return { started: true, reason: "started", status };
}

export async function stopDaemon(options: RuntimeOptions): Promise<DaemonStopResult> {
  const result = await readDaemonStatus(options);
  if (!result.ok) {
    return { stopped: false, status: result.status };
  }

  if (!result.status.pid || !result.status.pidAlive || !result.status.identityVerified) {
    const ownedDeadDaemon = !result.status.pidAlive && await statusMatchesLock(options.storeDir, result.status);
    const stopped = ownedDeadDaemon
      ? cleanStoppedStatus(result.status)
      : { ...result.status, running: false };
    if (ownedDeadDaemon) {
      await cleanupDaemon(options.storeDir, stopped, { removeLock: "owner" });
    }
    return { stopped: false, status: stopped };
  }

  try {
    process.kill(result.status.pid, "SIGTERM");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ESRCH") {
      throw error;
    }
  }

  const stopped = cleanStoppedStatus(result.status, {
    lastHeartbeatAt: new Date().toISOString()
  });
  await cleanupDaemon(options.storeDir, stopped, { removeLock: "owner" });
  return { stopped: true, status: stopped };
}

export async function readDaemonStatus(options: RuntimeOptions): Promise<DaemonStatusReadResult> {
  const fallback = stoppedStatus(options, false, []);
  try {
    const statusPath = daemonPaths(options.storeDir).status;
    if (!(await pathExists(statusPath))) {
      return { ok: true, status: fallback };
    }

    const parsed = JSON.parse(await readFile(statusPath, "utf8")) as DaemonStatus;
    const heartbeatAt = parsed.lastHeartbeatAt ? Date.parse(parsed.lastHeartbeatAt) : 0;
    const stale = !heartbeatAt || Date.now() - heartbeatAt > STALE_MS;
    const pidAlive = typeof parsed.pid === "number" ? processExists(parsed.pid) : false;
    const lockMatches = await statusMatchesLock(options.storeDir, parsed);
    const processIdentity = await inspectProcessIdentity(parsed.pid, parsed.identityHash);
    const identityVerified = Boolean(parsed.pid && pidAlive && lockMatches && processIdentity.verified);
    const identityError = identityErrorFor(parsed, pidAlive, lockMatches, processIdentity.error);
    const staleReason = stale
      ? heartbeatAt
        ? `heartbeat older than ${STALE_MS}ms`
        : "missing heartbeat"
      : undefined;
    const running = Boolean(parsed.running && identityVerified && !stale);
    return {
      ok: true,
      status: {
        ...parsed,
        running,
        pidAlive,
        identityVerified,
        ...(identityError ? { identityError } : {}),
        stale,
        ...(staleReason ? { staleReason } : {}),
        errors: parsed.errors ?? []
      }
    };
  } catch (error) {
    return {
      ok: false,
      error: readableError(error),
      status: stoppedStatus(options, true, [readableError(error)], "status unreadable")
    };
  }
}

export async function runDaemonWorker(options: RuntimeOptions, runId: string, workerOptions: DaemonStartOptions = {}): Promise<void> {
  const intervalMs = workerOptions.intervalMs ?? DEFAULT_INTERVAL_MS;
  const debounceMs = workerOptions.debounceMs ?? DEFAULT_DEBOUNCE_MS;
  const identityHash = workerOptions.identityToken ? hashIdentity(workerOptions.identityToken) : undefined;
  const watchedPaths = daemonWatchedPaths(options);
  let lastEventAt: string | undefined;
  let startedAt: string | undefined;
  let errors: string[] = [];
  let stopped = false;
  let captureScheduled = false;
  let captureTimer: NodeJS.Timeout | undefined;
  const watchers: FSWatcher[] = [];

  async function writeHeartbeat(): Promise<void> {
    const previous = await readDaemonStatus(options);
    if (previous.status.runId === runId) {
      lastEventAt ??= previous.status.lastEventAt;
      startedAt ??= previous.status.startedAt;
      if (previous.status.errors.length > 0 && errors.length === 0) {
        errors = previous.status.errors;
      }
    }
    await writeDaemonStatus(options.storeDir, {
      running: true,
      pid: process.pid,
      pidAlive: true,
      ...(identityHash ? { identityHash } : {}),
      identityVerified: true,
      startedAt: startedAt ?? startTime,
      lastHeartbeatAt: new Date().toISOString(),
      ...(lastEventAt ? { lastEventAt } : {}),
      runId,
      projectPath: options.projectPath,
      storeDir: options.storeDir,
      watchedPaths,
      stale: false,
      errors
    });
  }

  async function capture(reason: string): Promise<void> {
    const heartbeatTimer = setInterval(() => {
      void safeWriteHeartbeat().catch((error) => {
        errors = [readableError(error)];
      });
    }, HEARTBEAT_DURING_CAPTURE_MS);
    try {
      const result = await captureTimelineSnapshot(options, {
        daemonRunId: runId,
        title: reason === "watch" ? undefined : undefined,
        skipUnchanged: true
      });
      if (result.entry) {
        lastEventAt = result.entry.observedAt;
      }
      errors = [];
    } catch (error) {
      errors = [readableError(error)];
    } finally {
      clearInterval(heartbeatTimer);
    }
    await safeWriteHeartbeat();
  }

  function scheduleCapture(): void {
    captureScheduled = true;
    if (captureTimer) {
      clearTimeout(captureTimer);
    }
    captureTimer = setTimeout(() => {
      captureScheduled = false;
      void capture("watch");
    }, debounceMs);
  }

  const startTime = new Date().toISOString();
  startedAt = startTime;
  await prepareDaemonDir(options.storeDir);
  await safeWriteHeartbeat();

  for (const watchedPath of watchedPaths) {
    try {
      const watcher = watch(watchedPath, { persistent: true }, scheduleCapture);
      watcher.on("error", (error) => {
        errors = [readableError(error)];
        void safeWriteHeartbeat();
      });
      watchers.push(watcher);
    } catch (error) {
      // Missing paths are covered by the periodic rescan fallback.
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
        errors = [readableError(error)];
        await safeWriteHeartbeat();
      }
    }
  }

  async function shutdown(): Promise<void> {
    stopped = true;
    for (const watcher of watchers) watcher.close();
    if (captureTimer) clearTimeout(captureTimer);
    await cleanupDaemon(options.storeDir, {
      running: false,
      pid: process.pid,
      pidAlive: false,
      ...(identityHash ? { identityHash } : {}),
      identityVerified: true,
      startedAt: startTime,
      lastHeartbeatAt: new Date().toISOString(),
      ...(lastEventAt ? { lastEventAt } : {}),
      runId,
      projectPath: options.projectPath,
      storeDir: options.storeDir,
      watchedPaths,
      stale: false,
      errors: []
    }, { removeLock: "owner" });
  }

  process.once("SIGTERM", () => {
    void shutdown().finally(() => process.exit(0));
  });
  process.once("SIGINT", () => {
    void shutdown().finally(() => process.exit(0));
  });

  while (!stopped) {
    await sleep(intervalMs);
    if (!captureScheduled) {
      await capture("interval");
    } else {
      await safeWriteHeartbeat();
    }
  }

  async function safeWriteHeartbeat(): Promise<void> {
    try {
      await writeHeartbeat();
    } catch (error) {
      errors = [readableError(error)];
    }
  }
}

function stoppedStatus(options: RuntimeOptions, stale: boolean, errors: string[], staleReason?: string): DaemonStatus {
  return {
    running: false,
    pidAlive: false,
    identityVerified: false,
    projectPath: options.projectPath,
    storeDir: options.storeDir,
    watchedPaths: daemonWatchedPaths(options),
    stale,
    ...(staleReason ? { staleReason } : {}),
    errors
  };
}

function cleanStoppedStatus(status: DaemonStatus, overrides: Partial<DaemonStatus> = {}): DaemonStatus {
  return {
    ...status,
    ...overrides,
    running: false,
    stale: false,
    staleReason: undefined,
    errors: []
  };
}

async function prepareDaemonDir(storeDir: string): Promise<void> {
  const paths = daemonPaths(storeDir);
  await mkdir(paths.dir, { recursive: true, mode: 0o700 });
  await chmod(paths.dir, 0o700);
}

async function acquireLock(storeDir: string, runId: string, identityHash: string): Promise<void> {
  const lock = daemonPaths(storeDir).lock;
  await writeFile(lock, `${JSON.stringify(lockRecord(runId, identityHash, process.pid), null, 2)}\n`, { mode: 0o600, flag: "wx" });
}

async function writeDaemonLock(storeDir: string, runId: string, identityHash: string, pid: number | undefined): Promise<void> {
  await writeJsonAtomic(daemonPaths(storeDir).lock, lockRecord(runId, identityHash, pid));
}

function lockRecord(runId: string, identityHash: string, pid: number | undefined): { runId: string; identityHash: string; pid?: number; createdAt: string } {
  return {
    runId,
    identityHash,
    ...(pid ? { pid } : {}),
    createdAt: new Date().toISOString()
  };
}

async function statusMatchesLock(storeDir: string, status: DaemonStatus): Promise<boolean> {
  if (!status.runId || !status.identityHash || !status.pid) return false;
  try {
    const lock = await readDaemonLock(storeDir);
    return lock.runId === status.runId && lock.identityHash === status.identityHash && lock.pid === status.pid;
  } catch {
    return false;
  }
}

async function cleanupDaemon(storeDir: string, status: DaemonStatus, options: { removeLock: "owner" | "force" | "none" }): Promise<void> {
  if (options.removeLock === "force") {
    await writeDaemonStatus(storeDir, status);
    await rm(daemonPaths(storeDir).lock, { force: true });
  } else if (options.removeLock === "owner") {
    if (await daemonLockOwnedBy(storeDir, status)) {
      await writeDaemonStatus(storeDir, status);
      await rm(daemonPaths(storeDir).lock, { force: true });
    }
  } else {
    await writeDaemonStatus(storeDir, status);
  }
}

export async function writeDaemonStatus(storeDir: string, status: DaemonStatus): Promise<void> {
  await prepareDaemonDir(storeDir);
  await writeJsonAtomic(daemonPaths(storeDir).status, status);
}

function processExists(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

async function readDaemonLock(storeDir: string): Promise<DaemonLock> {
  return JSON.parse(await readFile(daemonPaths(storeDir).lock, "utf8")) as DaemonLock;
}

async function lockHasLivePid(storeDir: string): Promise<boolean> {
  try {
    const lock = await readDaemonLock(storeDir);
    return typeof lock.pid === "number" && processExists(lock.pid);
  } catch {
    return false;
  }
}

async function removeDaemonLockIfOwner(storeDir: string, status: DaemonStatus): Promise<void> {
  if (await daemonLockOwnedBy(storeDir, status)) {
    await rm(daemonPaths(storeDir).lock, { force: true });
  }
}

async function daemonLockOwnedBy(storeDir: string, status: DaemonStatus): Promise<boolean> {
  if (!status.runId || !status.identityHash) return false;
  try {
    const lock = await readDaemonLock(storeDir);
    const pidMatches = status.pid === undefined || lock.pid === status.pid;
    return lock.runId === status.runId && lock.identityHash === status.identityHash && pidMatches;
  } catch {
    // Missing or unreadable locks are already not owned by this status.
    return false;
  }
}

function shouldBlockStartForLivePid(status: DaemonStatus): boolean {
  return Boolean(status.pidAlive && !status.running);
}

function blockedStartStatus(status: DaemonStatus): DaemonStatus {
  return {
    ...status,
    running: false,
    stale: true,
    staleReason: status.staleReason ?? "daemon pid is still alive but status is not safely running",
    errors: [
      ...status.errors,
      "Refusing to start another daemon while the previous daemon pid is still alive."
    ]
  };
}

function identityErrorFor(
  status: DaemonStatus,
  pidAlive: boolean,
  lockMatches: boolean,
  processError?: string
): string | undefined {
  if (!status.pid) return "missing daemon pid";
  if (!pidAlive) return "daemon pid is not alive";
  if (!status.identityHash) return "missing daemon identity";
  if (!lockMatches) return "daemon status does not match daemon lock";
  return processError;
}

async function inspectProcessIdentity(
  pid: number | undefined,
  identityHash: string | undefined
): Promise<{ verified: boolean; error?: string }> {
  if (!pid || !identityHash) return { verified: false, error: "missing process identity" };

  try {
    const args = await readProcessArgs(pid);
    if (args.length > 0) {
      return verifyProcessArgs(args, identityHash);
    }

    try {
      const command = await readProcessCommand(pid);
      return verifyProcessCommand(command, identityHash);
    } catch (error) {
      if (isProcessInspectionUnavailable(error)) {
        return { verified: true };
      }
      throw error;
    }
  } catch (error) {
    return { verified: false, error: readableError(error) };
  }
}

async function readProcessArgs(pid: number): Promise<string[]> {
  if (!(await pathExists("/proc"))) return [];

  try {
    const cmdline = await readFile(`/proc/${pid}/cmdline`, "utf8");
    return cmdline.split("\0").filter(Boolean);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return [];
    throw error;
  }
}

async function readProcessCommand(pid: number): Promise<string> {
  const { stdout } = await execFileAsync("ps", ["-p", String(pid), "-o", "command="], {
    timeout: 1_000,
    maxBuffer: 64 * 1024
  });
  const command = stdout.trim();
  if (!command) {
    throw new Error("process command is not inspectable");
  }
  return command;
}

function verifyProcessArgs(args: string[], identityHash: string): { verified: boolean; error?: string } {
  const isHemWorker = args.includes("daemon") && args.includes("worker");
  const tokenIndex = args.indexOf("--identity-token");
  const token = tokenIndex >= 0 ? args[tokenIndex + 1] : undefined;
  if (!isHemWorker) return { verified: false, error: "live pid is not a hem daemon worker" };
  if (!token) return { verified: false, error: "live hem worker is missing identity token" };
  if (hashIdentity(token) !== identityHash) return { verified: false, error: "live hem worker identity does not match status" };
  return { verified: true };
}

function verifyProcessCommand(command: string, identityHash: string): { verified: boolean; error?: string } {
  if (!/(^|\s)daemon\s+worker(\s|$)/.test(command)) {
    return { verified: false, error: "live pid is not a hem daemon worker" };
  }

  const token = command.match(/(^|\s)--identity-token(?:=|\s+)(\S+)/)?.[2];
  if (!token) return { verified: false, error: "live hem worker is missing identity token" };
  if (hashIdentity(token) !== identityHash) return { verified: false, error: "live hem worker identity does not match status" };
  return { verified: true };
}

function isProcessInspectionUnavailable(error: unknown): boolean {
  const code = (error as NodeJS.ErrnoException).code;
  return code === "ENOENT" || code === "EACCES" || code === "EPERM";
}

async function writeJsonAtomic(filePath: string, value: unknown): Promise<void> {
  const tempPath = `${filePath}.${process.pid}.${randomUUID()}.tmp`;
  try {
    await writeFile(tempPath, `${JSON.stringify(value, null, 2)}\n`, { mode: 0o600 });
    await rename(tempPath, filePath);
  } catch (error) {
    await rm(tempPath, { force: true });
    throw error;
  }
}

async function pathExists(targetPath: string): Promise<boolean> {
  try {
    await access(targetPath, constants.F_OK);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return false;
    }
    throw error;
  }
}

function readableError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function hashIdentity(identityToken: string): string {
  return `sha256:${createHash("sha256").update(identityToken).digest("hex")}`;
}
