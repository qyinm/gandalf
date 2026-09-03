package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
)

func TestRunImport_UnifiedProjectAndGlobal(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	homeDir := filepath.Join(tempDir, "home")

	if err := os.MkdirAll(filepath.Join(projDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Project .cursor/mcp.json (with a secret database URL)
	cursorMCP := `{
  "mcpServers": {
    "db-service": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres", "postgres://user:supersecret@db.internal:5432/proddb"]
    },
    "shared-tool": {
      "command": "node",
      "args": ["project-version.js"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projDir, ".cursor", "mcp.json"), []byte(cursorMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Global ~/.claude.json (with shared-tool conflict and global-only server)
	claudeJSON := `{
  "mcpServers": {
    "shared-tool": {
      "command": "node",
      "args": ["global-version.js"]
    },
    "global-helper": {
      "command": "python",
      "args": ["helper.py"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(claudeJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Run import
	opts := ImportOptions{
		ProjectPath: projDir,
		HomeDir:     homeDir,
		ProjectOnly: false,
		DryRun:      false,
	}

	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// Verify servers
	if len(res.Manifest.MCPServers) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(res.Manifest.MCPServers))
	}

	// Verify project overrides global
	sharedSrv := res.Manifest.MCPServers["shared-tool"]
	if len(sharedSrv.Args) == 0 || sharedSrv.Args[0] != "project-version.js" {
		t.Errorf("expected project-version.js to override global, got %v", sharedSrv.Args)
	}

	// Verify secret templatization
	dbSrv := res.Manifest.MCPServers["db-service"]
	if len(dbSrv.Args) < 3 || !strings.Contains(dbSrv.Args[2], "${DATABASE_URL}") {
		t.Errorf("expected secret URL to be replaced with ${DATABASE_URL}, got %v", dbSrv.Args)
	}

	if res.Manifest.EnvTemplate["DATABASE_URL"] != "postgres://user:supersecret@db.internal:5432/proddb" {
		t.Errorf("expected env_template to hold original DB URL, got %v", res.Manifest.EnvTemplate["DATABASE_URL"])
	}

	// Verify file was written
	writtenPath := filepath.Join(projDir, "gandalf.toml")
	content, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("gandalf.toml was not written: %v", err)
	}

	parsed, err := manifest.Parse(string(content), nil)
	if err != nil {
		t.Fatalf("written gandalf.toml could not be parsed: %v", err)
	}
	if parsed.Manifest.Name != "proj" {
		t.Errorf("expected manifest name 'proj', got %s", parsed.Manifest.Name)
	}
}

func TestRunImport_ProjectOnlyFlag(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	homeDir := filepath.Join(tempDir, "home")

	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Project standard .mcp.json
	projMCP := `{
  "mcpServers": {
    "proj-tool": {
      "command": "npx",
      "args": ["proj-tool"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projDir, ".mcp.json"), []byte(projMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// Global ~/.cursor/mcp.json
	if err := os.MkdirAll(filepath.Join(homeDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	globalMCP := `{
  "mcpServers": {
    "global-cursor-tool": {
      "command": "npx",
      "args": ["global-cursor-tool"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(homeDir, ".cursor", "mcp.json"), []byte(globalMCP), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		HomeDir:     homeDir,
		ProjectOnly: true,
	}

	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	if len(res.Manifest.MCPServers) != 1 {
		t.Fatalf("expected 1 server with ProjectOnly=true, got %d", len(res.Manifest.MCPServers))
	}
	if _, exists := res.Manifest.MCPServers["proj-tool"]; !exists {
		t.Errorf("expected proj-tool to exist")
	}
	if _, exists := res.Manifest.MCPServers["global-cursor-tool"]; exists {
		t.Errorf("expected global-cursor-tool to be excluded with ProjectOnly=true")
	}
}

func TestRunImport_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")

	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	projMCP := `{
  "mcpServers": {
    "demo": {
      "command": "npx",
      "args": ["demo"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projDir, ".mcp.json"), []byte(projMCP), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
		DryRun:      true,
	}

	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	if res.FormattedTOML == "" {
		t.Errorf("expected formatted TOML in result")
	}

	// Ensure no file was created on disk
	if _, err := os.Stat(filepath.Join(projDir, "gandalf.toml")); !os.IsNotExist(err) {
		t.Errorf("expected gandalf.toml NOT to exist on dry-run")
	}
}

func TestRunImport_WithSkillsAndCodex(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	homeDir := filepath.Join(tempDir, "home")

	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Create a project skill: .cursor/skills/pr-reviewer/SKILL.md
	skillDir := filepath.Join(projDir, ".cursor", "skills", "pr-reviewer")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# PR Reviewer Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create Codex config: .codex/config.toml
	codexDir := filepath.Join(projDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexTOML := `
[mcp_servers.codex-helper]
command = "python"
args = ["helper.py"]
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		HomeDir:     homeDir,
		ProjectOnly: true,
	}

	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	if len(res.Manifest.MCPServers) != 1 {
		t.Errorf("expected 1 mcp server from codex, got %d", len(res.Manifest.MCPServers))
	}
	if _, exists := res.Manifest.MCPServers["codex-helper"]; !exists {
		t.Errorf("expected codex-helper server to exist")
	}

	if len(res.Manifest.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(res.Manifest.Skills))
	} else if res.Manifest.Skills[0].Name != "pr-reviewer" {
		t.Errorf("expected skill 'pr-reviewer', got %s", res.Manifest.Skills[0].Name)
	}

	// Verify skill was mirrored to .gandalf/skills/pr-reviewer/SKILL.md
	mirroredSkillMD := filepath.Join(projDir, ".gandalf", "skills", "pr-reviewer", "SKILL.md")
	if data, err := os.ReadFile(mirroredSkillMD); err != nil || string(data) != "# PR Reviewer Skill" {
		t.Errorf("expected skill to be mirrored to .gandalf/skills, got err: %v", err)
	}
}
