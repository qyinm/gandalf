package tui

import (
	"context"
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
	"github.com/charmbracelet/lipgloss"
	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/baseline"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
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
	snapshotNames   []string
	snapshotRefs    []snapshotRef
	baselineStatus  baseline.Status
	err             error
}

type rescanMsg bootMsg

type setupActionMsg struct {
	data bootMsg
	err  error
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
	snapshotNames   []string
	snapshotRefs    []snapshotRef
	baselineStatus  baseline.Status

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
	snapshotCursor     int
	environmentCursor  int

	undoPlan       *timelineundo.Plan
	undoError      string
	notice         string
	actionError    string
	pendingAction  *setup.ActionPlan
	skillViewer    *skillMarkdownViewerState
	rollbackReview *rollbackReview

	compareModel   *CompareViewModel
	saveSetupModel *SaveSetupViewModel

	cachedNav    *NavigationModel
	cachedNavKey string

	actionExecutor      func(context.Context, setup.ActionPlan) error
	restorePointCreator func(types.AgentID) (string, error)
	restoreExecutor     restore.RestoreExecutor
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

	case baselineCreateMsg:
		if typed.err != nil {
			a.actionError = typed.err.Error()
			return a, nil
		}
		if typed.data.err != nil {
			a.actionError = "Created baseline, but failed to rescan: " + typed.data.err.Error()
			return a, nil
		}
		a.applyWorkspaceData(typed.data)
		a.actionError = ""
		if len(typed.created) == 0 {
			a.notice = "Supported baselines already exist."
		} else {
			a.notice = "Created baselines: " + strings.Join(typed.created, ", ")
		}
		return a, nil

	case rollbackPreviewMsg:
		a.rollbackReview = nil
		a.actionError = ""
		if typed.err != nil {
			a.actionError = typed.err.Error()
			return a, nil
		}
		a.rollbackReview = typed.review
		return a, nil

	case rollbackApplyMsg:
		if typed.err != nil {
			a.actionError = typed.err.Error()
			return a, nil
		}
		if typed.data.err != nil {
			a.rollbackReview = nil
			a.actionError = "Applied rollback, but failed to rescan: " + typed.data.err.Error()
			return a, nil
		}
		a.applyWorkspaceData(typed.data)
		a.rollbackReview = nil
		a.actionError = ""
		a.notice = fmt.Sprintf("Applied rollback. Restore point: %s. %s", typed.restorePoint, typed.verify)
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
	contentHeight := a.height

	if !a.ready {
		if a.errText != "" {
			return views.RenderHistory(views.HistoryView{
				EmptyMessage: "Failed to load workspace.",
				EmptyCommand: a.errText,
			}, contentWidth, contentHeight)
		}
		return "Loading Gandalf global setup workspace..."
	}

	header := views.RenderHeader(a.headerView(), contentWidth)
	statusParts := make([]string, 0, 2)
	if a.notice != "" {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(a.notice))
	}
	if a.undoError != "" {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(a.undoError))
	}
	status := strings.Join(statusParts, "  ")

	// Reserve header (1) + divider (1) for the body height.
	headerLines := strings.Count(header, "\n") + 1
	bodyHeight := contentHeight - headerLines - 1
	if status != "" {
		bodyHeight--
	}
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	body := a.renderContent(contentWidth, bodyHeight)
	return views.RenderFrame(header, body, status, contentWidth, contentHeight)
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
		})
	}
	return views.HeaderView{
		Title: "Gandalf",
		Scope: scope,
		Chips: chips,
	}
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
		if a.rollbackReview != nil {
			a.rollbackReview = nil
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
	case "B":
		if a.screen == ScreenInventory && a.pendingAction == nil {
			return func() tea.Msg {
				created, err := a.createMissingBaselines()
				if err != nil {
					return baselineCreateMsg{err: err}
				}
				return baselineCreateMsg{created: created, data: a.fetchWorkspaceData()}
			}, false
		}
	case "H":
		if a.screen == ScreenInventory {
			a.screen = ScreenTimeline
			a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
		}
	case "S":
		if a.screen == ScreenInventory {
			a.screen = ScreenSnapshots
		}
	case "E":
		if a.screen == ScreenInventory || a.screen == ScreenSnapshots || a.screen == ScreenTimeline {
			a.screen = ScreenEnvironments
			a.environmentCursor = clampIndex(a.environmentCursor, len(a.baselineStatus.Agents))
			a.actionError = ""
		}
	case "s":
		if a.screen == ScreenEnvironments {
			return a.saveFocusedEnvironment(), false
		}
	case "R":
		if a.screen == ScreenEnvironments {
			return a.restoreFocusedEnvironment(), false
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
		if a.screen == ScreenSnapshots {
			a.moveSnapshotCursor(-1)
			return nil, false
		}
		if a.screen == ScreenEnvironments {
			a.moveEnvironmentCursor(-1)
			return nil, false
		}
		a.moveNavCursor(-1)
	case "down", "j":
		if a.screen == ScreenInventory && a.inventoryFocus && a.pendingAction == nil {
			a.moveInventoryCursor(1)
			return nil, false
		}
		if a.screen == ScreenSnapshots {
			a.moveSnapshotCursor(1)
			return nil, false
		}
		if a.screen == ScreenEnvironments {
			a.moveEnvironmentCursor(1)
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
		if a.screen == ScreenSnapshots {
			return a.handleSnapshotEnter(), false
		}
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

func (a *App) handleSkillEnter() {
	inventory := a.currentInventory()
	if len(inventory) == 0 {
		a.actionError = "No skill selected."
		return
	}
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	if a.expandedSetupRowID(SetupConsoleTabSkills) == item.ID {
		a.openSelectedSkillViewer()
		return
	}
	a.setExpandedSetupRowID(SetupConsoleTabSkills, item.ID)
	a.actionError = ""
}

func (a *App) handleMCPEnter() {
	model := a.currentSetupConsoleViewModel()
	if len(model.Rows) == 0 {
		a.actionError = "No MCP server selected."
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
		a.actionError = ""
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

// toggleSelectedMCPServer flips the enable/disable state of the selected MCP
// server. Available only for JSON-backed user-scope servers; otherwise it
// surfaces the gated reason without mutating anything.
func (a *App) toggleSelectedMCPServer(rowID string) tea.Cmd {
	item, ok := a.inventoryItemByID(rowID)
	if !ok {
		a.actionError = "No MCP server selected."
		return nil
	}
	plan := setup.PlanItemAction(item, setup.ActionToggle)
	if !plan.Available {
		a.actionError = plan.UnavailableReason
		return nil
	}
	home := a.runtime.HomeDir
	serverName := item.Name
	configPath := item.SourcePath
	return func() tea.Msg {
		if _, err := setup.ExecuteMCPToggle(plan, home, serverName, configPath); err != nil {
			return setupActionMsg{err: err}
		}
		return setupActionMsg{data: a.fetchWorkspaceData()}
	}
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
		a.actionError = "No setup item selected."
		return
	}
	activeTab := normalizeSetupConsoleTab(a.activeSetupTab)
	item := inventory[clampIndex(a.inventoryCursor, len(inventory))]
	a.setExpandedSetupRowID(activeTab, item.ID)
	a.actionError = ""
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
	a.actionError = ""
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
	a.actionError = ""
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
	a.snapshotRefs = data.snapshotRefs
	a.baselineStatus = data.baselineStatus
	a.cachedNav = nil
	a.cachedNavKey = ""
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
		ExpandedRowID:      a.expandedSetupRowID(a.activeSetupTab),
		ExpandedToolID:     a.setupConsole.expandedMCPTool,
		PendingAction:      a.pendingAction,
		ActionError:        a.actionError,
		BaselineStatus:     baselineStatusPtr(a.baselineStatus),
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
	a.actionError = ""
}

func (a *App) moveEnvironmentCursor(delta int) {
	agents := a.baselineStatus.Agents
	if len(agents) == 0 {
		a.environmentCursor = 0
		return
	}
	next := a.environmentCursor + delta
	if next < 0 {
		next = len(agents) - 1
	}
	if next >= len(agents) {
		next = 0
	}
	a.environmentCursor = next
	a.actionError = ""
}

func (a *App) focusedEnvironmentAgent() (types.AgentID, bool) {
	agents := a.baselineStatus.Agents
	if len(agents) == 0 {
		return "", false
	}
	return agents[clampIndex(a.environmentCursor, len(agents))].Agent, true
}

// saveFocusedEnvironment captures the focused agent's current setup as a new
// snapshot, reusing the same capture path as baseline creation.
func (a *App) saveFocusedEnvironment() tea.Cmd {
	agent, ok := a.focusedEnvironmentAgent()
	if !ok {
		a.actionError = "No environment selected."
		return nil
	}
	return func() tea.Msg {
		scope := types.ScopeUser
		runtime := a.runtime
		runtime.Agent = &agent
		runtime.Scope = &scope
		runtime.CaptureContent = agents.SupportsContentBackedUserSnapshot(agent, scope)
		name := fmt.Sprintf("snapshot-%s-%s", agent.String(), time.Now().UTC().Format("20060102-150405-000000000"))
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
		a.actionError = "No environment selected."
		return nil
	}
	var ref *snapshotRef
	for i := range a.snapshotRefs {
		if a.snapshotRefs[i].Agent == agent {
			ref = &a.snapshotRefs[i]
			break
		}
	}
	if ref == nil {
		a.actionError = "No saved snapshot for this agent yet. Press s to save one."
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
		a.actionError = "No supported snapshots selected."
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
				HasPreviousSnapshot: a.hasSnapshots(),
			})
			a.saveSetupModel = &model
		}
		return views.RenderSaveSetup(saveSetupViewFromModel(*a.saveSetupModel), width, height)
	case ScreenSnapshots:
		return a.renderSnapshots(width, height)
	case ScreenEnvironments:
		model := BuildEnvironmentsViewModel(BuildEnvironmentsViewModelInput{
			Status:        a.baselineStatus,
			SelectedIndex: a.environmentCursor,
		})
		return views.RenderEnvironments(environmentsViewFromModel(model), width, height)
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
			a.snapshotCount(), strings.Join(agentLabels, ", "), changedAt)
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

func (a *App) hasSnapshots() bool {
	return a.snapshotCount() > 0
}

func (a *App) snapshotCount() int {
	if len(a.snapshotRefs) > 0 {
		return len(a.snapshotRefs)
	}
	return len(a.snapshotNames)
}

func (a *App) renderSnapshots(width, height int) string {
	if a.rollbackReview != nil {
		return a.renderRollbackReview(width, height)
	}
	if len(a.snapshotRefs) == 0 {
		return "Saved setups\n\nNo supported Codex or Claude Code snapshots yet.\n\nB create baselines"
	}
	lines := []string{"Saved setups", ""}
	for i, ref := range a.snapshotRefs {
		prefix := "  "
		if i == clampIndex(a.snapshotCursor, len(a.snapshotRefs)) {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s  %s  %s", prefix, FormatAgentMarker(ref.Agent), ref.Name, formatDate(ref.CreatedAt)))
	}
	lines = append(lines, "", "enter review changes  B create missing baselines  esc cancel")
	return fitLines(lines, width, height)
}

func (a *App) renderRollbackReview(width, height int) string {
	review := a.rollbackReview
	lines := []string{
		"Review Changes",
		"",
		fmt.Sprintf("Rollback %s to %s", FormatAgentLabel(review.Agent), review.SnapshotName),
		"Restore point will be created before apply.",
		"",
		"Changes:",
	}
	if len(review.Items) == 0 {
		lines = append(lines, "  No supported changes.")
	}
	for _, item := range review.Items {
		action := ""
		if item.Action != nil {
			action = string(*item.Action)
		}
		lines = append(lines, fmt.Sprintf("  %s  %s  %s", action, item.ItemType, item.Dest))
	}
	if len(review.UnsupportedItems) > 0 {
		lines = append(lines, "", "Unsupported:")
		for _, item := range review.UnsupportedItems {
			lines = append(lines, fmt.Sprintf("  %s  %s  %s", item.Kind, item.SourcePath, item.Reason))
		}
	}
	lines = append(lines, "", "enter apply  esc cancel")
	return fitLines(lines, width, height)
}

func fitLines(lines []string, width, height int) string {
	if width > 0 {
		for i, line := range lines {
			lines[i] = TruncateText(line, width)
		}
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
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
		snapshotNames:   names,
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
	createRestorePoint := a.createPreApplyRestorePoint
	if a.restorePointCreator != nil {
		createRestorePoint = a.restorePointCreator
	}
	restorePoint, err := createRestorePoint(review.Agent)
	if err != nil {
		return rollbackApplyMsg{err: fmt.Errorf("failed to create pre-apply restore point: %w", err)}
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
			err:          fmt.Errorf("rollback apply failed: %d failed", summary.Failed),
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
		return "Verified selected baseline."
	}
	return fmt.Sprintf("Verification found %d remaining supported changes.", len(next.Plan.Items))
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
