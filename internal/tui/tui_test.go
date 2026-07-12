package tui_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/baseline"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	timelineundo "github.com/qyinm/gandalf/internal/gandalfcore/timeline_undo"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
	"github.com/qyinm/gandalf/internal/tui"
	"github.com/qyinm/gandalf/internal/tui/views"
)

func timelineEntry(overrides map[string]any) types.TimelineEntry {
	entry := types.TimelineEntry{
		SchemaVersion:      "0.1",
		ID:                 "changed-entry",
		Source:             types.TimelineSourceManual,
		EventKind:          types.TimelineEventSetupChanged,
		Title:              "MCP server changed",
		ProjectPath:        "/project",
		Agents:             []types.AgentID{types.AgentClaudeCode},
		BeforeSnapshotName: strPtr("before"),
		AfterSnapshotName:  "changed-snapshot",
		CaptureID:          "capture-test",
		CreatedAt:          "2026-06-08T00:01:00.000Z",
		ObservedAt:         "2026-06-08T00:01:00.000Z",
		ChangedSurfaces: []types.TimelineChangedSurface{
			{
				Kind:        "mcp_server",
				ChangeType:  "MCP_CHANGED",
				Path:        "/project/.mcp.json",
				EntityName:  strPtr("github"),
				Restorable:  true,
				ObserveOnly: false,
			},
			{
				Kind:        "skill",
				ChangeType:  "SKILL_ADDED",
				Path:        "/home/.claude/skills/review/SKILL.md",
				EntityName:  strPtr("review"),
				Restorable:  false,
				ObserveOnly: true,
			},
		},
		RestoreReadiness:  types.TimelineRestorePartial,
		Confidence:        types.TimelineConfidenceHigh,
		ConfidenceReason:  "semantic diff matched setup files",
		EvidenceCount:     2,
		GraphNodeCount:    2,
		AuditFindingCount: 0,
		Changes: types.TimelineChangeSummary{
			PreviousEntryID:      strPtr("prev"),
			PreviousSnapshotName: strPtr("before"),
			HasChanges:           true,
			SemanticChangeCount:  2,
			Highlights:           []string{"MCP_CHANGED: github", "SKILL_ADDED: review"},
		},
	}
	if id, ok := overrides["id"].(string); ok {
		entry.ID = id
	}
	if observedAt, ok := overrides["observedAt"].(string); ok {
		entry.ObservedAt = observedAt
		entry.CreatedAt = observedAt
	}
	if after, ok := overrides["afterSnapshotName"].(string); ok {
		entry.AfterSnapshotName = after
	}
	if eventKind, ok := overrides["eventKind"].(types.TimelineEntryEventKind); ok {
		entry.EventKind = eventKind
	}
	if title, ok := overrides["title"].(string); ok {
		entry.Title = title
	}
	if readiness, ok := overrides["restoreReadiness"].(types.TimelineRestoreReadiness); ok {
		entry.RestoreReadiness = readiness
	}
	if before, ok := overrides["beforeSnapshotName"].(*string); ok {
		entry.BeforeSnapshotName = before
	}
	if changedSurfaces, ok := overrides["changedSurfaces"].([]types.TimelineChangedSurface); ok {
		entry.ChangedSurfaces = changedSurfaces
	}
	if agent, ok := overrides["agent"].(types.AgentID); ok {
		entry.Agent = &agent
		entry.Agents = []types.AgentID{agent}
	}
	return entry
}

func discoveredItem(overrides map[string]any) types.DiscoveredItem {
	item := types.DiscoveredItem{
		ID:            "item",
		Agent:         types.AgentClaudeCode,
		Kind:          types.KindSkill,
		SourcePath:    "/project/AGENTS.md",
		Scope:         types.ScopeProject,
		Precedence:    0,
		Parser:        types.ParserJSON,
		Sensitivity:   "none",
		ContentPolicy: "metadata-only",
		RestorePolicy: types.RestoreNotSupported,
		CaptureStatus: types.CaptureCaptured,
		Confidence:    types.ConfidenceHigh,
	}
	if id, ok := overrides["id"].(string); ok {
		item.ID = id
	}
	if agent, ok := overrides["agent"].(types.AgentID); ok {
		item.Agent = agent
	}
	if kind, ok := overrides["kind"].(types.EvidenceKind); ok {
		item.Kind = kind
	}
	if name, ok := overrides["name"].(string); ok {
		item.Name = &name
	}
	if sourcePath, ok := overrides["sourcePath"].(string); ok {
		item.SourcePath = sourcePath
	}
	if scope, ok := overrides["scope"].(types.EvidenceScope); ok {
		item.Scope = scope
	}
	if metadata, ok := overrides["metadata"].(json.RawMessage); ok {
		item.Metadata = metadata
	}
	if value, ok := overrides["value"].(json.RawMessage); ok {
		item.Value = value
	}
	if captureStatus, ok := overrides["captureStatus"].(types.CaptureStatus); ok {
		item.CaptureStatus = captureStatus
	}
	return item
}

func graphNode(overrides map[string]any) types.GraphNode {
	node := types.GraphNode{
		ID:             "node",
		Agent:          types.AgentClaudeCode,
		Scope:          types.ScopeProject,
		SourcePath:     "/project/.mcp.json",
		EntityKind:     types.KindMcpServer,
		EntityName:     "linear",
		EffectiveValue: json.RawMessage(`{"command":"linear-old"}`),
		Confidence:     types.ConfidenceHigh,
		EvidenceID:     "node:evidence",
	}
	if id, ok := overrides["id"].(string); ok {
		node.ID = id
	}
	if entityKind, ok := overrides["entityKind"].(types.EvidenceKind); ok {
		node.EntityKind = entityKind
	}
	if entityName, ok := overrides["entityName"].(string); ok {
		node.EntityName = entityName
	}
	if effectiveValue, ok := overrides["effectiveValue"].(json.RawMessage); ok {
		node.EffectiveValue = effectiveValue
	}
	return node
}

