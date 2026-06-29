package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	environmentFocusAgents        = "agents"
	environmentFocusSurfaces      = "surfaces"
	environmentFocusDiff          = "diff"
	environmentModeSideBySide     = "side_by_side"
	environmentModeUnified        = "unified"
	environmentRowHunk            = "hunk"
	environmentRowContext         = "context"
	environmentRowRemoved         = "removed"
	environmentRowAdded           = "added"
	environmentRowChanged         = "changed"
	environmentSideBySideMinWidth = 88
)

// EnvironmentRow is one per-agent environment row in the snapshot workspace.
type EnvironmentRow struct {
	AgentLabel   string
	AgentMarker  string
	State        string // "clean" | "changed" | "missing"
	BaselineName string
	BaselineDate string
	Detail       string
	Selected     bool
}

// EnvironmentSurface is one changed setup surface for the focused agent.
type EnvironmentSurface struct {
	ID          string
	Marker      string
	Kind        string
	Name        string
	Detail      string
	SourcePath  string
	ChangeCount int
	Selected    bool
}

// EnvironmentDiffSide is one side of a side-by-side diff row.
type EnvironmentDiffSide struct {
	LineNumber int
	Marker     string
	Text       string
}

// EnvironmentDiffRow is one typed row in the focused surface diff.
type EnvironmentDiffRow struct {
	ID          string
	Kind        string
	HunkIndex   int
	HunkTitle   string
	CurrentHunk bool
	Left        EnvironmentDiffSide
	Right       EnvironmentDiffSide
}

// EnvironmentDiff is the selected surface's baseline-vs-current diff.
type EnvironmentDiff struct {
	SurfaceID  string
	Title      string
	SourcePath string
	Rows       []EnvironmentDiffRow
}

// EnvironmentsView is render input for the Environments / snapshot workspace.
type EnvironmentsView struct {
	Rows         []EnvironmentRow
	FocusAgent   string
	Focus        string
	Mode         string
	DiffOffset   int
	Surfaces     []EnvironmentSurface
	Diff         EnvironmentDiff
	ChangesEmpty string
	EmptyMessage string
}

// RenderEnvironments renders the per-agent snapshot workspace: an environments
// list, changed setup surfaces, and a focused baseline-vs-current diff.
func RenderEnvironments(model EnvironmentsView, width, height int) string {
	if width < 40 {
		width = 40
	}

	if model.Focus == "" {
		model.Focus = environmentFocusAgents
	}
	if model.Mode == "" {
		model.Mode = environmentModeSideBySide
	}

	lines := []string{titleStyle.Render("Environments")}
	if model.EmptyMessage != "" {
		lines = append(lines, "", mutedStyle.Render(model.EmptyMessage))
		return fitHeight(strings.Join(lines, "\n"), height)
	}

	bodyHeight := max(4, height-3)
	var body string
	switch {
	case width >= 128 && len(model.Surfaces) > 0 && environmentWideDiffWidth(width) >= environmentSideBySideMinWidth:
		body = renderEnvironmentsWide(model, width, bodyHeight)
	case width >= environmentSideBySideMinWidth && len(model.Surfaces) > 0:
		body = renderEnvironmentsReview(model, width, bodyHeight)
	default:
		body = renderEnvironmentsStacked(model, width, bodyHeight)
	}
	lines = append(lines, body)
	lines = append(lines, "", mutedStyle.Render(truncate(renderEnvironmentHelp(model), width)))
	return fitHeight(strings.Join(lines, "\n"), height)
}

func environmentWideDiffWidth(width int) int {
	_, _, diffWidth := environmentWideColumnWidths(width)
	return diffWidth
}

func environmentWideColumnWidths(width int) (agentsWidth, surfacesWidth, diffWidth int) {
	agentsWidth = min(30, max(22, width/5))
	surfacesWidth = min(40, max(28, width/4))
	diffWidth = max(48, width-agentsWidth-surfacesWidth-4)
	return agentsWidth, surfacesWidth, diffWidth
}

func renderEnvironmentsWide(model EnvironmentsView, width, height int) string {
	agentsWidth, surfacesWidth, diffWidth := environmentWideColumnWidths(width)
	agents := renderEnvironmentAgents(model.Rows, agentsWidth, height, model.Focus == environmentFocusAgents)
	surfaces := renderEnvironmentSurfaces(model.Surfaces, surfacesWidth, height, model.Focus == environmentFocusSurfaces)
	diff := renderEnvironmentDiffPane(model, diffWidth, height, model.Focus == environmentFocusDiff)
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(agentsWidth).Render(agents),
		mutedStyle.Render(" │ "),
		lipgloss.NewStyle().Width(surfacesWidth).Render(surfaces),
		mutedStyle.Render(" │ "),
		lipgloss.NewStyle().Width(diffWidth).Render(diff),
	)
}

