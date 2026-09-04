package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// makeImportSandbox creates a project with a Claude Code .mcp.json (including a
// secret), a Cursor mcp.json, and a skills directory.
func makeImportSandbox(t *testing.T) (projectPath, homeDir string) {
	t.Helper()
	projectPath = t.TempDir()
	homeDir = t.TempDir()

	claudeMCP := `{
  "mcpServers": {
    "db-server": {
      "command": "npx",
      "args": ["-y", "@mcp/db", "postgres://user:pass@localhost:5432/app"]
    },
    "fs-server": {
      "command": "uvx",
      "args": ["fs-mcp"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(claudeMCP), 0644); err != nil {
		t.Fatal(err)
	}

	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	cursorMCP := `{
  "mcpServers": {
    "web-server": {"url": "https://mcp.example.com/sse"}
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(cursorMCP), 0644); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(projectPath, ".claude", "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# deploy"), 0644); err != nil {
		t.Fatal(err)
	}
	return projectPath, homeDir
}

func newTestImportApp(t *testing.T) (ImportApp, string, string) {
	t.Helper()
	projectPath, homeDir := makeImportSandbox(t)
	app := NewImportApp(
		types.RuntimeOptions{ProjectPath: projectPath, HomeDir: homeDir},
		importer.ImportOptions{},
	)
	return app, projectPath, homeDir
}

// scanApp drives the async scan phase to completion.
func scanApp(t *testing.T, app ImportApp) ImportApp {
	t.Helper()
	msg := app.Init()()
	model, _ := app.Update(msg)
	return model.(ImportApp)
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestImportApp_ScanBuildsGroups(t *testing.T) {
	app, _, _ := newTestImportApp(t)
	app = scanApp(t, app)

	if app.step != importStepSelect {
		t.Fatalf("expected select step after scan, got %v (err=%v)", app.step, app.err)
	}
	if app.itemCount() != 4 {
		t.Fatalf("expected 4 items (3 servers + 1 skill), got %d", app.itemCount())
	}

	agents := map[string]bool{}
	for _, g := range app.groups {
		agents[g.agent] = true
	}
	if !agents["Claude Code"] || !agents["Cursor"] {
		t.Errorf("expected Claude Code and Cursor groups, got %v", agents)
	}
}

func TestImportApp_NavigationWraps(t *testing.T) {
	app, _, _ := newTestImportApp(t)
	app = scanApp(t, app)

	if app.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", app.cursor)
	}
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyUp})
	app = model.(ImportApp)
	if app.cursor != app.itemCount()-1 {
		t.Errorf("expected up from 0 to wrap to last item, got %d", app.cursor)
	}
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = model.(ImportApp)
	if app.cursor != 0 {
		t.Errorf("expected j on last item to wrap to 0, got %d", app.cursor)
	}
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = model.(ImportApp)
	if app.cursor != 1 {
		t.Errorf("expected down to move to 1, got %d", app.cursor)
	}
}

func TestImportApp_SpaceTogglesSelection(t *testing.T) {
	app, _, _ := newTestImportApp(t)
	app = scanApp(t, app)

	gi, ii, ok := app.flatItem(0)
	if !ok {
		t.Fatal("no item at cursor 0")
	}
	item := app.groups[gi].items[ii]
	if item.kind != "server" {
		t.Fatalf("expected first item to be a server, got %q", item.kind)
	}
	if !app.selServers[item.name] {
		t.Fatal("expected item selected by default")
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeySpace})
	app = model.(ImportApp)
	if app.selServers[item.name] {
		t.Errorf("expected space to deselect %q", item.name)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	app = model.(ImportApp)
	if !app.selServers[item.name] {
		t.Errorf("expected second space to reselect %q", item.name)
	}
}

func TestImportApp_PreviewShowsMaskedManifest(t *testing.T) {
	app, _, _ := newTestImportApp(t)
	app = scanApp(t, app)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = model.(ImportApp)
	if app.step != importStepPreview {
		t.Fatalf("expected tab to enter preview, got %v", app.step)
	}

	preview := app.previewTOML()
	if !strings.Contains(preview, "[mcp_servers.db-server]") {
		t.Errorf("expected preview to contain db-server")
	}
	if !strings.Contains(preview, "${DATABASE_URL}") {
		t.Errorf("expected preview to mask the DB credential as ${DATABASE_URL}")
	}
	if strings.Contains(preview, "postgres://user:pass@") {
		t.Errorf("preview leaked a raw secret")
	}

	// Esc returns to selection.
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(ImportApp)
	if app.step != importStepSelect {
		t.Errorf("expected esc to return to select, got %v", app.step)
	}
}

func TestImportApp_PreviewReflectsDeselection(t *testing.T) {
	app, _, _ := newTestImportApp(t)
	app = scanApp(t, app)

	gi, ii, _ := app.flatItem(0)
	name := app.groups[gi].items[ii].name

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeySpace})
	app = model.(ImportApp)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = model.(ImportApp)

	if strings.Contains(app.previewTOML(), name) {
		t.Errorf("expected deselected server %q to disappear from preview", name)
	}
}

