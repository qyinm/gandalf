package setup

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// MarketplaceActionKind identifies an agent-native marketplace/source action.
type MarketplaceActionKind string

const (
	MarketplaceActionInstall      MarketplaceActionKind = "install"
	MarketplaceActionUpdate       MarketplaceActionKind = "update"
	MarketplaceActionUninstall    MarketplaceActionKind = "uninstall"
	MarketplaceActionAddSource    MarketplaceActionKind = "add_source"
	MarketplaceActionRemoveSource MarketplaceActionKind = "remove_source"
)

// MarketplaceActionAvailability describes whether a marketplace action can run.
type MarketplaceActionAvailability struct {
	Action    MarketplaceActionKind
	Available bool
	Reason    string
}

// MarketplaceSourceKind describes the agent-native structure behind a source.
type MarketplaceSourceKind string

const (
	MarketplaceSourceMarketplace MarketplaceSourceKind = "marketplace"
	MarketplaceSourceCatalog     MarketplaceSourceKind = "catalog"
	MarketplaceSourceGit         MarketplaceSourceKind = "git_marketplace"
	MarketplaceSourcePlugin      MarketplaceSourceKind = "plugin_source"
	MarketplaceSourceExtension   MarketplaceSourceKind = "extension_source"
	MarketplaceSourcePackage     MarketplaceSourceKind = "package_source"
	MarketplaceSourceSkill       MarketplaceSourceKind = "skill_source"
	MarketplaceSourceUnknown     MarketplaceSourceKind = "source"
)

// MarketplaceSource groups entries from one observed agent ecosystem source.
type MarketplaceSource struct {
	ID      string
	Label   string
	Kind    MarketplaceSourceKind
	Agent   types.AgentID
	Scope   types.EvidenceScope
	Path    string
	Actions []MarketplaceActionAvailability
	Entries []MarketplaceEntry
}

// MarketplaceEntry is one source-backed plugin/extension/capability row.
type MarketplaceEntry struct {
	ID          string
	SourceID    string
	SourceKind  MarketplaceSourceKind
	Agent       types.AgentID
	Name        string
	Kind        types.EvidenceKind
	SourcePath  string
	Installed   bool
	Status      string
	Description string
	Author      string
	Category    string
	Version     string
	Provides    []string
	Actions     []MarketplaceActionAvailability
}

// BuildMarketplace derives agent ecosystem source rows from observed setup evidence.
func BuildMarketplace(evidence []types.DiscoveredItem) []MarketplaceSource {
	sourcesByID := make(map[string]*MarketplaceSource)
	for _, item := range evidence {
		if !isMarketplaceEvidence(item) {
			continue
		}
		meta := metadataMap(item.Metadata)
		sourceID, sourceLabel, sourcePath := marketplaceSourceIdentity(item, meta)
		sourceKind := marketplaceSourceKind(item, meta, sourcePath)
		source, ok := sourcesByID[sourceID]
		if !ok {
			source = &MarketplaceSource{
				ID:      sourceID,
				Label:   sourceLabel,
				Kind:    sourceKind,
				Agent:   item.Agent,
				Scope:   item.Scope,
				Path:    sourcePath,
				Actions: defaultMarketplaceSourceActions(),
			}
			sourcesByID[sourceID] = source
		}
		if metadataBool(meta, "sourceOnly") {
			continue
		}
		source.Entries = append(source.Entries, marketplaceEntryFromEvidence(item, sourceID, source.Kind, meta))
	}

	sources := make([]MarketplaceSource, 0, len(sourcesByID))
	for _, source := range sourcesByID {
		sort.SliceStable(source.Entries, func(i, j int) bool {
			return strings.ToLower(source.Entries[i].Name) < strings.ToLower(source.Entries[j].Name)
		})
		sources = append(sources, *source)
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Agent != sources[j].Agent {
			return sources[i].Agent < sources[j].Agent
		}
		return strings.ToLower(sources[i].Label) < strings.ToLower(sources[j].Label)
	})
	return sources
}

