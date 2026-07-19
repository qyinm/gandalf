package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/baseline"
	"github.com/qyinm/gandalf/internal/gandalfcore/restore"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	timelineundo "github.com/qyinm/gandalf/internal/gandalfcore/timeline_undo"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
	"github.com/qyinm/gandalf/internal/tui/views"
)

type bootMsg struct {
	evidence        []types.DiscoveredItem
	timelineEntries []types.TimelineEntry
	corruptEvents   []store.TimelineCorruptEvent
	snapshotRefs    []snapshotRef
	baselineStatus  baseline.Status
	err             error
}

type rescanMsg bootMsg

type setupActionMsg struct {
	data       bootMsg
	err        error
	plan       *setup.ActionPlan
	installed  bool
	rolledBack bool
}

type marketplaceReviewMsg struct {
	data   bootMsg
	result *setup.MarketplaceReviewResult
	err    error
}

type baselineCreateMsg struct {
	data    bootMsg
	created []string
	err     error
}

type rollbackPreviewMsg struct {
	review *rollbackReview
	err    error
}

type rollbackApplyMsg struct {
	data         bootMsg
	summary      types.ApplySummary
	restorePoint string
	verify       string
	err          error
}

type undoPreviewMsg struct {
	plan          *timelineundo.Plan
	corruptEvents []store.TimelineCorruptEvent
	err           error
}

// App is the Bubble Tea root model for the Gandalf global setup workspace.
type App struct {
	runtime types.RuntimeOptions
	width   int
	height  int

	ready   bool
	errText string

	evidence        []types.DiscoveredItem
	inventory       []setup.InventoryItem
	timelineEntries []types.TimelineEntry
	corruptEvents   []store.TimelineCorruptEvent
	snapshotRefs    []snapshotRef
	baselineStatus  baseline.Status

	screen        Screen
	selectedAgent *types.AgentID
	keymapVisible bool

	inventoryCursor    int
	inventoryFocus     bool
	activeSetupTab     SetupConsoleTab
	setupSearchFocused bool
	setupConsole       setupConsoleState
	timelineCursor     int
	snapshotCursor     int
	environments       environmentState

	undoPlan                   *timelineundo.Plan
	status                     views.StatusLine
	pendingSave                *saveReview
	pendingAction              *setup.ActionPlan
	pendingMarketplaceReview   *setup.MarketplaceReviewPlan
	pendingMarketplaceRollback *setup.ActionPlan
	marketplaceReviewResult    *setup.MarketplaceReviewResult
	marketplaceInstallResult   *setup.ActionPlan
	skillViewer                *skillMarkdownViewerState
	rollbackReview             *rollbackReview

	actionExecutor      func(context.Context, setup.ActionPlan) error
	marketplaceRunner   setup.CommandRunner
	restorePointCreator func(types.AgentID) (string, error)
	restoreExecutor     restore.RestoreExecutor
	now                 func() time.Time
}

type snapshotRef struct {
	Name      string
	Agent     types.AgentID
	CreatedAt string
}

type rollbackReview struct {
	SnapshotName     string
	Agent            types.AgentID
	Plan             *types.RestorePlan
	Items            []types.RestoreItem
	UnsupportedItems []types.UnsupportedPlanItem
}

type saveReview struct {
	Agent *types.AgentID
}

type setupConsoleState struct {
	tabs            map[SetupConsoleTab]*setupConsoleTabState
	expandedSources map[string]bool
	expandedRows    map[SetupConsoleTab]string
	expandedMCPTool string
	rowsViewport    viewport.Model
}

type setupConsoleTabState struct {
	cursor      int
	search      string
	searchInput textinput.Model
}

type skillMarkdownViewerState struct {
	title       string
	agentLabel  string
	objectKind  string
	status      string
	sourcePath  string
	content     string
	rendered    string
	errorText   string
	viewport    viewport.Model
	renderWidth int
}

// NewApp creates a TUI app bound to engine runtime options.
func NewApp(runtime types.RuntimeOptions) *App {
	setupState := newSetupConsoleState()
	return &App{
		runtime:           runtime,
		screen:            ScreenHome,
		inventoryFocus:    true,
		activeSetupTab:    SetupConsoleTabHooks,
		setupConsole:      setupState,
		marketplaceRunner: nativeSetupCommandRunner{homeDir: runtime.HomeDir},
		now:               time.Now,
	}
}

func newSetupConsoleState() setupConsoleState {
	state := setupConsoleState{
		tabs:            make(map[SetupConsoleTab]*setupConsoleTabState, len(SetupConsoleTabs)),
		expandedSources: make(map[string]bool),
		expandedRows:    make(map[SetupConsoleTab]string),
		rowsViewport:    viewport.New(0, 0),
	}
	for _, tab := range SetupConsoleTabs {
		state.tabs[tab] = &setupConsoleTabState{searchInput: newSetupSearchInput()}
	}
	return state
}

func newSetupSearchInput() textinput.Model {
	searchInput := textinput.New()
	searchInput.Prompt = "/ "
	searchInput.Placeholder = "search"
	searchInput.CharLimit = 120
	return searchInput
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return a.loadData
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = typed.Width
		a.height = typed.Height
		return a, nil

	case tea.KeyMsg:
		if a.screen == ScreenInventory && a.skillViewer != nil {
			if cmd, handled := a.handleSkillViewerKey(typed); handled {
				return a, cmd
			}
		}
		if a.screen == ScreenInventory && a.setupSearchFocused {
			if cmd, handled := a.handleSetupSearchKey(typed); handled {
				return a, cmd
			}
		}
		if cmd, quit := a.handleKey(typed); quit {
			return a, tea.Quit
		} else if cmd != nil {
			return a, cmd
		}
		return a, nil

	case bootMsg:
		if typed.err != nil {
			a.errText = typed.err.Error()
			return a, nil
		}
		a.ready = true
		a.applyWorkspaceData(bootMsg(typed))
		a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
		return a, nil

	case rescanMsg:
		if typed.err != nil {
			a.setStatus(views.StatusError, typed.err.Error())
			return a, nil
		}
		a.applyWorkspaceData(bootMsg(typed))
		a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
		a.undoPlan = nil
		a.pendingSave = nil
		a.pendingAction = nil
		a.pendingMarketplaceReview = nil
		a.pendingMarketplaceRollback = nil
		a.marketplaceReviewResult = nil
		a.setStatus(views.StatusSuccess, "Rescanned global setup.")
		return a, nil

	case setupActionMsg:
		if typed.err != nil {
			if typed.data.err == nil && typed.data.evidence != nil {
				a.applyWorkspaceData(typed.data)
				a.clampSetupConsoleState()
			}
			if typed.plan != nil {
				a.pendingAction = nil
			}
			if typed.rolledBack {
				a.pendingMarketplaceRollback = nil
				a.marketplaceInstallResult = nil
			}
			a.setStatus(views.StatusError, typed.err.Error())
			return a, nil
		}
		if typed.data.err != nil {
			a.pendingAction = nil
			a.setStatus(views.StatusError, "Applied setup action, but failed to rescan: "+typed.data.err.Error())
			return a, nil
		}
		a.applyWorkspaceData(typed.data)
		a.clampSetupConsoleState()
		a.pendingAction = nil
		a.marketplaceReviewResult = nil
		if typed.installed && typed.plan != nil && typed.plan.MarketplaceInstall != nil {
			plan := *typed.plan
			a.marketplaceInstallResult = &plan
			a.setStatus(views.StatusSuccess, fmt.Sprintf("Installed %s, rescanned, and verified. Press u to roll back.", plan.MarketplaceInstall.Selector))
		} else if typed.rolledBack {
			a.pendingMarketplaceRollback = nil
			a.marketplaceInstallResult = nil
			a.setStatus(views.StatusSuccess, "Rolled back marketplace install, rescanned, and verified.")
		} else {
			a.setStatus(views.StatusSuccess, "Applied setup action and rescanned global setup.")
		}
		return a, nil

	case marketplaceReviewMsg:
		if typed.err != nil {
			if typed.data.err == nil && typed.data.evidence != nil {
				a.applyWorkspaceData(typed.data)
			}
			a.pendingMarketplaceReview = nil
			a.marketplaceReviewResult = nil
			a.setStatus(views.StatusError, typed.err.Error())
			return a, nil
		}
		if typed.data.err != nil {
			a.setStatus(views.StatusError, "Reviewed marketplace guidance, but failed to refresh setup data: "+typed.data.err.Error())
			a.pendingMarketplaceReview = nil
			a.marketplaceReviewResult = typed.result
			return a, nil
		}
		a.applyWorkspaceData(typed.data)
		a.clampSetupConsoleState()
		a.pendingMarketplaceReview = nil
		a.marketplaceReviewResult = typed.result
		if typed.result != nil {
			a.setStatus(views.StatusInfo, fmt.Sprintf("Reviewed marketplace guidance for %s. No files changed.", typed.result.Plan.EntryName))
		} else {
			a.setStatus(views.StatusInfo, "Reviewed marketplace guidance. No files changed.")
		}
		return a, nil

	case baselineCreateMsg:
		if typed.err != nil {
			if len(typed.created) > 0 {
				if typed.data.err == nil {
					a.applyWorkspaceData(typed.data)
				}
				a.pendingSave = nil
				a.setStatus(views.StatusError, fmt.Sprintf("Created %d save(s), then failed: %v", len(typed.created), typed.err))
			} else {
				a.setStatus(views.StatusError, typed.err.Error())
			}
			return a, nil
		}
		if typed.data.err != nil {
			a.pendingSave = nil
			a.setStatus(views.StatusError, "Created save, but failed to rescan: "+typed.data.err.Error())
			return a, nil
		}
		a.applyWorkspaceData(typed.data)
		a.pendingSave = nil
		a.screen = ScreenHome
		if len(typed.created) == 0 {
			a.setStatus(views.StatusInfo, "Supported saves already exist.")
		} else {
			label := "save"
			if len(typed.created) != 1 {
				label = "saves"
			}
			a.setStatus(views.StatusSuccess, fmt.Sprintf("Created %d %s from current setup.", len(typed.created), label))
		}
		return a, nil

	case rollbackPreviewMsg:
		a.rollbackReview = nil
		a.clearStatus()
		if typed.err != nil {
			a.setStatus(views.StatusError, typed.err.Error())
			return a, nil
		}
		a.rollbackReview = typed.review
		return a, nil

	case rollbackApplyMsg:
		if typed.err != nil {
			a.setStatus(views.StatusError, typed.err.Error())
			return a, nil
		}
		if typed.data.err != nil {
			a.rollbackReview = nil
			a.setStatus(views.StatusError, "Applied restore, but failed to rescan: "+typed.data.err.Error())
			return a, nil
		}
		a.applyWorkspaceData(typed.data)
		a.rollbackReview = nil
		a.setStatus(views.StatusSuccess, fmt.Sprintf("Restored from save. Safety save: %s. %s", typed.restorePoint, typed.verify))
		return a, nil

	case undoPreviewMsg:
		a.undoPlan = nil
		a.clearStatus()
		a.corruptEvents = typed.corruptEvents
		if typed.err != nil {
			a.setStatus(views.StatusWarn, typed.err.Error())
			return a, nil
		}
		a.undoPlan = typed.plan
		return a, nil
	}

	return a, nil
}

