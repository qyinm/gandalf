package tui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/qyinm/gandalf/internal/gandalfcore/baseline"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestNewAppOpensChangesFirstHomeAndKeepsSetupReachable(t *testing.T) {
	app := NewApp(makeTestRuntime(t))
	if app.screen != ScreenHome {
		t.Fatalf("initial screen = %q", app.screen)
	}
	app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if app.screen != ScreenInventory {
		t.Fatalf("setup screen = %q", app.screen)
	}
}

func TestHomeViewMakesDriftAndSafeActionsVisible(t *testing.T) {
	app := NewApp(makeTestRuntime(t))
	app.now = func() time.Time {
		return time.Date(2026, time.July, 12, 12, 0, 0, 0, time.Local)
	}
	app.ready = true
	app.width = 100
	app.height = 24
	app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{{
		Agent: types.AgentCodex, HasBaseline: true, BaselineCreatedAt: "2026-07-12T00:00:00Z",
		SemanticChangeCount: 1,
		Diff: diff.GraphDiff{SemanticChanges: []diff.SemanticChange{{
			Code: diff.SemanticSkillAdded, EntityKind: types.KindSkill, EntityName: "review",
		}}},
	}}}
	view := app.View()
	for _, want := range []string{"1 setup object changed", "since Today", "skills 1", "+ review", "[v] review", "[R] rollback", "[i] setup"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

func TestHomeAppViewNeverExceedsActualTerminalWidth(t *testing.T) {
	app := NewApp(makeTestRuntime(t))
	app.ready = true
	app.width = 24
	app.height = 16
	app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{{
		Agent: types.AgentCodex, HasBaseline: true, SemanticChangeCount: 1,
		Diff: diff.GraphDiff{SemanticChanges: []diff.SemanticChange{{
			Code: diff.SemanticSkillAdded, EntityKind: types.KindSkill, EntityName: "a-very-long-skill-name",
		}}},
	}}}
	for _, line := range strings.Split(app.View(), "\n") {
		if got := ansi.StringWidth(line); got > app.width {
			t.Fatalf("line width = %d, terminal width = %d: %q", got, app.width, line)
		}
	}
}

func TestHomeBaselineShortcutStartsBaselineCreation(t *testing.T) {
	app := NewApp(makeTestRuntime(t))
	app.ready = true
	cmd, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")})
	if quit || cmd == nil {
		t.Fatalf("Home B shortcut: cmd=%v quit=%v", cmd != nil, quit)
	}
}

func TestHomePartialBaselineCoverageShowsMissingBaselineCTA(t *testing.T) {
	for _, width := range []int{100, 36} {
		app := NewApp(makeTestRuntime(t))
		app.ready = true
		app.width = width
		app.height = 24
		app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{
			{Agent: types.AgentClaudeCode},
			{Agent: types.AgentCodex, HasBaseline: true, BaselineCreatedAt: "2026-07-12T00:00:00Z"},
		}}

		view := app.View()
		if !strings.Contains(view, "Some agents have no baseline.") || !strings.Contains(view, "[B] capture missing baselines") {
			t.Fatalf("width %d partial baseline home must offer the one-action CTA:\n%s", width, view)
		}
	}
}

func TestHomeReviewFocusesChangedEnvironmentWithoutMutation(t *testing.T) {
	app := NewApp(makeTestRuntime(t))
	app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{
		{Agent: types.AgentClaudeCode, HasBaseline: true},
		{Agent: types.AgentCodex, HasBaseline: true, SemanticChangeCount: 1},
	}}
	cmd, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if quit || cmd != nil || app.screen != ScreenEnvironments || app.environments.agentCursor != 1 {
		t.Fatalf("review route: screen=%q cursor=%d cmd=%v quit=%v", app.screen, app.environments.agentCursor, cmd != nil, quit)
	}
	if app.rollbackReview != nil {
		t.Fatal("review navigation must not prepare or apply rollback")
	}
}

func TestHomeReviewPrefersSemanticChangeOverEarlierRawOnlyDrift(t *testing.T) {
	app := NewApp(makeTestRuntime(t))
	app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{
		{Agent: types.AgentClaudeCode, HasBaseline: true, RawChangeCount: 3},
		{Agent: types.AgentCodex, HasBaseline: true, SemanticChangeCount: 1},
	}}
	app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if app.screen != ScreenEnvironments || app.environments.agentCursor != 1 {
		t.Fatalf("review route: screen=%q cursor=%d", app.screen, app.environments.agentCursor)
	}
}

func TestHomeRollbackStopsAtReviewChanges(t *testing.T) {
	app := NewApp(makeTestRuntime(t))
	app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{{
		Agent: types.AgentCodex, HasBaseline: true, SemanticChangeCount: 1,
	}}}
	app.snapshotRefs = []snapshotRef{{Name: "baseline-codex", Agent: types.AgentCodex}}
	cmd, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if quit || cmd == nil || app.screen != ScreenSnapshots {
		t.Fatalf("rollback route: screen=%q cmd=%v quit=%v", app.screen, cmd != nil, quit)
	}
	if app.rollbackReview != nil {
		t.Fatal("rollback must wait for the Review Changes command result")
	}
}

func TestSkillsEnterOpensMarkdownViewerUnavailableWhenNoEntrypointExists(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newInventoryTestApp(t, runtime)

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding skill row should not return a command")
	}
	if app.pendingAction != nil {
		t.Fatalf("pending action = %#v", app.pendingAction)
	}
	if app.skillViewer != nil {
		t.Fatalf("skill viewer = %#v", app.skillViewer)
	}
	if app.expandedSetupRowID(SetupConsoleTabSkills) != "skill-review" {
		t.Fatalf("expanded skill = %q", app.expandedSetupRowID(SetupConsoleTabSkills))
	}
	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("opening viewer should not return a command")
	}
	if app.actionError != "" {
		t.Fatalf("action error = %q", app.actionError)
	}
	if app.skillViewer == nil {
		t.Fatal("expected skill viewer")
	}
	if app.skillViewer.errorText == "" {
		t.Fatal("expected viewer error")
	}
}

