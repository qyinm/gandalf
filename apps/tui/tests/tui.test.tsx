import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { buildAgentFilterEntries } from "../src/components/Sidebar.js";
import {
  buildCurrentSetupSummaryModel,
  buildTimelineViewModel,
  timelineDetailModel,
  timelineUndoPreviewModel
} from "../src/components/TimelineViewModel.js";
import {
  INITIAL_NAV_ITEM_ID,
  buildTuiNavigationModel,
  navItemIdForSelection,
  selectTuiNavItem
} from "../src/components/TuiNavigationModel.js";
import { buildAgentDetailViewModel } from "../src/components/AgentDetailViewModel.js";
import { buildSaveSetupViewModel } from "../src/components/SaveSetupViewModel.js";
import { buildSnapshotListViewModel } from "../src/components/SnapshotListViewModel.js";
import { buildCompareViewModel, latestSnapshotByCreatedAt } from "../src/components/CompareViewModel.js";
import { buildProfileViewModel } from "../src/components/ProfileViewModel.js";
import {
  formatAgentLabel,
  formatInventoryNameWithSource,
  formatInventorySourceRoot,
  formatTimelineTimestamp,
  truncateText
} from "../src/components/TuiFormatters.js";
import type { TimelineUndoPlan } from "@qxinm/hem-core/timeline-undo.js";
import type { DiscoveredItem, GraphNode, Snapshot, TimelineEntry } from "@qxinm/hem-core/types.js";

