package diff

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/types"
)

// SemanticChangeCode identifies a semantic graph change.
type SemanticChangeCode string

const (
	SemanticAgentConfigAdded       SemanticChangeCode = "AGENT_CONFIG_ADDED"
	SemanticAgentConfigRemoved     SemanticChangeCode = "AGENT_CONFIG_REMOVED"
	SemanticAgentConfigChanged     SemanticChangeCode = "AGENT_CONFIG_CHANGED"
	SemanticMcpAdded               SemanticChangeCode = "MCP_ADDED"
	SemanticMcpRemoved             SemanticChangeCode = "MCP_REMOVED"
	SemanticMcpChanged             SemanticChangeCode = "MCP_CHANGED"
	SemanticSkillAdded             SemanticChangeCode = "SKILL_ADDED"
	SemanticSkillRemoved           SemanticChangeCode = "SKILL_REMOVED"
	SemanticHookAdded              SemanticChangeCode = "HOOK_ADDED"
	SemanticHookRemoved            SemanticChangeCode = "HOOK_REMOVED"
	SemanticHookChanged            SemanticChangeCode = "HOOK_CHANGED"
	SemanticPermissionChanged      SemanticChangeCode = "PERMISSION_CHANGED"
	SemanticInstructionChanged     SemanticChangeCode = "INSTRUCTION_CHANGED"
	SemanticPermissionWildcardAdded SemanticChangeCode = "PERMISSION_WILDCARD_ADDED"
	SemanticSkillExecutableAppeared SemanticChangeCode = "SKILL_EXECUTABLE_APPEARED"
	SemanticEnvKeyAdded            SemanticChangeCode = "ENV_KEY_ADDED"
	SemanticEnvKeyRemoved          SemanticChangeCode = "ENV_KEY_REMOVED"
	SemanticUnsupportedStateChanged SemanticChangeCode = "UNSUPPORTED_STATE_CHANGED"
)

func (c SemanticChangeCode) String() string { return string(c) }

// SemanticChangeDetails captures structured diff metadata.
type SemanticChangeDetails struct {
	ChangedFields []string
	SourcePath    *string
	Extra         map[string]json.RawMessage
}

func (d SemanticChangeDetails) MarshalJSON() ([]byte, error) {
	payload := make(map[string]any, len(d.Extra)+2)
	if len(d.ChangedFields) > 0 {
		payload["changedFields"] = d.ChangedFields
	} else {
		payload["changedFields"] = []string{}
	}
	if d.SourcePath != nil {
		payload["sourcePath"] = *d.SourcePath
	}
	for key, value := range d.Extra {
		payload[key] = value
	}
	return json.Marshal(payload)
}

func (d *SemanticChangeDetails) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.ChangedFields = nil
	d.SourcePath = nil
	d.Extra = make(map[string]json.RawMessage)
	for key, value := range raw {
		switch key {
		case "changedFields":
			if err := json.Unmarshal(value, &d.ChangedFields); err != nil {
				return err
			}
		case "sourcePath":
			var path string
			if err := json.Unmarshal(value, &path); err != nil {
				return err
			}
			d.SourcePath = &path
		default:
			d.Extra[key] = value
		}
	}
	return nil
}

// SemanticChange is a single semantic graph diff entry.
type SemanticChange struct {
	Code        SemanticChangeCode `json:"code"`
	EntityKind  types.EvidenceKind `json:"entityKind"`
	EntityName  string             `json:"entityName"`
	Severity    types.Severity     `json:"severity"`
	Before      json.RawMessage    `json:"before,omitempty"`
	After       json.RawMessage    `json:"after,omitempty"`
	Details     SemanticChangeDetails `json:"details"`
}

