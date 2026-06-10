import assert from "node:assert/strict";
import { chmod, mkdtemp, readdir, readFile, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, it } from "node:test";
import {
  agentStoreDir,
  appendTimelineEntry,
  ensureStore,
  findTimelineEntry,
  listAgents,
  listSnapshots,
  listTimelineEntries,
  readSnapshot,
  snapshotExists,
  writeSnapshot
} from "../src/store.js";
import type { Snapshot, TimelineEntry } from "../src/types.js";

async function tempStore(): Promise<string> {
  return mkdtemp(path.join(tmpdir(), "hem-store-test-"));
}

function snapshot(name: string): Snapshot {
  return {
    manifest: {
      schemaVersion: "0.1",
      name,
      createdAt: "2026-05-12T00:00:00.000Z",
      projectPath: "/tmp/project",
      security: {
        rawSecretsIncluded: false,
        redactionPolicy: "metadata-only"
      }
    },
    evidence: [
      {
        id: "claude.project.settings",
        agent: "claude-code",
        kind: "agent_config",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        parser: "json",
        sensitivity: "command_config",
        contentPolicy: "structured_safe_fields_only",
        restorePolicy: "not_supported",
        captureStatus: "captured",
        confidence: "high",
        checksum: "sha256:observed-config"
      }
    ],
    graph: [
      {
        id: "node.claude.project.settings",
        agent: "claude-code",
        scope: "project",
        sourcePath: ".claude/settings.json",
        entityKind: "agent_config",
        entityName: "settings",
        effectiveValue: { permissions: ["Bash(npm test)"] },
        confidence: "high",
        evidenceId: "claude.project.settings"
      }
    ],
    auditFindings: [
      {
        code: "EXECUTABLE_CONFIG_ADDED",
        severity: "medium",
        problem: "Project config allows an executable command.",
        cause: ".claude/settings.json allows Bash(npm test).",
        fix: "Review the allowed command before sharing the project config.",
        path: ".claude/settings.json",
        evidenceId: "claude.project.settings"
      }
    ],
    provenance: [
      {
        nodeId: "node.claude.project.settings",
        evidenceId: "claude.project.settings",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        confidence: "high",
        captureStatus: "captured"
      }
    ]
  };
}

