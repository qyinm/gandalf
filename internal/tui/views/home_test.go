package views

import (
	"strings"
	"testing"
)

func TestRenderHomeMissingBaseline(t *testing.T) {
	rendered := RenderHome(HomeView{}, 100, 24)
	for _, want := range []string{"What changed", "No baseline yet", "B capture baseline", "i setup console"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "R rollback") {
		t.Fatalf("rollback should not be offered without a baseline:\n%s", rendered)
	}
}

func TestRenderHomePresentBaseline(t *testing.T) {
	rendered := RenderHome(HomeView{
		HasBaseline: true, LastSnapshot: "2h ago", TotalChanges: 3,
		SkillsChanged: 1, MCPServersChanged: 1, PluginsChanged: 1,
		TopChanges: []HomeChange{{Agent: "Codex", Action: "added", Kind: "skill", Name: "review"}},
	}, 100, 24)
	for _, want := range []string{"3 changes since 2h ago", "Skills 1", "MCP 1", "Plugins 1", "Codex  added skill  review", "v review changes", "R rollback to baseline"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q:\n%s", want, rendered)
		}
	}
}
