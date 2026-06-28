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
