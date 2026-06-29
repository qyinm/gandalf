package agents

import "github.com/qyinm/gandalf/internal/gandalfcore/types"

var currentSupported = []types.AgentID{
	types.AgentClaudeCode,
	types.AgentCodex,
}

// CurrentSupportedIDs returns the product-visible agent set for the active loop.
func CurrentSupportedIDs() []types.AgentID {
	return append([]types.AgentID(nil), currentSupported...)
}

// CurrentSupportedNames returns string identifiers for the product-visible agent set.
func CurrentSupportedNames() []string {
	ids := CurrentSupportedIDs()
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = id.String()
	}
	return names
}

// IsCurrentSupported reports whether id belongs to the active product-visible set.
func IsCurrentSupported(id types.AgentID) bool {
	for _, supported := range currentSupported {
		if id == supported {
			return true
		}
	}
	return false
}

// SupportsContentBackedUserSnapshot reports whether agent/scope can capture rollback content.
func SupportsContentBackedUserSnapshot(agent types.AgentID, scope types.EvidenceScope) bool {
	if scope != types.ScopeUser {
		return false
	}
	switch agent {
	case types.AgentClaudeCode, types.AgentCodex:
		return true
	default:
		return false
	}
}
