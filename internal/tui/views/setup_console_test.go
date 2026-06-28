package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderSetupConsoleShowsLastTabWhenStyled(t *testing.T) {
	view := SetupConsoleView{
		Tabs: []SetupConsoleTab{
			{Label: "Hooks", Count: 41},
			{Label: "Plugins", Count: 5},
			{Label: "Marketplace", Count: 39},
			{Label: "Skills", Count: 606},
			{Label: "MCP Servers", Count: 6, Selected: true},
		},
		EmptyMessage: "No matching mcp servers.",
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 80, 20))
	firstLine := strings.SplitN(rendered, "\n", 2)[0]
	if !strings.Contains(firstLine, "MCP Servers 6") {
		t.Fatalf("expected final tab to be visible in %q", firstLine)
	}
}

func TestRenderSetupConsoleShowsMarketplaceExpandHelpAndChildren(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "marketplace",
		Tabs: []SetupConsoleTab{
			{Label: "Marketplace", Count: 1, Selected: true},
		},
		Rows: []SetupConsoleRow{
			{
				RowKind:     "marketplace_source",
				Toggleable:  true,
				Expanded:    true,
				AgentMarker: "CC",
				ObjectKind:  "marketplace",
				Name:        "openai-codex",
				Status:      "1 entries / 1 installed",
				SourcePath:  "~/.claude/plugins/marketplaces/openai-codex",
				Selected:    true,
			},
			{
				RowKind:     "marketplace_entry",
				Depth:       1,
				AgentMarker: "CC",
				ObjectKind:  "plugin",
				Name:        "codex",
				Status:      "installed",
				SourcePath:  "~/.claude/plugins/marketplaces/openai-codex/codex",
			},
		},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 120, 24))
	if !strings.Contains(rendered, "space collapse") || !strings.Contains(rendered, "enter collapse") {
		t.Fatalf("expected collapse help in view:\n%s", rendered)
	}
	if !strings.Contains(rendered, "plugin           codex") {
		t.Fatalf("expected child plugin row in view:\n%s", rendered)
	}
}
