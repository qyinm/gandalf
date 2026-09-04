package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCmd_ProjectOnly_Success(t *testing.T) {
	tempDir := t.TempDir()

	manifestContent := `
version = "1.0"
name = "ci-team"
agents = ["claude-code"]

[mcp_servers.echo]
command = "echo"
args = ["${APP_ENV}"]

[env_template]
APP_ENV = "production"
`
	if err := os.WriteFile(filepath.Join(tempDir, "gandalf.toml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newCheckCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--project", tempDir, "--project-only", "--ci"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected check to succeed, got: %v, stderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "IN SYNC") {
		t.Errorf("expected IN SYNC in output, got: %s", stdout.String())
	}
}

func TestCheckCmd_ProjectOnly_FailsOnDrift(t *testing.T) {
	tempDir := t.TempDir()

	// Missing env_template for ${SECRET_KEY}
	manifestContent := `
version = "1.0"
name = "ci-team"
agents = ["codex"]

[mcp_servers.echo]
command = "echo"
args = ["${SECRET_KEY}"]
`
	if err := os.WriteFile(filepath.Join(tempDir, "gandalf.toml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newCheckCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--project", tempDir, "--project-only", "--ci"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected check to fail with exit code 1, but succeeded")
	}

	if !strings.Contains(stdout.String(), "DRIFT DETECTED") {
		t.Errorf("expected DRIFT DETECTED in output, got: %s", stdout.String())
	}

	// Verify GitHub workflow annotation
	if !strings.Contains(stderr.String(), "::error") || !strings.Contains(stderr.String(), "SECRET_KEY") {
		t.Errorf("expected ::error annotation for SECRET_KEY, got: %s", stderr.String())
	}
}

func TestCheckCmd_WritesGitHubStepSummary(t *testing.T) {
	tempDir := t.TempDir()
	summaryFile := filepath.Join(tempDir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryFile)

	manifestContent := `
version = "1.0"
name = "summary-test"
agents = ["cursor"]

[mcp_servers.srv]
command = "run"
`
	if err := os.WriteFile(filepath.Join(tempDir, "gandalf.toml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newCheckCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--project", tempDir, "--project-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("check failed: %v", err)
	}

	summaryBytes, err := os.ReadFile(summaryFile)
	if err != nil {
		t.Fatalf("failed to read GITHUB_STEP_SUMMARY: %v", err)
	}

	summary := string(summaryBytes)
	if !strings.Contains(summary, "Gandalf Agent Environment Check") {
		t.Errorf("expected header in summary, got:\n%s", summary)
	}
	if !strings.Contains(summary, "summary-test") {
		t.Errorf("expected manifest name in summary, got:\n%s", summary)
	}
}
