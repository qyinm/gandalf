package boost

import "github.com/qyinm/gandalf/internal/gandalfcore/types"

// Scope defines where symlinks are injected: project-local (default) or user-global.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// ActionType represents the planned mutation on an agent symlink.
type ActionType string

const (
	ActionCreateSymlink ActionType = "create"
	ActionRemoveSymlink ActionType = "remove"
	ActionKeepSymlink   ActionType = "keep"
	ActionConflictSkip  ActionType = "conflict_skip"
)

// SymlinkMutation describes a single symlink creation, removal, retention, or skip.
type SymlinkMutation struct {
	Action        ActionType    `json:"action"`
	SkillName     string        `json:"skill_name"`
	Agent         types.AgentID `json:"agent"`
	SourcePath    string        `json:"source_path"`    // Actual directory containing the skill
	TargetPath    string        `json:"target_path"`    // Path where symlink lives
	SymlinkTarget string        `json:"symlink_target"` // Value written into the symlink (relative or absolute)
	Reason        string        `json:"reason,omitempty"`
}

// BoostPlan represents the planned actions to boost an agent environment.
type BoostPlan struct {
	ProfileName    string            `json:"profile_name"`
	Scope          Scope             `json:"scope"`
	ResolvedSkills []string          `json:"resolved_skills"`
	Mutations      []SymlinkMutation `json:"mutations"`
	Warnings       []string          `json:"warnings,omitempty"`
}

// BoostApplyResult contains the results of applying a BoostPlan.
type BoostApplyResult struct {
	Success       bool              `json:"success"`
	ActiveProfile string            `json:"active_profile"`
	Scope         Scope             `json:"scope"`
	Created       []SymlinkMutation `json:"created,omitempty"`
	Removed       []SymlinkMutation `json:"removed,omitempty"`
	Skipped       []SymlinkMutation `json:"skipped,omitempty"`
	Errors        []string          `json:"errors,omitempty"`
}

// BoostState represents the persisted state in .gandalf/.boost.json.
type BoostState struct {
	ActiveProfile  string               `json:"active_profile"`
	Scope          Scope                `json:"scope"`
	ProjectRoot    string               `json:"project_root"`
	UpdatedAt      string               `json:"updated_at"`
	InjectedSkills []InjectedSkillEntry `json:"injected_skills,omitempty"`
}

// InjectedSkillEntry tracks a currently injected symlink.
type InjectedSkillEntry struct {
	SkillName  string        `json:"skill_name"`
	Agent      types.AgentID `json:"agent"`
	TargetPath string        `json:"target_path"`
}
