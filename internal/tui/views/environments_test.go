package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderEnvironmentsShowsSideBySideDiff(t *testing.T) {
	view := sideBySideEnvironmentFixture()

	rendered := ansi.Strip(RenderEnvironments(view, 120, 24))
	for _, expected := range []string{
		"Agents",
		"Surfaces",
		"Save",
		"Current",
		"@@ MCP aside",
		`1 - command: "old-aside"`,
		`1 + command: "new-aside"`,
		`2   env: {"A":"1"}`,
		`3 + timeout: 30`,
		"> @@",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected %q in side-by-side render:\n%s", expected, rendered)
		}
	}
	if !strings.Contains(rendered, "source: ~/.codex/config.toml") {
		t.Fatalf("expected source path context:\n%s", rendered)
	}
	if !strings.Contains(rendered, "v unified") {
		t.Fatalf("expected footer to show target toggles:\n%s", rendered)
	}
}

func TestRenderEnvironmentsKeepsFullWidthSideBySideWhenThreeColumnDiffWouldBeTooNarrow(t *testing.T) {
	rendered := ansi.Strip(RenderEnvironments(sideBySideEnvironmentFixture(), 150, 28))
	if !strings.Contains(rendered, "Save") || !strings.Contains(rendered, "Current") {
		t.Fatalf("expected side-by-side diff headers at 150 columns:\n%s", rendered)
	}
	if !hasLineWithAll(rendered, `command: "old-aside"`, `command: "new-aside"`, "│") {
		t.Fatalf("expected old/new values on the same side-by-side row:\n%s", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if ansi.StringWidth(line) > 150 {
			t.Fatalf("line overflows 150 columns (%d): %q\n%s", ansi.StringWidth(line), line, rendered)
		}
	}
}

func TestRenderChangesShowsCapabilityBadgesOnSurfaces(t *testing.T) {
	view := EnvironmentsView{
		Focus:      "surfaces",
		FocusAgent: "Codex",
		Rows: []EnvironmentRow{{
			AgentLabel: "Codex", AgentMarker: "CX", State: "changed", Detail: "3 changes", Selected: true,
		}},
		Surfaces: []EnvironmentSurface{
			{Marker: "~", Kind: "MCP", Name: "aside", Detail: "changed", Capability: "reviewable", Selected: true},
			{Marker: "-", Kind: "Hook", Name: "Stop", Detail: "removed", Capability: "restore-only"},
			{Marker: "~", Kind: "Source", Name: "~/.codex/config.toml", Detail: "changed", Capability: "read-only", CapabilityReason: "save has no captured content"},
		},
	}

	rendered := ansi.Strip(RenderEnvironments(view, 120, 24))
	for _, want := range []string{"[reviewable]", "[restore-only]", "[read-only · save has no captured content]"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q from Changes surfaces:\n%s", want, rendered)
		}
	}

	narrow := ansi.Strip(RenderEnvironments(view, 40, 24))
	for _, want := range []string{"[reviewable]", "[restore-only]", "[read-only]"} {
		if !strings.Contains(narrow, want) {
			t.Fatalf("narrow Changes dropped capability %q:\n%s", want, narrow)
		}
	}
}

func sideBySideEnvironmentFixture() EnvironmentsView {
	return EnvironmentsView{
		Focus: "diff",
		Mode:  "side_by_side",
		Rows: []EnvironmentRow{{
			AgentLabel:   "Codex",
			AgentMarker:  "CX",
			State:        "changed",
			BaselineName: "baseline-codex",
			Detail:       "1 change",
			Selected:     true,
		}},
		FocusAgent: "Codex",
		Surfaces: []EnvironmentSurface{{
			ID:         "mcp:aside",
			Marker:     "~",
			Kind:       "MCP",
			Name:       "aside",
			Detail:     "2 changes",
			SourcePath: "~/.codex/config.toml",
			Selected:   true,
		}},
		Diff: EnvironmentDiff{
			Title:      "MCP aside",
			SourcePath: "~/.codex/config.toml",
			Rows: []EnvironmentDiffRow{
				{Kind: "hunk", HunkIndex: 0, HunkTitle: "@@ MCP aside · ~/.codex/config.toml @@", CurrentHunk: true},
				{
					Kind: "changed", HunkIndex: 0,
					Left:  EnvironmentDiffSide{LineNumber: 1, Marker: "-", Text: `command: "old-aside"`},
					Right: EnvironmentDiffSide{LineNumber: 1, Marker: "+", Text: `command: "new-aside"`},
				},
				{
					Kind: "context", HunkIndex: 0,
					Left:  EnvironmentDiffSide{LineNumber: 2, Marker: " ", Text: `env: {"A":"1"}`},
					Right: EnvironmentDiffSide{LineNumber: 2, Marker: " ", Text: `env: {"A":"1"}`},
				},
				{
					Kind: "added", HunkIndex: 0,
					Right: EnvironmentDiffSide{LineNumber: 3, Marker: "+", Text: `timeout: 30`},
				},
			},
		},
	}
}

