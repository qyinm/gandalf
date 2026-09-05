package plugins

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// CursorCLIConfig discovers official Cursor Agent CLI configuration.
//
// Documented paths (https://cursor.com/docs/cli/reference/configuration):
//   - user:   ~/.cursor/cli-config.json
//   - project: .cursor/cli.json (permissions only)
type CursorCLIConfig struct{}

func (CursorCLIConfig) Targets(projectPath, homeDir string) []scan.ScanTarget {
	overrides := scan.ScanTargetOverrides{
		Sensitivity:   stringPtr("command_config"),
		ContentPolicy: stringPtr("structured_safe_fields_only"),
	}
	return []scan.ScanTarget{
		scan.ProjectTarget(projectPath, ".cursor/cli.json", types.AgentCursor, types.KindAgentConfig, types.ParserJSON, overrides),
		scan.HomeTarget(homeDir, ".cursor/cli-config.json", types.AgentCursor, types.KindAgentConfig, types.ParserJSON, overrides),
	}
}

func (c CursorCLIConfig) Scan(context *scan.ScannerContext) []types.DiscoveredItem {
	var targets []scan.ScanTarget
	for _, target := range c.Targets(context.ProjectPath, context.HomeDir) {
		if scan.ScopeEnabled(target.Scope, context.Scope) {
			targets = append(targets, target)
		}
	}
	return scan.ScanTargets(targets)
}