func snapshotForTui(name, createdAt string, graph []types.GraphNode) types.Snapshot {
	return types.Snapshot{
		Manifest: types.SnapshotManifest{
			SchemaVersion: "0.1",
			Name:          name,
			CreatedAt:     createdAt,
			ProjectPath:   "/project",
			Security: types.SnapshotSecurity{
				RawSecretsIncluded: false,
				RedactionPolicy:    "metadata-only",
			},
		},
		Graph: graph,
	}
}

func strPtr(value string) *string {
	return &value
}

func TestFormattersAndSourceRootLabels(t *testing.T) {
	if got := tui.FormatAgentLabel(types.AgentClaudeCode); got != "Claude Code" {
		t.Fatalf("FormatAgentLabel: got %q", got)
	}
	if got := tui.FormatAgentLabel(types.AgentOpencode); got != "OpenCode" {
		t.Fatalf("FormatAgentLabel opencode: got %q", got)
	}

	now := time.Date(2026, 6, 8, 15, 0, 0, 0, time.UTC)
	if got := tui.FormatTimelineTimestamp("2026-06-08T14:22:00.000Z", now); got != "Today 14:22" {
		t.Fatalf("today timestamp: got %q", got)
	}
	if got := tui.FormatTimelineTimestamp("2026-06-07T14:22:00.000Z", now); got != "Yesterday 14:22" {
		t.Fatalf("yesterday timestamp: got %q", got)
	}
	if got := tui.TruncateText("abcdefghijkl", 8); got != "abcde..." {
		t.Fatalf("truncate: got %q", got)
	}

	skill := discoveredItem(map[string]any{
		"id": "skill:review", "agent": types.AgentClaudeCode, "kind": types.KindSkill,
		"name": "review", "sourcePath": "~/.claude/skills/review", "scope": types.ScopeUser,
	})
	mcpServer := discoveredItem(map[string]any{
		"id": "mcp:github", "agent": types.AgentClaudeCode, "kind": types.KindMcpServer,
		"name": "github", "sourcePath": ".mcp.json", "scope": types.ScopeProject,
	})
	hook := discoveredItem(map[string]any{
		"id": "hook:pre", "agent": types.AgentCursor, "kind": types.KindHook,
		"name": "pre", "sourcePath": "~/.cursor/hooks.json", "scope": types.ScopeUser,
	})
	managed := discoveredItem(map[string]any{
		"id": "hook:managed", "agent": types.AgentCursor, "kind": types.KindHook,
		"name": "managed", "sourcePath": "/Library/Application Support/Cursor/hooks.json", "scope": types.ScopeManaged,
	})

	if got := tui.FormatInventorySourceRoot(skill); got != "~/.claude/skills" {
		t.Fatalf("skill source root: got %q", got)
	}
	if got := tui.FormatInventorySourceRoot(mcpServer); got != ".mcp.json" {
		t.Fatalf("mcp source root: got %q", got)
	}
	if got := tui.FormatInventorySourceRoot(hook); got != "~/.cursor/hooks.json" {
		t.Fatalf("hook source root: got %q", got)
	}
	if got := tui.FormatInventorySourceRoot(managed); got != "Cursor/hooks.json" {
		t.Fatalf("managed source root: got %q", got)
	}
	if got := tui.FormatInventoryNameWithSource("github", mcpServer); got != "github (project: .mcp.json)" {
		t.Fatalf("name with source: got %q", got)
	}
}

func TestBuildBaselineStatusViewModel(t *testing.T) {
	model := tui.BuildBaselineStatusViewModel(baseline.Status{
		Agents: []baseline.AgentStatus{
			{
				Agent:               types.AgentClaudeCode,
				HasBaseline:         false,
				UnsupportedCount:    2,
				OmittedContentCount: 1,
			},
			{
				Agent:               types.AgentCodex,
				HasBaseline:         true,
				BaselineName:        "baseline-codex",
				SemanticChangeCount: 1,
			},
		},
	})

	if !model.HasMissing || !model.HasChanges {
		t.Fatalf("model flags = %#v", model)
	}
	if len(model.Rows) != 2 {
		t.Fatalf("rows = %#v", model.Rows)
	}
	if model.Rows[0].Status != "missing baseline" || model.Rows[0].Baseline != "-" {
		t.Fatalf("missing row = %#v", model.Rows[0])
	}
	if model.Rows[0].Unsupported != "2 unsupported, 1 omitted" {
		t.Fatalf("unsupported label = %q", model.Rows[0].Unsupported)
	}
	if model.Rows[1].Status != "changed" || model.Rows[1].Changes != "1 changes" {
		t.Fatalf("changed row = %#v", model.Rows[1])
	}
}

func TestBuildHeaderChipsDistinguishesRawSourceDriftFromClean(t *testing.T) {
	chips := tui.BuildHeaderChips(baseline.Status{Agents: []baseline.AgentStatus{{
		Agent: types.AgentCodex, HasBaseline: true, RawChangeCount: 2,
	}}})
	if len(chips) != 1 || chips[0].State != "drift" || chips[0].Detail != "source drift" || !chips[0].SourceDrift {
		t.Fatalf("raw-only chip = %#v", chips)
	}
}

func TestBuildHomeViewModelWithoutBaseline(t *testing.T) {
	model := tui.BuildHomeViewModel(baseline.Status{Agents: []baseline.AgentStatus{
		{Agent: types.AgentClaudeCode},
		{Agent: types.AgentCodex},
	}})
	if model.HasBaseline || !model.HasMissingBaseline {
		t.Fatalf("baseline state = %#v", model)
	}
	if model.TotalChanges != 0 || model.LastSnapshotAt != "" || len(model.TopChanges) != 0 {
		t.Fatalf("empty home state = %#v", model)
	}
}

