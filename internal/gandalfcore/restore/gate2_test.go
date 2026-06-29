package restore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/restore"
	_ "github.com/qyinm/gandalf/internal/gandalfcore/scan/plugins"
	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// Gate 2 wedge: snapshot → corrupt config → diff → dry-run restore → apply → verify byte-exact.
func TestGate2CodexRollbackWedge(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	homeDir := filepath.Join(root, "home")
	storeDir := filepath.Join(root, "store")
	codexDir := filepath.Join(homeDir, ".codex")
	configPath := filepath.Join(codexDir, "config.toml")

	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}

	originalConfig := "model = \"gpt-5\"\napproval_policy = \"on-request\"\n\n[mcp_servers.github]\ncommand = \"gh\"\nargs = [\"mcp\", \"server\"]\n"
	if err := os.WriteFile(configPath, []byte(originalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "README.md"), []byte("Disposable Gate 2 acceptance project.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentCodex
	scope := types.ScopeUser
	runtime := &types.RuntimeOptions{
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		Agent:          &agent,
		Scope:          &scope,
		CaptureContent: true,
	}

	state, err := snapshot.CaptureCurrentState(runtime, "clean-codex")
	if err != nil {
		t.Fatalf("snapshot capture: %v", err)
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	if err := os.WriteFile(configPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	addedSkill := filepath.Join(codexDir, "skills", "synthetic-harness", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(addedSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := "---\nname: synthetic-harness\n---\nAdds a disposable acceptance skill.\n"
	if err := os.WriteFile(addedSkill, []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	baseline, err := store.ReadSnapshot(storeDir, "clean-codex", &agent)
	if err != nil {
		t.Fatalf("read baseline snapshot: %v", err)
	}
	current, err := snapshot.CaptureCurrentState(runtime, "current")
	if err != nil {
		t.Fatalf("capture current: %v", err)
	}
	graphDiff := diff.DiffGraphs(baseline.Graph, current.Snapshot.Graph)
	if len(graphDiff.SemanticChanges) == 0 && len(graphDiff.RawSourceChanges) == 0 {
		t.Fatal("expected diff after synthetic harness install")
	}

	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: "clean-codex",
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		DryRun:         true,
		Agent:          &agent,
		Scope:          &scope,
	})
	if err != nil {
		t.Fatalf("dry-run plan: %v", err)
	}
	if len(plan.Items) == 0 {
		t.Fatalf("expected restore items, got unsupported=%#v", plan.UnsupportedItems)
	}

	planJSON, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	parsed := restore.ParseDryRunOutput(string(planJSON))
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse dry-run: %#v", parsed.Errors)
	}

	summary := restore.ApplyRestoreItems(
		parsed.Items,
		restore.CreateDefaultApplyExecutor(),
		&types.ApplyOptions{
			FailFast:    true,
			HomeDir:     &homeDir,
			ProjectPath: &projectPath,
		},
	)
	if summary.Failed != 0 {
		t.Fatalf("apply failed: %#v", summary.Failures)
	}

	gotConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotConfig) != originalConfig {
		t.Fatalf("config = %q, want byte-exact restore", string(gotConfig))
	}
	if _, err := os.Stat(addedSkill); !os.IsNotExist(err) {
		t.Fatalf("synthetic skill still exists: %v", err)
	}
}

func TestGate2ClaudeCodeRollbackWedge(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	homeDir := filepath.Join(root, "home")
	storeDir := filepath.Join(root, "store")
	claudeDir := filepath.Join(homeDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	originalSettings := "{\n  \"permissions\": {\n    \"allow\": [\n      \"Bash(echo hi)\"\n    ]\n  }\n}\n"
	if err := os.WriteFile(settingsPath, []byte(originalSettings), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "README.md"), []byte("Disposable Claude Gate 2 acceptance project.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentClaudeCode
	scope := types.ScopeUser
	runtime := &types.RuntimeOptions{
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		Agent:          &agent,
		Scope:          &scope,
		CaptureContent: true,
	}

	state, err := snapshot.CaptureCurrentState(runtime, "clean-claude")
	if err != nil {
		t.Fatalf("snapshot capture: %v", err)
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	changedSettings := "{\n  \"permissions\": {\n    \"allow\": [\n      \"Bash(npm install)\"\n    ]\n  }\n}\n"
	if err := os.WriteFile(settingsPath, []byte(changedSettings), 0o644); err != nil {
		t.Fatal(err)
	}

	baseline, err := store.ReadSnapshot(storeDir, "clean-claude", &agent)
	if err != nil {
		t.Fatalf("read baseline snapshot: %v", err)
	}
	current, err := snapshot.CaptureCurrentState(runtime, "current")
	if err != nil {
		t.Fatalf("capture current: %v", err)
	}
	graphDiff := diff.DiffGraphs(baseline.Graph, current.Snapshot.Graph)
	if len(graphDiff.SemanticChanges) == 0 && len(graphDiff.RawSourceChanges) == 0 {
		t.Fatal("expected diff after Claude settings mutation")
	}

	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: "clean-claude",
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		DryRun:         true,
		Agent:          &agent,
		Scope:          &scope,
	})
	if err != nil {
		t.Fatalf("dry-run plan: %v", err)
	}
	if len(plan.Items) == 0 {
		t.Fatalf("expected restore items, got unsupported=%#v", plan.UnsupportedItems)
	}

	parsed := restore.RestoreItemsFromPlan(plan)
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse plan: %#v", parsed.Errors)
	}
	summary := restore.ApplyRestoreItems(
		parsed.Items,
		restore.CreateDefaultApplyExecutor(),
		&types.ApplyOptions{
			FailFast:    true,
			HomeDir:     &homeDir,
			ProjectPath: &projectPath,
		},
	)
	if summary.Failed != 0 {
		t.Fatalf("apply failed: %#v", summary.Failures)
	}

	gotSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSettings) != originalSettings {
		t.Fatalf("settings = %q, want byte-exact restore", string(gotSettings))
	}
}
