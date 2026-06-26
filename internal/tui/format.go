package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qyinm/hem/internal/hemcore/types"
)

var agentLabels = map[types.AgentID]string{
	types.AgentClaudeCode: "Claude Code",
	types.AgentCodex:      "Codex",
	types.AgentCursor:     "Cursor",
	types.AgentOpencode:   "OpenCode",
	types.AgentPiAgent:    "Pi Agent",
	types.AgentProject:    "Project",
	types.AgentUnknown:    "Unknown",
}

// FormatAgentLabel returns a human-readable agent name.
func FormatAgentLabel(id types.AgentID) string {
	if label, ok := agentLabels[id]; ok {
		return label
	}
	return id.String()
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

// TruncateText truncates text to width with an ellipsis suffix.
func TruncateText(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(value) <= width {
		return value
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	return value[:width-3] + "..."
}

// PadDisplay truncates then pads text to width.
func PadDisplay(value string, width int) string {
	return TruncateText(value, width) + strings.Repeat(" ", max(0, width-len(TruncateText(value, width))))
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