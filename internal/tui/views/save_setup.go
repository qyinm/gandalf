package views

import (
	"fmt"
	"strings"
)

// SaveSetupDestination is a save destination row.
type SaveSetupDestination struct {
	Label    string
	Selected bool
}

// SaveSetupView is render input for save-setup preview.
type SaveSetupView struct {
	Title           string
	DetectedChanges []string
	Destinations    []SaveSetupDestination
	NoChanges       bool
}

// RenderSaveSetup renders save-setup preview with deterministic title.
func RenderSaveSetup(model SaveSetupView, width, height int) string {
	var lines []string
	lines = append(lines, titleStyle.Render("Save setup"))
	lines = append(lines, labelStyle.Render("Title preview: "+model.Title))

	if model.NoChanges {
		lines = append(lines, mutedStyle.Render("No changes detected."))
	}

	if len(model.DetectedChanges) > 0 {
		lines = append(lines, "", labelStyle.Render("Detected changes"))
		for _, change := range model.DetectedChanges {
			lines = append(lines, "  "+truncate(change, width-4))
		}
	}

	if len(model.Destinations) > 0 {
		lines = append(lines, "", labelStyle.Render("Destinations"))
		for _, dest := range model.Destinations {
			marker := "[ ]"
			if dest.Selected {
				marker = "[x]"
			}
			lines = append(lines, fmt.Sprintf("  %s %s", marker, dest.Label))
		}
	}

	return fitHeight(strings.Join(lines, "\n"), height)
}