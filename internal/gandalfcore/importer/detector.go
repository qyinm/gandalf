package importer

import (
	"os"
	"path/filepath"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// DetectedCandidate represents a detected file or directory candidate for import.
type DetectedCandidate struct {
	Agent types.AgentID
	Scope string // "project" or "global"
	Path  string
	Kind  string // "mcp_json", "claude_json", "codex_toml", "skills_dir"
}

// DetectCandidates discovers available configuration files according to the options.
func DetectCandidates(opts ImportOptions) []DetectedCandidate {
	var candidates []DetectedCandidate

	// 1. Project-level detection
	if opts.ProjectPath != "" {
		// Cursor project MCP
		cursorProjMCP := filepath.Join(opts.ProjectPath, ".cursor", "mcp.json")
		if fileExists(cursorProjMCP) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentCursor,
				Scope: "project",
				Path:  cursorProjMCP,
				Kind:  "mcp_json",
			})
		}

		// Cursor project skills
		cursorProjSkills := filepath.Join(opts.ProjectPath, ".cursor", "skills")
		if dirExists(cursorProjSkills) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentCursor,
				Scope: "project",
				Path:  cursorProjSkills,
				Kind:  "skills_dir",
			})
		}

		// Claude Code / Standard project MCP
		standardProjMCP := filepath.Join(opts.ProjectPath, ".mcp.json")
		if fileExists(standardProjMCP) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentClaudeCode,
				Scope: "project",
				Path:  standardProjMCP,
				Kind:  "mcp_json",
			})
		}

		// Claude Code project skills
		claudeProjSkills := filepath.Join(opts.ProjectPath, ".claude", "skills")
		if dirExists(claudeProjSkills) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentClaudeCode,
				Scope: "project",
				Path:  claudeProjSkills,
				Kind:  "skills_dir",
			})
		}

		// OpenAI Codex project config
		codexProjConfig := filepath.Join(opts.ProjectPath, ".codex", "config.toml")
		if fileExists(codexProjConfig) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentCodex,
				Scope: "project",
				Path:  codexProjConfig,
				Kind:  "codex_toml",
			})
		}

		// Codex project skills
		codexProjSkills := filepath.Join(opts.ProjectPath, ".codex", "skills")
		if dirExists(codexProjSkills) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentCodex,
				Scope: "project",
				Path:  codexProjSkills,
				Kind:  "skills_dir",
			})
		}

		// Universal project skills (.agents/skills)
		universalSkills := filepath.Join(opts.ProjectPath, ".agents", "skills")
		if dirExists(universalSkills) {
			candidates = append(candidates, DetectedCandidate{
				Agent: "universal",
				Scope: "project",
				Path:  universalSkills,
				Kind:  "skills_dir",
			})
		}
	}

	// 2. Global-level detection (unless ProjectOnly is requested)
	if !opts.ProjectOnly && opts.HomeDir != "" {
		// Cursor global MCP
		cursorGlobalMCP := filepath.Join(opts.HomeDir, ".cursor", "mcp.json")
		if fileExists(cursorGlobalMCP) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentCursor,
				Scope: "global",
				Path:  cursorGlobalMCP,
				Kind:  "mcp_json",
			})
		}

		// Cursor global skills
		cursorGlobalSkills := filepath.Join(opts.HomeDir, ".cursor", "skills")
		if dirExists(cursorGlobalSkills) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentCursor,
				Scope: "global",
				Path:  cursorGlobalSkills,
				Kind:  "skills_dir",
			})
		}

		// Claude Code global config (~/.claude.json)
		claudeGlobalConfig := filepath.Join(opts.HomeDir, ".claude.json")
		if fileExists(claudeGlobalConfig) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentClaudeCode,
				Scope: "global",
				Path:  claudeGlobalConfig,
				Kind:  "claude_json",
			})
		}

		// Claude Code global settings (~/.claude/settings.json)
		claudeGlobalSettings := filepath.Join(opts.HomeDir, ".claude", "settings.json")
		if fileExists(claudeGlobalSettings) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentClaudeCode,
				Scope: "global",
				Path:  claudeGlobalSettings,
				Kind:  "mcp_json",
			})
		}

		// Claude Code global skills
		claudeGlobalSkills := filepath.Join(opts.HomeDir, ".claude", "skills")
		if dirExists(claudeGlobalSkills) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentClaudeCode,
				Scope: "global",
				Path:  claudeGlobalSkills,
				Kind:  "skills_dir",
			})
		}

		// OpenAI Codex global config (~/.codex/config.toml)
		codexGlobalConfig := filepath.Join(opts.HomeDir, ".codex", "config.toml")
		if fileExists(codexGlobalConfig) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentCodex,
				Scope: "global",
				Path:  codexGlobalConfig,
				Kind:  "codex_toml",
			})
		}

		// Codex global skills
		codexGlobalSkills := filepath.Join(opts.HomeDir, ".codex", "skills")
		if dirExists(codexGlobalSkills) {
			candidates = append(candidates, DetectedCandidate{
				Agent: types.AgentCodex,
				Scope: "global",
				Path:  codexGlobalSkills,
				Kind:  "skills_dir",
			})
		}
	}

	return candidates
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.IsDir()
}
