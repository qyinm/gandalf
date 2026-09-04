package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
)

func TestFilterManifest_NilSelectionReturnsUnchanged(t *testing.T) {
	m := &manifest.Manifest{
		MCPServers: map[string]manifest.MCPServerDef{
			"a": {Command: "npx"},
		},
	}
	if got := FilterManifest(m, nil); got != m {
		t.Fatalf("expected nil selection to return the manifest unchanged")
	}
}

func TestFilterManifest_DropsUnselectedServersAndSkills(t *testing.T) {
	m := &manifest.Manifest{
		MCPServers: map[string]manifest.MCPServerDef{
			"keep": {Command: "npx", Env: map[string]string{"KEY": "${KEEP_KEY}"}},
			"drop": {Command: "uvx", Env: map[string]string{"KEY": "${DROP_KEY}"}},
		},
		Skills: []manifest.SkillDef{
			{Name: "keep-skill"},
			{Name: "drop-skill"},
		},
		EnvTemplate: map[string]string{
			"KEEP_KEY": "sample",
			"DROP_KEY": "sample",
		},
	}
	sel := &Selection{
		Servers: map[string]bool{"keep": true, "drop": false},
		Skills:  map[string]bool{"keep-skill": true, "drop-skill": false},
	}

	got := FilterManifest(m, sel)

	if _, ok := got.MCPServers["keep"]; !ok {
		t.Errorf("expected 'keep' server to survive filtering")
	}
	if _, ok := got.MCPServers["drop"]; ok {
		t.Errorf("expected 'drop' server to be filtered out")
	}
	if len(got.Skills) != 1 || got.Skills[0].Name != "keep-skill" {
		t.Errorf("expected only keep-skill to survive, got %+v", got.Skills)
	}
	if _, ok := got.EnvTemplate["KEEP_KEY"]; !ok {
		t.Errorf("expected KEEP_KEY to remain in env_template (still referenced)")
	}
	if _, ok := got.EnvTemplate["DROP_KEY"]; ok {
		t.Errorf("expected DROP_KEY to be dropped from env_template (unreferenced)")
	}

	// Original must not be mutated.
	if len(m.MCPServers) != 2 || len(m.Skills) != 2 || len(m.EnvTemplate) != 2 {
		t.Errorf("FilterManifest mutated the source manifest")
	}
}

func TestFilterManifest_EnvRefsDetectedAcrossFields(t *testing.T) {
	m := &manifest.Manifest{
		MCPServers: map[string]manifest.MCPServerDef{
			"srv": {
				Command: "run ${CMD_VAR}",
				Args:    []string{"--token", "${ARG_VAR}"},
				URL:     "https://${HOST_VAR}/api",
				Env:     map[string]string{"A": "${ENV_VAR}"},
				Headers: map[string]string{"Authorization": "Bearer ${HEADER_VAR}"},
				EnvFile: "${FILE_VAR}.env",
				Auth:    map[string]any{"token": "${AUTH_VAR}"},
			},
		},
		EnvTemplate: map[string]string{
			"CMD_VAR":    "x",
			"ARG_VAR":    "x",
			"HOST_VAR":   "x",
			"ENV_VAR":    "x",
			"HEADER_VAR": "x",
			"FILE_VAR":   "x",
			"AUTH_VAR":   "x",
			"ORPHAN_VAR": "x",
		},
	}
	got := FilterManifest(m, &Selection{Servers: map[string]bool{"srv": true}})

	for _, want := range []string{"CMD_VAR", "ARG_VAR", "HOST_VAR", "ENV_VAR", "HEADER_VAR", "FILE_VAR", "AUTH_VAR"} {
		if _, ok := got.EnvTemplate[want]; !ok {
			t.Errorf("expected %s to remain referenced", want)
		}
	}
	if _, ok := got.EnvTemplate["ORPHAN_VAR"]; ok {
		t.Errorf("expected ORPHAN_VAR to be dropped")
	}
}

func TestRunImport_WithSelectionWritesFilteredManifest(t *testing.T) {
	projectPath := t.TempDir()
	homeDir := t.TempDir()

	mcpJSON := `{
  "mcpServers": {
    "alpha": {"command": "npx", "args": ["-y", "alpha"]},
    "beta": {"command": "uvx", "args": ["beta"]}
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// A skills directory with two skills; only one is selected.
	skillsDir := filepath.Join(projectPath, ".claude", "skills")
	for _, name := range []string{"skill-a", "skill-b"} {
		dir := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := RunImport(ImportOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		OutputFile:  "gandalf.toml",
		Selection: &Selection{
			Servers: map[string]bool{"alpha": true, "beta": false},
			Skills:  map[string]bool{"skill-a": true, "skill-b": false},
		},
	})
	if err != nil {
		t.Fatalf("RunImport with selection failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "[mcp_servers.alpha]") {
		t.Errorf("expected alpha server in written manifest")
	}
	if strings.Contains(content, "beta") {
		t.Errorf("expected beta server to be excluded from written manifest")
	}
	if !strings.Contains(content, `name = "skill-a"`) {
		t.Errorf("expected skill-a in written manifest")
	}
	if strings.Contains(content, "skill-b") {
		t.Errorf("expected skill-b to be excluded from written manifest")
	}

	// Deselected skill must not be mirrored to disk.
	if _, err := os.Stat(filepath.Join(projectPath, ".gandalf", "skills", "skill-a", "SKILL.md")); err != nil {
		t.Errorf("expected selected skill-a to be mirrored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectPath, ".gandalf", "skills", "skill-b")); !os.IsNotExist(err) {
		t.Errorf("expected deselected skill-b NOT to be mirrored")
	}

	if len(res.Manifest.MCPServers) != 1 {
		t.Errorf("expected result manifest to contain only the selected server, got %d", len(res.Manifest.MCPServers))
	}
}
