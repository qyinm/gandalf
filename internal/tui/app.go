package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
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

type undoPreviewMsg struct {
	plan *timelineundo.Plan
	err  error
}

// App is the Bubble Tea root model for the Gandalf setup-history workspace.
type App struct {
	runtime types.RuntimeOptions
	width   int
	height  int

	ready   bool
	errText string

	evidence        []types.DiscoveredItem
	timelineEntries []types.TimelineEntry
	corruptEvents   []store.TimelineCorruptEvent
	snapshotNames   []string

	screen          Screen
	selectedAgent   *types.AgentID
	selectedProfile string

	navCursor      int
	selectedNavID  string
	timelineCursor int

	undoPlan  *timelineundo.Plan
	undoError string
	notice    string

	compareModel   *CompareViewModel
	saveSetupModel *SaveSetupViewModel

	cachedNav    *NavigationModel
	cachedNavKey string
}

// NewApp creates a TUI app bound to engine runtime options.
func NewApp(runtime types.RuntimeOptions) *App {
	return &App{
		runtime:         runtime,
		screen:          ScreenTimeline,
		selectedNavID:   InitialNavItemID,
		selectedProfile: DefaultProfile,
	}
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
		a.evidence = typed.evidence
		a.timelineEntries = typed.timelineEntries
		a.corruptEvents = typed.corruptEvents
		a.snapshotNames = typed.snapshotNames
		a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
		return a, nil

	case rescanMsg:
		if typed.err != nil {
			a.notice = typed.err.Error()
			return a, nil
		}
		a.evidence = typed.evidence
		a.timelineEntries = typed.timelineEntries
		a.corruptEvents = typed.corruptEvents
		a.snapshotNames = typed.snapshotNames
		a.timelineCursor = ClampTimelineIndex(a.timelineCursor, a.filteredTimeline())
		a.undoPlan = nil
		a.undoError = ""
		a.notice = "Rescanned project setup."
		return a, nil

	case undoPreviewMsg:
		a.undoPlan = nil
		a.undoError = ""
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

	sidebarWidth := 28
	contentWidth := a.width - sidebarWidth - 3
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
		return "Loading Gandalf setup-history workspace..."
	}

	nav := a.navigationModel()
	sidebar := views.RenderSidebar(sidebarViewFromModel(nav), sidebarWidth, contentHeight)
	content := a.renderContent(contentWidth, contentHeight)

	header := lipgloss.NewStyle().Bold(true).Render("gandalf tui · setup-history workspace")
	if a.notice != "" {
		header += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(a.notice)
	}
	if a.undoError != "" {
		header += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(a.undoError)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(sidebarWidth).Render(sidebar),
		lipgloss.NewStyle().Width(contentWidth).Render(content),
	)

	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "q":
		return nil, true
	case "r":
		return func() tea.Msg {
			data := a.fetchWorkspaceData()
			return rescanMsg(data)
		}, false
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
			if len(corrupt) > 0 {
				a.corruptEvents = corrupt
			}
			return undoPreviewMsg{plan: plan}
		}, false
	case "up", "k":
		a.moveNavCursor(-1)
	case "down", "j":
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
		a.activateNavItem()
	}
	return nil, false
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
	a.selectedNavID = item.ID
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
