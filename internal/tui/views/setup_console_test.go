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

func TestRenderSetupConsoleShowsSkillsOpenHelp(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "skills",
		Tabs: []SetupConsoleTab{
			{Label: "Skills", Count: 1, Selected: true},
		},
		Rows: []SetupConsoleRow{{
			AgentMarker: "CX",
			ObjectKind:  "skill",
			Name:        "review",
			Status:      "local",
			SourcePath:  "~/.codex/skills/review",
			Selected:    true,
		}},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 100, 20))
	if !strings.Contains(rendered, "enter open") {
		t.Fatalf("expected skill open help in view:\n%s", rendered)
	}
}

func TestRenderSetupConsoleShowsMarkdownOverlay(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "skills",
		Tabs: []SetupConsoleTab{
			{Label: "Skills", Count: 1, Selected: true},
		},
		Rows: []SetupConsoleRow{{
			AgentMarker: "CX",
			ObjectKind:  "skill",
			Name:        "review",
			Status:      "local",
			SourcePath:  "~/.codex/skills/review",
			Selected:    true,
		}},
		MarkdownOverlay: &SetupMarkdownOverlay{
			Title:      "review",
			Subtitle:   "codex skill / local",
			SourcePath: "~/.codex/skills/review/SKILL.md",
			Body:       "# Review\n\nUse this skill.",
			Width:      72,
			Height:     12,
		},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 100, 24))
	if !strings.Contains(rendered, "review") ||
		!strings.Contains(rendered, "~/.codex/skills/review/SKILL.md") ||
		!strings.Contains(rendered, "Use this skill.") {
		t.Fatalf("expected markdown overlay content in view:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Esc close") || !strings.Contains(rendered, "scroll") {
		t.Fatalf("expected overlay help in view:\n%s", rendered)
	}
}

func TestOverlayLinePreservesBackgroundOutsideOverlay(t *testing.T) {
	background := "left-side 0123456789 right-side"
	foreground := "BOX"

	rendered := overlayLine(background, foreground, 10, 32)

	if !strings.HasPrefix(rendered, "left-side ") {
		t.Fatalf("expected left background to remain: %q", rendered)
	}
	if !strings.Contains(rendered, "BOX") {
		t.Fatalf("expected overlay foreground: %q", rendered)
	}
	if !strings.Contains(rendered, "3456789 right-side") {
		t.Fatalf("expected right background to remain: %q", rendered)
	}
}
