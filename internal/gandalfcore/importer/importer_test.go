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

func TestAuth_EndToEndRoundTripAndRedaction(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	if err := os.MkdirAll(filepath.Join(projDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Raw Cursor mcp.json with sensitive oauth auth object
	rawCursor := `{
  "mcpServers": {
    "remote-oauth": {
      "type": "sse",
      "url": "https://mcp.internal.com/sse",
      "auth": {
        "clientId": "client_abc",
        "clientSecret": "super_secret_oauth_client_secret_xyz123"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projDir, ".cursor", "mcp.json"), []byte(rawCursor), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Run import
	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
	}
	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// 3. Verify sensitive clientSecret was redacted
	srv := res.Manifest.MCPServers["remote-oauth"]
	authMap, ok := srv.Auth.(map[string]any)
	if !ok {
		t.Fatalf("expected srv.Auth to be map[string]any, got %T: %v", srv.Auth, srv.Auth)
	}
	secretVal, ok := authMap["clientSecret"].(string)
	if !ok || !strings.Contains(secretVal, "${") {
		t.Fatalf("expected clientSecret to be templated into ${...}, got: %v", authMap["clientSecret"])
	}

	// Verify env_template contains a safe placeholder and NOT the actual secret
	for _, envVal := range res.Manifest.EnvTemplate {
		if strings.Contains(envVal, "super_secret_oauth") {
			t.Fatalf("security violation: raw clientSecret leaked into env_template: %s", envVal)
		}
	}

	// 4. Verify round-trip parsing from generated gandalf.toml
	manifestFile := filepath.Join(projDir, "gandalf.toml")
	manifestBytes, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("failed to read generated gandalf.toml: %v", err)
	}

	parsed, err := manifest.Parse(string(manifestBytes), nil)
	if err != nil {
		t.Fatalf("failed to parse generated manifest: %v\nManifest:\n%s", err, string(manifestBytes))
	}

	parsedSrv, exists := parsed.Manifest.MCPServers["remote-oauth"]
	if !exists {
		t.Fatalf("remote-oauth not found in parsed manifest")
	}
	if parsedSrv.Type != "sse" {
		t.Errorf("expected type 'sse', got: %s", parsedSrv.Type)
	}
	parsedAuth, ok := parsedSrv.Auth.(map[string]any)
	if !ok {
		t.Fatalf("expected parsed auth to be map[string]any, got %T", parsedSrv.Auth)
	}
	if parsedAuth["clientId"] != "client_abc" {
		t.Errorf("expected clientId 'client_abc', got %v", parsedAuth["clientId"])
	}
}

func TestRunImport_OutputSymlinkRejected(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create valid source
	if err := os.WriteFile(filepath.Join(projDir, ".mcp.json"), []byte(`{"mcpServers":{"srv":{"command":"echo"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink inside project pointing outside
	linkDir := filepath.Join(projDir, "symlink_dir")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
		OutputFile:  "symlink_dir/escaped.toml",
	}

	_, err := RunImport(opts)
	if err == nil {
		t.Fatalf("expected error when output crosses a symlinked parent directory")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("expected security violation error, got: %v", err)
	}
}

func TestRedactAndTemplatizeServer_CommandAndMultiTokens(t *testing.T) {
	envTemplate := make(map[string]string)
	srv := manifest.MCPServerDef{
		Command: "connect --url=postgres://usr:pwd@localhost:5432/test --key=sk-ant-api03-abcdefghijklmnop",
		Args:    []string{"--first=sk-proj-abcdefghijklmnopqrstuvwxyz012345", "--second=ghp_0123456789abcdefghijklmnopqrstuvwxyz"},
		Headers: map[string]string{
			"Authorization": "Basic dXNlcjpwYXNz",
			"X-API-Key":     "raw-secret-key-1234",
		},
	}

	RedactAndTemplatizeServer("multi-test", &srv, envTemplate)

	// Verify command credentials redacted
	if strings.Contains(srv.Command, "pwd@") || strings.Contains(srv.Command, "sk-ant") {
		t.Errorf("expected credentials in command to be redacted, got: %s", srv.Command)
	}

	// Verify multiple tokens in args all redacted
	for _, arg := range srv.Args {
		if strings.Contains(arg, "sk-proj-xyz123") || strings.Contains(arg, "ghp_") {
			t.Errorf("expected all secrets in args to be redacted, got: %s", arg)
		}
	}

	// Verify headers redacted
	if srv.Headers["Authorization"] == "Basic dXNlcjpwYXNz" {
		t.Errorf("expected Authorization header to be redacted")
	}
	if srv.Headers["X-API-Key"] == "raw-secret-key-1234" {
		t.Errorf("expected X-API-Key header to be redacted")
	}
}

func TestParseStandardJSONMCPServers_SanitizesEnvFile(t *testing.T) {
	rawJSON := `{
  "mcpServers": {
    "bad-server": {
      "command": "tool",
      "envFile": "../../etc/shadow"
    },
    "good-server": {
      "command": "tool",
      "envFile": "${workspaceFolder}/.env"
    }
  }
}`
	servers, err := ParseStandardJSONMCPServers([]byte(rawJSON))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if servers["bad-server"].EnvFile != "" {
		t.Errorf("expected bad-server envFile to be rejected, got: %s", servers["bad-server"].EnvFile)
	}
	if servers["good-server"].EnvFile != "${workspaceFolder}/.env" {
		t.Errorf("expected good-server envFile preserved, got: %s", servers["good-server"].EnvFile)
	}
}

func TestRunImport_ExistingSkillRequiresForce(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	if err := os.MkdirAll(filepath.Join(projDir, ".cursor", "skills", "my-skill"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, ".cursor", "skills", "my-skill", "SKILL.md"), []byte("# Imported Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Pre-create destination team skill
	destSkillDir := filepath.Join(projDir, ".gandalf", "skills", "my-skill")
	if err := os.MkdirAll(destSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	originalContent := "# Manually Maintained Skill"
	if err := os.WriteFile(filepath.Join(destSkillDir, "SKILL.md"), []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
		Force:       false,
	}

	_, err := RunImport(opts)
	if err == nil {
		t.Fatalf("expected error when skill already exists without --force")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}

	// Verify original content was preserved
	content, _ := os.ReadFile(filepath.Join(destSkillDir, "SKILL.md"))
	if string(content) != originalContent {
		t.Errorf("expected original skill content to be preserved, got: %s", string(content))
	}
}

func TestReconcileSources_ProjectSkillOverridesGlobal(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	homeDir := filepath.Join(tempDir, "home")

	// Global skill
	if err := os.MkdirAll(filepath.Join(homeDir, ".cursor", "skills", "common-skill"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".cursor", "skills", "common-skill", "SKILL.md"), []byte("# Global Common"), 0644); err != nil {
		t.Fatal(err)
	}

	// Project skill with same name
	if err := os.MkdirAll(filepath.Join(projDir, ".cursor", "skills", "common-skill"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, ".cursor", "skills", "common-skill", "SKILL.md"), []byte("# Project Common"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		HomeDir:     homeDir,
		Force:       true,
	}

	importRes, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// Verify the mirrored skill has project content, not global content
	copiedMD, err := os.ReadFile(filepath.Join(projDir, ".gandalf", "skills", "common-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read copied skill: %v", err)
	}
	if !strings.Contains(string(copiedMD), "# Project Common") {
		t.Errorf("expected project skill to override global, got: %s", string(copiedMD))
	}

	_ = importRes
}

func TestRunImport_IgnoresFolderWithoutSkillMD(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	skillsDir := filepath.Join(projDir, ".cursor", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Valid skill folder
	validDir := filepath.Join(skillsDir, "valid-skill")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "SKILL.md"), []byte("# Valid"), 0644); err != nil {
		t.Fatal(err)
	}

	// Non-skill folder (e.g. node_modules or temp docs)
	invalidDir := filepath.Join(skillsDir, "invalid-folder")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "README.txt"), []byte("not a skill"), 0644); err != nil {
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

	// Only valid-skill should be mirrored
	if len(res.MirroredSkills) != 1 || res.MirroredSkills[0] != "valid-skill" {
		t.Errorf("expected only 'valid-skill' mirrored, got: %v", res.MirroredSkills)
	}
	if _, err := os.Stat(filepath.Join(projDir, ".gandalf", "skills", "invalid-folder")); err == nil {
		t.Errorf("expected invalid-folder to NOT be mirrored")
	}
}

func TestRunImport_NestedOutputCreatesParentDir(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	if err := os.MkdirAll(filepath.Join(projDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, ".cursor", "mcp.json"), []byte(`{"mcpServers":{"test":{"command":"ls"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	nestedOutput := "config/nested/gandalf.toml"
	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
		OutputFile:  nestedOutput,
	}

	_, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed with nested output: %v", err)
	}

	if _, err := os.Stat(filepath.Join(projDir, nestedOutput)); err != nil {
		t.Errorf("expected nested output file to exist, got err: %v", err)
	}
}

func TestParseCodexConfigTOML_DottedEnvKeys(t *testing.T) {
	codexTOML := `
[mcp_servers.my-server]
command = "npx"
args = ["-y", "my-tool"]
env.API_KEY = "test-key-123"
env.DEBUG = "1"
`
	servers, err := ParseCodexConfigTOML([]byte(codexTOML))
	if err != nil {
		t.Fatalf("ParseCodexConfigTOML failed: %v", err)
	}

	srv, ok := servers["my-server"]
	if !ok {
		t.Fatalf("my-server not found")
	}
	if srv.Env["API_KEY"] != "test-key-123" {
		t.Errorf("expected API_KEY 'test-key-123', got: %s", srv.Env["API_KEY"])
	}
	if srv.Env["DEBUG"] != "1" {
		t.Errorf("expected DEBUG '1', got: %s", srv.Env["DEBUG"])
	}
}

func TestRunImport_FromDirectoryDetectsNestedMCP(t *testing.T) {
	tempDir := t.TempDir()
	customDir := filepath.Join(tempDir, "custom_agent_dir")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "mcp.json"), []byte(`{"mcpServers":{"nested-srv":{"command":"node","args":["index.js"]}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	projDir := filepath.Join(tempDir, "proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		FromPath:    customDir,
		DryRun:      true,
	}

	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	if _, exists := res.Manifest.MCPServers["nested-srv"]; !exists {
		t.Errorf("expected nested-srv to be discovered from directory --from")
	}
}

func TestFormatManifestTOML_QuotesDottedServerNames(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-proj",
		Agents:  []types.AgentID{types.AgentCursor},
		MCPServers: map[string]manifest.MCPServerDef{
			"server.with.dots": {
				Command: "run-server",
			},
		},
	}

	formatted := FormatManifestTOML(m)
	if !strings.Contains(formatted, `[mcp_servers."server.with.dots"]`) {
		t.Errorf("expected dotted server name to be quoted in table header, got:\n%s", formatted)
	}

	// Verify it parses back cleanly
	parsed, err := manifest.Parse(formatted, nil)
	if err != nil {
		t.Fatalf("failed to parse back manifest: %v\nContent:\n%s", err, formatted)
	}
	if _, ok := parsed.Manifest.MCPServers["server.with.dots"]; !ok {
		t.Errorf("expected server.with.dots in parsed manifest")
	}
}

func TestRedactAndTemplatizeServer_FlagAndPositionalCredentials(t *testing.T) {
	envTemplate := make(map[string]string)
	srv := manifest.MCPServerDef{
		Command: "npx",
		Args: []string{
			"-y",
			"my-tool",
			"--api-key=supersecretapikey12345",
			"--token",
			"rawcustomtoken987654321",
		},
	}

	RedactAndTemplatizeServer("my-srv", &srv, envTemplate)

	if !strings.Contains(srv.Args[2], "${MY_SRV_API_KEY}") {
		t.Errorf("expected --api-key= to be templatized, got: %s", srv.Args[2])
	}
	if srv.Args[4] != "${MY_SRV_TOKEN}" {
		t.Errorf("expected positional token to be templatized, got: %s", srv.Args[4])
	}
	if _, ok := envTemplate["MY_SRV_API_KEY"]; !ok {
		t.Errorf("expected MY_SRV_API_KEY in envTemplate")
	}
	if _, ok := envTemplate["MY_SRV_TOKEN"]; !ok {
		t.Errorf("expected MY_SRV_TOKEN in envTemplate")
	}
}

func TestReconcileSources_ClaudeCodeAgentTargeting(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, ".mcp.json"), []byte(`{"mcpServers":{"claude-srv":{"command":"ls"}}}`), 0644); err != nil {
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

	foundClaude := false
	for _, a := range res.Manifest.Agents {
		if a == types.AgentClaudeCode {
			foundClaude = true
			break
		}
	}
	if !foundClaude {
		t.Errorf("expected AgentClaudeCode ('claude-code') in manifest agents, got: %v", res.Manifest.Agents)
	}
}

func TestFormatManifestTOML_StructuredAuthTypesPreserved(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-auth",
		Agents:  []types.AgentID{types.AgentCursor},
		MCPServers: map[string]manifest.MCPServerDef{
			"auth-srv": {
				Command: "run",
				Auth: map[string]any{
					"enabled": true,
					"retries": 5,
					"token":   "${AUTH_SRV_TOKEN}",
				},
			},
		},
	}

	formatted := FormatManifestTOML(m)
	if !strings.Contains(formatted, "enabled = true") {
		t.Errorf("expected 'enabled = true' without quotes, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "retries = 5") {
		t.Errorf("expected 'retries = 5' without quotes, got:\n%s", formatted)
	}

	// Parse back and verify types
	parsed, err := manifest.Parse(formatted, nil)
	if err != nil {
		t.Fatalf("failed to parse formatted TOML: %v\n%s", err, formatted)
	}
	authMap, ok := parsed.Manifest.MCPServers["auth-srv"].Auth.(map[string]any)
	if !ok {
		t.Fatalf("expected Auth to be map[string]any, got: %T", parsed.Manifest.MCPServers["auth-srv"].Auth)
	}
	if authMap["enabled"] != true {
		t.Errorf("expected authMap['enabled'] to be bool true, got: %v (%T)", authMap["enabled"], authMap["enabled"])
	}
	if authMap["retries"] != 5 {
		t.Errorf("expected authMap['retries'] to be int 5, got: %v (%T)", authMap["retries"], authMap["retries"])
	}
}

func TestRunImport_ForceCleansObsoleteFiles(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	srcSkillDir := filepath.Join(projDir, ".cursor", "skills", "clean-skill")
	if err := os.MkdirAll(srcSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkillDir, "SKILL.md"), []byte("# New Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Existing destination team skill with obsolete file
	destSkillDir := filepath.Join(projDir, ".gandalf", "skills", "clean-skill")
	if err := os.MkdirAll(destSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destSkillDir, "SKILL.md"), []byte("# Old Skill"), 0644); err != nil {
		t.Fatal(err)
	}
	obsoleteFile := filepath.Join(destSkillDir, "old_obsolete_script.sh")
	if err := os.WriteFile(obsoleteFile, []byte("#!/bin/sh\necho old"), 0755); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
		Force:       true,
	}

	_, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport with force failed: %v", err)
	}

	// Verify obsolete file was removed
	if _, err := os.Stat(obsoleteFile); err == nil {
		t.Errorf("expected obsolete file to be removed on --force overwrite")
	}

	// Verify new content is present
	content, _ := os.ReadFile(filepath.Join(destSkillDir, "SKILL.md"))
	if string(content) != "# New Skill" {
		t.Errorf("expected updated skill content, got: %s", string(content))
	}
}

func TestRunImport_FailedImportRestoresExistingSkill(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	srcSkillDir := filepath.Join(projDir, ".cursor", "skills", "test-skill")
	if err := os.MkdirAll(srcSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkillDir, "SKILL.md"), []byte("# Source Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Existing skill
	destSkillDir := filepath.Join(projDir, ".gandalf", "skills", "test-skill")
	if err := os.MkdirAll(destSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	origFile := filepath.Join(destSkillDir, "original.txt")
	if err := os.WriteFile(origFile, []byte("preserve me"), 0644); err != nil {
		t.Fatal(err)
	}

	// Intentionally make output manifest path fail to write (e.g. symlink to root or read-only directory)
	// By specifying a directory path as the OutputFile, WriteTextAtomically will fail!
	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
		Force:       true,
		OutputFile:  ".cursor", // exists as directory, write will fail
	}

	_, err := RunImport(opts)
	if err == nil {
		t.Fatalf("expected RunImport to fail on directory output file")
	}

	// Verify original skill directory was restored!
	content, err := os.ReadFile(origFile)
	if err != nil {
		t.Fatalf("original file not found after rollback: %v", err)
	}
	if string(content) != "preserve me" {
		t.Errorf("expected original content preserved, got: %s", string(content))
	}
}

func TestRunImport_FromExplicitFileInfersAgent(t *testing.T) {
	tempDir := t.TempDir()
	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(codexDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[mcp_servers.my-codex]\ncommand = \"node\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: tempDir,
		FromPath:    configPath,
		DryRun:      true,
	}

	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	if len(res.Manifest.Agents) != 1 || res.Manifest.Agents[0] != types.AgentCodex {
		t.Errorf("expected only AgentCodex in agents, got: %v", res.Manifest.Agents)
	}
}

func TestFormatManifestTOML_SpecialKeysQuoted(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "special-keys",
		Agents:  []types.AgentID{types.AgentCursor},
		MCPServers: map[string]manifest.MCPServerDef{
			"srv": {
				Command: "run",
				Headers: map[string]string{
					"X-Custom.Header": "value",
					"Space Header":    "val2",
				},
				Env: map[string]string{
					"dot.env.key": "val3",
				},
				Auth: map[string]any{
					"auth.token": "tok123",
				},
			},
		},
		EnvTemplate: map[string]string{
			"TEMPLATE.WITH.DOTS": "dummy",
		},
	}

	formatted := FormatManifestTOML(m)
	if !strings.Contains(formatted, `"X-Custom.Header" = "value"`) {
		t.Errorf("expected quoted header key, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, `"Space Header" = "val2"`) {
		t.Errorf("expected quoted space header key, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, `"dot.env.key" = "val3"`) {
		t.Errorf("expected quoted env key, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, `"auth.token" = "tok123"`) {
		t.Errorf("expected quoted auth key, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, `"TEMPLATE.WITH.DOTS" = "dummy"`) {
		t.Errorf("expected quoted env_template key, got:\n%s", formatted)
	}

	// Validate it parses cleanly
	parsed, err := manifest.Parse(formatted, nil)
	if err != nil {
		t.Fatalf("failed to parse formatted TOML with special keys: %v\n%s", err, formatted)
	}
	if parsed.Manifest.MCPServers["srv"].Headers["X-Custom.Header"] != "value" {
		t.Errorf("header value mismatch after parse")
	}
}

func TestRunImport_SkillPreservesExecutablePermissions(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	skillDir := filepath.Join(projDir, ".claude", "skills", "exec-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Exec Skill"), 0644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(skillDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok"), 0755); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: projDir,
		ProjectOnly: true,
	}

	_, err := RunImport(opts)
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	destScript := filepath.Join(projDir, ".gandalf", "skills", "exec-skill", "run.sh")
	fi, err := os.Stat(destScript)
	if err != nil {
		t.Fatalf("dest script not found: %v", err)
	}
	if fi.Mode().Perm()&0111 == 0 {
		t.Errorf("expected executable permissions preserved, got: %o", fi.Mode().Perm())
	}
}

func TestFormatManifestTOML_NestedAuthRoundTrip(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "nested-auth",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		MCPServers: map[string]manifest.MCPServerDef{
			"api-srv": {
				Command: "run",
				Auth: map[string]any{
					"token": "secret-token",
					"options": map[string]any{
						"retry":  3,
						"secure": true,
					},
				},
			},
		},
	}

	formatted := FormatManifestTOML(m)
	parsed, err := manifest.Parse(formatted, nil)
	if err != nil {
		t.Fatalf("failed to parse TOML with nested auth: %v\n%s", err, formatted)
	}

	authMap, ok := parsed.Manifest.MCPServers["api-srv"].Auth.(map[string]any)
	if !ok {
		t.Fatalf("expected Auth to be map[string]any, got: %T", parsed.Manifest.MCPServers["api-srv"].Auth)
	}
	optsMap, ok := authMap["options"].(map[string]any)
	if !ok {
		t.Fatalf("expected options to be nested map[string]any, got: %T", authMap["options"])
	}
	if optsMap["retry"] != 3 {
		t.Errorf("expected retry 3, got: %v", optsMap["retry"])
	}
	if optsMap["secure"] != true {
		t.Errorf("expected secure true, got: %v", optsMap["secure"])
	}
}

func TestParseCodexConfigTOML_PreservesExistingPlaceholders(t *testing.T) {
	os.Setenv("SECRET_KEY", "real-secret-must-not-leak")
	defer os.Unsetenv("SECRET_KEY")

	codexToml := `
[mcp_servers.my-tool]
command = "run"
args = ["${SECRET_KEY}"]
`
	servers, err := ParseCodexConfigTOML([]byte(codexToml))
	if err != nil {
		t.Fatalf("ParseCodexConfigTOML failed: %v", err)
	}

	srv := servers["my-tool"]
	if len(srv.Args) != 1 || srv.Args[0] != "${SECRET_KEY}" {
		t.Errorf("expected ${SECRET_KEY} placeholder preserved without interpolation, got: %v", srv.Args)
	}
}

func TestRunImport_DotPrefixedOutputFileAllowed(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(`{"mcpServers":{"srv":{"command":"ls"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	opts := ImportOptions{
		ProjectPath: tempDir,
		ProjectOnly: true,
		OutputFile:  "..backup.toml",
	}

	res, err := RunImport(opts)
	if err != nil {
		t.Fatalf("expected ..backup.toml to be allowed in project root, got: %v", err)
	}

	expectedFile := filepath.Join(tempDir, "..backup.toml")
	if !fileExists(expectedFile) {
		t.Errorf("expected output file '%s' to be created", expectedFile)
	}
	_ = res
}

func TestRedactAndTemplatizeServer_NestedAuthSecrets(t *testing.T) {
	envTemplate := make(map[string]string)
	srv := manifest.MCPServerDef{
		Command: "npx",
		Auth: map[string]any{
			"provider": "custom",
			"config": map[string]any{
				"api_key": "rawsupersecretkey123456",
			},
		},
	}

	RedactAndTemplatizeServer("my-srv", &srv, envTemplate)

	authMap, ok := srv.Auth.(map[string]any)
	if !ok {
		t.Fatalf("expected Auth to be map[string]any")
	}
	cfgMap, ok := authMap["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config to be map[string]any")
	}
	apiKeyVal, ok := cfgMap["api_key"].(string)
	if !ok || !strings.HasPrefix(apiKeyVal, "${MY_SRV_") {
		t.Errorf("expected nested api_key to be templatized, got: %v", cfgMap["api_key"])
	}
	foundInEnv := false
	for k := range envTemplate {
		if strings.Contains(k, "API_KEY") {
			foundInEnv = true
			break
		}
	}
	if !foundInEnv {
		t.Errorf("expected templatized key in envTemplate, got: %v", envTemplate)
	}
}