func TestSkillsEnterOpensMarkdownViewerForEntrypoint(t *testing.T) {
	runtime := makeTestRuntime(t)
	skillDir := filepath.Join(runtime.HomeDir, ".codex", "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Review\n\nUse this skill."), 0o644); err != nil {
		t.Fatal(err)
	}
	app := newInventoryTestApp(t, runtime)
	app.evidence[0].Metadata = []byte(`{"entrypoint":"SKILL.md","entrypointStatus":"captured"}`)
	app.applyWorkspaceData(bootMsg{evidence: app.evidence})

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding skill row should not return a command")
	}
	if app.pendingAction != nil {
		t.Fatalf("pending action = %#v", app.pendingAction)
	}
	if app.skillViewer != nil {
		t.Fatalf("skill viewer = %#v", app.skillViewer)
	}
	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("opening viewer should not return a command")
	}
	if app.skillViewer == nil {
		t.Fatal("expected skill viewer")
	}
	if !strings.Contains(app.skillViewer.content, "# Review") {
		t.Fatalf("viewer content = %q", app.skillViewer.content)
	}
	if app.skillViewer.sourcePath != "~/.codex/skills/review/SKILL.md" {
		t.Fatalf("source path = %q", app.skillViewer.sourcePath)
	}
}

func TestSkillsEnterFollowsMarkdownEntrypointSymlink(t *testing.T) {
	runtime := makeTestRuntime(t)
	targetDir := filepath.Join(runtime.HomeDir, "gstack", "diagram")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("# Diagram\n\nRender diagrams."), 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(runtime.HomeDir, ".codex", "skills", "diagram")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(targetDir, "SKILL.md"), filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	app := newInventoryTestApp(t, runtime)
	name := "diagram"
	app.evidence[0].Name = &name
	app.evidence[0].SourcePath = "~/.codex/skills/diagram"
	app.evidence[0].Metadata = []byte(`{"entrypoint":"SKILL.md","entrypointStatus":"symlink_not_followed"}`)
	app.applyWorkspaceData(bootMsg{evidence: app.evidence})

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding skill row should not return a command")
	}
	if app.skillViewer != nil {
		t.Fatalf("skill viewer = %#v", app.skillViewer)
	}
	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("opening viewer should not return a command")
	}
	if app.skillViewer == nil {
		t.Fatal("expected skill viewer")
	}
	if app.skillViewer.errorText != "" {
		t.Fatalf("viewer error = %q", app.skillViewer.errorText)
	}
	if !strings.Contains(app.skillViewer.content, "# Diagram") {
		t.Fatalf("viewer content = %q", app.skillViewer.content)
	}
	if app.skillViewer.sourcePath != "~/.codex/skills/diagram/SKILL.md -> ~/gstack/diagram/SKILL.md" {
		t.Fatalf("source path = %q", app.skillViewer.sourcePath)
	}
}

func TestSkillsEnterRejectsSymlinkOutsideReadableRoots(t *testing.T) {
	runtime := makeTestRuntime(t)
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "SKILL.md"), []byte("# Outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(runtime.HomeDir, ".codex", "skills", "outside")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outsideDir, "SKILL.md"), filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	app := newInventoryTestApp(t, runtime)
	name := "outside"
	app.evidence[0].Name = &name
	app.evidence[0].SourcePath = "~/.codex/skills/outside"
	app.evidence[0].Metadata = []byte(`{"entrypoint":"SKILL.md","entrypointStatus":"symlink_not_followed"}`)
	app.applyWorkspaceData(bootMsg{evidence: app.evidence})

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding skill row should not return a command")
	}
	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("opening viewer should not return a command")
	}
	if app.skillViewer == nil {
		t.Fatal("expected skill viewer")
	}
	if !strings.Contains(app.skillViewer.errorText, "outside readable global setup roots") {
		t.Fatalf("viewer error = %q", app.skillViewer.errorText)
	}
}

func TestSkillsEnterRejectsSymlinkedSkillDirectoryOutsideReadableRoots(t *testing.T) {
	runtime := makeTestRuntime(t)
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "SKILL.md"), []byte("# Outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	parentDir := filepath.Join(runtime.HomeDir, ".codex", "skills")
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(parentDir, "outside-dir")); err != nil {
		t.Fatal(err)
	}
	app := newInventoryTestApp(t, runtime)
	name := "outside-dir"
	app.evidence[0].Name = &name
	app.evidence[0].SourcePath = "~/.codex/skills/outside-dir"
	app.evidence[0].Metadata = []byte(`{"entrypoint":"SKILL.md","entrypointStatus":"captured"}`)
	app.applyWorkspaceData(bootMsg{evidence: app.evidence})

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding skill row should not return a command")
	}
	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("opening viewer should not return a command")
	}
	if app.skillViewer == nil {
		t.Fatal("expected skill viewer")
	}
	if !strings.Contains(app.skillViewer.errorText, "outside readable global setup roots") {
		t.Fatalf("viewer error = %q", app.skillViewer.errorText)
	}
}

func TestMarketplaceEnterReportsUnavailableProvider(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newInventoryTestApp(t, runtime)
	app.activeSetupTab = SetupConsoleTabMarketplace

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("marketplace action should not return a command")
	}
	if !strings.Contains(app.actionError, "marketplace source") {
		t.Fatalf("action error = %q", app.actionError)
	}
}

