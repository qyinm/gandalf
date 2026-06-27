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

// MarketplaceSource groups entries from one observed agent ecosystem source.
type MarketplaceSource struct {
	ID      string
	Label   string
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
		source, ok := sourcesByID[sourceID]
		if !ok {
			source = &MarketplaceSource{
				ID:      sourceID,
				Label:   sourceLabel,
				Agent:   item.Agent,
				Scope:   item.Scope,
				Path:    sourcePath,
				Actions: defaultMarketplaceSourceActions(),
			}
			sourcesByID[sourceID] = source
		}
		source.Entries = append(source.Entries, marketplaceEntryFromEvidence(item, sourceID, meta))
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
	return source == "plugin" || metadataString(meta, "sourceRoot") != ""
}

func marketplaceSourceIdentity(item types.DiscoveredItem, meta map[string]any) (id, label, path string) {
	label = firstNonEmpty(
		metadataString(meta, "marketplaceSource"),
		metadataString(meta, "sourceName"),
		metadataString(meta, "source"),
		metadataString(meta, "sourceRoot"),
	)
	path = firstNonEmpty(metadataString(meta, "sourceRoot"), item.SourcePath)
	if label == "" {
		label = sourceLabelFromPath(path)
	}
	if label == "" {
		label = item.Agent.String()
	}
	id = strings.Join([]string{item.Agent.String(), string(item.Scope), label, path}, ":")
	return id, label, path
}

func marketplaceEntryFromEvidence(item types.DiscoveredItem, sourceID string, meta map[string]any) MarketplaceEntry {
	name := marketplaceEntryName(item, meta)
	return MarketplaceEntry{
		ID:          item.ID,
		SourceID:    sourceID,
		Agent:       item.Agent,
		Name:        name,
		Kind:        item.Kind,
		SourcePath:  item.SourcePath,
		Installed:   true,
		Status:      "installed",
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
