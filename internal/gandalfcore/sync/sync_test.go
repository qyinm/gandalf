package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestMergeClaudeSettingsJSON(t *testing.T) {
	existing := `{
  "theme": "dark",
  "mcpServers": {
    "my-personal-db": {
      "command": "sqlite3",
      "args": ["mydb.sqlite"]
    }
  }
}`

	m := &manifest.Manifest{
		MCPServers: map[string]manifest.MCPServerDef{
			"team-postgres": {
				Command: "npx",
				Args:    []string{"-y", "@mcp/postgres", "pg://localhost"},
				Env:     map[string]string{"ENV": "test"},
			},
		},
	}

	merged, err := MergeClaudeSettingsJSON(existing, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(merged), &parsed); err != nil {
		t.Fatalf("failed to parse merged json: %v", err)
	}

	if parsed["theme"] != "dark" {
		t.Errorf("expected theme to be preserved, got %v", parsed["theme"])
	}

	mcpServers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers map, got %v", parsed["mcpServers"])
	}

	if _, ok := mcpServers["my-personal-db"]; !ok {
		t.Errorf("expected personal db to be preserved")
	}

	teamSrv, ok := mcpServers["team-postgres"].(map[string]any)
	if !ok {
		t.Errorf("expected team-postgres to be added")
	} else if teamSrv["command"] != "npx" {
		t.Errorf("expected command npx, got %v", teamSrv["command"])
	}

	// Test null JSON does not panic
	mergedNull, err := MergeClaudeSettingsJSON("null", m)
	if err != nil {
		t.Fatalf("unexpected error merging null json: %v", err)
	}
	if !strings.Contains(mergedNull, "team-postgres") {
		t.Errorf("expected team-postgres in merged null JSON")
	}
}

func TestMergeCodexConfigTOML(t *testing.T) {
	existing := `
# User personal config
model = "gpt-5-turbo"

[mcp_servers.personal-sqlite]
command = "sqlite3"
args = ["test.db"]
`

	m := &manifest.Manifest{
		MCPServers: map[string]manifest.MCPServerDef{
			"team-api": {
				Command: "node",
				Args:    []string{"server.js"},
			},
		},
	}

	merged, err := MergeCodexConfigTOML(existing, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(merged, "model = \"gpt-5-turbo\"") {
		t.Errorf("expected personal config to be preserved")
	}
	if !strings.Contains(merged, "[mcp_servers.personal-sqlite]") {
		t.Errorf("expected personal sqlite to be preserved")
	}
	if !strings.Contains(merged, "[mcp_servers.team-api]") {
		t.Errorf("expected team-api to be added")
	}
	if !strings.Contains(merged, "command = \"node\"") {
		t.Errorf("expected command = node")
	}
}

func TestMergeCursorMCPJSON(t *testing.T) {
	existing := `{
  "customKey": true,
  "mcpServers": {
    "my-personal-tool": {
      "command": "python",
      "args": ["tool.py"]
    }
  }
}`

	m := &manifest.Manifest{
		MCPServers: map[string]manifest.MCPServerDef{
			"team-postgres": {
				Command: "npx",
				Args:    []string{"-y", "@mcp/postgres", "pg://localhost"},
				Env:     map[string]string{"ENV": "test"},
			},
		},
	}

	merged, err := MergeCursorMCPJSON(existing, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(merged), &parsed); err != nil {
		t.Fatalf("failed to parse merged json: %v", err)
	}

	if parsed["customKey"] != true {
		t.Errorf("expected customKey to be preserved, got %v", parsed["customKey"])
	}

	mcpServers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers map, got %v", parsed["mcpServers"])
	}

	if _, ok := mcpServers["my-personal-tool"]; !ok {
		t.Errorf("expected personal tool to be preserved")
	}

	teamSrv, ok := mcpServers["team-postgres"].(map[string]any)
	if !ok {
		t.Errorf("expected team-postgres to be added")
	} else if teamSrv["command"] != "npx" {
		t.Errorf("expected command npx, got %v", teamSrv["command"])
	}

	// Test null JSON does not panic
	mergedNull, err := MergeCursorMCPJSON("null", m)
	if err != nil {
		t.Fatalf("unexpected error merging null json for cursor: %v", err)
	}
	if !strings.Contains(mergedNull, "team-postgres") {
		t.Errorf("expected team-postgres in merged null JSON for cursor")
	}
}

func TestEndToEndSyncPlanAndApply(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	projectRoot := filepath.Join(tempDir, "project")
	storeDir := filepath.Join(tempDir, "store")

	_ = os.MkdirAll(homeDir, 0755)
	_ = os.MkdirAll(projectRoot, 0755)
	_ = os.MkdirAll(storeDir, 0755)

	// Create a dummy skill in project root
	skillDir := filepath.Join(projectRoot, ".gandalf", "skills", "reviewer")
	_ = os.MkdirAll(skillDir, 0755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Reviewer Skill"), 0644)

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "e2e-team",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCodex, types.AgentCursor},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-postgres": {
				Command: "npx",
				Args:    []string{"-y", "@mcp/postgres"},
			},
		},
		Skills: []manifest.SkillDef{
			{
				Name:   "reviewer",
				Source: "./.gandalf/skills/reviewer",
			},
		},
	}

	plan, err := CreateSyncPlan(m, projectRoot, homeDir, nil)
	if err != nil {
		t.Fatalf("unexpected error creating sync plan: %v", err)
	}

	if len(plan.Items) != 6 { // 3 agent configs + 3 skills
		t.Fatalf("expected 6 sync items, got %d", len(plan.Items))
	}

	roots := &pathconfinement.Roots{
		HomeDir:     homeDir,
		ProjectPath: projectRoot,
	}

	res, err := ApplySyncPlan(plan, roots, storeDir)
	if err != nil {
		t.Fatalf("unexpected error applying sync plan: %v", err)
	}

	if !res.Success {
		t.Fatalf("apply failed with errors: %v", res.Errors)
	}

	// Verify files written
	claudeSettings := filepath.Join(homeDir, ".claude", "settings.json")
	if _, err := os.Stat(claudeSettings); err != nil {
		t.Errorf("claude settings.json was not created: %v", err)
	}

	codexConfig := filepath.Join(homeDir, ".codex", "config.toml")
	if _, err := os.Stat(codexConfig); err != nil {
		t.Errorf("codex config.toml was not created: %v", err)
	}

	cursorConfig := filepath.Join(homeDir, ".cursor", "mcp.json")
	if _, err := os.Stat(cursorConfig); err != nil {
		t.Errorf("cursor mcp.json was not created: %v", err)
	}

	claudeSkill := filepath.Join(homeDir, ".claude", "skills", "reviewer", "SKILL.md")
	if _, err := os.Stat(claudeSkill); err != nil {
		t.Errorf("claude skill was not copied: %v", err)
	}

	codexSkill := filepath.Join(homeDir, ".codex", "skills", "reviewer", "SKILL.md")
	if _, err := os.Stat(codexSkill); err != nil {
		t.Errorf("codex skill was not copied: %v", err)
	}

	cursorSkill := filepath.Join(homeDir, ".cursor", "skills", "reviewer", "SKILL.md")
	if _, err := os.Stat(cursorSkill); err != nil {
		t.Errorf("cursor skill was not copied: %v", err)
	}
}
