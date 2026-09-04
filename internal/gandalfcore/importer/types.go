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
	// Selection optionally restricts which reconciled MCP servers and skills are
	// written. Nil means include everything (default CLI behavior). Interactive
	// flows (TUI wizard) populate this from user toggles.
	Selection *Selection
}

// Selection describes which reconciled items to include in the final manifest.
// Maps are keyed by server/skill name; a missing key means excluded.
type Selection struct {
	Servers map[string]bool
	Skills  map[string]bool
}

// IncludesServer reports whether the named server is selected. A nil Selection
// includes everything.
func (s *Selection) IncludesServer(name string) bool {
	if s == nil {
		return true
	}
	return s.Servers[name]
}

// IncludesSkill reports whether the named skill is selected. A nil Selection
// includes everything.
func (s *Selection) IncludesSkill(name string) bool {
	if s == nil {
		return true
	}
	return s.Skills[name]
}

// DiscoveredSource represents a detected configuration source.
type DiscoveredSource struct {
	Agent       types.AgentID // types.AgentCursor, types.AgentClaudeCode, types.AgentCodex, or "standard"
	Scope       string        // "project" or "global"
	Path        string
	ServerCount int
	SkillCount  int
	// ServerNames and SkillNames list the item names this source contributed
	// (before cross-source precedence merging). Interactive surfaces use them
	// to group items by source agent.
	ServerNames []string
	SkillNames  []string
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
