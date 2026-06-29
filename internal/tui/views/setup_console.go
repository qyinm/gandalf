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
	RowKind       string
	ParentID      string
	Depth         int
	Expanded      bool
	Toggleable    bool
	AgentLabel    string
	AgentMarker   string
	ObjectKind    string
	Name          string
	SourcePath    string
	Scope         string
	Status        string
	Entrypoint    string
	EntryStatus   string
	RuntimeStatus string
	Tools         []SetupConsoleTool
	ToolCount     int
	Description   string
	ActionLabel   string
	ToggleControl bool
	Disabled      bool
	Selected      bool
}

type SetupConsoleTool struct {
	Name        string
	Description string
}

// SetupConsoleBaselineRow summarizes baseline state for one supported agent.
type SetupConsoleBaselineRow struct {
	AgentMarker string
	Status      string
	Baseline    string
	Changes     string
	Unsupported string
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
	BaselineRows    []SetupConsoleBaselineRow
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

	// Wide terminals get a master-detail split: list left, detail right.
	const detailMinWidth = 32
	const splitMinWidth = 88
	listWidth := width
	detailWidth := 0
	if width >= splitMinWidth && model.Selected != nil {
		detailWidth = width / 3
		if detailWidth < detailMinWidth {
			detailWidth = detailMinWidth
		}
		listWidth = width - detailWidth - 1
	}

	lines := []string{
		renderSetupTabs(model.Tabs, listWidth),
		renderSetupSearch(model),
		"",
	}
	if len(model.BaselineRows) > 0 {
		lines = append(lines, renderSetupBaselineRows(model.BaselineRows, listWidth), "")
	}
	if model.ActionError != "" {
		lines = append(lines, warnStyle.Render(model.ActionError), "")
	}

	listHeight := height - 10
	if len(model.BaselineRows) > 0 {
		listHeight -= len(model.BaselineRows) + 1
	}
	if listHeight < 4 {
		listHeight = 4
	}
	lines = append(lines, renderSetupConsoleRows(model, listWidth, listHeight))

	// Narrow terminals stack the detail block below the list.
	if detailWidth == 0 && model.Selected != nil {
		lines = append(lines, "", divider(listWidth))
		lines = append(lines, renderSetupConsoleDetail(*model.Selected, listWidth)...)
	}

	if model.Confirmation != nil {
		lines = append(lines, "")
		lines = append(lines, renderSetupActionConfirmation(*model.Confirmation)...)
	}

	body := strings.Join(lines, "\n")
	if detailWidth > 0 {
		detail := strings.Join(renderSetupConsoleDetail(*model.Selected, detailWidth-2), "\n")
		left := lipgloss.NewStyle().Width(listWidth).Render(body)
		right := lipgloss.NewStyle().
			Width(detailWidth).
			Border(lipgloss.RoundedBorder(), false, false, false, true).
			BorderForeground(colorBorder).
			PaddingLeft(1).
			Render(detail)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	lines = []string{body, "", renderSetupConsoleHelp(model, width)}
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

func renderSetupBaselineRows(rows []SetupConsoleBaselineRow, width int) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		parts := []string{
			row.AgentMarker,
			row.Status,
			"baseline " + row.Baseline,
			row.Changes,
		}
		if row.Unsupported != "" {
			parts = append(parts, row.Unsupported)
		}
		lines = append(lines, labelStyle.Render(truncate(strings.Join(parts, "  "), width)))
	}
	return strings.Join(lines, "\n")
}

func renderSetupConsoleRows(model SetupConsoleView, width, height int) string {
	if model.EmptyMessage != "" {
		return mutedStyle.Render(model.EmptyMessage)
	}
	return renderSetupCompactRows(model, width, height)
}