func renderEnvironmentsReview(model EnvironmentsView, width, height int) string {
	pickerHeight := min(max(5, max(len(model.Rows), len(model.Surfaces))+2), max(5, height/3))
	agentsWidth := min(34, max(24, width/3))
	surfacesWidth := max(30, width-agentsWidth-3)
	top := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(agentsWidth).Render(renderEnvironmentAgents(model.Rows, agentsWidth, pickerHeight, model.Focus == environmentFocusAgents)),
		mutedStyle.Render(" │ "),
		lipgloss.NewStyle().Width(surfacesWidth).Render(renderEnvironmentSurfaces(model.Surfaces, surfacesWidth, pickerHeight, model.Focus == environmentFocusSurfaces)),
	)
	diffHeight := max(4, height-pickerHeight-1)
	return fitHeight(strings.Join([]string{
		top,
		divider(width - 2),
		renderEnvironmentDiffPane(model, width, diffHeight, model.Focus == "diff"),
	}, "\n"), height)
}

func renderEnvironmentsStacked(model EnvironmentsView, width, height int) string {
	lines := []string{}
	agentHeight := min(max(4, len(model.Rows)+2), max(4, height/3))
	lines = append(lines, renderEnvironmentAgents(model.Rows, width, agentHeight, model.Focus == environmentFocusAgents))
	if len(model.Surfaces) > 0 {
		lines = append(lines, divider(width-2))
		surfaceHeight := min(max(3, len(model.Surfaces)+1), max(3, height/4))
		lines = append(lines, renderEnvironmentSurfaces(model.Surfaces, width, surfaceHeight, model.Focus == environmentFocusSurfaces))
	}
	lines = append(lines, divider(width-2))
	used := environmentRenderedBlocksHeight(lines)
	diffHeight := max(4, height-used)
	lines = append(lines, renderEnvironmentDiffPane(model, width, diffHeight, model.Focus == environmentFocusDiff))
	return fitHeight(strings.Join(lines, "\n"), height)
}

func environmentRenderedBlocksHeight(blocks []string) int {
	height := 0
	for _, block := range blocks {
		height += strings.Count(block, "\n") + 1
	}
	return height
}

func renderEnvironmentAgents(rows []EnvironmentRow, width, height int, focused bool) string {
	lines := []string{environmentSectionTitle("Agents", focused)}
	if len(rows) == 0 {
		lines = append(lines, mutedStyle.Render("no agents"))
		return fitHeight(strings.Join(lines, "\n"), height)
	}
	for _, row := range rows {
		prefix := "  "
		rowStyle := mutedStyle
		if row.Selected {
			prefix = "› "
			rowStyle = focusStyle
		}
		dot := driftDot(row.State)
		baseline := "no baseline"
		if row.BaselineName != "" {
			baseline = row.BaselineName
			if row.BaselineDate != "" {
				baseline += " · " + row.BaselineDate
			}
		}
		line := fmt.Sprintf("%s%-12s %s %s", prefix, truncate(row.AgentLabel, 12), row.AgentMarker, driftStyle(row.State).Render(dot+" "+row.Detail))
		lines = append(lines, rowStyle.Render(truncate(line, width)))
		if row.Selected && width >= 54 {
			lines = append(lines, mutedStyle.Render(truncate("  baseline "+baseline, width)))
		}
	}
	return fitHeight(strings.Join(lines, "\n"), height)
}

func renderEnvironmentSurfaces(rows []EnvironmentSurface, width, height int, focused bool) string {
	lines := []string{environmentSectionTitle("Surfaces", focused)}
	if len(rows) == 0 {
		lines = append(lines, mutedStyle.Render("no changed surfaces"))
		return fitHeight(strings.Join(lines, "\n"), height)
	}
	for _, row := range rows {
		prefix := "  "
		style := mutedStyle
		if row.Selected {
			prefix = "› "
			style = focusStyle
		}
		title := strings.TrimSpace(row.Kind + " " + row.Name)
		if title == "" {
			title = row.Detail
		}
		line := fmt.Sprintf("%s%s %-20s %s", prefix, row.Marker, truncate(title, 20), row.Detail)
		lines = append(lines, style.Render(truncate(line, width)))
		if row.Selected && strings.TrimSpace(row.SourcePath) != "" {
			lines = append(lines, mutedStyle.Render(truncate("  "+row.SourcePath, width)))
		}
	}
	return fitHeight(strings.Join(lines, "\n"), height)
}

func renderEnvironmentDiffPane(model EnvironmentsView, width, height int, focused bool) string {
	lines := []string{environmentSectionTitle("Diff", focused)}
	if strings.TrimSpace(model.FocusAgent) != "" {
		lines = append(lines, labelStyle.Render(truncate(model.FocusAgent+" · baseline vs current", width)))
	}
	if model.ChangesEmpty != "" {
		lines = append(lines, mutedStyle.Render(truncate(model.ChangesEmpty, width)))
		return fitHeight(strings.Join(lines, "\n"), height)
	}
	if len(model.Diff.Rows) == 0 {
		lines = append(lines, mutedStyle.Render("Current environment matches the baseline."))
		return fitHeight(strings.Join(lines, "\n"), height)
	}
	if strings.TrimSpace(model.Diff.SourcePath) != "" {
		lines = append(lines, mutedStyle.Render(truncate("source: "+model.Diff.SourcePath, width)))
	}
	diffHeight := max(2, height-len(lines))
	var diffLines []string
	if model.Mode == environmentModeUnified || width < environmentSideBySideMinWidth {
		diffLines = renderEnvironmentDiffUnified(model.Diff.Rows, width)
	} else {
		diffLines = renderEnvironmentDiffSideBySide(model.Diff.Rows, width)
	}
	diffLines = environmentVisibleDiffLines(diffLines, model.DiffOffset, diffHeight)
	lines = append(lines, fitHeight(strings.Join(diffLines, "\n"), diffHeight))
	return fitHeight(strings.Join(lines, "\n"), height)
}