// View implements tea.Model.
func (a *App) View() string {
	if a.width == 0 {
		a.width = 100
	}
	if a.height == 0 {
		a.height = 28
	}

	contentWidth := a.width
	if contentWidth < 1 {
		contentWidth = 1
	}
	contentHeight := a.height

	if !a.ready {
		if a.errText != "" {
			return views.RenderBootError("Failed to load workspace.", a.errText, contentWidth, contentHeight)
		}
		return views.RenderLoading("Loading Gandalf global setup workspace...", contentWidth, contentHeight)
	}

	header := views.RenderHeader(a.headerView(), contentWidth)
	status := views.RenderStatusLine(a.statusLine(), contentWidth)

	// Reserve header (1) + divider (1) for the body height.
	headerLines := strings.Count(header, "\n") + 1
	bodyHeight := contentHeight - headerLines - 1
	if status != "" {
		bodyHeight--
	}
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	sidebar := a.sidebarItems()
	var body string
	if contentWidth >= views.SidebarCollapseWidth {
		screenWidth := contentWidth - views.SidebarWidth - 1
		if screenWidth < 20 {
			screenWidth = 20
		}
		screen := a.renderContent(screenWidth, bodyHeight)
		body = views.JoinSidebar(views.RenderSidebar(sidebar, bodyHeight), screen, screenWidth)
	} else {
		strip := views.RenderSidebarStrip(sidebar, contentWidth)
		screenHeight := bodyHeight - 1
		if screenHeight < 1 {
			screenHeight = 1
		}
		body = strip + "\n" + a.renderContent(contentWidth, screenHeight)
	}

	if a.keymapVisible {
		body = views.RenderKeymapOverlay(body, a.keymapSections(), contentWidth, bodyHeight)
	}

	return views.RenderFrame(header, body, status, contentWidth, contentHeight)
}

// statusLine merges the three legacy feedback channels into one frame-level
// status model so feedback is visible on every screen.
func (a *App) statusLine() views.StatusLine {
	return a.status
}

func (a *App) setStatus(level views.StatusLevel, text string) {
	a.status = views.StatusLine{Level: level, Text: text}
}

func (a *App) clearStatus() {
	a.status = views.StatusLine{}
}

func (a *App) sidebarItems() []views.SidebarItem {
	items := make([]views.SidebarItem, 0, len(Destinations))
	for _, dest := range Destinations {
		items = append(items, views.SidebarItem{
			Key:    dest.Key,
			Label:  dest.Label,
			Active: dest.Screen == a.screen,
		})
	}
	return items
}

func (a *App) keymapSections() []views.KeymapSection {
	sections := []views.KeymapSection{
		{
			Title: "Navigate",
			Keys: []views.KeymapEntry{
				{Key: "1-5", Help: "jump to Home / Console / Changes / Timeline / Saves"},
				{Key: "esc", Help: "close overlay, else back to Home"},
				{Key: "↑↓ / jk", Help: "move selection"},
				{Key: "tab", Help: "cycle focus / tabs"},
				{Key: "enter", Help: "primary action on selection"},
			},
		},
		{
			Title: "Global",
			Keys: []views.KeymapEntry{
				{Key: "/", Help: "search (Console)"},
				{Key: "r", Help: "rescan setup"},
				{Key: "?", Help: "toggle this keymap"},
				{Key: "q", Help: "quit"},
			},
		},
	}
	var contextual []views.KeymapEntry
	switch a.screen {
	case ScreenHome:
		contextual = []views.KeymapEntry{
			{Key: "enter", Help: "review changes"},
			{Key: "s", Help: "save setup (creates missing saves)"},
		}
	case ScreenInventory:
		contextual = []views.KeymapEntry{
			{Key: "enter", Help: "expand / review / open"},
			{Key: "space", Help: "toggle / expand"},
			{Key: "u", Help: "roll back last marketplace install"},
		}
	case ScreenEnvironments:
		contextual = []views.KeymapEntry{
			{Key: "enter", Help: "restore focused agent (review first)"},
			{Key: "s", Help: "save focused agent setup"},
			{Key: "n / p", Help: "next / previous hunk"},
			{Key: "v", Help: "toggle diff layout"},
			{Key: "pgup/pgdn", Help: "scroll diff"},
		}
	case ScreenTimeline:
		contextual = []views.KeymapEntry{
			{Key: "u", Help: "preview undo (writes nothing)"},
		}
	case ScreenSnapshots:
		contextual = []views.KeymapEntry{
			{Key: "enter", Help: "review restore from save"},
			{Key: "s", Help: "save setup (creates missing saves)"},
		}
	}
	if len(contextual) > 0 {
		sections = append(sections, views.KeymapSection{Title: "This screen", Keys: contextual})
	}
	return sections
}

func (a *App) headerView() views.HeaderView {
	scope := "~/"
	if a.runtime.HomeDir != "" {
		scope = a.runtime.HomeDir
	}
	chips := make([]views.HeaderChip, 0)
	for _, chip := range BuildHeaderChips(a.baselineStatus) {
		chips = append(chips, views.HeaderChip{
			AgentMarker: chip.AgentMarker,
			State:       chip.State,
			Detail:      chip.Detail,
			ChangeCount: chip.ChangeCount,
			SourceDrift: chip.SourceDrift,
		})
	}
	return views.HeaderView{
		Title: "Gandalf",
		Scope: scope,
		Chips: chips,
	}
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	key := msg.String()
	if key == "ctrl+c" || key == "q" {
		return nil, true
	}
	if a.keymapVisible {
		a.keymapVisible = false
		return nil, false
	}
	if key == "?" {
		a.keymapVisible = true
		return nil, false
	}
	if cmd, handled := a.handleBlockingReviewKey(key); handled {
		return cmd, false
	}
	switch key {
	case "esc":
		if a.dismissTopOverlay() {
			return nil, false
		}
		if a.screen != ScreenHome {
			a.navigateTo(ScreenHome)
		}
		return nil, false
	case "1", "2", "3", "4", "5":
		if dest, ok := DestinationForKey(key); ok {
			a.navigateTo(dest.Screen)
		}
		return nil, false
	case "/":
		if a.screen == ScreenInventory && !a.hasPendingSetupReview() {
			a.setupSearchFocused = true
			state := a.activeSetupTabState()
			state.searchInput.Focus()
			a.mirrorActiveSetupTabState()
			return nil, false
		}
	case "tab":
		if a.screen == ScreenEnvironments {
			a.environments.cycleFocus()
			return nil, false
		}
		if a.screen == ScreenInventory && !a.hasPendingSetupReview() {
			a.moveSetupTab(1)
		}
	case "shift+tab":
		if a.screen == ScreenEnvironments {
			a.environments.cycleFocus()
			return nil, false
		}
		if a.screen == ScreenInventory && !a.hasPendingSetupReview() {
			a.moveSetupTab(-1)
		}
	case "r":
		return func() tea.Msg {
			data := a.fetchWorkspaceData()
			return rescanMsg(data)
		}, false
	case "v":
		if a.screen == ScreenEnvironments {
			a.environments.toggleMode()
			return nil, false
		}
	case "n":
		if a.screen == ScreenEnvironments {
			model := a.currentEnvironmentsViewModel()
			a.environments.moveHunk(model, 1, a.environmentDiffViewportHeight())
			return nil, false
		}
	case "p":
		if a.screen == ScreenEnvironments {
			model := a.currentEnvironmentsViewModel()
			a.environments.moveHunk(model, -1, a.environmentDiffViewportHeight())
			return nil, false
		}
	case "s":
		if a.screen == ScreenEnvironments {
			return a.saveFocusedEnvironment(), false
		}
		if (a.screen == ScreenHome || a.screen == ScreenSnapshots) && !a.hasPendingSetupReview() {
			a.pendingSave = &saveReview{}
			a.clearStatus()
			return nil, false
		}
	case "u":
		if a.screen == ScreenInventory && a.marketplaceInstallResult != nil && !a.hasPendingSetupReview() {
			plan := *a.marketplaceInstallResult
			a.pendingMarketplaceRollback = &plan
			a.clearStatus()
			return nil, false
		}
		if a.screen != ScreenTimeline {
			return nil, false
		}
		entries := a.filteredTimeline()
		if len(entries) == 0 {
			a.setStatus(views.StatusWarn, "No timeline entry selected.")
			return nil, false
		}
		selected := entries[a.timelineCursor]
		return func() tea.Msg {
			var corrupt []store.TimelineCorruptEvent
			plan, err := timelineundo.BuildPlan(a.runtime.StoreDir, selected.ID, timelineundo.BuildOptions{
				OnCorruptEntry: func(event store.TimelineCorruptEvent) {
					corrupt = append(corrupt, event)
				},
			})
			if err != nil {
				return undoPreviewMsg{err: err}
			}
			return undoPreviewMsg{plan: plan, corruptEvents: corrupt}
		}, false
	case "up", "k":
		if a.screen == ScreenInventory && a.inventoryFocus && !a.hasPendingSetupReview() {
			a.moveInventoryCursor(-1)
			return nil, false
		}
		if a.screen == ScreenSnapshots {
			a.moveSnapshotCursor(-1)
			return nil, false
		}
		if a.screen == ScreenEnvironments {
			a.moveEnvironmentSelection(-1)
			return nil, false
		}
		if a.screen == ScreenTimeline {
			a.moveTimelineCursor(-1)
			return nil, false
		}
	case "down", "j":
		if a.screen == ScreenInventory && a.inventoryFocus && !a.hasPendingSetupReview() {
			a.moveInventoryCursor(1)
			return nil, false
		}
		if a.screen == ScreenSnapshots {
			a.moveSnapshotCursor(1)
			return nil, false
		}
		if a.screen == ScreenEnvironments {
			a.moveEnvironmentSelection(1)
			return nil, false
		}
		if a.screen == ScreenTimeline {
			a.moveTimelineCursor(1)
			return nil, false
		}
	case "pgup":
		if a.screen == ScreenEnvironments {
			a.environments.pageDiff(a.currentEnvironmentsViewModel(), a.environmentDiffViewportHeight(), -1)
			return nil, false
		}
	case "pgdown":
		if a.screen == ScreenEnvironments {
			a.environments.pageDiff(a.currentEnvironmentsViewModel(), a.environmentDiffViewportHeight(), 1)
			return nil, false
		}
	case "left", "h":
		if a.screen == ScreenTimeline {
			a.moveTimelineCursor(-1)
		}
	case "right", "l":
		if a.screen == ScreenTimeline {
			a.moveTimelineCursor(1)
		}
	case "enter":
		if a.screen == ScreenHome {
			a.focusFirstChangedEnvironment()
			a.navigateTo(ScreenEnvironments)
			return nil, false
		}
		if a.screen == ScreenEnvironments && a.environments.focus == EnvironmentFocusAgents {
			return a.restoreFocusedEnvironment(), false
		}
		if a.screen == ScreenSnapshots {
			return a.handleSnapshotEnter(), false
		}
		if a.screen == ScreenInventory && a.inventoryFocus {
			return a.handleInventoryEnter(), false
		}
	case " ", "space":
		if a.screen == ScreenInventory && a.inventoryFocus && !a.hasPendingSetupReview() {
			return a.handleSetupToggle(), false
		}
	}
	return nil, false
}

