package graph

import (
	"encoding/json"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func entityNameFor(item *types.DiscoveredItem) string {
	if item.Name != nil {
		return *item.Name
	}
	return item.ID
}

func nodeIDFor(item *types.DiscoveredItem) string {
	raw := strings.Join([]string{
		item.Agent.String(),
		item.Scope.String(),
		item.Kind.String(),
		entityNameFor(item),
		item.ID,
	}, ":")
	parts := strings.Fields(raw)
	return strings.TrimSpace(strings.Join(parts, " "))
}

func overrideIdentity(node *types.GraphNode) string {
	return strings.Join([]string{
		node.Agent.String(),
		node.EntityKind.String(),
		node.EntityName,
	}, "\x00")
}

func valueFor(item *types.DiscoveredItem) json.RawMessage {
	if len(item.Value) > 0 && string(item.Value) != "null" {
		return item.Value
	}

	if item.CaptureStatus == types.CaptureUnsupported {
		state := json.RawMessage(`"present"`)
		if len(item.Metadata) > 0 {
			var meta map[string]json.RawMessage
			if json.Unmarshal(item.Metadata, &meta) == nil {
				if v, ok := meta["state"]; ok {
					state = v
				}
			}
		}
		out, _ := json.Marshal(map[string]json.RawMessage{
			"captureStatus": json.RawMessage(`"unsupported"`),
			"state":         state,
		})
		return out
	}

	out, _ := json.Marshal(map[string]string{
		"captureStatus": item.CaptureStatus.String(),
	})
	return out
}

// BuildGraph constructs effective-value graph nodes from discovered evidence.
func BuildGraph(evidence []types.DiscoveredItem) []types.GraphNode {
	evidenceByID := make(map[string]*types.DiscoveredItem, len(evidence))
	for i := range evidence {
		evidenceByID[evidence[i].ID] = &evidence[i]
	}

	nodes := make([]types.GraphNode, 0, len(evidence))
	for i := range evidence {
		item := &evidence[i]
		nodes = append(nodes, types.GraphNode{
			ID:             nodeIDFor(item),
			Agent:          item.Agent,
			Scope:          item.Scope,
			SourcePath:     item.SourcePath,
			EntityKind:     item.Kind,
			EntityName:     entityNameFor(item),
			EffectiveValue: valueFor(item),
			OverriddenBy:   nil,
			Confidence:     item.Confidence,
			EvidenceID:     item.ID,
		})
	}

	strongestByIdentity := make(map[string]struct {
		nodeID     string
		precedence uint32
	})

	for i := range nodes {
		node := &nodes[i]
		item := evidenceByID[node.EvidenceID]
		precedence := uint32(0)
		if item != nil {
			precedence = item.Precedence
		}
		identity := overrideIdentity(node)
		current, ok := strongestByIdentity[identity]
		if !ok || precedence > current.precedence {
			strongestByIdentity[identity] = struct {
				nodeID     string
				precedence uint32
			}{nodeID: node.ID, precedence: precedence}
		}
	}

	for i := range nodes {
		node := &nodes[i]
		item := evidenceByID[node.EvidenceID]
		strongest := strongestByIdentity[overrideIdentity(node)]
		if item != nil && strongest.nodeID != node.ID && strongest.precedence > item.Precedence {
			id := strongest.nodeID
			node.OverriddenBy = &id
		}
	}

	return nodes
}