func TestMarketplaceEnterTogglesSourceAndChildOpensReviewAction(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := NewApp(runtime)
	app.ready = true
	app.activeSetupTab = SetupConsoleTabMarketplace
	name := "codex"
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{{
		ID:         "marketplace-plugin",
		Agent:      types.AgentClaudeCode,
		Kind:       types.KindSkill,
		Name:       &name,
		SourcePath: "~/.claude/plugins/marketplaces/openai-codex/codex/1.0.2/skills/codex",
		Scope:      types.ScopeUser,
		Metadata:   []byte(`{"source":"plugin","sourceRoot":"~/.claude/plugins/marketplaces/openai-codex"}`),
	}}})

	model := app.currentSetupConsoleViewModel()
	if len(model.Rows) != 1 || model.Rows[0].RowKind != SetupConsoleRowMarketplaceSource {
		t.Fatalf("collapsed marketplace rows = %#v", model.Rows)
	}

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("source toggle should not return a command")
	}
	model = app.currentSetupConsoleViewModel()
	if len(model.Rows) != 2 || !model.Rows[0].Expanded || model.Rows[1].RowKind != SetupConsoleRowMarketplaceEntry {
		t.Fatalf("expanded marketplace rows = %#v", model.Rows)
	}

	app.moveInventoryCursor(1)
	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("opening marketplace review should not return a command")
	}
	if app.actionError != "" {
		t.Fatalf("action error = %q", app.actionError)
	}
	if app.pendingMarketplaceReview == nil {
		t.Fatal("expected pending marketplace review")
	}
	if !strings.Contains(app.pendingMarketplaceReview.Instructions, "non-mutating") {
		t.Fatalf("pending marketplace review = %#v", app.pendingMarketplaceReview)
	}

	result, err := setup.ExecuteMarketplaceReviewPlan(*app.pendingMarketplaceReview, setup.BuildMarketplace(app.evidence))
	if err != nil {
		t.Fatal(err)
	}
	teaModel, _ := app.Update(marketplaceReviewMsg{data: bootMsg{evidence: app.evidence}, result: &result})
	updated := teaModel.(*App)
	if updated.pendingMarketplaceReview != nil {
		t.Fatalf("pending marketplace review was not cleared: %#v", updated.pendingMarketplaceReview)
	}
	if updated.marketplaceReviewResult == nil {
		t.Fatal("expected marketplace review result")
	}
	if strings.Contains(updated.notice, "Applied setup action") {
		t.Fatalf("notice should not claim an apply: %q", updated.notice)
	}

	app = updated
	app.moveInventoryCursor(-1)
	if cmd := app.handleSetupToggle(); cmd != nil {
		t.Fatal("space toggle should not return a command")
	}
	model = app.currentSetupConsoleViewModel()
	if len(model.Rows) != 1 || model.Rows[0].Expanded {
		t.Fatalf("collapsed marketplace rows after toggle = %#v", model.Rows)
	}
}

func TestMarketplaceSearchClampsCursorAgainstMarketplaceRows(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := NewApp(runtime)
	app.ready = true
	app.activeSetupTab = SetupConsoleTabMarketplace
	name := "codex"
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{{
		ID:         "marketplace-plugin",
		Agent:      types.AgentClaudeCode,
		Kind:       types.KindSkill,
		Name:       &name,
		SourcePath: "~/.claude/plugins/marketplaces/openai-codex/codex/1.0.2/skills/codex",
		Scope:      types.ScopeUser,
		Metadata:   []byte(`{"source":"plugin","sourceRoot":"~/.claude/plugins/marketplaces/openai-codex"}`),
	}}})

	app.handleInventoryEnter()
	app.moveInventoryCursor(1)
	if app.activeSetupTabState().cursor != 1 {
		t.Fatalf("cursor before search = %d", app.activeSetupTabState().cursor)
	}

	app.setupSearchFocused = true
	app.activeSetupTabState().searchInput.Focus()
	if _, handled := app.handleSetupSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("codex")}); !handled {
		t.Fatal("search key should be handled")
	}
	if app.activeSetupTabState().cursor != 1 {
		t.Fatalf("marketplace cursor should clamp against marketplace rows, got %d", app.activeSetupTabState().cursor)
	}
}

func TestMCPEnterExpandsServerToolsAndToolDescription(t *testing.T) {
	runtime := makeTestRuntime(t)
	name := "posthog"
	app := NewApp(runtime)
	app.ready = true
	app.activeSetupTab = SetupConsoleTabMCPServers
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{{
		ID:         "mcp-posthog",
		Agent:      types.AgentCursor,
		Kind:       types.KindMcpServer,
		Name:       &name,
		SourcePath: "~/.cursor/mcp.json",
		Scope:      types.ScopeUser,
		Metadata: []byte(`{
			"runtimeStatus": "ready",
			"toolCount": 1,
			"tools": [{"name":"dashboard-get","description":"Fetch a dashboard."}]
		}`),
	}}})

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding MCP server should not return a command")
	}
	model := app.currentSetupConsoleViewModel()
	if len(model.Rows) != 2 || !model.Rows[0].Expanded || model.Rows[1].RowKind != SetupConsoleRowMCPTool {
		t.Fatalf("mcp rows = %#v", model.Rows)
	}
	app.moveInventoryCursor(1)
	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding MCP tool should not return a command")
	}
	model = app.currentSetupConsoleViewModel()
	if len(model.Rows) != 2 || !model.Rows[1].Expanded {
		t.Fatalf("mcp tool rows = %#v", model.Rows)
	}
}

