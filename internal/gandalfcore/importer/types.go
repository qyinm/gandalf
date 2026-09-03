package importer

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ImportOptions configures the import run.
type ImportOptions struct {
	ProjectPath string
	HomeDir     string
	ProjectOnly bool   // If true, ignore user-global ~/.cursor, ~/.claude.json, ~/.codex
	FromPath    string // If set, import strictly from this single file
	DryRun      bool
	Force       bool
	OutputFile  string
}

// DiscoveredSource represents a detected configuration source.
type DiscoveredSource struct {
	Agent       types.AgentID // types.AgentCursor, types.AgentClaudeCode, types.AgentCodex, or "standard"
	Scope       string        // "project" or "global"
	Path        string
	ServerCount int
	SkillCount  int
}

// ImportResult contains the outcome of an import operation.
type ImportResult struct {
	Manifest       *manifest.Manifest
	Sources        []DiscoveredSource
	ExtractedEnvs  map[string]string
	Warnings       []string
	FormattedTOML  string
	MirroredSkills []string
}