func TestBuildHomeViewModelKeepsAvailableDriftWithPartialBaselineCoverage(t *testing.T) {
	model := tui.BuildHomeViewModel(baseline.Status{Agents: []baseline.AgentStatus{
		{Agent: types.AgentClaudeCode},
		{Agent: types.AgentCodex, HasBaseline: true, Diff: diff.GraphDiff{SemanticChanges: []diff.SemanticChange{{
			Code: diff.SemanticSkillAdded, EntityKind: types.KindSkill, EntityName: "review",
		}}}},
	}})
	if !model.HasBaseline || !model.HasMissingBaseline || model.TotalChanges != 1 {
		t.Fatalf("partial baseline home = %#v", model)
	}
}

func TestBuildHomeViewModelSummarizesPresentBaselines(t *testing.T) {
	model := tui.BuildHomeViewModel(baseline.Status{Agents: []baseline.AgentStatus{
		{
			Agent: types.AgentClaudeCode, HasBaseline: true, BaselineCreatedAt: "2026-07-12T00:00:00Z",
			SemanticChangeCount: 2,
			Diff: diff.GraphDiff{SemanticChanges: []diff.SemanticChange{
				{Code: diff.SemanticSkillAdded, EntityKind: types.KindSkill, EntityName: "review"},
				{Code: diff.SemanticHookChanged, EntityKind: types.KindHook, EntityName: "format"},
			}},
		},
		{
			Agent: types.AgentCodex, HasBaseline: true, BaselineCreatedAt: "2026-07-12T01:00:00Z",
			SemanticChangeCount: 3, RawChangeCount: 7,
			Diff: diff.GraphDiff{SemanticChanges: []diff.SemanticChange{
				{Code: diff.SemanticMcpChanged, EntityKind: types.KindMcpServer, EntityName: "posthog"},
				{Code: diff.SemanticAgentConfigChanged, EntityKind: types.KindExtension, EntityName: "linear"},
				{Code: diff.SemanticAgentConfigChanged, EntityKind: types.KindAgentConfig, EntityName: "config"},
			}},
		},
	}})
	if !model.HasBaseline || model.HasMissingBaseline {
		t.Fatalf("baseline state = %#v", model)
	}
	if model.LastSnapshotAt != "2026-07-12T01:00:00Z" || model.TotalChanges != 5 {
		t.Fatalf("summary = %#v", model)
	}
	if model.SkillsChanged != 1 || model.HooksChanged != 1 || model.MCPServersChanged != 1 || model.PluginsChanged != 1 {
		t.Fatalf("counts = %#v", model)
	}
	if model.OtherChanged != 1 {
		t.Fatalf("other count = %d", model.OtherChanged)
	}
	if got := model.SkillsChanged + model.HooksChanged + model.MCPServersChanged + model.PluginsChanged + model.OtherChanged; got != model.TotalChanges {
		t.Fatalf("category total = %d, home total = %d", got, model.TotalChanges)
	}
	if len(model.TopChanges) != 5 || model.TopChanges[0].Action != "added" || model.TopChanges[2].Name != "posthog" {
		t.Fatalf("top changes = %#v", model.TopChanges)
	}
}

func TestTimelineCurrentSetupSourceRootRows(t *testing.T) {
	model := tui.BuildCurrentSetupSummaryModel(tui.BuildCurrentSetupSummaryInput{
		AgentFilter: nil,
		Evidence: []types.DiscoveredItem{
			discoveredItem(map[string]any{"id": "agent:claude", "agent": types.AgentClaudeCode, "kind": types.KindAgentConfig}),
			discoveredItem(map[string]any{
				"id": "skill:review", "agent": types.AgentClaudeCode, "kind": types.KindSkill,
				"name": "review", "sourcePath": "~/.claude/skills/review", "scope": types.ScopeUser,
			}),
			discoveredItem(map[string]any{
				"id": "mcp:github", "agent": types.AgentClaudeCode, "kind": types.KindMcpServer,
				"name": "github", "sourcePath": "~/.claude/settings.json", "scope": types.ScopeUser,
			}),
			discoveredItem(map[string]any{
				"id": "skill:codex", "agent": types.AgentCodex, "kind": types.KindSkill,
				"name": "codex-skill", "sourcePath": "~/.codex/skills/codex-skill", "scope": types.ScopeUser,
			}),
			discoveredItem(map[string]any{"id": "env:OPENAI_API_KEY", "agent": types.AgentProject, "kind": types.KindEnvKey, "name": "OPENAI_API_KEY"}),
		},
	})

	wantSkills := []string{
		"Claude Code: review (~/.claude/skills)",
		"Codex: codex-skill (~/.codex/skills)",
	}
	if strings.Join(model.SkillRows, "|") != strings.Join(wantSkills, "|") {
		t.Fatalf("skill rows: got %#v want %#v", model.SkillRows, wantSkills)
	}
	if strings.Join(model.McpServerRows, "|") != "Claude Code: github (~/.claude/settings.json)" {
		t.Fatalf("mcp rows: got %#v", model.McpServerRows)
	}
	if strings.Join(model.EnvKeyRows, "|") != "Project: OPENAI_API_KEY" {
		t.Fatalf("env rows: got %#v", model.EnvKeyRows)
	}
}