// RawSourceChange tracks source-level evidence identity changes.
type RawSourceChange struct {
	SourcePath        string  `json:"sourcePath"`
	BeforeEvidenceID  *string `json:"beforeEvidenceId,omitempty"`
	AfterEvidenceID   *string `json:"afterEvidenceId,omitempty"`
	BeforeChecksum    *string `json:"beforeChecksum,omitempty"`
	AfterChecksum     *string `json:"afterChecksum,omitempty"`
	Status            string  `json:"status"`
}

// GraphDiff aggregates semantic and raw source changes between graphs.
type GraphDiff struct {
	SemanticChanges   []SemanticChange   `json:"semanticChanges"`
	RawSourceChanges  []RawSourceChange  `json:"rawSourceChanges"`
}

func graphIdentity(node *types.GraphNode) string {
	return strings.Join([]string{
		node.Agent.String(),
		node.EntityKind.String(),
		node.EntityName,
	}, "\x00")
}

func sourceIdentity(node *types.GraphNode) string {
	return strings.Join([]string{
		node.SourcePath,
		node.EntityKind.String(),
		node.EntityName,
	}, "\x00")
}

func sortValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		sorted := make(map[string]any, len(v))
		for _, key := range keys {
			sorted[key] = sortValue(v[key])
		}
		return sorted
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sortValue(item)
		}
		return out
	default:
		return value
	}
}

func stable(value json.RawMessage) string {
	if len(value) == 0 {
		return "null"
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return string(value)
	}
	out, err := json.Marshal(sortValue(decoded))
	if err != nil {
		return string(value)
	}
	return string(out)
}

func asRecord(value json.RawMessage) map[string]json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(value, &obj) != nil {
		return nil
	}
	return obj
}

func jsonString(value json.RawMessage) string {
	var s string
	if json.Unmarshal(value, &s) == nil {
		return s
	}
	return ""
}

func jsonBool(value json.RawMessage) bool {
	var b bool
	if json.Unmarshal(value, &b) == nil {
		return b
	}
	return false
}

func urlHost(value json.RawMessage) string {
	obj := asRecord(value)
	if obj == nil {
		return ""
	}
	urlValue, ok := obj["url"]
	if !ok {
		return ""
	}
	url := jsonString(urlValue)
	if url == "" {
		return ""
	}
	withoutScheme := url
	if parts := strings.SplitN(url, "://", 2); len(parts) == 2 {
		withoutScheme = parts[1]
	}
	if slash := strings.Index(withoutScheme, "/"); slash >= 0 {
		return withoutScheme[:slash]
	}
	return withoutScheme
}

func isWildcardPermission(node *types.GraphNode) bool {
	if node.EntityKind != types.KindPermission {
		return false
	}
	rule := node.EntityName
	if obj := asRecord(node.EffectiveValue); obj != nil {
		if v, ok := obj["rule"]; ok {
			if s := jsonString(v); s != "" {
				rule = s
			}
		}
	}
	return strings.Contains(rule, "*") || strings.Contains(rule, "(*)") || rule == "*"
}

func executableAppeared(before *types.GraphNode, after *types.GraphNode) bool {
	if after.EntityKind != types.KindSkill {
		return false
	}
	beforeExec := false
	if before != nil {
		if obj := asRecord(before.EffectiveValue); obj != nil {
			if v, ok := obj["executable"]; ok {
				beforeExec = jsonBool(v)
			}
		}
	}
	afterExec := false
	if obj := asRecord(after.EffectiveValue); obj != nil {
		if v, ok := obj["executable"]; ok {
			afterExec = jsonBool(v)
		}
	}
	return !beforeExec && afterExec
}

