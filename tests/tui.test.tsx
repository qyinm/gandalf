import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { daemonTrustHeaderModel } from "../src/tui/components/Dashboard.js";
import { buildAgentFilterEntries } from "../src/tui/components/Sidebar.js";
import { TABS } from "../src/tui/components/TabBar.js";
import {
  buildTimelineViewModel,
  timelineDetailModel,
  timelineUndoPreviewModel
} from "../src/tui/components/TimelineViewModel.js";
import type { TimelineUndoPlan } from "../src/timeline-undo.js";
import type { DaemonStatusReadResult, TimelineEntry } from "../src/types.js";

function statusResult(overrides: Partial<DaemonStatusReadResult["status"]> = {}): DaemonStatusReadResult {
  return {
    ok: true,
    status: {
      running: true,
      pid: 123,
      identityHash: "sha256:test",
      startedAt: "2026-06-08T00:00:00.000Z",
      lastHeartbeatAt: "2026-06-08T00:00:01.000Z",
      lastEventAt: "2026-06-08T00:00:02.000Z",
      runId: "run-test",
      projectPath: "/project",
      storeDir: "/store",
      watchedPaths: ["/project/.mcp.json", "/home/.claude/settings.json"],
      stale: false,
      errors: [],
      ...overrides,
      identityVerified: overrides.identityVerified ?? true
    }
  };
}

function statusReadError(): DaemonStatusReadResult {
  return {
    ok: false,
    error: "status unreadable",
    status: statusResult({
      running: false,
      pidAlive: false,
      identityVerified: false,
      stale: true,
      errors: ["status unreadable"]
    }).status
  };
}

function timelineEntry(overrides: Partial<TimelineEntry> & Pick<TimelineEntry, "id" | "observedAt" | "afterSnapshotName">): TimelineEntry {
  const { id, afterSnapshotName, observedAt, ...rest } = overrides;
  return {
    schemaVersion: "0.1",
    id,
    source: "daemon",
    eventKind: "setup_changed",
    title: "MCP server changed",
    projectPath: "/project",
    agent: "claude-code",
    agents: ["claude-code"],
    beforeSnapshotName: "before",
    afterSnapshotName,
    daemonRunId: "run-test",
    createdAt: observedAt,
    observedAt,
    changedSurfaces: [
      {
        kind: "mcp_server",
        changeType: "MCP_CHANGED",
        path: "/project/.mcp.json",
        entityName: "github",
        restorable: true,
        observeOnly: false,
        before: { command: "gh-old" },
        after: { command: "gh-new" }
      },
      {
        kind: "skill",
        changeType: "SKILL_ADDED",
        path: "/home/.claude/skills/review/SKILL.md",
        entityName: "review",
        restorable: false,
        observeOnly: true
      }
    ],
    restoreReadiness: "partial",
    confidence: "high",
    confidenceReason: "semantic diff matched setup files",
    evidenceCount: 2,
    graphNodeCount: 2,
    auditFindingCount: 0,
    changes: {
      previousEntryId: "prev",
      previousSnapshotName: "before",
      hasChanges: true,
      semanticChangeCount: 2,
      rawSourceChangeCount: 0,
      highlights: ["MCP_CHANGED: github", "SKILL_ADDED: review"]
    },
    ...rest
  };
}

describe("TUI daemon trust header", () => {
  it("renders checking state before daemon status is loaded", () => {
    const model = daemonTrustHeaderModel(null);

    assert.equal(model.title, "Daemon: checking...");
    assert.equal(model.color, "yellow");
    assert.equal(model.lastEvent, "-");
  });

  it("renders running daemon trust metadata", () => {
    const model = daemonTrustHeaderModel(statusResult());

    assert.equal(model.title, "Daemon: running");
    assert.equal(model.color, "green");
    assert.equal(model.lastEvent, "2026-06-08T00:00:02.000Z");
    assert.equal(model.watchedCount, 2);
    assert.equal(model.storeDir, "/store");
  });

  it("renders stopped daemon state", () => {
    const model = daemonTrustHeaderModel(statusResult({
      running: false,
      pidAlive: false,
      identityVerified: false,
      stale: false
    }));

    assert.equal(model.title, "Daemon: stopped");
    assert.equal(model.color, "yellow");
    assert.equal(model.stale, false);
  });

  it("renders stale daemon warning", () => {
    const model = daemonTrustHeaderModel(statusResult({ running: false, stale: true }));

    assert.equal(model.title, "Daemon: stale");
    assert.equal(model.color, "red");
    assert.equal(model.stale, true);
  });

  it("renders daemon status read errors", () => {
    const model = daemonTrustHeaderModel(statusReadError());

    assert.equal(model.title, "Daemon: error");
    assert.equal(model.color, "red");
    assert.equal(model.error, "status unreadable");
  });
});