func environmentVisibleDiffLines(lines []string, offset, height int) []string {
	if height <= 0 || len(lines) == 0 {
		return nil
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(lines) {
		offset = max(0, len(lines)-1)
	}
	end := min(len(lines), offset+height)
	return lines[offset:end]
}

func renderEnvironmentDiffSideBySide(rows []EnvironmentDiffRow, width int) []string {
	lineNoWidth := 4
	contentWidth := max(40, width-3)
	colWidth := max(18, (contentWidth-3)/2)
	textWidth := max(8, colWidth-lineNoWidth-3)
	lines := []string{
		"  " + padRight("Baseline", colWidth) + mutedStyle.Render(" │ ") + "  " + "Current",
	}
	for _, row := range rows {
		if row.Kind == environmentRowHunk {
			lines = append(lines, renderEnvironmentHunkRow(row, width))
			continue
		}
		left := renderEnvironmentDiffSide(row.Left, textWidth)
		right := renderEnvironmentDiffSide(row.Right, textWidth)
		lines = append(lines, environmentRowStyle(row, true).Render(padRight(left, colWidth))+
			mutedStyle.Render(" │ ")+
			environmentRowStyle(row, false).Render(truncate(right, colWidth)))
	}
	return lines
}

func renderEnvironmentDiffUnified(rows []EnvironmentDiffRow, width int) []string {
	lines := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		if row.Kind == environmentRowHunk {
			lines = append(lines, renderEnvironmentHunkRow(row, width))
			continue
		}
		switch row.Kind {
		case environmentRowChanged:
			lines = append(lines, removedStyle.Render(truncate(renderEnvironmentUnifiedSide(row.Left), width)))
			lines = append(lines, cleanStyle.Render(truncate(renderEnvironmentUnifiedSide(row.Right), width)))
		case environmentRowRemoved:
			lines = append(lines, removedStyle.Render(truncate(renderEnvironmentUnifiedSide(row.Left), width)))
		case environmentRowAdded:
			lines = append(lines, cleanStyle.Render(truncate(renderEnvironmentUnifiedSide(row.Right), width)))
		default:
			lines = append(lines, mutedStyle.Render(truncate(renderEnvironmentUnifiedSide(row.Left), width)))
		}
	}
	return lines
}

func renderEnvironmentDiffSide(side EnvironmentDiffSide, textWidth int) string {
	lineNo := "    "
	if side.LineNumber > 0 {
		lineNo = fmt.Sprintf("%4d", side.LineNumber)
	}
	marker := side.Marker
	if marker == "" {
		marker = " "
	}
	return lineNo + " " + marker + " " + truncate(side.Text, textWidth)
}

func renderEnvironmentUnifiedSide(side EnvironmentDiffSide) string {
	lineNo := "    "
	if side.LineNumber > 0 {
		lineNo = fmt.Sprintf("%4d", side.LineNumber)
	}
	marker := side.Marker
	if marker == "" {
		marker = " "
	}
	return lineNo + " " + marker + " " + side.Text
}

func renderEnvironmentHunkRow(row EnvironmentDiffRow, width int) string {
	prefix := "  "
	if row.CurrentHunk {
		prefix = "> "
	}
	return changedStyle.Render(truncate(prefix+row.HunkTitle, width))
}

func environmentSectionTitle(label string, focused bool) string {
	if focused {
		return focusStyle.Render(label)
	}
	return titleStyle.Render(label)
}

func environmentRowStyle(row EnvironmentDiffRow, left bool) lipgloss.Style {
	switch row.Kind {
	case environmentRowRemoved:
		if left {
			return removedStyle
		}
	case environmentRowAdded:
		if !left {
			return cleanStyle
		}
	case environmentRowChanged:
		if left {
			return removedStyle
		}
		return cleanStyle
	}
	return mutedStyle
}

func renderEnvironmentHelp(model EnvironmentsView) string {
	focus := environmentFocusAgents
	if model.Focus != "" {
		focus = model.Focus
	}
	mode := environmentModeUnified
	if model.Mode == environmentModeUnified {
		mode = "side"
	}
	return fmt.Sprintf("tab focus:%s · ↑↓/jk move · pg scroll · n/p hunk · v %s · s save · R restore · i console · q quit", focus, mode)
}

func padRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) >= width {
		return truncate(value, width)
	}
	return value + strings.Repeat(" ", width-lipgloss.Width(value))
}