func hasLineWithAll(rendered string, parts ...string) bool {
	for _, line := range strings.Split(rendered, "\n") {
		matches := true
		for _, part := range parts {
			if !strings.Contains(line, part) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func TestRenderEnvironmentsFallsBackToUnifiedDiffWhenNarrow(t *testing.T) {
	view := EnvironmentsView{
		Focus:      "diff",
		Mode:       "side_by_side",
		FocusAgent: "Codex",
		Rows: []EnvironmentRow{{
			AgentLabel: "Codex", AgentMarker: "CX", State: "changed", Detail: "1 change", Selected: true,
		}},
		Surfaces: []EnvironmentSurface{{
			ID: "setup", Marker: "~", Kind: "Setup", Name: "config", Detail: "1 change", Selected: true,
		}},
		Diff: EnvironmentDiff{
			Title: "Setup config",
			Rows: []EnvironmentDiffRow{
				{Kind: "hunk", HunkIndex: 0, HunkTitle: "@@ Setup config @@", CurrentHunk: true},
				{
					Kind: "changed", HunkIndex: 0,
					Left:  EnvironmentDiffSide{LineNumber: 1, Marker: "-", Text: `model: "gpt-4"`},
					Right: EnvironmentDiffSide{LineNumber: 1, Marker: "+", Text: `model: "gpt-5"`},
				},
			},
		},
	}

	rendered := ansi.Strip(RenderEnvironments(view, 80, 24))
	for _, expected := range []string{"@@ Setup config", `1 - model: "gpt-4"`, `1 + model: "gpt-5"`} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected %q in narrow unified render:\n%s", expected, rendered)
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if ansi.StringWidth(line) > 80 {
			t.Fatalf("line overflows 80 columns (%d): %q\n%s", ansi.StringWidth(line), line, rendered)
		}
	}
}

func TestRenderEnvironmentsKeepsSelectedPickerRowsVisible(t *testing.T) {
	view := EnvironmentsView{
		Focus:      "surfaces",
		Mode:       "side_by_side",
		FocusAgent: "Codex",
		Rows:       []EnvironmentRow{{AgentLabel: "Codex", AgentMarker: "CX", State: "changed", Detail: "8 changes", Selected: true}},
		Surfaces: []EnvironmentSurface{
			{Marker: "~", Kind: "Skill", Name: "first", Detail: "1 change"},
			{Marker: "~", Kind: "Skill", Name: "second", Detail: "1 change"},
			{Marker: "~", Kind: "Skill", Name: "third", Detail: "1 change"},
			{Marker: "~", Kind: "Skill", Name: "last", Detail: "1 change", SourcePath: "~/.claude/skills/last", Selected: true},
		},
		Diff: EnvironmentDiff{Title: "Skill last", Rows: []EnvironmentDiffRow{{Kind: "hunk", HunkTitle: "@@ Skill last @@", CurrentHunk: true}}},
	}

	rendered := ansi.Strip(RenderEnvironments(view, 100, 18))
	if !strings.Contains(rendered, "last") || !strings.Contains(rendered, "~/.claude/skills/last") {
		t.Fatalf("selected surface should remain visible after scrolling:\n%s", rendered)
	}
	if strings.Contains(rendered, "› ~ Skill first") {
		t.Fatalf("top surface should have scrolled away:\n%s", rendered)
	}
}
