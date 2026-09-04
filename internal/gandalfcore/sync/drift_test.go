package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestDetectProjectDrift_InSync(t *testing.T) {
	tempDir := t.TempDir()

	// Create skill directory with SKILL.md
	skillDir := filepath.Join(tempDir, ".gandalf", "skills", "code-review")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Code Review"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create matching .mcp.json
	mcpJSON := `{"mcpServers": {"pg-srv": {"command": "npx", "args": ["-y", "mcp-pg", "${PG_HOST}"]}}}`
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Name:    "team-env",
		Version: "1.0",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCursor},
		MCPServers: map[string]manifest.MCPServerDef{
			"pg-srv": {
				Command: "npx",
				Args:    []string{"-y", "mcp-pg", "${PG_HOST}"},
			},
		},
		Skills: []manifest.SkillDef{
			{Name: "code-review", Source: ".gandalf/skills/code-review"},
		},
		EnvTemplate: map[string]string{
			"PG_HOST": "localhost:5432",
		},
	}

	report, err := DetectProjectDrift(m, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift failed: %v", err)
	}

	if !report.InSync {
		t.Errorf("expected InSync=true, got false with items: %+v", report.Items)
	}
}

func TestDetectProjectDrift_ChangedServerSettings(t *testing.T) {
	tempDir := t.TempDir()

	// Same server name "pg-srv", but modified command from "npx" to "node"
	mcpJSON := `{"mcpServers": {"pg-srv": {"command": "node", "args": ["server.js"]}}}`
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Name:    "team-env",
		Version: "1.0",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		MCPServers: map[string]manifest.MCPServerDef{
			"pg-srv": {
				Command: "npx",
				Args:    []string{"-y", "mcp-pg"},
			},
		},
	}

	report, err := DetectProjectDrift(m, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift failed: %v", err)
	}

	if report.InSync {
		t.Errorf("expected InSync=false for modified server settings")
	}

	foundModified := false
	for _, item := range report.Items {
		if item.Kind == DriftOutdatedConfig && item.Name == "pg-srv" {
			foundModified = true
			break
		}
	}
	if !foundModified {
		t.Errorf("expected DriftOutdatedConfig for modified pg-srv, got: %+v", report.Items)
	}
}

func TestDetectProjectDrift_MalformedProjectJSON(t *testing.T) {
	tempDir := t.TempDir()

	// Invalid broken JSON syntax
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte("{broken json"), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Name:    "team-env",
		Version: "1.0",
		Agents:  []types.AgentID{types.AgentClaudeCode},
	}

	report, err := DetectProjectDrift(m, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift failed: %v", err)
	}

	if report.InSync {
		t.Errorf("expected InSync=false for malformed JSON")
	}

	foundMalformed := false
	for _, item := range report.Items {
		if item.Kind == DriftOutdatedConfig && item.TargetFile == ".mcp.json" {
			foundMalformed = true
			break
		}
	}
	if !foundMalformed {
		t.Errorf("expected DriftOutdatedConfig for malformed .mcp.json, got: %+v", report.Items)
	}
}

