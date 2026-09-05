package plugins

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/audit"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/graph"
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
	if allow.Name == nil || *allow.Name != "allow" {
		t.Fatalf("allow permission name = %#v", allow.Name)
	}
	if ruleDisplay(allow.Metadata) != "Shell(ls),Shell(echo)" {
		t.Fatalf("allow ruleDisplay = %q", ruleDisplay(allow.Metadata))
	}
	deny := findCursorPermission(t, evidence, "~/.cursor/cli-config.json", "deny")
	if deny.Name == nil || *deny.Name != "deny" {
		t.Fatalf("deny permission name = %#v", deny.Name)
	}
	if ruleDisplay(deny.Metadata) != "Shell(rm)" {
		t.Fatalf("deny ruleDisplay = %q", ruleDisplay(deny.Metadata))
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
	if allow.Name == nil || *allow.Name != "allow" {
		t.Fatalf("project allow name = %#v", allow.Name)
	}
	if ruleDisplay(allow.Metadata) != "Shell(git status)" {
		t.Fatalf("project allow ruleDisplay = %q", ruleDisplay(allow.Metadata))
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

func TestCursorCLIEqualAllowDenyStayDistinctAndTrackOneKeyChange(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()
	writeFile(t, filepath.Join(homeDir, ".cursor/cli-config.json"), `{
  "version": 1,
  "editor": {"vimMode": false},
  "permissions": {
    "allow": ["Shell(ls)"],
    "deny": ["Shell(ls)"]
  }
}`)
	baseline := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
	})
	baselineAllow := findCursorPermission(t, baseline, "~/.cursor/cli-config.json", "allow")
	baselineDeny := findCursorPermission(t, baseline, "~/.cursor/cli-config.json", "deny")
	if baselineAllow.Name == nil || baselineDeny.Name == nil || *baselineAllow.Name == *baselineDeny.Name {
		t.Fatalf("equal allow/deny lists must keep distinct names: allow=%#v deny=%#v", baselineAllow.Name, baselineDeny.Name)
	}

	writeFile(t, filepath.Join(homeDir, ".cursor/cli-config.json"), `{
  "version": 1,
  "editor": {"vimMode": false},
  "permissions": {
    "allow": ["Shell(ls)"],
    "deny": ["Shell(rm)"]
  }
}`)
	current := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
	})

	graphDiff := diff.DiffGraphs(graph.BuildGraph(baseline), graph.BuildGraph(current))
	var denyChanges, allowChanges int
	for _, change := range graphDiff.SemanticChanges {
		if change.EntityKind != types.KindPermission {
			continue
		}
		switch change.EntityName {
		case "deny":
			denyChanges++
			if change.Code != diff.SemanticPermissionChanged {
				t.Fatalf("deny change code = %s", change.Code)
			}
		case "allow":
			allowChanges++
		}
	}
	if denyChanges != 1 {
		t.Fatalf("expected one deny permission change, got %d in %#v", denyChanges, graphDiff.SemanticChanges)
	}
	if allowChanges != 0 {
		t.Fatalf("allow should stay stable, got %d allow changes in %#v", allowChanges, graphDiff.SemanticChanges)
	}
}

func TestCursorCLIWildcardPermissionsAreAuditedAndDiffed(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()
	writeFile(t, filepath.Join(homeDir, ".cursor/cli-config.json"), `{
  "version": 1,
  "editor": {"vimMode": false},
  "permissions": {
    "allow": ["Shell(ls)"],
    "deny": ["Shell(rm)"]
  }
}`)
	baseline := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
	})
	writeFile(t, filepath.Join(homeDir, ".cursor/cli-config.json"), `{
  "version": 1,
  "editor": {"vimMode": false},
  "permissions": {
    "allow": ["Shell(*)"],
    "deny": ["Read(*)"]
  }
}`)
	current := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
	})

	allow := findCursorPermission(t, current, "~/.cursor/cli-config.json", "allow")
	if allow.Name == nil || *allow.Name != "allow" {
		t.Fatalf("wildcard must keep stable allow name, got %#v", allow.Name)
	}
	deny := findCursorPermission(t, current, "~/.cursor/cli-config.json", "deny")
	if deny.Name == nil || *deny.Name != "deny" {
		t.Fatalf("wildcard must keep stable deny name, got %#v", deny.Name)
	}

	findings := audit.AuditEvidence(current, graph.BuildGraph(current))
	wildcardFindings := 0
	for _, finding := range findings {
		if finding.Code == "PERMISSION_WILDCARD_ADDED" {
			wildcardFindings++
		}
	}
	if wildcardFindings < 2 {
		t.Fatalf("expected wildcard audit for allow and deny, got %d in %#v", wildcardFindings, findings)
	}

	graphDiff := diff.DiffGraphs(graph.BuildGraph(baseline), graph.BuildGraph(current))
	wildcardChanges := 0
	for _, change := range graphDiff.SemanticChanges {
		if change.Code == diff.SemanticPermissionWildcardAdded &&
			(change.EntityName == "allow" || change.EntityName == "deny") {
			wildcardChanges++
		}
	}
	if wildcardChanges != 2 {
		t.Fatalf("expected wildcard diffs for allow and deny, got %d in %#v", wildcardChanges, graphDiff.SemanticChanges)
	}
}

func TestCursorCLIConfigRedactsSecretLikeFields(t *testing.T) {
	homeDir := t.TempDir()
	writeFile(t, filepath.Join(homeDir, ".cursor/cli-config.json"), `{
  "version": 1,
  "editor": {"vimMode": false},
  "permissions": {"allow": ["Shell(ls)"], "deny": []},
  "apiKey": "sk-secret-value",
  "headers": {"Authorization": "Bearer leaked-token"}
}`)

	evidence := CursorScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: t.TempDir(),
		HomeDir:     homeDir,
	})
	serialized, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "sk-secret-value") || strings.Contains(string(serialized), "leaked-token") {
		t.Fatalf("CLI evidence leaked secrets: %s", serialized)
	}

	config := findCursorItem(t, evidence, types.KindAgentConfig, "~/.cursor/cli-config.json")
	var value map[string]any
	if err := json.Unmarshal(config.Value, &value); err != nil {
		t.Fatal(err)
	}
	if value["apiKey"] != "[redacted]" {
		t.Fatalf("apiKey = %#v", value["apiKey"])
	}
	headers, _ := value["headers"].(map[string]any)
	if headers["Authorization"] != "[redacted]" {
		t.Fatalf("Authorization = %#v", headers["Authorization"])
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
	return metadataString(raw, "permissionKey")
}

func ruleDisplay(raw json.RawMessage) string {
	return metadataString(raw, "ruleDisplay")
}

func metadataString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	value, _ := meta[key].(string)
	return value
}
