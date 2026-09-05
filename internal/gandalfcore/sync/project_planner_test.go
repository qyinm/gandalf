package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestCreateProjectSyncPlan_CreatesMissingConfigsAndCheckPasses(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "project")
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "project-apply",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCodex, types.AgentCursor},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {
				Command: "echo",
				Args:    []string{"${APP_ENV}"},
			},
		},
		EnvTemplate: map[string]string{
			"APP_ENV": "production",
		},
	}

	plan, err := CreateProjectSyncPlan(m, projectRoot, homeDir)
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}
	if len(plan.Items) != 3 {
		t.Fatalf("expected 3 project config items, got %d", len(plan.Items))
	}

	for _, item := range plan.Items {
		if !strings.HasPrefix(item.TargetFile, projectRoot) {
			t.Errorf("project plan must target project root, got %s", item.TargetFile)
		}
		if strings.Contains(item.Content, "production") {
			t.Errorf("project plan must keep ${APP_ENV} uninterpolated, got:\n%s", item.Content)
		}
		if !strings.Contains(item.Content, "${APP_ENV}") {
			t.Errorf("expected ${APP_ENV} template in project content:\n%s", item.Content)
		}
	}

	roots := &pathconfinement.Roots{HomeDir: homeDir, ProjectPath: projectRoot}
	result, err := ApplySyncPlan(plan, roots, "")
	if err != nil {
		t.Fatalf("ApplySyncPlan: %v", err)
	}
	if !result.Success {
		t.Fatalf("apply failed: %v", result.Errors)
	}

	if _, err := os.Stat(filepath.Join(homeDir, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("project-only apply must not write user-home Claude settings")
	}

	report, err := DetectProjectDrift(m, projectRoot)
	if err != nil {
		t.Fatalf("DetectProjectDrift: %v", err)
	}
	if !report.InSync {
		t.Fatalf("expected InSync after project apply, items: %+v", report.Items)
	}
}

func TestCreateSyncPlan_DoesNotCoverProjectMCPFiles(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "project")
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "home-only",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {Command: "echo"},
		},
	}

	plan, err := CreateSyncPlan(m, projectRoot, homeDir, nil)
	if err != nil {
		t.Fatalf("CreateSyncPlan: %v", err)
	}

	roots := &pathconfinement.Roots{HomeDir: homeDir, ProjectPath: projectRoot}
	result, err := ApplySyncPlan(plan, roots, "")
	if err != nil {
		t.Fatalf("ApplySyncPlan: %v", err)
	}
	if !result.Success {
		t.Fatalf("home apply failed: %v", result.Errors)
	}

	if _, err := os.Stat(filepath.Join(projectRoot, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("default home apply must not create project .mcp.json")
	}

	report, err := DetectProjectDrift(m, projectRoot)
	if err != nil {
		t.Fatalf("DetectProjectDrift: %v", err)
	}
	if report.InSync {
		t.Fatal("project-only check must still report missing project MCP after home-only apply")
	}
}

func TestCreateProjectSyncPlan_PreservesUnmanagedProjectServers(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(`{
  "mcpServers": {
    "personal-db": {"command": "sqlite3"}
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "preserve",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {Command: "echo"},
		},
	}

	plan, err := CreateProjectSyncPlan(m, tempDir, filepath.Join(tempDir, "home"))
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(plan.Items))
	}
	if !strings.Contains(plan.Items[0].Content, "personal-db") {
		t.Errorf("expected personal-db to be preserved, got:\n%s", plan.Items[0].Content)
	}
	if !strings.Contains(plan.Items[0].Content, "team-echo") {
		t.Errorf("expected team-echo to be added, got:\n%s", plan.Items[0].Content)
	}
}

func TestCreateProjectSyncPlan_EmptyServersIsNoop(t *testing.T) {
	tempDir := t.TempDir()
	m := &manifest.Manifest{
		Version:    "1.0",
		Name:       "empty",
		Agents:     []types.AgentID{types.AgentClaudeCode, types.AgentCursor, types.AgentCodex},
		MCPServers: map[string]manifest.MCPServerDef{},
	}

	plan, err := CreateProjectSyncPlan(m, tempDir, tempDir)
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}
	if len(plan.Items) != 0 {
		t.Fatalf("expected no project writes when no servers are declared, got %d", len(plan.Items))
	}
}
