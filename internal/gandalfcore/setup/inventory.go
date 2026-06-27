package setup

import (
	"sort"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type ObjectKind string

const (
	ObjectSkill     ObjectKind = "skill"
	ObjectHook      ObjectKind = "hook"
	ObjectMCPServer ObjectKind = "mcp_server"
	ObjectPlugin    ObjectKind = "plugin"
)

type ActionKind string

const (
	ActionAdd    ActionKind = "add"
	ActionRemove ActionKind = "remove"
	ActionEdit   ActionKind = "edit"
)

type ActionAvailability struct {
	Action    ActionKind
	Available bool
	Reason    string
}

type InventoryItem struct {
	ID           string
	EvidenceID   string
	Agent        types.AgentID
	ObjectKind   ObjectKind
	EvidenceKind types.EvidenceKind
	Name         string
	SourcePath   string
	Scope        types.EvidenceScope
	Actions      []ActionAvailability
}

func BuildInventory(evidence []types.DiscoveredItem) []InventoryItem {
	items := make([]InventoryItem, 0, len(evidence))
	for _, item := range evidence {
		objectKind, ok := objectKindForEvidence(item.Kind)
		if !ok || !inventoryScopeEnabled(item) {
			continue
		}
		items = append(items, InventoryItem{
			ID:           inventoryItemID(item),
			EvidenceID:   item.ID,
			Agent:        item.Agent,
			ObjectKind:   objectKind,
			EvidenceKind: item.Kind,
			Name:         inventoryItemName(item),
			SourcePath:   item.SourcePath,
			Scope:        item.Scope,
			Actions:      defaultActions(item.Scope),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		for _, pair := range [][2]string{
			{string(left.ObjectKind), string(right.ObjectKind)},
			{string(left.Agent), string(right.Agent)},
			{strings.ToLower(left.Name), strings.ToLower(right.Name)},
			{left.SourcePath, right.SourcePath},
			{left.ID, right.ID},
		} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return false
	})

	return items
}

func objectKindForEvidence(kind types.EvidenceKind) (ObjectKind, bool) {
	switch kind {
	case types.KindSkill:
		return ObjectSkill, true
	case types.KindHook:
		return ObjectHook, true
	case types.KindMcpServer:
		return ObjectMCPServer, true
	case types.KindExtension:
		return ObjectPlugin, true
	default:
		return "", false
	}
}

func inventoryScopeEnabled(item types.DiscoveredItem) bool {
	if item.Agent == types.AgentProject {
		return false
	}
	return item.Scope == types.ScopeUser || item.Scope == types.ScopeManaged
}

func inventoryItemID(item types.DiscoveredItem) string {
	if item.ID != "" {
		return item.ID
	}
	parts := []string{
		item.Scope.String(),
		item.Agent.String(),
		item.Kind.String(),
		item.SourcePath,
		inventoryItemName(item),
	}
	return strings.Join(parts, ":")
}

func inventoryItemName(item types.DiscoveredItem) string {
	if item.Name != nil && strings.TrimSpace(*item.Name) != "" {
		return strings.TrimSpace(*item.Name)
	}
	if strings.TrimSpace(item.SourcePath) != "" {
		return item.SourcePath
	}
	return item.Kind.String()
}

func defaultActions(scope types.EvidenceScope) []ActionAvailability {
	if scope == types.ScopeUser {
		return []ActionAvailability{
			{Action: ActionEdit, Available: true},
			{Action: ActionRemove, Available: true},
		}
	}
	return []ActionAvailability{
		{Action: ActionEdit, Available: false, Reason: "managed setup cannot be edited directly"},
		{Action: ActionRemove, Available: false, Reason: "managed setup cannot be removed directly"},
	}
}
