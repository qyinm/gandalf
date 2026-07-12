package views

import (
	"fmt"
	"strings"
)

// HomeView is the changes-first landing surface.
type HomeView struct {
	HasBaseline        bool
	HasMissingBaseline bool
	LastSnapshot       string
	TotalChanges       int
	SkillsChanged      int
	HooksChanged       int
	MCPServersChanged  int
	PluginsChanged     int
	OtherChanged       int
	TopChanges         []HomeChange
}

type HomeChange struct {
	Agent  string
	Kind   string
	Name   string
	Action string
}

func RenderHome(model HomeView, width, height int) string {
	if width < 1 {
		width = 1
	}
	if !model.HasBaseline {
		return renderHomeSections([]string{
			labelStyle.Render("○ No baseline yet"),
			mutedStyle.Render("Capture a baseline to measure setup drift."),
		}, []string{"", "[B] capture baseline  [i] setup", "[r] rescan  [q] quit"}, width, height)
	}

	narrow := width < 60
	short := height <= 12
	objectLabel := "objects"
	if model.TotalChanges == 1 {
		objectLabel = "object"
	}
	state := fmt.Sprintf("▲ %d setup %s changed", model.TotalChanges, objectLabel)
	if model.TotalChanges == 0 {
		state = "● No setup objects changed"
	}
	lines := []string{labelStyle.Render(state), mutedStyle.Render("since " + model.LastSnapshot)}
	if model.HasMissingBaseline {
		lines = append(lines, mutedStyle.Render("Some agents have no baseline."))
	}

	if narrow {
		lines = append(lines,
			fmt.Sprintf("skills %d · hooks %d", model.SkillsChanged, model.HooksChanged),
			fmt.Sprintf("mcp %d · plugins %d · other %d", model.MCPServersChanged, model.PluginsChanged, model.OtherChanged),
		)
	} else {
		lines = append(lines, fmt.Sprintf(
			"skills %d · hooks %d · mcp %d · plugins %d · other %d",
			model.SkillsChanged, model.HooksChanged, model.MCPServersChanged, model.PluginsChanged, model.OtherChanged,
		))
	}

	footer := []string{"", "[v] review  [R] rollback  [i] setup  [r] rescan  [q] quit"}
	if model.HasMissingBaseline {
		footer = []string{"", "[B] capture missing baselines  [v] review  [R] rollback", "[i] setup  [r] rescan  [q] quit"}
	}
	if narrow {
		footer = []string{"", "[v] review  [R] rollback", "[i] setup  [r] rescan", "[q] quit"}
		if model.HasMissingBaseline {
			footer = []string{"", "[B] capture missing baselines", "[v] review  [R] rollback", "[i] setup  [r] rescan", "[q] quit"}
		}
	}
	availableChanges := height - len(footer) - len(lines) - 1
	limit := min(5, max(0, availableChanges))
	if short {
		limit = min(limit, 2)
	}
	if len(model.TopChanges) > 0 && limit > 0 {
		lines = append(lines, "")
		for i, change := range model.TopChanges {
			if i >= limit {
				break
			}
			row := fmt.Sprintf("%s %s", homeChangeMarker(change.Action), change.Name)
			if !narrow {
				row += fmt.Sprintf("  %s · %s", change.Agent, change.Kind)
			}
			lines = append(lines, row)
		}
	}

	return renderHomeSections(lines, footer, width, height)
}

func homeChangeMarker(action string) string {
	switch action {
	case "added", "appeared":
		return "+"
	case "removed":
		return "-"
	default:
		return "~"
	}
}

func renderHomeLines(lines []string, width, height int) string {
	for i := range lines {
		lines[i] = truncate(lines[i], width)
	}
	return fitHeight(strings.Join(lines, "\n"), height)
}

func renderHomeSections(body, footer []string, width, height int) string {
	if height < 1 {
		height = 1
	}
	maxBody := height - len(footer)
	if maxBody < 0 {
		maxBody = 0
	}
	if len(body) > maxBody {
		body = body[:maxBody]
	}
	lines := append(append([]string(nil), body...), footer...)
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	return renderHomeLines(lines, width, height)
}