func renderSetupCompactRows(model SetupConsoleView, width, height int) string {
	allRows := make([]string, 0, len(model.Rows))
	for _, row := range model.Rows {
		line := renderSetupCompactRow(row, model.ActiveTab, width)
		if row.Selected {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("236")).
				Render(line)
		} else {
			line = mutedStyle.Render(line)
		}
		allRows = append(allRows, line)
		if row.Expanded && shouldRenderSetupExpandedDetails(row, model.ActiveTab) {
			allRows = append(allRows, renderSetupExpandedRows(row, model.ActiveTab, width)...)
		}
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

func renderSetupCompactRow(row SetupConsoleRow, activeTab string, width int) string {
	if width < 20 {
		width = 20
	}
	origin := setupCompactOriginLabel(row, activeTab)
	prefix := "›"
	if row.Expanded {
		prefix = "⌄"
	}
	leftParts := []string{prefix}
	if dot := rowStateDot(row, activeTab); dot != "" {
		leftParts = append(leftParts, dot)
	}
	if strings.TrimSpace(row.AgentMarker) != "" && !isAgentOriginCompactTab(activeTab) {
		leftParts = append(leftParts, row.AgentMarker)
	}
	if strings.TrimSpace(row.ObjectKind) != "" && !isSkillsTab(activeTab) && !isAgentOriginCompactTab(activeTab) {
		leftParts = append(leftParts, row.ObjectKind)
	}
	leftPrefix := strings.Join(leftParts, " ")
	if !strings.HasSuffix(leftPrefix, " ") {
		leftPrefix += " "
	}
	nameWidth := width - ansi.StringWidth(origin) - ansi.StringWidth(leftPrefix) - 2
	if nameWidth < 8 {
		nameWidth = width - ansi.StringWidth(leftPrefix)
		origin = ""
	}
	nameValue := strings.Repeat("  ", max(0, row.Depth)) + row.Name
	if isMCPServerRow(row, activeTab) {
		nameValue += " " + mcpRuntimeBadge(row)
	}
	name := truncate(nameValue, nameWidth)
	left := leftPrefix + name
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(origin)
	if gap < 1 {
		gap = 1
	}
	return truncate(left+strings.Repeat(" ", gap)+origin, width)
}

// rowStateDot returns an on/off indicator for rows that carry a real toggle
// (MCP servers today). Other rows return "" so layout is unchanged.
func rowStateDot(row SetupConsoleRow, activeTab string) string {
	if !isMCPServerRow(row, activeTab) {
		return ""
	}
	if row.Disabled {
		return changedStyle.Render("○")
	}
	switch mcpRuntimeState(row) {
	case "unavailable", "missing", "disabled", "error":
		return removedStyle.Render("●")
	}
	return cleanStyle.Render("●")
}

func shouldRenderSetupExpandedDetails(row SetupConsoleRow, activeTab string) bool {
	if strings.EqualFold(activeTab, "marketplace") && row.RowKind == "marketplace_source" {
		return false
	}
	if isMCPServerRow(row, activeTab) {
		return false
	}
	return row.Selected
}

func renderSetupExpandedRows(row SetupConsoleRow, activeTab string, width int) []string {
	if isMCPToolRow(row) {
		return renderSetupMCPToolExpandedRows(row, width)
	}
	agentLabel := strings.TrimSpace(row.AgentLabel)
	if agentLabel == "" {
		agentLabel = setupAgentLabelFromMarker(row.AgentMarker)
	}
	lines := []string{
		"    " + truncate(fmt.Sprintf("%s · %s · %s", agentLabel, row.ObjectKind, row.Status), max(8, width-4)),
		"    " + truncate("source: "+row.SourcePath, max(8, width-4)),
	}
	if isMCPServerRow(row, activeTab) {
		if row.ToolCount > 0 {
			lines = append(lines, "    "+truncate(fmt.Sprintf("%d tools", row.ToolCount), max(8, width-4)))
		} else {
			lines = append(lines, "    tools unavailable")
		}
	}
	if isSkillsTab(activeTab) {
		entrypoint := strings.TrimSpace(row.Entrypoint)
		if entrypoint == "" {
			entrypoint = "SKILL.md"
		}
		lines = append(lines, "    "+truncate("entry: "+entrypoint, max(8, width-4)))
		if strings.TrimSpace(row.EntryStatus) != "" {
			lines = append(lines, "    "+truncate("entry status: "+row.EntryStatus, max(8, width-4)))
		}
	}
	if strings.TrimSpace(row.Scope) != "" {
		lines = append(lines, "    "+truncate("scope: "+row.Scope, max(8, width-4)))
	}
	if strings.TrimSpace(row.ActionLabel) != "" {
		lines = append(lines, "    "+truncate("actions: "+row.ActionLabel, max(8, width-4)))
	}
	footer := "    Enter action  |  Space collapse"
	if isSkillsTab(activeTab) {
		footer = "    Enter open markdown  |  Space collapse"
	} else if strings.EqualFold(activeTab, "mcp_servers") {
		footer = "    Enter collapse  |  Space collapse"
	} else if strings.EqualFold(activeTab, "marketplace") {
		footer = "    Enter collapse  |  Space collapse"
	}
	lines = append(lines, mutedStyle.Render(footer))
	for i := range lines {
		if i < len(lines)-1 {
			lines[i] = labelStyle.Render(lines[i])
		}
	}
	return lines
}

func renderSetupMCPToolExpandedRows(row SetupConsoleRow, width int) []string {
	description := strings.TrimSpace(row.Description)
	if description == "" {
		description = "No description available."
	}
	wrapped := wrapText(description, max(16, width-6))
	if len(wrapped) > 4 {
		wrapped = wrapped[:4]
	}
	lines := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		lines = append(lines, labelStyle.Render("      "+line))
	}
	return lines
}

