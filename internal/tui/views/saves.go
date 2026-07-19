package views

import (
	"fmt"
	"strings"
)

// SaveRow is one saved setup in the Saves destination.
type SaveRow struct {
	AgentMarker string
	Name        string
	CreatedAt   string
	Selected    bool
}

// SavesView is render input for the Saves destination.
type SavesView struct {
	Rows []SaveRow
}

// RenderSaves renders the saves list.
func RenderSaves(model SavesView, width, height int) string {
	lines := []string{titleStyle.Render("Saves"), ""}
	if len(model.Rows) == 0 {
		lines = append(lines,
			mutedStyle.Render("No saves yet."),
			mutedStyle.Render("Save your current setup to measure drift and restore safely."),
			"",
			mutedStyle.Render("s save setup"),
		)
		return fitHeight(strings.Join(lines, "\n"), height)
	}
	for _, row := range model.Rows {
		line := fmt.Sprintf("%s  %s  %s", row.AgentMarker, row.Name, row.CreatedAt)
		if row.Selected {
			lines = append(lines, selectedRow.Render(truncate("› "+line, width)))
		} else {
			lines = append(lines, mutedStyle.Render(truncate("  "+line, width)))
		}
	}
	lines = append(lines, "", mutedStyle.Render("enter review restore · s save setup · ? keys"))
	return fitHeight(strings.Join(lines, "\n"), height)
}

// ReviewChangeRow is one change line inside the Review Changes modal.
type ReviewChangeRow struct {
	Marker string
	Kind   string
	Target string
}

// ReviewUnsupportedRow is one non-executable item with its concrete reason.
type ReviewUnsupportedRow struct {
	Kind   string
	Source string
	Reason string
}

// ReviewChangesView is the single review surface every mutation flows through.
type ReviewChangesView struct {
	Title       string
	Subtitle    string
	Notes       []string
	Changes     []ReviewChangeRow
	Unsupported []ReviewUnsupportedRow
	EmptyText   string
}

// RenderReviewChanges renders the unified Review Changes surface.
func RenderReviewChanges(model ReviewChangesView, width, height int) string {
	lines := []string{
		titleStyle.Render("Review Changes"),
		"",
		labelStyle.Render(truncate(model.Title, width)),
	}
	if model.Subtitle != "" {
		lines = append(lines, mutedStyle.Render(truncate(model.Subtitle, width)))
	}
	for _, note := range model.Notes {
		lines = append(lines, mutedStyle.Render(truncate(note, width)))
	}
	lines = append(lines, "", labelStyle.Render("Changes"))
	if len(model.Changes) == 0 {
		empty := model.EmptyText
		if empty == "" {
			empty = "No supported changes."
		}
		lines = append(lines, mutedStyle.Render("  "+empty))
	}
	for _, change := range model.Changes {
		style := changedStyle
		switch change.Marker {
		case "+":
			style = cleanStyle
		case "-":
			style = removedStyle
		}
		lines = append(lines, style.Render(truncate(fmt.Sprintf("  %s %s  %s", change.Marker, change.Kind, change.Target), width)))
	}
	if len(model.Unsupported) > 0 {
		lines = append(lines, "", labelStyle.Render("Unsupported"))
		for _, item := range model.Unsupported {
			lines = append(lines, mutedStyle.Render(truncate(fmt.Sprintf("  %s  %s — %s", item.Kind, item.Source, item.Reason), width)))
		}
	}
	lines = append(lines, "", mutedStyle.Render("enter apply · esc cancel"))
	return fitHeight(strings.Join(lines, "\n"), height)
}