func TestSetupInventoryViewModelShowsGlobalItemsWithAgentMarkers(t *testing.T) {
	evidence := []types.DiscoveredItem{
		discoveredItem(map[string]any{
			"id": "skill:review", "agent": types.AgentClaudeCode, "kind": types.KindSkill,
			"name": "review", "sourcePath": "~/.claude/skills/review", "scope": types.ScopeUser,
		}),
		discoveredItem(map[string]any{
			"id": "mcp:docs", "agent": types.AgentCodex, "kind": types.KindMcpServer,
			"name": "docs", "sourcePath": "~/.codex/config.toml", "scope": types.ScopeUser,
		}),
		discoveredItem(map[string]any{
			"id": "project:env", "agent": types.AgentProject, "kind": types.KindEnvKey,
			"name": "OPENAI_API_KEY", "sourcePath": ".env", "scope": types.ScopeProject,
		}),
	}
	model := tui.BuildSetupInventoryViewModel(tui.BuildSetupInventoryViewModelInput{
		Inventory: setup.BuildInventory(evidence),
	})

	if len(model.Rows) != 2 {
		t.Fatalf("rows = %#v", model.Rows)
	}
	if model.Rows[0].AgentMarker == "" || model.Rows[0].AgentLabel == "" {
		t.Fatalf("missing agent identity: %#v", model.Rows[0])
	}
	for _, row := range model.Rows {
		if row.SourcePath == ".env" {
			t.Fatalf("project row included: %#v", row)
		}
	}
	if model.Skills != 1 || model.McpServers != 1 {
		t.Fatalf("counts = %#v", model)
	}
}

func TestSetupConsoleViewModelFiltersTabsAndBuildsDetail(t *testing.T) {
	evidence := []types.DiscoveredItem{
		discoveredItem(map[string]any{
			"id": "hook:claude", "agent": types.AgentClaudeCode, "kind": types.KindHook,
			"name": "PostToolUse.Write", "sourcePath": "~/.claude/settings.json", "scope": types.ScopeUser,
		}),
		discoveredItem(map[string]any{
			"id": "hook:codex", "agent": types.AgentCodex, "kind": types.KindHook,
			"name": "SessionStart", "sourcePath": "~/.codex/hooks.json", "scope": types.ScopeUser,
		}),
		discoveredItem(map[string]any{
			"id": "skill:review", "agent": types.AgentClaudeCode, "kind": types.KindSkill,
			"name": "review", "sourcePath": "~/.claude/skills/review", "scope": types.ScopeUser,
		}),
		discoveredItem(map[string]any{
			"id": "plugin:pi", "agent": types.AgentPiAgent, "kind": types.KindExtension,
			"name": "cmux-session", "sourcePath": "~/.pi/agent/extensions/cmux-session.ts", "scope": types.ScopeUser,
		}),
		discoveredItem(map[string]any{
			"id": "mcp:docs", "agent": types.AgentCodex, "kind": types.KindMcpServer,
			"name": "docs", "sourcePath": "~/.codex/config.toml", "scope": types.ScopeUser,
		}),
		discoveredItem(map[string]any{
			"id": "project:skill", "agent": types.AgentCodex, "kind": types.KindSkill,
			"name": "project-only", "sourcePath": ".codex/skills/project-only", "scope": types.ScopeProject,
		}),
	}

	model := tui.BuildSetupConsoleViewModel(tui.BuildSetupConsoleViewModelInput{
		Inventory:     setup.BuildInventory(evidence),
		ActiveTab:     tui.SetupConsoleTabHooks,
		Search:        "session",
		SelectedIndex: 3,
	})

	if model.ActiveTab != tui.SetupConsoleTabHooks {
		t.Fatalf("active tab = %q", model.ActiveTab)
	}
	if len(model.Rows) != 1 || model.Rows[0].Name != "SessionStart" {
		t.Fatalf("rows = %#v", model.Rows)
	}
	if !model.Rows[0].Selected {
		t.Fatalf("selected row not marked: %#v", model.Rows[0])
	}
	if model.Rows[0].AgentMarker != "CX" {
		t.Fatalf("agent marker = %q", model.Rows[0].AgentMarker)
	}
	if model.Selected == nil {
		t.Fatal("selected detail missing")
	}
	if model.Selected.SourcePath != "~/.codex/hooks.json" || model.Selected.Scope != "user" {
		t.Fatalf("selected detail = %#v", model.Selected)
	}
	if len(model.Selected.Actions) == 0 || model.Selected.Actions[0].Available {
		t.Fatalf("expected unavailable action detail: %#v", model.Selected.Actions)
	}

	counts := map[tui.SetupConsoleTab]int{}
	for _, tab := range model.Tabs {
		counts[tab.Tab] = tab.Count
	}
	if counts[tui.SetupConsoleTabHooks] != 2 ||
		counts[tui.SetupConsoleTabSkills] != 1 ||
		counts[tui.SetupConsoleTabPlugins] != 1 ||
		counts[tui.SetupConsoleTabMCPServers] != 1 ||
		counts[tui.SetupConsoleTabMarketplace] != 0 {
		t.Fatalf("tab counts = %#v", counts)
	}
}

