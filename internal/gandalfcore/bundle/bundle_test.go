package bundle_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/bundle"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/tar"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type sandbox struct {
	storeDir    string
	projectPath string
	homeDir     string
	root        string
}

func makeSandbox(t *testing.T) sandbox {
	t.Helper()
	root := t.TempDir()
	sb := sandbox{
		root:        root,
		storeDir:    filepath.Join(root, "store"),
		projectPath: filepath.Join(root, "project"),
		homeDir:     filepath.Join(root, "home"),
	}
	for _, dir := range []string{sb.storeDir, sb.projectPath, sb.homeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return sb
}

func sampleSnapshot(name string) types.Snapshot {
	return types.Snapshot{
		Manifest: types.SnapshotManifest{
			SchemaVersion: "0.1",
			Name:          name,
			CreatedAt:     "2026-05-12T00:00:00.000Z",
			ProjectPath:   "/tmp/project",
			Security: types.SnapshotSecurity{
				RawSecretsIncluded: false,
				RedactionPolicy:    "metadata-only",
			},
		},
		Evidence: []types.DiscoveredItem{{
			ID: "project.claude-code..mcp.json.mcp-github", Agent: types.AgentClaudeCode,
			Kind: types.KindMcpServer, SourcePath: ".mcp.json", Scope: types.ScopeProject,
			Precedence: 40, Parser: types.ParserJSON, Sensitivity: "command_config",
			ContentPolicy: "structured_safe_fields_only", RestorePolicy: types.RestoreNotSupported,
			CaptureStatus: types.CaptureCaptured, Confidence: types.ConfidenceHigh,
			Name: stringPtr("github"),
		}},
		Graph: []types.GraphNode{{
			ID: "node.project.claude-code.mcp_server.github", Agent: types.AgentClaudeCode,
			Scope: types.ScopeProject, SourcePath: ".mcp.json", EntityKind: types.KindMcpServer,
			EntityName: "github", Confidence: types.ConfidenceHigh,
			EvidenceID: "project.claude-code..mcp.json.mcp-github",
		}},
		AuditFindings: []types.AuditFinding{{
			Code: "EXECUTABLE_CONFIG_ADDED", Severity: types.SeverityHigh,
			Problem: "MCP server references an executable command.",
			Cause:   ".mcp.json github: command = gh.", Fix: "Confirm the command is trusted.",
		}},
		Provenance: []types.ProvenanceEntry{{
			NodeID:     "node.project.claude-code.mcp_server.github",
			EvidenceID: "project.claude-code..mcp.json.mcp-github", SourcePath: ".mcp.json",
			Scope: types.ScopeProject, Precedence: 40, Confidence: types.ConfidenceHigh,
			CaptureStatus: types.CaptureCaptured,
		}},
	}
}

func stringPtr(value string) *string { return &value }

func TestExportImportRoundTripPreservesEvidence(t *testing.T) {
	t.Parallel()
	sb := makeSandbox(t)
	name := "roundtrip-test"
	if err := store.WriteSnapshot(sb.storeDir, store.StoreSnapshotFrom(sampleSnapshot(name)), nil); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(sb.root, name+".gandalf")
	result, err := bundle.Export(&types.BundleExportOptions{
		SnapshotName: name, OutputPath: out, StoreDir: sb.storeDir,
		ProjectPath: sb.projectPath, HomeDir: sb.homeDir,
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if result.Checksum == "" {
		t.Fatal("expected checksum")
	}
	importResult, err := bundle.Import(&types.BundleImportOptions{
		BundlePath: out, StoreDir: sb.storeDir, ProjectPath: sb.projectPath, HomeDir: sb.homeDir,
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if importResult.SnapshotName != name || importResult.EvidenceCount != 1 {
		t.Fatalf("import result = %#v", importResult)
	}
	imported, err := store.ReadSnapshot(sb.storeDir, name, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Evidence) != 1 || imported.Evidence[0].SourcePath != ".mcp.json" {
		t.Fatalf("imported evidence = %#v", imported.Evidence)
	}
}

func TestHomePathsStoredAsHomeTokenAndResolvedOnImport(t *testing.T) {
	t.Parallel()
	sb := makeSandbox(t)
	name := "home-abstraction"
	settingsPath := filepath.Join(sb.homeDir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := sampleSnapshot(name)
	snap.Evidence[0].Kind = types.KindAgentConfig
	snap.Evidence[0].SourcePath = settingsPath
	snap.Evidence[0].RestorePolicy = types.RestoreFullContent
	snap.Graph[0].SourcePath = settingsPath
	snap.Graph[0].EntityKind = types.KindAgentConfig
	snap.Provenance[0].SourcePath = settingsPath
	if err := store.WriteSnapshot(sb.storeDir, store.StoreSnapshotFrom(snap), nil); err != nil {
		t.Fatal(err)
	}
	includeContent := true
	out := filepath.Join(sb.root, name+".gandalf")
	if _, err := bundle.Export(&types.BundleExportOptions{
		SnapshotName: name, OutputPath: out, StoreDir: sb.storeDir,
		ProjectPath: sb.projectPath, HomeDir: sb.homeDir, IncludeContent: &includeContent,
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	entries, _, err := tar.ReadTar(out)
	if err != nil {
		t.Fatal(err)
	}
	var bundledEvidence []types.DiscoveredItem
	for _, entry := range entries {
		if entry.Path == "snapshot/evidence.json" {
			if err := json.Unmarshal(entry.Content, &bundledEvidence); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if len(bundledEvidence) == 0 || bundledEvidence[0].SourcePath != "{home}/.claude/settings.json" {
		t.Fatalf("bundled evidence = %#v", bundledEvidence)
	}
	importBox := makeSandbox(t)
	if _, err := bundle.Import(&types.BundleImportOptions{
		BundlePath: out, StoreDir: importBox.storeDir, ProjectPath: importBox.projectPath, HomeDir: importBox.homeDir,
	}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	imported, err := store.ReadSnapshot(importBox.storeDir, name, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(importBox.homeDir, ".claude", "settings.json")
	if imported.Evidence[0].SourcePath != expected {
		t.Fatalf("imported path = %q want %q", imported.Evidence[0].SourcePath, expected)
	}
}

func TestSignsBundleWithHMACSHA256(t *testing.T) {
	t.Parallel()
	sb := makeSandbox(t)
	name := "signed-bundle"
	if err := store.WriteSnapshot(sb.storeDir, store.StoreSnapshotFrom(sampleSnapshot(name)), nil); err != nil {
		t.Fatal(err)
	}
	key := "test-secret"
	out := filepath.Join(sb.root, name+".gandalf")
	if _, err := bundle.Export(&types.BundleExportOptions{
		SnapshotName: name, OutputPath: out, StoreDir: sb.storeDir,
		ProjectPath: sb.projectPath, HomeDir: sb.homeDir, SignatureKey: &key,
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	inspect, err := bundle.Inspect(out)
	if err != nil {
		t.Fatal(err)
	}
	if !inspect.IsSigned || inspect.SignatureAlgorithm == nil || *inspect.SignatureAlgorithm != "HMAC-SHA256" {
		t.Fatalf("inspect = %#v", inspect)
	}
	verify, err := bundle.Verify(&types.BundleVerifyOptions{BundlePath: out, SignatureKey: &key})
	if err != nil || !verify.Valid {
		t.Fatalf("verify = %#v err=%v", verify, err)
	}
}
