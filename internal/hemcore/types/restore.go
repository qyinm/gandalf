package types

import "encoding/json"

type RestoreAction string

const (
	RestoreActionCreate      RestoreAction = "create"
	RestoreActionUpdate      RestoreAction = "update"
	RestoreActionDelete      RestoreAction = "delete"
	RestoreActionSkip        RestoreAction = "skip"
	RestoreActionConflict    RestoreAction = "conflict"
	RestoreActionUnsupported RestoreAction = "unsupported"
)

type ItemDiff struct {
	Changes   []string `json:"changes"`
	Additions []string `json:"additions"`
	Removals  []string `json:"removals"`
}

type RestorePlanItem struct {
	ItemID               string          `json:"itemId"`
	Agent                AgentID         `json:"agent"`
	Kind                 EvidenceKind    `json:"kind"`
	SourcePath           string          `json:"sourcePath"`
	DependsOn            []string        `json:"dependsOn"`
	Action               RestoreAction   `json:"action"`
	CurrentState         *DiscoveredItem `json:"currentState,omitempty"`
	TargetState          *DiscoveredItem `json:"targetState,omitempty"`
	Diff                 ItemDiff        `json:"diff"`
	RiskLevel            Severity        `json:"riskLevel"`
	RiskReason           string          `json:"riskReason"`
	NeedsConfirmation    bool            `json:"needsConfirmation"`
	ConfirmationPrompt   string          `json:"confirmationPrompt"`
	RollbackInstruction  string          `json:"rollbackInstruction"`
}

type RiskSummary struct {
	None     uint32 `json:"none"`
	Low      uint32 `json:"low"`
	Medium   uint32 `json:"medium"`
	High     uint32 `json:"high"`
	Critical uint32 `json:"critical"`
}

type UnsupportedPlanItem struct {
	ItemID     string       `json:"itemId"`
	Agent      AgentID      `json:"agent"`
	Kind       EvidenceKind `json:"kind"`
	SourcePath string       `json:"sourcePath"`
	Reason     string       `json:"reason"`
}

type RollbackStep struct {
	ItemID      string `json:"itemId"`
	Action      string `json:"action"`
	Instruction string `json:"instruction"`
}

type RollbackPlan struct {
	Steps []RollbackStep `json:"steps"`
}

type RestorePlanMetadata struct {
	PlannerVersion string `json:"plannerVersion"`
	GeneratedBy    string `json:"generatedBy"`
}

type RestorePlan struct {
	PlanID            string                `json:"planId"`
	SourceSnapshot    string                `json:"sourceSnapshot"`
	TargetProject     string                `json:"targetProject"`
	TargetHome        string                `json:"targetHome"`
	CreatedAt         string                `json:"createdAt"`
	ItemCount         uint32                `json:"itemCount"`
	RiskSummary       RiskSummary           `json:"riskSummary"`
	Items             []RestorePlanItem     `json:"items"`
	RollbackPlan      RollbackPlan          `json:"rollbackPlan"`
	ExecutionOrder    []string              `json:"executionOrder"`
	UnsupportedItems  []UnsupportedPlanItem `json:"unsupportedItems"`
	PlanMetadata      RestorePlanMetadata   `json:"planMetadata"`
}

type RestoreItemStatus string

const (
	RestoreItemStatusPending     RestoreItemStatus = "pending"
	RestoreItemStatusApplied     RestoreItemStatus = "applied"
	RestoreItemStatusFailed      RestoreItemStatus = "failed"
	RestoreItemStatusSkipped     RestoreItemStatus = "skipped"
	RestoreItemStatusUnsupported RestoreItemStatus = "unsupported"
)

type RestoreItem struct {
	ItemID         string             `json:"itemId"`
	Path           string             `json:"path"`
	ItemType       string             `json:"type"`
	Source         string             `json:"source"`
	Dest           string             `json:"dest"`
	Action         *RestoreAction     `json:"action,omitempty"`
	Status         RestoreItemStatus  `json:"status"`
	ErrorMessage   *string            `json:"errorMessage,omitempty"`
	SkipReason     *string            `json:"skipReason,omitempty"`
	ExecutionOrder uint32             `json:"executionOrder"`
	RollbackState  json.RawMessage    `json:"rollbackState,omitempty"`
	TargetContent  json.RawMessage    `json:"targetContent,omitempty"`
	CanRollback    bool               `json:"canRollback"`
	Metadata       json.RawMessage    `json:"metadata,omitempty"`
	ApplyAt        *string            `json:"applyAt,omitempty"`
}

type RestoreOptions struct {
	SourceSnapshot string
	ProjectPath    string
	HomeDir        string
	StoreDir       string
	DryRun         bool
	Agent          *AgentID
	Scope          *EvidenceScope
}

type ApplyOptions struct {
	FailFast    bool
	Rollback    *bool
	HomeDir     *string
	ProjectPath *string
}

type ApplyFailure struct {
	ItemID string `json:"itemId"`
	Reason string `json:"reason"`
}

type ApplySummary struct {
	Total          uint32                        `json:"total"`
	Successful     uint32                        `json:"successful"`
	Failed         uint32                        `json:"failed"`
	Skipped        uint32                        `json:"skipped"`
	Unsupported    uint32                        `json:"unsupported"`
	Failures       []ApplyFailure                `json:"failures"`
	AppliedItems   []RestoreItem                 `json:"appliedItems"`
	StatusRegistry map[string]RestoreItemStatus  `json:"statusRegistry"`
}

type UndoStatus string

const (
	UndoStatusUndone  UndoStatus = "undone"
	UndoStatusSkipped UndoStatus = "skipped"
	UndoStatusFailed  UndoStatus = "failed"
)

type UndoResult struct {
	ItemID string      `json:"itemId"`
	Status UndoStatus  `json:"status"`
	Reason *string     `json:"reason,omitempty"`
}

type RollbackSummary struct {
	Total   uint32       `json:"total"`
	Undone  uint32       `json:"undone"`
	Skipped uint32       `json:"skipped"`
	Failed  uint32       `json:"failed"`
	Results []UndoResult `json:"results"`
}

type ApplyWithRollbackResult struct {
	ApplySummary    ApplySummary     `json:"applySummary"`
	RollbackSummary *RollbackSummary `json:"rollbackSummary,omitempty"`
}