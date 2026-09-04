package importer_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/sync"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// Test 1: Server Deduplication and Precedence with Overlapping Project vs Global Configs
func TestStress_PrecedenceAndDeduplication_MixedKeys(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "project")
	homeDir := filepath.Join(tempDir, "home")

	if err := os.MkdirAll(filepath.Join(projDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Project has cursor mcp.json with serverA and serverB
	projCursorMCP := `{
		"mcpServers": {
			"serverA": {
				"command": "project-cursor-cmd",
				"args": ["--arg1", "proj-val"],
				"env": {"PROJ_ENV_A": "true"}
			},
			"serverB": {
				"command": "serverB-proj-cmd"
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(projDir, ".cursor", "mcp.json"), []byte(projCursorMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// Project also has .mcp.json (Claude Code) with serverB (overlapping project config)
	projClaudeMCP := `{
		"mcpServers": {
			"serverB": {
				"command": "serverB-claude-cmd",
				"args": ["--from-claude"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(projDir, ".mcp.json"), []byte(projClaudeMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// Global has ~/.claude.json with serverA (overlapping with project), serverC (global-only)
	globalClaudeJSON := `{
		"mcpServers": {
			"serverA": {
				"command": "global-claude-cmd",
				"args": ["--global-arg"],
				"env": {"GLOBAL_ENV": "true"},
				"headers": {"X-Auth": "secret-global"}
			},
			"serverC": {
				"command": "global-c-cmd",
				"args": ["c-arg"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(globalClaudeJSON), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := importer.RunImport(importer.ImportOptions{
		ProjectPath: projDir,
		HomeDir:     homeDir,
		ProjectOnly: false,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// 1. Verify deduplication: serverA, serverB, serverC must be unified
	if len(res.Manifest.MCPServers) != 3 {
		t.Fatalf("expected 3 servers (serverA, serverB, serverC), got %d: %v", len(res.Manifest.MCPServers), res.Manifest.MCPServers)
	}

	// 2. Verify project precedence over global for serverA
	srvA, ok := res.Manifest.MCPServers["serverA"]
	if !ok {
		t.Fatalf("missing serverA in manifest")
	}
	if srvA.Command != "project-cursor-cmd" {
		t.Errorf("expected serverA command to be 'project-cursor-cmd' (project precedence), got '%s'", srvA.Command)
	}
	// Check whether global mixed keys (headers, GLOBAL_ENV) leaked into project definition
	if srvA.Headers != nil && len(srvA.Headers) > 0 {
		t.Logf("Notice: global headers merged into project serverA: %v", srvA.Headers)
	} else {
		t.Logf("Project serverA completely overrides global serverA (global keys not merged)")
	}

	// 3. Verify serverB was unified across project files
	srvB, ok := res.Manifest.MCPServers["serverB"]
	if !ok {
		t.Fatalf("missing serverB in manifest")
	}
	t.Logf("serverB definition after reconciliation: %+v", srvB)

	// 4. Verify serverC from global is retained
	srvC, ok := res.Manifest.MCPServers["serverC"]
	if !ok {
		t.Fatalf("missing serverC in manifest")
	}
	if srvC.Command != "global-c-cmd" {
		t.Errorf("expected serverC command 'global-c-cmd', got '%s'", srvC.Command)
	}

	// 5. Verify validation
	valErrs := manifest.Validate(res.Manifest, projDir)
	if len(valErrs) > 0 {
		t.Errorf("manifest.Validate failed with %d errors: %v", len(valErrs), valErrs)
	}
}

// Test 2: TOML Serialization with Dotted Server Names and Roundtrip
func TestStress_TOMLSerialization_DottedServerNames(t *testing.T) {
	dottedNames := []string{
		"api.github.com",
		"sub.domain.service.co.uk",
		"service.v2",
		"dot.at.start",
	}

	for _, name := range dottedNames {
		t.Run(name, func(t *testing.T) {
			m := &manifest.Manifest{
				Version: "1.0",
				Name:    "dotted-test",
				Agents:  []types.AgentID{types.AgentClaudeCode},
				MCPServers: map[string]manifest.MCPServerDef{
					name: {
						Command: "npx",
						Args:    []string{"-y", name},
						Env: map[string]string{
							"PORT": "8080",
						},
					},
				},
			}

			// Format to TOML
			formatted := importer.FormatManifestTOML(m)
			t.Logf("Emitted TOML for %s:\n%s", name, formatted)

			// Verify table header is properly quoted
			expectedHeader := fmt.Sprintf("[mcp_servers.%q]", name)
			if !strings.Contains(formatted, expectedHeader) {
				t.Errorf("expected TOML to contain quoted header %s, but got:\n%s", expectedHeader, formatted)
			}

			// Parse back
			parsed, err := manifest.Parse(formatted, &manifest.ParseOptions{NoInterpolate: true})
			if err != nil {
				t.Fatalf("failed to parse back emitted TOML: %v", err)
			}

			srv, exists := parsed.Manifest.MCPServers[name]
			if !exists {
				t.Fatalf("server %q was lost or corrupted during roundtrip! Parsed servers: %v", name, parsed.Manifest.MCPServers)
			}
			if srv.Command != "npx" {
				t.Errorf("server %q command corrupted: got %q, want 'npx'", name, srv.Command)
			}
			if srv.Env["PORT"] != "8080" {
				t.Errorf("server %q env corrupted: got %v", name, srv.Env)
			}

			// Validate
			errs := manifest.Validate(parsed.Manifest, "")
			if len(errs) > 0 {
				t.Errorf("manifest.Validate failed on roundtripped manifest: %v", errs)
			}
		})
	}
}

// Test 2b: Server Name Ending in ".env"
func TestStress_TOMLSerialization_ServerNamedDotEnv(t *testing.T) {
	name := "my-service.env"
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "dotenv-test",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		MCPServers: map[string]manifest.MCPServerDef{
			name: {
				Command: "npx",
				Args:    []string{"-y", "my-service"},
			},
		},
	}

	formatted := importer.FormatManifestTOML(m)
	t.Logf("Emitted TOML for %s:\n%s", name, formatted)

	parsed, err := manifest.Parse(formatted, &manifest.ParseOptions{NoInterpolate: true})
	if err != nil {
		t.Fatalf("failed to parse back emitted TOML: %v", err)
	}

	srv, exists := parsed.Manifest.MCPServers[name]
	if !exists {
		t.Errorf("BUG FOUND: server %q was corrupted by .env table handler! Parsed servers: %v", name, parsed.Manifest.MCPServers)
	} else if srv.Command != "npx" {
		t.Errorf("server %q command corrupted: got %q, want 'npx'", name, srv.Command)
	}
}

// Test 2c: Special Unicode Characters
func TestStress_TOMLSerialization_UnicodeServerNames(t *testing.T) {
	unicodeNames := []string{
		"сервер-1",          // Cyrillic
		"🚀-rocket-server",   // Emoji
		"日本語サーバー",          // Japanese CJK
		"中文-agent-服务",       // Chinese CJK
		"café-service",      // Latin accented
		"srv with spaces",   // Spaces
		"srv:colons;semi",   // Punctuation
		"srv/slashes\\back", // Slashes
		"srv\"with\"quotes", // Embedded double quotes
	}

	for _, name := range unicodeNames {
		t.Run(name, func(t *testing.T) {
			m := &manifest.Manifest{
				Version: "1.0",
				Name:    "unicode-test",
				Agents:  []types.AgentID{types.AgentClaudeCode},
				MCPServers: map[string]manifest.MCPServerDef{
					name: {
						Command: "node",
						Args:    []string{"server.js", name},
						Headers: map[string]string{
							"X-Custom-Header": "val-" + name,
						},
					},
				},
			}

			formatted := importer.FormatManifestTOML(m)
			t.Logf("Emitted TOML for %q:\n%s", name, formatted)

			parsed, err := manifest.Parse(formatted, &manifest.ParseOptions{NoInterpolate: true})
			if err != nil {
				t.Fatalf("failed to parse back emitted TOML for %q: %v", name, err)
			}

			srv, exists := parsed.Manifest.MCPServers[name]
			if !exists {
				t.Fatalf("server %q lost during roundtrip! Parsed servers: %v", name, parsed.Manifest.MCPServers)
			}
			if srv.Command != "node" {
				t.Errorf("server %q command corrupted: got %q", name, srv.Command)
			}

			valErrs := manifest.Validate(parsed.Manifest, "")
			if len(valErrs) > 0 {
				t.Errorf("manifest.Validate failed for %q: %v", name, valErrs)
			}
		})
	}
}

// Test 2d: Empty arrays vs Nil arrays
func TestStress_TOMLSerialization_EmptyVsNilArrays(t *testing.T) {
	cases := []struct {
		desc        string
		args        []string
		requiredEnv []string
		env         map[string]string
		headers     map[string]string
	}{
		{
			desc:        "nil slices and maps",
			args:        nil,
			requiredEnv: nil,
			env:         nil,
			headers:     nil,
		},
		{
			desc:        "empty slices and maps",
			args:        []string{},
			requiredEnv: []string{},
			env:         map[string]string{},
			headers:     map[string]string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			m := &manifest.Manifest{
				Version: "1.0",
				Name:    "array-test",
				Agents:  []types.AgentID{types.AgentClaudeCode},
				MCPServers: map[string]manifest.MCPServerDef{
					"srv": {
						Command:     "echo",
						Args:        tc.args,
						RequiredEnv: tc.requiredEnv,
						Env:         tc.env,
						Headers:     tc.headers,
					},
				},
			}

			formatted := importer.FormatManifestTOML(m)
			t.Logf("Formatted TOML (%s):\n%s", tc.desc, formatted)

			parsed, err := manifest.Parse(formatted, &manifest.ParseOptions{NoInterpolate: true})
			if err != nil {
				t.Fatalf("failed to parse back: %v", err)
			}

			srv := parsed.Manifest.MCPServers["srv"]
			if srv.Command != "echo" {
				t.Errorf("expected command 'echo', got %q", srv.Command)
			}

			valErrs := manifest.Validate(parsed.Manifest, "")
			if len(valErrs) > 0 {
				t.Errorf("manifest.Validate failed: %v", valErrs)
			}
		})
	}
}

// Test 3a: Pure clean servers (no variables) passes Validate and DetectProjectDrift InSync=true
func TestStress_EmittedManifest_PureClean_InSync(t *testing.T) {
	tempDir := t.TempDir()

	mcpJSON := `{
		"mcpServers": {
			"pg-server": {
				"command": "npx",
				"args": ["-y", "pg-mcp", "localhost:5432"],
				"env": {
					"PORT": "3000"
				}
			},
			"local-helper": {
				"command": "node",
				"args": ["helper.js"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create skill
	skillDir := filepath.Join(tempDir, ".claude", "skills", "linter")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Linter Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run Import with ProjectOnly = true
	res, err := importer.RunImport(importer.ImportOptions{
		ProjectPath: tempDir,
		ProjectOnly: true,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// Assertion 1: Passes manifest.Validate with 0 errors
	valErrs := manifest.Validate(res.Manifest, tempDir)
	if len(valErrs) > 0 {
		t.Fatalf("manifest.Validate failed: %v", valErrs)
	}

	// Assertion 2: sync.DetectProjectDrift returns InSync = true
	drift, err := sync.DetectProjectDrift(res.Manifest, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift returned error: %v", err)
	}

	t.Logf("Drift Report: InSync=%v, Items=%+v, MissingEnvs=%v", drift.InSync, drift.Items, drift.MissingEnvs)
	if !drift.InSync {
		t.Errorf("DetectProjectDrift failed! InSync was false, items: %+v", drift.Items)
	}
}

// Test 3a2: Existing ${VAR} references in source configs must be declared in env_template
func TestStress_EmittedManifest_ExistingVars_MissingFromEnvTemplate(t *testing.T) {
	tempDir := t.TempDir()

	mcpJSON := `{
		"mcpServers": {
			"pg-server": {
				"command": "npx",
				"args": ["-y", "pg-mcp", "${PG_HOST}"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := importer.RunImport(importer.ImportOptions{
		ProjectPath: tempDir,
		ProjectOnly: true,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// Check if PG_HOST is in env_template
	if _, ok := res.Manifest.EnvTemplate["PG_HOST"]; !ok {
		t.Errorf("BUG FOUND: existing variable ${PG_HOST} in source config was not declared in manifest [env_template]!")
	}

	drift, err := sync.DetectProjectDrift(res.Manifest, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift error: %v", err)
	}
	if !drift.InSync {
		t.Errorf("DetectProjectDrift failed due to missing env_template: InSync=%v, items=%+v", drift.InSync, drift.Items)
	}
}

// Test 3b: Behavior of DetectProjectDrift when importer templatizes raw secrets
func TestStress_EmittedManifest_RawSecret_DetectProjectDriftBehavior(t *testing.T) {
	tempDir := t.TempDir()

	// .mcp.json contains a raw database secret
	mcpJSON := `{
		"mcpServers": {
			"db-svc": {
				"command": "npx",
				"args": ["-y", "pg", "postgres://admin:secretpass@127.0.0.1:5432/proddb"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := importer.RunImport(importer.ImportOptions{
		ProjectPath: tempDir,
		ProjectOnly: true,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// Manifest validation MUST pass
	valErrs := manifest.Validate(res.Manifest, tempDir)
	if len(valErrs) > 0 {
		t.Fatalf("manifest.Validate failed: %v", valErrs)
	}

	// Observe DetectProjectDrift behavior
	drift, err := sync.DetectProjectDrift(res.Manifest, tempDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift returned error: %v", err)
	}

	t.Logf("Observation: When raw secrets are templatized, InSync=%v. Items: %+v", drift.InSync, drift.Items)
	// Because .mcp.json still has the raw secret and gandalf.toml has ${DATABASE_URL},
	// DetectProjectDrift will detect this as drift until `gandalf apply` is run!
}

// Test 3c: Behavior of DetectProjectDrift when multi-source import discovers global agent missing in project
func TestStress_EmittedManifest_GlobalAgentDiscovery_DetectProjectDrift(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "proj")
	homeDir := filepath.Join(tempDir, "home")

	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Project has ONLY .mcp.json (Claude Code)
	projMCP := `{"mcpServers": {"proj-svc": {"command": "echo", "args": ["hello"]}}}`
	if err := os.WriteFile(filepath.Join(projDir, ".mcp.json"), []byte(projMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// Global has Cursor ~/.cursor/mcp.json
	cursorMCP := `{"mcpServers": {"cursor-svc": {"command": "cursor-cmd"}}}`
	if err := os.MkdirAll(filepath.Join(homeDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".cursor", "mcp.json"), []byte(cursorMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// Default multi-source import (ProjectOnly = false)
	res, err := importer.RunImport(importer.ImportOptions{
		ProjectPath: projDir,
		HomeDir:     homeDir,
		ProjectOnly: false,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// Manifest targets both agents: [claude-code, cursor]
	t.Logf("Emitted agents: %v", res.Manifest.Agents)

	valErrs := manifest.Validate(res.Manifest, projDir)
	if len(valErrs) > 0 {
		t.Fatalf("manifest.Validate failed: %v", valErrs)
	}

	drift, err := sync.DetectProjectDrift(res.Manifest, projDir)
	if err != nil {
		t.Fatalf("DetectProjectDrift failed: %v", err)
	}

	t.Logf("Project drift for multi-source: InSync=%v, Items=%+v", drift.InSync, drift.Items)
}

// Test 4: Auth structures roundtrip and templatization
func TestStress_AuthStructures_RoundTripAndRedaction(t *testing.T) {
	tempDir := t.TempDir()

	mcpJSON := `{
		"mcpServers": {
			"string-auth-srv": {
				"url": "https://api.example.com/mcp",
				"auth": "Bearer super-secret-auth-token-12345"
			},
			"map-auth-srv": {
				"url": "https://api2.example.com/mcp",
				"auth": {
					"type": "bearer",
					"token": "ghp_123456789012345678901234567890123456",
					"custom_field": 42
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := importer.RunImport(importer.ImportOptions{
		ProjectPath: tempDir,
		ProjectOnly: true,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("RunImport failed: %v", err)
	}

	// 1. Verify secrets templatized
	strSrv := res.Manifest.MCPServers["string-auth-srv"]
	if strAuth, ok := strSrv.Auth.(string); !ok || !strings.Contains(strAuth, "${") {
		t.Errorf("expected string auth to be templatized with ${...}, got %v", strSrv.Auth)
	}

	mapSrv := res.Manifest.MCPServers["map-auth-srv"]
	if authMap, ok := mapSrv.Auth.(map[string]any); !ok {
		t.Fatalf("expected map auth, got %T: %v", mapSrv.Auth, mapSrv.Auth)
	} else {
		tokenVal, _ := authMap["token"].(string)
		if !strings.Contains(tokenVal, "${") {
			t.Errorf("expected token in map auth to be templatized with ${...}, got %v", tokenVal)
		}
		if authMap["custom_field"] != 42 && authMap["custom_field"] != float64(42) && authMap["custom_field"] != int64(42) {
			t.Errorf("expected custom_field to be preserved, got %v", authMap["custom_field"])
		}
	}

	// 2. Verify validation passes
	valErrs := manifest.Validate(res.Manifest, tempDir)
	if len(valErrs) > 0 {
		t.Fatalf("manifest.Validate failed: %v", valErrs)
	}

	// 3. Verify TOML roundtrip
	parsed, err := manifest.Parse(res.FormattedTOML, &manifest.ParseOptions{NoInterpolate: true})
	if err != nil {
		t.Fatalf("Parse roundtrip failed: %v", err)
	}

	roundtripMapSrv := parsed.Manifest.MCPServers["map-auth-srv"]
	if _, ok := roundtripMapSrv.Auth.(map[string]any); !ok {
		t.Errorf("expected map auth to survive TOML roundtrip, got %T: %v", roundtripMapSrv.Auth, roundtripMapSrv.Auth)
	}
}
