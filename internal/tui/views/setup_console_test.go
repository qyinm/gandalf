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
	if !strings.Contains(rendered, "›   codex") {
		t.Fatalf("expected child plugin row in view:\n%s", rendered)
	}
	if strings.Contains(rendered, "source: ~/.claude/plugins/marketplaces/openai-codex") ||
		strings.Contains(rendered, "add_source:unavailable") {
		t.Fatalf("expected marketplace source expansion to show entries without metadata details:\n%s", rendered)
	}
}

func TestRenderSetupConsoleShowsExpandedInventoryRowDetails(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "hooks",
		Tabs: []SetupConsoleTab{
			{Label: "Hooks", Count: 1, Selected: true},
		},
		Rows: []SetupConsoleRow{{
			AgentMarker: "CC",
			ObjectKind:  "hook",
			Name:        "PermissionRequest.*",
			SourcePath:  "~/.claude/settings.json",
			Scope:       "user",
			Status:      "user",
			ActionLabel: "edit:unavailable remove:unavailable",
			Expanded:    true,
			Toggleable:  true,
			Selected:    true,
		}},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 100, 22))
	if strings.Contains(rendered, "⌄ CC hook") {
		t.Fatalf("expected compact hook row without agent marker and kind:\n%s", rendered)
	}
	if !strings.Contains(rendered, "⌄ PermissionRequest.*") ||
		!strings.Contains(rendered, "source: ~/.claude/settings.json") ||
		!strings.Contains(rendered, "actions: edit:unavailable remove:unavailable") {
		t.Fatalf("expected expanded inventory details in view:\n%s", rendered)
	}
	if !strings.Contains(rendered, "enter action") || !strings.Contains(rendered, "space collapse") {
		t.Fatalf("expected expanded inventory help in view:\n%s", rendered)
	}
}

func TestRenderSetupConsoleHidesExpandedDetailsForUnselectedRows(t *testing.T) {
	view := SetupConsoleView{
		Tabs: []SetupConsoleTab{
			{Label: "Hooks", Count: 2, Selected: true},
		},
		ActiveTab: "hooks",
		Rows: []SetupConsoleRow{
			{
				RowKind:     "inventory",
				AgentLabel:  "Claude Code",
				AgentMarker: "CC",
				ObjectKind:  "hook",
				Name:        "old-hook",
				SourcePath:  "~/.claude/settings.json",
				Scope:       "user",
				Status:      "user",
				Expanded:    true,
			},
			{
				RowKind:     "inventory",
				AgentLabel:  "Claude Code",
				AgentMarker: "CC",
				ObjectKind:  "hook",
				Name:        "selected-hook",
				SourcePath:  "~/.claude/settings.json",
				Scope:       "user",
				Status:      "user",
				Selected:    true,
			},
		},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 100, 18))
	if strings.Contains(rendered, "source: ~/.claude/settings.json") {
		t.Fatalf("unselected expanded detail rendered:\n%s", rendered)
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
	if !strings.Contains(rendered, "enter expand") {
		t.Fatalf("expected skill expand help in view:\n%s", rendered)
	}
	if strings.Contains(rendered, "source:") || strings.Contains(rendered, "actions:") {
		t.Fatalf("expected compact skills browser without detail panel:\n%s", rendered)
	}
}