func mcpChangedFields(before, after *types.GraphNode) []string {
	empty := map[string]json.RawMessage{}
	beforeValue := asRecord(before.EffectiveValue)
	if beforeValue == nil {
		beforeValue = empty
	}
	afterValue := asRecord(after.EffectiveValue)
	if afterValue == nil {
		afterValue = empty
	}

	var fields []string
	for _, field := range []string{"command", "transport"} {
		beforeField := json.RawMessage("null")
		if v, ok := beforeValue[field]; ok {
			beforeField = v
		}
		afterField := json.RawMessage("null")
		if v, ok := afterValue[field]; ok {
			afterField = v
		}
		if stable(beforeField) != stable(afterField) {
			fields = append(fields, field)
		}
	}
	if urlHost(before.EffectiveValue) != urlHost(after.EffectiveValue) {
		fields = append(fields, "urlHost")
	}
	return fields
}

func pushAdded(
	semanticChanges *[]SemanticChange,
	after *types.GraphNode,
	code SemanticChangeCode,
	severity types.Severity,
) {
	sourcePath := after.SourcePath
	*semanticChanges = append(*semanticChanges, SemanticChange{
		Code:       code,
		EntityKind: after.EntityKind,
		EntityName: after.EntityName,
		Severity:   severity,
		Before:     nil,
		After:      after.EffectiveValue,
		Details: SemanticChangeDetails{
			ChangedFields: []string{},
			SourcePath:    &sourcePath,
			Extra:         map[string]json.RawMessage{},
		},
	})
}