func TestSetupConsoleViewModelShowsMarketplaceSources(t *testing.T) {
	name := "codex"
	evidence := []types.DiscoveredItem{
		discoveredItem(map[string]any{
			"id": "plugin-skill", "agent": types.AgentClaudeCode, "kind": types.KindSkill,
			"name": "codex", "sourcePath": "~/.claude/plugins/cache/openai-codex/codex/1.0.2/skills/codex", "scope": types.ScopeUser,
			"metadata": json.RawMessage(`{
				"source": "plugin",
				"sourceRoot": "~/.claude/plugins/marketplaces/openai-codex",
				"description": "Use Codex from Claude Code",
				"author": "OpenAI",
				"version": "1.0.5",
				"provides": ["skills", "hooks"]
			}`),
		}),
		discoveredItem(map[string]any{
			"id": "project-skill", "agent": types.AgentCodex, "kind": types.KindSkill,
			"name": name, "sourcePath": ".codex/skills/codex", "scope": types.ScopeProject,
			"metadata": json.RawMessage(`{"source":"plugin"}`),
		}),
	}

	sources := setup.BuildMarketplace(evidence)
	model := tui.BuildSetupConsoleViewModel(tui.BuildSetupConsoleViewModelInput{
		Inventory:          setup.BuildInventory(evidence),
		MarketplaceSources: sources,
		ActiveTab:          tui.SetupConsoleTabMarketplace,
		SelectedIndex:      1,
	})

	if len(model.Rows) != 1 {
		t.Fatalf("rows = %#v", model.Rows)
	}
	if model.Rows[0].ObjectKind != "marketplace" || model.Rows[0].RowKind != tui.SetupConsoleRowMarketplaceSource || model.Rows[0].Expanded {
		t.Fatalf("marketplace rows = %#v", model.Rows)
	}
	if model.Tabs[2].Count != 1 {
		t.Fatalf("marketplace tab count = %#v", model.Tabs)
	}
	if model.Selected == nil || model.Selected.Title != "openai-codex" {
		t.Fatalf("selected marketplace source detail = %#v", model.Selected)
	}

	expanded := tui.BuildSetupConsoleViewModel(tui.BuildSetupConsoleViewModelInput{
		Inventory:          setup.BuildInventory(evidence),
		MarketplaceSources: sources,
		ActiveTab:          tui.SetupConsoleTabMarketplace,
		SelectedIndex:      1,
		ExpandedSources:    map[string]bool{sources[0].ID: true},
	})
	if len(expanded.Rows) != 2 {
		t.Fatalf("expanded rows = %#v", expanded.Rows)
	}
	if expanded.Rows[0].RowKind != tui.SetupConsoleRowMarketplaceSource || !expanded.Rows[0].Expanded || expanded.Rows[1].RowKind != tui.SetupConsoleRowMarketplaceEntry {
		t.Fatalf("expanded marketplace rows = %#v", expanded.Rows)
	}
	if model.Selected == nil {
		t.Fatal("selected marketplace detail missing")
	}
	if expanded.Selected == nil {
		t.Fatal("selected marketplace entry detail missing")
	}
	if expanded.Selected.Description != "Use Codex from Claude Code" || expanded.Selected.Author != "OpenAI" || expanded.Selected.Version != "1.0.5" {
		t.Fatalf("selected metadata = %#v", expanded.Selected)
	}
	if len(expanded.Selected.Provides) != 2 || expanded.Selected.Provides[0] != "skills" {
		t.Fatalf("provides = %#v", expanded.Selected.Provides)
	}
	if len(expanded.Selected.Actions) == 0 || expanded.Selected.Actions[0].Label != "review" || !expanded.Selected.Actions[0].Available {
		t.Fatalf("marketplace review action should be available: %#v", expanded.Selected.Actions)
	}
	if expanded.Selected.Actions[1].Available {
		t.Fatalf("mutating marketplace actions should be unavailable: %#v", expanded.Selected.Actions)
	}

	filtered := tui.BuildSetupConsoleViewModel(tui.BuildSetupConsoleViewModelInput{
		Inventory:          setup.BuildInventory(evidence),
		MarketplaceSources: sources,
		ActiveTab:          tui.SetupConsoleTabMarketplace,
		Search:             "openai-codex",
	})
	if len(filtered.Rows) != 2 {
		t.Fatalf("source search should keep source and entries: %#v", filtered.Rows)
	}
}