func isMarketplaceEvidence(item types.DiscoveredItem) bool {
	if item.Agent == types.AgentProject || item.Scope == types.ScopeProject {
		return false
	}
	if item.Kind == types.KindExtension {
		return true
	}
	meta := metadataMap(item.Metadata)
	source := metadataString(meta, "source")
	if source == "plugin" || metadataString(meta, "sourceRoot") != "" {
		return true
	}
	return inferredMarketplaceSourceRoot(item) != ""
}

func marketplaceSourceIdentity(item types.DiscoveredItem, meta map[string]any) (id, label, path string) {
	sourceKind := metadataString(meta, "source")
	sourceRoot := metadataString(meta, "sourceRoot")
	if sourceRoot == "" {
		sourceRoot = inferredMarketplaceSourceRoot(item)
	}
	label = firstNonEmpty(
		metadataString(meta, "marketplaceSource"),
		metadataString(meta, "sourceName"),
		sourceLabelFromPath(sourceRoot),
		sourceKind,
	)
	path = firstNonEmpty(sourceRoot, item.SourcePath)
	if label == "" {
		label = sourceLabelFromPath(path)
	}
	if label == "" {
		label = item.Agent.String()
	}
	id = strings.Join([]string{item.Agent.String(), string(item.Scope), sourceKind, label, path}, ":")
	return id, label, path
}

func marketplaceEntryFromEvidence(item types.DiscoveredItem, sourceID string, sourceKind MarketplaceSourceKind, meta map[string]any) MarketplaceEntry {
	name := marketplaceEntryName(item, meta)
	installed := metadataBoolDefault(meta, "installed", true)
	status := metadataString(meta, "status")
	if status == "" {
		if installed {
			status = "installed"
		} else {
			status = "available"
		}
	}
	return MarketplaceEntry{
		ID:          item.ID,
		SourceID:    sourceID,
		SourceKind:  sourceKind,
		Agent:       item.Agent,
		Name:        name,
		Kind:        item.Kind,
		SourcePath:  item.SourcePath,
		Installed:   installed,
		Status:      status,
		Description: metadataString(meta, "description"),
		Author:      firstNonEmpty(metadataString(meta, "author"), metadataString(meta, "publisher")),
		Category:    metadataString(meta, "category"),
		Version:     metadataString(meta, "version"),
		Provides:    marketplaceProvides(item, meta),
		Actions:     defaultMarketplaceEntryActions(),
	}
}

func marketplaceEntryName(item types.DiscoveredItem, meta map[string]any) string {
	if item.Name != nil && strings.TrimSpace(*item.Name) != "" {
		return strings.TrimSpace(*item.Name)
	}
	if name := metadataString(meta, "name"); name != "" {
		return name
	}
	if path := strings.TrimSpace(item.SourcePath); path != "" {
		return sourceLabelFromPath(path)
	}
	return item.Kind.String()
}

func marketplaceProvides(item types.DiscoveredItem, meta map[string]any) []string {
	if values := metadataStringArray(meta, "provides"); len(values) > 0 {
		return values
	}
	if values := metadataStringArray(meta, "capabilities"); len(values) > 0 {
		return values
	}
	return []string{item.Kind.String()}
}

func metadataBool(meta map[string]any, key string) bool {
	if meta == nil {
		return false
	}
	value, ok := meta[key]
	if !ok {
		return false
	}
	boolValue, ok := value.(bool)
	return ok && boolValue
}

func metadataBoolDefault(meta map[string]any, key string, defaultValue bool) bool {
	if meta == nil {
		return defaultValue
	}
	value, ok := meta[key]
	if !ok {
		return defaultValue
	}
	boolValue, ok := value.(bool)
	if !ok {
		return defaultValue
	}
	return boolValue
}