func TestMCPSpaceTogglesDisabledFlagOnDisk(t *testing.T) {
	runtime := makeTestRuntime(t)
	cfgPath := filepath.Join(runtime.HomeDir, ".mcp.json")
	if err := os.WriteFile(cfgPath, []byte(`{"mcpServers":{"postgres":{"command":"pg-mcp"}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	name := "postgres"
	app := NewApp(runtime)
	app.ready = true
	app.activeSetupTab = SetupConsoleTabMCPServers
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{{
		ID:         "mcp-postgres",
		Agent:      types.AgentClaudeCode,
		Kind:       types.KindMcpServer,
		Name:       &name,
		SourcePath: "~/.mcp.json",
		Scope:      types.ScopeUser,
		Value:      []byte(`{"command":"pg-mcp"}`),
	}}})

	cmd := app.handleSetupToggle()
	if cmd == nil {
		t.Fatal("toggling a JSON MCP server should return a command")
	}
	msg := cmd()
	if actionMsg, ok := msg.(setupActionMsg); ok && actionMsg.err != nil {
		t.Fatalf("toggle command failed: %v", actionMsg.err)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"disabled": true`) {
		t.Fatalf("expected disabled flag written to config:\n%s", raw)
	}
}

func TestMCPSpaceToggleGatedForTOMLServer(t *testing.T) {
	runtime := makeTestRuntime(t)
	name := "github"
	app := NewApp(runtime)
	app.ready = true
	app.activeSetupTab = SetupConsoleTabMCPServers
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{{
		ID:         "mcp-github",
		Agent:      types.AgentCodex,
		Kind:       types.KindMcpServer,
		Name:       &name,
		SourcePath: "~/.codex/config.toml",
		Scope:      types.ScopeUser,
		Value:      []byte(`{"command":"gh-mcp"}`),
	}}})

	if cmd := app.handleSetupToggle(); cmd != nil {
		t.Fatal("TOML-backed MCP server should not run a real toggle command")
	}
	if app.actionError == "" {
		t.Fatal("expected a gated reason for TOML MCP toggle")
	}
}

func TestMCPSearchShowsMatchingToolRows(t *testing.T) {
	runtime := makeTestRuntime(t)
	name := "posthog"
	app := NewApp(runtime)
	app.ready = true
	app.activeSetupTab = SetupConsoleTabMCPServers
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{{
		ID:         "mcp-posthog",
		Agent:      types.AgentCursor,
		Kind:       types.KindMcpServer,
		Name:       &name,
		SourcePath: "~/.cursor/mcp.json",
		Scope:      types.ScopeUser,
		Metadata: []byte(`{
			"runtimeStatus": "ready",
			"toolCount": 2,
			"tools": [
				{"name":"dashboard-get","description":"Fetch a dashboard."},
				{"name":"feature-flag-create","description":"Create a feature flag."}
			]
		}`),
	}}})

	app.setupSearchFocused = true
	app.activeSetupTabState().searchInput.Focus()
	if _, handled := app.handleSetupSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("dashboard-get")}); !handled {
		t.Fatal("search key should be handled")
	}
	model := app.currentSetupConsoleViewModel()
	if len(model.Rows) != 2 {
		t.Fatalf("mcp search rows = %#v", model.Rows)
	}
	if model.Rows[0].RowKind != SetupConsoleRowInventory || model.Rows[1].RowKind != SetupConsoleRowMCPTool {
		t.Fatalf("mcp search row kinds = %#v", model.Rows)
	}
	if model.Rows[1].Name != "dashboard-get" {
		t.Fatalf("tool row = %#v", model.Rows[1])
	}
}

func TestMCPToolSelectionUsesParentServerDetail(t *testing.T) {
	runtime := makeTestRuntime(t)
	posthogName := "posthog"
	otherName := "zcontext7"
	app := NewApp(runtime)
	app.ready = true
	app.activeSetupTab = SetupConsoleTabMCPServers
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{
		{
			ID:         "mcp-posthog",
			Agent:      types.AgentCursor,
			Kind:       types.KindMcpServer,
			Name:       &posthogName,
			SourcePath: "~/.cursor/mcp.json",
			Scope:      types.ScopeUser,
			Metadata:   []byte(`{"tools":[{"name":"dashboard-get","description":"Fetch a dashboard."}]}`),
		},
		{
			ID:         "mcp-zcontext7",
			Agent:      types.AgentCursor,
			Kind:       types.KindMcpServer,
			Name:       &otherName,
			SourcePath: "~/.codex/config.toml",
			Scope:      types.ScopeUser,
		},
	}})

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding MCP server should not return a command")
	}
	app.moveInventoryCursor(1)
	model := app.currentSetupConsoleViewModel()
	if model.Rows[1].RowKind != SetupConsoleRowMCPTool {
		t.Fatalf("selected row = %#v", model.Rows[1])
	}
	if model.Selected == nil || model.Selected.Title != "posthog" {
		t.Fatalf("selected detail = %#v", model.Selected)
	}
}

func TestInventoryEnterConfirmsActionAndRescans(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newHookInventoryTestApp(t, runtime)
	enableInventoryAction(app, setup.ActionEdit)

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("expanding hook row should not return a command")
	}
	if app.pendingAction != nil {
		t.Fatalf("pending action = %#v", app.pendingAction)
	}
	if app.expandedSetupRowID(SetupConsoleTabHooks) != "hook-session-start" {
		t.Fatalf("expanded hook = %q", app.expandedSetupRowID(SetupConsoleTabHooks))
	}
	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("opening confirmation should not return a command")
	}
	if app.pendingAction == nil {
		t.Fatal("expected pending action")
	}
	if app.pendingAction.TargetName != "session_start" {
		t.Fatalf("pending action = %#v", app.pendingAction)
	}

	executed := 0
	app.actionExecutor = func(_ context.Context, plan setup.ActionPlan) error {
		executed++
		if plan.TargetName != "session_start" {
			t.Fatalf("executed plan = %#v", plan)
		}
		return nil
	}

	cmd := app.handleInventoryEnter()
	if cmd == nil {
		t.Fatal("confirming action should return a command")
	}
	model, _ := app.Update(cmd())
	updated := model.(*App)

	if executed != 1 {
		t.Fatalf("executed = %d", executed)
	}
	if updated.pendingAction != nil {
		t.Fatalf("pending action was not cleared: %#v", updated.pendingAction)
	}
	if updated.notice == "" {
		t.Fatal("expected success notice")
	}
}