func setupCompactOriginLabel(row SetupConsoleRow, activeTab string) string {
	if isSkillsTab(activeTab) {
		return setupSkillOriginLabel(row)
	}
	if isMCPToolRow(row) {
		return ""
	}
	if strings.EqualFold(activeTab, "marketplace") {
		if strings.TrimSpace(row.Status) != "" {
			return row.Status
		}
	}
	if agent := strings.TrimSpace(row.AgentLabel); agent != "" {
		return agent
	}
	if agent := setupAgentLabelFromMarker(row.AgentMarker); agent != "Unknown agent" {
		return agent
	}
	return ""
}

func isMCPServerRow(row SetupConsoleRow, activeTab string) bool {
	return isMCPServerTab(activeTab) && row.RowKind == "inventory"
}

func isMCPToolRow(row SetupConsoleRow) bool {
	return row.RowKind == "mcp_tool"
}

func isMCPServerTab(activeTab string) bool {
	return strings.EqualFold(activeTab, "mcp_servers")
}

func isAgentOriginCompactTab(activeTab string) bool {
	return strings.EqualFold(activeTab, "hooks") ||
		strings.EqualFold(activeTab, "plugins") ||
		strings.EqualFold(activeTab, "marketplace") ||
		isSkillsTab(activeTab) ||
		isMCPServerTab(activeTab)
}

func mcpRuntimeBadge(row SetupConsoleRow) string {
	status := mcpRuntimeState(row)
	switch status {
	case "ready", "available", "enabled", "ok":
		return "[ready]"
	case "unavailable", "missing", "disabled", "error":
		return "[unavailable]"
	default:
		return "[" + status + "]"
	}
}

func mcpRuntimeState(row SetupConsoleRow) string {
	status := strings.ToLower(strings.TrimSpace(row.RuntimeStatus))
	if status != "" {
		return status
	}
	if row.ToolCount > 0 || len(row.Tools) > 0 {
		return "ready"
	}
	return "unavailable"
}

func wrapText(value string, width int) []string {
	if width <= 0 {
		return nil
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		candidate := current + " " + word
		if ansi.StringWidth(candidate) > width {
			lines = append(lines, truncate(current, width))
			current = word
		} else {
			current = candidate
		}
	}
	if current != "" {
		lines = append(lines, truncate(current, width))
	}
	return lines
}

func setupAgentLabelFromMarker(marker string) string {
	switch strings.TrimSpace(marker) {
	case "CC":
		return "Claude Code"
	case "CX":
		return "Codex"
	case "PI":
		return "Pi Agent"
	case "CU":
		return "Cursor"
	default:
		if strings.TrimSpace(marker) == "" {
			return "Unknown agent"
		}
		return marker
	}
}

func setupSkillOriginLabel(row SetupConsoleRow) string {
	if plugin := setupSkillPluginName(row.SourcePath); plugin != "" {
		return "(plugin: " + plugin + ")"
	}
	switch strings.ToLower(strings.TrimSpace(row.Scope)) {
	case "project":
		return "(project)"
	case "managed":
		return "(managed)"
	}
	if strings.TrimSpace(row.SourcePath) == "<built-in>" {
		return "(built-in)"
	}
	return "(local)"
}