func TestSetupConsoleRendersMarketplaceReviewResultAsNonMutating(t *testing.T) {
	rendered := views.RenderSetupConsole(views.SetupConsoleView{
		ActiveTab: "marketplace",
		Tabs: []views.SetupConsoleTab{
			{Label: "Marketplace", Count: 1, Selected: true},
		},
		Rows: []views.SetupConsoleRow{{
			RowKind:       "marketplace_entry",
			Name:          "codex",
			Status:        "available",
			ToggleControl: true,
			Selected:      true,
		}},
		MarketplaceReview: &views.SetupMarketplaceReview{
			Title:          "Marketplace Review Action",
			Status:         "reviewed guidance",
			AgentLabel:     "Claude Code",
			SourceLabel:    "openai-codex",
			TargetName:     "codex",
			Operation:      "review marketplace setup guidance",
			ExpectedEffect: "non-mutating setup guidance only",
			Instructions:   "Description: red plugin\nEffect: no files changed",
		},
	}, 100, 32)

	if !strings.Contains(rendered, "Marketplace Review Action") || !strings.Contains(rendered, "No files changed") {
		t.Fatalf("review result not rendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "Applied setup action") {
		t.Fatalf("review result should not claim apply:\n%s", rendered)
	}
	if strings.Contains(rendered, "[31m") {
		t.Fatalf("source terminal control leaked into render:\n%s", rendered)
	}
}

func TestSetupConsoleViewModelClassifiesPiExtensionInPluginsAndExcludesMarketplace(t *testing.T) {
	name := "cmux-session"
	evidence := []types.DiscoveredItem{
		discoveredItem(map[string]any{
			"id": "pi-extension", "agent": types.AgentPiAgent, "kind": types.KindExtension,
			"name": name, "sourcePath": "~/.pi/agent/extensions/cmux-session.ts", "scope": types.ScopeUser,
			"metadata": json.RawMessage(`{"source":"settings"}`),
		}),
	}

	plugins := tui.BuildSetupConsoleViewModel(tui.BuildSetupConsoleViewModelInput{
		Inventory: setup.BuildInventory(evidence),
		ActiveTab: tui.SetupConsoleTabPlugins,
	})
	if len(plugins.Rows) != 1 || plugins.Rows[0].ObjectKind != "extension" {
		t.Fatalf("plugin rows = %#v", plugins.Rows)
	}

	marketplace := tui.BuildSetupConsoleViewModel(tui.BuildSetupConsoleViewModelInput{
		Inventory:          setup.BuildInventory(evidence),
		MarketplaceSources: setup.BuildMarketplace(evidence),
		ActiveTab:          tui.SetupConsoleTabMarketplace,
		SelectedIndex:      1,
	})

	if len(marketplace.Rows) != 0 {
		t.Fatalf("rows = %#v", marketplace.Rows)
	}
	if marketplace.Tabs[2].Count != 0 {
		t.Fatalf("tab count = %#v", marketplace.Tabs)
	}
	if marketplace.Selected != nil {
		t.Fatalf("selected detail = %#v", marketplace.Selected)
	}
}

func TestTimelineCorruptWarning(t *testing.T) {
	model := tui.BuildTimelineViewModel(tui.BuildTimelineViewModelInput{
		Entries: []types.TimelineEntry{timelineEntry(nil)},
		CorruptEvents: []store.TimelineCorruptEvent{{
			FilePath: "/store/timeline/events/bad.json",
			Error:    "Unexpected token",
		}},
	})
	if model.CorruptWarning != "1 corrupt timeline event skipped" {
		t.Fatalf("corrupt warning: got %q", model.CorruptWarning)
	}
	if len(model.Rows) != 1 {
		t.Fatalf("rows length: got %d", len(model.Rows))
	}
}

func TestCompareScopeAndLabels(t *testing.T) {
	before := snapshotForTui("baseline", "2026-06-07T00:00:00.000Z", []types.GraphNode{
		graphNode(map[string]any{"id": "mcp-linear-before", "entityKind": types.KindMcpServer, "entityName": "linear", "effectiveValue": json.RawMessage(`{"command":"linear-old"}`)}),
		graphNode(map[string]any{"id": "hook-pre-before", "entityKind": types.KindHook, "entityName": "pre-tool-use", "effectiveValue": json.RawMessage(`{"command":"notify"}`)}),
	})
	after := snapshotForTui("current", "2026-06-08T00:00:00.000Z", []types.GraphNode{
		graphNode(map[string]any{"id": "mcp-linear-after", "entityKind": types.KindMcpServer, "entityName": "linear", "effectiveValue": json.RawMessage(`{"command":"linear-new"}`)}),
		graphNode(map[string]any{"id": "skill-review-after", "entityKind": types.KindSkill, "entityName": "react-review", "effectiveValue": json.RawMessage(`{"installed":true}`)}),
	})

	model := tui.BuildCompareViewModel(tui.BuildCompareViewModelInput{
		FromSnapshot: before,
		ToSnapshot:   after,
		ToLabel:      "Current  unsaved changes",
		Scope:        "Full setup",
		Diff: diff.GraphDiff{
			SemanticChanges: []diff.SemanticChange{{
				Code:       diff.SemanticSkillAdded,
				EntityKind: types.KindSkill,
				EntityName: "react-review",
				Severity:   types.SeverityLow,
			}},
		},
	})

	if !strings.HasPrefix(model.FromLabel, "baseline") {
		t.Fatalf("from label: got %q", model.FromLabel)
	}
	if model.ToLabel != "Current  unsaved changes" {
		t.Fatalf("to label: got %q", model.ToLabel)
	}
	if model.ScopeLabel != "Full setup" {
		t.Fatalf("scope label: got %q", model.ScopeLabel)
	}
	if len(model.Summary) != 1 || model.Summary[0] != "+ Skill: react-review" {
		t.Fatalf("summary: got %#v", model.Summary)
	}
	if len(model.Sections) == 0 || model.Sections[0].Title != "Claude Code" {
		t.Fatalf("sections: got %#v", model.Sections)
	}
}

func TestEnvironmentsViewModelBuildsStructuredDiffSurface(t *testing.T) {
	sourcePath := "~/.codex/config.toml"
	model := tui.BuildEnvironmentsViewModel(tui.BuildEnvironmentsViewModelInput{
		Status: baseline.Status{Agents: []baseline.AgentStatus{{
			Agent:               types.AgentCodex,
			HasBaseline:         true,
			BaselineName:        "baseline-codex",
			SemanticChangeCount: 1,
			Diff: diff.GraphDiff{SemanticChanges: []diff.SemanticChange{{
				Code:       diff.SemanticMcpChanged,
				EntityKind: types.KindMcpServer,
				EntityName: "aside",
				Severity:   types.SeverityMedium,
				Before:     json.RawMessage(`{"args":["--old"],"command":"old-aside","env":{"A":"1"},"removed":true}`),
				After:      json.RawMessage(`{"args":["--new"],"command":"new-aside","env":{"A":"1"},"timeout":30}`),
				Details: diff.SemanticChangeDetails{
					ChangedFields: []string{"command", "args", "removed", "timeout"},
					SourcePath:    &sourcePath,
				},
			}}},
		}}},
	})

	if len(model.Surfaces) != 1 {
		t.Fatalf("surfaces = %#v", model.Surfaces)
	}
	surface := model.Surfaces[0]
	if surface.Marker != "~" || surface.Kind != "MCP" || surface.Name != "aside" || !surface.Selected {
		t.Fatalf("surface = %#v", surface)
	}
	if surface.SourcePath != sourcePath {
		t.Fatalf("source path = %q", surface.SourcePath)
	}
	if model.Diff.Title != "MCP aside" || model.Diff.SourcePath != sourcePath {
		t.Fatalf("diff = %#v", model.Diff)
	}
	if !hasEnvironmentHunk(model.Diff.Rows) {
		t.Fatalf("expected hunk row: %#v", model.Diff.Rows)
	}
	if !hasEnvironmentPair(model.Diff.Rows, `command: "old-aside"`, `command: "new-aside"`) {
		t.Fatalf("expected command pair: %#v", model.Diff.Rows)
	}
	if !hasEnvironmentPair(model.Diff.Rows, `args: ["--old"]`, `args: ["--new"]`) {
		t.Fatalf("expected args pair: %#v", model.Diff.Rows)
	}
	if !hasEnvironmentLeftOnly(model.Diff.Rows, `removed: true`) {
		t.Fatalf("expected removed left-only row: %#v", model.Diff.Rows)
	}
	if !hasEnvironmentRightOnly(model.Diff.Rows, `timeout: 30`) {
		t.Fatalf("expected added right-only row: %#v", model.Diff.Rows)
	}
	if !hasEnvironmentContext(model.Diff.Rows, `env: {"A":"1"}`) {
		t.Fatalf("expected unchanged context row near hunk: %#v", model.Diff.Rows)
	}
}

func TestEnvironmentsViewModelDoesNotHideLargeDiffBehindSummary(t *testing.T) {
	before := map[string]any{}
	after := map[string]any{}
	changedFields := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		field := fmt.Sprintf("field%02d", i)
		before[field] = i
		after[field] = i + 100
		changedFields = append(changedFields, field)
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)

	model := tui.BuildEnvironmentsViewModel(tui.BuildEnvironmentsViewModelInput{
		Status: baseline.Status{Agents: []baseline.AgentStatus{{
			Agent: types.AgentCodex, HasBaseline: true, SemanticChangeCount: 1,
			Diff: diff.GraphDiff{SemanticChanges: []diff.SemanticChange{{
				Code: diff.SemanticAgentConfigChanged, EntityKind: types.KindAgentConfig, EntityName: "config",
				Before: beforeJSON, After: afterJSON,
				Details: diff.SemanticChangeDetails{ChangedFields: changedFields},
			}}},
		}}},
	})

	if len(model.Diff.Rows) == 0 {
		t.Fatal("expected diff rows")
	}
	for _, row := range model.Diff.Rows {
		if strings.Contains(row.Left.Text, "... +") || strings.Contains(row.Right.Text, "... +") || strings.Contains(row.HunkTitle, "... +") {
			t.Fatalf("diff rows must not hide changes behind summary: %#v", row)
		}
	}
	if !hasEnvironmentPair(model.Diff.Rows, "field11: 11", "field11: 111") {
		t.Fatalf("expected later changed field to remain visible: %#v", model.Diff.Rows)
	}
}

