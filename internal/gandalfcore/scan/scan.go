package scan

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
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
	return nil
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
			if ScopeEnabled(target.Scope, options.Scope) {
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
		if !ScopeEnabled(item.Scope, options.Scope) {
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

func ScopeEnabled(scope types.EvidenceScope, requested *types.EvidenceScope) bool {
	if requested != nil {
		return scope == *requested
	}
	return scope != types.ScopeProject
}
