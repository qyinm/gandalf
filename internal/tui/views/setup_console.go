package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
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
	Description  string
	Author       string
	Category     string
	Version      string
	Provides     []string
	Actions      []SetupConsoleAction
	ConfigTarget string
}

// SetupConsoleView contains all data needed to render the setup console.
type SetupConsoleView struct {
	ActiveTab     string
	Tabs          []SetupConsoleTab
	Rows          []SetupConsoleRow
	Search        string
	SearchInput   string
	SearchFocused bool
	EmptyMessage  string
	Selected      *SetupConsoleDetail
	Confirmation  *SetupActionConfirmation
	ActionError   string
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
		renderSetupSearch(model),
		"",
	}
	if model.ActionError != "" {
		lines = append(lines, warnStyle.Render(model.ActionError), "")
	}

	listHeight := height - 10
	if model.Selected != nil {
		listHeight -= 5
	}
	if listHeight < 4 {
		listHeight = 4
	}
	lines = append(lines, renderSetupConsoleRows(model, width, listHeight))

	if model.Confirmation != nil {
		lines = append(lines, "")
		lines = append(lines, renderSetupActionConfirmation(*model.Confirmation)...)
	} else if model.Selected != nil {
		lines = append(lines, "")
		lines = append(lines, renderSetupConsoleDetail(*model.Selected, width)...)
	}

	lines = append(lines, "", renderSetupConsoleHelp(width))
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

func renderSetupSearch(model SetupConsoleView) string {
	if strings.TrimSpace(model.SearchInput) != "" {
		if model.SearchFocused {
			return activeStyle.Render(model.SearchInput)
		}
		return labelStyle.Render(model.SearchInput)
	}
	return labelStyle.Render("/ to search")
}

func renderSetupConsoleRows(model SetupConsoleView, width, height int) string {
	if model.EmptyMessage != "" {
		return mutedStyle.Render(model.EmptyMessage)
	}
	rows := make([]string, 0, len(model.Rows))
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
		rows = append(rows, style.Render(truncate(line, width-2)))
	}
	vp := viewport.New(width, height)
	vp.SetContent(strings.Join(rows, "\n"))
	return strings.TrimRight(vp.View(), "\n")
}

func renderSetupConsoleDetail(detail SetupConsoleDetail, width int) []string {
	lines := []string{
		titleStyle.Render(detail.Title),
		labelStyle.Render(fmt.Sprintf("%s · %s · %s", detail.AgentLabel, detail.ObjectKind, detail.Status)),
		mutedStyle.Render(truncate("source: "+detail.SourcePath, width)),
	}
	if detail.Scope != "" {
		lines = append(lines, mutedStyle.Render("scope: "+detail.Scope))
	}
	if detail.Description != "" {
		lines = append(lines, mutedStyle.Render(truncate(detail.Description, width)))
	}
	if detail.Author != "" || detail.Category != "" || detail.Version != "" {
		lines = append(lines, mutedStyle.Render(truncate(renderMetadataLine(detail), width)))
	}
	if len(detail.Provides) > 0 {
		lines = append(lines, mutedStyle.Render(truncate("provides: "+strings.Join(detail.Provides, ", "), width)))
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

func renderMetadataLine(detail SetupConsoleDetail) string {
	parts := make([]string, 0, 3)
	if detail.Version != "" {
		parts = append(parts, "version: "+detail.Version)
	}
	if detail.Author != "" {
		parts = append(parts, "author: "+detail.Author)
	}
	if detail.Category != "" {
		parts = append(parts, "category: "+detail.Category)
	}
	return strings.Join(parts, " · ")
}

type setupConsoleKeyMap struct {
	Tabs      key.Binding
	Search    key.Binding
	Rescan    key.Binding
	Action    key.Binding
	History   key.Binding
	Snapshots key.Binding
	Quit      key.Binding
}

func (m setupConsoleKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{m.Tabs, m.Search, m.Rescan, m.Action, m.History, m.Snapshots, m.Quit}
}

func (m setupConsoleKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{m.ShortHelp()}
}

func renderSetupConsoleHelp(width int) string {
	keyMap := setupConsoleKeyMap{
		Tabs:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "tabs")),
		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Rescan:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Action:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "action")),
		History:   key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "history")),
		Snapshots: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "snapshots")),
		Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
	helpView := help.New()
	helpView.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	helpView.Styles.ShortDesc = mutedStyle
	return truncate(helpView.ShortHelpView(keyMap.ShortHelp()), width)
}
