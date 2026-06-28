package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
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
	snapshotNames   []string
	err             error
}

type rescanMsg bootMsg

type setupActionMsg struct {
	data bootMsg
	err  error
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
	snapshotNames   []string

	screen          Screen
	selectedAgent   *types.AgentID
	selectedProfile string

	navCursor          int
	inventoryCursor    int
	inventoryFocus     bool
	activeSetupTab     SetupConsoleTab
	setupSearchFocused bool
	setupConsole       setupConsoleState
	timelineCursor     int

	undoPlan      *timelineundo.Plan
	undoError     string
	notice        string
	actionError   string
	pendingAction *setup.ActionPlan
	skillViewer   *skillMarkdownViewerState

	compareModel   *CompareViewModel
	saveSetupModel *SaveSetupViewModel

	cachedNav    *NavigationModel
	cachedNavKey string

	actionExecutor func(context.Context, setup.ActionPlan) error
}

type setupConsoleState struct {
	tabs            map[SetupConsoleTab]*setupConsoleTabState
	expandedSources map[string]bool
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
		runtime:         runtime,
		screen:          ScreenInventory,
		selectedProfile: DefaultProfile,
		inventoryFocus:  true,
		activeSetupTab:  SetupConsoleTabHooks,
		setupConsole:    setupState,
		actionExecutor:  defaultSetupActionExecutor,
	}
}

