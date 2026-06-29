package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// FormatAgentLabel returns a human-readable agent name.
func FormatAgentLabel(id types.AgentID) string {
	return agents.DisplayName(id)
}

// FormatAgentMarker returns a compact stable marker for inventory rows.
func FormatAgentMarker(id types.AgentID) string {
	switch id {
	case types.AgentClaudeCode:
		return "CC"
	case types.AgentCodex:
		return "CX"
	case types.AgentCursor:
		return "CU"
	case types.AgentOpencode:
		return "OC"
	case types.AgentPiAgent:
		return "PI"
	default:
		return "??"
	}
}

// FormatAgentScope formats timeline agent scope for list rows.
func FormatAgentScope(agent *types.AgentID, agents []types.AgentID) string {
	if agent != nil {
		return FormatAgentLabel(*agent)
	}
	if len(agents) == 0 {
		return "all"
	}
	if len(agents) > 1 {
		labels := make([]string, len(agents))
		for i, a := range agents {
			labels[i] = FormatAgentLabel(a)
		}
		return strings.Join(labels, ", ")
	}
	return FormatAgentLabel(agents[0])
}

// FormatTimelineTimestamp formats an ISO timestamp for timeline display.
func FormatTimelineTimestamp(value string, now time.Time) string {
	date, err := time.Parse(time.RFC3339, value)
	if err != nil {
		if date, err = time.Parse("2006-01-02T15:04:05.000", value); err != nil {
			if date, err = time.Parse("2006-01-02T15:04:05.000Z", value); err != nil {
				return value
			}
		}
	}

	dateKey := localDateKey(date)
	nowKey := localDateKey(now)
	yesterday := now.AddDate(0, 0, -1)

	if dateKey == nowKey {
		return fmt.Sprintf("Today %s", formatClock(date))
	}
	if dateKey == localDateKey(yesterday) {
		return fmt.Sprintf("Yesterday %s", formatClock(date))
	}
	return fmt.Sprintf("%s %s", date.Format("Jan 2"), formatClock(date))
}

// TruncateText truncates text to a display width with an ellipsis suffix.
// It is display-width aware so it does not corrupt wide-rune or ANSI content.
func TruncateText(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width <= 3 {
		return "..."
	}
	return ansi.Truncate(value, width, "...")
}

// PadDisplay truncates then pads text to a display width.
func PadDisplay(value string, width int) string {
	truncated := TruncateText(value, width)
	if pad := width - ansi.StringWidth(truncated); pad > 0 {
		return truncated + strings.Repeat(" ", pad)
	}
	return truncated
}

// FormatInventorySourceRoot returns a compact source-root label for inventory rows.
func FormatInventorySourceRoot(item types.DiscoveredItem) string {
	if item.Kind != types.KindSkill && item.Kind != types.KindMcpServer && item.Kind != types.KindHook {
		return ""
	}
	meta := parseMetadata(item.Metadata)
	if metadataBool(meta, "builtIn") {
		return ""
	}

	sourceRoot := metadataString(meta, "sourceRoot")
	if sourceRoot == "" {
		sourceRoot = derivedSourceRoot(item)
	}
	if sourceRoot == "" {
		return ""
	}
	return compactAbsoluteSourceRoot(sourceRoot)
}

// FormatInventoryNameWithSource appends a source-root suffix when available.
func FormatInventoryNameWithSource(name string, item types.DiscoveredItem) string {
	sourceRoot := FormatInventorySourceRoot(item)
	if sourceRoot == "" {
		return name
	}
	if item.Scope == types.ScopeProject {
		return fmt.Sprintf("%s (project: %s)", name, sourceRoot)
	}
	return fmt.Sprintf("%s (%s)", name, sourceRoot)
}

func parseMetadata(metadata json.RawMessage) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal(metadata, &meta); err != nil {
		return nil
	}
	return meta
}