func TestInventoryActionFailureKeepsUserInContext(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newHookInventoryTestApp(t, runtime)
	enableInventoryAction(app, setup.ActionEdit)

	app.handleInventoryEnter()
	app.handleInventoryEnter()
	app.actionExecutor = func(context.Context, setup.ActionPlan) error {
		return os.ErrPermission
	}

	cmd := app.handleInventoryEnter()
	model, _ := app.Update(cmd())
	updated := model.(*App)

	if updated.pendingAction == nil {
		t.Fatal("pending action should remain for a failed confirmation")
	}
	if updated.actionError == "" {
		t.Fatal("expected action error")
	}
}

func TestInventoryRescanFailureAfterActionClearsPendingAction(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newHookInventoryTestApp(t, runtime)
	enableInventoryAction(app, setup.ActionEdit)
	app.handleInventoryEnter()
	app.handleInventoryEnter()

	model, _ := app.Update(setupActionMsg{data: bootMsg{err: os.ErrPermission}})
	updated := model.(*App)

	if updated.pendingAction != nil {
		t.Fatalf("pending action should be cleared after executed action: %#v", updated.pendingAction)
	}
	if updated.actionError == "" {
		t.Fatal("expected rescan error")
	}
}

func TestCreateMissingBaselinesWritesAgentScopedSnapshots(t *testing.T) {
	runtime := makeTestRuntime(t)
	codexConfig := filepath.Join(runtime.HomeDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(codexConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexConfig, []byte("model = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	claudeSettings := filepath.Join(runtime.HomeDir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(claudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeSettings, []byte(`{"permissions":{"allow":["Bash(echo hi)"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp(runtime)
	created, err := app.createMissingBaselines()
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %#v", created)
	}
	for _, agent := range []types.AgentID{types.AgentClaudeCode, types.AgentCodex} {
		names, err := store.ListSnapshots(runtime.StoreDir, &agent)
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 1 {
			t.Fatalf("%s snapshots = %#v", agent, names)
		}
		snap, err := store.ReadSnapshot(runtime.StoreDir, names[0], &agent)
		if err != nil {
			t.Fatal(err)
		}
		if snap.Manifest.Security.RedactionPolicy != "content-backed" {
			t.Fatalf("%s redaction policy = %q", agent, snap.Manifest.Security.RedactionPolicy)
		}
	}

	createdAgain, err := app.createMissingBaselines()
	if err != nil {
		t.Fatal(err)
	}
	if len(createdAgain) != 0 {
		t.Fatalf("expected no duplicate baselines, got %#v", createdAgain)
	}
}

func TestCreateMissingBaselinesPreservesExistingAgentBaseline(t *testing.T) {
	runtime := makeTestRuntime(t)
	writeCodexConfig(t, runtime.HomeDir, "model = \"gpt-5\"\n")
	claudeSettings := filepath.Join(runtime.HomeDir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(claudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeSettings, []byte(`{"permissions":{"allow":["Bash(echo hi)"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp(runtime)
	created, err := app.createMissingBaselines()
	if err != nil {
		t.Fatal(err)
	}
	codexBaseline := findCreatedSnapshot(t, created, "baseline-codex-")
	claudeBaseline := findCreatedSnapshot(t, created, "baseline-claude-code-")
	codex := types.AgentCodex
	codexBefore, err := store.ReadSnapshot(runtime.StoreDir, codexBaseline, &codex)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(store.AgentStoreDir(runtime.StoreDir, agentIDPtr(types.AgentClaudeCode)), claudeBaseline)); err != nil {
		t.Fatal(err)
	}

	created, err = app.createMissingBaselines()
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || !strings.HasPrefix(created[0], "baseline-claude-code-") {
		t.Fatalf("expected only the missing Claude Code baseline, got %#v", created)
	}
	codexNames, err := store.ListSnapshots(runtime.StoreDir, &codex)
	if err != nil {
		t.Fatal(err)
	}
	if len(codexNames) != 1 || codexNames[0] != codexBaseline {
		t.Fatalf("existing Codex baseline list changed: %#v", codexNames)
	}
	codexAfter, err := store.ReadSnapshot(runtime.StoreDir, codexBaseline, &codex)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(codexBefore, codexAfter) {
		t.Fatal("existing Codex baseline changed while filling missing coverage")
	}
}

func agentIDPtr(agent types.AgentID) *types.AgentID {
	return &agent
}

func TestHomeBaselineActionCreatesBothSupportedBaselinesAndReturnsClean(t *testing.T) {
	runtime := makeTestRuntime(t)
	writeCodexConfig(t, runtime.HomeDir, "model = \"gpt-5\"\n")
	claudeSettings := filepath.Join(runtime.HomeDir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(claudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeSettings, []byte(`{"permissions":{"allow":["Bash(echo hi)"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp(runtime)
	app.ready = true
	app.applyWorkspaceData(app.fetchWorkspaceData())
	cmd, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")})
	if quit || cmd == nil {
		t.Fatalf("baseline CTA: cmd=%v quit=%v", cmd != nil, quit)
	}

	msg := cmd()
	model, next := app.Update(msg)
	updated := model.(*App)
	if next != nil {
		t.Fatal("baseline completion should not schedule another command")
	}
	if updated.screen != ScreenHome || updated.actionError != "" {
		t.Fatalf("baseline completion: screen=%q error=%q", updated.screen, updated.actionError)
	}
	if !strings.Contains(updated.notice, "Created baselines:") {
		t.Fatalf("notice = %q", updated.notice)
	}
	home := BuildHomeViewModel(updated.baselineStatus)
	if !home.HasBaseline || home.HasMissingBaseline || home.TotalChanges != 0 {
		t.Fatalf("home after baseline creation = %#v", home)
	}
	for _, status := range updated.baselineStatus.Agents {
		if !status.HasBaseline || !status.ContentBacked || status.ChangeCount() != 0 {
			t.Fatalf("agent baseline after creation = %#v", status)
		}
	}
}

func TestSnapshotsBaselineShortcutReturnsToChangesHome(t *testing.T) {
	app := NewApp(makeTestRuntime(t))
	app.ready = true
	app.applyWorkspaceData(app.fetchWorkspaceData())
	app.screen = ScreenSnapshots

	cmd, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")})
	if quit || cmd == nil {
		t.Fatalf("snapshots baseline CTA: cmd=%v quit=%v", cmd != nil, quit)
	}
	model, _ := app.Update(cmd())
	updated := model.(*App)
	if updated.screen != ScreenHome || updated.actionError != "" {
		t.Fatalf("baseline completion: screen=%q error=%q", updated.screen, updated.actionError)
	}
}

func TestSnapshotEnterBuildsRollbackReview(t *testing.T) {
	runtime := makeTestRuntime(t)
	configPath := writeCodexConfig(t, runtime.HomeDir, "model = \"gpt-5\"\n")
	app := NewApp(runtime)
	created, err := app.createMissingBaselines()
	if err != nil {
		t.Fatal(err)
	}
	codexBaseline := findCreatedSnapshot(t, created, "baseline-codex-")
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5.1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.applyWorkspaceData(app.fetchWorkspaceData())
	app.screen = ScreenSnapshots
	app.snapshotCursor = findSnapshotRefIndex(t, app.snapshotRefs, codexBaseline, types.AgentCodex)

	cmd := app.handleSnapshotEnter()
	if cmd == nil {
		t.Fatal("expected preview command")
	}
	model, _ := app.Update(cmd())
	updated := model.(*App)
	if updated.rollbackReview == nil {
		t.Fatal("expected rollback review")
	}
	if updated.rollbackReview.SnapshotName != codexBaseline || len(updated.rollbackReview.Items) == 0 {
		t.Fatalf("review = %#v", updated.rollbackReview)
	}
}

func TestRollbackApplyBlocksWhenRestorePointFails(t *testing.T) {
	runtime := makeTestRuntime(t)
	configPath := writeCodexConfig(t, runtime.HomeDir, "model = \"gpt-5\"\n")
	app := NewApp(runtime)
	created, err := app.createMissingBaselines()
	if err != nil {
		t.Fatal(err)
	}
	codexBaseline := findCreatedSnapshot(t, created, "baseline-codex-")
	changed := "model = \"gpt-5.1\"\n"
	if err := os.WriteFile(configPath, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	review, err := app.buildRollbackReview(snapshotRef{Name: codexBaseline, Agent: types.AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	app.restorePointCreator = func(types.AgentID) (string, error) {
		return "", os.ErrPermission
	}

	msg := app.applyRollbackReview(review)
	if msg.err == nil {
		t.Fatal("expected restore point failure")
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != changed {
		t.Fatalf("config changed despite restore point failure: %q", got)
	}
}

func TestRollbackApplyRestoresConfigAndVerifies(t *testing.T) {
	runtime := makeTestRuntime(t)
	configPath := writeCodexConfig(t, runtime.HomeDir, "model = \"gpt-5\"\n")
	app := NewApp(runtime)
	created, err := app.createMissingBaselines()
	if err != nil {
		t.Fatal(err)
	}
	codexBaseline := findCreatedSnapshot(t, created, "baseline-codex-")
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5.1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	review, err := app.buildRollbackReview(snapshotRef{Name: codexBaseline, Agent: types.AgentCodex})
	if err != nil {
		t.Fatal(err)
	}

	msg := app.applyRollbackReview(review)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if !strings.Contains(msg.verify, "Verified") {
		t.Fatalf("verify = %q", msg.verify)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "model = \"gpt-5\"\n" {
		t.Fatalf("config = %q", got)
	}
}

func TestRollbackApplyRejectsStaleReview(t *testing.T) {
	runtime := makeTestRuntime(t)
	configPath := writeCodexConfig(t, runtime.HomeDir, "model = \"gpt-5\"\n")
	app := NewApp(runtime)
	created, err := app.createMissingBaselines()
	if err != nil {
		t.Fatal(err)
	}
	codexBaseline := findCreatedSnapshot(t, created, "baseline-codex-")
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5.1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	review, err := app.buildRollbackReview(snapshotRef{Name: codexBaseline, Agent: types.AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5.2\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := app.applyRollbackReview(review)
	if msg.err == nil || !strings.Contains(msg.err.Error(), "stale") {
		t.Fatalf("expected stale review error, got %v", msg.err)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "model = \"gpt-5.2\"\n" {
		t.Fatalf("stale review should not write config, got %q", got)
	}
}

func TestUndoPreviewMessageUpdatesCorruptEventsInUpdate(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := NewApp(runtime)
	app.corruptEvents = []store.TimelineCorruptEvent{{FilePath: "old"}}
	next := []store.TimelineCorruptEvent{{FilePath: "new"}}

	model, _ := app.Update(undoPreviewMsg{corruptEvents: next})
	updated := model.(*App)
	if len(updated.corruptEvents) != 1 || updated.corruptEvents[0].FilePath != "new" {
		t.Fatalf("corrupt events = %#v", updated.corruptEvents)
	}

	model, _ = updated.Update(undoPreviewMsg{})
	updated = model.(*App)
	if len(updated.corruptEvents) != 0 {
		t.Fatalf("corrupt events should clear: %#v", updated.corruptEvents)
	}
}

func TestEnvironmentsModeOpensAndListsPerAgentDrift(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newInventoryTestApp(t, runtime)
	app.width = 120
	app.height = 32
	app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{
		{Agent: types.AgentClaudeCode, HasBaseline: true, BaselineName: "base-cc"},
		{Agent: types.AgentCodex, HasBaseline: true, BaselineName: "base-cx", SemanticChangeCount: 2},
	}}

	if _, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")}); quit {
		t.Fatal("E should not quit")
	}
	if app.screen != ScreenEnvironments {
		t.Fatalf("expected environments screen, got %s", app.screen)
	}
	view := app.View()
	if !strings.Contains(view, "Environments") {
		t.Fatalf("expected environments title:\n%s", view)
	}
	if !strings.Contains(view, "Claude Code") || !strings.Contains(view, "Codex") {
		t.Fatalf("expected per-agent rows:\n%s", view)
	}
	if !strings.Contains(view, "2 changes") {
		t.Fatalf("expected drift count for changed agent:\n%s", view)
	}

	// Navigating down moves the focused agent.
	app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if app.environments.agentCursor != 1 {
		t.Fatalf("expected environment cursor to advance, got %d", app.environments.agentCursor)
	}
}

func TestEnvironmentsKeysMoveFocusSurfacesDiffAndHunks(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newInventoryTestApp(t, runtime)
	app.width = 140
	app.height = 36
	sourcePath := "~/.codex/config.toml"
	app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{{
		Agent: types.AgentCodex, HasBaseline: true, BaselineName: "base-cx", SemanticChangeCount: 2,
		Diff: diff.GraphDiff{SemanticChanges: []diff.SemanticChange{
			{
				Code:       diff.SemanticAgentConfigChanged,
				EntityKind: types.KindAgentConfig,
				EntityName: "config",
				Before:     []byte(`{"field00":"old","field01":"same","field02":"same","field03":"same","field04":"same","field05":"same","field06":"same","field07":"old"}`),
				After:      []byte(`{"field00":"new","field01":"same","field02":"same","field03":"same","field04":"same","field05":"same","field06":"same","field07":"new"}`),
				Details: diff.SemanticChangeDetails{
					ChangedFields: []string{"field00", "field07"},
					SourcePath:    &sourcePath,
				},
			},
			{
				Code:       diff.SemanticMcpAdded,
				EntityKind: types.KindMcpServer,
				EntityName: "postgres",
				After:      []byte(`{"command":"pg-mcp"}`),
				Details:    diff.SemanticChangeDetails{SourcePath: &sourcePath},
			},
		}},
	}}}

	app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	if app.screen != ScreenEnvironments {
		t.Fatalf("screen = %s", app.screen)
	}
	if app.environments.focus != EnvironmentFocusAgents {
		t.Fatalf("focus = %s", app.environments.focus)
	}

	app.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if app.environments.focus != EnvironmentFocusSurfaces {
		t.Fatalf("focus after tab = %s", app.environments.focus)
	}
	app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if app.environments.surfaceCursor != 1 || app.environments.agentCursor != 0 {
		t.Fatalf("surface cursor = %d agent cursor = %d", app.environments.surfaceCursor, app.environments.agentCursor)
	}
	app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if app.environments.surfaceCursor != 0 {
		t.Fatalf("surface cursor after k = %d", app.environments.surfaceCursor)
	}

	app.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if app.environments.focus != EnvironmentFocusDiff {
		t.Fatalf("focus after second tab = %s", app.environments.focus)
	}
	app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if app.environments.hunkCursor != 1 {
		t.Fatalf("hunk cursor after n = %d", app.environments.hunkCursor)
	}
	model := app.currentEnvironmentsViewModel()
	if !environmentModelHasCurrentHunk(model, 1) {
		t.Fatalf("expected hunk 1 selected: %#v", model.Diff.Rows)
	}

	app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if app.environments.mode != EnvironmentRenderModeUnified {
		t.Fatalf("mode after v = %s", app.environments.mode)
	}
}

func environmentModelHasCurrentHunk(model EnvironmentsViewModel, index int) bool {
	for _, row := range model.Diff.Rows {
		if row.Kind == EnvironmentDiffRowHunk && row.HunkIndex == index && row.CurrentHunk {
			return true
		}
	}
	return false
}

func TestEnvironmentsRestoreWithoutSnapshotIsGated(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newInventoryTestApp(t, runtime)
	app.baselineStatus = baseline.Status{Agents: []baseline.AgentStatus{
		{Agent: types.AgentClaudeCode, HasBaseline: false},
	}}
	app.screen = ScreenEnvironments

	if cmd := app.restoreFocusedEnvironment(); cmd != nil {
		t.Fatal("restore should be gated when no snapshot exists for the agent")
	}
	if app.actionError == "" {
		t.Fatal("expected a gated reason when restoring without a snapshot")
	}
}

func TestSetupConsoleFirstScreenUsesTopTabsWithoutSidebar(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newInventoryTestApp(t, runtime)
	app.width = 120
	app.height = 32

	view := app.View()
	if !strings.Contains(view, "Hooks") || !strings.Contains(view, "Plugins") || !strings.Contains(view, "Marketplace") {
		t.Fatalf("expected top tabs in view:\n%s", view)
	}
	if strings.Contains(view, "gandalf tui · setup console") {
		t.Fatalf("expected no setup console title header in view:\n%s", view)
	}
	if strings.Contains(view, "Profiles") || strings.Contains(view, "Agents") || strings.Contains(view, "Inventory\n") {
		t.Fatalf("expected no persistent sidebar in view:\n%s", view)
	}
}

func TestSetupConsoleSearchFiltersActiveTab(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newInventoryTestApp(t, runtime)
	name := "planning"
	app.applyWorkspaceData(bootMsg{evidence: append(app.evidence, types.DiscoveredItem{
		ID:         "skill-plan",
		Agent:      types.AgentCodex,
		Kind:       types.KindSkill,
		Name:       &name,
		SourcePath: "~/.codex/skills/planning",
		Scope:      types.ScopeUser,
	})})

	if _, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}); quit {
		t.Fatal("slash should not quit")
	}
	if !app.setupSearchFocused {
		t.Fatal("search should be focused")
	}
	if cmd, handled := app.handleSetupSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("plan")}); !handled {
		t.Fatal("search key should be handled")
	} else if cmd != nil {
		cmd()
	}
	rows := app.currentInventory()
	if len(rows) != 1 || rows[0].Name != "planning" {
		t.Fatalf("filtered rows = %#v", rows)
	}
	if _, handled := app.handleSetupSearchKey(tea.KeyMsg{Type: tea.KeyEnter}); !handled {
		t.Fatal("enter should blur search")
	}
	if app.setupSearchFocused {
		t.Fatal("search should be blurred")
	}
}

func TestInventoryKeyboardFlowSwitchesTabsAndCancelsPendingAction(t *testing.T) {
	runtime := makeTestRuntime(t)
	app := newInventoryTestApp(t, runtime)

	if !app.inventoryFocus {
		t.Fatal("inventory should start focused")
	}

	if _, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyTab}); quit {
		t.Fatal("tab should not quit")
	}
	if app.activeSetupTab != SetupConsoleTabMCPServers {
		t.Fatalf("tab should switch from skills to mcp servers: %s", app.activeSetupTab)
	}
	if _, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab}); quit {
		t.Fatal("shift+tab should not quit")
	}
	if app.activeSetupTab != SetupConsoleTabSkills {
		t.Fatalf("shift+tab should return to skills: %s", app.activeSetupTab)
	}
	cmd, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if quit {
		t.Fatal("enter should not quit")
	}
	if cmd != nil {
		t.Fatal("expanding skill row should not return a command")
	}
	if app.skillViewer != nil {
		t.Fatalf("skill viewer = %#v", app.skillViewer)
	}
	if app.expandedSetupRowID(SetupConsoleTabSkills) != "skill-review" {
		t.Fatalf("expanded skill = %q", app.expandedSetupRowID(SetupConsoleTabSkills))
	}
	cmd, quit = app.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if quit {
		t.Fatal("enter should not quit")
	}
	if cmd != nil {
		t.Fatal("opening viewer should not return a command")
	}
	if app.skillViewer == nil {
		t.Fatal("expected skill viewer")
	}
	if _, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyEsc}); quit {
		t.Fatal("esc should not quit")
	}
	if app.skillViewer != nil {
		t.Fatalf("skill viewer after esc = %#v", app.skillViewer)
	}

	app = newHookInventoryTestApp(t, runtime)
	enableInventoryAction(app, setup.ActionEdit)
	cmd, quit = app.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if quit {
		t.Fatal("enter should not quit")
	}
	if cmd != nil {
		t.Fatal("expanding hook row should not return a command")
	}
	if app.pendingAction != nil {
		t.Fatalf("pending action = %#v", app.pendingAction)
	}
	cmd, quit = app.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if quit {
		t.Fatal("enter should not quit")
	}
	if cmd != nil {
		t.Fatal("opening confirmation should not return a command")
	}
	if app.pendingAction == nil {
		t.Fatal("expected pending action")
	}

	if _, quit := app.handleKey(tea.KeyMsg{Type: tea.KeyEsc}); quit {
		t.Fatal("esc should not quit")
	}
	if app.pendingAction != nil {
		t.Fatalf("pending action after esc = %#v", app.pendingAction)
	}
}

func newInventoryTestApp(t *testing.T, runtime types.RuntimeOptions) *App {
	t.Helper()
	name := "review"
	app := NewApp(runtime)
	app.screen = ScreenInventory
	app.activeSetupTab = SetupConsoleTabSkills
	app.ready = true
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{{
		ID:         "skill-review",
		Agent:      types.AgentCodex,
		Kind:       types.KindSkill,
		Name:       &name,
		SourcePath: "~/.codex/skills/review",
		Scope:      types.ScopeUser,
	}}})
	return app
}

func newHookInventoryTestApp(t *testing.T, runtime types.RuntimeOptions) *App {
	t.Helper()
	name := "session_start"
	app := NewApp(runtime)
	app.screen = ScreenInventory
	app.activeSetupTab = SetupConsoleTabHooks
	app.ready = true
	app.applyWorkspaceData(bootMsg{evidence: []types.DiscoveredItem{{
		ID:         "hook-session-start",
		Agent:      types.AgentCodex,
		Kind:       types.KindHook,
		Name:       &name,
		SourcePath: "~/.codex/hooks.json",
		Scope:      types.ScopeUser,
	}}})
	return app
}

func enableInventoryAction(app *App, action setup.ActionKind) {
	app.inventory[0].Actions = []setup.ActionAvailability{
		{Action: action, Available: true},
	}
}

func makeTestRuntime(t *testing.T) types.RuntimeOptions {
	t.Helper()
	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	homeDir := filepath.Join(root, "home")
	storeDir := filepath.Join(homeDir, ".gandalf")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
	}
}

func writeCodexConfig(t *testing.T, homeDir, content string) string {
	t.Helper()
	configPath := filepath.Join(homeDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func findCreatedSnapshot(t *testing.T, names []string, prefix string) string {
	t.Helper()
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			return name
		}
	}
	t.Fatalf("missing created snapshot with prefix %q in %#v", prefix, names)
	return ""
}

func findSnapshotRefIndex(t *testing.T, refs []snapshotRef, name string, agent types.AgentID) int {
	t.Helper()
	for i, ref := range refs {
		if ref.Name == name && ref.Agent == agent {
			return i
		}
	}
	t.Fatalf("missing snapshot ref %s/%s in %#v", agent, name, refs)
	return 0
}
