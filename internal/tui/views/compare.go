package views

import (
	"fmt"
	"strings"
)

// CompareSideBySideRow is a rendered compare row.
type CompareSideBySideRow struct {
	Marker string
	Before string
	After  string
}

// CompareSection is a rendered compare section.
type CompareSection struct {
	Title string
	Rows  []CompareSideBySideRow
}

// CompareView is render input for compare output.
type CompareView struct {
	FromLabel    string
	ToLabel      string
	ScopeLabel   string
	Summary      []string
	Sections     []CompareSection
	EmptyMessage string
}

// RenderCompare renders explicit From/To/Scope compare output.
func RenderCompare(model CompareView, width, height int) string {
	var lines []string
	lines = append(lines, titleStyle.Render("Compare"))
	lines = append(lines, labelStyle.Render("From: "+model.FromLabel))
	lines = append(lines, labelStyle.Render("To:   "+model.ToLabel))
	lines = append(lines, labelStyle.Render("Scope: "+model.ScopeLabel))

	if model.EmptyMessage != "" {
		lines = append(lines, "", mutedStyle.Render(model.EmptyMessage))
	}

	if len(model.Summary) > 0 {
		lines = append(lines, "", labelStyle.Render("Summary"))
		for _, item := range model.Summary {
			lines = append(lines, "  "+truncate(item, width-4))
		}
	}

	for _, section := range model.Sections {
		lines = append(lines, "", titleStyle.Render(section.Title))
		for _, row := range section.Rows {
			line := fmt.Sprintf("  %s  %-28s  %s", row.Marker, truncate(row.Before, 28), truncate(row.After, width-40))
			lines = append(lines, truncate(line, width))
		}
	}

	return fitHeight(strings.Join(lines, "\n"), height)
}