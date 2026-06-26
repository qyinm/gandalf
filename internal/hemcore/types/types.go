package types

import (
	"encoding/json"
	"strings"
)

type AgentID string

const (
	AgentClaudeCode AgentID = "claude-code"
	AgentCodex      AgentID = "codex"
	AgentCursor     AgentID = "cursor"
	AgentOpencode   AgentID = "opencode"
	AgentPiAgent    AgentID = "pi-agent"
	AgentProject    AgentID = "project"
	AgentUnknown    AgentID = "unknown"
)

func ParseAgentID(value string) AgentID {
	switch value {
	case "claude-code":
		return AgentClaudeCode
	case "codex":
		return AgentCodex
	case "cursor":
		return AgentCursor
	case "opencode":
		return AgentOpencode
	case "pi-agent":
		return AgentPiAgent
	case "project":
		return AgentProject
	case "unknown":
		return AgentUnknown
	default:
		return AgentUnknown
	}
}

func (a AgentID) String() string {
	if a == "" {
		return string(AgentUnknown)
	}
	return string(a)
}

func (a *AgentID) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*a = ParseAgentID(value)
	return nil
}

type EvidenceKind string

const (
	KindAgentConfig       EvidenceKind = "agent_config"
	KindAgentInstruction  EvidenceKind = "agent_instruction"
	KindMcpServer         EvidenceKind = "mcp_server"
	KindPermission        EvidenceKind = "permission"
	KindSkill             EvidenceKind = "skill"
	KindExtension         EvidenceKind = "extension"
	KindEnvKey            EvidenceKind = "env_key"
	KindHook              EvidenceKind = "hook"
	KindSymlink           EvidenceKind = "symlink"
	KindUnsupported       EvidenceKind = "unsupported"
)

func (k EvidenceKind) String() string { return string(k) }

type RestorePolicy string

const (
	RestoreFullContent         RestorePolicy = "full_content_supported"
	RestoreStructuredFields    RestorePolicy = "structured_fields_only"
	RestoreKeyInventory        RestorePolicy = "key_inventory_only"
	RestoreNotSupported        RestorePolicy = "not_supported"
)

type EvidenceScope string

const (
	ScopeUser    EvidenceScope = "user"
	ScopeProject EvidenceScope = "project"
	ScopeManaged EvidenceScope = "managed"
	ScopeUnknown EvidenceScope = "unknown"
)

func (s EvidenceScope) String() string { return string(s) }

type CaptureStatus string

const (
	CaptureCaptured       CaptureStatus = "captured"
	CaptureRedacted       CaptureStatus = "redacted"
	CaptureOmitted        CaptureStatus = "omitted"
	CaptureParseFailed    CaptureStatus = "parse_failed"
	CaptureUnsafeToExport CaptureStatus = "unsafe_to_export"
	CaptureUnsupported    CaptureStatus = "unsupported"
)

func (s CaptureStatus) String() string { return string(s) }

type Severity string