function timelineEntry(overrides: Partial<TimelineEntry> & Pick<TimelineEntry, "id" | "observedAt" | "afterSnapshotName">): TimelineEntry {
  const { afterSnapshotName, observedAt, ...rest } = overrides;
  return {
    schemaVersion: "0.1",
    source: "manual",
    eventKind: "setup_changed",
    title: "update github mcp",
    projectPath: "/tmp/project",
    agents: ["claude-code"],
    afterSnapshotName,
    captureId: "capture-test",
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

describe("snapshot store", () => {
  it("creates the store directory with 0700 permissions", async () => {
    const storeDir = path.join(await tempStore(), "store");

    const findings = await ensureStore(storeDir);
    const mode = (await stat(storeDir)).mode & 0o777;

    assert.equal(mode, 0o700);
    assert.deepEqual(findings, []);
  });

  it("returns an audit finding when an existing store is group or world writable", async () => {
    const storeDir = path.join(await tempStore(), "store");
    await ensureStore(storeDir);
    await chmod(storeDir, 0o777);

    const findings = await ensureStore(storeDir);

    assert.equal(findings.length, 1);
    assert.equal(findings[0].code, "WORLD_WRITABLE_STORE");
    assert.equal(findings[0].severity, "high");
    assert.equal(findings[0].path, storeDir);
  });

  it("rejects unsafe snapshot names", async () => {
    const storeDir = await tempStore();
    const badNames = ["", "../baseline", "base/line", "base\\line", "safe/../unsafe", ".."];

    for (const name of badNames) {
      await assert.rejects(() => writeSnapshot(storeDir, snapshot(name)), /unsafe snapshot name/i);
      await assert.rejects(() => readSnapshot(storeDir, name), /unsafe snapshot name/i);
    }
  });

  it("lists snapshots and reads a snapshot round trip", async () => {
    const storeDir = await tempStore();
    const baseline = snapshot("baseline");
    const current = snapshot("current");
    await writeSnapshot(storeDir, current);
    await writeSnapshot(storeDir, baseline);

    assert.deepEqual(await listSnapshots(storeDir), ["baseline", "current"]);
    assert.equal(await snapshotExists(storeDir, "baseline"), true);
    assert.equal(await snapshotExists(storeDir, "missing"), false);
    assert.deepEqual(await readSnapshot(storeDir, "baseline"), baseline);
  });

  it("writes the metadata-only snapshot file set and preserves observed checksums", async () => {
    const storeDir = await tempStore();
    await writeSnapshot(storeDir, snapshot("baseline"));

    const snapshotDir = path.join(storeDir, "baseline");
    const files = await readdir(snapshotDir);
    assert.deepEqual(files.sort(), [
      "audit-findings.json",
      "checksums.json",
      "evidence.json",
      "graph.json",
      "manifest.json",
      "provenance.json",
      "redactions.json"
    ]);

    const checksums = JSON.parse(await readFile(path.join(snapshotDir, "checksums.json"), "utf8"));
    const redactions = JSON.parse(await readFile(path.join(snapshotDir, "redactions.json"), "utf8"));

    assert.deepEqual(checksums, {
      "claude.project.settings": {
        sourcePath: ".claude/settings.json",
        checksum: "sha256:observed-config"
      }
    });
    assert.deepEqual(redactions, []);
  });
});

describe("per-agent snapshot store", () => {
  it("agentStoreDir returns store/agent for scoped paths, store root for unscoped", () => {
    assert.equal(agentStoreDir("/store", "claude-code"), "/store/claude-code");
    assert.equal(agentStoreDir("/store", "codex"), "/store/codex");
    assert.equal(agentStoreDir("/store"), "/store");
  });

  it("writes and reads snapshots per agent", async () => {
    const storeDir = await tempStore();
    const ccSnap = snapshot("baseline");
    const codexSnap = snapshot("codex-baseline");

    await writeSnapshot(storeDir, ccSnap, "claude-code");
    await writeSnapshot(storeDir, codexSnap, "codex");

    // Check isolation: each agent's snapshots live under their own subdir
    const ccDir = path.join(storeDir, "claude-code", "baseline");
    const codexDir = path.join(storeDir, "codex", "codex-baseline");
    assert.equal((await stat(ccDir)).isDirectory(), true);
    assert.equal((await stat(codexDir)).isDirectory(), true);

    // Flat store listing should NOT see agent-scoped snapshots
    assert.deepEqual(await listSnapshots(storeDir), []);

    // Agent-scoped listing should work
    const ccSnaps = await listSnapshots(storeDir, "claude-code");
    assert.deepEqual(ccSnaps, ["baseline"]);

    const codexSnaps = await listSnapshots(storeDir, "codex");
    assert.deepEqual(codexSnaps, ["codex-baseline"]);

    // Read back
    assert.deepEqual(await readSnapshot(storeDir, "baseline", "claude-code"), ccSnap);
    assert.deepEqual(await readSnapshot(storeDir, "codex-baseline", "codex"), codexSnap);

    // Snapshot exists check
    assert.equal(await snapshotExists(storeDir, "baseline", "claude-code"), true);
    assert.equal(await snapshotExists(storeDir, "baseline", "codex"), false);
  });

  it("listAgents returns agents with snapshots", async () => {
    const storeDir = await tempStore();
    await writeSnapshot(storeDir, snapshot("v1"), "claude-code");
    await writeSnapshot(storeDir, snapshot("v1"), "codex");
    await writeSnapshot(storeDir, snapshot("v1"), "cursor");

    const agents = await listAgents(storeDir);
    assert.deepEqual(agents, ["claude-code", "codex", "cursor"]);
  });

  it("listAgents returns empty for empty store", async () => {
    const storeDir = await tempStore();
    await ensureStore(storeDir);
    assert.deepEqual(await listAgents(storeDir), []);
  });
});

describe("timeline event store", () => {
  it("persists immutable event files and sorts by observed time", async () => {
    const storeDir = await tempStore();
    const older = timelineEntry({
      id: "older",
      observedAt: "2026-06-07T00:00:00.000Z",
      afterSnapshotName: "after-older"
    });
    const newer = timelineEntry({
      id: "newer",
      observedAt: "2026-06-07T00:01:00.000Z",
      afterSnapshotName: "after-newer"
    });

    await appendTimelineEntry(storeDir, older);
    await appendTimelineEntry(storeDir, newer);

    assert.deepEqual((await listTimelineEntries(storeDir)).map((entry) => entry.id), ["newer", "older"]);
    assert.equal((await findTimelineEntry(storeDir, "after-older"))?.id, "older");
    assert.deepEqual(await listAgents(storeDir), []);
  });

  it("skips corrupt event files without hiding valid timeline history", async () => {
    const storeDir = await tempStore();
    await appendTimelineEntry(storeDir, timelineEntry({
      id: "valid",
      observedAt: "2026-06-07T00:00:00.000Z",
      afterSnapshotName: "after-valid"
    }));
    await writeFile(path.join(storeDir, "timeline", "events", "bad.json"), "{bad json", "utf8");

    const corruptEvents: { filePath: string; error: string }[] = [];
    const entries = await listTimelineEntries(storeDir, {
      onCorruptEntry: (event) => corruptEvents.push(event)
    });
    assert.deepEqual(entries.map((entry) => entry.id), ["valid"]);
    assert.equal(corruptEvents.length, 1);
    assert.equal(path.basename(corruptEvents[0].filePath), "bad.json");
    assert.match(corruptEvents[0].error, /json/i);
  });

  it("normalizes legacy daemon timeline events at the store boundary", async () => {
    const storeDir = await tempStore();
    const eventsDir = path.join(storeDir, "timeline", "events");
    await ensureStore(storeDir);
    await import("node:fs/promises").then(({ mkdir }) => mkdir(eventsDir, { recursive: true }));
    await writeFile(path.join(eventsDir, "legacy.json"), JSON.stringify({
      schemaVersion: "0.1",
      id: "legacy-event",
      source: "daemon",
      eventKind: "baseline",
      title: "legacy daemon baseline",
      projectPath: "/tmp/project",
      agents: ["claude-code"],
      afterSnapshotName: "legacy-baseline",
      daemonRunId: "run-legacy",
      createdAt: "2026-06-07T00:00:00.000Z",
      observedAt: "2026-06-07T00:00:00.000Z",
      changedSurfaces: [],
      restoreReadiness: "observe-only",
      confidence: "high",
      confidenceReason: "legacy",
      evidenceCount: 0,
      graphNodeCount: 0,
      auditFindingCount: 0,
      changes: {
        hasChanges: false,
        semanticChangeCount: 0,
        rawSourceChangeCount: 0,
        highlights: []
      }
    }, null, 2));

    const entry = (await listTimelineEntries(storeDir))[0];
    assert.equal(entry.source, "manual");
    assert.equal(entry.captureId, "run-legacy");
    assert.equal(entry.afterSnapshotName, "legacy-baseline");
  });
});
