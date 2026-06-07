import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, it } from "node:test";

import {
  DaemonStartError,
  daemonPaths,
  readDaemonStatus,
  startDaemon,
  stopDaemon,
  writeDaemonStatus
} from "../src/daemon.js";
import { listTimelineEntries } from "../src/store.js";
import type { RuntimeOptions } from "../src/cli-shared.js";
import type { DaemonStatus } from "../src/types.js";

async function makeRuntime(): Promise<RuntimeOptions> {
  const root = await mkdtemp(path.join(tmpdir(), "hem-daemon-test-"));
  const projectPath = path.join(root, "project");
  const homeDir = path.join(root, "home");
  const storeDir = path.join(root, "store");
  await mkdir(projectPath, { recursive: true });
  await mkdir(homeDir, { recursive: true });
  return { projectPath, homeDir, storeDir };
}

function runningStatus(options: RuntimeOptions, overrides: Partial<DaemonStatus> = {}): DaemonStatus {
  const now = new Date().toISOString();
  return {
    running: true,
    pid: process.pid,
    identityHash: "sha256:status",
    startedAt: now,
    lastHeartbeatAt: now,
    runId: "run-status",
    projectPath: options.projectPath,
    storeDir: options.storeDir,
    watchedPaths: [],
    stale: false,
    errors: [],
    ...overrides,
    identityVerified: overrides.identityVerified ?? true
  };
}

describe("daemon status identity", () => {
  it("does not report running or stop a pid when the status identity does not match the daemon lock", async () => {
    const options = await makeRuntime();
    await writeDaemonStatus(options.storeDir, runningStatus(options));
    await writeFile(daemonPaths(options.storeDir).lock, JSON.stringify({
      runId: "run-status",
      identityHash: "sha256:different",
      pid: process.pid,
      createdAt: new Date().toISOString()
    }, null, 2));

    const status = await readDaemonStatus(options);
    assert.equal(status.ok, true);
    assert.equal(status.status.running, false);
    assert.equal(status.status.stale, false);

    const stopped = await stopDaemon(options);
    assert.equal(stopped.stopped, false);
    assert.equal(stopped.status.running, false);
    assert.equal(JSON.parse(await readFile(daemonPaths(options.storeDir).lock, "utf8")).identityHash, "sha256:different");
  });

  it("refuses to start a replacement when stale status still has a live pid", async () => {
    const options = await makeRuntime();
    const staleHeartbeat = new Date(Date.now() - 60_000).toISOString();
    await writeDaemonStatus(options.storeDir, runningStatus(options, {
      lastHeartbeatAt: staleHeartbeat
    }));
    await writeFile(daemonPaths(options.storeDir).lock, JSON.stringify({
      runId: "run-status",
      identityHash: "sha256:status",
      pid: process.pid,
      createdAt: new Date().toISOString()
    }, null, 2));

    const result = await startDaemon(options, {
      intervalMs: 50,
      debounceMs: 10
    });

    assert.equal(result.started, false);
    assert.equal(result.reason, "stale-live");
    assert.equal(result.status.pidAlive, true);
    assert.equal(result.status.stale, true);
    assert.match(result.status.errors.join("\n"), /Refusing to start another daemon/);
    assert.equal(JSON.parse(await readFile(daemonPaths(options.storeDir).lock, "utf8")).runId, "run-status");
  });

  it("keeps a foreign lock when status is corrupt", async () => {
    const options = await makeRuntime();
    await mkdir(daemonPaths(options.storeDir).dir, { recursive: true });
    await writeFile(daemonPaths(options.storeDir).status, "{bad json", "utf8");
    await writeFile(daemonPaths(options.storeDir).lock, JSON.stringify({
      runId: "foreign-run",
      identityHash: "sha256:foreign",
      pid: process.pid,
      createdAt: new Date().toISOString()
    }, null, 2));

    const status = await readDaemonStatus(options);
    assert.equal(status.ok, false);

    const stopped = await stopDaemon(options);
    assert.equal(stopped.stopped, false);
    assert.equal(JSON.parse(await readFile(daemonPaths(options.storeDir).lock, "utf8")).runId, "foreign-run");
  });

  it("fails start and cleans its own lock when first-run baseline capture fails", async () => {
    const options = await makeRuntime();
    await mkdir(path.join(options.storeDir, "timeline"), { recursive: true });
    await writeFile(path.join(options.storeDir, "timeline", "events"), "not a directory", "utf8");

    await assert.rejects(
      () => startDaemon(options, { intervalMs: 50, debounceMs: 10 }),
      (error) => error instanceof DaemonStartError && error.code === "HEM_DAEMON_BASELINE_FAILED"
    );

    await assert.rejects(
      () => readFile(daemonPaths(options.storeDir).lock, "utf8"),
      (error) => (error as NodeJS.ErrnoException).code === "ENOENT"
    );
    const status = await readDaemonStatus(options);
    assert.equal(status.ok, true);
    assert.equal(status.status.running, false);
    assert.match(status.status.errors.join("\n"), /ENOTDIR|not a directory/i);
  });

  it("keeps the baseline but cleans its own lock when worker spawn fails", async () => {
    const options = await makeRuntime();
    await writeFile(path.join(options.projectPath, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "stdio", command: "gh-mcp" }
      }
    }));

    await assert.rejects(
      () => startDaemon(options, {
        intervalMs: 50,
        debounceMs: 10,
        nodeExecPath: path.join(options.projectPath, "missing-node")
      }),
      (error) => error instanceof DaemonStartError && error.code === "HEM_DAEMON_SPAWN_FAILED"
    );

    const entries = await listTimelineEntries(options.storeDir, {
      projectPath: options.projectPath
    });
    assert.equal(entries.length, 1);
    assert.equal(entries[0].eventKind, "baseline");
    await assert.rejects(
      () => readFile(daemonPaths(options.storeDir).lock, "utf8"),
      (error) => (error as NodeJS.ErrnoException).code === "ENOENT"
    );
    const status = await readDaemonStatus(options);
    assert.equal(status.ok, true);
    assert.equal(status.status.running, false);
    assert.match(status.status.errors.join("\n"), /missing-node|ENOENT/i);
  });
});
