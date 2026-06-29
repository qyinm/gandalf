package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/baseline"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	timelineundo "github.com/qyinm/gandalf/internal/gandalfcore/timeline_undo"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// VisibleAgents is the stable product-visible sidebar agent order.
var VisibleAgents = agents.CurrentSupportedIDs()

// Screen identifies the active workspace panel.
type Screen string

const (
	ScreenInventory    Screen = "inventory"
	ScreenTimeline     Screen = "timeline"
	ScreenSnapshots    Screen = "snapshots"
	ScreenEnvironments Screen = "environments"
	ScreenAgentDetail  Screen = "agent-detail"
	ScreenProfile      Screen = "profile"
	ScreenCompare      Screen = "compare"
	ScreenSaveSetup    Screen = "save-setup"
)

const (
	DefaultProfile      = "default"
	InventoryNavItemID  = "inventory:global"
	HistoryAllNavItemID = "history:all"
	InitialNavItemID    = InventoryNavItemID
)

// --- Setup inventory view model ---

type SetupInventoryRowModel struct {
	ID          string
	AgentLabel  string
	AgentMarker string
	ObjectKind  string
	Name        string
	SourcePath  string
	ActionLabel string
	Selected    bool
}

type SetupInventoryViewModel struct {
	Rows         []SetupInventoryRowModel
	Skills       int
	McpServers   int
	Hooks        int
	Plugins      int
	EmptyMessage string
	Confirmation *SetupActionConfirmationModel
	ActionError  string
}

type SetupActionConfirmationModel struct {
	Action       string
	AgentLabel   string
	ObjectKind   string
	TargetName   string
	Operation    string
	ConfigTarget string
	Command      string
}

// SetupConsoleTab identifies the active top-level setup console tab.
type SetupConsoleTab string

const (
	SetupConsoleTabHooks       SetupConsoleTab = "hooks"
	SetupConsoleTabPlugins     SetupConsoleTab = "plugins"
	SetupConsoleTabMarketplace SetupConsoleTab = "marketplace"
	SetupConsoleTabSkills      SetupConsoleTab = "skills"
	SetupConsoleTabMCPServers  SetupConsoleTab = "mcp_servers"
)

// SetupConsoleTabs is the stable top-tab order.
var SetupConsoleTabs = []SetupConsoleTab{
	SetupConsoleTabHooks,
	SetupConsoleTabPlugins,
	SetupConsoleTabMarketplace,
	SetupConsoleTabSkills,
	SetupConsoleTabMCPServers,
}

type SetupConsoleTabModel struct {
	Tab      SetupConsoleTab
	Label    string
	Count    int
	Selected bool
}

type SetupConsoleRowModel struct {
	ID            string
	RowKind       SetupConsoleRowKind
	ParentID      string
	Depth         int
	Expanded      bool
	Toggleable    bool
	AgentLabel    string
	AgentMarker   string
	ObjectKind    string
	Name          string
	SourcePath    string
	Scope         string
	Status        string
	Entrypoint    string
	EntryStatus   string
	RuntimeStatus string
	Tools         []SetupConsoleToolModel
	ToolCount     int
	Description   string
	ActionLabel   string
	ToggleControl bool
	Disabled      bool
	Selected      bool
}

type SetupConsoleRowKind string

const (
	SetupConsoleRowInventory         SetupConsoleRowKind = "inventory"
	SetupConsoleRowMarketplaceSource SetupConsoleRowKind = "marketplace_source"
	SetupConsoleRowMarketplaceEntry  SetupConsoleRowKind = "marketplace_entry"
	SetupConsoleRowMCPTool           SetupConsoleRowKind = "mcp_tool"
)

type SetupConsoleToolModel struct {
	Name        string
	Description string
}

type SetupConsoleDetailModel struct {
	Title        string
	AgentLabel   string
	ObjectKind   string
	SourcePath   string
	Scope        string
	Status       string
	Entrypoint   string
	EntryStatus  string
	Description  string
	Author       string
	Category     string
	Version      string
	Provides     []string
	Actions      []SetupConsoleActionModel
	ConfigTarget string
}

type SetupConsoleActionModel struct {
	Label     string
	Available bool
	Reason    string
}

type SetupConsoleViewModel struct {
	ActiveTab     SetupConsoleTab
	Tabs          []SetupConsoleTabModel
	Rows          []SetupConsoleRowModel
	Baseline      *BaselineStatusViewModel
	RowOffset     int
	Search        string
	SearchInput   string
	SearchFocused bool
	EmptyMessage  string
	Selected      *SetupConsoleDetailModel
	Confirmation  *SetupActionConfirmationModel
	ActionError   string
}

type BuildSetupConsoleViewModelInput struct {
	Inventory          []setup.InventoryItem
	MarketplaceSources []setup.MarketplaceSource
	ActiveTab          SetupConsoleTab
	Search             string
	SearchInput        string
	SearchFocused      bool
	SelectedIndex      int
	ExpandedSources    map[string]bool
	ExpandedRowID      string
	ExpandedToolID     string
	PendingAction      *setup.ActionPlan
	ActionError        string
	BaselineStatus     *baseline.Status
}

func BuildSetupConsoleViewModel(input BuildSetupConsoleViewModelInput) SetupConsoleViewModel {
	activeTab := normalizeSetupConsoleTab(input.ActiveTab)
	marketplaceSources := setupConsoleMarketplaceSources(input.MarketplaceSources)
	counts := setupConsoleTabCounts(input.Inventory)
	counts[SetupConsoleTabMarketplace] = len(marketplaceSources)

	model := SetupConsoleViewModel{
		ActiveTab:     activeTab,
		Tabs:          buildSetupConsoleTabs(activeTab, counts),
		Search:        strings.TrimSpace(input.Search),
		SearchInput:   input.SearchInput,
		SearchFocused: input.SearchFocused,
		ActionError:   input.ActionError,
	}
	if input.BaselineStatus != nil {
		baselineModel := BuildBaselineStatusViewModel(*input.BaselineStatus)
		model.Baseline = &baselineModel
	}

	if activeTab == SetupConsoleTabMarketplace {
		rows, details := setupConsoleMarketplaceRows(marketplaceSources, input.Search, input.ExpandedSources)
		selectedIndex := clampIndex(input.SelectedIndex, len(rows))
		model.Rows = make([]SetupConsoleRowModel, 0, len(rows))
		for i := range rows {
			rows[i].Selected = i == selectedIndex
			model.Rows = append(model.Rows, rows[i])
		}
		if len(model.Rows) == 0 {
			model.EmptyMessage = setupConsoleEmptyMessage(activeTab, input.Search)
		} else {
			selected := details[selectedIndex]
			model.Selected = &selected
		}
	} else {
		filtered := filterSetupConsoleInventory(input.Inventory, activeTab, input.Search)
		model.Rows = make([]SetupConsoleRowModel, 0, len(filtered))
		query := strings.ToLower(strings.TrimSpace(input.Search))
		for _, item := range filtered {
			row := setupConsoleRowFromInventory(item, false)
			if activeTab != SetupConsoleTabMarketplace {
				row.Toggleable = true
				row.Expanded = item.ID == input.ExpandedRowID || (activeTab == SetupConsoleTabMCPServers && query != "")
			}
			model.Rows = append(model.Rows, row)
			if activeTab == SetupConsoleTabMCPServers && row.Expanded {
				model.Rows = append(model.Rows, setupConsoleMCPToolRows(item, input.ExpandedToolID, query)...)
			}
		}
		selectedIndex := clampIndex(input.SelectedIndex, len(model.Rows))
		for i := range model.Rows {
			model.Rows[i].Selected = i == selectedIndex
			if activeTab != SetupConsoleTabMarketplace && activeTab != SetupConsoleTabMCPServers && i != selectedIndex {
				model.Rows[i].Expanded = false
			}
		}
		if len(model.Rows) == 0 {
			model.EmptyMessage = setupConsoleEmptyMessage(activeTab, input.Search)
		} else {
			if selected, ok := setupConsoleSelectedDetail(model.Rows[selectedIndex], filtered); ok {
				model.Selected = &selected
			}
		}
	}
	if input.PendingAction != nil {
		model.Confirmation = buildSetupActionConfirmation(*input.PendingAction)
	}
	return model
}

func setupConsoleMarketplaceSources(sources []setup.MarketplaceSource) []setup.MarketplaceSource {
	filtered := make([]setup.MarketplaceSource, 0, len(sources))
	for _, source := range sources {
		if source.Kind == setup.MarketplaceSourceMarketplace {
			filtered = append(filtered, source)
		}
	}
	return filtered
}

func setupConsoleMarketplaceRows(sources []setup.MarketplaceSource, search string, expandedSources map[string]bool) ([]SetupConsoleRowModel, []SetupConsoleDetailModel) {
	query := strings.ToLower(strings.TrimSpace(search))
	var rows []SetupConsoleRowModel
	var details []SetupConsoleDetailModel
	for _, source := range sources {
		sourceMatches := query == "" || marketplaceSourceMatches(source, query)
		matchingEntries := make([]setup.MarketplaceEntry, 0, len(source.Entries))
		for _, entry := range source.Entries {
			if query == "" || sourceMatches || marketplaceEntryMatches(entry, source, query) {
				matchingEntries = append(matchingEntries, entry)
			}
		}
		if query != "" && !sourceMatches && len(matchingEntries) == 0 {
			continue
		}
		expanded := query != "" || expandedSources[source.ID]
		rows = append(rows, setupConsoleRowFromMarketplaceSource(source, expanded))
		details = append(details, setupConsoleDetailFromMarketplaceSource(source))
		if expanded {
			for _, entry := range matchingEntries {
				rows = append(rows, setupConsoleRowFromMarketplaceEntry(entry))
				details = append(details, setupConsoleDetailFromMarketplaceEntry(entry))
			}
		}
	}
	return rows, details
}

func marketplaceSourceMatches(source setup.MarketplaceSource, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		source.ID,
		source.Label,
		source.Path,
		marketplaceSourceKindLabel(source.Kind),
		string(source.Agent),
		string(source.Scope),
	}, " "))
	return strings.Contains(haystack, query)
}

func marketplaceEntryMatches(entry setup.MarketplaceEntry, source setup.MarketplaceSource, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		entry.ID,
		entry.Name,
		entry.SourcePath,
		entry.Description,
		entry.Author,
		entry.Category,
		entry.Version,
		entry.Status,
		marketplaceSourceKindLabel(entry.SourceKind),
		marketplaceEntryKindLabel(entry),
		source.Label,
		string(entry.Agent),
		string(entry.Kind),
		strings.Join(entry.Provides, " "),
	}, " "))
	return strings.Contains(haystack, query)
}

