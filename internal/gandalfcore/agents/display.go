package agents

import "github.com/qyinm/gandalf/internal/gandalfcore/types"

var labels = map[types.AgentID]string{
	types.AgentClaudeCode: "Claude Code",
	types.AgentCodex:      "Codex",
	types.AgentCursor:     "Cursor",
	types.AgentOpencode:   "OpenCode",
	types.AgentPiAgent:    "Pi Agent",
	types.AgentProject:    "Project",
	types.AgentUnknown:    "Unknown",
}

// DisplayName returns a human-readable agent label.
func DisplayName(id types.AgentID) string {
	if label, ok := labels[id]; ok {
		return label
	}
	return id.String()
}
