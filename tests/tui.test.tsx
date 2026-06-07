import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { daemonTrustHeaderModel } from "../src/tui/components/Dashboard.js";
import { buildAgentFilterEntries } from "../src/tui/components/Sidebar.js";
import {
  buildTimelineViewModel,
  timelineDetailModel,
  timelineUndoPreviewModel
} from "../src/tui/components/TimelineViewModel.js";
import {
  INITIAL_NAV_ITEM_ID,
  buildTuiNavigationModel,
  selectTuiNavItem
} from "../src/tui/components/TuiNavigationModel.js";
import { buildAgentDetailViewModel } from "../src/tui/components/AgentDetailViewModel.js";
import {
  formatAgentLabel,
  formatTimelineTimestamp,
  truncateText
} from "../src/tui/components/TuiFormatters.js";
import type { TimelineUndoPlan } from "../src/timeline-undo.js";
import type { DaemonStatusReadResult, DiscoveredItem, TimelineEntry } from "../src/types.js";

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

function discoveredItem(overrides: Partial<DiscoveredItem> & Pick<DiscoveredItem, "id" | "agent" | "kind">): DiscoveredItem {
  return {
    sourcePath: "/project/AGENTS.md",
    scope: "project",
    precedence: 0,
    parser: "json",
    sensitivity: "none",
    contentPolicy: "metadata-only",
    restorePolicy: "not_supported",
    captureStatus: "captured",
    confidence: "high",
    ...overrides
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
  it("formats shared display labels and widths", () => {
    assert.equal(formatAgentLabel("claude-code"), "Claude Code");
    assert.equal(formatAgentLabel("opencode"), "OpenCode");
    assert.equal(formatAgentLabel("pi-agent"), "Pi Agent");
    assert.equal(
      formatTimelineTimestamp("2026-06-08T14:22:00.000", new Date("2026-06-08T15:00:00.000")),
      "Today 14:22"
    );
    assert.equal(
      formatTimelineTimestamp("2026-06-07T14:22:00.000", new Date("2026-06-08T15:00:00.000")),
      "Yesterday 14:22"
    );
    assert.equal(truncateText("abcdefghijkl", 8), "abcde...");
  });

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
      observedAt: "2026-06-08T00:00:00.000",
      afterSnapshotName: "baseline-snapshot",
      restoreReadiness: "observe-only",
      beforeSnapshotName: undefined,
      changedSurfaces: []
    });
    const changed = timelineEntry({
      id: "changed-entry",
      observedAt: "2026-06-08T00:01:00.000",
      afterSnapshotName: "changed-snapshot"
    });

    const model = buildTimelineViewModel({
      entries: [changed, baseline],
      selectedIndex: 0,
      agentFilter: "claude-code",
      now: new Date("2026-06-08T00:02:00.000")
    });

    assert.equal(model.filterLabel, "Claude Code");
    assert.equal(model.rows[0].shortId, "changed-");
    assert.equal(model.rows[0].observedAt, "Today 00:01");
    assert.equal(model.rows[0].eventKind, "setup_changed");
    assert.equal(model.rows[0].readiness, "partial");
    assert.equal(model.rows[0].agentScope, "Claude Code");
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

  it("builds the design navigation sections with Timeline selected first", () => {
    const model = buildTuiNavigationModel({
      evidence: [
        { agent: "claude-code" },
        { agent: "codex" },
        { agent: "claude-code" }
      ]
    });

    assert.deepEqual(model.sections.map((section) => section.label), ["Profiles", "Agents", "History"]);
    assert.equal(model.sections[0].items[0].label, "default");
    assert.deepEqual(model.sections[1].items.map((item) => item.label), ["Claude Code", "Codex"]);
    assert.deepEqual(model.sections[2].items.map((item) => item.label), ["All changes", "Snapshots"]);
    assert.equal(model.selectedItemId, INITIAL_NAV_ITEM_ID);
    assert.equal(model.flatItems[model.cursor]?.id, INITIAL_NAV_ITEM_ID);
  });

  it("keeps agent selection on Timeline as a filter", () => {
    const model = buildTuiNavigationModel({
      evidence: [{ agent: "claude-code" }]
    });
    const agentItem = model.flatItems.find((item) => item.id === "agent:claude-code");
    assert.ok(agentItem);

    const selection = selectTuiNavItem({
      item: agentItem,
      currentScreen: "timeline",
      currentAgent: null,
      currentProfile: "default"
    });

    assert.equal(selection.screen, "timeline");
    assert.equal(selection.selectedAgent, "claude-code");
  });

  it("opens agent detail when selecting an agent outside Timeline", () => {
    const model = buildTuiNavigationModel({
      evidence: [{ agent: "claude-code" }]
    });
    const agentItem = model.flatItems.find((item) => item.id === "agent:claude-code");
    assert.ok(agentItem);

    const selection = selectTuiNavItem({
      item: agentItem,
      currentScreen: "snapshots",
      currentAgent: null,
      currentProfile: "default"
    });

    assert.equal(selection.screen, "agent-detail");
    assert.equal(selection.selectedAgent, "claude-code");
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

describe("TUI agent detail model", () => {
  it("builds current setup inventory and filtered history for an agent", () => {
    const model = buildAgentDetailViewModel({
      agent: "claude-code",
      evidence: [
        discoveredItem({ id: "skill:review", agent: "claude-code", kind: "skill", name: "review" }),
        discoveredItem({ id: "mcp:github", agent: "claude-code", kind: "mcp_server", name: "github", value: { disabled: true } }),
        discoveredItem({ id: "permission:bash", agent: "claude-code", kind: "permission", name: "bash" }),
        discoveredItem({ id: "hook:pre", agent: "claude-code", kind: "hook", name: "pre-run" }),
        discoveredItem({ id: "instructions", agent: "claude-code", kind: "agent_instruction", sourcePath: "/project/AGENTS.md" }),
        discoveredItem({ id: "skill:codex", agent: "codex", kind: "skill", name: "codex-skill" })
      ],
      timelineEntries: [
        timelineEntry({
          id: "claude-change",
          observedAt: "2026-06-08T14:22:00.000",
          afterSnapshotName: "after"
        }),
        timelineEntry({
          id: "codex-change",
          agent: "codex",
          agents: ["codex"],
          observedAt: "2026-06-08T14:30:00.000",
          afterSnapshotName: "after-codex"
        })
      ],
      now: new Date("2026-06-08T15:00:00.000")
    });

    assert.equal(model.title, "Claude Code");
    assert.deepEqual(model.counts, {
      skills: 1,
      mcpServers: 1,
      hooks: 1,
      permissions: 1,
      instructions: 1
    });
    assert.equal(model.skills[0].name, "review");
    assert.equal(model.mcpServers[0].name, "github");
    assert.equal(model.mcpServers[0].status, "disabled");
    assert.equal(model.instructions[0].path, "/project/AGENTS.md");
    assert.equal(model.history.length, 1);
    assert.equal(model.history[0].id, "claude-c");
    assert.equal(model.history[0].observedAt, "Today 14:22");
  });

  it("shows an empty message for agents without captured evidence", () => {
    const model = buildAgentDetailViewModel({
      agent: "cursor",
      evidence: [],
      timelineEntries: []
    });

    assert.equal(model.emptyMessage, "No supported agent setup found.");
  });
});