func (a *App) handleBlockingReviewKey(key string) (tea.Cmd, bool) {
	if a.pendingSave == nil &&
		a.pendingMarketplaceRollback == nil &&
		a.pendingAction == nil &&
		a.pendingMarketplaceReview == nil &&
		a.rollbackReview == nil {
		return nil, false
	}
	switch key {
	case "esc":
		a.dismissTopOverlay()
		return nil, true
	case "enter":
		switch {
		case a.pendingSave != nil:
			return a.confirmSaveReview(), true
		case a.pendingMarketplaceRollback != nil:
			return a.rollbackMarketplaceInstall(), true
		case a.pendingAction != nil, a.pendingMarketplaceReview != nil:
			return a.handleInventoryEnter(), true
		case a.rollbackReview != nil:
			return a.handleSnapshotEnter(), true
		}
	}
	return nil, true
}

// navigateTo switches the active destination, resetting transient screen state.
func (a *App) navigateTo(screen Screen) {
	if a.screen == screen {
		return
	}
	a.screen = screen
	a.clearStatus()
	switch screen {
	case ScreenTimeline:
		a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
	case ScreenEnvironments:
		a.environments.clampAgents(a.baselineStatus)
	case ScreenInventory:
		a.undoPlan = nil
	}
}

// dismissTopOverlay closes the top-most open overlay and reports whether one
// was open. esc only ever closes one layer per press.
func (a *App) dismissTopOverlay() bool {
	if a.skillViewer != nil {
		a.skillViewer = nil
		a.clearStatus()
		return true
	}
	if a.pendingSave != nil {
		a.pendingSave = nil
		a.clearStatus()
		return true
	}
	if a.pendingAction != nil {
		a.pendingAction = nil
		a.clearStatus()
		return true
	}
	if a.pendingMarketplaceRollback != nil {
		a.pendingMarketplaceRollback = nil
		a.clearStatus()
		return true
	}
	if a.pendingMarketplaceReview != nil || a.marketplaceReviewResult != nil {
		a.pendingMarketplaceReview = nil
		a.marketplaceReviewResult = nil
		a.clearStatus()
		return true
	}
	if a.rollbackReview != nil {
		a.rollbackReview = nil
		a.clearStatus()
		return true
	}
	return false
}

func (a *App) focusFirstChangedEnvironment() {
	a.environments.ensure()
	for i, status := range a.baselineStatus.Agents {
		if status.HasBaseline && status.SemanticChangeCount > 0 {
			a.environments.agentCursor = i
			return
		}
	}
	for i, status := range a.baselineStatus.Agents {
		if status.HasBaseline {
			a.environments.agentCursor = i
			return
		}
	}
	a.environments.agentCursor = 0
}

func (a *App) handleSetupSearchKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "q":
		return nil, false
	case "esc":
		a.setupSearchFocused = false
		a.activeSetupTabState().searchInput.Blur()
		a.mirrorActiveSetupTabState()
		return nil, true
	case "enter":
		a.setupSearchFocused = false
		a.activeSetupTabState().searchInput.Blur()
		a.mirrorActiveSetupTabState()
		return nil, true
	}
	var cmd tea.Cmd
	tabState := a.activeSetupTabState()
	tabState.searchInput, cmd = tabState.searchInput.Update(msg)
	tabState.search = tabState.searchInput.Value()
	a.mirrorActiveSetupTabState()
	a.setInventoryCursor(clampIndex(a.activeSetupTabState().cursor, len(a.currentSetupConsoleViewModel().Rows)))
	a.clearStatus()
	return cmd, true
}

func (a *App) handleSkillViewerKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if a.skillViewer == nil {
		return nil, false
	}
	switch msg.String() {
	case "ctrl+c", "q":
		return nil, false
	case "esc":
		a.skillViewer = nil
		return nil, true
	}
	var cmd tea.Cmd
	a.skillViewer.viewport, cmd = a.skillViewer.viewport.Update(msg)
	return cmd, true
}

func (a *App) handleInventoryEnter() tea.Cmd {
	if a.pendingAction != nil {
		plan := *a.pendingAction
		if plan.MarketplaceInstall != nil {
			return a.executeMarketplaceInstall(plan)
		}
		executor := a.actionExecutor
		if executor == nil {
			executor = a.defaultSetupActionExecutor
		}
		return func() tea.Msg {
			if err := executor(context.Background(), plan); err != nil {
				return setupActionMsg{err: err}
			}
			return setupActionMsg{data: a.fetchWorkspaceData()}
		}
	}
	if a.pendingMarketplaceReview != nil {
		plan := *a.pendingMarketplaceReview
		return func() tea.Msg {
			data := a.fetchWorkspaceData()
			if data.err != nil {
				return marketplaceReviewMsg{data: data, err: data.err}
			}
			result, err := setup.ExecuteMarketplaceReviewPlan(plan, setup.BuildMarketplace(data.evidence))
			if err != nil {
				return marketplaceReviewMsg{data: data, err: err}
			}
			return marketplaceReviewMsg{data: data, result: &result}
		}
	}
	if a.marketplaceReviewResult != nil && normalizeSetupConsoleTab(a.activeSetupTab) == SetupConsoleTabMarketplace {
		a.handleMarketplaceInstall()
		return nil
	}

	if normalizeSetupConsoleTab(a.activeSetupTab) == SetupConsoleTabMarketplace {
		return a.handleMarketplaceEnter()
	}
	if normalizeSetupConsoleTab(a.activeSetupTab) == SetupConsoleTabMCPServers {
		a.handleMCPEnter()
		return nil
	}
	if normalizeSetupConsoleTab(a.activeSetupTab) == SetupConsoleTabSkills {
		a.handleSkillEnter()
		return nil
	}
	if !a.selectedSetupInventoryExpanded() {
		a.expandSelectedSetupInventory()
		return nil
	}

	inventory := a.currentInventory()
	if len(inventory) == 0 {
		a.setStatus(views.StatusError, "No setup item selected.")
		return nil
	}
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	action, ok := firstAvailableInventoryAction(item)
	if !ok {
		a.setStatus(views.StatusError, "No supported action is available for this setup item.")
		return nil
	}
	plan := setup.PlanItemAction(item, action)
	if !plan.Available {
		a.setStatus(views.StatusError, plan.UnavailableReason)
		return nil
	}
	a.pendingAction = &plan
	a.clearStatus()
	return nil
}

func (a *App) handleMarketplaceInstall() {
	model := a.currentSetupConsoleViewModel()
	if len(model.Rows) == 0 {
		a.setStatus(views.StatusError, "No marketplace entry selected.")
		return
	}
	row := model.Rows[clampIndex(a.activeSetupTabState().cursor, len(model.Rows))]
	if row.RowKind != SetupConsoleRowMarketplaceEntry {
		a.setStatus(views.StatusError, "Select a marketplace plugin entry to install.")
		return
	}
	source, entry, ok := a.marketplaceEntryByID(row.ID)
	if !ok {
		a.setStatus(views.StatusError, "Marketplace entry was not found in current scan data.")
		return
	}
	plan := setup.PlanMarketplaceInstall(source, entry)
	if !plan.Available {
		a.setStatus(views.StatusError, plan.UnavailableReason)
		return
	}
	a.pendingAction = &plan
	a.marketplaceReviewResult = nil
	a.clearStatus()
}