// DiffGraphs compares baseline and current graphs for semantic and raw changes.
func DiffGraphs(baselineGraph, currentGraph []types.GraphNode) GraphDiff {
	beforeByIdentity := make(map[string]*types.GraphNode, len(baselineGraph))
	for i := range baselineGraph {
		node := &baselineGraph[i]
		beforeByIdentity[graphIdentity(node)] = node
	}
	afterByIdentity := make(map[string]*types.GraphNode, len(currentGraph))
	for i := range currentGraph {
		node := &currentGraph[i]
		afterByIdentity[graphIdentity(node)] = node
	}

	var semanticChanges []SemanticChange

	for identity, after := range afterByIdentity {
		before := beforeByIdentity[identity]
		if before == nil {
			switch after.EntityKind {
			case types.KindMcpServer:
				pushAdded(&semanticChanges, after, SemanticMcpAdded, types.SeverityMedium)
			case types.KindAgentConfig:
				pushAdded(&semanticChanges, after, SemanticAgentConfigAdded, types.SeverityMedium)
			case types.KindEnvKey:
				pushAdded(&semanticChanges, after, SemanticEnvKeyAdded, types.SeverityMedium)
			case types.KindSkill:
				pushAdded(&semanticChanges, after, SemanticSkillAdded, types.SeverityLow)
			case types.KindHook:
				pushAdded(&semanticChanges, after, SemanticHookAdded, types.SeverityMedium)
			case types.KindPermission:
				pushAdded(&semanticChanges, after, SemanticPermissionChanged, types.SeverityMedium)
			case types.KindAgentInstruction:
				pushAdded(&semanticChanges, after, SemanticInstructionChanged, types.SeverityMedium)
			}
			if isWildcardPermission(after) {
				pushAdded(&semanticChanges, after, SemanticPermissionWildcardAdded, types.SeverityHigh)
			}
			if executableAppeared(nil, after) {
				pushAdded(&semanticChanges, after, SemanticSkillExecutableAppeared, types.SeverityHigh)
			}
			continue
		}

		if after.EntityKind == types.KindMcpServer && stable(before.EffectiveValue) != stable(after.EffectiveValue) {
			sourcePath := after.SourcePath
			semanticChanges = append(semanticChanges, SemanticChange{
				Code:       SemanticMcpChanged,
				EntityKind: after.EntityKind,
				EntityName: after.EntityName,
				Severity:   types.SeverityMedium,
				Before:     before.EffectiveValue,
				After:      after.EffectiveValue,
				Details: SemanticChangeDetails{
					ChangedFields: mcpChangedFields(before, after),
					SourcePath:    &sourcePath,
					Extra:         map[string]json.RawMessage{},
				},
			})
		}

		pushChanged := func(kind types.EvidenceKind, code SemanticChangeCode) {
			if after.EntityKind == kind && stable(before.EffectiveValue) != stable(after.EffectiveValue) {
				sourcePath := after.SourcePath
				semanticChanges = append(semanticChanges, SemanticChange{
					Code:       code,
					EntityKind: after.EntityKind,
					EntityName: after.EntityName,
					Severity:   types.SeverityMedium,
					Before:     before.EffectiveValue,
					After:      after.EffectiveValue,
					Details: SemanticChangeDetails{
						ChangedFields: []string{},
						SourcePath:    &sourcePath,
						Extra:         map[string]json.RawMessage{},
					},
				})
			}
		}
		pushChanged(types.KindAgentConfig, SemanticAgentConfigChanged)
		pushChanged(types.KindHook, SemanticHookChanged)
		pushChanged(types.KindPermission, SemanticPermissionChanged)
		pushChanged(types.KindAgentInstruction, SemanticInstructionChanged)

		if isWildcardPermission(after) && !isWildcardPermission(before) {
			sourcePath := after.SourcePath
			semanticChanges = append(semanticChanges, SemanticChange{
				Code:       SemanticPermissionWildcardAdded,
				EntityKind: after.EntityKind,
				EntityName: after.EntityName,
				Severity:   types.SeverityHigh,
				Before:     before.EffectiveValue,
				After:      after.EffectiveValue,
				Details: SemanticChangeDetails{
					ChangedFields: []string{},
					SourcePath:    &sourcePath,
					Extra:         map[string]json.RawMessage{},
				},
			})
		}
		if executableAppeared(before, after) {
			sourcePath := after.SourcePath
			semanticChanges = append(semanticChanges, SemanticChange{
				Code:       SemanticSkillExecutableAppeared,
				EntityKind: after.EntityKind,
				EntityName: after.EntityName,
				Severity:   types.SeverityHigh,
				Before:     before.EffectiveValue,
				After:      after.EffectiveValue,
				Details: SemanticChangeDetails{
					ChangedFields: []string{},
					SourcePath:    &sourcePath,
					Extra:         map[string]json.RawMessage{},
				},
			})
		}
		if after.EntityKind == types.KindUnsupported && stable(before.EffectiveValue) != stable(after.EffectiveValue) {
			sourcePath := after.SourcePath
			semanticChanges = append(semanticChanges, SemanticChange{
				Code:       SemanticUnsupportedStateChanged,
				EntityKind: after.EntityKind,
				EntityName: after.EntityName,
				Severity:   types.SeverityMedium,
				Before:     before.EffectiveValue,
				After:      after.EffectiveValue,
				Details: SemanticChangeDetails{
					ChangedFields: []string{},
					SourcePath:    &sourcePath,
					Extra:         map[string]json.RawMessage{},
				},
			})
		}
	}

	for identity, before := range beforeByIdentity {
		if _, ok := afterByIdentity[identity]; ok {
			continue
		}
		sourcePath := before.SourcePath
		switch before.EntityKind {
		case types.KindMcpServer:
			semanticChanges = append(semanticChanges, SemanticChange{
				Code: SemanticMcpRemoved, EntityKind: before.EntityKind, EntityName: before.EntityName,
				Severity: types.SeverityMedium, Before: before.EffectiveValue, After: nil,
				Details: SemanticChangeDetails{ChangedFields: []string{}, SourcePath: &sourcePath, Extra: map[string]json.RawMessage{}},
			})
		case types.KindAgentConfig:
			semanticChanges = append(semanticChanges, SemanticChange{
				Code: SemanticAgentConfigRemoved, EntityKind: before.EntityKind, EntityName: before.EntityName,
				Severity: types.SeverityMedium, Before: before.EffectiveValue, After: nil,
				Details: SemanticChangeDetails{ChangedFields: []string{}, SourcePath: &sourcePath, Extra: map[string]json.RawMessage{}},
			})
		case types.KindEnvKey:
			semanticChanges = append(semanticChanges, SemanticChange{
				Code: SemanticEnvKeyRemoved, EntityKind: before.EntityKind, EntityName: before.EntityName,
				Severity: types.SeverityMedium, Before: before.EffectiveValue, After: nil,
				Details: SemanticChangeDetails{ChangedFields: []string{}, SourcePath: &sourcePath, Extra: map[string]json.RawMessage{}},
			})
		case types.KindSkill:
			semanticChanges = append(semanticChanges, SemanticChange{
				Code: SemanticSkillRemoved, EntityKind: before.EntityKind, EntityName: before.EntityName,
				Severity: types.SeverityLow, Before: before.EffectiveValue, After: nil,
				Details: SemanticChangeDetails{ChangedFields: []string{}, SourcePath: &sourcePath, Extra: map[string]json.RawMessage{}},
			})
		case types.KindHook:
			semanticChanges = append(semanticChanges, SemanticChange{
				Code: SemanticHookRemoved, EntityKind: before.EntityKind, EntityName: before.EntityName,
				Severity: types.SeverityMedium, Before: before.EffectiveValue, After: nil,
				Details: SemanticChangeDetails{ChangedFields: []string{}, SourcePath: &sourcePath, Extra: map[string]json.RawMessage{}},
			})
		case types.KindPermission:
			extra := map[string]json.RawMessage{"removed": json.RawMessage("true")}
			semanticChanges = append(semanticChanges, SemanticChange{
				Code: SemanticPermissionChanged, EntityKind: before.EntityKind, EntityName: before.EntityName,
				Severity: types.SeverityMedium, Before: before.EffectiveValue, After: nil,
				Details: SemanticChangeDetails{ChangedFields: []string{}, SourcePath: &sourcePath, Extra: extra},
			})
		}
	}

	beforeBySource := make(map[string]*types.GraphNode, len(baselineGraph))
	for i := range baselineGraph {
		node := &baselineGraph[i]
		beforeBySource[sourceIdentity(node)] = node
	}
	afterBySource := make(map[string]*types.GraphNode, len(currentGraph))
	for i := range currentGraph {
		node := &currentGraph[i]
		afterBySource[sourceIdentity(node)] = node
	}

	sourceKeys := make(map[string]struct{})
	for key := range beforeBySource {
		sourceKeys[key] = struct{}{}
	}
	for key := range afterBySource {
		sourceKeys[key] = struct{}{}
	}
	keys := make([]string, 0, len(sourceKeys))
	for key := range sourceKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var rawSourceChanges []RawSourceChange
	for _, key := range keys {
		before := beforeBySource[key]
		after := afterBySource[key]
		switch {
		case before == nil && after != nil:
			afterID := after.EvidenceID
			rawSourceChanges = append(rawSourceChanges, RawSourceChange{
				SourcePath:       after.SourcePath,
				AfterEvidenceID:  &afterID,
				Status:           "added",
			})
		case before != nil && after == nil:
			beforeID := before.EvidenceID
			rawSourceChanges = append(rawSourceChanges, RawSourceChange{
				SourcePath:        before.SourcePath,
				BeforeEvidenceID:  &beforeID,
				Status:            "removed",
			})
		case before != nil && after != nil &&
			(before.EvidenceID != after.EvidenceID || stable(before.EffectiveValue) != stable(after.EffectiveValue)):
			beforeID := before.EvidenceID
			afterID := after.EvidenceID
			rawSourceChanges = append(rawSourceChanges, RawSourceChange{
				SourcePath:        after.SourcePath,
				BeforeEvidenceID:  &beforeID,
				AfterEvidenceID:   &afterID,
				Status:            "changed",
			})
		}
	}

	return GraphDiff{
		SemanticChanges:  semanticChanges,
		RawSourceChanges: rawSourceChanges,
	}
}