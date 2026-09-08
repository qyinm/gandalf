package boost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestResolver_Inheritance(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-proj",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCursor},
		Profiles: map[string]manifest.ProfileDef{
			"fe": {
				Skills: []string{"react-expert", "tailwind-design"},
			},
			"infra": {
				Skills: []string{"terraform-infra", "aws-audit"},
			},
			"fullstack": {
				Includes: []string{"fe", "infra"},
				Skills:   []string{"graphql-api"},
			},
		},
	}

	skills, err := ResolveSkills(m, "fullstack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"aws-audit", "graphql-api", "react-expert", "tailwind-design", "terraform-infra"}
	if len(skills) != len(expected) {
		t.Fatalf("expected %d skills, got %d: %v", len(expected), len(skills), skills)
	}
	for i, exp := range expected {
		if skills[i] != exp {
			t.Errorf("skill[%d]: expected %s, got %s", i, exp, skills[i])
		}
	}
}

func TestPlanner_RelativeSymlinksAndPruning(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Create dummy skills in project
	reactDir := filepath.Join(tmpDir, ".gandalf", "skills", "react-expert")
	tailwindDir := filepath.Join(tmpDir, ".gandalf", "skills", "tailwind-design")
	infraDir := filepath.Join(tmpDir, ".gandalf", "skills", "terraform-infra")

	_ = os.MkdirAll(reactDir, 0755)
	_ = os.MkdirAll(tailwindDir, 0755)
	_ = os.MkdirAll(infraDir, 0755)

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "my-app",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCursor},
		Skills: []manifest.SkillDef{
			{Name: "react-expert", Source: "./.gandalf/skills/react-expert"},
			{Name: "tailwind-design", Source: "./.gandalf/skills/tailwind-design"},
			{Name: "terraform-infra", Source: "./.gandalf/skills/terraform-infra"},
		},
		Profiles: map[string]manifest.ProfileDef{
			"frontend": {Skills: []string{"react-expert", "tailwind-design"}},
			"infra":    {Skills: []string{"terraform-infra"}},
		},
	}

	// 1. Boost frontend
	plan1, err := CreateBoostPlan(m, "frontend", ScopeProject, tmpDir, homeDir)
	if err != nil {
		t.Fatalf("CreateBoostPlan frontend failed: %v", err)
	}

	// 2 skills * 2 agents = 4 create mutations
	if len(plan1.Mutations) != 4 {
		t.Fatalf("expected 4 mutations, got %d", len(plan1.Mutations))
	}
	for _, mut := range plan1.Mutations {
		if mut.Action != ActionCreateSymlink {
			t.Errorf("expected ActionCreateSymlink, got: %s", mut.Action)
		}
		// Check relative symlink target
		expectedRel := filepath.Join("..", "..", ".gandalf", "skills", mut.SkillName)
		if mut.SymlinkTarget != expectedRel {
			t.Errorf("expected relative symlink target %q, got %q", expectedRel, mut.SymlinkTarget)
		}
	}

	res1, err := ApplyBoostPlan(plan1, tmpDir)
	if err != nil || !res1.Success {
		t.Fatalf("ApplyBoostPlan 1 failed: %v, errors: %v", err, res1.Errors)
	}
	if len(res1.Created) != 4 {
		t.Errorf("expected 4 created symlinks, got: %d", len(res1.Created))
	}

	// Verify symlinks on disk
	claudeReact := filepath.Join(tmpDir, ".claude", "skills", "react-expert")
	target, err := os.Readlink(claudeReact)
	if err != nil {
		t.Fatalf("readlink failed: %v", err)
	}
	expectedRel := filepath.Join("..", "..", ".gandalf", "skills", "react-expert")
	if target != expectedRel {
		t.Errorf("symlink target mismatch: got %q, want %q", target, expectedRel)
	}

	// 2. Switch to infra profile
	plan2, err := CreateBoostPlan(m, "infra", ScopeProject, tmpDir, homeDir)
	if err != nil {
		t.Fatalf("CreateBoostPlan infra failed: %v", err)
	}

	// Should remove react-expert & tailwind-design (4 removals), create terraform-infra (2 creates)
	removes := 0
	creates := 0
	for _, mut := range plan2.Mutations {
		if mut.Action == ActionRemoveSymlink {
			removes++
		}
		if mut.Action == ActionCreateSymlink {
			creates++
		}
	}
	if removes != 4 {
		t.Errorf("expected 4 removals, got %d", removes)
	}
	if creates != 2 {
		t.Errorf("expected 2 creates, got %d", creates)
	}

	res2, err := ApplyBoostPlan(plan2, tmpDir)
	if err != nil || !res2.Success {
		t.Fatalf("ApplyBoostPlan 2 failed: %v, errors: %v", err, res2.Errors)
	}

	// Verify previous links removed
	if _, err := os.Lstat(claudeReact); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed, but exists", claudeReact)
	}

	// Verify new link exists
	claudeInfra := filepath.Join(tmpDir, ".claude", "skills", "terraform-infra")
	if _, err := os.Lstat(claudeInfra); err != nil {
		t.Errorf("expected %s to exist", claudeInfra)
	}

	// 3. Clear profile
	planClear, err := CreateBoostPlan(m, "", ScopeProject, tmpDir, homeDir)
	if err != nil {
		t.Fatalf("CreateBoostPlan clear failed: %v", err)
	}
	resClear, err := ApplyBoostPlan(planClear, tmpDir)
	if err != nil || !resClear.Success {
		t.Fatalf("ApplyBoostPlan clear failed: %v", err)
	}

	if _, err := os.Lstat(claudeInfra); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed after clear, but exists", claudeInfra)
	}
}

func TestPlanner_RefuseRegularDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	reactDir := filepath.Join(tmpDir, ".gandalf", "skills", "react-expert")
	_ = os.MkdirAll(reactDir, 0755)

	// User created a REAL directory at .claude/skills/react-expert!
	userRealDir := filepath.Join(tmpDir, ".claude", "skills", "react-expert")
	_ = os.MkdirAll(userRealDir, 0755)
	_ = os.WriteFile(filepath.Join(userRealDir, "my-notes.txt"), []byte("important user work"), 0644)

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "my-app",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		Profiles: map[string]manifest.ProfileDef{
			"frontend": {Skills: []string{"react-expert"}},
		},
	}

	plan, err := CreateBoostPlan(m, "frontend", ScopeProject, tmpDir, homeDir)
	if err != nil {
		t.Fatalf("CreateBoostPlan failed: %v", err)
	}

	if len(plan.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(plan.Mutations))
	}
	if plan.Mutations[0].Action != ActionConflictSkip {
		t.Fatalf("expected ActionConflictSkip to protect user directory, got: %s", plan.Mutations[0].Action)
	}

	res, err := ApplyBoostPlan(plan, tmpDir)
	if err != nil {
		t.Fatalf("ApplyBoostPlan failed: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("expected 1 skipped item, got: %d", len(res.Skipped))
	}

	// Verify user's file is intact
	noteData, err := os.ReadFile(filepath.Join(userRealDir, "my-notes.txt"))
	if err != nil || string(noteData) != "important user work" {
		t.Errorf("user data was damaged or deleted!")
	}
}
