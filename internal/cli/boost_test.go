package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIBoostWorkflow(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	projectDir := filepath.Join(tempDir, "project")
	storeDir := filepath.Join(tempDir, "store")

	_ = os.MkdirAll(homeDir, 0755)
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.MkdirAll(storeDir, 0755)

	// Create dummy skills in project
	reactDir := filepath.Join(projectDir, ".gandalf", "skills", "react-expert")
	tailwindDir := filepath.Join(projectDir, ".gandalf", "skills", "tailwind-design")
	infraDir := filepath.Join(projectDir, ".gandalf", "skills", "terraform-infra")
	_ = os.MkdirAll(reactDir, 0755)
	_ = os.MkdirAll(tailwindDir, 0755)
	_ = os.MkdirAll(infraDir, 0755)
	_ = os.WriteFile(filepath.Join(reactDir, "SKILL.md"), []byte("---\nname: react-expert\n---\n"), 0644)
	_ = os.WriteFile(filepath.Join(tailwindDir, "SKILL.md"), []byte("---\nname: tailwind-design\n---\n"), 0644)
	_ = os.WriteFile(filepath.Join(infraDir, "SKILL.md"), []byte("---\nname: terraform-infra\n---\n"), 0644)

	manifestContent := `version = "1.0"
name = "boost-test"
agents = ["claude-code", "cursor"]

[[skills]]
name = "react-expert"
source = "./.gandalf/skills/react-expert"

[[skills]]
name = "tailwind-design"
source = "./.gandalf/skills/tailwind-design"

[[skills]]
name = "terraform-infra"
source = "./.gandalf/skills/terraform-infra"

[profiles.frontend]
description = "Frontend UI profile"
skills = ["react-expert", "tailwind-design"]

[profiles.infra]
description = "Cloud Infrastructure profile"
skills = ["terraform-infra"]
`
	_ = os.WriteFile(filepath.Join(projectDir, "gandalf.toml"), []byte(manifestContent), 0644)

	// 1. Test --list
	listCmd := newBoostCmd()
	var listBuf bytes.Buffer
	listCmd.SetOut(&listBuf)
	listCmd.SetErr(&listBuf)
	listCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--list"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("boost --list failed: %v", err)
	}
	if !strings.Contains(listBuf.String(), "frontend") || !strings.Contains(listBuf.String(), "infra") {
		t.Errorf("expected frontend and infra in list, got: %s", listBuf.String())
	}

	// 2. Test boost frontend
	boostCmd := newBoostCmd()
	var boostBuf bytes.Buffer
	boostCmd.SetOut(&boostBuf)
	boostCmd.SetErr(&boostBuf)
	boostCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "frontend"})
	if err := boostCmd.Execute(); err != nil {
		t.Fatalf("boost frontend failed: %v, output: %s", err, boostBuf.String())
	}

	// Check relative symlinks on disk
	claudeReact := filepath.Join(projectDir, ".claude", "skills", "react-expert")
	linkTarget, err := os.Readlink(claudeReact)
	if err != nil {
		t.Fatalf("expected symlink at %s, error: %v", claudeReact, err)
	}
	expectedRel := filepath.Join("..", "..", ".gandalf", "skills", "react-expert")
	if linkTarget != expectedRel {
		t.Errorf("expected relative link %q, got %q", expectedRel, linkTarget)
	}

	// 3. Test --status
	statusCmd := newBoostCmd()
	var statusBuf bytes.Buffer
	statusCmd.SetOut(&statusBuf)
	statusCmd.SetErr(&statusBuf)
	statusCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--status"})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("boost --status failed: %v", err)
	}
	if !strings.Contains(statusBuf.String(), "Active Profile: frontend") {
		t.Errorf("expected active profile frontend, got: %s", statusBuf.String())
	}

	// 4. Test switch to infra
	infraCmd := newBoostCmd()
	var infraBuf bytes.Buffer
	infraCmd.SetOut(&infraBuf)
	infraCmd.SetErr(&infraBuf)
	infraCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "infra"})
	if err := infraCmd.Execute(); err != nil {
		t.Fatalf("boost infra failed: %v", err)
	}

	// Verify react-expert unlinked, terraform-infra linked
	if _, err := os.Lstat(claudeReact); !os.IsNotExist(err) {
		t.Errorf("expected claudeReact to be removed, but exists")
	}
	claudeInfra := filepath.Join(projectDir, ".claude", "skills", "terraform-infra")
	if _, err := os.Lstat(claudeInfra); err != nil {
		t.Errorf("expected claudeInfra to exist, got: %v", err)
	}

	// 5. Test --clear
	clearCmd := newBoostCmd()
	var clearBuf bytes.Buffer
	clearCmd.SetOut(&clearBuf)
	clearCmd.SetErr(&clearBuf)
	clearCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--clear"})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("boost --clear failed: %v", err)
	}
	if _, err := os.Lstat(claudeInfra); !os.IsNotExist(err) {
		t.Errorf("expected claudeInfra to be removed after clear")
	}

	// 6. Test --setup-agents
	setupCmd := newBoostCmd()
	var setupBuf bytes.Buffer
	setupCmd.SetOut(&setupBuf)
	setupCmd.SetErr(&setupBuf)
	setupCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--setup-agents"})
	if err := setupCmd.Execute(); err != nil {
		t.Fatalf("boost --setup-agents failed: %v", err)
	}
	claudeCmdFile := filepath.Join(projectDir, ".claude", "commands", "boost.md")
	cursorRuleFile := filepath.Join(projectDir, ".cursor", "rules", "boost.mdc")
	boostSkillFile := filepath.Join(projectDir, ".gandalf", "skills", "boost", "SKILL.md")

	if _, err := os.Stat(claudeCmdFile); err != nil {
		t.Errorf("expected %s to exist", claudeCmdFile)
	}
	if _, err := os.Stat(cursorRuleFile); err != nil {
		t.Errorf("expected %s to exist", cursorRuleFile)
	}
	if _, err := os.Stat(boostSkillFile); err != nil {
		t.Errorf("expected %s to exist", boostSkillFile)
	}
}
