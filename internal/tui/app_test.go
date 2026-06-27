package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestInventoryEnterConfirmsActionAndRescans(t *testing.T) {
	runtime := makeTestRuntime(t)
	name := "review"
	app := NewApp(runtime)
	app.ready = true
	app.evidence = []types.DiscoveredItem{{
		ID:         "skill-review",
		Agent:      types.AgentCodex,
		Kind:       types.KindSkill,
		Name:       &name,
		SourcePath: "~/.codex/skills/review",
		Scope:      types.ScopeUser,
	}}

	if cmd := app.handleInventoryEnter(); cmd != nil {
		t.Fatal("opening confirmation should not return a command")
	}
	if app.pendingAction == nil {
		t.Fatal("expected pending action")
	}
	if app.pendingAction.TargetName != "review" {
		t.Fatalf("pending action = %#v", app.pendingAction)
	}

	executed := 0
	app.actionExecutor = func(_ context.Context, plan setup.ActionPlan) error {
		executed++
		if plan.TargetName != "review" {
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
	name := "review"
	app := NewApp(runtime)
	app.ready = true
	app.evidence = []types.DiscoveredItem{{
		ID:         "skill-review",
		Agent:      types.AgentCodex,
		Kind:       types.KindSkill,
		Name:       &name,
		SourcePath: "~/.codex/skills/review",
		Scope:      types.ScopeUser,
	}}

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
