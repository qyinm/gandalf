package setup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type fakeRunner struct {
	commands []CommandPlan
	err      error
}

func (f *fakeRunner) Run(_ context.Context, command CommandPlan) error {
	f.commands = append(f.commands, command)
	return f.err
}

func TestPlanItemActionBuildsConfirmationFields(t *testing.T) {
	item := InventoryItem{
		ID:         "mcp-github",
		Agent:      types.AgentCodex,
		ObjectKind: ObjectMCPServer,
		Name:       "github",
		SourcePath: "~/.codex/config.toml",
		Scope:      types.ScopeUser,
		Actions: []ActionAvailability{
			{Action: ActionRemove, Available: true},
		},
	}

	plan := PlanItemAction(item, ActionRemove)

	if !plan.Available {
		t.Fatalf("plan unavailable: %#v", plan)
	}
	if plan.Agent != types.AgentCodex {
		t.Fatalf("agent = %q", plan.Agent)
	}
	if plan.ObjectKind != ObjectMCPServer {
		t.Fatalf("object kind = %q", plan.ObjectKind)
	}
	if plan.TargetName != "github" {
		t.Fatalf("target = %q", plan.TargetName)
	}
	if plan.Operation == "" {
		t.Fatal("expected operation")
	}
	if plan.ConfigTarget != "~/.codex/config.toml" {
		t.Fatalf("config target = %q", plan.ConfigTarget)
	}
}

func TestPlanItemActionRejectsUnavailableActions(t *testing.T) {
	name := "review"
	items := BuildInventory([]types.DiscoveredItem{
		{
			ID:         "user-skill",
			Agent:      types.AgentCodex,
			Kind:       types.KindSkill,
			Name:       &name,
			SourcePath: "~/.codex/skills/review",
			Scope:      types.ScopeUser,
		},
	})

	plan := PlanItemAction(items[0], ActionRemove)

	if plan.Available {
		t.Fatalf("action should be unavailable without a provider: %#v", plan)
	}
	if plan.UnavailableReason == "" {
		t.Fatalf("missing unavailable reason: %#v", plan)
	}
}

func TestPlanItemActionRejectsAddAndUnknownActions(t *testing.T) {
	item := InventoryItem{
		ID:         "user-skill",
		Agent:      types.AgentCodex,
		ObjectKind: ObjectSkill,
		Name:       "review",
		SourcePath: "~/.codex/skills/review",
		Scope:      types.ScopeUser,
		Actions: []ActionAvailability{
			{Action: ActionAdd, Available: true},
			{Action: ActionKind("bogus"), Available: true},
		},
	}

	for _, action := range []ActionKind{ActionAdd, ActionKind("bogus")} {
		plan := PlanItemAction(item, action)
		if plan.Available {
			t.Fatalf("%s action should be unavailable: %#v", action, plan)
		}
		if plan.UnavailableReason == "" {
			t.Fatalf("%s action missing unavailable reason: %#v", action, plan)
		}
	}
}

func TestPlanItemActionRejectsProjectTargets(t *testing.T) {
	item := InventoryItem{
		ID:         "project-hook",
		Agent:      types.AgentCodex,
		ObjectKind: ObjectHook,
		Name:       "project-hook",
		SourcePath: ".codex/hooks.json",
		Scope:      types.ScopeProject,
		Actions: []ActionAvailability{
			{Action: ActionRemove, Available: true},
		},
	}

	plan := PlanItemAction(item, ActionRemove)

	if plan.Available {
		t.Fatalf("project action should be unavailable: %#v", plan)
	}
}

func TestExecuteActionPlanRunsCommandPlan(t *testing.T) {
	runner := &fakeRunner{}
	command := CommandPlan{Program: "pi", Args: []string{"extension", "install", "browser"}}
	plan := ActionPlan{
		ID:           "install-browser",
		Action:       ActionAdd,
		Agent:        types.AgentPiAgent,
		ObjectKind:   ObjectPlugin,
		TargetName:   "browser",
		Operation:    "run agent-native command",
		ConfigTarget: "~/.pi/agent/settings.json",
		Command:      &command,
		Available:    true,
	}

	result, err := ExecuteActionPlan(context.Background(), plan, runner)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExecutedCommand {
		t.Fatalf("result = %#v", result)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %#v", runner.commands)
	}
	if runner.commands[0].Program != "pi" {
		t.Fatalf("command = %#v", runner.commands[0])
	}
}

