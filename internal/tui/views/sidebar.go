package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SidebarItem is one fixed navigation destination.
type SidebarItem struct {
	Key    string
	Label  string
	Active bool
}

// SidebarWidth is the fixed column width of the expanded sidebar.
const SidebarWidth = 14

// SidebarCollapseWidth is the terminal width below which the sidebar
// collapses into a horizontal strip under the header.
const SidebarCollapseWidth = 88

// RenderSidebar renders the persistent vertical destination list.
func RenderSidebar(items []SidebarItem, height int) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		label := item.Key + " " + item.Label
		if item.Active {
			lines = append(lines, focusStyle.Render(truncate("›"+label, SidebarWidth)))
		} else {
			lines = append(lines, mutedStyle.Render(truncate(" "+label, SidebarWidth)))
		}
	}
	body := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(SidebarWidth).Render(fitHeight(body, height))
}

// RenderSidebarStrip renders the collapsed one-line destination strip for
// narrow terminals.
func RenderSidebarStrip(items []SidebarItem, width int) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		label := item.Key + " " + item.Label
		if item.Active {
			parts = append(parts, focusStyle.Render("›"+label))
		} else {
			parts = append(parts, mutedStyle.Render(label))
		}
	}
	return truncate(strings.Join(parts, "  "), width)
}

// JoinSidebar composes the sidebar column and the screen body horizontally.
func JoinSidebar(sidebar, body string, bodyWidth int) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebar,
		mutedStyle.Render("│"),
		lipgloss.NewStyle().Width(bodyWidth).MaxWidth(bodyWidth).Render(body),
	)
}
