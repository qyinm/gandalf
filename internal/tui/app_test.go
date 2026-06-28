package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

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

func TestMarketplaceEnterTogglesSourceAndChildActionIsProviderGated(t *testing.T) {
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
		t.Fatal("provider-gated child action should not return a command")
	}
	if !strings.Contains(app.actionError, "provider") {
		t.Fatalf("action error = %q", app.actionError)
	}

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
