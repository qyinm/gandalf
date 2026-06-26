package views

import (
	"fmt"
	"strings"
)

// AgentInventoryRow is a rendered inventory row.
type AgentInventoryRow struct {
	Name   string
	Status string
}

// AgentHistoryRow is a rendered history row.
type AgentHistoryRow struct {
	ID         string
	ObservedAt string
	Title      string
}

// AgentDetailView is render input for per-agent inventory.
type AgentDetailView struct {
	Title        string
	ProfileLabel string
	Counts       struct {
		Skills       int
		McpServers   int
		Hooks        int
		Permissions  int
		EnvKeys      int
		Instructions int
	}
	Skills       []AgentInventoryRow
	McpServers   []AgentInventoryRow
	Hooks        []AgentInventoryRow
	EnvKeys      []AgentInventoryRow
	Instructions []AgentInventoryRow
	History      []AgentHistoryRow
	EmptyMessage string
}

// RenderAgentDetail renders the per-agent inventory workspace.
func RenderAgentDetail(model AgentDetailView, width, height int) string {
	var lines []string
	lines = append(lines, titleStyle.Render(model.Title))
	lines = append(lines, labelStyle.Render("profile: "+model.ProfileLabel))

	if model.EmptyMessage != "" {
		lines = append(lines, mutedStyle.Render(model.EmptyMessage))
		return fitHeight(strings.Join(lines, "\n"), height)
	}

	lines = append(lines, fmt.Sprintf("skills %d · mcp %d · hooks %d · permissions %d · env %d · instructions %d",
		model.Counts.Skills, model.Counts.McpServers, model.Counts.Hooks,
		model.Counts.Permissions, model.Counts.EnvKeys, model.Counts.Instructions))

	appendInventory := func(title string, rows []AgentInventoryRow) {
		if len(rows) == 0 {
			return
		}
		lines = append(lines, "", labelStyle.Render(title))
		for _, row := range rows {
			status := ""
			if row.Status != "" {
				status = " [" + row.Status + "]"
			}
			lines = append(lines, "  "+truncate(row.Name+status, width-4))
		}
	}

	appendInventory("Skills", model.Skills)
	appendInventory("MCP servers", model.McpServers)
	appendInventory("Hooks", model.Hooks)
	appendInventory("Env keys", model.EnvKeys)
	appendInventory("Instructions", model.Instructions)

	if len(model.History) > 0 {
		lines = append(lines, "", labelStyle.Render("Recent history"))
		for _, row := range model.History {
			lines = append(lines, fmt.Sprintf("  %s  %s  %s", row.ID, row.ObservedAt, truncate(row.Title, width-20)))
		}
	}

	return fitHeight(strings.Join(lines, "\n"), height)
}

func fitHeight(content string, height int) string {
	lines := strings.Split(content, "\n")
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