func TestEnvironmentsViewModelShowsRawOnlyChanges(t *testing.T) {
	beforeID := "codex.config.old"
	afterID := "codex.config.new"
	beforeChecksum := "sha256:old"
	afterChecksum := "sha256:new"
	model := tui.BuildEnvironmentsViewModel(tui.BuildEnvironmentsViewModelInput{
		Status: baseline.Status{Agents: []baseline.AgentStatus{{
			Agent:          types.AgentCodex,
			HasBaseline:    true,
			RawChangeCount: 1,
			Diff: diff.GraphDiff{RawSourceChanges: []diff.RawSourceChange{{
				SourcePath:       "~/.codex/config.toml",
				BeforeEvidenceID: &beforeID,
				AfterEvidenceID:  &afterID,
				BeforeChecksum:   &beforeChecksum,
				AfterChecksum:    &afterChecksum,
				Status:           "changed",
			}}},
		}}},
	})

	if model.ChangesEmpty != "" {
		t.Fatalf("raw-only diff should not be reported clean: %q", model.ChangesEmpty)
	}
	if len(model.Surfaces) != 1 {
		t.Fatalf("surfaces = %#v", model.Surfaces)
	}
	surface := model.Surfaces[0]
	if surface.Kind != "Source" || surface.Marker != "~" || surface.Name != "~/.codex/config.toml" {
		t.Fatalf("surface = %#v", surface)
	}
	if !hasEnvironmentPair(model.Diff.Rows, "checksum: sha256:old", "checksum: sha256:new") {
		t.Fatalf("expected raw checksum diff: %#v", model.Diff.Rows)
	}
	if !hasEnvironmentPair(model.Diff.Rows, "status: baseline", "status: current") {
		t.Fatalf("expected raw status diff: %#v", model.Diff.Rows)
	}
}

func hasEnvironmentHunk(rows []tui.EnvironmentDiffRowModel) bool {
	for _, row := range rows {
		if row.Kind == tui.EnvironmentDiffRowHunk && strings.HasPrefix(row.HunkTitle, "@@") {
			return true
		}
	}
	return false
}

func hasEnvironmentPair(rows []tui.EnvironmentDiffRowModel, left, right string) bool {
	for _, row := range rows {
		if row.Left.Text == left && row.Right.Text == right {
			return true
		}
	}
	return false
}

func hasEnvironmentLeftOnly(rows []tui.EnvironmentDiffRowModel, left string) bool {
	for _, row := range rows {
		if row.Left.Text == left && row.Right.Text == "" {
			return true
		}
	}
	return false
}

func hasEnvironmentRightOnly(rows []tui.EnvironmentDiffRowModel, right string) bool {
	for _, row := range rows {
		if row.Left.Text == "" && row.Right.Text == right {
			return true
		}
	}
	return false
}

func hasEnvironmentContext(rows []tui.EnvironmentDiffRowModel, text string) bool {
	for _, row := range rows {
		if row.Kind == tui.EnvironmentDiffRowContext && row.Left.Text == text && row.Right.Text == text {
			return true
		}
	}
	return false
}

