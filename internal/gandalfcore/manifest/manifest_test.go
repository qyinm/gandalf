package manifest

import (
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestParseManifest(t *testing.T) {
	tomlContent := `
version = "1.0"
name = "payment-team"
description = "Team AI Agent Setup"
agents = ["claude-code", "codex"]

[mcp_servers.postgres-db]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres", "${DB_URL}"]
required_env = ["DB_URL"]
description = "Staging database"

[mcp_servers.remote-docs]
url = "https://mcp.example.com/sse"
description = "Remote API Docs"

[[skills]]
name = "api-builder"
source = "./.gandalf/skills/api-builder"
description = "API code generator"

[hooks.pre-save-lint]
event = "before_save"
command = "./scripts/lint.sh"

[env_template]
DB_URL = "postgres://user:pass@localhost:5432/db"
`

	envMap := map[string]string{
		"DB_URL": "postgres://prod:secret@db.internal:5432/pay",
	}
	envGetter := func(k string) string {
		return envMap[k]
	}

	result, err := Parse(tomlContent, &ParseOptions{EnvGetter: envGetter})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.MissingEnvs) != 0 {
		t.Fatalf("expected 0 missing envs, got %v", result.MissingEnvs)
	}

	m := result.Manifest
	if m.Version != "1.0" || m.Name != "payment-team" {
		t.Errorf("manifest header mismatch: version=%s, name=%s", m.Version, m.Name)
	}

	if len(m.Agents) != 2 || m.Agents[0] != types.AgentClaudeCode || m.Agents[1] != types.AgentCodex {
		t.Errorf("unexpected agents: %v", m.Agents)
	}

	dbServer, ok := m.MCPServers["postgres-db"]
	if !ok {
		t.Fatalf("postgres-db server missing")
	}
	if dbServer.Command != "npx" || len(dbServer.Args) != 3 || dbServer.Args[2] != "postgres://prod:secret@db.internal:5432/pay" {
		t.Errorf("unexpected server args: %v", dbServer.Args)
	}

	if len(m.Skills) != 1 || m.Skills[0].Name != "api-builder" {
		t.Errorf("unexpected skills: %v", m.Skills)
	}

	hook, ok := m.Hooks["pre-save-lint"]
	if !ok || hook.Event != "before_save" {
		t.Errorf("unexpected hook: %v", hook)
	}
}

func TestParseMissingEnv(t *testing.T) {
	tomlContent := `
version = "1.0"
name = "test"
agents = ["codex"]

[mcp_servers.db]
command = "npx"
args = ["server", "${SECRET_KEY}"]
required_env = ["SECRET_KEY", "ANOTHER_KEY"]
`
	result, err := Parse(tomlContent, &ParseOptions{
		EnvGetter: func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.MissingEnvs) < 2 {
		t.Errorf("expected at least 2 missing envs, got %v", result.MissingEnvs)
	}
}

func TestValidateManifest(t *testing.T) {
	tempDir := t.TempDir()

	m := &Manifest{
		Version: "1.0",
		Name:    "my-team",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCodex},
		MCPServers: map[string]MCPServerDef{
			"valid-mcp": {Command: "npx", Args: []string{"test"}},
			"bad-mcp":   {}, // missing command/url
		},
		Skills: []SkillDef{
			{Name: "ok-skill", Source: "./.gandalf/skills/ok"},
			{Name: "bad-skill", Source: "../../etc/passwd"}, // escaping project root
		},
		Hooks: map[string]HookDef{
			"valid-hook": {Event: "before_save", Command: "./lint.sh"},
		},
	}

	errs := Validate(m, tempDir)
	if len(errs) != 2 {
		t.Fatalf("expected 2 validation errors, got %d: %v", len(errs), errs)
	}
}

func TestParseManifest_EscapesEnvInjection(t *testing.T) {
	tomlContent := `
version = "1.0"
name = "injection-test"
agents = ["codex"]

[mcp_servers.test]
command = "echo"
args = ["${USER_INPUT}"]
`
	// Malicious environment value trying to inject a new table and command
	maliciousEnv := "\"\n[mcp_servers.injected]\ncommand = \"evil\"\nfoo = \""
	envGetter := func(k string) string {
		if k == "USER_INPUT" {
			return maliciousEnv
		}
		return ""
	}

	result, err := Parse(tomlContent, &ParseOptions{EnvGetter: envGetter})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Injected server should NOT exist
	if _, exists := result.Manifest.MCPServers["injected"]; exists {
		t.Errorf("security violation: malicious environment value injected new server")
	}

	// The argument should contain the escaped string safely inside quotes
	testSrv := result.Manifest.MCPServers["test"]
	if len(testSrv.Args) != 1 {
		t.Fatalf("expected 1 arg, got: %d", len(testSrv.Args))
	}
}

func TestParseManifest_QuotedDottedServerEnvTable(t *testing.T) {
	tomlContent := `
version = "1.0"
name = "dotted-test"
agents = ["cursor"]

[mcp_servers."server.with.dots"]
command = "run-tool"

[mcp_servers."server.with.dots".env]
API_KEY = "my-secret-key"
DEBUG = "1"
`
	result, err := Parse(tomlContent, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	srv, ok := result.Manifest.MCPServers["server.with.dots"]
	if !ok {
		t.Fatalf("expected 'server.with.dots' in MCPServers, got keys: %v", result.Manifest.MCPServers)
	}
	if srv.Command != "run-tool" {
		t.Errorf("expected command 'run-tool', got: %s", srv.Command)
	}
	if srv.Env["API_KEY"] != "my-secret-key" {
		t.Errorf("expected API_KEY 'my-secret-key', got: %s", srv.Env["API_KEY"])
	}
	if srv.Env["DEBUG"] != "1" {
		t.Errorf("expected DEBUG '1', got: %s", srv.Env["DEBUG"])
	}
}

func TestParseManifest_StripsInlineComments(t *testing.T) {
	tomlContent := `
version = "1.0" # version comment
name = "comments-test" # team name
agents = ["codex"]

[mcp_servers.my-server]
command = "node" # launch node
url = "https://example.com/api#fragment" # url with valid fragment
args = ["-y", "tool#1"] # inline flags
enabled = false # disabled for testing
`
	result, err := Parse(tomlContent, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	srv := result.Manifest.MCPServers["my-server"]
	if srv.Command != "node" {
		t.Errorf("expected command 'node', got: %s", srv.Command)
	}
	if srv.URL != "https://example.com/api#fragment" {
		t.Errorf("expected URL 'https://example.com/api#fragment', got: %s", srv.URL)
	}
	if len(srv.Args) != 2 || srv.Args[1] != "tool#1" {
		t.Errorf("expected arg 'tool#1', got: %v", srv.Args)
	}
	if !srv.Disabled {
		t.Errorf("expected enabled = false to map to Disabled = true")
	}
}