func metadataBool(meta map[string]any, key string) bool {
	if meta == nil {
		return false
	}
	value, ok := meta[key]
	if !ok {
		return false
	}
	b, ok := value.(bool)
	return ok && b
}

func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return s
}

func derivedSourceRoot(item types.DiscoveredItem) string {
	if item.SourcePath == "" {
		return ""
	}
	if item.Kind != types.KindSkill {
		return stripEntrypoint(item.SourcePath)
	}

	skillPath := stripEntrypoint(item.SourcePath)
	parts := strings.Split(strings.Trim(skillPath, "/"), "/")
	if len(parts) == 0 {
		return skillPath
	}
	last := parts[len(parts)-1]
	if item.Name != nil && last == *item.Name && len(parts) > 1 {
		trimmed := strings.TrimSuffix(skillPath, "/"+*item.Name)
		if trimmed == "" {
			return skillPath
		}
		return trimmed
	}
	return skillPath
}

func stripEntrypoint(sourcePath string) string {
	if strings.HasSuffix(sourcePath, "/SKILL.md") || strings.HasSuffix(sourcePath, "/skill.md") {
		return strings.TrimSuffix(strings.TrimSuffix(sourcePath, "/SKILL.md"), "/skill.md")
	}
	return sourcePath
}

func compactAbsoluteSourceRoot(sourceRoot string) string {
	if !strings.HasPrefix(sourceRoot, "/") {
		return sourceRoot
	}
	parts := strings.Split(strings.Trim(sourceRoot, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "Cursor" {
			return strings.Join(parts[i:], "/")
		}
	}
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return sourceRoot
}

func localDateKey(date time.Time) string {
	y, m, d := date.Date()
	return fmt.Sprintf("%d-%d-%d", y, int(m), d)
}

func formatClock(date time.Time) string {
	return fmt.Sprintf("%02d:%02d", date.Hour(), date.Minute())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatSetupObjectKind(kind setup.ObjectKind) string {
	switch kind {
	case setup.ObjectMCPServer:
		return "mcp"
	case setup.ObjectSkill:
		return "skill"
	case setup.ObjectHook:
		return "hook"
	case setup.ObjectPlugin:
		return "plugin"
	default:
		return "setup"
	}
}

func formatSetupActions(actions []setup.ActionAvailability) string {
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		label := string(action.Action)
		if !action.Available {
			label = label + ":unavailable"
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return "none"
	}
	return strings.Join(labels, " ")
}

func formatMarketplaceActions(actions []setup.MarketplaceActionAvailability) string {
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		label := string(action.Action)
		if !action.Available {
			label = label + ":unavailable"
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return "none"
	}
	return strings.Join(labels, " ")
}

func buildSetupActionConfirmation(plan setup.ActionPlan) *SetupActionConfirmationModel {
	command := plan.Operation
	if plan.Command != nil {
		command = strings.Join(append([]string{plan.Command.Program}, plan.Command.Args...), " ")
	}
	return &SetupActionConfirmationModel{
		Action:       string(plan.Action),
		AgentLabel:   FormatAgentLabel(plan.Agent),
		ObjectKind:   formatSetupObjectKind(plan.ObjectKind),
		TargetName:   plan.TargetName,
		Operation:    plan.Operation,
		ConfigTarget: plan.ConfigTarget,
		Command:      command,
	}
}

func buildMarketplaceReviewModel(plan setup.MarketplaceReviewPlan, pending bool) MarketplaceReviewModel {
	status := "reviewed guidance"
	if pending {
		status = "pending review"
	}
	return MarketplaceReviewModel{
		Title:          "Marketplace Review Action",
		Status:         status,
		AgentLabel:     FormatAgentLabel(types.AgentID(plan.Agent)),
		SourceLabel:    plan.SourceLabel,
		SourcePath:     plan.SourcePath,
		TargetName:     plan.EntryName,
		Operation:      plan.Operation,
		ExpectedEffect: plan.ExpectedEffect,
		Instructions:   plan.Instructions,
		Pending:        pending,
	}
}