func TestRenderSetupConsoleShowsCompactPluginSkillRows(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "skills",
		Tabs: []SetupConsoleTab{
			{Label: "Skills", Count: 2, Selected: true},
		},
		Rows: []SetupConsoleRow{
			{
				ObjectKind: "skill",
				Name:       "ce-plan",
				SourcePath: "~/.codex/plugins/cache/compound-engineering-plugin/compound-engineering/3.15.0/skills/ce-plan",
				Scope:      "user",
				Selected:   true,
			},
			{
				ObjectKind: "skill",
				Name:       "review",
				SourcePath: "~/.codex/skills/review",
				Scope:      "user",
			},
		},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 100, 18))
	if strings.Contains(rendered, "› CC") || strings.Contains(rendered, "› CX") {
		t.Fatalf("expected compact skill rows without agent markers:\n%s", rendered)
	}
	if !strings.Contains(rendered, "› ce-plan") || !strings.Contains(rendered, "(plugin: compound-engineering)") {
		t.Fatalf("expected plugin skill row in view:\n%s", rendered)
	}
	if !strings.Contains(rendered, "› review") || !strings.Contains(rendered, "(local)") {
		t.Fatalf("expected local skill row in view:\n%s", rendered)
	}
	if strings.Contains(rendered, "edit:") || strings.Contains(rendered, "~/.codex/plugins/cache/compound-engineering-plugin") {
		t.Fatalf("expected compact rows without inventory columns:\n%s", rendered)
	}
}

func TestRenderSetupConsoleShowsExpandedSkillRowDetails(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "skills",
		Tabs: []SetupConsoleTab{
			{Label: "Skills", Count: 1, Selected: true},
		},
		Rows: []SetupConsoleRow{{
			Name:        "review",
			SourcePath:  "~/.codex/skills/review",
			Scope:       "user",
			Status:      "local",
			Entrypoint:  "SKILL.md",
			EntryStatus: "captured",
			ActionLabel: "edit:unavailable remove:unavailable",
			Expanded:    true,
			Toggleable:  true,
			Selected:    true,
		}},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 100, 22))
	if !strings.Contains(rendered, "⌄ review") ||
		!strings.Contains(rendered, "source: ~/.codex/skills/review") ||
		!strings.Contains(rendered, "entry: SKILL.md") ||
		!strings.Contains(rendered, "Enter open markdown") {
		t.Fatalf("expected expanded skill details in view:\n%s", rendered)
	}
	if !strings.Contains(rendered, "enter open markdown") || !strings.Contains(rendered, "space collapse") {
		t.Fatalf("expected expanded skill help in view:\n%s", rendered)
	}
}

func TestRenderSetupConsoleShowsAgentOriginForMCPRows(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "mcp_servers",
		Tabs: []SetupConsoleTab{
			{Label: "MCP Servers", Count: 2, Selected: true},
		},
		Rows: []SetupConsoleRow{
			{
				RowKind:     "inventory",
				AgentLabel:  "Codex",
				AgentMarker: "CX",
				ObjectKind:  "mcp",
				Name:        "context7",
				SourcePath:  "~/.codex/config.toml",
				Selected:    true,
				ToolCount:   2,
			},
			{
				RowKind:     "inventory",
				AgentLabel:  "Cursor",
				AgentMarker: "CU",
				ObjectKind:  "mcp",
				Name:        "posthog",
				SourcePath:  "~/.cursor/mcp.json",
			},
		},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 100, 18))
	if !strings.Contains(rendered, "context7 [ready]") || !strings.Contains(rendered, "Codex") {
		t.Fatalf("expected Codex MCP origin in view:\n%s", rendered)
	}
	if !strings.Contains(rendered, "posthog [unavailable]") || !strings.Contains(rendered, "Cursor") {
		t.Fatalf("expected Cursor MCP origin in view:\n%s", rendered)
	}
	if strings.Contains(rendered, "~/.codex/config.toml") || strings.Contains(rendered, "~/.cursor/mcp.json") {
		t.Fatalf("expected compact MCP rows to hide source paths:\n%s", rendered)
	}
}

