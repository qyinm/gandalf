package scan

import (
	"github.com/qyinm/hem/internal/hemcore/types"
)

var defaultPluginFactory func() []ScannerPlugin

// SetDefaultPluginFactory registers the full default plugin list (called from plugins package init).
func SetDefaultPluginFactory(factory func() []ScannerPlugin) {
	defaultPluginFactory = factory
}

// DefaultScannerPlugins returns the configured scanner plugin list.
func DefaultScannerPlugins() []ScannerPlugin {
	if defaultPluginFactory != nil {
		return defaultPluginFactory()
	}
	return fallbackScannerPlugins()
}

func fallbackScannerPlugins() []ScannerPlugin {
	return []ScannerPlugin{
		ClaudeCodeScanner{},
		CursorScanner{},
		OpenCodeScanner{},
		PiAgentScanner{},
	}
}

// ScanProject performs a read-only project scan and returns discovered evidence.
func ScanProject(options *types.ScanOptions) types.ScanResult {
	projectPath := resolvePath(options.ProjectPath)
	homeDir := resolvePath(options.HomeDir)
	context := &ScannerContext{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    options.StoreDir,
		Explain:     options.Explain,
		Scope:       options.Scope,
	}

	var evidence []types.DiscoveredItem
	for _, plugin := range DefaultScannerPlugins() {
		if options.Agent != nil && plugin.AgentID() != *options.Agent {
			continue
		}

		if items := plugin.Scan(context); items != nil {
			evidence = append(evidence, items...)
			continue
		}

		targets := plugin.Targets(projectPath, homeDir)
		var filtered []ScanTarget
		for _, target := range targets {
			if options.Scope == nil || target.Scope == *options.Scope {
				filtered = append(filtered, target)
			}
		}
		evidence = append(evidence, ScanTargets(filtered)...)
	}

	var filteredEvidence []types.DiscoveredItem
	for _, item := range evidence {
		if options.Agent != nil && item.Agent != *options.Agent {
			continue
		}
		if options.Scope != nil && item.Scope != *options.Scope {
			continue
		}
		filteredEvidence = append(filteredEvidence, item)
	}

	return types.ScanResult{
		Trust: types.ScanTrust{
			ReadOnly:           true,
			Network:            "disabled",
			CommandsExecuted:   nil,
			StoreWriteLocation: options.StoreDir,
		},
		Evidence: filteredEvidence,
		BlindSpots: []string{
			"Remote MCP server behavior cannot be captured",
			"Provider-side model routing cannot be verified",
			"Raw env values are omitted by policy",
		},
	}
}

// ClaudeCodeScanner is a target-only stub until full Claude Code port (U8).
type ClaudeCodeScanner struct{}

func (ClaudeCodeScanner) AgentID() types.AgentID   { return types.AgentClaudeCode }
func (ClaudeCodeScanner) AgentName() string        { return "Claude Code" }
func (ClaudeCodeScanner) Description() string {
	return "Claude Code agent configuration (prompts, MCP servers, settings, skills)"
}
func (ClaudeCodeScanner) Scan(*ScannerContext) []types.DiscoveredItem { return nil }
func (ClaudeCodeScanner) Targets(projectPath, homeDir string) []ScanTarget {
	metadataOnly := true
	return []ScanTarget{
		ProjectTarget(projectPath, "CLAUDE.md", types.AgentClaudeCode, types.KindAgentInstruction, types.ParserMarkdown, ScanTargetOverrides{}),
		ProjectTarget(projectPath, ".mcp.json", types.AgentClaudeCode, types.KindAgentConfig, types.ParserJSON, ScanTargetOverrides{}),
		ProjectTarget(projectPath, ".claude/settings.json", types.AgentClaudeCode, types.KindAgentConfig, types.ParserJSON, ScanTargetOverrides{}),
		HomeTarget(homeDir, ".claude/settings.json", types.AgentClaudeCode, types.KindAgentConfig, types.ParserJSON, ScanTargetOverrides{}),
		HomeTarget(homeDir, ".claude.json", types.AgentClaudeCode, types.KindAgentConfig, types.ParserJSON, ScanTargetOverrides{
			MetadataOnly: &metadataOnly,
		}),
		HomeTarget(homeDir, ".claude/agents", types.AgentClaudeCode, types.KindUnsupported, types.ParserFilesystem, ScanTargetOverrides{
			Directory: boolPtr(true),
		}),
		HomeTarget(homeDir, ".claude/skills", types.AgentClaudeCode, types.KindSkill, types.ParserFilesystem, ScanTargetOverrides{
			Directory: boolPtr(true),
		}),
	}
}

