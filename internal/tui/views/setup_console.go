package views

import (
	"fmt"
	"strings"
)

// SetupConsoleTab is one top-level setup console tab.
type SetupConsoleTab struct {
	Label    string
	Count    int
	Selected bool
}

// SetupConsoleRow is one setup object row in the active tab.
type SetupConsoleRow struct {
	AgentMarker string
	ObjectKind  string
	Name        string
	SourcePath  string
	Scope       string
	Status      string
	ActionLabel string
	Selected    bool
}

// SetupConsoleAction is one action exposed in selected-row detail.
type SetupConsoleAction struct {
	Label     string
	Available bool
	Reason    string
}

// SetupConsoleDetail is selected setup object detail.
type SetupConsoleDetail struct {
	Title        string
	AgentLabel   string
	ObjectKind   string
	SourcePath   string
	Scope        string
	Status       string
	Actions      []SetupConsoleAction
	ConfigTarget string
}

// SetupConsoleView contains all data needed to render the setup console.
type SetupConsoleView struct {
	ActiveTab    string
	Tabs         []SetupConsoleTab
	Rows         []SetupConsoleRow
	Search       string
	EmptyMessage string
	Selected     *SetupConsoleDetail
	Confirmation *SetupActionConfirmation
	ActionError  string
}

// RenderSetupConsole renders the default top-tab setup console.
func RenderSetupConsole(model SetupConsoleView, width, height int) string {
	if width < 40 {
		width = 40
	}
	if height < 12 {
		height = 12
	}

	lines := []string{
		renderSetupTabs(model.Tabs, width),
		labelStyle.Render(renderSearchLine(model.Search)),
		"",
	}
	if model.ActionError != "" {
		lines = append(lines, warnStyle.Render(model.ActionError), "")
	}

	if model.EmptyMessage != "" {
		lines = append(lines, mutedStyle.Render(model.EmptyMessage))
	} else {
		for _, row := range model.Rows {
			prefix := "  "
			style := mutedStyle
			if row.Selected {
				prefix = "> "
				style = activeStyle
			}
			line := fmt.Sprintf("%s%-2s  %-6s  %-28s  %-10s  %-20s  %s",
				prefix,
				row.AgentMarker,
				row.ObjectKind,
				row.Name,
				row.Status,
				row.SourcePath,
				row.ActionLabel,
			)
			lines = append(lines, style.Render(truncate(line, width-2)))
		}
	}

	if model.Confirmation != nil {
		lines = append(lines, "")
		lines = append(lines, renderSetupActionConfirmation(*model.Confirmation)...)
	} else if model.Selected != nil {
		lines = append(lines, "")
		lines = append(lines, renderSetupConsoleDetail(*model.Selected, width)...)
	}

	lines = append(lines, "", labelStyle.Render("Tab tabs · r rescan · Enter action · H history · S snapshots · q quit"))
	return fitHeight(strings.Join(lines, "\n"), height)
}

func renderSetupTabs(tabs []SetupConsoleTab, width int) string {
	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		label := fmt.Sprintf("%s %d", tab.Label, tab.Count)
		if tab.Selected {
			parts = append(parts, activeStyle.Render(label))
		} else {
			parts = append(parts, mutedStyle.Render(label))
		}
	}
	return truncate(strings.Join(parts, "  "), width)
}

func renderSearchLine(search string) string {
	if strings.TrimSpace(search) == "" {
		return "/ to search"
	}
	return "/ " + search
}

func renderSetupConsoleDetail(detail SetupConsoleDetail, width int) []string {
	lines := []string{
		titleStyle.Render(detail.Title),
		labelStyle.Render(fmt.Sprintf("%s · %s · %s", detail.AgentLabel, detail.ObjectKind, detail.Status)),
		mutedStyle.Render(truncate("source: "+detail.SourcePath, width)),
		mutedStyle.Render("scope: " + detail.Scope),
	}
	if detail.ConfigTarget != "" {
		lines = append(lines, mutedStyle.Render(truncate("target: "+detail.ConfigTarget, width)))
	}
	if len(detail.Actions) > 0 {
		actionLabels := make([]string, 0, len(detail.Actions))
		for _, action := range detail.Actions {
			label := action.Label
			if !action.Available {
				label += ":unavailable"
			}
			if action.Reason != "" {
				label += " (" + action.Reason + ")"
			}
			actionLabels = append(actionLabels, label)
		}
		lines = append(lines, mutedStyle.Render(truncate("actions: "+strings.Join(actionLabels, ", "), width)))
	}
	return lines
}