const (
	SeverityNone     Severity = "none"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type EvidenceParser string

const (
	ParserJSON       EvidenceParser = "json"
	ParserToml       EvidenceParser = "toml"
	ParserMarkdown   EvidenceParser = "markdown"
	ParserDotenv     EvidenceParser = "dotenv"
	ParserFilesystem EvidenceParser = "filesystem"
	ParserUnknown    EvidenceParser = "unknown"
)

type EvidenceConfidence string

const (
	ConfidenceLow    EvidenceConfidence = "low"
	ConfidenceMedium EvidenceConfidence = "medium"
	ConfidenceHigh   EvidenceConfidence = "high"
)

type DiscoveredItem struct {
	ID             string             `json:"id"`
	Agent          AgentID            `json:"agent"`
	Kind           EvidenceKind       `json:"kind"`
	SourcePath     string             `json:"sourcePath"`
	Scope          EvidenceScope      `json:"scope"`
	Precedence     uint32             `json:"precedence"`
	Parser         EvidenceParser     `json:"parser"`
	Sensitivity    string             `json:"sensitivity"`
	ContentPolicy  string             `json:"contentPolicy"`
	RestorePolicy  RestorePolicy      `json:"restorePolicy"`
	CaptureStatus  CaptureStatus      `json:"captureStatus"`
	Confidence     EvidenceConfidence `json:"confidence"`
	Name           *string            `json:"name,omitempty"`
	Value          json.RawMessage    `json:"value,omitempty"`
	Checksum       *string            `json:"checksum,omitempty"`
	Metadata       json.RawMessage    `json:"metadata,omitempty"`
}

type SnapshotManifest struct {
	SchemaVersion string           `json:"schemaVersion"`
	Name          string           `json:"name"`
	CreatedAt     string           `json:"createdAt"`
	ProjectPath   string           `json:"projectPath"`
	Security      SnapshotSecurity `json:"security"`
}

type SnapshotSecurity struct {
	RawSecretsIncluded bool   `json:"rawSecretsIncluded"`
	RedactionPolicy    string `json:"redactionPolicy"`
}

type SnapshotContentEntry struct {
	EvidenceID    string  `json:"evidenceId"`
	SourcePath    string  `json:"sourcePath"`
	RestorePath   string  `json:"restorePath"`
	Checksum      string  `json:"checksum"`
	ByteLength    uint64  `json:"byteLength"`
	Encoding      string  `json:"encoding"`
	StoragePath   string  `json:"storagePath"`
	CaptureStatus string  `json:"captureStatus"`
	Reason        *string `json:"reason,omitempty"`
	Content       *string `json:"content,omitempty"`
}

type Snapshot struct {
	Manifest      SnapshotManifest       `json:"manifest"`
	Evidence      []DiscoveredItem       `json:"evidence"`
	Graph         []GraphNode            `json:"graph"`
	AuditFindings []AuditFinding         `json:"auditFindings"`
	Provenance    []ProvenanceEntry      `json:"provenance"`
	Content       []SnapshotContentEntry `json:"content,omitempty"`
}

type GraphNode struct {
	ID             string             `json:"id"`
	Agent          AgentID            `json:"agent"`
	Scope          EvidenceScope      `json:"scope"`
	SourcePath     string             `json:"sourcePath"`
	EntityKind     EvidenceKind       `json:"entityKind"`
	EntityName     string             `json:"entityName"`
	EffectiveValue json.RawMessage    `json:"effectiveValue"`
	OverriddenBy   *string            `json:"overriddenBy,omitempty"`
	Confidence     EvidenceConfidence `json:"confidence"`
	EvidenceID     string             `json:"evidenceId"`
}

type AuditFinding struct {
	Code       string   `json:"code"`
	Severity   Severity `json:"severity"`
	Problem    string   `json:"problem"`
	Cause      string   `json:"cause"`
	Fix        string   `json:"fix"`
	Path       *string  `json:"path,omitempty"`
	EvidenceID *string  `json:"evidenceId,omitempty"`
}

type ProvenanceEntry struct {
	NodeID        string             `json:"nodeId"`
	EvidenceID    string             `json:"evidenceId"`
	SourcePath    string             `json:"sourcePath"`
	Scope         EvidenceScope      `json:"scope"`
	Precedence    uint32             `json:"precedence"`
	Confidence    EvidenceConfidence `json:"confidence"`
	CaptureStatus CaptureStatus      `json:"captureStatus"`
}

type ScanOptions struct {
	ProjectPath string
	HomeDir     string
	StoreDir    string
	Explain     bool
	Agent       *AgentID
	Scope       *EvidenceScope
}

type ScanTrust struct {
	ReadOnly            bool     `json:"readOnly"`
	Network             string   `json:"network"`
	CommandsExecuted    []string `json:"commandsExecuted"`
	StoreWriteLocation  string   `json:"storeWriteLocation"`
}

type ScanResult struct {
	Trust      ScanTrust        `json:"trust"`
	Evidence   []DiscoveredItem `json:"evidence"`
	BlindSpots []string         `json:"blindSpots"`
}

type RuntimeOptions struct {
	ProjectPath    string
	HomeDir        string
	StoreDir       string
	Agent          *AgentID
	Scope          *EvidenceScope
	CaptureContent bool
}

type CurrentState struct {
	Scan          ScanResult
	Snapshot      Snapshot
	StoreFindings []AuditFinding
}

type SnapError struct {
	Code    string
	Problem string
	Cause   string
	Fix     string
	Path    *string
}

func ParseScope(value string) (EvidenceScope, bool) {
	switch strings.TrimSpace(value) {
	case "user":
		return ScopeUser, true
	case "project":
		return ScopeProject, true
	case "managed":
		return ScopeManaged, true
	case "unknown":
		return ScopeUnknown, true
	default:
		return "", false
	}
}

type TimelineEntrySource string

const TimelineSourceManual TimelineEntrySource = "manual"

type TimelineEntryEventKind string

const (
	TimelineEventBaseline      TimelineEntryEventKind = "baseline"
	TimelineEventSetupChanged  TimelineEntryEventKind = "setup_changed"
	TimelineEventUnchanged     TimelineEntryEventKind = "unchanged"
)

type TimelineRestoreReadiness string

const (
	TimelineRestoreFull        TimelineRestoreReadiness = "full"
	TimelineRestorePartial     TimelineRestoreReadiness = "partial"
	TimelineRestoreObserveOnly TimelineRestoreReadiness = "observe-only"
)

type TimelineConfidence string

const (
	TimelineConfidenceLow    TimelineConfidence = "low"
	TimelineConfidenceMedium TimelineConfidence = "medium"
	TimelineConfidenceHigh   TimelineConfidence = "high"
)

type TimelineChangeSummary struct {
	PreviousEntryID       *string  `json:"previousEntryId,omitempty"`
	PreviousSnapshotName  *string  `json:"previousSnapshotName,omitempty"`
	HasChanges            bool     `json:"hasChanges"`
	SemanticChangeCount   uint32   `json:"semanticChangeCount"`
	RawSourceChangeCount  uint32   `json:"rawSourceChangeCount"`
	Highlights            []string `json:"highlights"`
}

type TimelineChangedSurface struct {
	Kind        string          `json:"kind"`
	ChangeType  string          `json:"changeType"`
	Path        string          `json:"path"`
	EntityName  *string         `json:"entityName,omitempty"`
	Restorable  bool            `json:"restorable"`
	ObserveOnly bool            `json:"observeOnly"`
	Before      json.RawMessage `json:"before,omitempty"`
	After       json.RawMessage `json:"after,omitempty"`
}

type TimelineEntry struct {
	SchemaVersion      string                   `json:"schemaVersion"`
	ID                 string                   `json:"id"`
	Source             TimelineEntrySource      `json:"source"`
	EventKind          TimelineEntryEventKind   `json:"eventKind"`
	Title              string                   `json:"title"`
	ProjectPath        string                   `json:"projectPath"`
	Agent              *AgentID                 `json:"agent,omitempty"`
	Agents             []AgentID                `json:"agents"`
	BeforeSnapshotName *string                  `json:"beforeSnapshotName,omitempty"`
	AfterSnapshotName  string                   `json:"afterSnapshotName"`
	CaptureID          string                   `json:"captureId"`
	CreatedAt          string                   `json:"createdAt"`
	ObservedAt         string                   `json:"observedAt"`
	ChangedSurfaces    []TimelineChangedSurface `json:"changedSurfaces"`
	RestoreReadiness   TimelineRestoreReadiness `json:"restoreReadiness"`
	Confidence         TimelineConfidence       `json:"confidence"`
	ConfidenceReason   string                   `json:"confidenceReason"`
	EvidenceCount      uint32                   `json:"evidenceCount"`
	GraphNodeCount     uint32                   `json:"graphNodeCount"`
	AuditFindingCount  uint32                   `json:"auditFindingCount"`
	Changes            TimelineChangeSummary    `json:"changes"`
}