func TestExecuteActionPlanReturnsUnavailableError(t *testing.T) {
	_, err := ExecuteActionPlan(context.Background(), ActionPlan{
		Available:         false,
		UnavailableReason: "no native installer",
	}, &fakeRunner{})

	if !errors.Is(err, ErrActionUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestExecuteActionPlanRejectsIncompleteCommandPlans(t *testing.T) {
	command := CommandPlan{Program: "   "}
	tests := []struct {
		name string
		plan ActionPlan
	}{
		{
			name: "empty target",
			plan: ActionPlan{Available: true},
		},
		{
			name: "nil command",
			plan: ActionPlan{Available: true, ConfigTarget: "~/.codex/config.toml"},
		},
		{
			name: "blank command program",
			plan: ActionPlan{Available: true, ConfigTarget: "~/.codex/config.toml", Command: &command},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ExecuteActionPlan(context.Background(), tt.plan, &fakeRunner{}); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	validCommand := CommandPlan{Program: "pi"}
	if _, err := ExecuteActionPlan(context.Background(), ActionPlan{
		Available:    true,
		ConfigTarget: "~/.pi/agent/settings.json",
		Command:      &validCommand,
	}, nil); err == nil {
		t.Fatal("expected nil runner error")
	}
}

func TestToggleAvailabilityGatedToJSONMCPUserScope(t *testing.T) {
	jsonMCP := BuildInventory([]types.DiscoveredItem{{
		ID: "mcp-pg", Agent: types.AgentClaudeCode, Kind: types.KindMcpServer,
		Name: stringPtr("postgres"), SourcePath: "~/.claude/.mcp.json", Scope: types.ScopeUser,
	}})[0]
	if !hasAvailableAction(jsonMCP, ActionToggle) {
		t.Fatalf("JSON MCP user scope should expose a real toggle: %#v", jsonMCP.Actions)
	}

	tomlMCP := BuildInventory([]types.DiscoveredItem{{
		ID: "mcp-cx", Agent: types.AgentCodex, Kind: types.KindMcpServer,
		Name: stringPtr("github"), SourcePath: "~/.codex/config.toml", Scope: types.ScopeUser,
	}})[0]
	if hasAvailableAction(tomlMCP, ActionToggle) {
		t.Fatalf("TOML MCP should not expose a real toggle: %#v", tomlMCP.Actions)
	}

	skill := BuildInventory([]types.DiscoveredItem{{
		ID: "skill-a", Agent: types.AgentCodex, Kind: types.KindSkill,
		Name: stringPtr("review"), SourcePath: "~/.codex/skills/review", Scope: types.ScopeUser,
	}})[0]
	if hasAvailableAction(skill, ActionToggle) {
		t.Fatalf("non-MCP object should not expose a toggle: %#v", skill.Actions)
	}
}

func TestExecuteMCPToggleFlipsDisabledFlag(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, ".claude", ".mcp.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{
  "mcpServers": {
    "postgres": { "command": "pg-mcp" }
  }
}` + "\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	item := BuildInventory([]types.DiscoveredItem{{
		ID: "mcp-pg", Agent: types.AgentClaudeCode, Kind: types.KindMcpServer,
		Name: stringPtr("postgres"), SourcePath: "~/.claude/.mcp.json", Scope: types.ScopeUser,
	}})[0]
	plan := PlanItemAction(item, ActionToggle)
	if !plan.Available {
		t.Fatalf("toggle plan should be available: %#v", plan)
	}

	// Disable.
	res, err := ExecuteMCPToggle(plan, home, "postgres", item.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Disabled {
		t.Fatalf("expected disabled after first toggle: %#v", res)
	}
	if got := mcpServerDisabledOnDisk(t, cfgPath, "postgres"); !got {
		t.Fatal("disabled flag not written")
	}

	// Enable again (flag removed).
	res, err = ExecuteMCPToggle(plan, home, "postgres", item.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if res.Disabled {
		t.Fatalf("expected enabled after second toggle: %#v", res)
	}
	if got := mcpServerDisabledOnDisk(t, cfgPath, "postgres"); got {
		t.Fatal("disabled flag should be cleared")
	}
}

func TestExecuteMCPToggleRefusesPathOutsideHome(t *testing.T) {
	home := t.TempDir()
	plan := ActionPlan{Action: ActionToggle, ObjectKind: ObjectMCPServer, Available: true}
	if _, err := ExecuteMCPToggle(plan, home, "postgres", "/etc/evil.mcp.json"); err == nil {
		t.Fatal("expected confinement error for path outside home")
	}
}

func hasAvailableAction(item InventoryItem, action ActionKind) bool {
	for _, a := range item.Actions {
		if a.Action == action {
			return a.Available
		}
	}
	return false
}

func mcpServerDisabledOnDisk(t *testing.T, path, server string) bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	entry, _ := servers[server].(map[string]any)
	disabled, _ := entry["disabled"].(bool)
	return disabled
}

func TestExecuteActionPlanPropagatesRunnerFailure(t *testing.T) {
	expected := errors.New("boom")
	runner := &fakeRunner{err: expected}
	command := CommandPlan{Program: "pi", Args: []string{"extension", "install", "browser"}}
	plan := ActionPlan{
		ID:           "install-browser",
		Action:       ActionAdd,
		Agent:        types.AgentPiAgent,
		ObjectKind:   ObjectPlugin,
		TargetName:   "browser",
		Operation:    "run agent-native command",
		ConfigTarget: "~/.pi/agent/settings.json",
		Command:      &command,
		Available:    true,
	}

	_, err := ExecuteActionPlan(context.Background(), plan, runner)
	if !errors.Is(err, expected) {
		t.Fatalf("err = %v", err)
	}
}