func normalizeSetupConsoleTab(tab SetupConsoleTab) SetupConsoleTab {
	for _, known := range SetupConsoleTabs {
		if tab == known {
			return tab
		}
	}
	return SetupConsoleTabHooks
}

func buildSetupConsoleTabs(active SetupConsoleTab, counts map[SetupConsoleTab]int) []SetupConsoleTabModel {
	tabs := make([]SetupConsoleTabModel, 0, len(SetupConsoleTabs))
	for _, tab := range SetupConsoleTabs {
		tabs = append(tabs, SetupConsoleTabModel{
			Tab:      tab,
			Label:    setupConsoleTabLabel(tab),
			Count:    counts[tab],
			Selected: tab == active,
		})
	}
	return tabs
}

func setupConsoleTabCounts(inventory []setup.InventoryItem) map[SetupConsoleTab]int {
	counts := make(map[SetupConsoleTab]int)
	for _, item := range inventory {
		if tab, ok := setupConsoleTabForObjectKind(item.ObjectKind); ok {
			counts[tab]++
		}
	}
	return counts
}

func filterSetupConsoleInventory(inventory []setup.InventoryItem, tab SetupConsoleTab, search string) []setup.InventoryItem {
	objectKind, ok := setupObjectKindForConsoleTab(tab)
	if !ok {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(search))
	filtered := make([]setup.InventoryItem, 0, len(inventory))
	for _, item := range inventory {
		if item.ObjectKind != objectKind {
			continue
		}
		if query != "" && !setupInventoryMatchesSearch(item, query) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func setupInventoryMatchesSearch(item setup.InventoryItem, query string) bool {
	if setupInventorySelfMatchesSearch(item, query) {
		return true
	}
	for _, tool := range item.Tools {
		if setupInventoryToolMatchesSearch(tool, query) {
			return true
		}
	}
	return false
}

func setupInventorySelfMatchesSearch(item setup.InventoryItem, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		item.ID,
		item.Name,
		item.SourcePath,
		string(item.Scope),
		string(item.Agent),
		formatSetupObjectKind(item.ObjectKind),
		item.RuntimeStatus,
	}, " "))
	return strings.Contains(haystack, query)
}

func setupInventoryToolMatchesSearch(tool setup.InventoryTool, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		tool.Name,
		tool.Description,
	}, " "))
	return strings.Contains(haystack, query)
}

func setupConsoleTabForObjectKind(kind setup.ObjectKind) (SetupConsoleTab, bool) {
	switch kind {
	case setup.ObjectHook:
		return SetupConsoleTabHooks, true
	case setup.ObjectPlugin:
		return SetupConsoleTabPlugins, true
	case setup.ObjectSkill:
		return SetupConsoleTabSkills, true
	case setup.ObjectMCPServer:
		return SetupConsoleTabMCPServers, true
	default:
		return "", false
	}
}

func setupObjectKindForConsoleTab(tab SetupConsoleTab) (setup.ObjectKind, bool) {
	switch tab {
	case SetupConsoleTabHooks:
		return setup.ObjectHook, true
	case SetupConsoleTabPlugins:
		return setup.ObjectPlugin, true
	case SetupConsoleTabSkills:
		return setup.ObjectSkill, true
	case SetupConsoleTabMCPServers:
		return setup.ObjectMCPServer, true
	default:
		return "", false
	}
}

func setupConsoleRowFromInventory(item setup.InventoryItem, selected bool) SetupConsoleRowModel {
	return SetupConsoleRowModel{
		RowKind:       SetupConsoleRowInventory,
		ID:            item.ID,
		AgentLabel:    FormatAgentLabel(item.Agent),
		AgentMarker:   FormatAgentMarker(item.Agent),
		ObjectKind:    formatInventoryObjectKind(item),
		Name:          item.Name,
		SourcePath:    item.SourcePath,
		Scope:         string(item.Scope),
		Status:        setupInventoryStatus(item),
		Entrypoint:    item.Entrypoint,
		EntryStatus:   item.EntryStatus,
		RuntimeStatus: item.RuntimeStatus,
		Tools:         setupConsoleToolsFromInventory(item.Tools),
		ToolCount:     item.ToolCount,
		ActionLabel:   formatSetupActions(item.Actions),
		ToggleControl: inventoryActionAvailable(item, setup.ActionToggle),
		Disabled:      item.Disabled,
		Selected:      selected,
	}
}

func inventoryActionAvailable(item setup.InventoryItem, action setup.ActionKind) bool {
	for _, availability := range item.Actions {
		if availability.Action == action {
			return availability.Available
		}
	}
	return false
}

