package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// TimelineRow is a rendered timeline list row.
type TimelineRow struct {
	ShortID    string
	ObservedAt string
	EventKind  string
	Readiness  string
	Title      string
	Selected   bool
}

// TimelineDetail is selected-entry detail for rendering.
type TimelineDetail struct {
	Title              string
	EventKind          string
	Readiness          string
	Confidence         string
	BeforeSnapshotName string
	AfterSnapshotName  string
	Counts             string
	Highlights         []string
	WritableCount      int
	ObserveOnlyCount   int
}

// UndoWritableItem is a dry-run undo item row.
type UndoWritableItem struct {
	Action     string
	Path       string
	ServerName string
}

// UndoPreview is dry-run undo preview content.
type UndoPreview struct {
	Title                string
	WritesFiles          string
	WritableItems        []UndoWritableItem
	ObserveOnlyCount     int
	EmptyWritableMessage string
}

// CurrentSetup is the current-setup summary block.
type CurrentSetup struct {
	ScopeLabel    string
	Agents        int
	Skills        int
	McpServers    int
	Hooks         int
	Permissions   int
	EnvKeys       int
	SkillRows     []string
	McpServerRows []string
	HookRows      []string
	EnvKeyRows    []string
	Instructions  string
}

// HistoryView is render input for History > All changes.
type HistoryView struct {
	FilterLabel    string
	CurrentSetup   CurrentSetup
	EmptyMessage   string
	EmptyCommand   string
	CorruptWarning string
	Rows           []TimelineRow
	SelectedEntry  *TimelineDetail
	UndoPreview    *UndoPreview
}

// RenderHistory renders the History > All changes workspace panel.
func RenderHistory(model HistoryView, width, height int) string {
	if width < 40 {
		width = 40
	}
	if height < 12 {
		height = 12
	}

	var sections []string
	sections = append(sections, titleStyle.Render("History · All changes"))
	sections = append(sections, labelStyle.Render(fmt.Sprintf("Filter: %s", model.FilterLabel)))

	if model.CorruptWarning != "" {
		sections = append(sections, warnStyle.Render(model.CorruptWarning))
	}

	sections = append(sections, "")
	sections = append(sections, titleStyle.Render("Current setup"))
	sections = append(sections, renderCurrentSetup(model.CurrentSetup, width-4)...)

	sections = append(sections, "")
	sections = append(sections, titleStyle.Render("Timeline"))

	if model.EmptyMessage != "" {
		sections = append(sections, mutedStyle.Render(model.EmptyMessage))
		if model.EmptyCommand != "" {
			sections = append(sections, mutedStyle.Render(model.EmptyCommand))
		}
	} else {
		for _, row := range model.Rows {
			prefix := "  "
			style := mutedStyle
			if row.Selected {
				prefix = "▸ "
				style = activeStyle
			}
			line := fmt.Sprintf("%s%s  %s  %s  %s  %s",
				prefix, row.ShortID, row.ObservedAt, row.EventKind, row.Readiness, row.Title)
			sections = append(sections, style.Render(truncate(line, width-2)))
		}
	}

	if model.SelectedEntry != nil {
		sections = append(sections, "")
		sections = append(sections, renderTimelineDetail(*model.SelectedEntry)...)
	}
	if model.UndoPreview != nil {
		sections = append(sections, "")
		sections = append(sections, renderUndoPreview(*model.UndoPreview)...)
	}

	lines := strings.Split(strings.Join(sections, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func renderCurrentSetup(model CurrentSetup, width int) []string {
	lines := []string{
		labelStyle.Render(fmt.Sprintf("Scope: %s", model.ScopeLabel)),
		fmt.Sprintf("agents %d · skills %d · mcp %d · hooks %d · permissions %d · env %d",
			model.Agents, model.Skills, model.McpServers, model.Hooks, model.Permissions, model.EnvKeys),
		labelStyle.Render("instructions: " + model.Instructions),
	}
	appendRows := func(title string, rows []string) {
		if len(rows) == 0 {
			return
		}
		lines = append(lines, labelStyle.Render(title+":"))
		for _, row := range rows {
			lines = append(lines, "  "+truncate(row, width))
		}
	}
	appendRows("skills", model.SkillRows)
	appendRows("mcp", model.McpServerRows)
	appendRows("hooks", model.HookRows)
	appendRows("env", model.EnvKeyRows)
	return lines
}

func renderTimelineDetail(detail TimelineDetail) []string {
	lines := []string{
		titleStyle.Render("Selected event"),
		fmt.Sprintf("%s · %s · %s", detail.Title, detail.EventKind, detail.Readiness),
		labelStyle.Render(detail.Confidence),
		fmt.Sprintf("before %s → after %s", detail.BeforeSnapshotName, detail.AfterSnapshotName),
		labelStyle.Render(detail.Counts),
	}
	if len(detail.Highlights) > 0 {
		lines = append(lines, labelStyle.Render("highlights: "+strings.Join(detail.Highlights, ", ")))
	}
	if detail.WritableCount > 0 {
		lines = append(lines, labelStyle.Render(fmt.Sprintf("restorable surfaces: %d", detail.WritableCount)))
	}
	if detail.ObserveOnlyCount > 0 {
		lines = append(lines, labelStyle.Render(fmt.Sprintf("observe-only surfaces: %d", detail.ObserveOnlyCount)))
	}
	return lines
}

func renderUndoPreview(preview UndoPreview) []string {
	lines := []string{
		titleStyle.Render("Undo preview"),
		labelStyle.Render(preview.Title),
		labelStyle.Render("writes files: " + preview.WritesFiles),
	}
	if preview.EmptyWritableMessage != "" {
		lines = append(lines, mutedStyle.Render(preview.EmptyWritableMessage))
	}
	for _, item := range preview.WritableItems {
		lines = append(lines, fmt.Sprintf("  %s %s (%s)", item.Action, item.ServerName, item.Path))
	}
	if preview.ObserveOnlyCount > 0 {
		lines = append(lines, labelStyle.Render(fmt.Sprintf("observe-only: %d surface(s)", preview.ObserveOnlyCount)))
	}
	return lines
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width <= 3 {
		return "..."
	}
	return ansi.Truncate(value, width, "...")
}
