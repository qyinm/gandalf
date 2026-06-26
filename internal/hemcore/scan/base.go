package scan

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/policy"
	"github.com/qyinm/hem/internal/hemcore/types"
)

// EvidenceBaseTarget carries shared fields for constructing discovered items.
type EvidenceBaseTarget struct {
	Agent         types.AgentID
	SourcePath    string
	Scope         types.EvidenceScope
	Precedence    uint32
	Parser        types.EvidenceParser
	Sensitivity   string
	ContentPolicy string
}

// ItemIDTarget carries fields used for deterministic item IDs.
type ItemIDTarget struct {
	Agent      types.AgentID
	SourcePath string
	Scope      types.EvidenceScope
}

// ScannerBase provides helpers shared by agent-specific scanners.
type ScannerBase struct {
	AgentID types.AgentID
}

// NewScannerBase constructs a scanner helper for the given agent.
func NewScannerBase(agentID types.AgentID) ScannerBase {
	return ScannerBase{AgentID: agentID}
}

// ItemID builds a deterministic evidence ID.
func (b ScannerBase) ItemID(target ItemIDTarget, suffix string) string {
	return ScannerItemID(target.Scope, target.Agent, target.SourcePath, suffix)
}

// Captured builds a captured evidence item.
func (b ScannerBase) Captured(
	target EvidenceBaseTarget,
	kind types.EvidenceKind,
	metadata any,
	value any,
) types.DiscoveredItem {
	return types.DiscoveredItem{
		ID:            ScannerItemID(target.Scope, target.Agent, target.SourcePath, string(kind)),
		Agent:         target.Agent,
		Kind:          kind,
		SourcePath:    target.SourcePath,
		Scope:         target.Scope,
		Precedence:    target.Precedence,
		Parser:        target.Parser,
		Sensitivity:   target.Sensitivity,
		ContentPolicy: target.ContentPolicy,
		RestorePolicy: policy.RestorePolicyFor(kind),
		CaptureStatus: types.CaptureCaptured,
		Confidence:    types.ConfidenceHigh,
		Value:         marshalRaw(value),
		Metadata:      marshalRaw(metadata),
	}
}

// ParseFailed builds a parse-failed evidence item.
func (b ScannerBase) ParseFailed(
	target EvidenceBaseTarget,
	kind types.EvidenceKind,
	err string,
) types.DiscoveredItem {
	item := b.Captured(target, kind, map[string]any{"error": err}, nil)
	item.ID = ScannerItemID(
		target.Scope,
		target.Agent,
		target.SourcePath,
		string(kind)+"-parse-failed",
	)
	item.CaptureStatus = types.CaptureParseFailed
	return item
}

// EvidenceBaseTargetFromScanTarget converts a scan target to evidence base fields.
func EvidenceBaseTargetFromScanTarget(target ScanTarget) EvidenceBaseTarget {
	return EvidenceBaseTarget{
		Agent:         target.Agent,
		SourcePath:    target.SourcePath,
		Scope:         target.Scope,
		Precedence:    target.Precedence,
		Parser:        target.Parser,
		Sensitivity:   target.Sensitivity,
		ContentPolicy: target.ContentPolicy,
	}
}

// ScannerItemID builds a stable, filesystem-safe evidence identifier.
func ScannerItemID(
	scope types.EvidenceScope,
	agent types.AgentID,
	sourcePath string,
	suffix string,
) string {
	id := scope.String() + "." + agent.String() + "." + sourcePath + "." + suffix
	if strings.HasPrefix(id, "~/") {
		id = "home/" + id[2:]
	}
	var b strings.Builder
	for _, ch := range id {
		if (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '.' || ch == '_' || ch == '-' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('.')
		}
	}
	return strings.Trim(strings.ToLower(b.String()), ".")
}

// AsRecord returns the value as a JSON object map.
func AsRecord(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	case json.RawMessage:
		var obj map[string]any
		if err := json.Unmarshal(v, &obj); err != nil {
			return nil, false
		}
		return obj, true
	default:
		return nil, false
	}
}

// ArrayOfStrings extracts string values from a JSON array.
func ArrayOfStrings(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

// NormalizeSourcePath maps an absolute path to a project-relative source path.
func NormalizeSourcePath(root, absolutePath string) string {
	rel, err := filepath.Rel(root, absolutePath)
	if err != nil {
		return filepath.ToSlash(absolutePath)
	}
	return filepath.ToSlash(rel)
}

// UnquoteYAMLScalar strips optional YAML/JSON quotes from a scalar value.
func UnquoteYAMLScalar(value string) string {
	trimmed := strings.TrimSpace(value)
	return strings.Trim(strings.Trim(trimmed, "'"), "\"")
}

// ValueToJSString approximates JavaScript's String() for JSON values.
func ValueToJSString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, ValueToJSString(item))
		}
		return strings.Join(parts, ",")
	case map[string]any:
		return "[object Object]"
	case float64:
		if v == float64(int64(v)) {
			return strings.TrimSuffix(strings.TrimSuffix(
				strings.Replace(jsonNumber(v), ".0", "", 1),
				".0",
			), ".")
		}
		return jsonNumber(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func jsonNumber(v float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func marshalRaw(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return b
}

func stringPtr(value string) *string {
	return &value
}