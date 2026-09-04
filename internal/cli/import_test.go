package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestCLIImport_Success(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Create project-level .mcp.json
	mcpJSON := `{
  "mcpServers": {
    "test-server": {
      "command": "npx",
      "args": ["-y", "@mcp/test", "postgres://user:pass@localhost:5432/mydb"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"import",
		"--project", projectPath,
		"--home", homeDir,
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	if !strings.Contains(stdout, "Successfully generated gandalf.toml") {
		t.Errorf("expected success message in stdout, got: %s", stdout)
	}

	// Verify gandalf.toml was created and contains templated DATABASE_URL
	manifestData, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatalf("failed to read generated gandalf.toml: %v", err)
	}

	manifestContent := string(manifestData)
	if !strings.Contains(manifestContent, "[mcp_servers.test-server]") {
		t.Errorf("expected test-server in gandalf.toml")
	}
	if !strings.Contains(manifestContent, "${DATABASE_URL}") {
		t.Errorf("expected secret URL to be replaced with ${DATABASE_URL}")
	}
	if !strings.Contains(manifestContent, "[env_template]") {
		t.Errorf("expected [env_template] section in gandalf.toml")
	}
}

func TestCLIImport_DryRun(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"demo": {"command": "node"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"import",
		"--project", projectPath,
		"--home", homeDir,
		"--dry-run",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	if !strings.Contains(stdout, "[DRY-RUN]") {
		t.Errorf("expected [DRY-RUN] in stdout, got: %s", stdout)
	}

	// Ensure no file was written
	if _, err := os.Stat(filepath.Join(projectPath, "gandalf.toml")); !os.IsNotExist(err) {
		t.Errorf("expected gandalf.toml NOT to exist on dry-run")
	}
}

func TestCLIImport_JSONOutput(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"json-tool": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"import",
		"--project", projectPath,
		"--home", homeDir,
		"--json",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("failed to parse json output: %v. Raw stdout: %s", err, stdout)
	}

	if parsed["outputFile"] != "gandalf.toml" {
		t.Errorf("expected outputFile 'gandalf.toml', got %v", parsed["outputFile"])
	}
}

func TestCLIImport_ForceOverwrite(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"tool": {"command": "node"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create pre-existing gandalf.toml
	if err := os.WriteFile(filepath.Join(projectPath, "gandalf.toml"), []byte("existing content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Running without --force should fail
	_, stderr, code := runCLI(t,
		"import",
		"--project", projectPath,
		"--home", homeDir,
	)

	if code == 0 {
		t.Fatalf("expected non-zero exit code when manifest exists without --force")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("expected 'already exists' in stderr, got: %s", stderr)
	}

	// Running with --force should succeed
	stdout, stderr, code := runCLI(t,
		"import",
		"--project", projectPath,
		"--home", homeDir,
		"--force",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0 with --force, got %d. Stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Successfully generated gandalf.toml") {
		t.Errorf("expected success with --force, got: %s", stdout)
	}
}

func TestCLIImport_ProjectOnly(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Project-level .cursor/mcp.json
	if err := os.MkdirAll(filepath.Join(projectPath, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	projMCP := `{"mcpServers": {"proj-server": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".cursor", "mcp.json"), []byte(projMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// Global-level ~/.cursor/mcp.json
	if err := os.MkdirAll(filepath.Join(homeDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	globalMCP := `{"mcpServers": {"global-server": {"command": "python"}}}`
	if err := os.WriteFile(filepath.Join(homeDir, ".cursor", "mcp.json"), []byte(globalMCP), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"import",
		"--project", projectPath,
		"--home", homeDir,
		"--project-only",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	content, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "proj-server") {
		t.Errorf("expected proj-server in manifest")
	}
	if strings.Contains(string(content), "global-server") {
		t.Errorf("expected global-server to be excluded with --project-only, stdout: %s", stdout)
	}
}

func TestShouldLaunchImportTUI(t *testing.T) {
	t.Parallel()
	withJSON := func(f *importFlags) { f.JSON = true }
	withDryRun := func(f *importFlags) { f.DryRun = true }
	withForce := func(f *importFlags) { f.Force = true }
	cases := []struct {
		name   string
		mutate func(*importFlags)
		isTTY  bool
		want   bool
	}{
		{"interactive default", nil, true, true},
		{"interactive with force", withForce, true, true},
		{"json stays headless", withJSON, true, false},
		{"dry-run stays headless", withDryRun, true, false},
		{"piped output stays headless", nil, false, false},
		{"piped json stays headless", withJSON, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := importFlags{}
			if tc.mutate != nil {
				tc.mutate(&flags)
			}
			if got := shouldLaunchImportTUI(&flags, tc.isTTY); got != tc.want {
				t.Errorf("shouldLaunchImportTUI(%+v, %v) = %v, want %v", flags, tc.isTTY, got, tc.want)
			}
		})
	}
}

func TestRunCLIImport_DoesNotLaunchTUIWhenPiped(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"piped-server": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Stub the TUI launch seam: it must NOT be called for captured (non-TTY)
	// output, otherwise automation would block on an interactive screen.
	launched := false
	previous := launchImportTUI
	launchImportTUI = func(_ types.RuntimeOptions, _ importer.ImportOptions) int {
		launched = true
		return 0
	}
	defer func() { launchImportTUI = previous }()

	_, stderr, code := runCLI(t,
		"import",
		"--project", projectPath,
		"--home", homeDir,
	)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}
	if launched {
		t.Errorf("TUI must not launch when stdout is not a terminal")
	}
}
