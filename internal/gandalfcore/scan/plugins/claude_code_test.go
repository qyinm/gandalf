package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
)

func TestClaudeCodeScannerDiscoversPluginMarketplaces(t *testing.T) {
	homeDir := t.TempDir()
	writeFile(t, filepath.Join(homeDir, ".claude/plugins/known_marketplaces.json"), `{
		"empty-marketplace": {
			"source": {"source": "github", "repo": "example/empty"},
			"installLocation": "`+filepath.ToSlash(filepath.Join(homeDir, ".claude/plugins/marketplaces/empty-marketplace"))+`"
		},
		"openai-codex": {
			"source": {"source": "github", "repo": "openai/codex-plugin-cc"},
			"installLocation": "`+filepath.ToSlash(filepath.Join(homeDir, ".claude/plugins/marketplaces/openai-codex"))+`"
		}
	}`)
	writeFile(t, filepath.Join(homeDir, ".claude/plugins/installed_plugins.json"), `{
		"version": 2,
		"plugins": {
			"codex@openai-codex": [{
				"scope": "user",
				"version": "1.0.2"
			}]
		}
	}`)
	writeFile(t, filepath.Join(homeDir, ".claude/plugins/marketplaces/empty-marketplace/.claude-plugin/marketplace.json"), `{
		"name": "empty-marketplace",
		"plugins": []
	}`)
	writeFile(t, filepath.Join(homeDir, ".claude/plugins/marketplaces/openai-codex/.claude-plugin/marketplace.json"), `{
		"name": "openai-codex",
		"owner": {"name": "OpenAI"},
		"plugins": [{
			"name": "codex",
			"description": "Use Codex from Claude Code.",
			"version": "1.0.2",
			"author": {"name": "OpenAI"},
			"source": "./plugins/codex",
			"category": "development"
		}]
	}`)

	evidence := ClaudeCodeScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: t.TempDir(),
		HomeDir:     homeDir,
	})
	sources := setup.BuildMarketplace(evidence)

	if len(sources) != 2 {
		t.Fatalf("sources = %#v", sources)
	}
	empty := findMarketplaceSourceByLabel(t, sources, "empty-marketplace")
	if empty.Kind != setup.MarketplaceSourceMarketplace || len(empty.Entries) != 0 {
		t.Fatalf("empty marketplace = %#v", empty)
	}
	openai := findMarketplaceSourceByLabel(t, sources, "openai-codex")
	if openai.Kind != setup.MarketplaceSourceMarketplace || len(openai.Entries) != 1 {
		t.Fatalf("openai marketplace = %#v", openai)
	}
	entry := openai.Entries[0]
	if entry.Name != "codex" || !entry.Installed || entry.Status != "installed" {
		t.Fatalf("entry = %#v", entry)
	}
	if len(entry.Provides) != 1 || entry.Provides[0] != "plugin" {
		t.Fatalf("provides = %#v", entry.Provides)
	}
}

func TestClaudeCodeScannerEmitsInstalledPluginsAsInventory(t *testing.T) {
	homeDir := t.TempDir()
	writeFile(t, filepath.Join(homeDir, ".claude/plugins/installed_plugins.json"), `{
		"version": 2,
		"plugins": {
			"codex@openai-codex": [{"scope": "user", "version": "1.0.2"}],
			"vercel@claude-plugins-official": [{"scope": "user", "version": "1.0.0"}]
		}
	}`)

	evidence := ClaudeCodeScanner{}.Scan(&scan.ScannerContext{
		ProjectPath: t.TempDir(),
		HomeDir:     homeDir,
	})

	inventory := setup.BuildInventory(evidence)
	plugins := make(map[string]setup.InventoryItem)
	for _, item := range inventory {
		if item.ObjectKind == setup.ObjectPlugin {
			plugins[item.Name] = item
		}
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugin inventory items, got %d: %#v", len(plugins), plugins)
	}
	if _, ok := plugins["codex"]; !ok {
		t.Fatalf("expected codex plugin in inventory: %#v", plugins)
	}
	if _, ok := plugins["vercel"]; !ok {
		t.Fatalf("expected vercel plugin in inventory: %#v", plugins)
	}

	// Installed plugins are inventory, not extra marketplace sources.
	if sources := setup.BuildMarketplace(evidence); len(sources) != 0 {
		t.Fatalf("installed plugins must not appear as marketplace sources: %#v", sources)
	}
}

func findMarketplaceSourceByLabel(t *testing.T, sources []setup.MarketplaceSource, label string) setup.MarketplaceSource {
	t.Helper()
	for _, source := range sources {
		if source.Label == label {
			return source
		}
	}
	t.Fatalf("missing marketplace source %q: %#v", label, sources)
	return setup.MarketplaceSource{}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