func (a *App) executeMarketplaceInstall(plan setup.ActionPlan) tea.Cmd {
	return func() tea.Msg {
		before := a.fetchWorkspaceData()
		if before.err != nil {
			return setupActionMsg{err: before.err, plan: &plan}
		}
		if _, err := setup.ExecuteMarketplaceInstallPlan(context.Background(), plan, setup.BuildMarketplace(before.evidence), a.marketplaceRunner); err != nil {
			failed := err
			afterFailure := a.fetchWorkspaceData()
			if afterFailure.err != nil {
				return setupActionMsg{data: afterFailure, err: fmt.Errorf("install command failed (%v); rollback unavailable because rescan failed: %w", failed, afterFailure.err), plan: &plan}
			}
			if verifyErr := setup.VerifyMarketplaceInstallPlan(plan, setup.BuildMarketplace(afterFailure.evidence)); verifyErr != nil {
				return setupActionMsg{data: afterFailure, err: failed, plan: &plan}
			}
			if _, rollbackErr := setup.RollbackMarketplaceInstallPlan(context.Background(), plan, setup.BuildMarketplace(afterFailure.evidence), a.marketplaceRunner); rollbackErr != nil {
				return setupActionMsg{data: afterFailure, err: fmt.Errorf("install command failed (%v) after changing state; rollback unavailable: %w", failed, rollbackErr), plan: &plan}
			}
			rolledBack := a.fetchWorkspaceData()
			if rolledBack.err != nil {
				return setupActionMsg{data: rolledBack, err: fmt.Errorf("install command failed (%v); rollback ran but rescan failed: %w", failed, rolledBack.err), plan: &plan}
			}
			if rollbackErr := setup.VerifyMarketplaceInstallRollback(plan, setup.BuildMarketplace(rolledBack.evidence)); rollbackErr != nil {
				return setupActionMsg{data: rolledBack, err: fmt.Errorf("install command failed (%v); rollback verification failed: %w", failed, rollbackErr), plan: &plan}
			}
			return setupActionMsg{data: rolledBack, err: fmt.Errorf("install command failed: %v; partial change was rolled back and verified", failed), plan: &plan, rolledBack: true}
		}
		after := a.fetchWorkspaceData()
		if after.err == nil {
			if err := setup.VerifyMarketplaceInstallPlan(plan, setup.BuildMarketplace(after.evidence)); err == nil {
				return setupActionMsg{data: after, plan: &plan, installed: true}
			}
		}
		fresh := before
		if after.err == nil {
			fresh = after
		}
		verifyErr := after.err
		if verifyErr == nil {
			verifyErr = setup.VerifyMarketplaceInstallPlan(plan, setup.BuildMarketplace(after.evidence))
		}
		if _, err := setup.RollbackMarketplaceInstallPlan(context.Background(), plan, setup.BuildMarketplace(fresh.evidence), a.marketplaceRunner); err != nil {
			return setupActionMsg{data: after, err: fmt.Errorf("install verification failed (%v); rollback unavailable: %w", verifyErr, err), plan: &plan}
		}
		rolledBack := a.fetchWorkspaceData()
		if rolledBack.err != nil {
			return setupActionMsg{data: rolledBack, err: fmt.Errorf("install verification failed (%v); rollback ran but rescan failed: %w", verifyErr, rolledBack.err), plan: &plan}
		}
		if err := setup.VerifyMarketplaceInstallRollback(plan, setup.BuildMarketplace(rolledBack.evidence)); err != nil {
			return setupActionMsg{data: rolledBack, err: fmt.Errorf("install verification failed (%v); rollback verification failed: %w", verifyErr, err), plan: &plan}
		}
		return setupActionMsg{data: rolledBack, err: fmt.Errorf("install verification failed: %v; rollback was verified", verifyErr), plan: &plan, rolledBack: true}
	}
}

func (a *App) rollbackMarketplaceInstall() tea.Cmd {
	if a.pendingMarketplaceRollback == nil {
		a.setStatus(views.StatusError, "No marketplace rollback is pending.")
		return nil
	}
	plan := *a.pendingMarketplaceRollback
	return func() tea.Msg {
		before := a.fetchWorkspaceData()
		if before.err != nil {
			return setupActionMsg{err: before.err, plan: &plan}
		}
		if _, err := setup.RollbackMarketplaceInstallPlan(context.Background(), plan, setup.BuildMarketplace(before.evidence), a.marketplaceRunner); err != nil {
			return setupActionMsg{data: before, err: err, plan: &plan}
		}
		after := a.fetchWorkspaceData()
		if after.err != nil {
			return setupActionMsg{data: after, err: fmt.Errorf("rollback ran but rescan failed: %w", after.err), plan: &plan}
		}
		if err := setup.VerifyMarketplaceInstallRollback(plan, setup.BuildMarketplace(after.evidence)); err != nil {
			return setupActionMsg{data: after, err: err, plan: &plan}
		}
		return setupActionMsg{data: after, plan: &plan, rolledBack: true}
	}
}

func (a *App) openSelectedSkillViewer() {
	inventory := a.currentInventory()
	if len(inventory) == 0 {
		a.setStatus(views.StatusError, "No skill selected.")
		return
	}
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	viewer := a.buildSkillMarkdownViewer(item)
	a.skillViewer = &viewer
	a.clearStatus()
}

func (a *App) handleSkillEnter() {
	inventory := a.currentInventory()
	if len(inventory) == 0 {
		a.setStatus(views.StatusError, "No skill selected.")
		return
	}
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	if a.expandedSetupRowID(SetupConsoleTabSkills) == item.ID {
		a.openSelectedSkillViewer()
		return
	}
	a.setExpandedSetupRowID(SetupConsoleTabSkills, item.ID)
	a.clearStatus()
}

func (a *App) handleMCPEnter() {
	model := a.currentSetupConsoleViewModel()
	if len(model.Rows) == 0 {
		a.setStatus(views.StatusError, "No MCP server selected.")
		return
	}
	row := model.Rows[clampIndex(a.inventoryCursor, len(model.Rows))]
	if row.RowKind == SetupConsoleRowMCPTool {
		a.toggleMCPTool(row.ID)
		return
	}
	if row.RowKind == SetupConsoleRowInventory {
		if a.expandedSetupRowID(SetupConsoleTabMCPServers) == row.ID {
			a.setExpandedSetupRowID(SetupConsoleTabMCPServers, "")
			a.setupConsole.expandedMCPTool = ""
		} else {
			a.setExpandedSetupRowID(SetupConsoleTabMCPServers, row.ID)
			a.setupConsole.expandedMCPTool = ""
		}
		a.clearStatus()
	}
}

func (a *App) buildSkillMarkdownViewer(item setup.InventoryItem) skillMarkdownViewerState {
	content, resolvedPath, err := a.readSkillMarkdown(item)
	viewer := skillMarkdownViewerState{
		title:      item.Name,
		agentLabel: FormatAgentLabel(item.Agent),
		objectKind: formatInventoryObjectKind(item),
		status:     setupInventoryStatus(item),
		sourcePath: resolvedPath,
		content:    content,
		viewport:   viewport.New(0, 0),
	}
	if viewer.sourcePath == "" {
		viewer.sourcePath = item.SourcePath
	}
	if err != nil {
		viewer.errorText = err.Error()
		viewer.rendered = viewer.errorText
		viewer.viewport.SetContent(viewer.rendered)
	}
	return viewer
}

func (a *App) readSkillMarkdown(item setup.InventoryItem) (string, string, error) {
	if item.ObjectKind != setup.ObjectSkill {
		return "", item.SourcePath, fmt.Errorf("Selected setup item is not a skill.")
	}

	entrypoint := strings.TrimSpace(item.Entrypoint)
	if entrypoint == "" {
		entrypoint = "SKILL.md"
	}
	if filepath.IsAbs(entrypoint) || strings.Contains(filepath.ToSlash(entrypoint), "/") {
		return "", item.SourcePath, fmt.Errorf("Skill markdown entrypoint is unsupported: %s", entrypoint)
	}

	basePath, ok := a.resolveSetupSourcePath(item.SourcePath)
	if !ok {
		return "", item.SourcePath, fmt.Errorf("Skill markdown path is outside readable global setup roots.")
	}
	skillPath := basePath
	if !strings.EqualFold(filepath.Base(skillPath), entrypoint) {
		skillPath = filepath.Join(skillPath, entrypoint)
	}
	if !pathWithinRoot(skillPath, a.runtime.HomeDir) {
		return "", item.SourcePath, fmt.Errorf("Skill markdown path is outside readable global setup roots.")
	}
	displayPath := displaySetupPath(skillPath, a.runtime.HomeDir)
	info, err := os.Lstat(skillPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", displayPath, fmt.Errorf("Skill markdown entrypoint was not found.")
		}
		return "", displayPath, fmt.Errorf("Skill markdown entrypoint is unreadable: %v", err)
	}
	readPath := skillPath
	resolvedPath, err := filepath.EvalSymlinks(skillPath)
	if err != nil {
		return "", displayPath, fmt.Errorf("Skill markdown symlink target is unreadable: %v", err)
	}
	if !pathWithinRootOrResolved(resolvedPath, a.runtime.HomeDir) {
		return "", displayPath, fmt.Errorf("Skill markdown path is outside readable global setup roots.")
	}
	readPath = resolvedPath
	if info.Mode()&os.ModeSymlink != 0 {
		displayPath = displayPath + " -> " + displaySetupPath(readPath, a.runtime.HomeDir)
	}
	if readPath != skillPath {
		info, err = os.Stat(readPath)
		if err != nil {
			return "", displayPath, fmt.Errorf("Skill markdown symlink target is unreadable: %v", err)
		}
	}
	if !info.Mode().IsRegular() {
		return "", displayPath, fmt.Errorf("Skill markdown entrypoint is not a regular file.")
	}
	content, err := os.ReadFile(readPath)
	if err != nil {
		return "", displayPath, fmt.Errorf("Skill markdown entrypoint is unreadable: %v", err)
	}
	return string(content), displayPath, nil
}

func (a *App) resolveSetupSourcePath(sourcePath string) (string, bool) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" || strings.HasPrefix(sourcePath, "<") {
		return "", false
	}
	if strings.HasPrefix(sourcePath, "~/") {
		rel := strings.TrimPrefix(sourcePath, "~/")
		return filepath.Join(a.runtime.HomeDir, filepath.FromSlash(rel)), true
	}
	if filepath.IsAbs(sourcePath) && pathWithinRoot(sourcePath, a.runtime.HomeDir) {
		return filepath.Clean(sourcePath), true
	}
	return "", false
}

