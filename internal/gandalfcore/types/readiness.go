package types

// ReadinessCategory groups readiness findings.
type ReadinessCategory string

const (
	ReadinessReady             ReadinessCategory = "ready"
	ReadinessNeedsManualAction ReadinessCategory = "needs_manual_action"
	ReadinessWarning           ReadinessCategory = "warning"
	ReadinessUnverified        ReadinessCategory = "unverified"
	ReadinessUnsupported       ReadinessCategory = "unsupported"
	ReadinessBlocked           ReadinessCategory = "blocked"
)

// ReadinessAction is a suggested remediation step.
type ReadinessAction struct {
	Label   string  `json:"label"`
	Command *string `json:"command,omitempty"`
	URL     *string `json:"url,omitempty"`
}

// ReadinessItem is a single readiness finding.
type ReadinessItem struct {
	ID         string            `json:"id"`
	Category   ReadinessCategory `json:"category"`
	Severity   Severity          `json:"severity"`
	Code       string            `json:"code"`
	Problem    string            `json:"problem"`
	Cause      string            `json:"cause"`
	Fix        string            `json:"fix"`
	Path       *string           `json:"path,omitempty"`
	EvidenceID *string           `json:"evidenceId,omitempty"`
	Command    *string           `json:"command,omitempty"`
	Actions    []ReadinessAction `json:"actions,omitempty"`
}

// ReadinessReport aggregates readiness findings for a target machine.
type ReadinessReport struct {
	TargetPlatform string                       `json:"targetPlatform"`
	Summary        map[ReadinessCategory]uint32 `json:"summary"`
	Items          []ReadinessItem              `json:"items"`
}

// ReadinessOptions configures readiness analysis.
type ReadinessOptions struct {
	SourceHomeDir  *string
	TargetPlatform *string
	ApplyContent   bool
	TargetEvidence []DiscoveredItem
	ProcessEnv     map[string]string
	PathEnv        *string
}

// ReadinessFormatOptions configures human-readable readiness output.
type ReadinessFormatOptions struct {
	MaxItems       int
	IncludeFixes   bool
	IncludeActions bool
}