func TestDetectProjectDrift_CodexProjectConfig(t *testing.T) {
	tempDir := t.TempDir()

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Codex config with missing server
	codexTOML := `
[mcp_servers.srv1]
command = "npx"
args = ["srv1"]
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Name:    "codex-env",
		Version: "1.0",
		Agents:  []types.AgentID{types.AgentCodex},
		MCPServers: map[string]manifest.MCPServerDef{
			"srv1": {Command: "npx", Args: []string{"srv1"}},
			"srv2": {Command: "npx", Args: []string{"srv2"}},
		},
	}

	report, err := DetectProjectDrift(m, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift failed: %v", err)
	}

	if report.InSync {
		t.Errorf("expected InSync=false due to missing srv2 in Codex config")
	}

	foundMissingSrv2 := false
	for _, item := range report.Items {
		if item.Kind == DriftUnsyncedProjectConfig && item.Name == "srv2" && item.TargetFile == filepath.Join(".codex", "config.toml") {
			foundMissingSrv2 = true
			break
		}
	}
	if !foundMissingSrv2 {
		t.Errorf("expected DriftUnsyncedProjectConfig for srv2 in Codex config, got: %+v", report.Items)
	}
}

func TestDetectProjectDrift_MissingEnvTemplate(t *testing.T) {
	tempDir := t.TempDir()

	m := &manifest.Manifest{
		Name:    "team-env",
		Version: "1.0",
		Agents:  []types.AgentID{types.AgentCodex},
		MCPServers: map[string]manifest.MCPServerDef{
			"db-srv": {
				Command: "run-db",
				Args:    []string{"--token=${SECRET_TOKEN}"},
			},
		},
		EnvTemplate: map[string]string{
			// SECRET_TOKEN is deliberately missing
		},
	}

	report, err := DetectProjectDrift(m, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift failed: %v", err)
	}

	if report.InSync {
		t.Errorf("expected InSync=false due to missing env_template")
	}

	foundMissingEnv := false
	for _, item := range report.Items {
		if item.Kind == DriftMissingEnvTemplate && item.Name == "SECRET_TOKEN" {
			foundMissingEnv = true
			break
		}
	}
	if !foundMissingEnv {
		t.Errorf("expected DriftMissingEnvTemplate for SECRET_TOKEN, got: %+v", report.Items)
	}
}

func TestDetectProjectDrift_MissingSkillAndSkillMD(t *testing.T) {
	tempDir := t.TempDir()

	// Skill 1: directory missing completely
	// Skill 2: directory exists, but SKILL.md missing
	skill2Dir := filepath.Join(tempDir, ".gandalf", "skills", "partial-skill")
	if err := os.MkdirAll(skill2Dir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Name:    "team-env",
		Version: "1.0",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		Skills: []manifest.SkillDef{
			{Name: "non-existent", Source: ".gandalf/skills/non-existent"},
			{Name: "partial-skill", Source: ".gandalf/skills/partial-skill"},
		},
	}

	report, err := DetectProjectDrift(m, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift failed: %v", err)
	}

	if report.InSync {
		t.Errorf("expected InSync=false")
	}

	hasMissingDir := false
	hasMissingFile := false
	for _, item := range report.Items {
		if item.Kind == DriftMissingSkill && item.Name == "non-existent" {
			hasMissingDir = true
		}
		if item.Kind == DriftMissingSkillFile && item.Name == "partial-skill" {
			hasMissingFile = true
		}
	}

	if !hasMissingDir {
		t.Errorf("expected DriftMissingSkill for non-existent, got: %+v", report.Items)
	}
	if !hasMissingFile {
		t.Errorf("expected DriftMissingSkillFile for partial-skill, got: %+v", report.Items)
	}
}

func TestDetectProjectDrift_UnsyncedProjectConfig(t *testing.T) {
	tempDir := t.TempDir()

	// .mcp.json has srv-1, but gandalf.toml has srv-1 and srv-2
	mcpJSON := `{"mcpServers": {"srv-1": {"command": "cmd1"}}}`
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Name:    "team-env",
		Version: "1.0",
		Agents:  []types.AgentID{types.AgentCursor},
		MCPServers: map[string]manifest.MCPServerDef{
			"srv-1": {Command: "cmd1"},
			"srv-2": {Command: "cmd2"},
		},
	}

	report, err := DetectProjectDrift(m, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift failed: %v", err)
	}

	if report.InSync {
		t.Errorf("expected InSync=false for unsynced project config")
	}

	foundUnsynced := false
	for _, item := range report.Items {
		if item.Kind == DriftUnsyncedProjectConfig && item.Name == "srv-2" {
			foundUnsynced = true
			break
		}
	}
	if !foundUnsynced {
		t.Errorf("expected DriftUnsyncedProjectConfig for srv-2, got: %+v", report.Items)
	}
}