func setupConsoleMCPToolRows(item setup.InventoryItem, expandedToolID string, search string) []SetupConsoleRowModel {
	query := strings.ToLower(strings.TrimSpace(search))
	rows := make([]SetupConsoleRowModel, 0, len(item.Tools))
	for _, tool := range item.Tools {
		if query != "" && !setupInventoryToolMatchesSearch(tool, query) {
			continue
		}
		id := item.ID + ":tool:" + tool.Name
		rows = append(rows, SetupConsoleRowModel{
			ID:          id,
			RowKind:     SetupConsoleRowMCPTool,
			ParentID:    item.ID,
			Depth:       1,
			Toggleable:  true,
			Expanded:    id == expandedToolID,
			ObjectKind:  "tool",
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	return rows
}

func setupConsoleSelectedDetail(row SetupConsoleRowModel, inventory []setup.InventoryItem) (SetupConsoleDetailModel, bool) {
	id := row.ID
	if row.RowKind == SetupConsoleRowMCPTool {
		id = row.ParentID
	}
	for _, item := range inventory {
		if item.ID == id {
			return setupConsoleDetailFromInventory(item), true
		}
	}
	return SetupConsoleDetailModel{}, false
}

func setupConsoleToolsFromInventory(tools []setup.InventoryTool) []SetupConsoleToolModel {
	if len(tools) == 0 {
		return nil
	}
	models := make([]SetupConsoleToolModel, 0, len(tools))
	for _, tool := range tools {
		models = append(models, SetupConsoleToolModel{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	return models
}

func setupConsoleDetailFromInventory(item setup.InventoryItem) SetupConsoleDetailModel {
	actions := make([]SetupConsoleActionModel, 0, len(item.Actions))
	for _, action := range item.Actions {
		actions = append(actions, SetupConsoleActionModel{
			Label:     string(action.Action),
			Available: action.Available,
			Reason:    action.Reason,
		})
	}
	return SetupConsoleDetailModel{
		Title:        item.Name,
		AgentLabel:   FormatAgentLabel(item.Agent),
		ObjectKind:   formatInventoryObjectKind(item),
		SourcePath:   item.SourcePath,
		Scope:        string(item.Scope),
		Status:       setupInventoryStatus(item),
		Entrypoint:   item.Entrypoint,
		EntryStatus:  item.EntryStatus,
		Actions:      actions,
		ConfigTarget: item.SourcePath,
	}
}

func setupConsoleRowFromMarketplaceSource(source setup.MarketplaceSource, expanded bool) SetupConsoleRowModel {
	return SetupConsoleRowModel{
		ID:          source.ID,
		RowKind:     SetupConsoleRowMarketplaceSource,
		Expanded:    expanded,
		Toggleable:  true,
		AgentLabel:  FormatAgentLabel(source.Agent),
		AgentMarker: FormatAgentMarker(source.Agent),
		ObjectKind:  marketplaceSourceKindLabel(source.Kind),
		Name:        source.Label,
		SourcePath:  source.Path,
		Scope:       string(source.Scope),
		Status:      marketplaceSourceStatus(source),
		ActionLabel: formatMarketplaceActions(source.Actions),
	}
}

func setupConsoleRowFromMarketplaceEntry(entry setup.MarketplaceEntry) SetupConsoleRowModel {
	return SetupConsoleRowModel{
		ID:          entry.ID,
		RowKind:     SetupConsoleRowMarketplaceEntry,
		ParentID:    entry.SourceID,
		Depth:       1,
		AgentLabel:  FormatAgentLabel(entry.Agent),
		AgentMarker: FormatAgentMarker(entry.Agent),
		ObjectKind:  marketplaceEntryKindLabel(entry),
		Name:        entry.Name,
		SourcePath:  entry.SourcePath,
		Scope:       "",
		Status:      entry.Status,
		ActionLabel: formatMarketplaceActions(entry.Actions),
	}
}

func setupConsoleDetailFromMarketplaceSource(source setup.MarketplaceSource) SetupConsoleDetailModel {
	return SetupConsoleDetailModel{
		Title:        source.Label,
		AgentLabel:   FormatAgentLabel(source.Agent),
		ObjectKind:   marketplaceSourceKindLabel(source.Kind),
		SourcePath:   source.Path,
		Scope:        string(source.Scope),
		Status:       marketplaceSourceStatus(source),
		Actions:      setupConsoleMarketplaceActions(source.Actions),
		ConfigTarget: source.Path,
	}
}

func formatInventoryObjectKind(item setup.InventoryItem) string {
	if item.Agent == types.AgentPiAgent && item.EvidenceKind == types.KindExtension {
		return "extension"
	}
	return formatSetupObjectKind(item.ObjectKind)
}

func marketplaceSourceStatus(source setup.MarketplaceSource) string {
	installed := 0
	for _, entry := range source.Entries {
		if entry.Installed {
			installed++
		}
	}
	if len(source.Entries) == 0 {
		return "0 entries"
	}
	if installed > 0 {
		return fmt.Sprintf("%d entries / %d installed", len(source.Entries), installed)
	}
	return fmt.Sprintf("%d entries", len(source.Entries))
}

func setupConsoleDetailFromMarketplaceEntry(entry setup.MarketplaceEntry) SetupConsoleDetailModel {
	return SetupConsoleDetailModel{
		Title:        entry.Name,
		AgentLabel:   FormatAgentLabel(entry.Agent),
		ObjectKind:   marketplaceEntryKindLabel(entry),
		SourcePath:   entry.SourcePath,
		Scope:        "",
		Status:       entry.Status,
		Description:  entry.Description,
		Author:       entry.Author,
		Category:     entry.Category,
		Version:      entry.Version,
		Provides:     append([]string(nil), entry.Provides...),
		Actions:      setupConsoleMarketplaceActions(entry.Actions),
		ConfigTarget: entry.SourcePath,
	}
}

func marketplaceSourceKindLabel(kind setup.MarketplaceSourceKind) string {
	switch kind {
	case setup.MarketplaceSourceMarketplace:
		return "marketplace"
	case setup.MarketplaceSourceCatalog:
		return "catalog"
	case setup.MarketplaceSourceGit:
		return "git market"
	case setup.MarketplaceSourcePlugin:
		return "plugin src"
	case setup.MarketplaceSourceExtension:
		return "extension src"
	case setup.MarketplaceSourcePackage:
		return "package src"
	case setup.MarketplaceSourceSkill:
		return "skill src"
	default:
		return "source"
	}
}

func marketplaceEntryKindLabel(entry setup.MarketplaceEntry) string {
	for _, provided := range entry.Provides {
		if provided == "plugin" {
			return "plugin"
		}
	}
	if entry.Agent == types.AgentPiAgent && entry.Kind == types.KindExtension {
		return "extension"
	}
	if entry.Agent == types.AgentOpencode && entry.SourceKind == setup.MarketplaceSourcePlugin {
		return "plugin"
	}
	return entry.Kind.String()
}

func setupConsoleMarketplaceActions(actions []setup.MarketplaceActionAvailability) []SetupConsoleActionModel {
	models := make([]SetupConsoleActionModel, 0, len(actions))
	for _, action := range actions {
		models = append(models, SetupConsoleActionModel{
			Label:     string(action.Action),
			Available: action.Available,
			Reason:    action.Reason,
		})
	}
	return models
}

func setupInventoryStatus(item setup.InventoryItem) string {
	switch item.Scope {
	case types.ScopeUser:
		return "user"
	case types.ScopeManaged:
		return "managed"
	default:
		return string(item.Scope)
	}
}

func setupConsoleTabLabel(tab SetupConsoleTab) string {
	switch tab {
	case SetupConsoleTabHooks:
		return "Hooks"
	case SetupConsoleTabPlugins:
		return "Plugins"
	case SetupConsoleTabMarketplace:
		return "Marketplace"
	case SetupConsoleTabSkills:
		return "Skills"
	case SetupConsoleTabMCPServers:
		return "MCP Servers"
	default:
		return string(tab)
	}
}

func setupConsoleEmptyMessage(tab SetupConsoleTab, search string) string {
	if strings.TrimSpace(search) != "" {
		return "No matching " + strings.ToLower(setupConsoleTabLabel(tab)) + "."
	}
	if tab == SetupConsoleTabMarketplace {
		return "No marketplace sources found."
	}
	return "No global " + strings.ToLower(setupConsoleTabLabel(tab)) + " found."
}

type BuildSetupInventoryViewModelInput struct {
	Inventory      []setup.InventoryItem
	SelectedIndex  int
	InventoryFocus bool
	PendingAction  *setup.ActionPlan
	ActionError    string
}

func BuildSetupInventoryViewModel(input BuildSetupInventoryViewModelInput) SetupInventoryViewModel {
	model := SetupInventoryViewModel{
		Rows: make([]SetupInventoryRowModel, 0, len(input.Inventory)),
	}
	selectedIndex := clampIndex(input.SelectedIndex, len(input.Inventory))
	for i, item := range input.Inventory {
		switch item.ObjectKind {
		case setup.ObjectSkill:
			model.Skills++
		case setup.ObjectMCPServer:
			model.McpServers++
		case setup.ObjectHook:
			model.Hooks++
		case setup.ObjectPlugin:
			model.Plugins++
		}
		model.Rows = append(model.Rows, SetupInventoryRowModel{
			ID:          item.ID,
			AgentLabel:  FormatAgentLabel(item.Agent),
			AgentMarker: FormatAgentMarker(item.Agent),
			ObjectKind:  formatSetupObjectKind(item.ObjectKind),
			Name:        item.Name,
			SourcePath:  item.SourcePath,
			ActionLabel: formatSetupActions(item.Actions),
			Selected:    input.InventoryFocus && i == selectedIndex,
		})
	}
	if len(model.Rows) == 0 {
		model.EmptyMessage = "No global skills, hooks, MCP servers, or plugins found."
	}
	if input.PendingAction != nil {
		model.Confirmation = buildSetupActionConfirmation(*input.PendingAction)
	}
	model.ActionError = input.ActionError
	return model
}

// --- Timeline view model ---

type TimelineRowModel struct {
	ID         string
	ShortID    string
	ObservedAt string
	EventKind  string
	Readiness  types.TimelineRestoreReadiness
	AgentScope string
	Title      string
	Selected   bool
}

type TimelineDetailModel struct {
	ID                  string
	Title               string
	EventKind           string
	Readiness           types.TimelineRestoreReadiness
	Confidence          string
	BeforeSnapshotName  string
	AfterSnapshotName   string
	CaptureID           string
	Counts              string
	Highlights          []string
	WritableSurfaces    []types.TimelineChangedSurface
	ObserveOnlySurfaces []types.TimelineChangedSurface
}

type TimelineUndoPreviewModel struct {
	Title                string
	WritesFiles          string
	WritableItems        []TimelineUndoWritableItem
	ObserveOnlySurfaces  []types.TimelineChangedSurface
	EmptyWritableMessage string
}

type TimelineUndoWritableItem struct {
	Action     string
	Path       string
	ServerName string
}

type CurrentSetupSummaryModel struct {
	ScopeLabel    string
	Agents        int
	Skills        int
	McpServers    int
	Hooks         int
	Permissions   int
	EnvKeys       int
	SkillRows     []string
	McpServerRows []string
	HookRows      []string
	EnvKeyRows    []string
	Instructions  string
}

type TimelineViewModel struct {
	FilterLabel    string
	CurrentSetup   CurrentSetupSummaryModel
	EmptyMessage   string
	EmptyCommand   string
	CorruptWarning string
	Rows           []TimelineRowModel
	SelectedEntry  *TimelineDetailModel
	UndoPreview    *TimelineUndoPreviewModel
}

type BuildTimelineViewModelInput struct {
	Entries       []types.TimelineEntry
	SelectedIndex int
	AgentFilter   *types.AgentID
	Evidence      []types.DiscoveredItem
	CorruptEvents []store.TimelineCorruptEvent
	UndoPlan      *timelineundo.Plan
	Now           time.Time
}

// BuildTimelineViewModel builds the History > All changes presentation model.
func BuildTimelineViewModel(input BuildTimelineViewModelInput) TimelineViewModel {
	selectedIndex := clampIndex(input.SelectedIndex, len(input.Entries))
	var selected *types.TimelineEntry
	if len(input.Entries) > 0 {
		selected = &input.Entries[selectedIndex]
	}
	corruptCount := len(input.CorruptEvents)

	filterLabel := "All agents"
	if input.AgentFilter != nil {
		filterLabel = FormatAgentLabel(*input.AgentFilter)
	}

	model := TimelineViewModel{
		FilterLabel: filterLabel,
		CurrentSetup: BuildCurrentSetupSummaryModel(BuildCurrentSetupSummaryInput{
			Evidence:    input.Evidence,
			AgentFilter: input.AgentFilter,
		}),
		CorruptWarning: corruptWarning(corruptCount),
		Rows:           make([]TimelineRowModel, 0, len(input.Entries)),
	}

	if len(input.Entries) == 0 {
		model.EmptyMessage = "No timeline entries yet."
		model.EmptyCommand = "Save a setup to start local history."
	}

	for i, entry := range input.Entries {
		model.Rows = append(model.Rows, BuildTimelineRow(entry, i == selectedIndex, input.Now))
	}
	if selected != nil {
		detail := BuildTimelineDetail(*selected)
		model.SelectedEntry = &detail
	}
	if input.UndoPlan != nil {
		preview := BuildTimelineUndoPreview(*input.UndoPlan)
		model.UndoPreview = &preview
	}
	return model
}

// BuildCurrentSetupSummaryInput configures current-setup summary rendering.
type BuildCurrentSetupSummaryInput struct {
	Evidence    []types.DiscoveredItem
	AgentFilter *types.AgentID
}

// BuildCurrentSetupSummaryModel summarizes live inventory above the timeline.
func BuildCurrentSetupSummaryModel(input BuildCurrentSetupSummaryInput) CurrentSetupSummaryModel {
	evidence := input.Evidence
	if input.AgentFilter != nil {
		filtered := make([]types.DiscoveredItem, 0, len(evidence))
		for _, item := range evidence {
			if item.Agent == *input.AgentFilter || item.Agent == types.AgentProject {
				filtered = append(filtered, item)
			}
		}
		evidence = filtered
	}

	instructionPaths := uniqueSortedPaths(evidence, types.KindAgentInstruction)

	scopeLabel := "All agents"
	if input.AgentFilter != nil {
		scopeLabel = FormatAgentLabel(*input.AgentFilter)
	}

	agentSet := make(map[types.AgentID]struct{})
	for _, item := range evidence {
		if item.Agent != types.AgentProject {
			agentSet[item.Agent] = struct{}{}
		}
	}

	instructions := "none"
	if len(instructionPaths) > 0 {
		limit := instructionPaths
		if len(limit) > 3 {
			limit = limit[:3]
		}
		instructions = strings.Join(limit, ", ")
	}

	return CurrentSetupSummaryModel{
		ScopeLabel:    scopeLabel,
		Agents:        len(agentSet),
		Skills:        countKind(evidence, types.KindSkill),
		McpServers:    countKind(evidence, types.KindMcpServer),
		Hooks:         countKind(evidence, types.KindHook),
		Permissions:   countKind(evidence, types.KindPermission),
		EnvKeys:       countKind(evidence, types.KindEnvKey),
		SkillRows:     rowsForKind(evidence, types.KindSkill, input.AgentFilter),
		McpServerRows: rowsForKind(evidence, types.KindMcpServer, input.AgentFilter),
		HookRows:      rowsForKind(evidence, types.KindHook, input.AgentFilter),
		EnvKeyRows:    rowsForKind(evidence, types.KindEnvKey, input.AgentFilter),
		Instructions:  instructions,
	}
}

func BuildTimelineRow(entry types.TimelineEntry, selected bool, now time.Time) TimelineRowModel {
	shortID := entry.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	return TimelineRowModel{
		ID:         entry.ID,
		ShortID:    shortID,
		ObservedAt: FormatTimelineTimestamp(entry.ObservedAt, now),
		EventKind:  string(entry.EventKind),
		Readiness:  entry.RestoreReadiness,
		AgentScope: TimelineAgentScope(entry),
		Title:      entry.Title,
		Selected:   selected,
	}
}

func BuildTimelineDetail(entry types.TimelineEntry) TimelineDetailModel {
	var writable, observeOnly []types.TimelineChangedSurface
	for _, surface := range entry.ChangedSurfaces {
		if surface.Restorable {
			writable = append(writable, surface)
		}
		if surface.ObserveOnly || !surface.Restorable {
			observeOnly = append(observeOnly, surface)
		}
	}

	beforeName := "-"
	if entry.BeforeSnapshotName != nil {
		beforeName = *entry.BeforeSnapshotName
	}

	return TimelineDetailModel{
		ID:                  entry.ID,
		Title:               entry.Title,
		EventKind:           string(entry.EventKind),
		Readiness:           entry.RestoreReadiness,
		Confidence:          fmt.Sprintf("%s: %s", entry.Confidence, entry.ConfidenceReason),
		BeforeSnapshotName:  beforeName,
		AfterSnapshotName:   entry.AfterSnapshotName,
		CaptureID:           entry.CaptureID,
		Counts:              fmt.Sprintf("%d evidence, %d graph nodes, %d findings", entry.EvidenceCount, entry.GraphNodeCount, entry.AuditFindingCount),
		Highlights:          append([]string(nil), entry.Changes.Highlights...),
		WritableSurfaces:    writable,
		ObserveOnlySurfaces: observeOnly,
	}
}

func BuildTimelineUndoPreview(plan timelineundo.Plan) TimelineUndoPreviewModel {
	items := make([]TimelineUndoWritableItem, 0, len(plan.WritableItems))
	for _, item := range plan.WritableItems {
		items = append(items, TimelineUndoWritableItem{
			Action:     string(item.Action),
			Path:       item.Path,
			ServerName: item.ServerName,
		})
	}
	model := TimelineUndoPreviewModel{
		Title:               plan.Title,
		WritesFiles:         "no",
		WritableItems:       items,
		ObserveOnlySurfaces: append([]types.TimelineChangedSurface(nil), plan.ObserveOnlySurfaces...),
	}
	if len(plan.WritableItems) == 0 {
		model.EmptyWritableMessage = "No writable MCP undo items for this event."
	}
	return model
}

func TimelineAgentScope(entry types.TimelineEntry) string {
	return FormatAgentScope(entry.Agent, entry.Agents)
}

func ClampTimelineIndex(index int, entries []types.TimelineEntry) int {
	return clampIndex(index, len(entries))
}

func corruptWarning(count int) string {
	if count <= 0 {
		return ""
	}
	suffix := "s"
	if count == 1 {
		suffix = ""
	}
	return fmt.Sprintf("%d corrupt timeline event%s skipped", count, suffix)
}

// --- Navigation model ---

type NavSectionID string

const (
	NavProfiles NavSectionID = "profiles"
	NavAgents   NavSectionID = "agents"
	NavHistory  NavSectionID = "history"
)

type NavItemKind string

const (
	NavProfile     NavItemKind = "profile"
	NavAgent       NavItemKind = "agent"
	NavHistoryItem NavItemKind = "history"
)

type NavItem struct {
	ID            string
	Kind          NavItemKind
	Label         string
	Screen        Screen
	Agent         *types.AgentID
	Profile       string
	EvidenceCount int
}

type NavSection struct {
	ID    NavSectionID
	Label string
	Items []NavItem
}

type NavigationModel struct {
	Sections       []NavSection
	FlatItems      []NavItem
	SelectedItemID string
	Cursor         int
}

type NavigationSelection struct {
	Screen          Screen
	SelectedAgent   *types.AgentID
	SelectedProfile string
}

type BuildNavigationModelInput struct {
	Evidence       []types.DiscoveredItem
	SelectedItemID string
	Cursor         int
}

// BuildNavigationModel builds sidebar navigation sections.
func BuildNavigationModel(input BuildNavigationModelInput) NavigationModel {
	sections := buildNavSections(input.Evidence)
	flat := flattenNavItems(sections)

	selectedItemID := InitialNavItemID
	if input.SelectedItemID != "" && navItemExists(flat, input.SelectedItemID) {
		selectedItemID = input.SelectedItemID
	}

	selectedIndex := 0
	for i, item := range flat {
		if item.ID == selectedItemID {
			selectedIndex = i
			break
		}
	}
	cursor := input.Cursor
	if input.Cursor == 0 && selectedIndex > 0 {
		cursor = selectedIndex
	}
	cursor = clampIndex(cursor, len(flat))

	return NavigationModel{
		Sections:       sections,
		FlatItems:      flat,
		SelectedItemID: selectedItemID,
		Cursor:         cursor,
	}
}

func buildNavSections(evidence []types.DiscoveredItem) []NavSection {
	agentItems := make([]NavItem, 0)
	for _, agent := range buildAgentEntries(evidence) {
		if agent.ID == nil {
			continue
		}
		id := *agent.ID
		agentItems = append(agentItems, NavItem{
			ID:            "agent:" + id.String(),
			Kind:          NavAgent,
			Label:         agent.Label,
			Screen:        ScreenAgentDetail,
			Agent:         &id,
			EvidenceCount: agent.EvidenceCount,
		})
	}

	return []NavSection{
		{
			ID:    "inventory",
			Label: "Inventory",
			Items: []NavItem{{
				ID:            InitialNavItemID,
				Kind:          NavHistoryItem,
				Label:         "Global setup",
				Screen:        ScreenInventory,
				EvidenceCount: countSidebarInventory(evidence, nil),
			}},
		},
		{
			ID:    NavProfiles,
			Label: "Profiles",
			Items: []NavItem{{
				ID:      "profile:" + DefaultProfile,
				Kind:    NavProfile,
				Label:   DefaultProfile,
				Screen:  ScreenProfile,
				Profile: DefaultProfile,
			}},
		},
		{
			ID:    NavAgents,
			Label: "Agents",
			Items: agentItems,
		},
		{
			ID:    NavHistory,
			Label: "History",
			Items: []NavItem{
				{
					ID:     HistoryAllNavItemID,
					Kind:   NavHistoryItem,
					Label:  "All changes",
					Screen: ScreenTimeline,
				},
				{
					ID:     "history:snapshots",
					Kind:   NavHistoryItem,
					Label:  "Snapshots",
					Screen: ScreenSnapshots,
				},
			},
		},
	}
}

func NavItemIDForSelection(selection NavigationSelection) string {
	if selection.Screen == ScreenInventory {
		return InventoryNavItemID
	}
	if selection.Screen == ScreenTimeline {
		if selection.SelectedAgent != nil {
			return "agent:" + selection.SelectedAgent.String()
		}
		return HistoryAllNavItemID
	}
	if selection.Screen == ScreenSnapshots {
		return "history:snapshots"
	}
	if selection.Screen == ScreenProfile {
		profile := selection.SelectedProfile
		if profile == "" {
			profile = DefaultProfile
		}
		return "profile:" + profile
	}
	if selection.Screen == ScreenAgentDetail && selection.SelectedAgent != nil {
		return "agent:" + selection.SelectedAgent.String()
	}
	return InitialNavItemID
}

func SelectNavItem(item NavItem, currentScreen Screen, currentAgent *types.AgentID, currentProfile string) NavigationSelection {
	if item.Kind == NavAgent {
		screen := ScreenAgentDetail
		if currentScreen == ScreenTimeline {
			screen = ScreenTimeline
		}
		agent := currentAgent
		if item.Agent != nil {
			agent = item.Agent
		}
		profile := currentProfile
		if profile == "" {
			profile = DefaultProfile
		}
		return NavigationSelection{
			Screen:          screen,
			SelectedAgent:   agent,
			SelectedProfile: profile,
		}
	}

	profile := currentProfile
	if profile == "" {
		profile = DefaultProfile
	}
	if item.Profile != "" {
		profile = item.Profile
	}
	return NavigationSelection{
		Screen:          item.Screen,
		SelectedAgent:   nil,
		SelectedProfile: profile,
	}
}

type AgentEntry struct {
	ID            *types.AgentID
	Label         string
	EvidenceCount int
}

func buildAgentEntries(evidence []types.DiscoveredItem) []AgentEntry {
	found := make(map[types.AgentID]struct{})
	for _, item := range evidence {
		found[item.Agent] = struct{}{}
	}
	entries := make([]AgentEntry, 0)
	for _, agent := range VisibleAgents {
		if _, ok := found[agent]; !ok {
			continue
		}
		entries = append(entries, AgentEntry{
			ID:            &agent,
			Label:         FormatAgentLabel(agent),
			EvidenceCount: countSidebarInventory(evidence, &agent),
		})
	}
	return entries
}

func countSidebarInventory(evidence []types.DiscoveredItem, agent *types.AgentID) int {
	count := 0
	for _, item := range evidence {
		if !setup.IsInventoryEvidence(item) {
			continue
		}
		if agent != nil {
			if item.Agent == *agent {
				count++
			}
			continue
		}
		if item.Agent != types.AgentProject {
			count++
		}
	}
	return count
}

// --- Compare view model ---

type CompareSideBySideRow struct {
	Marker string
	Before string
	After  string
}

type CompareSection struct {
	Title string
	Rows  []CompareSideBySideRow
}

type CompareViewModel struct {
	FromLabel    string
	ToLabel      string
	ScopeLabel   string
	Summary      []string
	Sections     []CompareSection
	EmptyMessage string
}

type BuildCompareViewModelInput struct {
	FromSnapshot types.Snapshot
	ToSnapshot   types.Snapshot
	Diff         diff.GraphDiff
	ToLabel      string
	Scope        string
}

// BuildCompareViewModel builds explicit From/To/Scope compare presentation.
func BuildCompareViewModel(input BuildCompareViewModelInput) CompareViewModel {
	summary := make([]string, 0, len(input.Diff.SemanticChanges))
	for _, change := range input.Diff.SemanticChanges {
		summary = append(summary, compareSummaryLabel(change))
	}
	if len(summary) == 0 {
		summary = []string{"No structured setup changes."}
	}

	toLabel := input.ToLabel
	if toLabel == "" {
		toLabel = fmt.Sprintf("%s  %s", input.ToSnapshot.Manifest.Name, formatDate(input.ToSnapshot.Manifest.CreatedAt))
	}
	scope := input.Scope
	if scope == "" {
		scope = "Full setup"
	}

	model := CompareViewModel{
		FromLabel:  fmt.Sprintf("%s  %s", input.FromSnapshot.Manifest.Name, formatDate(input.FromSnapshot.Manifest.CreatedAt)),
		ToLabel:    toLabel,
		ScopeLabel: scope,
		Summary:    summary,
		Sections:   buildSideBySideSections(input.FromSnapshot.Graph, input.ToSnapshot.Graph),
	}
	if len(input.Diff.SemanticChanges) == 0 && len(input.Diff.RawSourceChanges) == 0 {
		model.EmptyMessage = "Current setup matches the selected saved setup."
	}
	return model
}

// LatestSnapshotByCreatedAt returns the newest snapshot by manifest timestamp.
func LatestSnapshotByCreatedAt(snapshots []types.Snapshot) *types.Snapshot {
	if len(snapshots) == 0 {
		return nil
	}
	sorted := append([]types.Snapshot(nil), snapshots...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Manifest.CreatedAt > sorted[j].Manifest.CreatedAt
	})
	return &sorted[0]
}

// --- Save setup view model ---

type SaveSetupDestination struct {
	Label    string
	Selected bool
	Disabled bool
	Note     string
}

type SaveSetupViewModel struct {
	Title           string
	DetectedChanges []string
	Destinations    []SaveSetupDestination
	NoChanges       bool
}

type BaselineStatusRowModel struct {
	Agent       types.AgentID
	AgentLabel  string
	AgentMarker string
	Status      string
	Baseline    string
	Changes     string
	Unsupported string
}

type BaselineStatusViewModel struct {
	Rows       []BaselineStatusRowModel
	HasMissing bool
	HasChanges bool
}

type BuildSaveSetupViewModelInput struct {
	Diff                *diff.GraphDiff
	HasPreviousSnapshot bool
}

// BuildSaveSetupViewModel previews save-setup title and detected changes.
func BuildSaveSetupViewModel(input BuildSaveSetupViewModelInput) SaveSetupViewModel {
	semantic := []diff.SemanticChange{}
	rawCount := 0
	if input.Diff != nil {
		semantic = input.Diff.SemanticChanges
		rawCount = len(input.Diff.RawSourceChanges)
	}
	noChanges := input.HasPreviousSnapshot && len(semantic) == 0 && rawCount == 0

	changes := detectedChangeLabels(semantic, rawCount)
	if noChanges {
		changes = []string{"Current setup matches latest saved setup."}
	}

	return SaveSetupViewModel{
		Title:           snapshotTitleForChanges(input.Diff, input.HasPreviousSnapshot),
		DetectedChanges: changes,
		Destinations: []SaveSetupDestination{
			{Label: "Local history", Selected: true},
			{Label: "Export as .gandalf", Selected: false},
		},
		NoChanges: noChanges,
	}
}

// --- Environments (snapshot workspace) view model ---

type EnvironmentRowModel struct {
	Agent        types.AgentID
	AgentLabel   string
	AgentMarker  string
	State        string // "clean" | "changed" | "missing"
	BaselineName string
	BaselineDate string
	Detail       string
	Selected     bool
}

type EnvironmentFocus string

const (
	EnvironmentFocusAgents   EnvironmentFocus = "agents"
	EnvironmentFocusSurfaces EnvironmentFocus = "surfaces"
	EnvironmentFocusDiff     EnvironmentFocus = "diff"
)

type EnvironmentRenderMode string

const (
	EnvironmentRenderModeSideBySide EnvironmentRenderMode = "side_by_side"
	EnvironmentRenderModeUnified    EnvironmentRenderMode = "unified"
)

type EnvironmentSurfaceModel struct {
	ID          string
	Marker      string // + - ~
	Kind        string
	Name        string
	Detail      string
	SourcePath  string
	ChangeCount int
	Selected    bool
	Diff        EnvironmentDiffModel
}

type EnvironmentDiffRowKind string

const (
	EnvironmentDiffRowHunk    EnvironmentDiffRowKind = "hunk"
	EnvironmentDiffRowContext EnvironmentDiffRowKind = "context"
	EnvironmentDiffRowRemoved EnvironmentDiffRowKind = "removed"
	EnvironmentDiffRowAdded   EnvironmentDiffRowKind = "added"
	EnvironmentDiffRowChanged EnvironmentDiffRowKind = "changed"
)

type EnvironmentDiffSideModel struct {
	LineNumber int
	Marker     string
	Text       string
}

type EnvironmentDiffRowModel struct {
	ID          string
	Kind        EnvironmentDiffRowKind
	HunkIndex   int
	HunkTitle   string
	CurrentHunk bool
	Left        EnvironmentDiffSideModel
	Right       EnvironmentDiffSideModel
}

type EnvironmentDiffModel struct {
	SurfaceID  string
	Title      string
	SourcePath string
	Rows       []EnvironmentDiffRowModel
}

type EnvironmentsViewModel struct {
	Rows         []EnvironmentRowModel
	FocusAgent   string
	Focus        EnvironmentFocus
	Mode         EnvironmentRenderMode
	Surfaces     []EnvironmentSurfaceModel
	Diff         EnvironmentDiffModel
	DiffOffset   int
	ChangesEmpty string
	EmptyMessage string
}

type BuildEnvironmentsViewModelInput struct {
	Status               baseline.Status
	SelectedIndex        int
	SelectedSurfaceIndex int
	Focus                EnvironmentFocus
	Mode                 EnvironmentRenderMode
	CurrentHunkIndex     int
	DiffOffset           int
}

// BuildEnvironmentsViewModel builds the per-agent snapshot workspace view.
func BuildEnvironmentsViewModel(input BuildEnvironmentsViewModelInput) EnvironmentsViewModel {
	model := EnvironmentsViewModel{
		Focus:      input.Focus,
		Mode:       input.Mode,
		DiffOffset: input.DiffOffset,
	}
	if model.Focus == "" {
		model.Focus = EnvironmentFocusAgents
	}
	if model.Mode == "" {
		model.Mode = EnvironmentRenderModeSideBySide
	}
	if len(input.Status.Agents) == 0 {
		model.EmptyMessage = "No supported agents detected."
		return model
	}
	selected := clampIndex(input.SelectedIndex, len(input.Status.Agents))
	chips := BuildHeaderChips(input.Status)
	for i, agentStatus := range input.Status.Agents {
		chip := HeaderChipModel{}
		if i < len(chips) {
			chip = chips[i]
		}
		row := EnvironmentRowModel{
			Agent:        agentStatus.Agent,
			AgentLabel:   FormatAgentLabel(agentStatus.Agent),
			AgentMarker:  chip.AgentMarker,
			State:        chip.State,
			BaselineName: agentStatus.BaselineName,
			BaselineDate: formatDate(agentStatus.BaselineCreatedAt),
			Detail:       chip.Detail,
			Selected:     i == selected,
		}
		if row.AgentMarker == "" {
			row.AgentMarker = FormatAgentMarker(agentStatus.Agent)
		}
		model.Rows = append(model.Rows, row)
	}

	focus := input.Status.Agents[selected]
	model.FocusAgent = FormatAgentLabel(focus.Agent)
	if !focus.HasBaseline {
		model.ChangesEmpty = "No baseline yet. Press s to save the current environment as a baseline."
		return model
	}
	totalSurfaces := len(focus.Diff.SemanticChanges) + len(focus.Diff.RawSourceChanges)
	surfaceIndex := clampIndex(input.SelectedSurfaceIndex, totalSurfaces)
	model.Surfaces = buildEnvironmentSurfaces(focus.Diff, surfaceIndex, input.CurrentHunkIndex)
	if len(model.Surfaces) == 0 {
		model.ChangesEmpty = "Current environment matches the baseline."
		return model
	}
	for i := range model.Surfaces {
		model.Surfaces[i].Selected = i == surfaceIndex
	}
	model.Diff = model.Surfaces[surfaceIndex].Diff
	return model
}

func environmentChangeDetail(change diff.SemanticChange) string {
	if len(change.Details.ChangedFields) > 0 {
		return strings.Join(change.Details.ChangedFields, ", ") + " changed"
	}
	s := string(change.Code)
	switch {
	case strings.HasSuffix(s, "_ADDED"):
		return "added"
	case strings.HasSuffix(s, "_REMOVED"):
		return "removed"
	default:
		return "changed"
	}
}

func buildEnvironmentSurfaces(graphDiff diff.GraphDiff, selectedSurfaceIndex, currentHunkIndex int) []EnvironmentSurfaceModel {
	surfaces := make([]EnvironmentSurfaceModel, 0, len(graphDiff.SemanticChanges)+len(graphDiff.RawSourceChanges))
	for index, change := range graphDiff.SemanticChanges {
		sourcePath := ""
		if change.Details.SourcePath != nil {
			sourcePath = *change.Details.SourcePath
		}
		kind := entityKindLabel(change.EntityKind)
		title := strings.TrimSpace(kind + " " + change.EntityName)
		if title == "" {
			title = environmentChangeDetail(change)
		}
		id := fmt.Sprintf("%s:%s:%s:%d", change.EntityKind, change.EntityName, sourcePath, index)
		changeCount := countEnvironmentChanges(change)
		detail := environmentChangeDetail(change)
		if changeCount > 0 {
			suffix := "changes"
			if changeCount == 1 {
				suffix = "change"
			}
			detail = fmt.Sprintf("%d %s", changeCount, suffix)
		}
		var diffModel EnvironmentDiffModel
		if index == selectedSurfaceIndex {
			diffModel = buildEnvironmentDiffModel(id, title, sourcePath, change, currentHunkIndex)
		}
		surfaces = append(surfaces, EnvironmentSurfaceModel{
			ID:          id,
			Marker:      markerForChange(change.Code),
			Kind:        kind,
			Name:        change.EntityName,
			Detail:      detail,
			SourcePath:  sourcePath,
			ChangeCount: changeCount,
			Diff:        diffModel,
		})
	}
	for index, change := range graphDiff.RawSourceChanges {
		globalIndex := len(graphDiff.SemanticChanges) + index
		id := fmt.Sprintf("raw:%s:%d", change.SourcePath, index)
		changeCount := countEnvironmentRawChanges(change)
		var diffModel EnvironmentDiffModel
		if globalIndex == selectedSurfaceIndex {
			diffModel = buildEnvironmentRawDiffModel(id, change, currentHunkIndex)
		}
		surfaces = append(surfaces, EnvironmentSurfaceModel{
			ID:          id,
			Marker:      markerForRawSourceChange(change),
			Kind:        "Source",
			Name:        change.SourcePath,
			Detail:      rawSourceChangeDetail(change),
			SourcePath:  change.SourcePath,
			ChangeCount: changeCount,
			Diff:        diffModel,
		})
	}
	return surfaces
}

func buildEnvironmentRawDiffModel(surfaceID string, change diff.RawSourceChange, currentHunkIndex int) EnvironmentDiffModel {
	title := "Source " + change.SourcePath
	model := EnvironmentDiffModel{
		SurfaceID:  surfaceID,
		Title:      title,
		SourcePath: change.SourcePath,
	}
	rows := []EnvironmentDiffRowModel{{
		ID:          surfaceID + ":hunk:0",
		Kind:        EnvironmentDiffRowHunk,
		HunkIndex:   0,
		HunkTitle:   environmentHunkTitle(title, change.SourcePath),
		CurrentHunk: currentHunkIndex == 0,
	}}
	addRawPair := func(field string, before, after *string) {
		if before == nil && after == nil {
			return
		}
		row := EnvironmentDiffRowModel{
			ID:        surfaceID + ":" + field,
			Kind:      EnvironmentDiffRowContext,
			HunkIndex: 0,
			Left: EnvironmentDiffSideModel{
				LineNumber: 1,
				Marker:     " ",
			},
			Right: EnvironmentDiffSideModel{
				LineNumber: 1,
				Marker:     " ",
			},
		}
		if before != nil {
			row.Left.Text = field + ": " + *before
		}
		if after != nil {
			row.Right.Text = field + ": " + *after
		}
		if stringPtrValue(before) != stringPtrValue(after) {
			switch {
			case before != nil && after != nil:
				row.Kind = EnvironmentDiffRowChanged
				row.Left.Marker = "-"
				row.Right.Marker = "+"
			case before != nil:
				row.Kind = EnvironmentDiffRowRemoved
				row.Left.Marker = "-"
				row.Right = EnvironmentDiffSideModel{}
			case after != nil:
				row.Kind = EnvironmentDiffRowAdded
				row.Left = EnvironmentDiffSideModel{}
				row.Right.Marker = "+"
			}
		}
		rows = append(rows, row)
	}
	addRawPair("evidence", change.BeforeEvidenceID, change.AfterEvidenceID)
	addRawPair("checksum", change.BeforeChecksum, change.AfterChecksum)
	beforeStatus, afterStatus := rawStatusPair(change.Status)
	addRawPair("status", beforeStatus, afterStatus)
	model.Rows = rows
	return model
}

func buildEnvironmentDiffModel(surfaceID, title, sourcePath string, change diff.SemanticChange, currentHunkIndex int) EnvironmentDiffModel {
	model := EnvironmentDiffModel{
		SurfaceID:  surfaceID,
		Title:      title,
		SourcePath: sourcePath,
	}
	beforeParsed, beforeOK := parseJSONValue(change.Before)
	afterParsed, afterOK := parseJSONValue(change.After)
	beforeObject, beforeIsObject := beforeParsed.(map[string]any)
	afterObject, afterIsObject := afterParsed.(map[string]any)
	if beforeIsObject || afterIsObject {
		model.Rows = buildEnvironmentObjectDiffRows(surfaceID, title, sourcePath, beforeObject, beforeIsObject, afterObject, afterIsObject, change.Details.ChangedFields, currentHunkIndex)
		return model
	}
	model.Rows = buildEnvironmentScalarDiffRows(surfaceID, title, sourcePath, change.Before, beforeParsed, beforeOK, change.After, afterParsed, afterOK, currentHunkIndex)
	return model
}

func buildEnvironmentObjectDiffRows(surfaceID, title, sourcePath string, before map[string]any, hasBefore bool, after map[string]any, hasAfter bool, changedFields []string, currentHunkIndex int) []EnvironmentDiffRowModel {
	fields := environmentAllFields(before, hasBefore, after, hasAfter)
	changed := environmentChangedFieldSet(before, hasBefore, after, hasAfter, changedFields, fields)
	if len(changed) == 0 {
		return nil
	}
	beforeLines := environmentLineNumbers(fields, before, hasBefore)
	afterLines := environmentLineNumbers(fields, after, hasAfter)
	ranges := environmentHunkRanges(fields, changed, 2)
	rows := make([]EnvironmentDiffRowModel, 0, len(fields)+len(ranges))
	for hunkIndex, hunkRange := range ranges {
		rows = append(rows, EnvironmentDiffRowModel{
			ID:          fmt.Sprintf("%s:hunk:%d", surfaceID, hunkIndex),
			Kind:        EnvironmentDiffRowHunk,
			HunkIndex:   hunkIndex,
			HunkTitle:   environmentHunkTitle(title, sourcePath),
			CurrentHunk: hunkIndex == currentHunkIndex,
		})
		for _, field := range fields[hunkRange.Start : hunkRange.End+1] {
			beforeValue, beforeOK := before[field]
			afterValue, afterOK := after[field]
			row := EnvironmentDiffRowModel{
				ID:        fmt.Sprintf("%s:%d:%s", surfaceID, hunkIndex, field),
				Kind:      EnvironmentDiffRowContext,
				HunkIndex: hunkIndex,
				Left: EnvironmentDiffSideModel{
					LineNumber: beforeLines[field],
					Marker:     " ",
				},
				Right: EnvironmentDiffSideModel{
					LineNumber: afterLines[field],
					Marker:     " ",
				},
			}
			if beforeOK && hasBefore {
				row.Left.Text = environmentFieldText(field, beforeValue)
			}
			if afterOK && hasAfter {
				row.Right.Text = environmentFieldText(field, afterValue)
			}
			if _, ok := changed[field]; ok {
				switch {
				case beforeOK && hasBefore && afterOK && hasAfter:
					row.Kind = EnvironmentDiffRowChanged
					row.Left.Marker = "-"
					row.Right.Marker = "+"
				case beforeOK && hasBefore:
					row.Kind = EnvironmentDiffRowRemoved
					row.Left.Marker = "-"
					row.Right = EnvironmentDiffSideModel{}
				case afterOK && hasAfter:
					row.Kind = EnvironmentDiffRowAdded
					row.Left = EnvironmentDiffSideModel{}
					row.Right.Marker = "+"
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func buildEnvironmentScalarDiffRows(surfaceID, title, sourcePath string, before json.RawMessage, beforeParsed any, beforeOK bool, after json.RawMessage, afterParsed any, afterOK bool, currentHunkIndex int) []EnvironmentDiffRowModel {
	left := environmentValueText(before, beforeParsed, beforeOK)
	right := environmentValueText(after, afterParsed, afterOK)
	if left == right {
		return nil
	}
	row := EnvironmentDiffRowModel{
		ID:        surfaceID + ":scalar",
		Kind:      EnvironmentDiffRowChanged,
		HunkIndex: 0,
		Left: EnvironmentDiffSideModel{
			LineNumber: 1,
			Marker:     "-",
			Text:       left,
		},
		Right: EnvironmentDiffSideModel{
			LineNumber: 1,
			Marker:     "+",
			Text:       right,
		},
	}
	if strings.TrimSpace(left) == "" {
		row.Kind = EnvironmentDiffRowAdded
		row.Left = EnvironmentDiffSideModel{}
	}
	if strings.TrimSpace(right) == "" {
		row.Kind = EnvironmentDiffRowRemoved
		row.Right = EnvironmentDiffSideModel{}
	}
	return []EnvironmentDiffRowModel{{
		ID:          surfaceID + ":hunk:0",
		Kind:        EnvironmentDiffRowHunk,
		HunkIndex:   0,
		HunkTitle:   environmentHunkTitle(title, sourcePath),
		CurrentHunk: currentHunkIndex == 0,
	}, row}
}

func environmentHunkTitle(title, sourcePath string) string {
	parts := []string{strings.TrimSpace(title)}
	if strings.TrimSpace(sourcePath) != "" {
		parts = append(parts, strings.TrimSpace(sourcePath))
	}
	return "@@ " + strings.Join(parts, " · ") + " @@"
}

type environmentHunkRange struct {
	Start int
	End   int
}

func environmentHunkRanges(fields []string, changed map[string]struct{}, context int) []environmentHunkRange {
	var ranges []environmentHunkRange
	for index, field := range fields {
		if _, ok := changed[field]; !ok {
			continue
		}
		start := max(0, index-context)
		end := min(len(fields)-1, index+context)
		if len(ranges) > 0 && start <= ranges[len(ranges)-1].End+1 {
			if end > ranges[len(ranges)-1].End {
				ranges[len(ranges)-1].End = end
			}
			continue
		}
		ranges = append(ranges, environmentHunkRange{Start: start, End: end})
	}
	return ranges
}

func environmentAllFields(before map[string]any, hasBefore bool, after map[string]any, hasAfter bool) []string {
	keys := make(map[string]struct{}, len(before)+len(after))
	if hasBefore {
		for key := range before {
			keys[key] = struct{}{}
		}
	}
	if hasAfter {
		for key := range after {
			keys[key] = struct{}{}
		}
	}
	fields := make([]string, 0, len(keys))
	for key := range keys {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
}

func environmentChangedFieldSet(before map[string]any, hasBefore bool, after map[string]any, hasAfter bool, changedFields []string, allFields []string) map[string]struct{} {
	changed := make(map[string]struct{})
	for _, field := range changedFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		beforeValue, beforeOK := before[field]
		afterValue, afterOK := after[field]
		if hasVisibleFieldDiff(beforeValue, beforeOK && hasBefore, afterValue, afterOK && hasAfter) {
			changed[field] = struct{}{}
		}
	}
	if len(changed) > 0 {
		return changed
	}
	for _, field := range allFields {
		beforeValue, beforeOK := before[field]
		afterValue, afterOK := after[field]
		if hasVisibleFieldDiff(beforeValue, beforeOK && hasBefore, afterValue, afterOK && hasAfter) {
			changed[field] = struct{}{}
		}
	}
	return changed
}

func environmentLineNumbers(fields []string, value map[string]any, ok bool) map[string]int {
	out := make(map[string]int, len(value))
	if !ok {
		return out
	}
	line := 1
	for _, field := range fields {
		if _, ok := value[field]; !ok {
			continue
		}
		out[field] = line
		line++
	}
	return out
}

func environmentFieldText(field string, value any) string {
	return fmt.Sprintf("%s: %s", field, compactJSONValue(value))
}

func environmentValueText(value json.RawMessage, parsed any, parsedOK bool) string {
	if len(value) == 0 {
		return ""
	}
	if parsedOK {
		return compactJSONValue(parsed)
	}
	return strings.TrimSpace(string(value))
}

func countEnvironmentChanges(change diff.SemanticChange) int {
	beforeParsed, beforeOK := parseJSONValue(change.Before)
	afterParsed, afterOK := parseJSONValue(change.After)
	beforeObject, beforeIsObject := beforeParsed.(map[string]any)
	afterObject, afterIsObject := afterParsed.(map[string]any)
	if beforeIsObject || afterIsObject {
		fields := environmentAllFields(beforeObject, beforeIsObject, afterObject, afterIsObject)
		return len(environmentChangedFieldSet(beforeObject, beforeIsObject, afterObject, afterIsObject, change.Details.ChangedFields, fields))
	}
	if environmentValueText(change.Before, beforeParsed, beforeOK) == environmentValueText(change.After, afterParsed, afterOK) {
		return 0
	}
	return 1
}

func countEnvironmentRawChanges(change diff.RawSourceChange) int {
	count := 0
	if stringPtrValue(change.BeforeEvidenceID) != stringPtrValue(change.AfterEvidenceID) {
		count++
	}
	if stringPtrValue(change.BeforeChecksum) != stringPtrValue(change.AfterChecksum) {
		count++
	}
	if count == 0 && strings.TrimSpace(change.Status) != "" {
		count = 1
	}
	return count
}

func rawSourceChangeDetail(change diff.RawSourceChange) string {
	switch strings.ToLower(strings.TrimSpace(change.Status)) {
	case "added":
		return "added"
	case "removed":
		return "removed"
	case "changed", "modified":
		return "changed"
	default:
		if change.BeforeEvidenceID == nil {
			return "added"
		}
		if change.AfterEvidenceID == nil {
			return "removed"
		}
		return "changed"
	}
}

func markerForRawSourceChange(change diff.RawSourceChange) string {
	switch rawSourceChangeDetail(change) {
	case "added":
		return "+"
	case "removed":
		return "-"
	default:
		return "~"
	}
}

func rawStatusPair(status string) (*string, *string) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "added":
		after := "present"
		return nil, &after
	case "removed":
		before := "present"
		return &before, nil
	default:
		before := "baseline"
		after := "current"
		return &before, &after
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func hasVisibleFieldDiff(before any, beforeOK bool, after any, afterOK bool) bool {
	if beforeOK != afterOK {
		return true
	}
	if !beforeOK && !afterOK {
		return false
	}
	return compactJSONValue(before) != compactJSONValue(after)
}

func parseJSONValue(value json.RawMessage) (any, bool) {
	if len(value) == 0 {
		return nil, false
	}
	var parsed any
	if err := json.Unmarshal(value, &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

func compactJSONValue(value any) string {
	data, err := json.Marshal(normalizeValue(value))
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

// HeaderChipModel is one per-agent drift indicator for the app header.
type HeaderChipModel struct {
	AgentMarker string
	State       string
	Detail      string
}

// BuildHeaderChips converts baseline status into per-agent header chips.
func BuildHeaderChips(status baseline.Status) []HeaderChipModel {
	chips := make([]HeaderChipModel, 0, len(status.Agents))
	for _, agentStatus := range status.Agents {
		chip := HeaderChipModel{AgentMarker: FormatAgentMarker(agentStatus.Agent)}
		switch {
		case !agentStatus.HasBaseline:
			chip.State = "missing"
			chip.Detail = "no baseline"
		case agentStatus.ChangeCount() == 0:
			chip.State = "clean"
			chip.Detail = "clean"
		default:
			chip.State = "changed"
			count := agentStatus.ChangeCount()
			suffix := "s"
			if count == 1 {
				suffix = ""
			}
			chip.Detail = fmt.Sprintf("%d change%s", count, suffix)
		}
		chips = append(chips, chip)
	}
	return chips
}

// BuildBaselineStatusViewModel converts core baseline status into compact TUI rows.
func BuildBaselineStatusViewModel(status baseline.Status) BaselineStatusViewModel {
	model := BaselineStatusViewModel{Rows: make([]BaselineStatusRowModel, 0, len(status.Agents))}
	for _, agentStatus := range status.Agents {
		row := BaselineStatusRowModel{
			Agent:       agentStatus.Agent,
			AgentLabel:  FormatAgentLabel(agentStatus.Agent),
			AgentMarker: FormatAgentMarker(agentStatus.Agent),
			Baseline:    agentStatus.BaselineName,
			Changes:     fmt.Sprintf("%d changes", agentStatus.ChangeCount()),
		}
		if !agentStatus.HasBaseline {
			row.Status = "missing baseline"
			row.Baseline = "-"
			row.Changes = "-"
			model.HasMissing = true
		} else if agentStatus.ChangeCount() == 0 {
			row.Status = "clean"
		} else {
			row.Status = "changed"
			model.HasChanges = true
		}
		if agentStatus.UnsupportedCount > 0 || agentStatus.OmittedContentCount > 0 {
			row.Unsupported = fmt.Sprintf("%d unsupported, %d omitted", agentStatus.UnsupportedCount, agentStatus.OmittedContentCount)
		}
		model.Rows = append(model.Rows, row)
	}
	return model
}

// --- Agent detail view model ---

type AgentInventoryRow struct {
	Name   string
	Status string
	Path   string
}

type AgentHistoryRow struct {
	ID         string
	ObservedAt string
	Title      string
}

type AgentDetailViewModel struct {
	Title        string
	ProfileLabel string
	Counts       struct {
		Skills       int
		McpServers   int
		Hooks        int
		Permissions  int
		EnvKeys      int
		Instructions int
	}
	Skills       []AgentInventoryRow
	McpServers   []AgentInventoryRow
	Hooks        []AgentInventoryRow
	EnvKeys      []AgentInventoryRow
	Instructions []AgentInventoryRow
	History      []AgentHistoryRow
	EmptyMessage string
}

type BuildAgentDetailViewModelInput struct {
	Agent           types.AgentID
	Evidence        []types.DiscoveredItem
	TimelineEntries []types.TimelineEntry
	Profile         string
	Now             time.Time
}

// BuildAgentDetailViewModel builds per-agent inventory and filtered history.
func BuildAgentDetailViewModel(input BuildAgentDetailViewModelInput) AgentDetailViewModel {
	agentEvidence := filterEvidenceByAgent(input.Evidence, input.Agent, false)
	setupEvidence := filterEvidenceByAgent(input.Evidence, input.Agent, true)

	profile := input.Profile
	if profile == "" {
		profile = DefaultProfile
	}

	model := AgentDetailViewModel{
		Title:        FormatAgentLabel(input.Agent),
		ProfileLabel: profile,
	}
	model.Counts.Skills = countKind(setupEvidence, types.KindSkill)
	model.Counts.McpServers = countKind(setupEvidence, types.KindMcpServer)
	model.Counts.Hooks = countKind(setupEvidence, types.KindHook)
	model.Counts.Permissions = countKind(setupEvidence, types.KindPermission)
	model.Counts.EnvKeys = countKind(setupEvidence, types.KindEnvKey)
	model.Counts.Instructions = countKind(setupEvidence, types.KindAgentInstruction)

	model.Skills = agentRowsForKind(setupEvidence, types.KindSkill)
	model.McpServers = agentRowsForKind(setupEvidence, types.KindMcpServer)
	for i := range model.McpServers {
		if model.McpServers[i].Status == "" {
			model.McpServers[i].Status = "enabled"
		}
	}
	model.Hooks = agentRowsForKind(setupEvidence, types.KindHook)
	model.EnvKeys = agentRowsForKind(setupEvidence, types.KindEnvKey)
	model.Instructions = agentRowsForKind(setupEvidence, types.KindAgentInstruction)

	for _, entry := range input.TimelineEntries {
		if !timelineEntryMatchesAgent(entry, input.Agent) {
			continue
		}
		shortID := entry.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		model.History = append(model.History, AgentHistoryRow{
			ID:         shortID,
			ObservedAt: FormatTimelineTimestamp(entry.ObservedAt, input.Now),
			Title:      entry.Title,
		})
		if len(model.History) >= 6 {
			break
		}
	}

	if len(agentEvidence) == 0 {
		model.EmptyMessage = "No supported agent setup found."
	}
	return model
}

// FilterTimelineEntries filters timeline entries by optional agent scope.
func FilterTimelineEntries(entries []types.TimelineEntry, agent *types.AgentID) []types.TimelineEntry {
	if agent == nil {
		return append([]types.TimelineEntry(nil), entries...)
	}
	filtered := make([]types.TimelineEntry, 0)
	for _, entry := range entries {
		if timelineEntryMatchesAgent(entry, *agent) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// --- helpers ---

func countKind(evidence []types.DiscoveredItem, kind types.EvidenceKind) int {
	count := 0
	for _, item := range evidence {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func rowsForKind(evidence []types.DiscoveredItem, kind types.EvidenceKind, agentFilter *types.AgentID) []string {
	rows := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range evidence {
		if item.Kind != kind {
			continue
		}
		name := displayNameForItem(item)
		var row string
		if agentFilter != nil {
			if item.Agent == types.AgentProject && FormatInventorySourceRoot(item) == "" {
				row = fmt.Sprintf("%s (project)", name)
			} else {
				row = name
			}
		} else {
			row = fmt.Sprintf("%s: %s", FormatAgentLabel(item.Agent), name)
		}
		if _, ok := seen[row]; ok {
			continue
		}
		seen[row] = struct{}{}
		rows = append(rows, row)
	}
	sort.Strings(rows)
	return rows
}

func displayNameForItem(item types.DiscoveredItem) string {
	meta := parseMetadata(item.Metadata)
	suffix := ""
	if item.Scope == types.ScopeManaged || metadataBool(meta, "builtIn") {
		suffix = " (built-in)"
	}
	if item.Name != nil && *item.Name != "" {
		return FormatInventoryNameWithSource(*item.Name+suffix, item)
	}
	parts := strings.Split(strings.Trim(item.SourcePath, "/"), "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if last != "" && last != "SKILL.md" {
			return FormatInventoryNameWithSource(last+suffix, item)
		}
	}
	if len(parts) > 1 {
		parent := parts[len(parts)-2]
		return FormatInventoryNameWithSource(parent+suffix, item)
	}
	return FormatInventoryNameWithSource(item.ID+suffix, item)
}

func agentRowsForKind(evidence []types.DiscoveredItem, kind types.EvidenceKind) []AgentInventoryRow {
	rows := make([]AgentInventoryRow, 0)
	for _, item := range evidence {
		if item.Kind != kind {
			continue
		}
		rows = append(rows, AgentInventoryRow{
			Name:   agentDisplayNameForItem(item),
			Path:   item.SourcePath,
			Status: statusForItem(item),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func agentDisplayNameForItem(item types.DiscoveredItem) string {
	name := item.ID
	if item.Name != nil {
		name = *item.Name
	}
	sourceLabeled := FormatInventoryNameWithSource(name, item)
	if sourceLabeled != name {
		return sourceLabeled
	}
	if item.Agent == types.AgentProject {
		return name + " (project)"
	}
	return name
}

func statusForItem(item types.DiscoveredItem) string {
	if item.Kind == types.KindMcpServer {
		var value map[string]any
		if len(item.Value) > 0 {
			_ = json.Unmarshal(item.Value, &value)
		}
		if value != nil {
			if disabled, ok := value["disabled"].(bool); ok && disabled {
				return "disabled"
			}
			if enabled, ok := value["enabled"].(bool); ok && !enabled {
				return "disabled"
			}
		}
		return "enabled"
	}
	if item.CaptureStatus != types.CaptureCaptured {
		return item.CaptureStatus.String()
	}
	return ""
}

func filterEvidenceByAgent(evidence []types.DiscoveredItem, agent types.AgentID, includeProject bool) []types.DiscoveredItem {
	filtered := make([]types.DiscoveredItem, 0)
	for _, item := range evidence {
		if item.Agent == agent {
			filtered = append(filtered, item)
			continue
		}
		if includeProject && item.Agent == types.AgentProject {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func timelineEntryMatchesAgent(entry types.TimelineEntry, agent types.AgentID) bool {
	if entry.Agent != nil && *entry.Agent == agent {
		return true
	}
	for _, a := range entry.Agents {
		if a == agent {
			return true
		}
	}
	return false
}

func uniqueSortedPaths(evidence []types.DiscoveredItem, kind types.EvidenceKind) []string {
	seen := make(map[string]struct{})
	paths := make([]string, 0)
	for _, item := range evidence {
		if item.Kind != kind {
			continue
		}
		if _, ok := seen[item.SourcePath]; ok {
			continue
		}
		seen[item.SourcePath] = struct{}{}
		paths = append(paths, item.SourcePath)
	}
	sort.Strings(paths)
	return paths
}

func clampIndex(index, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func flattenNavItems(sections []NavSection) []NavItem {
	items := make([]NavItem, 0)
	for _, section := range sections {
		items = append(items, section.Items...)
	}
	return items
}

func navItemExists(items []NavItem, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func formatDate(value string) string {
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func compareSummaryLabel(change diff.SemanticChange) string {
	prefix := markerForChange(change.Code)
	owner := ""
	if change.EntityKind == types.KindAgentInstruction {
		owner = "Project "
	}
	return fmt.Sprintf("%s %s%s: %s", prefix, owner, entityKindLabel(change.EntityKind), change.EntityName)
}

func markerForChange(code diff.SemanticChangeCode) string {
	s := string(code)
	if strings.HasSuffix(s, "_ADDED") {
		return "+"
	}
	if strings.HasSuffix(s, "_REMOVED") {
		return "-"
	}
	return "~"
}

func entityKindLabel(kind types.EvidenceKind) string {
	switch kind {
	case types.KindMcpServer:
		return "MCP"
	case types.KindSkill:
		return "Skill"
	case types.KindPermission:
		return "Permission"
	case types.KindHook:
		return "Hook"
	case types.KindEnvKey:
		return "Env key"
	case types.KindAgentInstruction:
		return "Instructions"
	default:
		return "Setup"
	}
}

func buildSideBySideSections(beforeGraph, afterGraph []types.GraphNode) []CompareSection {
	beforeByIdentity := make(map[string]types.GraphNode)
	for _, node := range beforeGraph {
		beforeByIdentity[graphIdentity(node)] = node
	}
	afterByIdentity := make(map[string]types.GraphNode)
	for _, node := range afterGraph {
		afterByIdentity[graphIdentity(node)] = node
	}

	identities := make([]string, 0)
	seen := make(map[string]struct{})
	for id := range beforeByIdentity {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			identities = append(identities, id)
		}
	}
	for id := range afterByIdentity {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			identities = append(identities, id)
		}
	}
	sort.Strings(identities)

	sections := make(map[string][]CompareSideBySideRow)
	order := make([]string, 0)
	for _, identity := range identities {
		before, hasBefore := beforeByIdentity[identity]
		after, hasAfter := afterByIdentity[identity]
		var node types.GraphNode
		switch {
		case hasAfter:
			node = after
		case hasBefore:
			node = before
		default:
			continue
		}
		title := FormatAgentLabel(node.Agent)
		if _, ok := sections[title]; !ok {
			order = append(order, title)
		}
		var beforeNode, afterNode *types.GraphNode
		if hasBefore {
			b := before
			beforeNode = &b
		}
		if hasAfter {
			a := after
			afterNode = &a
		}
		sections[title] = append(sections[title], CompareSideBySideRow{
			Marker: markerForNodes(beforeNode, afterNode),
			Before: nodeLabel(beforeNode),
			After:  nodeLabel(afterNode),
		})
	}

	result := make([]CompareSection, 0, len(order))
	for _, title := range order {
		result = append(result, CompareSection{
			Title: title,
			Rows:  sections[title],
		})
	}
	return result
}

func graphIdentity(node types.GraphNode) string {
	return strings.Join([]string{node.Agent.String(), node.EntityKind.String(), node.EntityName}, "\x00")
}

func nodeLabel(node *types.GraphNode) string {
	if node == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", node.EntityKind, node.EntityName)
}

func markerForNodes(before, after *types.GraphNode) string {
	if before == nil && after != nil {
		return "+"
	}
	if before != nil && after == nil {
		return "-"
	}
	if before != nil && after != nil && stableJSON(before.EffectiveValue) != stableJSON(after.EffectiveValue) {
		return "~"
	}
	return " "
}

func stableJSON(value json.RawMessage) string {
	if len(value) == 0 {
		return "null"
	}
	var parsed any
	if err := json.Unmarshal(value, &parsed); err != nil {
		return string(value)
	}
	normalized := normalizeValue(parsed)
	data, err := json.Marshal(normalized)
	if err != nil {
		return string(value)
	}
	return string(data)
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make(map[string]any, len(keys))
		for _, key := range keys {
			result[key] = normalizeValue(typed[key])
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = normalizeValue(item)
		}
		return result
	default:
		return value
	}
}

func snapshotTitleForChanges(graphDiff *diff.GraphDiff, hasPreviousSnapshot bool) string {
	if !hasPreviousSnapshot {
		return "capture baseline"
	}
	if graphDiff == nil || (len(graphDiff.SemanticChanges) == 0 && len(graphDiff.RawSourceChanges) == 0) {
		return "current setup unchanged"
	}

	semantic := graphDiff.SemanticChanges
	if change := firstChange(semantic, diff.SemanticMcpAdded, diff.SemanticMcpRemoved, diff.SemanticMcpChanged); change != nil {
		return titleForSemanticChange(*change)
	}
	if change := firstChange(semantic, diff.SemanticSkillAdded, diff.SemanticSkillRemoved, diff.SemanticSkillExecutableAppeared); change != nil {
		return titleForSemanticChange(*change)
	}
	if change := firstChange(semantic,
		diff.SemanticHookAdded, diff.SemanticHookRemoved, diff.SemanticHookChanged,
		diff.SemanticPermissionChanged, diff.SemanticPermissionWildcardAdded,
	); change != nil {
		return titleForSemanticChange(*change)
	}
	if change := firstChange(semantic, diff.SemanticInstructionChanged); change != nil {
		return titleForSemanticChange(*change)
	}
	if change := firstChange(semantic, diff.SemanticEnvKeyAdded, diff.SemanticEnvKeyRemoved); change != nil {
		return titleForSemanticChange(*change)
	}
	if len(semantic) > 1 || len(graphDiff.RawSourceChanges) > 1 {
		return fmt.Sprintf("change %d setup items and %d files", len(semantic), len(graphDiff.RawSourceChanges))
	}
	return "update setup"
}

func detectedChangeLabels(semantic []diff.SemanticChange, rawSourceChangeCount int) []string {
	labels := make([]string, 0, 8)
	limit := len(semantic)
	if limit > 8 {
		limit = 8
	}
	for _, change := range semantic[:limit] {
		labels = append(labels, titleForSemanticChange(change))
	}
	if rawSourceChangeCount > 0 {
		suffix := "s"
		if rawSourceChangeCount == 1 {
			suffix = ""
		}
		labels = append(labels, fmt.Sprintf("change %d source file%s", rawSourceChangeCount, suffix))
	}
	if len(labels) == 0 {
		return []string{"capture baseline"}
	}
	return labels
}

func firstChange(changes []diff.SemanticChange, codes ...diff.SemanticChangeCode) *diff.SemanticChange {
	codeSet := make(map[diff.SemanticChangeCode]struct{}, len(codes))
	for _, code := range codes {
		codeSet[code] = struct{}{}
	}
	for i := range changes {
		if _, ok := codeSet[changes[i].Code]; ok {
			return &changes[i]
		}
	}
	return nil
}

func titleForSemanticChange(change diff.SemanticChange) string {
	switch change.Code {
	case diff.SemanticMcpAdded:
		return fmt.Sprintf("add %s mcp", change.EntityName)
	case diff.SemanticMcpRemoved:
		return fmt.Sprintf("remove %s mcp", change.EntityName)
	case diff.SemanticMcpChanged:
		return fmt.Sprintf("update %s mcp", change.EntityName)
	case diff.SemanticSkillAdded, diff.SemanticSkillExecutableAppeared:
		return fmt.Sprintf("install %s skill", change.EntityName)
	case diff.SemanticSkillRemoved:
		return fmt.Sprintf("remove %s skill", change.EntityName)
	case diff.SemanticHookAdded, diff.SemanticHookRemoved, diff.SemanticHookChanged:
		return "update hooks"
	case diff.SemanticPermissionChanged, diff.SemanticPermissionWildcardAdded:
		return "update permissions"
	case diff.SemanticInstructionChanged:
		return "update project instructions"
	case diff.SemanticEnvKeyAdded, diff.SemanticEnvKeyRemoved:
		return "update env key inventory"
	default:
		return "update setup"
	}
}
