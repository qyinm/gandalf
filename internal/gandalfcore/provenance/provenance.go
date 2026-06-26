package provenance

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// BuildProvenance links graph nodes back to their source evidence.
func BuildProvenance(graph []types.GraphNode, evidence []types.DiscoveredItem) []types.ProvenanceEntry {
	evidenceByID := make(map[string]*types.DiscoveredItem, len(evidence))
	for i := range evidence {
		evidenceByID[evidence[i].ID] = &evidence[i]
	}

	entries := make([]types.ProvenanceEntry, 0, len(graph))
	for i := range graph {
		node := &graph[i]
		item := evidenceByID[node.EvidenceID]
		precedence := uint32(0)
		captureStatus := types.CaptureUnsupported
		if item != nil {
			precedence = item.Precedence
			captureStatus = item.CaptureStatus
		}
		entries = append(entries, types.ProvenanceEntry{
			NodeID:        node.ID,
			EvidenceID:    node.EvidenceID,
			SourcePath:    node.SourcePath,
			Scope:         node.Scope,
			Precedence:    precedence,
			Confidence:    node.Confidence,
			CaptureStatus: captureStatus,
		})
	}
	return entries
}
