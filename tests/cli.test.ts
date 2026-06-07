import assert from "node:assert/strict";
import { mkdir, readdir, readFile, writeFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { appendTimelineEntry } from "../src/store.js";
import { writeTar } from "../src/tar.js";
import { captureTimelineSnapshot } from "../src/timeline.js";
import type { TarEntry, TimelineEntry } from "../src/types.js";

async function makeTempRoot(): Promise<string> {
  return await import("node:fs/promises").then(({ mkdtemp }) => mkdtemp(join(tmpdir(), "hem-cli-")));
}

function runCli(args: string[], cwd: string, env: NodeJS.ProcessEnv = {}) {
  return spawnSync(process.execPath, [join(process.cwd(), "dist/src/cli.js"), ...args], {
    cwd,
    encoding: "utf8",
    env: { ...process.env, ...env }
  });
}

async function waitForTimelineEntries(cwd: string, env: NodeJS.ProcessEnv, minimum: number): Promise<any[]> {
  const deadline = Date.now() + 4_000;
  let lastError = "";
  while (Date.now() < deadline) {
    const timeline = runCli(["timeline", "list", "--project", cwd, "--json"], cwd, env);
    if (timeline.status === 0) {
      const entries = JSON.parse(timeline.stdout);
      if (entries.length >= minimum) {
        return entries;
      }
      lastError = `saw ${entries.length} entries`;
    } else {
      lastError = timeline.stderr;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`Timed out waiting for ${minimum} timeline entries: ${lastError}`);
}

async function waitForDaemonPidDead(cwd: string, env: NodeJS.ProcessEnv): Promise<void> {
  const deadline = Date.now() + 4_000;
  let lastStatus = "";
  while (Date.now() < deadline) {
    const status = runCli(["daemon", "status", "--project", cwd, "--json"], cwd, env);
    if (status.status === 0) {
      const daemon = JSON.parse(status.stdout);
      if (!daemon.pidAlive) {
        return;
      }
      lastStatus = status.stdout;
    } else {
      lastStatus = status.stderr;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`Timed out waiting for daemon pid to exit: ${lastStatus}`);
}

function timelineEntry(projectPath: string, overrides: Partial<TimelineEntry> & Pick<TimelineEntry, "id" | "observedAt" | "afterSnapshotName">): TimelineEntry {
  const { afterSnapshotName, observedAt, ...rest } = overrides;
  return {
    schemaVersion: "0.1",
    source: "daemon",
    eventKind: "setup_changed",
    title: "update github mcp",
    projectPath,
    agents: ["claude-code"],
    afterSnapshotName,
    daemonRunId: "run-cli",
    createdAt: observedAt,
    observedAt,
    changedSurfaces: [
      {
        kind: "mcp_server",
        changeType: "MCP_CHANGED",
        path: ".mcp.json",
        entityName: "github",
        restorable: true,
        observeOnly: false
      }
    ],
    restoreReadiness: "full",
    confidence: "high",
    confidenceReason: "test",
    evidenceCount: 1,
    graphNodeCount: 1,
    auditFindingCount: 0,
    changes: {
      hasChanges: true,
      semanticChangeCount: 1,
      rawSourceChangeCount: 0,
      highlights: ["MCP_CHANGED: github"]
    },
    ...rest
  };
}

async function writeCliBundle(root: string, snapshotName: string): Promise<string> {
  const bundlePath = join(root, `${snapshotName}.hem`);
  const entries: TarEntry[] = [
    { path: ".hem/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
    { path: ".hem/format-version", content: Buffer.from("1\n", "utf8"), mode: 0o644, mtime: 1000000, type: "file" },
    {
      path: ".hem/manifest.json",
      content: Buffer.from(JSON.stringify({
        formatVersion: 1,
        snapshotName,
        createdAt: "2026-06-07T00:00:00.000Z",
        projectPath: ".",
        includesContent: false,
        contentFileCount: 0,
        contentTotalBytes: 0,
        sourceMachine: { hostname: "source", platform: "darwin", homeDir: "/Users/source" },
        security: { rawSecretsIncluded: false, redactionPolicy: "metadata-only", signed: false }
      }) + "\n", "utf8"),
      mode: 0o644,
      mtime: 1000000,
      type: "file"
    },
    {
      path: "snapshot/evidence.json",
      content: Buffer.from(JSON.stringify([
        {
          id: "mcp-missing",
          agent: "claude-code",
          kind: "mcp_server",
          sourcePath: ".mcp.json",
          scope: "project",
          precedence: 40,
          parser: "json",
          sensitivity: "command_config",
          contentPolicy: "structured_safe_fields_only",
          restorePolicy: "structured_fields_only",
          captureStatus: "captured",
          confidence: "high",
          name: "missing",
          value: { command: "hem-missing-mcp-binary" }
        },
        {
          id: "env-openai",
          agent: "project",
          kind: "env_key",
          sourcePath: ".env",
          scope: "project",
          precedence: 40,
          parser: "dotenv",
          sensitivity: "env_key_inventory",
          contentPolicy: "key_inventory_only",
          restorePolicy: "key_inventory_only",
          captureStatus: "redacted",
          confidence: "high",
          name: "OPENAI_API_KEY",
          value: { key: "OPENAI_API_KEY" }
        }
      ]) + "\n", "utf8"),
      mode: 0o644,
      mtime: 1000000,
      type: "file"
    }
  ];
  await writeTar(entries, bundlePath);
  return bundlePath;
}

describe("hem CLI scaffold", () => {
  it("prints help with current diagnosis, restore, and bundle safety commands", () => {
    const result = runCli(["--help"], process.cwd());

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /Diagnosis commands:/);
    assert.match(result.stdout, /hem daemon start --project \. --json/);
    assert.match(result.stdout, /hem timeline undo <id> --dry-run --json/);
    assert.match(result.stdout, /hem scan --project/);
    assert.match(result.stdout, /snapshot create --name baseline --metadata-only/);
    assert.match(result.stdout, /diff baseline current --project/);
    assert.match(result.stdout, /audit current --project/);
    assert.match(result.stdout, /provenance current --project/);
    assert.match(result.stdout, /report current --project/);
    assert.match(result.stdout, /hem doctor --project/);
    assert.match(result.stdout, /hem bundle verify <file\.hem>/);
    assert.match(result.stdout, /--apply-content --quarantine --experimental/);
    assert.doesNotMatch(result.stdout, /v0\.1|dry-run only/);
  });

  it("prints current snapshot metadata-only guidance without stale version labels", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });

    const result = runCli(["snapshot", "create", "--name", "baseline", "--project", project], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(result.status, 1);
    assert.match(result.stderr, /Snapshots are metadata-only/);
    assert.match(result.stderr, /Add `--metadata-only`/);
    assert.doesNotMatch(result.stderr, /v0\.1/);
  });

  it("runs the read-only workflow from scan to report", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "stdio", command: "gh-mcp" }
      }
    }));

    const env = { HOME: home, HEM_STORE: store };

    const scan = runCli(["scan", "--project", project], project, env);
    assert.equal(scan.status, 0, scan.stderr);
    assert.match(scan.stdout, /Read-only: yes/);
    assert.match(scan.stdout, /Claude Code/);

    const explain = runCli(["scan", "--project", project, "--explain"], project, env);
    assert.equal(explain.status, 0, explain.stderr);
    assert.match(explain.stdout, /Paths considered/);
    assert.match(explain.stdout, /\.mcp\.json/);

    const create = runCli(["snapshot", "create", "--name", "baseline", "--metadata-only", "--project", project], project, env);
    assert.equal(create.status, 0, create.stderr);
    assert.match(create.stdout, /Created metadata-only snapshot: baseline/);

    const list = runCli(["snapshot", "list"], project, env);
    assert.equal(list.status, 0, list.stderr);
    assert.match(list.stdout, /baseline/);

    const show = runCli(["snapshot", "show", "baseline", "--json"], project, env);
    assert.equal(show.status, 0, show.stderr);
    assert.equal(JSON.parse(show.stdout).manifest.name, "baseline");

    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "http", url: "https://mcp.example.com/github" }
      }
    }));

    const diff = runCli(["diff", "baseline", "current", "--project", project, "--json"], project, env);
    assert.equal(diff.status, 0, diff.stderr);
    assert.equal(JSON.parse(diff.stdout).semanticChanges[0].code, "MCP_CHANGED");

    const audit = runCli(["audit", "current", "--project", project, "--json"], project, env);
    assert.equal(audit.status, 0, audit.stderr);
    assert.ok(Array.isArray(JSON.parse(audit.stdout)));

    const provenance = runCli(["provenance", "current", "--project", project, "--json"], project, env);
    assert.equal(provenance.status, 0, provenance.stderr);
    assert.ok(Array.isArray(JSON.parse(provenance.stdout)));

    const reportPath = join(root, "hem-report.md");
    const report = runCli(["report", "current", "--project", project, "--out", reportPath], project, env);
    assert.equal(report.status, 0, report.stderr);
    assert.match(await readFile(reportPath, "utf8"), /# hem report: current/);

    const reportJson = runCli(["report", "current", "--project", project, "--json"], project, env);
    assert.equal(reportJson.status, 0, reportJson.stderr);
    assert.equal(JSON.parse(reportJson.stdout).snapshot.manifest.name, "current");
  });

  it("runs doctor with JSON readiness output", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        missingTool: { command: "hem-missing-mcp-binary" }
      }
    }));

    const result = runCli(["doctor", "--project", project, "--json"], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(result.status, 0, result.stderr);
    const report = JSON.parse(result.stdout);
    assert.equal(report.summary.needs_manual_action >= 1, true);
    assert.equal(report.items.some((item: { code: string }) => item.code === "HEM_MCP_COMMAND_MISSING"), true);
  });

  it("runs daemon start/status/stop and writes a baseline timeline event", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "stdio", command: "gh-mcp" }
      }
    }));

    const env = { HOME: home, HEM_STORE: store };
    const start = runCli([
      "daemon",
      "start",
      "--project",
      project,
      "--interval-ms",
      "250",
      "--debounce-ms",
      "50",
      "--json"
    ], project, env);

    try {
      assert.equal(start.status, 0, start.stderr);
      const startJson = JSON.parse(start.stdout);
      assert.equal(startJson.started, true);
      assert.equal(startJson.status.running, true);

      const status = runCli(["daemon", "status", "--project", project, "--json"], project, env);
      assert.equal(status.status, 0, status.stderr);
      const statusJson = JSON.parse(status.stdout);
      assert.equal(statusJson.running, true);
      assert.equal(statusJson.identityVerified, true);

      const duplicateStart = runCli([
        "daemon",
        "start",
        "--project",
        project,
        "--interval-ms",
        "250",
        "--debounce-ms",
        "50",
        "--json"
      ], project, env);
      assert.equal(duplicateStart.status, 0, duplicateStart.stderr);
      const duplicateStartJson = JSON.parse(duplicateStart.stdout);
      assert.equal(duplicateStartJson.started, false);
      assert.equal(duplicateStartJson.status.runId, startJson.status.runId);

      const timeline = runCli(["timeline", "list", "--project", project, "--json"], project, env);
      assert.equal(timeline.status, 0, timeline.stderr);
      const entries = JSON.parse(timeline.stdout);
      assert.equal(entries.length, 1);
      assert.equal(entries[0].eventKind, "baseline");
    } finally {
      const stop = runCli(["daemon", "stop", "--project", project, "--json"], project, env);
      assert.equal(stop.status, 0, stop.stderr);
      assert.equal(JSON.parse(stop.stdout).status.running, false);
    }
  });

  it("does not create another baseline after restarting an already-baselined project", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "stdio", command: "gh-mcp" }
      }
    }));

    const env = { HOME: home, HEM_STORE: store };
    const start = runCli(["daemon", "start", "--project", project, "--interval-ms", "300", "--debounce-ms", "50", "--json"], project, env);
    assert.equal(start.status, 0, start.stderr);
    const firstEntries = await waitForTimelineEntries(project, env, 1);
    assert.equal(firstEntries.length, 1);

    const stop = runCli(["daemon", "stop", "--project", project, "--json"], project, env);
    assert.equal(stop.status, 0, stop.stderr);
    await waitForDaemonPidDead(project, env);

    const restart = runCli(["daemon", "start", "--project", project, "--interval-ms", "300", "--debounce-ms", "50", "--json"], project, env);
    try {
      assert.equal(restart.status, 0, restart.stderr);
      assert.equal(JSON.parse(restart.stdout).started, true);
      await new Promise((resolve) => setTimeout(resolve, 500));
      const entries = runCli(["timeline", "list", "--project", project, "--json"], project, env);
      assert.equal(entries.status, 0, entries.stderr);
      assert.equal(JSON.parse(entries.stdout).length, 1);
    } finally {
      const restartStop = runCli(["daemon", "stop", "--project", project, "--json"], project, env);
      assert.equal(restartStop.status, 0, restartStop.stderr);
    }
  });

  it("fails daemon start and removes its lock when first-run baseline capture fails", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await mkdir(join(store, "timeline"), { recursive: true });
    await writeFile(join(store, "timeline", "events"), "not a directory", "utf8");

    const start = runCli(["daemon", "start", "--project", project, "--json"], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(start.status, 1);
    assert.equal(JSON.parse(start.stdout).started, false);
    assert.equal(JSON.parse(start.stdout).error, "HEM_DAEMON_BASELINE_FAILED");
    assert.match(start.stderr, /HEM_DAEMON_BASELINE_FAILED/);
    await assert.rejects(
      () => readFile(join(store, "daemon", "lock.json"), "utf8"),
      (error) => (error as NodeJS.ErrnoException).code === "ENOENT"
    );
  });

  it("refuses daemon start when stale status still points at a live pid", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(join(store, "daemon"), { recursive: true });
    await mkdir(home, { recursive: true });
    const status = {
      running: true,
      pid: process.pid,
      pidAlive: true,
      identityHash: "sha256:status",
      identityVerified: true,
      startedAt: new Date(Date.now() - 120_000).toISOString(),
      lastHeartbeatAt: new Date(Date.now() - 60_000).toISOString(),
      runId: "run-live",
      projectPath: project,
      storeDir: store,
      watchedPaths: [],
      stale: false,
      errors: []
    };
    await writeFile(join(store, "daemon", "status.json"), JSON.stringify(status, null, 2));
    await writeFile(join(store, "daemon", "lock.json"), JSON.stringify({
      runId: "run-live",
      identityHash: "sha256:status",
      pid: process.pid,
      createdAt: new Date().toISOString()
    }, null, 2));

    const start = runCli(["daemon", "start", "--project", project, "--json"], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(start.status, 1);
    const result = JSON.parse(start.stdout);
    assert.equal(result.started, false);
    assert.equal(result.status.stale, true);
    assert.match(result.status.errors.join("\n"), /Refusing to start another daemon/);
    assert.equal(JSON.parse(await readFile(join(store, "daemon", "lock.json"), "utf8")).runId, "run-live");
  });

  it("daemon interval fallback captures files that were missing when watchers were installed", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });

    const env = { HOME: home, HEM_STORE: store };
    const start = runCli([
      "daemon",
      "start",
      "--project",
      project,
      "--interval-ms",
      "150",
      "--debounce-ms",
      "25",
      "--json"
    ], project, env);

    try {
      assert.equal(start.status, 0, start.stderr);
      await writeFile(join(project, ".mcp.json"), JSON.stringify({
        mcpServers: {
          github: { transport: "stdio", command: "gh-mcp" }
        }
      }));

      const entries = await waitForTimelineEntries(project, env, 2);
      assert.equal(entries[0].eventKind, "setup_changed");
      assert.equal(entries[1].eventKind, "baseline");
    } finally {
      const stop = runCli(["daemon", "stop", "--project", project, "--json"], project, env);
      assert.equal(stop.status, 0, stop.stderr);
    }
  });

  it("reports corrupt timeline files on stderr while keeping JSON stdout parseable", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await appendTimelineEntry(store, timelineEntry(project, {
      id: "valid",
      observedAt: "2026-06-08T00:00:00.000Z",
      afterSnapshotName: "valid-after"
    }));
    await writeFile(join(store, "timeline", "events", "bad.json"), "{bad json", "utf8");

    const list = runCli(["timeline", "list", "--project", project, "--json"], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(list.status, 0, list.stderr);
    const entries = JSON.parse(list.stdout);
    assert.equal(entries.length, 1);
    assert.equal(entries[0].id, "valid");
    assert.match(list.stderr, /Skipped corrupt timeline event/);
  });

  it("enforces timeline undo dry-run boundaries without mutating MCP files", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "stdio", command: "gh-mcp" }
      }
    }));
    const options = { projectPath: project, homeDir: home, storeDir: store };
    await captureTimelineSnapshot(options, {
      daemonRunId: "run-cli",
      snapshotName: "cli-baseline"
    });

    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "stdio", command: "gh-mcp-v2" }
      }
    }));
    const skillDir = join(home, ".claude", "skills", "react-review");
    await mkdir(skillDir, { recursive: true });
    await writeFile(join(skillDir, "SKILL.md"), "# React Review\n");
    const changed = await captureTimelineSnapshot(options, {
      daemonRunId: "run-cli",
      skipUnchanged: true
    });
    assert.equal(changed.written, true);

    const env = { HOME: home, HEM_STORE: store };
    const id = changed.entry!.id;
    const mcpBefore = await readFile(join(project, ".mcp.json"), "utf8");
    const eventCountBefore = (await readdir(join(store, "timeline", "events"))).length;

    const missingDryRun = runCli(["timeline", "undo", id, "--project", project, "--json"], project, env);
    assert.equal(missingDryRun.status, 1);
    assert.match(missingDryRun.stderr, /HEM_TIMELINE_UNDO_DRY_RUN_REQUIRED/);

    const dryRun = runCli(["timeline", "undo", id, "--project", project, "--dry-run", "--json"], project, env);
    assert.equal(dryRun.status, 0, dryRun.stderr);
    const plan = JSON.parse(dryRun.stdout);
    assert.equal(plan.dryRun, true);
    assert.equal(plan.writesFiles, false);
    assert.equal(plan.writableItems.length, 1);
    assert.ok(plan.observeOnlySurfaces.length >= 1);
    assert.equal(await readFile(join(project, ".mcp.json"), "utf8"), mcpBefore);
    assert.equal((await readdir(join(store, "timeline", "events"))).length, eventCountBefore);
  });

  it("runs doctor with human-readable readiness output", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        missingTool: { command: "hem-missing-mcp-binary" }
      }
    }));

    const result = runCli(["doctor", "--project", project], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /hem doctor/);
    assert.match(result.stdout, /Readiness:/);
    assert.match(result.stdout, /MCP command hem-missing-mcp-binary is missing/);
    assert.match(result.stdout, /fix:/);
  });

  it("prints bundle import dry-run readiness summary", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    const bundlePath = await writeCliBundle(root, "cli-readiness");

    const result = runCli(["bundle", "import", bundlePath, "--dry-run", "--project", project], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /Readiness:/);
    assert.match(result.stdout, /needs manual action:/);
    assert.match(result.stdout, /MCP command hem-missing-mcp-binary is missing/);
    assert.match(result.stdout, /Environment key OPENAI_API_KEY needs a value/);
  });
});
