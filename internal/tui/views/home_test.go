package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderHomeMissingBaseline(t *testing.T) {
	rendered := RenderHome(HomeView{}, 100, 24)
	for _, want := range []string{"No saves yet", "s save setup", "2 console"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "enter review") {
		t.Fatalf("review should not be offered without a save:\n%s", rendered)
	}
}

func TestRenderHomePresentBaseline(t *testing.T) {
	rendered := RenderHome(HomeView{
		HasBaseline: true, LastSnapshot: "2h ago", TotalChanges: 4,
		SkillsChanged: 1, MCPServersChanged: 1, PluginsChanged: 1, OtherChanged: 1,
		TopChanges: []HomeChange{{Agent: "Codex", Action: "added", Kind: "skill", Name: "review"}},
	}, 100, 24)
	for _, want := range []string{"4 setup objects changed", "since last save · 2h ago", "skills 1", "mcp 1", "plugins 1", "other 1", "+ review", "Codex · skill", "enter review", "s save"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderHomeFitsResponsiveWidths(t *testing.T) {
	model := responsiveHomeFixture()
	for _, width := range []int{80, 36, 24} {
		rendered := RenderHome(model, width, 16)
		for _, line := range strings.Split(rendered, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d: line is %d cells: %q\n%s", width, got, line, rendered)
			}
		}
	}

	narrow := RenderHome(model, 36, 16)
	for _, want := range []string{"skills 2", "hooks 1", "mcp 1", "plugins 0", "other 1", "+ image-to-code", "enter review", "q quit"} {
		if !strings.Contains(narrow, want) {
			t.Fatalf("narrow view missing %q:\n%s", want, narrow)
		}
	}
	if strings.Contains(narrow, "Claude Code · skill") {
		t.Fatalf("narrow view should omit row metadata:\n%s", narrow)
	}
}

func TestRenderHomeReducesChangesBeforeCountsOnShortScreens(t *testing.T) {
	rendered := RenderHome(responsiveHomeFixture(), 60, 8)
	for _, want := range []string{"skills 2", "hooks 1", "mcp 1", "plugins 0", "other 1", "image-to-code", "StopFailure.*"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("short view missing %q:\n%s", want, rendered)
		}
	}
	for _, omitted := range []string{"design-taste-frontend", "full-output-enforcement", "config.toml"} {
		if strings.Contains(rendered, omitted) {
			t.Fatalf("short view should omit %q:\n%s", omitted, rendered)
		}
	}
	for _, action := range []string{"enter review", "s save", "? keys"} {
		if !strings.Contains(rendered, action) {
			t.Fatalf("short view should preserve %q:\n%s", action, rendered)
		}
	}
}

func responsiveHomeFixture() HomeView {
	return HomeView{
		HasBaseline: true, LastSnapshot: "Jun 29, 03:54", TotalChanges: 5,
		SkillsChanged: 2, HooksChanged: 1, MCPServersChanged: 1, OtherChanged: 1,
		TopChanges: []HomeChange{
			{Agent: "Claude Code", Action: "added", Kind: "skill", Name: "image-to-code"},
			{Agent: "Claude Code", Action: "changed", Kind: "hook", Name: "StopFailure.*"},
			{Agent: "Claude Code", Action: "added", Kind: "skill", Name: "design-taste-frontend"},
			{Agent: "Codex", Action: "changed", Kind: "mcp_server", Name: "full-output-enforcement"},
			{Agent: "Codex", Action: "changed", Kind: "agent_config", Name: "config.toml"},
		},
	}
}
