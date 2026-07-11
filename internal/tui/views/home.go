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
	TopChanges         []HomeChange
}

type HomeChange struct {
	Agent  string
	Kind   string
	Name   string
	Action string
}

func RenderHome(model HomeView, width, height int) string {
	lines := []string{titleStyle.Render("What changed")}
	if !model.HasBaseline {
		lines = append(lines,
			"",
			labelStyle.Render("No baseline yet"),
			mutedStyle.Render("Capture a baseline before Gandalf can show drift."),
			"",
			"B capture baseline   i setup console   r rescan   q quit",
		)
		return fitHeight(strings.Join(lines, "\n"), height)
	}

	changeLabel := "changes"
	if model.TotalChanges == 1 {
		changeLabel = "change"
	}
	state := fmt.Sprintf("%d %s since %s", model.TotalChanges, changeLabel, model.LastSnapshot)
	if model.TotalChanges == 0 {
		state = "No changes since " + model.LastSnapshot
	}
	lines = append(lines, "", labelStyle.Render(state))
	if model.HasMissingBaseline {
		lines = append(lines, mutedStyle.Render("Some supported agents do not have a baseline yet."))
	}
	lines = append(lines,
		"",
		fmt.Sprintf("Skills %d   Hooks %d   MCP %d   Plugins %d", model.SkillsChanged, model.HooksChanged, model.MCPServersChanged, model.PluginsChanged),
		"",
		labelStyle.Render("Top changes"),
	)
	if len(model.TopChanges) == 0 {
		lines = append(lines, mutedStyle.Render("  Current setup matches the baseline."))
	} else {
		for _, change := range model.TopChanges {
			lines = append(lines, truncate(fmt.Sprintf("  %s  %s %s  %s", change.Agent, change.Action, change.Kind, change.Name), width))
		}
	}
	lines = append(lines, "", "v review changes   R rollback to baseline   i setup console   r rescan   q quit")
	return fitHeight(strings.Join(lines, "\n"), height)
}
