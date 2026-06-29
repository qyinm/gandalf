package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ClaudeCodeScanner discovers Claude Code configuration targets.
type ClaudeCodeScanner struct{}

func (ClaudeCodeScanner) AgentID() types.AgentID { return types.AgentClaudeCode }
func (ClaudeCodeScanner) AgentName() string      { return "Claude Code" }
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

func (c ClaudeCodeScanner) Scan(context *scan.ScannerContext) []types.DiscoveredItem {
	var targets []scan.ScanTarget
	for _, target := range c.Targets(context.ProjectPath, context.HomeDir) {
		if scan.ScopeEnabled(target.Scope, context.Scope) {
			targets = append(targets, target)
		}
	}
	evidence := scan.ScanTargets(targets)
	if context.Scope == nil || *context.Scope == types.ScopeUser {
		evidence = append(evidence, scanClaudeInstalledPlugins(context.HomeDir)...)
		evidence = append(evidence, scanClaudePluginMarketplaces(context.HomeDir)...)
	}
	return evidence
}

type claudeInstalledPlugin struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
	Version     string `json:"version"`
	LastUpdated string `json:"lastUpdated"`
}

// scanClaudeInstalledPlugins surfaces installed Claude Code plugins from
// installed_plugins.json as plugin inventory (distinct from marketplace
// catalog entries) so they appear in the Plugins tab.
func scanClaudeInstalledPlugins(homeDir string) []types.DiscoveredItem {
	installedPath := filepath.Join(homeDir, ".claude", "plugins", "installed_plugins.json")
	raw, err := os.ReadFile(installedPath)
	if err != nil {
		return nil
	}
	var registry struct {
		Plugins map[string][]claudeInstalledPlugin `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &registry); err != nil {
		return nil
	}
	evidence := make([]types.DiscoveredItem, 0, len(registry.Plugins))
	for key, installs := range registry.Plugins {
		name, marketplace := splitPluginKey(key)
		if name == "" {
			continue
		}
		install := claudeInstalledPlugin{}
		if len(installs) > 0 {
			install = installs[0]
		}
		sourcePath := displayClaudePath(filepath.Join(homeDir, ".claude", "plugins", "cache", marketplace, name), homeDir)
		if install.InstallPath != "" {
			sourcePath = displayClaudePath(install.InstallPath, homeDir)
		}
		metadata, _ := json.Marshal(map[string]any{
			"present":       true,
			"name":          name,
			"marketplace":   marketplace,
			"version":       install.Version,
			"installed":     true,
			"status":        "installed",
			"inventoryOnly": true,
		})
		itemName := name
		evidence = append(evidence, types.DiscoveredItem{
			ID:            scan.ScannerItemID(types.ScopeUser, types.AgentClaudeCode, sourcePath, "plugin-"+marketplace+"-"+name),
			Agent:         types.AgentClaudeCode,
			Kind:          types.KindExtension,
			SourcePath:    sourcePath,
			Scope:         types.ScopeUser,
			Precedence:    20,
			Parser:        types.ParserJSON,
			Sensitivity:   "metadata",
			ContentPolicy: "metadata_only",
			RestorePolicy: types.RestoreNotSupported,
			CaptureStatus: types.CaptureCaptured,
			Confidence:    types.ConfidenceHigh,
			Name:          &itemName,
			Metadata:      metadata,
		})
	}
	return evidence
}

// splitPluginKey splits a "name@marketplace" plugin registry key.
func splitPluginKey(key string) (name, marketplace string) {
	key = strings.TrimSpace(key)
	if at := strings.LastIndex(key, "@"); at >= 0 {
		return key[:at], key[at+1:]
	}
	return key, ""
}

type claudeKnownMarketplace struct {
	Source          map[string]any `json:"source"`
	InstallLocation string         `json:"installLocation"`
	LastUpdated     string         `json:"lastUpdated"`
}

type claudeMarketplaceManifest struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Owner       claudeMarketplacePerson   `json:"owner"`
	Plugins     []claudeMarketplacePlugin `json:"plugins"`
}

type claudeMarketplacePerson struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type claudeMarketplacePlugin struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Version     string                  `json:"version"`
	Author      claudeMarketplacePerson `json:"author"`
	Source      string                  `json:"source"`
	Category    string                  `json:"category"`
}

func scanClaudePluginMarketplaces(homeDir string) []types.DiscoveredItem {
	knownPath := filepath.Join(homeDir, ".claude", "plugins", "known_marketplaces.json")
	rawKnown, err := os.ReadFile(knownPath)
	if err != nil {
		return nil
	}
	var known map[string]claudeKnownMarketplace
	if err := json.Unmarshal(rawKnown, &known); err != nil {
		return nil
	}
	installed := installedClaudePlugins(homeDir)
	evidence := make([]types.DiscoveredItem, 0, len(known))
	for name, source := range known {
		sourceRoot := source.InstallLocation
		if sourceRoot == "" {
			sourceRoot = filepath.Join(homeDir, ".claude", "plugins", "marketplaces", name)
		}
		displayRoot := displayClaudePath(sourceRoot, homeDir)
		sourceMetadata := map[string]any{
			"source":            "marketplace",
			"sourceKind":        "marketplace",
			"sourceOnly":        true,
			"marketplaceSource": name,
			"sourceName":        name,
			"sourceRoot":        displayRoot,
			"description":       claudeMarketplaceSourceDescription(source.Source),
			"lastUpdated":       source.LastUpdated,
		}
		evidence = append(evidence, claudeMarketplaceItem(
			"marketplace-source-"+name,
			name,
			displayRoot,
			sourceMetadata,
		))

		manifest := readClaudeMarketplaceManifest(sourceRoot)
		for _, plugin := range manifest.Plugins {
			if strings.TrimSpace(plugin.Name) == "" {
				continue
			}
			pluginSourcePath := displayRoot
			if plugin.Source != "" {
				pluginSourcePath = displayClaudePath(filepath.Join(sourceRoot, plugin.Source), homeDir)
			}
			installedKey := plugin.Name + "@" + name
			pluginInstalled := installed[installedKey]
			metadata := map[string]any{
				"source":            "marketplace",
				"sourceKind":        "marketplace",
				"marketplaceSource": name,
				"sourceName":        name,
				"sourceRoot":        displayRoot,
				"name":              plugin.Name,
				"description":       plugin.Description,
				"author":            plugin.Author.Name,
				"category":          plugin.Category,
				"version":           plugin.Version,
				"provides":          []string{"plugin"},
				"installed":         pluginInstalled,
				"status":            claudeMarketplacePluginStatus(pluginInstalled),
			}
			evidence = append(evidence, claudeMarketplaceItem(
				"marketplace-plugin-"+name+"-"+plugin.Name,
				plugin.Name,
				pluginSourcePath,
				metadata,
			))
		}
	}
	return evidence
}

func readClaudeMarketplaceManifest(sourceRoot string) claudeMarketplaceManifest {
	manifestPath := filepath.Join(sourceRoot, ".claude-plugin", "marketplace.json")
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return claudeMarketplaceManifest{}
	}
	var manifest claudeMarketplaceManifest
	_ = json.Unmarshal(rawManifest, &manifest)
	return manifest
}

func installedClaudePlugins(homeDir string) map[string]bool {
	installedPath := filepath.Join(homeDir, ".claude", "plugins", "installed_plugins.json")
	rawInstalled, err := os.ReadFile(installedPath)
	if err != nil {
		return nil
	}
	var registry struct {
		Plugins map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(rawInstalled, &registry); err != nil {
		return nil
	}
	installed := make(map[string]bool, len(registry.Plugins))
	for key := range registry.Plugins {
		installed[key] = true
	}
	return installed
}

func claudeMarketplaceItem(idSuffix, name, sourcePath string, metadata map[string]any) types.DiscoveredItem {
	rawMetadata, _ := json.Marshal(metadata)
	itemName := name
	return types.DiscoveredItem{
		ID:            scan.ScannerItemID(types.ScopeUser, types.AgentClaudeCode, sourcePath, idSuffix),
		Agent:         types.AgentClaudeCode,
		Kind:          types.KindUnsupported,
		SourcePath:    sourcePath,
		Scope:         types.ScopeUser,
		Precedence:    20,
		Parser:        types.ParserJSON,
		Sensitivity:   "metadata",
		ContentPolicy: "metadata_only",
		RestorePolicy: types.RestoreNotSupported,
		CaptureStatus: types.CaptureCaptured,
		Confidence:    types.ConfidenceHigh,
		Name:          &itemName,
		Metadata:      rawMetadata,
	}
}

func claudeMarketplacePluginStatus(installed bool) string {
	if installed {
		return "installed"
	}
	return "available"
}

func claudeMarketplaceSourceDescription(source map[string]any) string {
	if len(source) == 0 {
		return ""
	}
	if repo, ok := source["repo"].(string); ok && strings.TrimSpace(repo) != "" {
		return strings.TrimSpace(repo)
	}
	if value, ok := source["source"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func displayClaudePath(path string, homeDir string) string {
	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(homeDir)
	if cleanPath == cleanHome {
		return "~"
	}
	if rel, err := filepath.Rel(cleanHome, cleanPath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return filepath.ToSlash(cleanPath)
}
