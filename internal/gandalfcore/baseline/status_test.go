package baseline_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/baseline"
	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func makeSandbox(t *testing.T) (projectPath, homeDir, storeDir string) {
	t.Helper()
	root := t.TempDir()
	projectPath = filepath.Join(root, "project")
	homeDir = filepath.Join(root, "home")
	storeDir = filepath.Join(root, "store")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return projectPath, homeDir, storeDir
}

func findAgentStatus(t *testing.T, status baseline.Status, agent types.AgentID) baseline.AgentStatus {
	t.Helper()
	for _, item := range status.Agents {
		if item.Agent == agent {
			return item
		}
	}
	t.Fatalf("missing agent status for %s in %#v", agent, status.Agents)
	return baseline.AgentStatus{}
}

func TestBuildStatusReportsAgentScopedBaselineAndChanges(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeSandbox(t)
	codexConfig := filepath.Join(homeDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(codexConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexConfig, []byte("model = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	claudeSettings := filepath.Join(homeDir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(claudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeSettings, []byte(`{"permissions":{"allow":["Bash(echo hi)"]}}`), 0o644); err != nil {
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
	state, err := snapshot.CaptureCurrentState(runtime, "baseline-codex")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(codexConfig, []byte("model = \"gpt-5.1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := baseline.BuildStatus(types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Agents) != 2 {
		t.Fatalf("agents = %#v", status.Agents)
	}

	codex := findAgentStatus(t, status, types.AgentCodex)
	if !codex.HasBaseline || codex.BaselineName != "baseline-codex" {
		t.Fatalf("codex baseline = %#v", codex)
	}
	if !codex.ContentBacked {
		t.Fatalf("expected content-backed codex baseline: %#v", codex)
	}
	if codex.ChangeCount() == 0 {
		t.Fatalf("expected codex changes: %#v", codex)
	}

	claude := findAgentStatus(t, status, types.AgentClaudeCode)
	if claude.HasBaseline {
		t.Fatalf("claude should be missing baseline: %#v", claude)
	}
}

func TestBuildStatusIgnoresPreApplyRestorePoints(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeSandbox(t)
	codexConfig := filepath.Join(homeDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(codexConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexConfig, []byte("model = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentCodex
	scope := types.ScopeUser
	runtime := &types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
		Agent:       &agent,
		Scope:       &scope,
	}
	baselineState, err := snapshot.CaptureCurrentState(runtime, "baseline-codex")
	if err != nil {
		t.Fatal(err)
	}
	baselineSnapshot := baselineState.Snapshot
	baselineSnapshot.Manifest.CreatedAt = "2026-06-29T01:00:00Z"
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(baselineSnapshot), &agent); err != nil {
		t.Fatal(err)
	}

	restorePointState, err := snapshot.CaptureCurrentState(runtime, "pre-apply-codex-20260629-020000-000000000")
	if err != nil {
		t.Fatal(err)
	}
	restorePoint := restorePointState.Snapshot
	restorePoint.Manifest.CreatedAt = "2026-06-29T02:00:00Z"
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(restorePoint), &agent); err != nil {
		t.Fatal(err)
	}

	status, err := baseline.BuildStatus(types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	codex := findAgentStatus(t, status, types.AgentCodex)
	if !codex.HasBaseline || codex.BaselineName != "baseline-codex" {
		t.Fatalf("codex baseline = %#v, want baseline-codex", codex)
	}
}

func TestBuildStatusDoesNotTreatOnlyPreApplySnapshotAsBaseline(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeSandbox(t)
	codexConfig := filepath.Join(homeDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(codexConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexConfig, []byte("model = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentCodex
	scope := types.ScopeUser
	runtime := &types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
		Agent:       &agent,
		Scope:       &scope,
	}
	state, err := snapshot.CaptureCurrentState(runtime, "pre-apply-codex-20260629-020000-000000000")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		t.Fatal(err)
	}

	status, err := baseline.BuildStatus(types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	codex := findAgentStatus(t, status, types.AgentCodex)
	if codex.HasBaseline {
		t.Fatalf("pre-apply restore point should not count as baseline: %#v", codex)
	}
}

func TestBuildStatusCountsOmittedContent(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeSandbox(t)
	configPath := filepath.Join(homeDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("api_key = \"secret\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := baseline.BuildStatus(types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	codex := findAgentStatus(t, status, types.AgentCodex)
	if codex.OmittedContentCount == 0 {
		t.Fatalf("expected omitted content count: %#v", codex)
	}
}
