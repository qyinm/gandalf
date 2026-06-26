package plugins

import (
	"github.com/qyinm/hem/internal/hemcore/scan"
	"github.com/qyinm/hem/internal/hemcore/types"
)

// ClaudeCodeScanner discovers Claude Code configuration targets.
type ClaudeCodeScanner struct{}

func (ClaudeCodeScanner) AgentID() types.AgentID   { return types.AgentClaudeCode }
func (ClaudeCodeScanner) AgentName() string        { return "Claude Code" }
func (ClaudeCodeScanner) Description() string {
	return "Claude Code agent configuration (prompts, MCP servers, settings, skills)"
}

func (ClaudeCodeScanner) Targets(projectPath, homeDir string) []scan.ScanTarget {
	metadataOnly := true
	dir := true
	return []scan.ScanTarget{
		scan.ProjectTarget(projectPath, "CLAUDE.md", types.AgentClaudeCode, types.KindAgentInstruction, types.ParserMarkdown, scan.ScanTargetOverrides{}),
		scan.ProjectTarget(projectPath, ".mcp.json", types.AgentClaudeCode, types.KindAgentConfig, types.ParserJSON, scan.ScanTargetOverrides{}),
		scan.ProjectTarget(projectPath, ".claude/settings.json", types.AgentClaudeCode, types.KindAgentConfig, types.ParserJSON, scan.ScanTargetOverrides{}),
		scan.HomeTarget(homeDir, ".claude/settings.json", types.AgentClaudeCode, types.KindAgentConfig, types.ParserJSON, scan.ScanTargetOverrides{}),
		scan.HomeTarget(homeDir, ".claude.json", types.AgentClaudeCode, types.KindAgentConfig, types.ParserJSON, scan.ScanTargetOverrides{
			MetadataOnly: &metadataOnly,
			Sensitivity:  stringPtr("metadata"),
		}),
		scan.HomeTarget(homeDir, ".claude/agents", types.AgentClaudeCode, types.KindUnsupported, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".claude/skills", types.AgentClaudeCode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
	}
}

func (ClaudeCodeScanner) Scan(*scan.ScannerContext) []types.DiscoveredItem { return nil }