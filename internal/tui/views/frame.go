package views

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HeaderChip is one per-agent environment/drift indicator in the app header.
type HeaderChip struct {
	AgentMarker string
	State       string // "clean" | "changed" | "missing"
	Detail      string // e.g. "clean", "2 changes", "no baseline"
}

// HeaderView is the persistent top frame: app identity, scope, and per-agent drift.
type HeaderView struct {
	Title  string
	Scope  string
	Chips  []HeaderChip
	Notice string
	Warn   string
}

// RenderHeader renders the persistent app header with per-agent drift chips.
// "difference" is always visible here so it is never buried in a sub-screen.
func RenderHeader(model HeaderView, width int) string {
	title := titleStyle.Render(model.Title)
	if model.Scope != "" {
		title = lipgloss.JoinHorizontal(lipgloss.Top, title, mutedStyle.Render("  "+model.Scope))
	}

	chips := make([]string, 0, len(model.Chips))
	for _, chip := range model.Chips {
		dot := driftDot(chip.State)
		style := driftStyle(chip.State)
		chips = append(chips, style.Render(chip.AgentMarker+" "+dot+" "+chip.Detail))
	}
	chipLine := strings.Join(chips, mutedStyle.Render("   "))

	left := title
	gap := width - lipgloss.Width(left) - lipgloss.Width(chipLine)
	if gap < 2 {
		// Stack on narrow terminals and collapse agent chips into one truthful
		// summary instead of truncating the last agent off-screen.
		lines := []string{truncate(left, width)}
		if chipLine != "" {
			lines = append(lines, truncate(compactHeaderChipSummary(model.Chips), width))
		}
		return strings.Join(lines, "\n")
	}
	return truncate(left+strings.Repeat(" ", gap)+chipLine, width)
}

func compactHeaderChipSummary(chips []HeaderChip) string {
	total := 0
	hasMissing := false
	for _, chip := range chips {
		if chip.State == "missing" {
			hasMissing = true
		}
		if chip.State != "changed" {
			continue
		}
		fields := strings.Fields(chip.Detail)
		if len(fields) == 0 {
			continue
		}
		count, err := strconv.Atoi(fields[0])
		if err == nil {
			total += count
		}
	}
	if total > 0 {
		label := "changes"
		if total == 1 {
			label = "change"
		}
		return driftStyle("changed").Render(fmt.Sprintf("▲ %d %s", total, label))
	}
	if hasMissing {
		return driftStyle("missing").Render("○ no baseline")
	}
	return driftStyle("clean").Render("● clean")
}

func driftDot(state string) string {
	switch state {
	case "clean":
		return "●"
	case "changed":
		return "▲"
	default:
		return "○"
	}
}

// RenderFrame composes header, body, and an optional status line into one view,
// sizing the body to the remaining height.
func RenderFrame(header, body, status string, width, height int) string {
	parts := []string{header, divider(width)}
	used := 2
	if strings.TrimSpace(status) != "" {
		used++
	}
	bodyHeight := height - used
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	parts = append(parts, fitHeight(body, bodyHeight))
	if strings.TrimSpace(status) != "" {
		parts = append(parts, status)
	}
	return strings.Join(parts, "\n")
}

func divider(width int) string {
	if width < 1 {
		width = 1
	}
	return mutedStyle.Render(strings.Repeat("─", width))
}