func marketplaceSourceKind(item types.DiscoveredItem, meta map[string]any, path string) MarketplaceSourceKind {
	if explicit := metadataString(meta, "sourceKind"); explicit != "" {
		return MarketplaceSourceKind(explicit)
	}
	source := metadataString(meta, "source")
	combinedPath := strings.ToLower(path + " " + item.SourcePath)
	switch item.Agent {
	case types.AgentClaudeCode:
		return MarketplaceSourceMarketplace
	case types.AgentCodex:
		switch {
		case strings.Contains(combinedPath, "remote_plugin_catalog"):
			return MarketplaceSourceCatalog
		case strings.Contains(combinedPath, ".tmp/marketplaces"):
			return MarketplaceSourceGit
		case strings.Contains(combinedPath, ".codex/plugins/cache"):
			return MarketplaceSourcePlugin
		default:
			return MarketplaceSourceSkill
		}
	case types.AgentOpencode:
		if strings.Contains(combinedPath, "plugin") || strings.Contains(combinedPath, "package") {
			return MarketplaceSourcePlugin
		}
		return MarketplaceSourceSkill
	case types.AgentPiAgent:
		if item.Kind == types.KindExtension {
			return MarketplaceSourceExtension
		}
		if source == "package" {
			return MarketplaceSourcePackage
		}
		return MarketplaceSourceSkill
	default:
		if source == "package" {
			return MarketplaceSourcePackage
		}
		if source == "plugin" {
			return MarketplaceSourcePlugin
		}
	}
	return MarketplaceSourceUnknown
}

func inferredMarketplaceSourceRoot(item types.DiscoveredItem) string {
	sourcePath := strings.TrimSpace(item.SourcePath)
	if sourcePath == "" {
		return ""
	}
	switch item.Agent {
	case types.AgentCodex:
		root := sourceRootBefore(sourcePath, []string{
			"/skills/",
			"/hooks/",
		}, []string{
			".codex/plugins/cache/",
			".codex/.tmp/marketplaces/",
			".codex/cache/remote_plugin_catalog/",
		})
		if strings.Contains(root, ".codex/plugins/cache/") {
			return stripLikelyVersionSegment(root)
		}
		return root
	case types.AgentOpencode:
		return sourceRootBefore(sourcePath, []string{
			"/skills/",
			"/skill/",
			"/plugins/",
		}, []string{
			".config/opencode/plugins/",
			".cache/opencode/packages/",
		})
	}
	return ""
}

func stripLikelyVersionSegment(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return path
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return path
	}
	last := parts[len(parts)-1]
	if strings.Contains(last, ".") || len(last) >= 7 && isHexLike(last) {
		prefix := ""
		if strings.HasPrefix(path, "/") {
			prefix = "/"
		}
		return prefix + strings.Join(parts[:len(parts)-1], "/")
	}
	return path
}

func isHexLike(value string) bool {
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func sourceRootBefore(sourcePath string, separators []string, requiredFragments []string) string {
	for _, fragment := range requiredFragments {
		if !strings.Contains(sourcePath, fragment) {
			continue
		}
		for _, separator := range separators {
			if index := strings.Index(sourcePath, separator); index > 0 {
				return sourcePath[:index]
			}
		}
		return sourcePath
	}
	return ""
}

func defaultMarketplaceEntryActions() []MarketplaceActionAvailability {
	reason := "agent-native marketplace action provider is not implemented yet"
	return []MarketplaceActionAvailability{
		{Action: MarketplaceActionInstall, Available: false, Reason: reason},
		{Action: MarketplaceActionUpdate, Available: false, Reason: reason},
		{Action: MarketplaceActionUninstall, Available: false, Reason: reason},
	}
}

func defaultMarketplaceSourceActions() []MarketplaceActionAvailability {
	reason := "agent-native source management provider is not implemented yet"
	return []MarketplaceActionAvailability{
		{Action: MarketplaceActionAddSource, Available: false, Reason: reason},
		{Action: MarketplaceActionRemoveSource, Available: false, Reason: reason},
	}
}

func metadataMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}
	return meta
}

func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	stringValue, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringValue)
}

func metadataStringArray(meta map[string]any, key string) []string {
	if meta == nil {
		return nil
	}
	value, ok := meta[key]
	if !ok {
		return nil
	}
	rawValues, ok := value.([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(rawValues))
	for _, raw := range rawValues {
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			values = append(values, strings.TrimSpace(value))
		}
	}
	return values
}

func sourceLabelFromPath(path string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
