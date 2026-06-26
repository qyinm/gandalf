package report

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/diff"
	"github.com/qyinm/hem/internal/hemcore/types"
)

// Trust captures scan trust metadata for reports.
type Trust struct {
	ReadOnly          bool
	Network           string
	CommandsExecuted  uint32
}

// Input carries all data needed to render a markdown report.
type Input struct {
	SnapshotName *string
	Current      *string
	Trust        Trust
	Evidence     []types.DiscoveredItem
	Graph        []types.GraphNode
	Findings     []types.AuditFinding
	Provenance   []types.ProvenanceEntry
	BlindSpots   []string
	Diffs        *diff.GraphDiff
}

func agentNames(agent string) string {
	switch agent {
	case "claude-code":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "cursor":
		return "Cursor"
	case "project":
		return "Project"
	case "unknown":
		return "Unknown"
	default:
		return agent
	}
}

func agentLine(agent string, items []types.DiscoveredItem) string {
	scopes := make(map[types.EvidenceScope]struct{})
	for _, item := range items {
		scopes[item.Scope] = struct{}{}
	}
	var states []string
	if _, ok := scopes[types.ScopeUser]; ok {
		states = append(states, "user state found")
	}
	if _, ok := scopes[types.ScopeProject]; ok {
		states = append(states, "project state found")
	}
	if _, ok := scopes[types.ScopeManaged]; ok {
		states = append(states, "managed state found")
	}
	if len(states) == 0 {
		states = append(states, "state found")
	}
	return "- " + agentNames(agent) + "  " + strings.Join(states, ", ")
}

func findingLine(finding types.AuditFinding) string {
	path := ""
	if finding.Path != nil {
		path = " (" + *finding.Path + ")"
	}
	return "- " + severityLabel(finding.Severity) + " " + finding.Code + ": " + finding.Problem + path
}

func severityLabel(severity types.Severity) string {
	switch severity {
	case types.SeverityNone:
		return "NONE"
	case types.SeverityLow:
		return "LOW"
	case types.SeverityMedium:
		return "MEDIUM"
	case types.SeverityHigh:
		return "HIGH"
	case types.SeverityCritical:
		return "CRITICAL"
	default:
		return strings.ToUpper(string(severity))
	}
}

func provenanceLine(entry types.ProvenanceEntry) string {
	return "- " + entry.EvidenceID + " -> " + entry.NodeID + " from " + entry.SourcePath +
		" (" + entry.Scope.String() + ", precedence " + itoa(int(entry.Precedence)) + ", " + entry.CaptureStatus.String() + ")"
}

func captureStatusCounts(evidence []types.DiscoveredItem) map[string]uint32 {
	counts := make(map[string]uint32)
	for _, item := range evidence {
		counts[item.CaptureStatus.String()]++
	}
	return counts
}

// RenderMarkdownReport renders a markdown audit report.
func RenderMarkdownReport(input *Input) string {
	snapshotName := "current"
	if input.SnapshotName != nil {
		snapshotName = *input.SnapshotName
	} else if input.Current != nil {
		snapshotName = *input.Current
	}

	lines := []string{
		"# hem report: " + snapshotName,
		"",
		"## Trust",
		"- Read-only: " + yesNo(input.Trust.ReadOnly),
		"- Network: " + input.Trust.Network,
		"- Commands executed: " + itoa(int(input.Trust.CommandsExecuted)),
		"",
		"## Detected agents",
	}

	byAgent := make(map[string][]types.DiscoveredItem)
	for _, item := range input.Evidence {
		byAgent[item.Agent.String()] = append(byAgent[item.Agent.String()], item)
	}
	if len(byAgent) == 0 {
		lines = append(lines, "- None detected")
	} else {
		agents := make([]string, 0, len(byAgent))
		for agent := range byAgent {
			agents = append(agents, agent)
		}
		sort.Strings(agents)
		for _, agent := range agents {
			lines = append(lines, agentLine(agent, byAgent[agent]))
		}
	}

	lines = append(lines, "", "## High-signal findings")
	if len(input.Findings) == 0 {
		lines = append(lines, "- None")
	} else {
		for _, finding := range input.Findings {
			lines = append(lines, findingLine(finding))
		}
	}

	lines = append(lines, "", "## Blind spots")
	if len(input.BlindSpots) == 0 {
		lines = append(lines, "- None")
	} else {
		for _, blindSpot := range input.BlindSpots {
			lines = append(lines, "- "+blindSpot)
		}
	}

	lines = append(lines, "", "## Reproducibility gaps")
	counts := captureStatusCounts(input.Evidence)
	if len(counts) == 0 {
		lines = append(lines, "- None")
	} else {
		statuses := make([]string, 0, len(counts))
		for status := range counts {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		for _, status := range statuses {
			lines = append(lines, "- "+status+": "+itoa(int(counts[status])))
		}
	}

	if input.Diffs != nil {
		lines = append(lines, "", "## Semantic diff")
		if len(input.Diffs.SemanticChanges) == 0 {
			lines = append(lines, "- None")
		} else {
			for _, change := range input.Diffs.SemanticChanges {
				lines = append(lines, "- "+severityLabel(change.Severity)+" "+change.Code.String()+": "+change.EntityName)
			}
		}
		lines = append(lines, "", "## Raw source changes")
		if len(input.Diffs.RawSourceChanges) == 0 {
			lines = append(lines, "- None")
		} else {
			for _, change := range input.Diffs.RawSourceChanges {
				lines = append(lines, "- "+change.Status+": "+change.SourcePath)
			}
		}
	}

	lines = append(lines, "", "## Provenance")
	if len(input.Provenance) == 0 {
		lines = append(lines, "- None")
	} else {
		for _, entry := range input.Provenance {
			lines = append(lines, provenanceLine(entry))
		}
	}

	lines = append(lines, "", "## Next", "- `hem snapshot create --name baseline --agent codex --scope user --project .`")
	return strings.Join(lines, "\n") + "\n"
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// MarshalInputJSON is a helper for CLI JSON output.
func MarshalInputJSON(snapshot types.Snapshot, markdown string) ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"snapshot": snapshot,
		"markdown": markdown,
	}, "", "  ")
}