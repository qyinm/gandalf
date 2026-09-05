package plugins

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

const cursorCLIConfigJSON = `{
  "version": 1,
  "editor": {"vimMode": false},
  "permissions": {
    "allow": ["Shell(ls)", "Shell(echo)"],
    "deny": ["Shell(rm)"]
  },
  "approvalMode": "allowlist"
}`

func TestCursorCLIConfigDiscoversUserCliConfigAndPermissions(t *testing.T) {
	homeDir := t.TempDir()
	writeFile(t, filepath.Join(homeDir, ".cursor/cli-config.json"), cursorCLIConfigJSON)

	evidence := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: t.TempDir(),
		HomeDir:     homeDir,
	})

	config := findCursorItem(t, evidence, types.KindAgentConfig, "~/.cursor/cli-config.json")
	if config.CaptureStatus != types.CaptureCaptured {
		t.Fatalf("cli-config capture = %s", config.CaptureStatus)
	}

	allow := findCursorPermission(t, evidence, "~/.cursor/cli-config.json", "allow")
	if allow.Name == nil || *allow.Name != "Shell(ls),Shell(echo)" {
		t.Fatalf("allow permission name = %#v", allow.Name)
	}
	deny := findCursorPermission(t, evidence, "~/.cursor/cli-config.json", "deny")
	if deny.Name == nil || *deny.Name != "Shell(rm)" {
		t.Fatalf("deny permission name = %#v", deny.Name)
	}
}

func TestCursorCLIConfigDiscoversProjectCliPermissions(t *testing.T) {
	projectPath := t.TempDir()
	writeFile(t, filepath.Join(projectPath, ".cursor/cli.json"), `{
  "permissions": {
    "allow": ["Shell(git status)"],
    "deny": []
  }
}`)
	scope := types.ScopeProject

	evidence := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: projectPath,
		HomeDir:     t.TempDir(),
		Scope:       &scope,
	})

	config := findCursorItem(t, evidence, types.KindAgentConfig, ".cursor/cli.json")
	if config.Scope != types.ScopeProject {
		t.Fatalf("project cli.json scope = %s", config.Scope)
	}
	allow := findCursorPermission(t, evidence, ".cursor/cli.json", "allow")
	if allow.Name == nil || *allow.Name != "Shell(git status)" {
		t.Fatalf("project allow name = %#v", allow.Name)
	}
}

func TestCursorCLIOnlySetupStillEmitsCursorEvidence(t *testing.T) {
	homeDir := t.TempDir()
	writeFile(t, filepath.Join(homeDir, ".cursor/cli-config.json"), cursorCLIConfigJSON)

	evidence := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: t.TempDir(),
		HomeDir:     homeDir,
	})
	if len(evidence) == 0 {
		t.Fatal("CLI-only Cursor setup returned no evidence")
	}
	foundCLI := false
	for _, item := range evidence {
		if item.SourcePath == "~/.cursor/cli-config.json" {
			foundCLI = true
			break
		}
	}
	if !foundCLI {
		t.Fatalf("CLI-only setup missed cli-config.json: %#v", evidence)
	}
}

func TestCursorCLIConfigMalformedJSONEmitsParseFailure(t *testing.T) {
	homeDir := t.TempDir()
	writeFile(t, filepath.Join(homeDir, ".cursor/cli-config.json"), `{"version": 1,`)

	evidence := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: t.TempDir(),
		HomeDir:     homeDir,
	})

	failed := findCursorItem(t, evidence, types.KindAgentConfig, "~/.cursor/cli-config.json")
	if failed.CaptureStatus != types.CaptureParseFailed {
		t.Fatalf("malformed cli-config capture = %s", failed.CaptureStatus)
	}
	for _, item := range evidence {
		if item.Kind == types.KindPermission && item.SourcePath == "~/.cursor/cli-config.json" {
			t.Fatalf("parse-failed CLI config should not emit permissions: %#v", item)
		}
	}
}

func TestCursorCLITargetsAreListedWithEditorMCPTargets(t *testing.T) {
	targets := CursorScanner{}.Targets("/repo", "/home/user")
	got := map[string]bool{}
	for _, target := range targets {
		got[target.SourcePath] = true
	}
	for _, want := range []string{
		".cursor/mcp.json",
		"~/.cursor/mcp.json",
		".cursor/cli.json",
		"~/.cursor/cli-config.json",
	} {
		if !got[want] {
			t.Fatalf("Targets missing %q: %#v", want, got)
		}
	}
}

func findCursorItem(t *testing.T, evidence []types.DiscoveredItem, kind types.EvidenceKind, sourcePath string) types.DiscoveredItem {
	t.Helper()
	for _, item := range evidence {
		if item.Agent == types.AgentCursor && item.Kind == kind && item.SourcePath == sourcePath {
			return item
		}
	}
	t.Fatalf("missing %s %s in %#v", kind, sourcePath, evidence)
	return types.DiscoveredItem{}
}

func findCursorPermission(t *testing.T, evidence []types.DiscoveredItem, sourcePath, key string) types.DiscoveredItem {
	t.Helper()
	for _, item := range evidence {
		if item.Agent != types.AgentCursor || item.Kind != types.KindPermission || item.SourcePath != sourcePath {
			continue
		}
		if permissionKey(item.Metadata) == key {
			return item
		}
	}
	t.Fatalf("missing permission %q from %s in %#v", key, sourcePath, evidence)
	return types.DiscoveredItem{}
}

func permissionKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	key, _ := meta["permissionKey"].(string)
	return key
}