func pathWithinRoot(path, root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func pathWithinRootOrResolved(path, root string) bool {
	if pathWithinRoot(path, root) {
		return true
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	return pathWithinRoot(path, resolvedRoot)
}

func displaySetupPath(path, homeDir string) string {
	if displayPath, ok := displayPathWithinRoot(path, homeDir); ok {
		return displayPath
	}
	if resolvedHome, err := filepath.EvalSymlinks(homeDir); err == nil {
		if displayPath, ok := displayPathWithinRoot(path, resolvedHome); ok {
			return displayPath
		}
	}
	return filepath.ToSlash(path)
}

func displayPathWithinRoot(path, root string) (string, bool) {
	if !pathWithinRoot(path, root) {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return "", false
	}
	return "~/" + filepath.ToSlash(rel), true
}

func (a *App) handleMarketplaceEnter() tea.Cmd {
	model := a.currentSetupConsoleViewModel()
	if len(model.Rows) == 0 {
		a.setStatus(views.StatusError, "No marketplace source selected.")
		return nil
	}
	row := model.Rows[clampIndex(a.activeSetupTabState().cursor, len(model.Rows))]
	if row.RowKind == SetupConsoleRowMarketplaceSource {
		a.toggleMarketplaceSource(row.ID)
		return nil
	}
	source, entry, ok := a.marketplaceEntryByID(row.ID)
	if !ok {
		a.setStatus(views.StatusError, "Marketplace entry was not found in current scan data.")
		return nil
	}
	plan := setup.PlanMarketplaceEntryAction(source, entry, setup.MarketplaceActionReview)
	if !plan.Available {
		a.setStatus(views.StatusError, plan.UnavailableReason)
		return nil
	}
	a.pendingMarketplaceReview = &plan
	a.marketplaceReviewResult = nil
	a.clearStatus()
	return nil
}

func (a *App) handleSetupToggle() tea.Cmd {
	activeTab := normalizeSetupConsoleTab(a.activeSetupTab)
	if activeTab == SetupConsoleTabMCPServers {
		model := a.currentSetupConsoleViewModel()
		if len(model.Rows) == 0 {
			return nil
		}
		row := model.Rows[clampIndex(a.inventoryCursor, len(model.Rows))]
		if row.RowKind == SetupConsoleRowMCPTool {
			a.toggleMCPTool(row.ID)
			return nil
		}
		return a.toggleSelectedMCPServer(row.ID)
	}
	if activeTab != SetupConsoleTabMarketplace {
		a.toggleSelectedSetupInventory()
		return nil
	}
	model := a.currentSetupConsoleViewModel()
	if len(model.Rows) == 0 {
		return nil
	}
	row := model.Rows[clampIndex(a.activeSetupTabState().cursor, len(model.Rows))]
	if row.RowKind == SetupConsoleRowMarketplaceSource {
		a.toggleMarketplaceSource(row.ID)
	}
	return nil
}

// toggleSelectedMCPServer opens Review Changes for flipping the selected MCP
// server's enable/disable state. Like every other mutation, the write only
// happens after the user confirms the pending action — never on the toggle
// key itself.
func (a *App) toggleSelectedMCPServer(rowID string) tea.Cmd {
	item, ok := a.inventoryItemByID(rowID)
	if !ok {
		a.setStatus(views.StatusError, "No MCP server selected.")
		return nil
	}
	plan := setup.PlanItemAction(item, setup.ActionToggle)
	if !plan.Available {
		a.setStatus(views.StatusError, plan.UnavailableReason)
		return nil
	}
	a.pendingAction = &plan
	a.clearStatus()
	return nil
}

func (a *App) inventoryItemByID(id string) (setup.InventoryItem, bool) {
	for _, item := range a.currentInventory() {
		if item.ID == id {
			return item, true
		}
	}
	return setup.InventoryItem{}, false
}

func (a *App) selectedSetupInventoryExpanded() bool {
	inventory := a.currentInventory()
	if len(inventory) == 0 {
		return false
	}
	activeTab := normalizeSetupConsoleTab(a.activeSetupTab)
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	return a.expandedSetupRowID(activeTab) == item.ID
}

func (a *App) expandSelectedSetupInventory() {
	inventory := a.currentInventory()
	if len(inventory) == 0 {
		a.setStatus(views.StatusError, "No setup item selected.")
		return
	}
	activeTab := normalizeSetupConsoleTab(a.activeSetupTab)
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	a.setExpandedSetupRowID(activeTab, item.ID)
	a.clearStatus()
}

func (a *App) toggleSelectedSetupInventory() {
	inventory := a.currentInventory()
	if len(inventory) == 0 {
		return
	}
	activeTab := normalizeSetupConsoleTab(a.activeSetupTab)
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	if a.expandedSetupRowID(activeTab) == item.ID {
		a.setExpandedSetupRowID(activeTab, "")
	} else {
		a.setExpandedSetupRowID(activeTab, item.ID)
	}
	a.clearStatus()
}

func (a *App) expandedSetupRowID(tab SetupConsoleTab) string {
	if a.setupConsole.expandedRows == nil {
		return ""
	}
	return a.setupConsole.expandedRows[normalizeSetupConsoleTab(tab)]
}

func (a *App) setExpandedSetupRowID(tab SetupConsoleTab, id string) {
	if a.setupConsole.expandedRows == nil {
		a.setupConsole.expandedRows = make(map[SetupConsoleTab]string)
	}
	tab = normalizeSetupConsoleTab(tab)
	if id == "" {
		delete(a.setupConsole.expandedRows, tab)
		return
	}
	a.setupConsole.expandedRows[tab] = id
}

func (a *App) toggleMCPTool(id string) {
	if a.setupConsole.expandedMCPTool == id {
		a.setupConsole.expandedMCPTool = ""
	} else {
		a.setupConsole.expandedMCPTool = id
	}
	a.clearStatus()
}

func (a *App) currentInventory() []setup.InventoryItem {
	return filterSetupConsoleInventory(a.inventory, normalizeSetupConsoleTab(a.activeSetupTab), a.activeSetupTabState().search)
}

func (a *App) hasPendingSetupReview() bool {
	return a.pendingAction != nil ||
		a.pendingMarketplaceReview != nil ||
		a.pendingMarketplaceRollback != nil
}

func (a *App) marketplaceEntryByID(entryID string) (setup.MarketplaceSource, setup.MarketplaceEntry, bool) {
	for _, source := range setupConsoleMarketplaceSources(setup.BuildMarketplace(a.evidence)) {
		for _, entry := range source.Entries {
			if entry.ID == entryID {
				return source, entry, true
			}
		}
	}
	return setup.MarketplaceSource{}, setup.MarketplaceEntry{}, false
}

func (a *App) applyWorkspaceData(data bootMsg) {
	a.evidence = data.evidence
	a.inventory = setup.BuildInventory(data.evidence)
	a.timelineEntries = data.timelineEntries
	a.corruptEvents = data.corruptEvents
	a.snapshotRefs = data.snapshotRefs
	a.baselineStatus = data.baselineStatus
	a.clampSetupConsoleState()
	a.snapshotCursor = clampIndex(a.snapshotCursor, len(a.snapshotRefs))
}

func (a *App) moveInventoryCursor(delta int) {
	rowCount := len(a.currentSetupConsoleViewModel().Rows)
	if rowCount == 0 {
		a.setInventoryCursor(0)
		return
	}
	next := a.inventoryCursor + delta
	if next < 0 {
		next = rowCount - 1
	}
	if next >= rowCount {
		next = 0
	}
	a.setInventoryCursor(next)
	a.clearStatus()
}

func (a *App) moveSetupTab(delta int) {
	active := normalizeSetupConsoleTab(a.activeSetupTab)
	index := 0
	for i, tab := range SetupConsoleTabs {
		if tab == active {
			index = i
			break
		}
	}
	next := index + delta
	if next < 0 {
		next = len(SetupConsoleTabs) - 1
	}
	if next >= len(SetupConsoleTabs) {
		next = 0
	}
	a.activeSetupTab = SetupConsoleTabs[next]
	a.mirrorActiveSetupTabState()
	a.setInventoryCursor(clampIndex(a.activeSetupTabState().cursor, len(a.currentSetupConsoleViewModel().Rows)))
	a.clearStatus()
}

func (a *App) activeSetupTabState() *setupConsoleTabState {
	active := normalizeSetupConsoleTab(a.activeSetupTab)
	if a.setupConsole.tabs == nil {
		a.setupConsole = newSetupConsoleState()
	}
	state, ok := a.setupConsole.tabs[active]
	if !ok || state == nil {
		state = &setupConsoleTabState{searchInput: newSetupSearchInput()}
		a.setupConsole.tabs[active] = state
	}
	return state
}

func (a *App) setInventoryCursor(cursor int) {
	a.activeSetupTabState().cursor = cursor
	a.inventoryCursor = cursor
}

func (a *App) mirrorActiveSetupTabState() {
	state := a.activeSetupTabState()
	a.inventoryCursor = state.cursor
	if a.setupSearchFocused {
		state.searchInput.Focus()
	} else {
		state.searchInput.Blur()
	}
}

func (a *App) clampSetupConsoleState() {
	active := normalizeSetupConsoleTab(a.activeSetupTab)
	a.pruneExpandedMarketplaceSources()
	for _, tab := range SetupConsoleTabs {
		a.activeSetupTab = tab
		state := a.activeSetupTabState()
		model := a.currentSetupConsoleViewModel()
		state.cursor = clampIndex(state.cursor, len(model.Rows))
	}
	a.activeSetupTab = active
	a.mirrorActiveSetupTabState()
}

func (a *App) pruneExpandedMarketplaceSources() {
	if len(a.setupConsole.expandedSources) == 0 {
		return
	}
	valid := make(map[string]struct{})
	for _, source := range setupConsoleMarketplaceSources(setup.BuildMarketplace(a.evidence)) {
		valid[source.ID] = struct{}{}
	}
	for sourceID := range a.setupConsole.expandedSources {
		if _, ok := valid[sourceID]; !ok {
			delete(a.setupConsole.expandedSources, sourceID)
		}
	}
}

func (a *App) toggleMarketplaceSource(sourceID string) {
	if a.setupConsole.expandedSources == nil {
		a.setupConsole.expandedSources = make(map[string]bool)
	}
	if a.setupConsole.expandedSources[sourceID] {
		delete(a.setupConsole.expandedSources, sourceID)
	} else {
		a.setupConsole.expandedSources[sourceID] = true
	}
	a.setInventoryCursor(clampIndex(a.inventoryCursor, len(a.currentSetupConsoleViewModel().Rows)))
	a.clearStatus()
}

func (a *App) currentSetupConsoleViewModel() SetupConsoleViewModel {
	state := a.activeSetupTabState()
	return BuildSetupConsoleViewModel(BuildSetupConsoleViewModelInput{
		Inventory:                a.inventory,
		MarketplaceSources:       setup.BuildMarketplace(a.evidence),
		ActiveTab:                a.activeSetupTab,
		Search:                   state.search,
		SearchInput:              state.searchInput.View(),
		SearchFocused:            a.setupSearchFocused,
		SelectedIndex:            state.cursor,
		ExpandedSources:          a.setupConsole.expandedSources,
		ExpandedRowID:            a.expandedSetupRowID(a.activeSetupTab),
		ExpandedToolID:           a.setupConsole.expandedMCPTool,
		PendingMarketplaceReview: a.pendingMarketplaceReview,
		MarketplaceReviewResult:  a.marketplaceReviewResult,
		BaselineStatus:           baselineStatusPtr(a.baselineStatus),
	})
}

func baselineStatusPtr(status baseline.Status) *baseline.Status {
	if len(status.Agents) == 0 {
		return nil
	}
	return &status
}

func firstAvailableInventoryAction(item setup.InventoryItem) (setup.ActionKind, bool) {
	for _, preferred := range []setup.ActionKind{setup.ActionEdit, setup.ActionRemove} {
		for _, action := range item.Actions {
			if action.Action == preferred && action.Available {
				return preferred, true
			}
		}
	}
	return "", false
}

func (a *App) moveTimelineCursor(delta int) {
	entries := a.filteredTimeline()
	if len(entries) == 0 {
		a.timelineCursor = 0
		return
	}
	next := a.timelineCursor + delta
	if next < 0 {
		next = len(entries) - 1
	}
	if next >= len(entries) {
		next = 0
	}
	a.timelineCursor = next
	a.undoPlan = nil
	a.clearStatus()
}

func (a *App) moveSnapshotCursor(delta int) {
	if len(a.snapshotRefs) == 0 {
		a.snapshotCursor = 0
		return
	}
	next := a.snapshotCursor + delta
	if next < 0 {
		next = len(a.snapshotRefs) - 1
	}
	if next >= len(a.snapshotRefs) {
		next = 0
	}
	a.snapshotCursor = next
	a.rollbackReview = nil
	a.clearStatus()
}

func (a *App) moveEnvironmentSelection(delta int) {
	switch a.environments.focus {
	case EnvironmentFocusSurfaces:
		model := a.currentEnvironmentsViewModel()
		a.environments.moveSurface(len(model.Surfaces), delta)
	case EnvironmentFocusDiff:
		a.environments.scrollDiff(a.currentEnvironmentsViewModel(), a.environmentDiffViewportHeight(), delta)
	default:
		a.environments.moveAgent(a.baselineStatus, delta)
	}
	a.clearStatus()
}

func (a *App) focusedEnvironmentAgent() (types.AgentID, bool) {
	return a.environments.selectedAgent(a.baselineStatus)
}

func (a *App) currentEnvironmentsViewModel() EnvironmentsViewModel {
	a.environments.ensure()
	a.environments.clampAgents(a.baselineStatus)
	requestedSurface := a.environments.surfaceCursor
	requestedHunk := a.environments.hunkCursor
	model := BuildEnvironmentsViewModel(BuildEnvironmentsViewModelInput{
		Status:               a.baselineStatus,
		Inventory:            a.inventory,
		SelectedIndex:        a.environments.agentCursor,
		SelectedSurfaceIndex: a.environments.surfaceCursor,
		Focus:                a.environments.focus,
		Mode:                 a.environments.mode,
		CurrentHunkIndex:     a.environments.hunkCursor,
		DiffOffset:           a.environments.diffOffset,
	})
	a.environments.clampSurfaces(len(model.Surfaces))
	a.environments.clampHunks(environmentHunkCount(model.Diff.Rows))
	if requestedSurface != a.environments.surfaceCursor || requestedHunk != a.environments.hunkCursor {
		model = BuildEnvironmentsViewModel(BuildEnvironmentsViewModelInput{
			Status:               a.baselineStatus,
			Inventory:            a.inventory,
			SelectedIndex:        a.environments.agentCursor,
			SelectedSurfaceIndex: a.environments.surfaceCursor,
			Focus:                a.environments.focus,
			Mode:                 a.environments.mode,
			CurrentHunkIndex:     a.environments.hunkCursor,
			DiffOffset:           a.environments.diffOffset,
		})
	}
	return model
}

func (a *App) environmentDiffViewportHeight() int {
	if a.height <= 0 {
		return 10
	}
	return max(4, a.height/2)
}

// saveFocusedEnvironment opens a review for saving the focused agent's current
// setup. The actual capture only begins after explicit confirmation.
func (a *App) saveFocusedEnvironment() tea.Cmd {
	agent, ok := a.focusedEnvironmentAgent()
	if !ok {
		a.setStatus(views.StatusError, "No environment selected.")
		return nil
	}
	selected := agent
	a.pendingSave = &saveReview{Agent: &selected}
	a.clearStatus()
	return nil
}

func (a *App) confirmSaveReview() tea.Cmd {
	if a.pendingSave == nil {
		a.setStatus(views.StatusError, "No save is pending.")
		return nil
	}
	if a.pendingSave.Agent == nil {
		return func() tea.Msg {
			created, err := a.createMissingBaselines()
			return baselineCreateMsg{
				created: created,
				data:    a.fetchWorkspaceData(),
				err:     err,
			}
		}
	}
	return a.captureEnvironmentSave(*a.pendingSave.Agent)
}

func (a *App) captureEnvironmentSave(agent types.AgentID) tea.Cmd {
	return func() tea.Msg {
		scope := types.ScopeUser
		runtime := a.runtime
		runtime.Agent = &agent
		runtime.Scope = &scope
		runtime.CaptureContent = agents.SupportsContentBackedUserSnapshot(agent, scope)
		name := fmt.Sprintf("snapshot-%s-%s", agent.String(), a.now().UTC().Format("20060102-150405-000000000"))
		state, err := snapshot.CaptureCurrentState(&runtime, name)
		if err != nil {
			return baselineCreateMsg{err: err}
		}
		if err := store.WriteSnapshot(runtime.StoreDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
			return baselineCreateMsg{err: err}
		}
		return baselineCreateMsg{created: []string{name}, data: a.fetchWorkspaceData()}
	}
}

// restoreFocusedEnvironment opens a rollback review for the focused agent's
// latest snapshot, routing into the existing review/apply safety flow.
func (a *App) restoreFocusedEnvironment() tea.Cmd {
	agent, ok := a.focusedEnvironmentAgent()
	if !ok {
		a.setStatus(views.StatusError, "No environment selected.")
		return nil
	}
	var ref *snapshotRef
	for i := range a.snapshotRefs {
		if a.snapshotRefs[i].Agent == agent && !store.IsRestorePointSnapshotName(a.snapshotRefs[i].Name) {
			ref = &a.snapshotRefs[i]
			break
		}
	}
	if ref == nil {
		a.setStatus(views.StatusError, "No save for this agent yet. Press s to create one.")
		return nil
	}
	selected := *ref
	a.screen = ScreenSnapshots
	for i := range a.snapshotRefs {
		if a.snapshotRefs[i].Name == selected.Name && a.snapshotRefs[i].Agent == selected.Agent {
			a.snapshotCursor = i
			break
		}
	}
	return func() tea.Msg {
		review, err := a.buildRollbackReview(selected)
		if err != nil {
			return rollbackPreviewMsg{err: err}
		}
		return rollbackPreviewMsg{review: review}
	}
}

func (a *App) handleSnapshotEnter() tea.Cmd {
	if a.rollbackReview != nil {
		review := *a.rollbackReview
		return func() tea.Msg {
			msg := a.applyRollbackReview(&review)
			return msg
		}
	}
	if len(a.snapshotRefs) == 0 {
		a.setStatus(views.StatusError, "No supported save selected.")
		return nil
	}
	ref := a.snapshotRefs[clampIndex(a.snapshotCursor, len(a.snapshotRefs))]
	return func() tea.Msg {
		review, err := a.buildRollbackReview(ref)
		if err != nil {
			return rollbackPreviewMsg{err: err}
		}
		return rollbackPreviewMsg{review: review}
	}
}

func (a *App) filteredTimeline() []types.TimelineEntry {
	return FilterTimelineEntries(a.timelineEntries, a.selectedAgent)
}

func (a *App) renderContent(width, height int) string {
	if a.pendingSave != nil {
		return a.renderSaveReview(width, height)
	}
	if a.pendingMarketplaceRollback != nil {
		return a.renderMarketplaceRollbackReview(width, height)
	}
	now := a.now()
	switch a.screen {
	case ScreenHome:
		model := BuildHomeViewModel(a.baselineStatus)
		lastSnapshot := "-"
		if model.LastSnapshotAt != "" {
			lastSnapshot = FormatTimelineTimestamp(model.LastSnapshotAt, now)
		}
		return views.RenderHome(homeViewFromModel(model, lastSnapshot), width, height)
	case ScreenInventory:
		if a.pendingAction != nil {
			return a.renderSetupActionReview(width, height)
		}
		model := a.currentSetupConsoleViewModel()
		a.syncSetupConsoleViewports(&model, width, height)
		view := setupConsoleViewFromModel(model)
		a.syncSkillViewer(&view, width, height)
		return views.RenderSetupConsole(view, width, height)
	case ScreenTimeline:
		model := BuildTimelineViewModel(BuildTimelineViewModelInput{
			Entries:       a.filteredTimeline(),
			SelectedIndex: a.timelineCursor,
			AgentFilter:   a.selectedAgent,
			Evidence:      a.evidence,
			CorruptEvents: a.corruptEvents,
			UndoPlan:      a.undoPlan,
			Now:           now,
		})
		return views.RenderHistory(historyViewFromModel(model), width, height)
	case ScreenSnapshots:
		return a.renderSnapshots(width, height)
	case ScreenEnvironments:
		model := a.currentEnvironmentsViewModel()
		return views.RenderEnvironments(environmentsViewFromModel(model), width, height)
	default:
		return views.RenderMuted("Unsupported screen.")
	}
}

func (a *App) renderSaveReview(width, height int) string {
	title := "Save current setup"
	target := "current supported agent setup"
	notes := []string{"Creates only missing agent saves."}
	if a.pendingSave != nil && a.pendingSave.Agent != nil {
		agentLabel := FormatAgentLabel(*a.pendingSave.Agent)
		title = "Save " + agentLabel + " setup"
		target = agentLabel + " user setup"
		notes = []string{"Creates a new local save for the selected agent."}
	}
	return views.RenderReviewChanges(views.ReviewChangesView{
		Title:    title,
		Subtitle: "Local save for drift comparison and safe restore.",
		Notes:    notes,
		Changes: []views.ReviewChangeRow{{
			Marker: "+",
			Kind:   "Save",
			Target: target,
		}},
	}, width, height)
}

func (a *App) renderMarketplaceRollbackReview(width, height int) string {
	target := "last marketplace install"
	notes := []string{"The provider rollback is followed by a rescan and verification."}
	if a.pendingMarketplaceRollback != nil {
		plan := *a.pendingMarketplaceRollback
		if plan.MarketplaceInstall != nil && strings.TrimSpace(plan.MarketplaceInstall.Selector) != "" {
			target = plan.MarketplaceInstall.Selector
		}
		if plan.Agent != "" {
			notes = append(notes, "agent: "+FormatAgentLabel(plan.Agent))
		}
	}
	return views.RenderReviewChanges(views.ReviewChangesView{
		Title:    "Undo marketplace install",
		Subtitle: "Remove the plugin installed by the last verified action.",
		Notes:    notes,
		Changes: []views.ReviewChangeRow{{
			Marker: "-",
			Kind:   "Plugin",
			Target: target,
		}},
	}, width, height)
}

func (a *App) renderSetupActionReview(width, height int) string {
	if a.pendingAction == nil {
		return views.RenderReviewChanges(views.ReviewChangesView{
			Title:     "Setup action",
			EmptyText: "No pending setup action.",
		}, width, height)
	}
	plan := *a.pendingAction
	agentLabel := FormatAgentLabel(plan.Agent)
	objectKind := formatSetupObjectKind(plan.ObjectKind)
	target := plan.ConfigTarget
	if strings.TrimSpace(target) == "" {
		target = plan.TargetName
	}
	marker := "~"
	if plan.Action == setup.ActionRemove {
		marker = "-"
	} else if plan.Action == setup.ActionAdd {
		marker = "+"
	}
	command := plan.Operation
	if plan.Command != nil {
		command = strings.Join(append([]string{plan.Command.Program}, plan.Command.Args...), " ")
	}
	notes := []string{"agent: " + agentLabel}
	if strings.TrimSpace(command) != "" {
		notes = append(notes, "command: "+command)
	}
	return views.RenderReviewChanges(views.ReviewChangesView{
		Title:    fmt.Sprintf("%s %s %q", plan.Action, objectKind, plan.TargetName),
		Subtitle: plan.Operation,
		Notes:    notes,
		Changes: []views.ReviewChangeRow{{
			Marker: marker,
			Kind:   objectKind,
			Target: target,
		}},
	}, width, height)
}

func (a *App) syncSkillViewer(view *views.SetupConsoleView, width, height int) {
	if a.skillViewer == nil || view == nil {
		return
	}
	overlayWidth := width - 2
	if overlayWidth < 34 {
		overlayWidth = max(20, width-2)
	}
	bodyWidth := overlayWidth - 4
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	overlayHeight := height - 3
	if overlayHeight < 8 {
		overlayHeight = max(6, height-2)
	}
	bodyHeight := overlayHeight - 6
	if bodyHeight < 3 {
		bodyHeight = 3
	}

	rendered := a.skillViewer.rendered
	if a.skillViewer.errorText == "" && a.skillViewer.renderWidth != bodyWidth {
		rendered = renderSkillMarkdown(a.skillViewer.content, bodyWidth)
		a.skillViewer.rendered = rendered
		a.skillViewer.renderWidth = bodyWidth
		a.skillViewer.viewport.SetContent(rendered)
	}
	if a.skillViewer.errorText != "" && a.skillViewer.rendered == "" {
		a.skillViewer.rendered = a.skillViewer.errorText
		a.skillViewer.viewport.SetContent(a.skillViewer.rendered)
	}
	a.skillViewer.viewport.Width = bodyWidth
	a.skillViewer.viewport.Height = bodyHeight
	view.MarkdownOverlay = &views.SetupMarkdownOverlay{
		Title:      a.skillViewer.title,
		Subtitle:   strings.Join([]string{a.skillViewer.agentLabel, a.skillViewer.objectKind, a.skillViewer.status}, " · "),
		SourcePath: a.skillViewer.sourcePath,
		Body:       a.skillViewer.viewport.View(),
		ErrorText:  a.skillViewer.errorText,
		Width:      overlayWidth,
		Height:     overlayHeight,
	}
}

func renderSkillMarkdown(content string, width int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}
	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimRight(rendered, "\n")
}

func (a *App) renderSnapshots(width, height int) string {
	if a.rollbackReview != nil {
		return a.renderRollbackReview(width, height)
	}
	model := views.SavesView{Rows: make([]views.SaveRow, 0, len(a.snapshotRefs))}
	selected := clampIndex(a.snapshotCursor, len(a.snapshotRefs))
	for i, ref := range a.snapshotRefs {
		model.Rows = append(model.Rows, views.SaveRow{
			AgentMarker: FormatAgentMarker(ref.Agent),
			Name:        formatSaveDisplayName(ref.Name),
			CreatedAt:   formatDate(ref.CreatedAt),
			Selected:    i == selected,
		})
	}
	return views.RenderSaves(model, width, height)
}

func (a *App) renderRollbackReview(width, height int) string {
	review := a.rollbackReview
	model := views.ReviewChangesView{
		Title:    fmt.Sprintf("Restore %s from save %s", FormatAgentLabel(review.Agent), formatSaveDisplayName(review.SnapshotName)),
		Subtitle: "Current setup is saved automatically before apply.",
	}
	for _, item := range review.Items {
		marker := "~"
		if item.Action != nil {
			switch string(*item.Action) {
			case "create", "add":
				marker = "+"
			case "delete", "remove":
				marker = "-"
			}
		}
		model.Changes = append(model.Changes, views.ReviewChangeRow{
			Marker: marker,
			Kind:   item.ItemType,
			Target: item.Dest,
		})
	}
	for _, item := range review.UnsupportedItems {
		model.Unsupported = append(model.Unsupported, views.ReviewUnsupportedRow{
			Kind:   string(item.Kind),
			Source: item.SourcePath,
			Reason: item.Reason,
		})
	}
	return views.RenderReviewChanges(model, width, height)
}

func (a *App) syncSetupConsoleViewports(model *SetupConsoleViewModel, width, height int) {
	if model == nil {
		return
	}
	listHeight := setupConsoleListHeight(height)
	a.setupConsole.rowsViewport.Width = width
	a.setupConsole.rowsViewport.Height = listHeight
	lines := make([]string, len(model.Rows))
	for i := range lines {
		lines[i] = model.Rows[i].ID
	}
	a.setupConsole.rowsViewport.SetContent(strings.Join(lines, "\n"))
	cursor := clampIndex(a.activeSetupTabState().cursor, len(model.Rows))
	if cursor < a.setupConsole.rowsViewport.YOffset {
		a.setupConsole.rowsViewport.SetYOffset(cursor)
	} else if cursor >= a.setupConsole.rowsViewport.YOffset+listHeight {
		a.setupConsole.rowsViewport.SetYOffset(cursor - listHeight + 1)
	} else {
		a.setupConsole.rowsViewport.SetYOffset(a.setupConsole.rowsViewport.YOffset)
	}
	model.RowOffset = a.setupConsole.rowsViewport.YOffset
}

func setupConsoleListHeight(height int) int {
	if height < 12 {
		height = 12
	}
	listHeight := height - 10
	if listHeight < 4 {
		listHeight = 4
	}
	return listHeight
}

func (a *App) defaultSetupActionExecutor(ctx context.Context, plan setup.ActionPlan) error {
	_, err := setup.ExecuteActionPlan(ctx, plan, nil, setup.WithHomeDir(a.runtime.HomeDir))
	return err
}

func (a *App) loadData() tea.Msg {
	return a.fetchWorkspaceData()
}

func (a *App) fetchWorkspaceData() bootMsg {
	if _, err := store.EnsureStore(a.runtime.StoreDir); err != nil {
		return bootMsg{err: err}
	}

	scanResult := scan.ScanProject(&types.ScanOptions{
		ProjectPath: a.runtime.ProjectPath,
		HomeDir:     a.runtime.HomeDir,
		StoreDir:    a.runtime.StoreDir,
	})

	var corrupt []store.TimelineCorruptEvent
	entries, err := store.ListTimelineEntries(a.runtime.StoreDir, store.TimelineListOptions{
		ProjectPath: a.runtime.ProjectPath,
		OnCorruptEntry: func(event store.TimelineCorruptEvent) {
			corrupt = append(corrupt, event)
		},
	})
	if err != nil {
		return bootMsg{err: err}
	}

	snapshotRefs, err := listSupportedSnapshotRefs(a.runtime.StoreDir)
	if err != nil {
		return bootMsg{err: err}
	}
	baselineStatus, err := baseline.BuildStatus(a.runtime)
	if err != nil {
		return bootMsg{err: err}
	}

	return bootMsg{
		evidence:        scanResult.Evidence,
		timelineEntries: entries,
		corruptEvents:   corrupt,
		snapshotRefs:    snapshotRefs,
		baselineStatus:  baselineStatus,
	}
}

func listSupportedSnapshotRefs(storeDir string) ([]snapshotRef, error) {
	var refs []snapshotRef
	for _, agent := range agents.CurrentSupportedIDs() {
		names, err := store.ListSnapshots(storeDir, &agent)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			snap, err := store.ReadSnapshot(storeDir, name, &agent)
			if err != nil {
				return nil, err
			}
			refs = append(refs, snapshotRef{Name: name, Agent: agent, CreatedAt: snap.Manifest.CreatedAt})
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].CreatedAt > refs[j].CreatedAt
	})
	return refs, nil
}

