package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
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

	if res.Manifest.EnvTemplate["DATABASE_URL"] == "" || strings.Contains(res.Manifest.EnvTemplate["DATABASE_URL"], "supersecret") {
		t.Errorf("expected safe template in env_template without exposing secret, got %v", res.Manifest.EnvTemplate["DATABASE_URL"])
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

func TestFormatManifestTOML_RoundTripWithEscapes(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "roundtrip-test",
		Agents:  []types.AgentID{"claude-code", "cursor"},
		MCPServers: map[string]manifest.MCPServerDef{
			"win-tool": {
				Command:     `C:\Program Files\Tool\tool.exe`,
				Args:        []string{`--config="C:\data\config.json"`, `with "quotes"`},
				Description: `A tool with "quotes" and \backslashes\`,
			},
		},
	}

	formatted := FormatManifestTOML(m)
	parsed, err := manifest.Parse(formatted, nil)
	if err != nil {
		t.Fatalf("failed to parse formatted TOML: %v\nFormatted content:\n%s", err, formatted)
	}

	winTool, ok := parsed.Manifest.MCPServers["win-tool"]
	if !ok {
		t.Fatalf("win-tool not found in parsed manifest")
	}

	if winTool.Command != `C:\Program Files\Tool\tool.exe` {
		t.Errorf("expected command to round-trip accurately, got: %s", winTool.Command)
	}
	if len(winTool.Args) != 2 || winTool.Args[0] != `--config="C:\data\config.json"` {
		t.Errorf("expected args to round-trip accurately, got: %v", winTool.Args)
	}
	if winTool.Description != `A tool with "quotes" and \backslashes\` {
		t.Errorf("expected description to round-trip accurately, got: %s", winTool.Description)
	}
}

func TestRunImport_OutputEscapesProjectRejected(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	projMCP := `{"mcpServers": {"demo": {"command": "node"}}}`
	if err := os.WriteFile(filepath.Join(projDir, ".mcp.json"), []byte(projMCP), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
		OutputFile:  "../outside.toml",
	}

	_, err := RunImport(opts)
	if err == nil {
		t.Fatalf("expected path traversal output file to be rejected, but it succeeded")
	}
	if !strings.Contains(err.Error(), "escapes project root") {
		t.Errorf("expected security violation error message, got: %v", err)
	}
}

func TestRunImport_SafePlaceholderInEnvTemplate(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	projMCP := `{
  "mcpServers": {
    "auth-service": {
      "command": "npx",
      "args": ["-y", "auth-tool"],
      "env": {
        "SECRET_KEY": "sk-ant-api03-actual-sensitive-key-123456789012345678901234567890123456789012345678901234567890"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projDir, ".mcp.json"), []byte(projMCP), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
	}

	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// Ensure actual raw secret key is NEVER stored in envTemplate
	envVal := res.Manifest.EnvTemplate["SECRET_KEY"]
	if strings.Contains(envVal, "actual-sensitive-key") {
		t.Errorf("security violation: raw secret key was exposed in env_template! Got: %s", envVal)
	}
	if !strings.Contains(envVal, "sample") && !strings.Contains(envVal, "your-") {
		t.Errorf("expected safe placeholder in env_template, got: %s", envVal)
	}
}

func TestParseStringArray_WithCommasInsideQuotes(t *testing.T) {
	manifestTOML := `
[mcp_servers.tool]
command = "node"
args = ["--param=a,b,c", "simple", "quoted, with comma"]
`
	parsed, err := manifest.Parse(manifestTOML, nil)
	if err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	tool, ok := parsed.Manifest.MCPServers["tool"]
	if !ok {
		t.Fatalf("tool server not found")
	}

	if len(tool.Args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(tool.Args), tool.Args)
	}
	if tool.Args[0] != "--param=a,b,c" {
		t.Errorf("expected '--param=a,b,c', got '%s'", tool.Args[0])
	}
	if tool.Args[1] != "simple" {
		t.Errorf("expected 'simple', got '%s'", tool.Args[1])
	}
	if tool.Args[2] != "quoted, with comma" {
		t.Errorf("expected 'quoted, with comma', got '%s'", tool.Args[2])
	}
}

func TestParseCodexConfigTOML_WithNestedEnvTable(t *testing.T) {
	codexTOML := `
[mcp_servers.my_server]
command = "node"
args = ["server.js"]

[mcp_servers.my_server.env]
API_KEY = "xyz123"
DEBUG = "true"
`
	servers, err := ParseCodexConfigTOML([]byte(codexTOML))
	if err != nil {
		t.Fatalf("ParseCodexConfigTOML failed: %v", err)
	}

	if len(servers) != 1 {
		t.Fatalf("expected exactly 1 server, got %d: %v", len(servers), servers)
	}

	srv, exists := servers["my_server"]
	if !exists {
		t.Fatalf("my_server not found in parsed servers")
	}

	if srv.Env["API_KEY"] != "xyz123" {
		t.Errorf("expected API_KEY=xyz123, got: %s", srv.Env["API_KEY"])
	}
	if srv.Env["DEBUG"] != "true" {
		t.Errorf("expected DEBUG=true, got: %s", srv.Env["DEBUG"])
	}
}

func TestParseStandardJSONMCPServers_PreservesTypeAndEnvFile(t *testing.T) {
	cursorJSON := `{
  "mcpServers": {
    "cursor-server": {
      "type": "stdio",
      "command": "npx",
      "args": ["my-tool"],
      "envFile": "${workspaceFolder}/.env"
    }
  }
}`
	servers, err := ParseStandardJSONMCPServers([]byte(cursorJSON))
	if err != nil {
		t.Fatalf("ParseStandardJSONMCPServers failed: %v", err)
	}

	srv, ok := servers["cursor-server"]
	if !ok {
		t.Fatalf("cursor-server not found")
	}

	if srv.Type != "stdio" {
		t.Errorf("expected type 'stdio', got: %s", srv.Type)
	}
	if srv.EnvFile != "${workspaceFolder}/.env" {
		t.Errorf("expected envFile '${workspaceFolder}/.env', got: %s", srv.EnvFile)
	}
}

func TestRunImport_DestinationSymlinkRejected(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	outsideDir := filepath.Join(tempDir, "outside")

	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Create a skill: .cursor/skills/test-skill/SKILL.md
	skillDir := filepath.Join(projDir, ".cursor", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill Content"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Make destination .gandalf/skills/test-skill a symlink to outsideDir
	gandalfSkillsDir := filepath.Join(projDir, ".gandalf", "skills")
	if err := os.MkdirAll(gandalfSkillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	destSymlink := filepath.Join(gandalfSkillsDir, "test-skill")
	if err := os.Symlink(outsideDir, destSymlink); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
	}

	_, err := RunImport(opts)
	if err == nil {
		t.Fatalf("expected import to fail when destination skill is a symlink")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink security error, got: %v", err)
	}
}

func TestRunImport_SymlinkedSkillParentRejected(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	outsideDir := filepath.Join(tempDir, "outside")

	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Create a skill: .cursor/skills/test-skill/SKILL.md
	skillDir := filepath.Join(projDir, ".cursor", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill Content"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Make the parent .gandalf a symlink pointing to outsideDir
	symlinkTarget := filepath.Join(projDir, ".gandalf")
	if err := os.Symlink(outsideDir, symlinkTarget); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
	}

	_, err := RunImport(opts)
	if err == nil {
		t.Fatalf("expected import to fail when .gandalf parent directory is a symlink")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink security error, got: %v", err)
	}
}

func TestParseStandardJSONMCPServers_PreservesAuth(t *testing.T) {
	cursorJSON := `{
  "mcpServers": {
    "oauth-server": {
      "type": "sse",
      "url": "https://mcp.example.com/sse",
      "auth": {
        "clientId": "client_123",
        "token": "tok_xyz"
      }
    }
  }
}`
	servers, err := ParseStandardJSONMCPServers([]byte(cursorJSON))
	if err != nil {
		t.Fatalf("ParseStandardJSONMCPServers failed: %v", err)
	}

	srv, ok := servers["oauth-server"]
	if !ok {
		t.Fatalf("oauth-server not found")
	}

	if srv.Type != "sse" {
		t.Errorf("expected type 'sse', got: %s", srv.Type)
	}
	if srv.Auth == nil {
		t.Fatalf("expected auth to be preserved, got nil")
	}

	authMap, ok := srv.Auth.(map[string]any)
	if !ok {
		t.Fatalf("expected auth to be map[string]any, got %T", srv.Auth)
	}
	if authMap["clientId"] != "client_123" {
		t.Errorf("expected clientId 'client_123', got %v", authMap["clientId"])
	}
}
