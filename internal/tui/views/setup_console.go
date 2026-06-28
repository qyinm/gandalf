package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// SetupConsoleTab is one top-level setup console tab.
type SetupConsoleTab struct {
	Label    string
	Count    int
	Selected bool
}

// SetupConsoleRow is one setup object row in the active tab.
type SetupConsoleRow struct {
	RowKind     string
	ParentID    string
	Depth       int
	Expanded    bool
	Toggleable  bool
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

// SetupMarkdownOverlay is a read-only markdown viewer rendered over the setup console.
type SetupMarkdownOverlay struct {
	Title      string
	Subtitle   string
	SourcePath string
	Body       string
	ErrorText  string
	Width      int
	Height     int
}

// SetupConsoleView contains all data needed to render the setup console.
type SetupConsoleView struct {
	ActiveTab       string
	Tabs            []SetupConsoleTab
	Rows            []SetupConsoleRow
	RowOffset       int
	Search          string
	SearchInput     string
	SearchFocused   bool
	EmptyMessage    string
	Selected        *SetupConsoleDetail
	Confirmation    *SetupActionConfirmation
	ActionError     string
	MarkdownOverlay *SetupMarkdownOverlay
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

	lines = append(lines, "", renderSetupConsoleHelp(model, width))
	rendered := fitHeight(strings.Join(lines, "\n"), height)
	if model.MarkdownOverlay != nil {
		return renderSetupConsoleWithOverlay(rendered, *model.MarkdownOverlay, width, height)
	}
	return rendered
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
	allRows := make([]string, 0, len(model.Rows))
	for _, row := range model.Rows {
		prefix := "  "
		style := mutedStyle
		if row.Selected {
			prefix = "> "
			style = activeStyle
		}
		indicator := " "
		if row.Toggleable {
			if row.Expanded {
				indicator = "-"
			} else {
				indicator = "+"
			}
		}
		indent := strings.Repeat("  ", max(0, row.Depth))
		name := indent + row.Name
		line := fmt.Sprintf("%s%s %-2s  %-13s  %-28s  %-10s  %-20s  %s",
			prefix,
			indicator,
			row.AgentMarker,
			row.ObjectKind,
			name,
			row.Status,
			row.SourcePath,
			row.ActionLabel,
		)
		allRows = append(allRows, style.Render(truncate(line, width-2)))
	}
	offset := model.RowOffset
	if offset < 0 {
		offset = 0
	}
	if offset > len(allRows) {
		offset = len(allRows)
	}
	end := offset + height
	if end > len(allRows) {
		end = len(allRows)
	}
	return strings.Join(allRows[offset:end], "\n")
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
	Toggle    key.Binding
	History   key.Binding
	Snapshots key.Binding
	Quit      key.Binding
}

func (m setupConsoleKeyMap) ShortHelp() []key.Binding {
	bindings := []key.Binding{m.Tabs, m.Search, m.Rescan}
	if m.Toggle.Help().Key != "" {
		bindings = append(bindings, m.Toggle)
	}
	bindings = append(bindings, m.Action, m.History, m.Snapshots, m.Quit)
	return bindings
}

func (m setupConsoleKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{m.ShortHelp()}
}

func renderSetupConsoleHelp(model SetupConsoleView, width int) string {
	action := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "action"))
	toggle := key.NewBinding()
	if model.MarkdownOverlay != nil {
		action = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close"))
		toggle = key.NewBinding(key.WithKeys("↑↓/jk"), key.WithHelp("↑↓/jk", "scroll"))
	} else if model.SearchFocused {
		action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "accept search"))
		toggle = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "blur"))
	} else if selected := selectedSetupConsoleRow(model.Rows); selected != nil && selected.Toggleable {
		verb := "expand"
		if selected.Expanded {
			verb = "collapse"
		}
		action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", verb))
		toggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", verb))
	} else if strings.EqualFold(model.ActiveTab, "marketplace") {
		action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "provider gated"))
	} else if strings.EqualFold(model.ActiveTab, "skills") {
		action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	}
	keyMap := setupConsoleKeyMap{
		Tabs:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "tabs")),
		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Rescan:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Action:    action,
		Toggle:    toggle,
		History:   key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "history")),
		Snapshots: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "snapshots")),
		Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
	helpView := help.New()
	helpView.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	helpView.Styles.ShortDesc = mutedStyle
	return truncate(helpView.ShortHelpView(keyMap.ShortHelp()), width)
}