func TestSaveSetupTitlePreview(t *testing.T) {
	baseline := tui.BuildSaveSetupViewModel(tui.BuildSaveSetupViewModelInput{HasPreviousSnapshot: false})
	if baseline.Title != "capture baseline" {
		t.Fatalf("baseline title: got %q", baseline.Title)
	}

	changed := tui.BuildSaveSetupViewModel(tui.BuildSaveSetupViewModelInput{
		HasPreviousSnapshot: true,
		Diff: &diff.GraphDiff{
			SemanticChanges: []diff.SemanticChange{{
				Code:       diff.SemanticSkillAdded,
				EntityKind: types.KindSkill,
				EntityName: "react-review",
				Severity:   types.SeverityLow,
			}},
		},
	})
	if changed.Title != "install react-review skill" {
		t.Fatalf("changed title: got %q", changed.Title)
	}
	if changed.DetectedChanges[0] != "install react-review skill" {
		t.Fatalf("detected changes: got %#v", changed.DetectedChanges)
	}

	unchanged := tui.BuildSaveSetupViewModel(tui.BuildSaveSetupViewModelInput{
		HasPreviousSnapshot: true,
		Diff:                &diff.GraphDiff{},
	})
	if !unchanged.NoChanges || unchanged.Title != "current setup unchanged" {
		t.Fatalf("unchanged model: %#v", unchanged)
	}
}

func TestUndoPreviewDryRunOnly(t *testing.T) {
	plan := timelineundo.Plan{
		EntryID:     "changed-entry",
		Title:       "dry-run MCP undo: MCP server changed",
		DryRun:      true,
		WritesFiles: false,
		WritableItems: []timelineundo.Item{{
			Action:     timelineundo.ActionUpdate,
			Path:       "/project/.mcp.json",
			ServerName: "github",
		}},
		ObserveOnlySurfaces: []types.TimelineChangedSurface{{
			Kind:        "skill",
			ChangeType:  "SKILL_ADDED",
			Path:        "/home/.claude/skills/review/SKILL.md",
			EntityName:  strPtr("review"),
			Restorable:  false,
			ObserveOnly: true,
		}},
	}
	preview := tui.BuildTimelineUndoPreview(plan)
	if preview.WritesFiles != "no" {
		t.Fatalf("writes files: got %q", preview.WritesFiles)
	}
	if len(preview.WritableItems) != 1 || preview.WritableItems[0].Action != "update" {
		t.Fatalf("writable items: got %#v", preview.WritableItems)
	}
	if len(preview.ObserveOnlySurfaces) != 1 {
		t.Fatalf("observe-only: got %#v", preview.ObserveOnlySurfaces)
	}
}

func TestNavigationAgentsExcludeProject(t *testing.T) {
	model := tui.BuildNavigationModel(tui.BuildNavigationModelInput{
		Evidence: []types.DiscoveredItem{
			discoveredItem(map[string]any{"id": "skill", "agent": types.AgentClaudeCode, "kind": types.KindSkill}),
			discoveredItem(map[string]any{"id": "env", "agent": types.AgentProject, "kind": types.KindEnvKey}),
		},
	})
	var agentsSection tui.NavSection
	for _, section := range model.Sections {
		if section.Label == "Agents" {
			agentsSection = section
			break
		}
	}
	if len(agentsSection.Items) != 1 || agentsSection.Items[0].Label != "Claude Code" {
		t.Fatalf("agents section: got %#v", agentsSection.Items)
	}
}

func TestNavigationDefaultsToInventory(t *testing.T) {
	model := tui.BuildNavigationModel(tui.BuildNavigationModelInput{})
	if model.SelectedItemID != "inventory:global" {
		t.Fatalf("selected item = %q", model.SelectedItemID)
	}
	if len(model.Sections) == 0 || model.Sections[0].Label != "Inventory" {
		t.Fatalf("sections = %#v", model.Sections)
	}
}

func TestNavigationSelectionIDsSplitInventoryAndHistory(t *testing.T) {
	if got := tui.NavItemIDForSelection(tui.NavigationSelection{Screen: tui.ScreenInventory}); got != tui.InventoryNavItemID {
		t.Fatalf("inventory id = %q", got)
	}
	if got := tui.NavItemIDForSelection(tui.NavigationSelection{Screen: tui.ScreenTimeline}); got != tui.HistoryAllNavItemID {
		t.Fatalf("history id = %q", got)
	}
	if got := tui.NavItemIDForSelection(tui.NavigationSelection{Screen: tui.ScreenSnapshots}); got != "history:snapshots" {
		t.Fatalf("snapshots id = %q", got)
	}

	agent := types.AgentCodex
	if got := tui.NavItemIDForSelection(tui.NavigationSelection{Screen: tui.ScreenTimeline, SelectedAgent: &agent}); got != "agent:codex" {
		t.Fatalf("agent timeline id = %q", got)
	}
	if got := tui.NavItemIDForSelection(tui.NavigationSelection{Screen: tui.ScreenProfile}); got != "profile:default" {
		t.Fatalf("profile id = %q", got)
	}
}

func TestSelectNavItemRoutesInventoryAndHistoryScreens(t *testing.T) {
	inventory := tui.NavItem{ID: tui.InventoryNavItemID, Kind: tui.NavHistoryItem, Screen: tui.ScreenInventory}
	if selection := tui.SelectNavItem(inventory, tui.ScreenTimeline, nil, ""); selection.Screen != tui.ScreenInventory {
		t.Fatalf("inventory selection = %#v", selection)
	}

	history := tui.NavItem{ID: tui.HistoryAllNavItemID, Kind: tui.NavHistoryItem, Screen: tui.ScreenTimeline}
	if selection := tui.SelectNavItem(history, tui.ScreenInventory, nil, ""); selection.Screen != tui.ScreenTimeline {
		t.Fatalf("history selection = %#v", selection)
	}
}
