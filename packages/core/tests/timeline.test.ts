import assert from "node:assert/strict";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, it } from "node:test";

import { listTimelineEntries, readSnapshot } from "../src/store.js";
import { captureTimelineSnapshot } from "../src/timeline.js";
import { buildTimelineUndoPlan } from "../src/timeline-undo.js";
import type { RuntimeOptions } from "../src/runtime-options.js";

async function makeRuntime(): Promise<RuntimeOptions> {
  const root = await mkdtemp(path.join(tmpdir(), "gandalf-timeline-test-"));
  const projectPath = path.join(root, "project");
  const homeDir = path.join(root, "home");
  const storeDir = path.join(root, "store");
  await mkdir(projectPath, { recursive: true });
  await mkdir(homeDir, { recursive: true });
  return { projectPath, homeDir, storeDir };
}

async function writeMcp(projectPath: string, command: string): Promise<void> {
  await writeFile(path.join(projectPath, ".mcp.json"), JSON.stringify({
    mcpServers: {
      github: { transport: "stdio", command }
    }
  }, null, 2));
}

describe("timeline capture", () => {
  it("creates a manual baseline, captures partial MCP+skill changes, and builds MCP-only dry-run undo", async () => {
    const options = await makeRuntime();
    await writeMcp(options.projectPath, "gh-mcp");

    const baseline = await captureTimelineSnapshot(options, {
      captureId: "capture-test",
      snapshotName: "manual-baseline-capture-test"
    });

    assert.equal(baseline.written, true);
    assert.equal(baseline.entry?.eventKind, "baseline");
    assert.equal(baseline.entry?.restoreReadiness, "observe-only");
    assert.equal((await listTimelineEntries(options.storeDir)).length, 1);
    assert.equal((await readSnapshot(options.storeDir, "manual-baseline-capture-test")).manifest.name, "manual-baseline-capture-test");

    await writeMcp(options.projectPath, "gh-mcp-v2");
    const skillDir = path.join(options.homeDir, ".claude", "skills", "react-review");
    await mkdir(skillDir, { recursive: true });
    await writeFile(path.join(skillDir, "SKILL.md"), "# React Review\n");

    const changed = await captureTimelineSnapshot(options, {
      captureId: "capture-test",
      skipUnchanged: true
    });

    assert.equal(changed.written, true);
    assert.equal(changed.entry?.eventKind, "setup_changed");
    assert.equal(changed.entry?.restoreReadiness, "partial");
    assert.equal(changed.entry?.changedSurfaces.some((surface) => surface.kind === "mcp_server" && surface.restorable), true);
    assert.equal(changed.entry?.changedSurfaces.some((surface) => surface.kind !== "mcp_server" && surface.observeOnly), true);

    const undo = await buildTimelineUndoPlan(options.storeDir, changed.entry!.id);
    assert.equal(undo.dryRun, true);
    assert.equal(undo.writesFiles, false);
    assert.equal(undo.writableItems.length, 1);
    assert.equal(undo.writableItems[0].action, "update");
    assert.equal(undo.writableItems[0].serverName, "github");
    assert.ok(undo.observeOnlySurfaces.length >= 1);

    const unchanged = await captureTimelineSnapshot(options, {
      captureId: "capture-test",
      skipUnchanged: true
    });
    assert.equal(unchanged.written, false);
    assert.equal(unchanged.skippedReason, "unchanged");
  });
});