func (a *App) createMissingBaselines() ([]string, error) {
	status := a.baselineStatus
	if len(status.Agents) == 0 {
		built, err := baseline.BuildStatus(a.runtime)
		if err != nil {
			return nil, err
		}
		status = built
	}

	scope := types.ScopeUser
	now := time.Now().UTC()
	var created []string
	for _, agentStatus := range status.Agents {
		if agentStatus.HasBaseline {
			continue
		}
		agent := agentStatus.Agent
		runtime := a.runtime
		runtime.Agent = &agent
		runtime.Scope = &scope
		runtime.CaptureContent = agents.SupportsContentBackedUserSnapshot(agent, scope)
		name := fmt.Sprintf("baseline-%s-%s", agent.String(), now.Format("20060102-150405-000000000"))
		state, err := snapshot.CaptureCurrentState(&runtime, name)
		if err != nil {
			return created, err
		}
		if err := store.WriteSnapshot(runtime.StoreDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
			return created, err
		}
		created = append(created, name)
	}
	return created, nil
}

func (a *App) buildRollbackReview(ref snapshotRef) (*rollbackReview, error) {
	scope := types.ScopeUser
	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: ref.Name,
		ProjectPath:    a.runtime.ProjectPath,
		HomeDir:        a.runtime.HomeDir,
		StoreDir:       a.runtime.StoreDir,
		DryRun:         true,
		Agent:          &ref.Agent,
		Scope:          &scope,
	})
	if err != nil {
		return nil, err
	}
	parsed := restore.RestoreItemsFromPlan(plan)
	if len(parsed.Errors) != 0 {
		return nil, fmt.Errorf("restore preview contains %d parse errors: %s", len(parsed.Errors), parsed.Errors[0].Message)
	}
	return &rollbackReview{
		SnapshotName:     ref.Name,
		Agent:            ref.Agent,
		Plan:             plan,
		Items:            parsed.Items,
		UnsupportedItems: append([]types.UnsupportedPlanItem(nil), plan.UnsupportedItems...),
	}, nil
}

