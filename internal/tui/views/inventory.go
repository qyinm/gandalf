package views

import (
	"fmt"
	"strings"
)

type SetupInventoryRow struct {
	AgentLabel  string
	AgentMarker string
	ObjectKind  string
	Name        string
	SourcePath  string
	ActionLabel string
	Selected    bool
}

type SetupInventoryView struct {
	Rows         []SetupInventoryRow
	Skills       int
	McpServers   int
	Hooks        int
	Plugins      int
	EmptyMessage string
	Confirmation *SetupActionConfirmation
	ActionError  string
}

type SetupActionConfirmation struct {
	Action       string
	AgentLabel   string
	ObjectKind   string
	TargetName   string
	Operation    string
	ConfigTarget string
	Command      string
}

func RenderSetupInventory(model SetupInventoryView, width, height int) string {
	if width < 40 {
		width = 40
	}

	lines := []string{
		titleStyle.Render("Global setup inventory"),
		fmt.Sprintf("skills %d · mcp %d · hooks %d · plugins %d",
			model.Skills, model.McpServers, model.Hooks, model.Plugins),
		"",
	}

	if model.EmptyMessage != "" {
		lines = append(lines, mutedStyle.Render(model.EmptyMessage))
		return fitHeight(strings.Join(lines, "\n"), height)
	}
	if model.ActionError != "" {
		lines = append(lines, warnStyle.Render(model.ActionError), "")
	}

	for _, row := range model.Rows {
		prefix := "  "
		style := mutedStyle
		if row.Selected {
			prefix = "> "
			style = activeStyle
		}
		line := fmt.Sprintf("%s%-2s  %-6s  %-28s  %-20s  %s",
			prefix,
			row.AgentMarker,
			row.ObjectKind,
			row.Name,
			row.SourcePath,
			row.ActionLabel,
		)
		lines = append(lines, style.Render(truncate(line, width-2)))
	}

	if model.Confirmation != nil {
		lines = append(lines, "")
		lines = append(lines, renderSetupActionConfirmation(*model.Confirmation)...)
	} else {
		lines = append(lines, "", labelStyle.Render("Enter action · r rescan"))
	}
	return fitHeight(strings.Join(lines, "\n"), height)
}

func renderSetupActionConfirmation(model SetupActionConfirmation) []string {
	return []string{
		titleStyle.Render("Confirm setup action"),
		fmt.Sprintf("%s %s: %s", model.Action, model.ObjectKind, model.TargetName),
		labelStyle.Render("agent: " + model.AgentLabel),
		labelStyle.Render("operation: " + model.Operation),
		labelStyle.Render("target: " + model.ConfigTarget),
		labelStyle.Render("command: " + model.Command),
		mutedStyle.Render("Enter confirm · esc cancel"),
	}
}