// CursorScanner is a target-only stub until full Cursor port (U8).
type CursorScanner struct{}

func (CursorScanner) AgentID() types.AgentID   { return types.AgentCursor }
func (CursorScanner) AgentName() string        { return "Cursor" }
func (CursorScanner) Description() string      { return "Cursor editor configuration (MCP servers, skills, hooks)" }
func (CursorScanner) Scan(*ScannerContext) []types.DiscoveredItem { return nil }
func (CursorScanner) Targets(projectPath, homeDir string) []ScanTarget {
	overrides := ScanTargetOverrides{
		Sensitivity:   stringPtr("command_config"),
		ContentPolicy: stringPtr("structured_safe_fields_only"),
	}
	return []ScanTarget{
		ProjectTarget(projectPath, ".cursor/mcp.json", types.AgentCursor, types.KindAgentConfig, types.ParserJSON, overrides),
		HomeTarget(homeDir, ".cursor/mcp.json", types.AgentCursor, types.KindAgentConfig, types.ParserJSON, overrides),
	}
}

// OpenCodeScanner is a target-only stub until full OpenCode port (U8).
type OpenCodeScanner struct{}

func (OpenCodeScanner) AgentID() types.AgentID   { return types.AgentOpencode }
func (OpenCodeScanner) AgentName() string        { return "OpenCode" }
func (OpenCodeScanner) Description() string {
	return "OpenCode CLI configuration (MCP servers, plugins, providers, skills)"
}
func (OpenCodeScanner) Scan(*ScannerContext) []types.DiscoveredItem { return nil }
func (OpenCodeScanner) Targets(_ string, homeDir string) []ScanTarget {
	return []ScanTarget{
		HomeTarget(homeDir, ".config/opencode/opencode.json", types.AgentOpencode, types.KindAgentConfig, types.ParserJSON, ScanTargetOverrides{}),
	}
}

// PiAgentScanner is a target-only stub until full Pi port (U8).
type PiAgentScanner struct{}

func (PiAgentScanner) AgentID() types.AgentID   { return types.AgentPiAgent }
func (PiAgentScanner) AgentName() string        { return "Pi Agent" }
func (PiAgentScanner) Description() string {
	return "Pi coding agent configuration (settings, models, agents, extensions, skills, themes, prompts)"
}
func (PiAgentScanner) Scan(*ScannerContext) []types.DiscoveredItem { return nil }
func (PiAgentScanner) Targets(projectPath, homeDir string) []ScanTarget {
	return []ScanTarget{
		ProjectTarget(projectPath, ".pi/settings.json", types.AgentPiAgent, types.KindAgentConfig, types.ParserJSON, ScanTargetOverrides{}),
		ProjectTarget(projectPath, ".pi/themes", types.AgentPiAgent, types.KindUnsupported, types.ParserFilesystem, ScanTargetOverrides{
			Directory:   boolPtr(true),
			Sensitivity: stringPtr("themes"),
		}),
		ProjectTarget(projectPath, ".pi/prompts", types.AgentPiAgent, types.KindAgentInstruction, types.ParserFilesystem, ScanTargetOverrides{
			Directory:   boolPtr(true),
			Sensitivity: stringPtr("prompt_templates"),
		}),
		HomeTarget(homeDir, ".pi/agent/settings.json", types.AgentPiAgent, types.KindAgentConfig, types.ParserJSON, ScanTargetOverrides{}),
	}
}

func boolPtr(value bool) *bool {
	return &value
}