func TestImportApp_EnterWritesManifest(t *testing.T) {
	app, projectPath, _ := newTestImportApp(t)
	app = scanApp(t, app)

	// Deselect the cursor-0 server so the write must honor selection.
	gi, ii, _ := app.flatItem(0)
	dropped := app.groups[gi].items[ii].name
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeySpace})
	app = model.(ImportApp)

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(ImportApp)
	if app.step != importStepWriting {
		t.Fatalf("expected enter to start writing, got %v", app.step)
	}
	if cmd == nil {
		t.Fatal("expected a write command")
	}
	model, _ = app.Update(cmd())
	app = model.(ImportApp)

	if app.step != importStepDone {
		t.Fatalf("expected done after write, got %v (err=%v)", app.step, app.err)
	}

	data, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatalf("expected gandalf.toml to be written: %v", err)
	}
	content := string(data)
	if strings.Contains(content, dropped) {
		t.Errorf("written manifest contains deselected server %q", dropped)
	}
	if !strings.Contains(content, "[env_template]") {
		t.Errorf("expected [env_template] in written manifest")
	}
	if _, err := os.Stat(filepath.Join(projectPath, ".gandalf", "skills", "deploy", "SKILL.md")); err != nil {
		t.Errorf("expected skill to be mirrored: %v", err)
	}

	// Any key exits after done.
	if _, quit := app.Update(keyRunes("x")); quit == nil {
		t.Errorf("expected quit command after done")
	}
}

func TestImportApp_OverwriteRequiresConfirmation(t *testing.T) {
	app, projectPath, _ := newTestImportApp(t)
	if err := os.WriteFile(filepath.Join(projectPath, "gandalf.toml"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	app = scanApp(t, app)
	if !app.manifestExists {
		t.Fatal("expected manifestExists to be detected")
	}

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(ImportApp)
	if app.step != importStepConfirmOverwrite {
		t.Fatalf("expected overwrite confirmation, got %v", app.step)
	}
	if cmd != nil {
		t.Errorf("no write should start before confirmation")
	}

	// Decline returns to selection without touching the file.
	model, _ = app.Update(keyRunes("n"))
	app = model.(ImportApp)
	if app.step != importStepSelect {
		t.Errorf("expected n to return to select, got %v", app.step)
	}
	data, _ := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if string(data) != "existing" {
		t.Errorf("declined overwrite must not modify the manifest")
	}

	// Confirm overwrites.
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(ImportApp)
	model, cmd = app.Update(keyRunes("y"))
	app = model.(ImportApp)
	if app.step != importStepWriting {
		t.Fatalf("expected y to start writing, got %v", app.step)
	}
	model, _ = app.Update(cmd())
	app = model.(ImportApp)
	if app.step != importStepDone {
		t.Fatalf("expected done after confirmed overwrite, got %v (err=%v)", app.step, app.err)
	}
	data, _ = os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if string(data) == "existing" {
		t.Errorf("confirmed overwrite must rewrite the manifest")
	}
}

func TestImportApp_CancelWritesNothing(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("q")},
		{Type: tea.KeyEsc},
	} {
		app, projectPath, _ := newTestImportApp(t)
		app = scanApp(t, app)

		model, cmd := app.Update(key)
		app = model.(ImportApp)
		if !app.Cancelled() {
			t.Errorf("expected %q to cancel", key.String())
		}
		if cmd == nil {
			t.Errorf("expected quit command on cancel")
		}
		if _, err := os.Stat(filepath.Join(projectPath, "gandalf.toml")); !os.IsNotExist(err) {
			t.Errorf("cancelled import must not write gandalf.toml")
		}
		if _, err := os.Stat(filepath.Join(projectPath, ".gandalf")); !os.IsNotExist(err) {
			t.Errorf("cancelled import must not mirror skills")
		}
	}
}

func TestImportApp_ScanFailureIsTerminal(t *testing.T) {
	projectPath := t.TempDir() // no agent configs
	homeDir := t.TempDir()
	app := NewImportApp(
		types.RuntimeOptions{ProjectPath: projectPath, HomeDir: homeDir},
		importer.ImportOptions{},
	)
	app = scanApp(t, app)

	if app.step != importStepFailed {
		t.Fatalf("expected failed step when nothing is found, got %v", app.step)
	}
	if !app.Failed() || app.Err() == nil {
		t.Errorf("expected Failed() with a terminal error")
	}
}

func TestImportApp_FromFlagScansCustomSource(t *testing.T) {
	projectPath := t.TempDir()
	homeDir := t.TempDir()

	// Default locations hold nothing; only the --from target has a server.
	customDir := t.TempDir()
	customMCP := `{"mcpServers": {"custom-server": {"command": "npx", "args": ["custom"]}}}`
	customPath := filepath.Join(customDir, "custom.json")
	if err := os.WriteFile(customPath, []byte(customMCP), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewImportApp(
		types.RuntimeOptions{ProjectPath: projectPath, HomeDir: homeDir},
		importer.ImportOptions{FromPath: customPath},
	)
	app = scanApp(t, app)

	if app.step != importStepSelect {
		t.Fatalf("expected select step for --from scan, got %v (err=%v)", app.step, app.err)
	}
	if app.itemCount() != 1 || !app.selServers["custom-server"] {
		t.Errorf("expected exactly the custom source server, got %+v", app.selServers)
	}
}

func TestImportApp_WindowResizeUpdatesDimensions(t *testing.T) {
	app, _, _ := newTestImportApp(t)
	app = scanApp(t, app)

	model, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app = model.(ImportApp)
	if app.width != 120 || app.height != 40 {
		t.Errorf("expected resize to update dimensions, got %dx%d", app.width, app.height)
	}
	if view := app.View(); view == "" {
		t.Errorf("expected non-empty view after resize")
	}
}
