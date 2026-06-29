package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// EnvironmentRow is one per-agent environment row in the snapshot workspace.
type EnvironmentRow struct {
	AgentLabel   string
	AgentMarker  string
	State        string // "clean" | "changed" | "missing"
	BaselineName string
	BaselineDate string
	Detail       string
	Selected     bool
}

// EnvironmentChange is one current-vs-baseline change in the focused agent.
type EnvironmentChange struct {
	Marker string
	Kind   string
	Name   string
	Detail string
}

// EnvironmentsView is render input for the Environments / snapshot workspace.
type EnvironmentsView struct {
	Rows         []EnvironmentRow
	FocusAgent   string
	Changes      []EnvironmentChange
	ChangesEmpty string
	EmptyMessage string
}

// RenderEnvironments renders the per-agent snapshot workspace: an environments
// list on top and the focused agent's current-vs-baseline changes below.
func RenderEnvironments(model EnvironmentsView, width, height int) string {
	if width < 40 {
		width = 40
	}

	lines := []string{titleStyle.Render("Environments")}
	if model.EmptyMessage != "" {
		lines = append(lines, "", mutedStyle.Render(model.EmptyMessage))
		return fitHeight(strings.Join(lines, "\n"), height)
	}

	for _, row := range model.Rows {
		prefix := "  "
		rowStyle := mutedStyle
		if row.Selected {
			prefix = "▸ "
			rowStyle = focusStyle
		}
		dot := driftDot(row.State)
		baseline := "no baseline"
		if row.BaselineName != "" {
			baseline = "baseline " + row.BaselineName
			if row.BaselineDate != "" {
				baseline += " · " + row.BaselineDate
			}
		}
		marker := row.AgentMarker + " " + driftStyle(row.State).Render(dot+" "+row.Detail)
		left := fmt.Sprintf("%s%-14s %s", prefix, row.AgentLabel, marker)
		right := labelStyle.Render(baseline)
		gap := width - 2 - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			gap = 2
		}
		lines = append(lines, rowStyle.Render(truncate(left, width-2))+strings.Repeat(" ", gap-1)+right)
	}

	lines = append(lines, "", divider(width-2))
	lines = append(lines, labelStyle.Render(fmt.Sprintf("%s · changes vs baseline", model.FocusAgent)))
	if model.ChangesEmpty != "" {
		lines = append(lines, mutedStyle.Render(model.ChangesEmpty))
	}
	for _, change := range model.Changes {
		line := fmt.Sprintf("  %s %-12s %-22s %s",
			change.Marker, change.Kind, truncate(change.Name, 22), change.Detail)
		lines = append(lines, changeMarkerStyle(change.Marker).Render(truncate(line, width-2)))
	}

	lines = append(lines, "", mutedStyle.Render(
		"↑↓ agent · s save baseline · R restore · u undo · i console · q quit"))
	return fitHeight(strings.Join(lines, "\n"), height)
}

func changeMarkerStyle(marker string) lipgloss.Style {
	switch marker {
	case "+":
		return cleanStyle
	case "-":
		return removedStyle
	default:
		return changedStyle
	}
}