func newSetupConsoleState() setupConsoleState {
	state := setupConsoleState{
		tabs:            make(map[SetupConsoleTab]*setupConsoleTabState, len(SetupConsoleTabs)),
		expandedSources: make(map[string]bool),
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
			a.notice = typed.err.Error()
			return a, nil
		}
		a.applyWorkspaceData(bootMsg(typed))
		a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
		a.undoPlan = nil
		a.undoError = ""
		a.pendingAction = nil
		a.actionError = ""
		a.notice = "Rescanned global setup."
		return a, nil

	case setupActionMsg:
		if typed.err != nil {
			a.actionError = typed.err.Error()
			return a, nil
		}
		if typed.data.err != nil {
			a.pendingAction = nil
			a.actionError = "Applied setup action, but failed to rescan: " + typed.data.err.Error()
			return a, nil
		}
		a.applyWorkspaceData(typed.data)
		a.clampSetupConsoleState()
		a.pendingAction = nil
		a.actionError = ""
		a.notice = "Applied setup action and rescanned global setup."
		return a, nil

	case undoPreviewMsg:
		a.undoPlan = nil
		a.undoError = ""
		a.corruptEvents = typed.corruptEvents
		if typed.err != nil {
			a.undoError = typed.err.Error()
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
	if contentWidth < 40 {
		contentWidth = 40
	}
	contentHeight := a.height - 2

	if !a.ready {
		if a.errText != "" {
			return views.RenderHistory(views.HistoryView{
				EmptyMessage: "Failed to load workspace.",
				EmptyCommand: a.errText,
			}, contentWidth, contentHeight)
		}
		return "Loading Gandalf global setup workspace..."
	}

	content := a.renderContent(contentWidth, contentHeight)

	header := lipgloss.NewStyle().Bold(true).Render("gandalf tui · setup console")
	if a.notice != "" {
		header += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(a.notice)
	}
	if a.undoError != "" {
		header += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(a.undoError)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, lipgloss.NewStyle().Width(contentWidth).Render(content))
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "q":
		return nil, true
	case "esc":
		if a.skillViewer != nil {
			a.skillViewer = nil
			a.actionError = ""
			return nil, false
		}
		if a.pendingAction != nil {
			a.pendingAction = nil
			a.actionError = ""
		}
	case "/":
		if a.screen == ScreenInventory && a.pendingAction == nil {
			a.setupSearchFocused = true
			state := a.activeSetupTabState()
			state.searchInput.Focus()
			a.mirrorActiveSetupTabState()
			return nil, false
		}
	case "tab":
		if a.screen == ScreenInventory && a.pendingAction == nil {
			a.moveSetupTab(1)
		}
	case "shift+tab":
		if a.screen == ScreenInventory && a.pendingAction == nil {
			a.moveSetupTab(-1)
		}
	case "r":
		return func() tea.Msg {
			data := a.fetchWorkspaceData()
			return rescanMsg(data)
		}, false
	case "H":
		if a.screen == ScreenInventory {
			a.screen = ScreenTimeline
			a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
		}
	case "S":
		if a.screen == ScreenInventory {
			a.screen = ScreenSnapshots
		}
	case "i":
		if a.screen != ScreenInventory {
			a.screen = ScreenInventory
			a.undoPlan = nil
			a.undoError = ""
		}
	case "u":
		if a.screen != ScreenTimeline {
			return nil, false
		}
		entries := a.filteredTimeline()
		if len(entries) == 0 {
			a.undoError = "No timeline entry selected."
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
		if a.screen == ScreenInventory && a.inventoryFocus && a.pendingAction == nil {
			a.moveInventoryCursor(-1)
			return nil, false
		}
		a.moveNavCursor(-1)
	case "down", "j":
		if a.screen == ScreenInventory && a.inventoryFocus && a.pendingAction == nil {
			a.moveInventoryCursor(1)
			return nil, false
		}
		a.moveNavCursor(1)
	case "left", "h":
		if a.screen == ScreenTimeline {
			a.moveTimelineCursor(-1)
		}
	case "right", "l":
		if a.screen == ScreenTimeline {
			a.moveTimelineCursor(1)
		}
	case "enter":
		if a.screen == ScreenInventory && a.inventoryFocus {
			return a.handleInventoryEnter(), false
		}
	case " ", "space":
		if a.screen == ScreenInventory && a.inventoryFocus && a.pendingAction == nil {
			return a.handleSetupToggle(), false
		}
	}
	return nil, false
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
	a.actionError = ""
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
		return func() tea.Msg {
			if a.actionExecutor == nil {
				return setupActionMsg{err: fmt.Errorf("setup action executor is unavailable")}
			}
			if err := a.actionExecutor(context.Background(), plan); err != nil {
				return setupActionMsg{err: err}
			}
			return setupActionMsg{data: a.fetchWorkspaceData()}
		}
	}

	if normalizeSetupConsoleTab(a.activeSetupTab) == SetupConsoleTabMarketplace {
		return a.handleMarketplaceEnter()
	}
	if normalizeSetupConsoleTab(a.activeSetupTab) == SetupConsoleTabSkills {
		a.openSelectedSkillViewer()
		return nil
	}

	inventory := a.currentInventory()
	if len(inventory) == 0 {
		a.actionError = "No setup item selected."
		return nil
	}
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	action, ok := firstAvailableInventoryAction(item)
	if !ok {
		a.actionError = "No supported action is available for this setup item."
		return nil
	}
	plan := setup.PlanItemAction(item, action)
	if !plan.Available {
		a.actionError = plan.UnavailableReason
		return nil
	}
	a.pendingAction = &plan
	a.actionError = ""
	return nil
}

func (a *App) openSelectedSkillViewer() {
	inventory := a.currentInventory()
	if len(inventory) == 0 {
		a.actionError = "No skill selected."
		return
	}
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	viewer := a.buildSkillMarkdownViewer(item)
	a.skillViewer = &viewer
	a.actionError = ""
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
	if info.Mode()&os.ModeSymlink != 0 {
		targetPath, err := filepath.EvalSymlinks(skillPath)
		if err != nil {
			return "", displayPath, fmt.Errorf("Skill markdown symlink target is unreadable: %v", err)
		}
		readPath = targetPath
		displayPath = displayPath + " -> " + displaySetupPath(targetPath, a.runtime.HomeDir)
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
		a.actionError = "No marketplace source selected."
		return nil
	}
	row := model.Rows[clampIndex(a.activeSetupTabState().cursor, len(model.Rows))]
	if row.RowKind == SetupConsoleRowMarketplaceSource {
		a.toggleMarketplaceSource(row.ID)
		return nil
	}
	a.actionError = "Marketplace actions are unavailable until an agent-native provider is implemented."
	return nil
}

