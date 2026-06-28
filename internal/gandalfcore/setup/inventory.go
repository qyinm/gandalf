package setup

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ObjectKind classifies setup inventory objects for action planning and display.
type ObjectKind string

const (
	ObjectSkill     ObjectKind = "skill"
	ObjectHook      ObjectKind = "hook"
	ObjectMCPServer ObjectKind = "mcp_server"
	ObjectPlugin    ObjectKind = "plugin"
)

// ActionKind identifies a supported setup inventory action.
type ActionKind string

const (
	ActionAdd    ActionKind = "add"
	ActionRemove ActionKind = "remove"
	ActionEdit   ActionKind = "edit"
)

// ActionAvailability describes whether an inventory action can currently run.
type ActionAvailability struct {
	Action    ActionKind
	Available bool
	Reason    string
}

// InventoryItem is one global setup object visible in the setup inventory.
type InventoryItem struct {
	ID           string
	EvidenceID   string
	Agent        types.AgentID
	ObjectKind   ObjectKind
	EvidenceKind types.EvidenceKind
	Name         string
	SourcePath   string
	Scope        types.EvidenceScope
	Entrypoint   string
	EntryStatus  string
	Actions      []ActionAvailability
}

// BuildInventory converts discovered evidence into global setup inventory rows.
func BuildInventory(evidence []types.DiscoveredItem) []InventoryItem {
	items := make([]InventoryItem, 0, len(evidence))
	for _, item := range evidence {
		objectKind, ok := objectKindForEvidence(item.Kind)
		if !ok || !IsInventoryEvidence(item) {
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
			Entrypoint:   inventoryMetadataString(item.Metadata, "entrypoint"),
			EntryStatus:  inventoryMetadataString(item.Metadata, "entrypointStatus"),
			Actions:      defaultActions(item.Scope),
		})
	}

	keyedItems := make([]inventoryKeyedItem, len(items))
	for i, item := range items {
		keyedItems[i] = inventoryKeyedItem{
			item: item,
			key: inventorySortKey{
				objectKind: string(item.ObjectKind),
				agent:      string(item.Agent),
				name:       strings.ToLower(item.Name),
				sourcePath: item.SourcePath,
				id:         item.ID,
			},
		}
	}
	sort.SliceStable(keyedItems, func(i, j int) bool {
		return keyedItems[i].key.less(keyedItems[j].key)
	})
	for i, keyedItem := range keyedItems {
		items[i] = keyedItem.item
	}

	return items
}

type inventoryKeyedItem struct {
	item InventoryItem
	key  inventorySortKey
}

type inventorySortKey struct {
	objectKind string
	agent      string
	name       string
	sourcePath string
	id         string
}

func (key inventorySortKey) less(other inventorySortKey) bool {
	if key.objectKind != other.objectKind {
		return key.objectKind < other.objectKind
	}
	if key.agent != other.agent {
		return key.agent < other.agent
	}
	if key.name != other.name {
		return key.name < other.name
	}
	if key.sourcePath != other.sourcePath {
		return key.sourcePath < other.sourcePath
	}
	return key.id < other.id
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

// IsInventoryEvidence reports whether discovered evidence belongs in the global setup inventory.
func IsInventoryEvidence(item types.DiscoveredItem) bool {
	if _, ok := objectKindForEvidence(item.Kind); !ok {
		return false
	}
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
			{Action: ActionEdit, Available: false, Reason: "edit action provider is not implemented yet"},
			{Action: ActionRemove, Available: false, Reason: "remove action provider is not implemented yet"},
		}
	}
	return []ActionAvailability{
		{Action: ActionEdit, Available: false, Reason: "managed setup cannot be edited directly"},
		{Action: ActionRemove, Available: false, Reason: "managed setup cannot be removed directly"},
	}
}

func inventoryMetadataString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	value, ok := metadata[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