describe("TUI timeline model", () => {
  it("renders an empty state with daemon start guidance", () => {
    const model = buildTimelineViewModel({
      entries: [],
      selectedIndex: 0,
      agentFilter: null
    });

    assert.equal(model.filterLabel, "All agents");
    assert.equal(model.emptyMessage, "No timeline entries yet.");
    assert.equal(model.emptyCommand, "hem daemon start --project .");
    assert.deepEqual(model.rows, []);
    assert.equal(model.selectedEntry, undefined);
  });

  it("formats rows with event context and selected entry detail", () => {
    const baseline = timelineEntry({
      id: "baseline-entry",
      eventKind: "baseline",
      title: "baseline captured",
      observedAt: "2026-06-08T00:00:00.000Z",
      afterSnapshotName: "baseline-snapshot",
      restoreReadiness: "observe-only",
      beforeSnapshotName: undefined,
      changedSurfaces: []
    });
    const changed = timelineEntry({
      id: "changed-entry",
      observedAt: "2026-06-08T00:01:00.000Z",
      afterSnapshotName: "changed-snapshot"
    });

    const model = buildTimelineViewModel({
      entries: [changed, baseline],
      selectedIndex: 0,
      agentFilter: "claude-code"
    });

    assert.equal(model.filterLabel, "claude-code");
    assert.equal(model.rows[0].shortId, "changed-");
    assert.equal(model.rows[0].eventKind, "setup_changed");
    assert.equal(model.rows[0].readiness, "partial");
    assert.equal(model.rows[0].agentScope, "claude-code");
    assert.equal(model.rows[0].selected, true);
    assert.equal(model.selectedEntry?.beforeSnapshotName, "before");
    assert.equal(model.selectedEntry?.afterSnapshotName, "changed-snapshot");
    assert.equal(model.selectedEntry?.highlights.length, 2);
  });

  it("separates writable and observe-only changed surfaces", () => {
    const detail = timelineDetailModel(timelineEntry({
      id: "changed-entry",
      observedAt: "2026-06-08T00:01:00.000Z",
      afterSnapshotName: "changed-snapshot"
    }));

    assert.equal(detail.writableSurfaces.length, 1);
    assert.equal(detail.writableSurfaces[0].kind, "mcp_server");
    assert.equal(detail.observeOnlySurfaces.length, 1);
    assert.equal(detail.observeOnlySurfaces[0].kind, "skill");
  });

  it("renders dry-run undo preview without promising writes", () => {
    const plan: TimelineUndoPlan = {
      entryId: "changed-entry",
      title: "dry-run MCP undo: MCP server changed",
      dryRun: true,
      writesFiles: false,
      restoreReadiness: "partial",
      targetSnapshotName: "before",
      currentSnapshotName: "changed-snapshot",
      writableItems: [
        {
          action: "update",
          kind: "mcp_server",
          path: "/project/.mcp.json",
          serverName: "github"
        }
      ],
      observeOnlySurfaces: [
        {
          kind: "skill",
          changeType: "SKILL_ADDED",
          path: "/home/.claude/skills/review/SKILL.md",
          entityName: "review",
          restorable: false,
          observeOnly: true
        }
      ]
    };

    const preview = timelineUndoPreviewModel(plan);

    assert.equal(preview.writesFiles, "no");
    assert.equal(preview.writableItems.length, 1);
    assert.equal(preview.writableItems[0].action, "update");
    assert.equal(preview.observeOnlySurfaces.length, 1);
    assert.equal(preview.emptyWritableMessage, undefined);
  });

  it("keeps corrupt event warnings visible while rendering valid entries", () => {
    const model = buildTimelineViewModel({
      entries: [timelineEntry({
        id: "changed-entry",
        observedAt: "2026-06-08T00:01:00.000Z",
        afterSnapshotName: "changed-snapshot"
      })],
      selectedIndex: 0,
      agentFilter: null,
      corruptEvents: [{ filePath: "/store/timeline/events/bad.json", error: "Unexpected token" }]
    });

    assert.equal(model.corruptWarning, "1 corrupt timeline event skipped");
    assert.equal(model.rows.length, 1);
  });

  it("puts Timeline first in the tab model", () => {
    assert.equal(TABS[0].id, "timeline");
    assert.deepEqual(TABS.map((tab) => tab.id), ["timeline", "snapshots", "scan", "audit", "diff"]);
  });

  it("adds All agents as the first timeline filter", () => {
    const filters = buildAgentFilterEntries([
      { agent: "claude-code" },
      { agent: "codex" },
      { agent: "claude-code" }
    ]);

    assert.equal(filters[0].id, null);
    assert.equal(filters[0].label, "All agents");
    assert.equal(filters[0].evidenceCount, 3);
    assert.deepEqual(filters.slice(1).map((filter) => filter.id), ["claude-code", "codex"]);
  });
});
