package setup

import (
	"context"
	"errors"
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
	name := "github"
	items := BuildInventory([]types.DiscoveredItem{
		{
			ID:         "mcp-github",
			Agent:      types.AgentCodex,
			Kind:       types.KindMcpServer,
			Name:       &name,
			SourcePath: "~/.codex/config.toml",
			Scope:      types.ScopeUser,
		},
	})

	plan := PlanItemAction(items[0], ActionRemove)

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
	name := "customize-opencode"
	items := BuildInventory([]types.DiscoveredItem{
		{
			ID:         "managed-skill",
			Agent:      types.AgentOpencode,
			Kind:       types.KindSkill,
			Name:       &name,
			SourcePath: "<built-in>",
			Scope:      types.ScopeManaged,
		},
	})

	plan := PlanItemAction(items[0], ActionRemove)

	if plan.Available {
		t.Fatalf("managed action should be unavailable: %#v", plan)
	}
	if plan.UnavailableReason == "" {
		t.Fatalf("missing unavailable reason: %#v", plan)
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
