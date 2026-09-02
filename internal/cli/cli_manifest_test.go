package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInitCheckApply(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	projectDir := filepath.Join(tempDir, "project")
	storeDir := filepath.Join(tempDir, "store")

	_ = os.MkdirAll(homeDir, 0755)
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.MkdirAll(storeDir, 0755)

	// 1. Test gandalf init
	initCmd := newInitCmd()
	var initBuf bytes.Buffer
	initCmd.SetOut(&initBuf)
	initCmd.SetErr(&initBuf)
	initCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--name", "test-project"})

	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v, output: %s", err, initBuf.String())
	}

	manifestFile := filepath.Join(projectDir, "gandalf.toml")
	if _, err := os.Stat(manifestFile); err != nil {
		t.Fatalf("gandalf.toml was not generated: %v", err)
	}

	// 2. Test gandalf check (should detect drift before apply)
	checkCmd := newCheckCmd()
	var checkBuf bytes.Buffer
	checkCmd.SetOut(&checkBuf)
	checkCmd.SetErr(&checkBuf)
	checkCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir})

	if err := checkCmd.Execute(); err != nil {
		t.Fatalf("check failed: %v", err)
	}

	if !strings.Contains(checkBuf.String(), "DRIFT DETECTED") {
		t.Errorf("expected drift detected output, got: %s", checkBuf.String())
	}

	// 3. Test gandalf apply --dry-run
	dryApplyCmd := newApplyCmd()
	var dryBuf bytes.Buffer
	dryApplyCmd.SetOut(&dryBuf)
	dryApplyCmd.SetErr(&dryBuf)
	dryApplyCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--dry-run"})

	if err := dryApplyCmd.Execute(); err != nil {
		t.Fatalf("dry-run apply failed: %v", err)
	}
	if !strings.Contains(dryBuf.String(), "Dry-run mode") {
		t.Errorf("expected dry-run message, got: %s", dryBuf.String())
	}

	// 4. Test gandalf apply --yes
	applyCmd := newApplyCmd()
	var applyBuf bytes.Buffer
	applyCmd.SetOut(&applyBuf)
	applyCmd.SetErr(&applyBuf)
	applyCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--yes"})

	if err := applyCmd.Execute(); err != nil {
		t.Fatalf("apply failed: %v, output: %s", err, applyBuf.String())
	}

	if !strings.Contains(applyBuf.String(), "Successfully synchronized") {
		t.Errorf("expected success message, got: %s", applyBuf.String())
	}

	// 5. Check again (should be in sync now)
	checkCmd2 := newCheckCmd()
	var checkBuf2 bytes.Buffer
	checkCmd2.SetOut(&checkBuf2)
	checkCmd2.SetErr(&checkBuf2)
	checkCmd2.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir})

	if err := checkCmd2.Execute(); err != nil {
		t.Fatalf("second check failed: %v", err)
	}

	if !strings.Contains(checkBuf2.String(), "IN SYNC") {
		t.Errorf("expected in sync output, got: %s", checkBuf2.String())
	}
}