func renderSetupConsoleWithOverlay(background string, overlay SetupMarkdownOverlay, width, height int) string {
	if overlay.Width <= 0 || overlay.Width > width {
		overlay.Width = max(20, width-2)
	}
	if overlay.Height <= 0 || overlay.Height > height {
		overlay.Height = max(8, height-2)
	}
	box := renderMarkdownOverlayBox(overlay)
	bgLines := strings.Split(fitHeight(ansi.Strip(background), height), "\n")
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}
	boxLines := strings.Split(box, "\n")
	top := max(0, (height-len(boxLines))/2)
	left := max(0, (width-overlay.Width)/2)
	for i, boxLine := range boxLines {
		target := top + i
		if target < 0 || target >= len(bgLines) {
			continue
		}
		bgLines[target] = overlayLine(bgLines[target], boxLine, left, width)
	}
	return strings.Join(bgLines, "\n")
}

func renderMarkdownOverlayBox(overlay SetupMarkdownOverlay) string {
	contentWidth := max(10, overlay.Width-4)
	bodyHeight := max(3, overlay.Height-6)
	lines := []string{
		titleStyle.Render(truncate(overlay.Title+"  [x]", contentWidth)),
		mutedStyle.Render(truncate(overlay.SourcePath, contentWidth)),
		labelStyle.Render(truncate(overlay.Subtitle, contentWidth)),
		"",
	}
	body := overlay.Body
	if strings.TrimSpace(body) == "" && overlay.ErrorText != "" {
		body = overlay.ErrorText
	}
	bodyLines := strings.Split(body, "\n")
	for len(bodyLines) < bodyHeight {
		bodyLines = append(bodyLines, "")
	}
	if len(bodyLines) > bodyHeight {
		bodyLines = bodyLines[:bodyHeight]
	}
	for i := range bodyLines {
		bodyLines[i] = truncate(bodyLines[i], contentWidth)
	}
	lines = append(lines, bodyLines...)
	lines = append(lines, mutedStyle.Render("↑↓/jk scroll  |  Esc close"))
	return lipgloss.NewStyle().
		Width(overlay.Width).
		Height(overlay.Height).
		Background(lipgloss.Color("235")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func overlayLine(background, foreground string, left, width int) string {
	if left < 0 {
		left = 0
	}
	background = truncate(background, width)
	prefix := ansi.Cut(background, 0, left)
	if prefixWidth := ansi.StringWidth(prefix); prefixWidth < left {
		prefix += strings.Repeat(" ", left-prefixWidth)
	}

	foregroundWidth := ansi.StringWidth(foreground)
	if left+foregroundWidth > width {
		foreground = ansi.Truncate(foreground, width-left, "")
		foregroundWidth = ansi.StringWidth(foreground)
	}
	suffixStart := left + foregroundWidth
	suffix := ""
	if suffixStart < width {
		suffix = ansi.Cut(background, suffixStart, width)
	}
	if lineWidth := ansi.StringWidth(prefix + foreground + suffix); lineWidth < width {
		suffix += strings.Repeat(" ", width-lineWidth)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, prefix, foreground, suffix)
}

func selectedSetupConsoleRow(rows []SetupConsoleRow) *SetupConsoleRow {
	for i := range rows {
		if rows[i].Selected {
			return &rows[i]
		}
	}
	return nil
}
