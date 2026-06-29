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
	ActionToggle ActionKind = "toggle"
)

// ActionAvailability describes whether an inventory action can currently run.
type ActionAvailability struct {
	Action    ActionKind
	Available bool
	Reason    string
}

// InventoryTool is one tool exposed by a setup inventory object such as an MCP server.
type InventoryTool struct {
	Name        string
	Description string
}

// InventoryItem is one global setup object visible in the setup inventory.
type InventoryItem struct {
	ID            string
	EvidenceID    string
	Agent         types.AgentID
	ObjectKind    ObjectKind
	EvidenceKind  types.EvidenceKind
	Name          string
	SourcePath    string
	Scope         types.EvidenceScope
	Entrypoint    string
	EntryStatus   string
	RuntimeStatus string
	Tools         []InventoryTool
	ToolCount     int
	Disabled      bool
	Actions       []ActionAvailability
}

// BuildInventory converts discovered evidence into global setup inventory rows.
func BuildInventory(evidence []types.DiscoveredItem) []InventoryItem {
	items := make([]InventoryItem, 0, len(evidence))
	for _, item := range evidence {
		objectKind, ok := objectKindForEvidence(item.Kind)
		if !ok || !IsInventoryEvidence(item) {
			continue
		}
		metadata := inventoryMetadataMap(item.Metadata)
		tools := inventoryMetadataTools(metadata)
		toolCount := inventoryMetadataInt(metadata, "toolCount")
		if toolCount == 0 {
			toolCount = len(tools)
		}
		items = append(items, InventoryItem{
			ID:            inventoryItemID(item),
			EvidenceID:    item.ID,
			Agent:         item.Agent,
			ObjectKind:    objectKind,
			EvidenceKind:  item.Kind,
			Name:          inventoryItemName(item),
			SourcePath:    item.SourcePath,
			Scope:         item.Scope,
			Entrypoint:    inventoryMetadataString(metadata, "entrypoint"),
			EntryStatus:   inventoryMetadataString(metadata, "entrypointStatus"),
			RuntimeStatus: inventoryRuntimeStatus(metadata),
			Tools:         tools,
			ToolCount:     toolCount,
			Disabled:      mcpDisabledFromValue(objectKind, item.Value),
			Actions:       defaultActions(item.Scope, objectKind, item.SourcePath),
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

func defaultActions(scope types.EvidenceScope, objectKind ObjectKind, sourcePath string) []ActionAvailability {
	actions := make([]ActionAvailability, 0, 3)
	actions = append(actions, toggleAvailability(scope, objectKind, sourcePath))
	if scope == types.ScopeUser {
		actions = append(actions,
			ActionAvailability{Action: ActionEdit, Available: false, Reason: "edit action provider is not implemented yet"},
			ActionAvailability{Action: ActionRemove, Available: false, Reason: "remove action provider is not implemented yet"},
		)
		return actions
	}
	actions = append(actions,
		ActionAvailability{Action: ActionEdit, Available: false, Reason: "managed setup cannot be edited directly"},
		ActionAvailability{Action: ActionRemove, Available: false, Reason: "managed setup cannot be removed directly"},
	)
	return actions
}

// toggleAvailability reports whether the enable/disable toggle can run.
// Only JSON-backed MCP servers under user scope expose a real toggle today,
// because their "off" state is a well-defined, reversible `disabled` flag.
func toggleAvailability(scope types.EvidenceScope, objectKind ObjectKind, sourcePath string) ActionAvailability {
	if objectKind != ObjectMCPServer {
		return ActionAvailability{Action: ActionToggle, Available: false, Reason: "enable/disable is only supported for MCP servers"}
	}
	if scope != types.ScopeUser {
		return ActionAvailability{Action: ActionToggle, Available: false, Reason: "managed MCP servers cannot be toggled directly"}
	}
	if !isJSONMCPSource(sourcePath) {
		return ActionAvailability{Action: ActionToggle, Available: false, Reason: "enable/disable requires a JSON MCP config (.mcp.json)"}
	}
	return ActionAvailability{Action: ActionToggle, Available: true}
}

// isJSONMCPSource reports whether the MCP server is backed by a JSON config file
// whose entries support a `disabled` flag.
func isJSONMCPSource(sourcePath string) bool {
	trimmed := strings.TrimSpace(sourcePath)
	if trimmed == "" || strings.HasPrefix(trimmed, "<") {
		return false
	}
	return strings.HasSuffix(trimmed, ".mcp.json") ||
		strings.HasSuffix(trimmed, "/mcp.json") ||
		trimmed == "mcp.json"
}

// mcpDisabledFromValue reads the persisted disabled/enabled flag for an MCP server.
func mcpDisabledFromValue(objectKind ObjectKind, value json.RawMessage) bool {
	if objectKind != ObjectMCPServer || len(value) == 0 {
		return false
	}
	var parsed map[string]any
	if err := json.Unmarshal(value, &parsed); err != nil {
		return false
	}
	if disabled, ok := parsed["disabled"].(bool); ok {
		return disabled
	}
	if enabled, ok := parsed["enabled"].(bool); ok {
		return !enabled
	}
	return false
}

func inventoryMetadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func inventoryRuntimeStatus(metadata map[string]any) string {
	for _, key := range []string{"runtimeStatus", "readiness", "status"} {
		if value := inventoryMetadataString(metadata, key); value != "" {
			return value
		}
	}
	return ""
}

func inventoryMetadataInt(metadata map[string]any, key string) int {
	switch value := metadata[key].(type) {
	case int:
		return value
	case float64:
		if value > 0 {
			return int(value)
		}
	case json.Number:
		parsed, err := value.Int64()
		if err == nil && parsed > 0 {
			return int(parsed)
		}
	}
	return 0
}

func inventoryMetadataTools(metadata map[string]any) []InventoryTool {
	values, ok := metadata["tools"].([]any)
	if !ok {
		return nil
	}
	tools := make([]InventoryTool, 0, len(values))
	for _, value := range values {
		switch tool := value.(type) {
		case string:
			name := strings.TrimSpace(tool)
			if name != "" {
				tools = append(tools, InventoryTool{Name: name})
			}
		case map[string]any:
			name := metadataMapString(tool, "name")
			if name == "" {
				name = metadataMapString(tool, "id")
			}
			if name == "" {
				continue
			}
			tools = append(tools, InventoryTool{
				Name:        name,
				Description: metadataMapString(tool, "description"),
			})
		}
	}
	return tools
}

func inventoryMetadataMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil
	}
	return metadata
}

func metadataMapString(metadata map[string]any, key string) string {
	value, ok := metadata[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