func (a *App) applyRollbackReview(review *rollbackReview) rollbackApplyMsg {
	fresh, err := a.buildRollbackReview(snapshotRef{Name: review.SnapshotName, Agent: review.Agent})
	if err != nil {
		return rollbackApplyMsg{err: fmt.Errorf("failed to refresh restore review before apply: %w", err)}
	}
	if !rollbackReviewMatches(review, fresh) {
		return rollbackApplyMsg{err: fmt.Errorf("review changes are stale; reopen Review Changes before applying")}
	}

	createRestorePoint := a.createPreApplyRestorePoint
	if a.restorePointCreator != nil {
		createRestorePoint = a.restorePointCreator
	}
	restorePoint, err := createRestorePoint(review.Agent)
	if err != nil {
		return rollbackApplyMsg{err: fmt.Errorf("failed to create pre-apply safety save: %w", err)}
	}

	executor := restore.CreateDefaultApplyExecutor()
	if a.restoreExecutor != nil {
		executor = a.restoreExecutor
	}
	homeDir := a.runtime.HomeDir
	projectPath := a.runtime.ProjectPath
	items := append([]types.RestoreItem(nil), review.Items...)
	summary := restore.ApplyRestoreItems(items, executor, &types.ApplyOptions{
		FailFast:    true,
		HomeDir:     &homeDir,
		ProjectPath: &projectPath,
	})
	if summary.Failed != 0 {
		return rollbackApplyMsg{
			summary:      summary,
			restorePoint: restorePoint,
			err:          fmt.Errorf("restore apply failed: %d failed", summary.Failed),
		}
	}
	verify := a.verifyRollback(review)
	return rollbackApplyMsg{
		data:         a.fetchWorkspaceData(),
		summary:      summary,
		restorePoint: restorePoint,
		verify:       verify,
	}
}