function timelineEntry(overrides: Partial<TimelineEntry> & Pick<TimelineEntry, "id" | "observedAt" | "afterSnapshotName">): TimelineEntry {
  const { id, afterSnapshotName, observedAt, ...rest } = overrides;
  return {
    schemaVersion: "0.1",
    id,
    source: "manual",
    eventKind: "setup_changed",
    title: "MCP server changed",
    projectPath: "/project",
    agent: "claude-code",
    agents: ["claude-code"],
    beforeSnapshotName: "before",
    afterSnapshotName,
    captureId: "capture-test",
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

function graphNode(overrides: Partial<GraphNode> & Pick<GraphNode, "id" | "agent" | "entityKind" | "entityName">): GraphNode {
  return {
    scope: "project",
    sourcePath: "/project/.mcp.json",
    effectiveValue: {},
    confidence: "high",
    evidenceId: `${overrides.id}:evidence`,
    ...overrides
  };
}

function snapshotForTui(name: string, createdAt: string, graph: GraphNode[]): Snapshot {
  return {
    manifest: {
      schemaVersion: "0.1",
      name,
      createdAt,
      projectPath: "/project",
      security: {
        rawSecretsIncluded: false,
        redactionPolicy: "metadata-only"
      }
    },
    evidence: [],
    graph,
    auditFindings: [],
    provenance: []
  };
}

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

  it("formats compact inventory source roots", () => {
    const skill = discoveredItem({
      id: "skill:review",
      agent: "claude-code",
      kind: "skill",
      name: "review",
      sourcePath: "~/.claude/skills/review",
      scope: "user"
    });
    const mcpServer = discoveredItem({
      id: "mcp:github",
      agent: "claude-code",
      kind: "mcp_server",
      name: "github",
      sourcePath: ".mcp.json",
      scope: "project"
    });
    const hook = discoveredItem({
      id: "hook:pre",
      agent: "cursor",
      kind: "hook",
      name: "pre",
      sourcePath: "~/.cursor/hooks.json",
      scope: "user"
    });
    const managed = discoveredItem({
      id: "hook:managed",
      agent: "cursor",
      kind: "hook",
      name: "managed",
      sourcePath: "/Library/Application Support/Cursor/hooks.json",
      scope: "managed"
    });

    assert.equal(formatInventorySourceRoot(skill), "~/.claude/skills");
    assert.equal(formatInventorySourceRoot(mcpServer), ".mcp.json");
    assert.equal(formatInventorySourceRoot(hook), "~/.cursor/hooks.json");
    assert.equal(formatInventorySourceRoot(managed), "Cursor/hooks.json");
    assert.equal(formatInventoryNameWithSource("github", mcpServer), "github (project: .mcp.json)");
  });

  it("renders an empty state with save setup guidance", () => {
    const model = buildTimelineViewModel({
      entries: [],
      selectedIndex: 0,
      agentFilter: null
    });

    assert.equal(model.filterLabel, "All agents");
    assert.equal(model.emptyMessage, "No timeline entries yet.");
    assert.equal(model.emptyCommand, "Save a setup to start local history.");
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

  it("summarizes the current setup above the Timeline", () => {
    const model = buildCurrentSetupSummaryModel({
      agentFilter: null,
      evidence: [
        discoveredItem({ id: "agent:claude", agent: "claude-code", kind: "agent_config" }),
        discoveredItem({
          id: "skill:review",
          agent: "claude-code",
          kind: "skill",
          name: "review",
          sourcePath: "~/.claude/skills/review",
          scope: "user"
        }),
        discoveredItem({
          id: "mcp:github",
          agent: "claude-code",
          kind: "mcp_server",
          name: "github",
          sourcePath: "~/.claude/settings.json",
          scope: "user"
        }),
        discoveredItem({
          id: "hook:pre",
          agent: "claude-code",
          kind: "hook",
          name: "pre",
          sourcePath: "~/.claude/settings.json",
          scope: "user"
        }),
        discoveredItem({ id: "permission:bash", agent: "claude-code", kind: "permission", name: "bash" }),
        discoveredItem({ id: "instructions", agent: "claude-code", kind: "agent_instruction", sourcePath: "/project/AGENTS.md" }),
        discoveredItem({
          id: "skill:codex",
          agent: "codex",
          kind: "skill",
          name: "codex-skill",
          sourcePath: "~/.codex/skills/codex-skill",
          scope: "user"
        }),
        discoveredItem({ id: "env:OPENAI_API_KEY", agent: "project", kind: "env_key", name: "OPENAI_API_KEY" })
      ]
    });

    assert.equal(model.scopeLabel, "All agents");
    assert.equal(model.agents, 2);
    assert.equal(model.skills, 2);
    assert.equal(model.mcpServers, 1);
    assert.equal(model.hooks, 1);
    assert.equal(model.permissions, 1);
    assert.equal(model.envKeys, 1);
    assert.deepEqual(model.skillRows, [
      "Claude Code: review (~/.claude/skills)",
      "Codex: codex-skill (~/.codex/skills)"
    ]);
    assert.deepEqual(model.mcpServerRows, ["Claude Code: github (~/.claude/settings.json)"]);
    assert.deepEqual(model.hookRows, ["Claude Code: pre (~/.claude/settings.json)"]);
    assert.deepEqual(model.envKeyRows, ["Project: OPENAI_API_KEY"]);
    assert.equal(model.instructions, "/project/AGENTS.md");
  });

  it("keeps all current setup inventory rows for scrollable rendering", () => {
    const model = buildCurrentSetupSummaryModel({
      agentFilter: "codex",
      evidence: Array.from({ length: 8 }, (_, index) =>
        discoveredItem({
          id: `skill:${index}`,
          agent: "codex",
          kind: "skill",
          name: `skill-${index}`,
          sourcePath: `~/.codex/skills/skill-${index}`,
          scope: "user"
        })
      )
    });

    assert.equal(model.skills, 8);
    assert.equal(model.skillRows.length, 8);
    assert.deepEqual(model.skillRows.slice(0, 2), [
      "skill-0 (~/.codex/skills)",
      "skill-1 (~/.codex/skills)"
    ]);
    assert.deepEqual(model.skillRows.slice(-2), [
      "skill-6 (~/.codex/skills)",
      "skill-7 (~/.codex/skills)"
    ]);
  });

  it("summarizes the current setup for an agent-filtered Timeline", () => {
    const model = buildCurrentSetupSummaryModel({
      agentFilter: "claude-code",
      evidence: [
        discoveredItem({
          id: "skill:review",
          agent: "claude-code",
          kind: "skill",
          name: "review",
          sourcePath: "~/.claude/skills/review",
          scope: "user"
        }),
        discoveredItem({
          id: "mcp:github",
          agent: "claude-code",
          kind: "mcp_server",
          name: "github",
          sourcePath: ".mcp.json",
          scope: "project"
        }),
        discoveredItem({
          id: "hook:pre",
          agent: "claude-code",
          kind: "hook",
          name: "pre",
          sourcePath: "~/.claude/settings.json",
          scope: "user"
        }),
        discoveredItem({ id: "skill:codex", agent: "codex", kind: "skill", name: "codex-skill" }),
        discoveredItem({ id: "env:OPENAI_API_KEY", agent: "project", kind: "env_key", name: "OPENAI_API_KEY" })
      ]
    });

    assert.equal(model.scopeLabel, "Claude Code");
    assert.equal(model.agents, 1);
    assert.equal(model.skills, 1);
    assert.equal(model.mcpServers, 1);
    assert.equal(model.hooks, 1);
    assert.equal(model.envKeys, 1);
    assert.deepEqual(model.skillRows, ["review (~/.claude/skills)"]);
    assert.deepEqual(model.mcpServerRows, ["github (project: .mcp.json)"]);
    assert.deepEqual(model.hookRows, ["pre (~/.claude/settings.json)"]);
    assert.deepEqual(model.envKeyRows, ["OPENAI_API_KEY (project)"]);
  });

  it("does not leak another agent MCP into the OpenCode current setup", () => {
    const model = buildCurrentSetupSummaryModel({
      agentFilter: "opencode",
      evidence: [
        discoveredItem({ id: "mcp:github", agent: "claude-code", kind: "mcp_server", name: "github" }),
        discoveredItem({
          id: "skill:opencode",
          agent: "opencode",
          kind: "skill",
          name: "customize-opencode",
          scope: "managed",
          metadata: { builtIn: true }
        })
      ]
    });

    assert.equal(model.scopeLabel, "OpenCode");
    assert.equal(model.mcpServers, 0);
    assert.deepEqual(model.mcpServerRows, []);
    assert.deepEqual(model.skillRows, ["customize-opencode (built-in)"]);
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
        { agent: "claude-code", kind: "skill" },
        { agent: "codex", kind: "mcp_server" },
        { agent: "claude-code", kind: "hook" },
        { agent: "project", kind: "env_key" }
      ]
    });

    assert.deepEqual(model.sections.map((section) => section.label), ["Profiles", "Agents", "History"]);
    assert.equal(model.sections[0].items[0].label, "default");
    assert.deepEqual(model.sections[1].items.map((item) => item.label), [
      "Claude Code",
      "Codex"
    ]);
    assert.deepEqual(model.sections[1].items.map((item) => item.evidenceCount), [2, 1]);
    assert.deepEqual(model.sections[2].items.map((item) => item.label), ["All changes", "Snapshots"]);
    assert.equal(model.selectedItemId, INITIAL_NAV_ITEM_ID);
    assert.equal(model.flatItems[model.cursor]?.id, INITIAL_NAV_ITEM_ID);
  });

  it("counts only setup inventory in agent navigation totals", () => {
    const model = buildTuiNavigationModel({
      evidence: [
        { agent: "claude-code", kind: "skill" },
        { agent: "claude-code", kind: "hook" },
        { agent: "claude-code", kind: "permission" },
        { agent: "claude-code", kind: "agent_config" },
        { agent: "claude-code", kind: "agent_instruction" },
        { agent: "codex", kind: "mcp_server" }
      ]
    });

    assert.deepEqual(model.sections[1].items.map((item) => item.evidenceCount), [2, 1]);
  });

  it("keeps agent selection on Timeline as a filter", () => {
    const model = buildTuiNavigationModel({
      evidence: [{ agent: "claude-code", kind: "skill" }]
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

  it("marks the agent nav item as selected when Timeline is agent-filtered", () => {
    const selectedItemId = navItemIdForSelection({
      screen: "timeline",
      selectedAgent: "claude-code",
      selectedProfile: "default"
    });
    const model = buildTuiNavigationModel({
      evidence: [{ agent: "claude-code", kind: "skill" }],
      selectedItemId
    });

    assert.equal(selectedItemId, "agent:claude-code");
    assert.equal(model.selectedItemId, "agent:claude-code");
    assert.equal(model.flatItems[model.cursor]?.id, "agent:claude-code");
  });

  it("opens agent detail when selecting an agent outside Timeline", () => {
    const model = buildTuiNavigationModel({
      evidence: [{ agent: "claude-code", kind: "skill" }]
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
      { agent: "claude-code", kind: "skill" },
      { agent: "codex", kind: "mcp_server" },
      { agent: "claude-code", kind: "hook" },
      { agent: "claude-code", kind: "permission" }
    ]);

    assert.equal(filters[0].id, null);
    assert.equal(filters[0].label, "All agents");
    assert.equal(filters[0].evidenceCount, 3);
    assert.deepEqual(filters.slice(1).map((filter) => filter.evidenceCount), [2, 1]);
    assert.deepEqual(filters.slice(1).map((filter) => filter.id), ["claude-code", "codex"]);
  });
});

describe("TUI agent detail model", () => {
  it("builds current setup inventory and filtered history for an agent", () => {
    const model = buildAgentDetailViewModel({
      agent: "claude-code",
      evidence: [
        discoveredItem({
          id: "skill:review",
          agent: "claude-code",
          kind: "skill",
          name: "review",
          sourcePath: "~/.claude/skills/review",
          scope: "user"
        }),
        discoveredItem({
          id: "skill:broken",
          agent: "claude-code",
          kind: "skill",
          name: "broken",
          sourcePath: "~/.claude/skills/broken",
          scope: "user",
          captureStatus: "parse_failed"
        }),
        discoveredItem({
          id: "mcp:docs",
          agent: "claude-code",
          kind: "mcp_server",
          name: "docs",
          sourcePath: "~/.claude/settings.json",
          scope: "user"
        }),
        discoveredItem({
          id: "mcp:github",
          agent: "claude-code",
          kind: "mcp_server",
          name: "github",
          sourcePath: "~/.claude/settings.json",
          scope: "user",
          value: { disabled: true }
        }),
        discoveredItem({
          id: "mcp:linear",
          agent: "claude-code",
          kind: "mcp_server",
          name: "linear",
          sourcePath: ".mcp.json",
          scope: "project",
          value: { enabled: false }
        }),
        discoveredItem({ id: "permission:bash", agent: "claude-code", kind: "permission", name: "bash" }),
        discoveredItem({
          id: "hook:pre",
          agent: "claude-code",
          kind: "hook",
          name: "pre-run",
          sourcePath: "~/.claude/settings.json",
          scope: "user"
        }),
        discoveredItem({ id: "instructions", agent: "claude-code", kind: "agent_instruction", sourcePath: "/project/AGENTS.md" }),
        discoveredItem({ id: "skill:codex", agent: "codex", kind: "skill", name: "codex-skill" }),
        discoveredItem({ id: "env:OPENAI_API_KEY", agent: "project", kind: "env_key", name: "OPENAI_API_KEY" })
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
      skills: 2,
      mcpServers: 3,
      hooks: 1,
      permissions: 1,
      envKeys: 1,
      instructions: 1
    });
    assert.equal(model.skills.find((row) => row.name === "broken (~/.claude/skills)")?.status, "parse_failed");
    assert.equal(model.skills.find((row) => row.name === "review (~/.claude/skills)")?.status, undefined);
    assert.equal(model.mcpServers.find((row) => row.name === "docs (~/.claude/settings.json)")?.status, "enabled");
    assert.equal(model.mcpServers.find((row) => row.name === "github (~/.claude/settings.json)")?.status, "disabled");
    assert.equal(model.mcpServers.find((row) => row.name === "linear (project: .mcp.json)")?.status, "disabled");
    assert.deepEqual(model.hooks.map((row) => row.name), ["pre-run (~/.claude/settings.json)"]);
    assert.equal(model.envKeys[0].name, "OPENAI_API_KEY (project)");
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

describe("TUI save setup model", () => {
  it("uses capture baseline for the first full setup snapshot", () => {
    const model = buildSaveSetupViewModel({ hasPreviousSnapshot: false });

    assert.equal(model.title, "capture baseline");
    assert.equal(model.detectedChanges[0], "capture baseline");
    assert.equal(model.destinations[0].label, "Local history");
    assert.equal(model.destinations[0].selected, true);
    assert.equal(model.destinations[1].label, "Export as .hem");
  });

  it("generates deterministic titles from structured changes", () => {
    const model = buildSaveSetupViewModel({
      hasPreviousSnapshot: true,
      diff: {
        semanticChanges: [
          {
            code: "SKILL_ADDED",
            entityKind: "skill",
            entityName: "react-review",
            severity: "low",
            details: { changedFields: [], sourcePath: "/skills/react-review/SKILL.md" }
          }
        ],
        rawSourceChanges: []
      }
    });

    assert.equal(model.title, "install react-review skill");
    assert.deepEqual(model.detectedChanges, ["install react-review skill"]);
  });

  it("renders no-change saved setup state without proposing duplicate changes", () => {
    const model = buildSaveSetupViewModel({
      hasPreviousSnapshot: true,
      diff: { semanticChanges: [], rawSourceChanges: [] }
    });

    assert.equal(model.noChanges, true);
    assert.equal(model.title, "current setup unchanged");
    assert.deepEqual(model.detectedChanges, ["Current setup matches latest saved setup."]);
  });

  it("uses saved setup empty state copy for root snapshots", () => {
    const model = buildSnapshotListViewModel({ names: [] });

    assert.equal(model.emptyMessage, "No saved setups yet.");
    assert.equal(model.emptyAction, "s save setup");
  });
});

describe("TUI compare model", () => {
  it("selects the latest snapshot by manifest creation time", () => {
    const older = snapshotForTui("z-name", "2026-06-07T00:00:00.000Z", []);
    const newer = snapshotForTui("a-name", "2026-06-08T00:00:00.000Z", []);

    assert.equal(latestSnapshotByCreatedAt([older, newer])?.manifest.name, "a-name");
  });

  it("builds explicit compare labels and side-by-side rows", () => {
    const before = snapshotForTui("baseline", "2026-06-07T00:00:00.000Z", [
      graphNode({
        id: "mcp-linear-before",
        agent: "claude-code",
        entityKind: "mcp_server",
        entityName: "linear",
        effectiveValue: { command: "linear-old" }
      }),
      graphNode({
        id: "hook-pre-before",
        agent: "claude-code",
        entityKind: "hook",
        entityName: "pre-tool-use",
        effectiveValue: { command: "notify" }
      })
    ]);
    const after = snapshotForTui("current", "2026-06-08T00:00:00.000Z", [
      graphNode({
        id: "mcp-linear-after",
        agent: "claude-code",
        entityKind: "mcp_server",
        entityName: "linear",
        effectiveValue: { command: "linear-new" }
      }),
      graphNode({
        id: "skill-review-after",
        agent: "claude-code",
        entityKind: "skill",
        entityName: "react-review",
        effectiveValue: { installed: true }
      })
    ]);

    const model = buildCompareViewModel({
      fromSnapshot: before,
      toSnapshot: after,
      toLabel: "Current  unsaved changes",
      diff: {
        semanticChanges: [
          {
            code: "SKILL_ADDED",
            entityKind: "skill",
            entityName: "react-review",
            severity: "low",
            details: { changedFields: [], sourcePath: "/skills/react-review/SKILL.md" }
          }
        ],
        rawSourceChanges: []
      }
    });

    assert.match(model.fromLabel, /^baseline/);
    assert.equal(model.toLabel, "Current  unsaved changes");
    assert.equal(model.scopeLabel, "Full setup");
    assert.deepEqual(model.summary, ["+ Skill: react-review"]);
    assert.equal(model.sections[0].title, "Claude Code");
    assert.equal(model.sections[0].rows.some((row) => row.marker === "+" && row.after === "skill: react-review"), true);
    assert.equal(model.sections[0].rows.some((row) => row.marker === "-" && row.before === "hook: pre-tool-use"), true);
    assert.equal(
      model.sections[0].rows.some((row) => row.marker === "~" && row.before === "mcp_server: linear" && row.after === "mcp_server: linear"),
      true
    );
  });
});

describe("TUI profile model", () => {
  it("renders the default profile summary from snapshots, agents, and timeline", () => {
    const model = buildProfileViewModel({
      evidence: [
        discoveredItem({ id: "claude", agent: "claude-code", kind: "agent_config" }),
        discoveredItem({ id: "codex", agent: "codex", kind: "agent_config" })
      ],
      snapshotNames: ["baseline", "current"],
      timelineEntries: [
        timelineEntry({
          id: "older",
          observedAt: "2026-06-07T12:00:00.000",
          afterSnapshotName: "baseline"
        }),
        timelineEntry({
          id: "newer",
          observedAt: "2026-06-08T14:22:00.000",
          afterSnapshotName: "current"
        })
      ],
      now: new Date("2026-06-08T15:00:00.000")
    });

    assert.equal(model.title, "Profiles");
    assert.equal(model.profiles[0].name, "default");
    assert.equal(model.profiles[0].selected, true);
    assert.equal(model.profiles[0].snapshotCount, 2);
    assert.equal(model.profiles[0].agents, "Claude Code, Codex");
    assert.equal(model.profiles[0].changedAt, "Today 14:22");
  });
});
