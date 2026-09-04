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
	mcpJSON := `{"mcpServers": {"pg-srv": {"command": "npx", "args": ["-y", "mcp-pg"]}}}`
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
