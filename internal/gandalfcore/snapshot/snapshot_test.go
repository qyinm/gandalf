package snapshot_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func makeSandbox(t *testing.T) (projectPath, homeDir, storeDir string) {
	t.Helper()
	root := t.TempDir()
	projectPath = filepath.Join(root, "project")
	homeDir = filepath.Join(root, "home")
	storeDir = filepath.Join(homeDir, ".gandalf")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return projectPath, homeDir, storeDir
}

func TestCaptureCurrentStateProducesManifestWithSchemaVersionAndRFC3339CreatedAt(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeSandbox(t)
	agent := types.AgentCodex
	scope := types.ScopeUser

	state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
		Agent:       &agent,
		Scope:       &scope,
	}, "baseline")
	if err != nil {
		t.Fatalf("CaptureCurrentState: %v", err)
	}
	if state.Snapshot.Manifest.SchemaVersion != "0.1" {
		t.Fatalf("schema version = %q", state.Snapshot.Manifest.SchemaVersion)
	}
	if _, err := time.Parse(time.RFC3339, state.Snapshot.Manifest.CreatedAt); err != nil {
		t.Fatalf("createdAt not RFC3339: %q (%v)", state.Snapshot.Manifest.CreatedAt, err)
	}
	if state.Snapshot.Manifest.Security.RedactionPolicy != "metadata-only" {
		t.Fatalf("redaction policy = %q", state.Snapshot.Manifest.Security.RedactionPolicy)
	}
	if len(state.Snapshot.Content) != 0 {
		t.Fatalf("metadata-only snapshot should have empty content index, got %#v", state.Snapshot.Content)
	}
}

func TestCapturesClaudeUserGlobalSettingsWithContentBacking(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeSandbox(t)
	claudeDir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"permissions":{"allow":["Bash(echo hi)"]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentClaudeCode
	scope := types.ScopeUser
	state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		Agent:          &agent,
		Scope:          &scope,
		CaptureContent: true,
	}, "claude-baseline")
	if err != nil {
		t.Fatalf("CaptureCurrentState: %v", err)
	}

	foundContent := false
	for _, entry := range state.Snapshot.Content {
		if entry.SourcePath == "~/.claude/settings.json" && entry.CaptureStatus == "captured" {
			foundContent = true
			if entry.Content == nil || *entry.Content != settings {
				t.Fatalf("content = %#v", entry.Content)
			}
			if entry.Checksum == "" || entry.StoragePath == "" {
				t.Fatalf("missing checksum/storage path: %#v", entry)
			}
		}
	}
	if !foundContent {
		t.Fatal("expected Claude settings.json content capture")
	}

	var settingsEvidence *types.DiscoveredItem
	for i := range state.Scan.Evidence {
		item := &state.Scan.Evidence[i]
		if item.Agent == types.AgentClaudeCode &&
			item.Kind == types.KindAgentConfig &&
			item.SourcePath == "~/.claude/settings.json" {
			settingsEvidence = item
			break
		}
	}
	if settingsEvidence == nil {
		t.Fatal("settings evidence not found")
	}
	if len(settingsEvidence.Metadata) == 0 {
		t.Fatal("expected metadata")
	}
	var meta map[string]any
	if err := json.Unmarshal(settingsEvidence.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["contentCaptureStatus"] != "captured" {
		t.Fatalf("contentCaptureStatus = %#v", meta["contentCaptureStatus"])
	}
}

func TestSkipsClaudeJSONMetadataOnlyFromContentCapture(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeSandbox(t)
	if err := os.WriteFile(
		filepath.Join(homeDir, ".claude.json"),
		[]byte(`{"mcpServers":{"docs":{"command":"npx"}}}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentClaudeCode
	scope := types.ScopeUser
	state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		Agent:          &agent,
		Scope:          &scope,
		CaptureContent: true,
	}, "claude-json")
	if err != nil {
		t.Fatalf("CaptureCurrentState: %v", err)
	}
	if len(state.Snapshot.Content) != 0 {
		t.Fatalf("~/.claude.json must stay metadata-only, got %#v", state.Snapshot.Content)
	}
}