func (a *App) createPreApplyRestorePoint(agent types.AgentID) (string, error) {
	scope := types.ScopeUser
	runtime := a.runtime
	runtime.Agent = &agent
	runtime.Scope = &scope
	runtime.CaptureContent = agents.SupportsContentBackedUserSnapshot(agent, scope)
	name := fmt.Sprintf("pre-apply-%s-%s", agent.String(), time.Now().UTC().Format("20060102-150405-000000000"))
	state, err := snapshot.CaptureCurrentState(&runtime, name)
	if err != nil {
		return "", err
	}
	if err := store.WriteSnapshot(runtime.StoreDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		return "", err
	}
	return name, nil
}

func (a *App) verifyRollback(review *rollbackReview) string {
	next, err := a.buildRollbackReview(snapshotRef{Name: review.SnapshotName, Agent: review.Agent})
	if err != nil {
		return "Verification failed: " + err.Error()
	}
	if len(next.Plan.Items) == 0 {
		return "Verified selected save."
	}
	return fmt.Sprintf("Verification found %d remaining supported changes.", len(next.Plan.Items))
}

type rollbackReviewFingerprint struct {
	Items       []rollbackReviewItemFingerprint        `json:"items"`
	Unsupported []rollbackReviewUnsupportedFingerprint `json:"unsupported"`
}

type rollbackReviewItemFingerprint struct {
	Agent        types.AgentID                `json:"agent"`
	Kind         types.EvidenceKind           `json:"kind"`
	SourcePath   string                       `json:"sourcePath"`
	Action       types.RestoreAction          `json:"action"`
	CurrentState *rollbackEvidenceFingerprint `json:"currentState,omitempty"`
	TargetState  *rollbackEvidenceFingerprint `json:"targetState,omitempty"`
}

type rollbackReviewUnsupportedFingerprint struct {
	Agent      types.AgentID      `json:"agent"`
	Kind       types.EvidenceKind `json:"kind"`
	SourcePath string             `json:"sourcePath"`
	Reason     string             `json:"reason"`
}

type rollbackEvidenceFingerprint struct {
	ID         string               `json:"id"`
	Agent      types.AgentID        `json:"agent"`
	Kind       types.EvidenceKind   `json:"kind"`
	SourcePath string               `json:"sourcePath"`
	Scope      types.EvidenceScope  `json:"scope"`
	Parser     types.EvidenceParser `json:"parser"`
	Name       string               `json:"name,omitempty"`
	Value      string               `json:"value,omitempty"`
	Checksum   string               `json:"checksum,omitempty"`
	Metadata   string               `json:"metadata,omitempty"`
}

func rollbackReviewMatches(left, right *rollbackReview) bool {
	if left == nil || right == nil {
		return left == right
	}
	return rollbackReviewFingerprintForPlan(left.Plan) == rollbackReviewFingerprintForPlan(right.Plan)
}

func rollbackReviewFingerprintForPlan(plan *types.RestorePlan) string {
	if plan == nil {
		return ""
	}
	fingerprint := rollbackReviewFingerprint{
		Items:       make([]rollbackReviewItemFingerprint, 0, len(plan.Items)),
		Unsupported: make([]rollbackReviewUnsupportedFingerprint, 0, len(plan.UnsupportedItems)),
	}
	for _, item := range plan.Items {
		fingerprint.Items = append(fingerprint.Items, rollbackReviewItemFingerprint{
			Agent:        item.Agent,
			Kind:         item.Kind,
			SourcePath:   item.SourcePath,
			Action:       item.Action,
			CurrentState: rollbackEvidenceFingerprintFor(item.CurrentState),
			TargetState:  rollbackEvidenceFingerprintFor(item.TargetState),
		})
	}
	for _, unsupported := range plan.UnsupportedItems {
		fingerprint.Unsupported = append(fingerprint.Unsupported, rollbackReviewUnsupportedFingerprint{
			Agent:      unsupported.Agent,
			Kind:       unsupported.Kind,
			SourcePath: unsupported.SourcePath,
			Reason:     unsupported.Reason,
		})
	}
	sort.Slice(fingerprint.Items, func(i, j int) bool {
		left, _ := json.Marshal(fingerprint.Items[i])
		right, _ := json.Marshal(fingerprint.Items[j])
		return string(left) < string(right)
	})
	sort.Slice(fingerprint.Unsupported, func(i, j int) bool {
		left, _ := json.Marshal(fingerprint.Unsupported[i])
		right, _ := json.Marshal(fingerprint.Unsupported[j])
		return string(left) < string(right)
	})
	raw, _ := json.Marshal(fingerprint)
	return string(raw)
}

func rollbackEvidenceFingerprintFor(item *types.DiscoveredItem) *rollbackEvidenceFingerprint {
	if item == nil {
		return nil
	}
	out := &rollbackEvidenceFingerprint{
		ID:         item.ID,
		Agent:      item.Agent,
		Kind:       item.Kind,
		SourcePath: item.SourcePath,
		Scope:      item.Scope,
		Parser:     item.Parser,
		Value:      canonicalRawJSON(item.Value),
		Metadata:   canonicalRawJSON(item.Metadata),
	}
	if item.Name != nil {
		out.Name = *item.Name
	}
	if item.Checksum != nil {
		out.Checksum = *item.Checksum
	}
	return out
}

func canonicalRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return strings.TrimSpace(string(raw))
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	return string(canonical)
}

// Run launches the interactive TUI and returns an exit code.
func Run(runtime types.RuntimeOptions) int {
	app := NewApp(runtime)
	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gandalf tui failed: %v\n", err)
		return 1
	}
	return 0
}

// PreviewUndo builds a dry-run undo plan for a timeline entry id.
func PreviewUndo(storeDir, entryID string, onCorrupt func(store.TimelineCorruptEvent)) (*timelineundo.Plan, error) {
	return timelineundo.BuildPlan(storeDir, entryID, timelineundo.BuildOptions{
		OnCorruptEntry: onCorrupt,
	})
}