func TestRenderSetupConsoleShowsMCPToolRowsAndDescription(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "mcp_servers",
		Tabs: []SetupConsoleTab{
			{Label: "MCP Servers", Count: 1, Selected: true},
		},
		Rows: []SetupConsoleRow{
			{
				RowKind:       "inventory",
				AgentLabel:    "Cursor",
				AgentMarker:   "CU",
				ObjectKind:    "mcp",
				Name:          "posthog",
				SourcePath:    "~/.cursor/mcp.json",
				RuntimeStatus: "ready",
				ToolCount:     140,
				Expanded:      true,
				Toggleable:    true,
			},
			{
				RowKind:     "mcp_tool",
				Depth:       1,
				ObjectKind:  "tool",
				Name:        "apm-attributes-list",
				Description: "List available span or resource attribute names before building filters.",
				Expanded:    true,
				Toggleable:  true,
				Selected:    true,
			},
		},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 110, 24))
	if !strings.Contains(rendered, "posthog [ready]") ||
		!strings.Contains(rendered, "apm-attributes-list") ||
		!strings.Contains(rendered, "List available span or resource attribute names") {
		t.Fatalf("expected MCP tools and description in view:\n%s", rendered)
	}
	if strings.Contains(rendered, "140 tools") {
		t.Fatalf("server detail should not push tool rows while a tool is selected:\n%s", rendered)
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

func TestRenderSetupConsoleShowsDetailPaneWhenWide(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "skills",
		Tabs:      []SetupConsoleTab{{Label: "Skills", Count: 1, Selected: true}},
		Rows: []SetupConsoleRow{
			{RowKind: "inventory", AgentMarker: "CC", ObjectKind: "skill", Name: "code-review", Selected: true},
		},
		Selected: &SetupConsoleDetail{
			Title:      "code-review",
			AgentLabel: "Claude Code",
			ObjectKind: "skill",
			Status:     "user",
			SourcePath: "~/.claude/skills/code-review",
		},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 120, 24))
	if !strings.Contains(rendered, "code-review") {
		t.Fatalf("expected detail title in wide render:\n%s", rendered)
	}
	if !strings.Contains(rendered, "~/.claude/skills/code-review") {
		t.Fatalf("expected detail source path in wide render:\n%s", rendered)
	}
}

func TestRenderSetupConsoleShowsMCPStateDot(t *testing.T) {
	view := SetupConsoleView{
		ActiveTab: "mcp_servers",
		Tabs:      []SetupConsoleTab{{Label: "MCP Servers", Count: 2, Selected: true}},
		Rows: []SetupConsoleRow{
			{RowKind: "inventory", AgentMarker: "CC", ObjectKind: "mcp", Name: "postgres", ToggleControl: true, Disabled: false, Selected: true},
			{RowKind: "inventory", AgentMarker: "CX", ObjectKind: "mcp", Name: "redis", ToggleControl: true, Disabled: true},
		},
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 80, 20))
	if !strings.Contains(rendered, "●") {
		t.Fatalf("expected enabled dot for MCP row:\n%s", rendered)
	}
	if !strings.Contains(rendered, "○") {
		t.Fatalf("expected disabled dot for MCP row:\n%s", rendered)
	}
}

func TestRenderSetupConsoleShowsBaselineStatus(t *testing.T) {
	view := SetupConsoleView{
		Tabs: []SetupConsoleTab{{Label: "Hooks", Count: 0, Selected: true}},
		BaselineRows: []SetupConsoleBaselineRow{
			{AgentMarker: "CC", Status: "missing baseline", Baseline: "-", Changes: "-"},
			{AgentMarker: "CX", Status: "changed", Baseline: "baseline-codex", Changes: "2 changes"},
		},
		EmptyMessage: "No global hooks found.",
	}

	rendered := ansi.Strip(RenderSetupConsole(view, 100, 24))
	if !strings.Contains(rendered, "CC  missing baseline  baseline -") {
		t.Fatalf("expected missing baseline row:\n%s", rendered)
	}
	if !strings.Contains(rendered, "CX  changed  baseline baseline-codex  2 changes") {
		t.Fatalf("expected changed baseline row:\n%s", rendered)
	}
	if !strings.Contains(rendered, "B baseline") {
		t.Fatalf("expected baseline key help:\n%s", rendered)
	}
}
