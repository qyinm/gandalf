package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderHeaderCollapsesChangedAgentsAtNarrowWidths(t *testing.T) {
	model := HeaderView{
		Title: "Gandalf",
		Scope: "/Users/hippoo",
		Chips: []HeaderChip{
			{AgentMarker: "CC", State: "changed", Detail: "64 changes", ChangeCount: 64},
			{AgentMarker: "CX", State: "changed", Detail: "645 changes", ChangeCount: 645},
		},
	}
	for _, width := range []int{40, 24} {
		rendered := RenderHeader(model, width)
		for _, line := range strings.Split(rendered, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d: line is %d cells: %q", width, got, line)
			}
		}
		if !strings.Contains(rendered, "709 changes") {
			t.Fatalf("width %d should show aggregate drift:\n%s", width, rendered)
		}
		if strings.Contains(rendered, "CC") || strings.Contains(rendered, "CX") {
			t.Fatalf("width %d should omit individual chips:\n%s", width, rendered)
		}
	}
}

func TestRenderHeaderCompactSummaryPreservesChangesAndMissingBaseline(t *testing.T) {
	rendered := RenderHeader(HeaderView{
		Title: "Gandalf",
		Scope: "/Users/hippoo",
		Chips: []HeaderChip{
			{AgentMarker: "CC", State: "missing", Detail: "no save"},
			{AgentMarker: "CX", State: "changed", Detail: "3 changes", ChangeCount: 3},
		},
	}, 40)
	if !strings.Contains(rendered, "3") || !strings.Contains(rendered, "no save") {
		t.Fatalf("mixed compact summary:\n%s", rendered)
	}
}

func TestRenderHeaderPreservesMissingBaselineWhenNarrow(t *testing.T) {
	rendered := RenderHeader(HeaderView{
		Title: "Gandalf",
		Scope: "/Users/hippoo",
		Chips: []HeaderChip{{AgentMarker: "CC", State: "missing", Detail: "no save"}},
	}, 24)
	if !strings.Contains(rendered, "no save") {
		t.Fatalf("missing save summary:\n%s", rendered)
	}
}