func (a *App) handleSetupToggle() tea.Cmd {
	if normalizeSetupConsoleTab(a.activeSetupTab) != SetupConsoleTabMarketplace {
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

func (a *App) currentInventory() []setup.InventoryItem {
	return filterSetupConsoleInventory(a.inventory, normalizeSetupConsoleTab(a.activeSetupTab), a.activeSetupTabState().search)
}

func (a *App) applyWorkspaceData(data bootMsg) {
	a.evidence = data.evidence
	a.inventory = setup.BuildInventory(data.evidence)
	a.timelineEntries = data.timelineEntries
	a.corruptEvents = data.corruptEvents
	a.snapshotNames = data.snapshotNames
	a.cachedNav = nil
	a.cachedNavKey = ""
	a.clampSetupConsoleState()
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
	a.actionError = ""
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
	a.actionError = ""
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
	a.actionError = ""
}

func (a *App) currentSetupConsoleViewModel() SetupConsoleViewModel {
	state := a.activeSetupTabState()
	return BuildSetupConsoleViewModel(BuildSetupConsoleViewModelInput{
		Inventory:          a.inventory,
		MarketplaceSources: setup.BuildMarketplace(a.evidence),
		ActiveTab:          a.activeSetupTab,
		Search:             state.search,
		SearchInput:        state.searchInput.View(),
		SearchFocused:      a.setupSearchFocused,
		SelectedIndex:      state.cursor,
		ExpandedSources:    a.setupConsole.expandedSources,
		PendingAction:      a.pendingAction,
		ActionError:        a.actionError,
	})
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

func (a *App) moveNavCursor(delta int) {
	nav := a.navigationModel()
	if len(nav.FlatItems) == 0 {
		return
	}
	maxCursor := len(nav.FlatItems) - 1
	next := a.navCursor + delta
	if next < 0 {
		next = maxCursor
	}
	if next > maxCursor {
		next = 0
	}
	a.navCursor = next
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
	a.undoError = ""
}

func (a *App) activateNavItem() {
	nav := a.navigationModel()
	if len(nav.FlatItems) == 0 {
		return
	}
	item := nav.FlatItems[a.navCursor]
	selection := SelectNavItem(item, a.screen, a.selectedAgent, a.selectedProfile)
	a.screen = selection.Screen
	a.selectedAgent = selection.SelectedAgent
	a.selectedProfile = selection.SelectedProfile
	a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
	a.undoPlan = nil
	a.undoError = ""
}

func (a *App) navigationModel() NavigationModel {
	selectedID := NavItemIDForSelection(NavigationSelection{
		Screen:          a.screen,
		SelectedAgent:   a.selectedAgent,
		SelectedProfile: a.selectedProfile,
	})
	key := fmt.Sprintf("%s:%d:%d", selectedID, a.navCursor, len(a.evidence))
	if a.cachedNav != nil && a.cachedNavKey == key {
		return *a.cachedNav
	}
	nav := BuildNavigationModel(BuildNavigationModelInput{
		Evidence:       a.evidence,
		SelectedItemID: selectedID,
		Cursor:         a.navCursor,
	})
	a.cachedNav = &nav
	a.cachedNavKey = key
	return nav
}

func (a *App) filteredTimeline() []types.TimelineEntry {
	return FilterTimelineEntries(a.timelineEntries, a.selectedAgent)
}

func (a *App) renderContent(width, height int) string {
	now := time.Now()
	switch a.screen {
	case ScreenInventory:
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
	case ScreenAgentDetail:
		if a.selectedAgent == nil {
			return "Select an agent."
		}
		model := BuildAgentDetailViewModel(BuildAgentDetailViewModelInput{
			Agent:           *a.selectedAgent,
			Evidence:        a.evidence,
			TimelineEntries: a.timelineEntries,
			Profile:         a.selectedProfile,
			Now:             now,
		})
		return views.RenderAgentDetail(agentDetailViewFromModel(model), width, height)
	case ScreenCompare:
		if a.compareModel == nil {
			return "No compare data loaded."
		}
		return views.RenderCompare(compareViewFromModel(*a.compareModel), width, height)
	case ScreenSaveSetup:
		if a.saveSetupModel == nil {
			model := BuildSaveSetupViewModel(BuildSaveSetupViewModelInput{
				HasPreviousSnapshot: len(a.snapshotNames) > 0,
			})
			a.saveSetupModel = &model
		}
		return views.RenderSaveSetup(saveSetupViewFromModel(*a.saveSetupModel), width, height)
	case ScreenSnapshots:
		if len(a.snapshotNames) == 0 {
			return "No saved setups yet.\n\ns save setup"
		}
		lines := []string{"Saved setups", ""}
		for _, name := range a.snapshotNames {
			lines = append(lines, "  "+name)
		}
		return strings.Join(lines, "\n")
	case ScreenProfile:
		agentLabels := make([]string, 0)
		seen := make(map[types.AgentID]struct{})
		for _, item := range a.evidence {
			if item.Agent == types.AgentProject {
				continue
			}
			if _, ok := seen[item.Agent]; ok {
				continue
			}
			seen[item.Agent] = struct{}{}
			agentLabels = append(agentLabels, FormatAgentLabel(item.Agent))
		}
		changedAt := "-"
		if len(a.timelineEntries) > 0 {
			changedAt = FormatTimelineTimestamp(a.timelineEntries[0].ObservedAt, now)
		}
		return fmt.Sprintf("Profiles\n\ndefault\n  snapshots: %d\n  agents: %s\n  changed: %s",
			len(a.snapshotNames), strings.Join(agentLabels, ", "), changedAt)
	default:
		return "Unsupported screen."
	}
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

func (a *App) syncSetupConsoleViewports(model *SetupConsoleViewModel, width, height int) {
	if model == nil {
		return
	}
	listHeight := setupConsoleListHeight(*model, height)
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

func setupConsoleListHeight(model SetupConsoleViewModel, height int) int {
	if height < 12 {
		height = 12
	}
	listHeight := height - 10
	if model.Selected != nil {
		listHeight -= 5
	}
	if listHeight < 4 {
		listHeight = 4
	}
	return listHeight
}

func defaultSetupActionExecutor(ctx context.Context, plan setup.ActionPlan) error {
	_, err := setup.ExecuteActionPlan(ctx, plan, nil)
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

	names, err := store.ListSnapshots(a.runtime.StoreDir, nil)
	if err != nil {
		return bootMsg{err: err}
	}

	return bootMsg{
		evidence:        scanResult.Evidence,
		timelineEntries: entries,
		corruptEvents:   corrupt,
		snapshotNames:   names,
	}
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

// PreviewCompare builds a compare view model from current and saved snapshots.
func PreviewCompare(runtime types.RuntimeOptions, fromName, toLabel string) (*CompareViewModel, error) {
	fromSnap, err := store.ReadSnapshot(runtime.StoreDir, fromName, runtime.Agent)
	if err != nil {
		return nil, err
	}
	current, err := snapshot.CaptureCurrentState(&runtime, "current")
	if err != nil {
		return nil, err
	}
	graphDiff := diff.DiffGraphs(fromSnap.Graph, current.Snapshot.Graph)
	model := BuildCompareViewModel(BuildCompareViewModelInput{
		FromSnapshot: fromSnap,
		ToSnapshot:   current.Snapshot,
		Diff:         graphDiff,
		ToLabel:      toLabel,
		Scope:        "Full setup",
	})
	return &model, nil
}

// PreviewUndo builds a dry-run undo plan for a timeline entry id.
func PreviewUndo(storeDir, entryID string, onCorrupt func(store.TimelineCorruptEvent)) (*timelineundo.Plan, error) {
	return timelineundo.BuildPlan(storeDir, entryID, timelineundo.BuildOptions{
		OnCorruptEntry: onCorrupt,
	})
}