func setupSkillPluginName(sourcePath string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(sourcePath), "\\", "/")
	if normalized == "" || !strings.Contains(normalized, "/plugins/cache/") {
		return ""
	}
	parts := strings.Split(strings.Trim(normalized, "/"), "/")
	skillsIndex := -1
	for i, part := range parts {
		if part == "skills" {
			skillsIndex = i
			break
		}
	}
	if skillsIndex <= 0 {
		return ""
	}
	for i := skillsIndex - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part == "" || looksLikePluginVersion(part) {
			continue
		}
		return strings.TrimSuffix(part, "-plugin")
	}
	return ""
}

func looksLikePluginVersion(value string) bool {
	if value == "" {
		return false
	}
	hasDigit := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '.' || r == '-' || r == '_' || (r >= 'a' && r <= 'f'):
		default:
			return false
		}
	}
	return hasDigit
}

func isSkillsTab(activeTab string) bool {
	return strings.EqualFold(activeTab, "skills")
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
		lines = append(lines, "", labelStyle.Render("controls"))
		for _, action := range detail.Actions {
			lines = append(lines, controlLine(action, width))
		}
	}
	return lines
}

// controlLine renders one control with an availability marker: available
// controls read as actionable, gated controls explain why they wait.
func controlLine(action SetupConsoleAction, width int) string {
	if action.Available {
		verb := controlVerb(action.Label)
		return cleanStyle.Render(truncate("  ✓ "+verb, width))
	}
	label := "  ✗ " + action.Label
	if action.Reason != "" {
		label += " — " + action.Reason
	}
	return mutedStyle.Render(truncate(label, width))
}

func controlVerb(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "toggle":
		return "space  enable / disable"
	case "edit":
		return "e  edit config"
	case "remove":
		return "x  remove"
	default:
		return label
	}
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
	Tabs         key.Binding
	Search       key.Binding
	Rescan       key.Binding
	Baseline     key.Binding
	Action       key.Binding
	Toggle       key.Binding
	Environments key.Binding
	History      key.Binding
	Snapshots    key.Binding
	Quit         key.Binding
}

func (m setupConsoleKeyMap) ShortHelp() []key.Binding {
	bindings := []key.Binding{m.Tabs, m.Search, m.Rescan}
	if m.Toggle.Help().Key != "" {
		bindings = append(bindings, m.Toggle)
	}
	bindings = append(bindings, m.Action, m.Environments, m.Baseline, m.History, m.Snapshots, m.Quit)
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
	} else if strings.EqualFold(model.ActiveTab, "mcp_servers") {
		verb := "expand"
		toggleVerb := "enable/disable"
		if selected := selectedSetupConsoleRow(model.Rows); selected != nil {
			if selected.Expanded {
				verb = "collapse"
			}
			if selected.RowKind == "mcp_tool" || !selected.ToggleControl {
				toggleVerb = verb
			}
		}
		action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", verb))
		toggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", toggleVerb))
	} else if strings.EqualFold(model.ActiveTab, "skills") {
		if selected := selectedSetupConsoleRow(model.Rows); selected != nil && selected.Expanded {
			action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open markdown"))
			toggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "collapse"))
		} else {
			action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand"))
			toggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "expand"))
		}
	} else if strings.EqualFold(model.ActiveTab, "marketplace") {
		if selected := selectedSetupConsoleRow(model.Rows); selected != nil && selected.Toggleable {
			verb := "expand"
			if selected.Expanded {
				verb = "collapse"
			}
			action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", verb))
			toggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", verb))
		} else {
			action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "provider gated"))
		}
	} else if selected := selectedSetupConsoleRow(model.Rows); selected != nil && selected.Toggleable {
		if selected.Expanded {
			action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "action"))
			toggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "collapse"))
		} else {
			action = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand"))
			toggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "expand"))
		}
	}
	keyMap := setupConsoleKeyMap{
		Tabs:         key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "tabs")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Rescan:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Baseline:     key.NewBinding(key.WithKeys("B"), key.WithHelp("B", "baseline")),
		Action:       action,
		Toggle:       toggle,
		Environments: key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "environments")),
		History:      key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "history")),
		Snapshots:    key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "snapshots")),
		Quit:         key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
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
	bgLines := strings.Split(background, "\n")
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
