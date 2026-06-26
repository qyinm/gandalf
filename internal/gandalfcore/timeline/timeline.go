package timeline

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// Error represents timeline operation failures.
type Error struct {
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// CaptureOptions configures timeline snapshot capture.
type CaptureOptions struct {
	CaptureID     *string
	SnapshotName  *string
	Title         *string
	SkipUnchanged bool
}

// CaptureResult is returned after timeline capture.
type CaptureResult struct {
	Written       bool
	Entry         *types.TimelineEntry
	State         *types.CurrentState
	Diff          *diff.GraphDiff
	SkippedReason *string
}

// CaptureSnapshot captures current state and appends a timeline entry when changed.
func CaptureSnapshot(options *types.RuntimeOptions, captureOptions *CaptureOptions) (*CaptureResult, error) {
	previous, err := store.LatestTimelineEntry(options.StoreDir, store.TimelineListOptions{
		Agent:       options.Agent,
		ProjectPath: options.ProjectPath,
	})
	if err != nil {
		return nil, &Error{Message: "read previous timeline entry", Cause: err}
	}

	captureID := shortID()
	if captureOptions != nil && captureOptions.CaptureID != nil {
		captureID = *captureOptions.CaptureID
	}
	snapshotName := TimelineSnapshotName(captureID, options.Agent)
	if captureOptions != nil && captureOptions.SnapshotName != nil {
		snapshotName = *captureOptions.SnapshotName
	}

	state, err := snapshot.CaptureCurrentState(options, snapshotName)
	if err != nil {
		return nil, &Error{Message: "capture current state", Cause: err}
	}

	var graphDiff *diff.GraphDiff
	var diffError *string
	if previous != nil {
		previousSnapshot, err := store.ReadSnapshot(options.StoreDir, previous.AfterSnapshotName, previous.Agent)
		if err != nil {
			msg := err.Error()
			diffError = &msg
		} else {
			d := diff.DiffGraphs(previousSnapshot.Graph, state.Snapshot.Graph)
			graphDiff = &d
		}
	}

	if previous != nil && graphDiff != nil && captureOptions != nil && captureOptions.SkipUnchanged &&
		len(graphDiff.SemanticChanges) == 0 && len(graphDiff.RawSourceChanges) == 0 {
		reason := "unchanged"
		return &CaptureResult{
			Written:       false,
			State:         state,
			Diff:          graphDiff,
			SkippedReason: &reason,
		}, nil
	}

	changedSurfaces := changedSurfacesForDiff(graphDiff)
	restoreReadiness := restoreReadinessFor(changedSurfaces)
	title := titleForDiff(graphDiff, options.Agent)
	if captureOptions != nil && captureOptions.Title != nil {
		title = *captureOptions.Title
	}

	entry := types.TimelineEntry{
		SchemaVersion:      "0.1",
		ID:                 shortID(),
		Source:             types.TimelineSourceManual,
		EventKind:          eventKindFor(previous, graphDiff),
		Title:              title,
		ProjectPath:        state.Snapshot.Manifest.ProjectPath,
		Agent:              options.Agent,
		Agents:             agentsForState(state),
		BeforeSnapshotName: nil,
		AfterSnapshotName:  snapshotName,
		CaptureID:          captureID,
		CreatedAt:          time.Now().UTC().Format(time.RFC3339),
		ObservedAt:         state.Snapshot.Manifest.CreatedAt,
		ChangedSurfaces:    changedSurfaces,
		RestoreReadiness:   restoreReadiness,
		Confidence:         confidenceFor(graphDiff, changedSurfaces, diffError),
		ConfidenceReason:   confidenceReasonFor(graphDiff, changedSurfaces, diffError),
		EvidenceCount:      uint32(len(state.Snapshot.Evidence)),
		GraphNodeCount:     uint32(len(state.Snapshot.Graph)),
		AuditFindingCount:  uint32(len(state.Snapshot.AuditFindings)),
	}
	if previous != nil {
		entry.BeforeSnapshotName = &previous.AfterSnapshotName
	}
	entry.Changes = types.TimelineChangeSummary{
		HasChanges: true,
		Highlights: highlightsForDiff(graphDiff),
	}
	if previous != nil {
		entry.Changes.PreviousEntryID = &previous.ID
		entry.Changes.PreviousSnapshotName = &previous.AfterSnapshotName
	}
	if graphDiff != nil {
		entry.Changes.HasChanges = len(graphDiff.SemanticChanges) > 0 || len(graphDiff.RawSourceChanges) > 0
		entry.Changes.SemanticChangeCount = uint32(len(graphDiff.SemanticChanges))
		entry.Changes.RawSourceChangeCount = uint32(len(graphDiff.RawSourceChanges))
	}

	if err := store.WriteSnapshot(options.StoreDir, store.StoreSnapshotFrom(state.Snapshot), options.Agent); err != nil {
		return nil, &Error{Message: "write snapshot", Cause: err}
	}
	if err := store.AppendTimelineEntry(options.StoreDir, &entry); err != nil {
		return nil, &Error{Message: "append timeline entry", Cause: err}
	}

	return &CaptureResult{
		Written: true,
		Entry:   &entry,
		State:   state,
		Diff:    graphDiff,
	}, nil
}

// TimelineSnapshotName builds a history snapshot name.
func TimelineSnapshotName(captureID string, agent *types.AgentID) string {
	timestamp := strings.NewReplacer(":", "-", ".", "-").Replace(time.Now().UTC().Format(time.RFC3339))
	parts := []string{"history", captureID}
	if agent != nil {
		parts = append(parts, agent.String())
	}
	parts = append(parts, timestamp, shortID())
	return strings.Join(parts, "-")
}

func shortID() string {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

func agentsForState(state *types.CurrentState) []types.AgentID {
	seen := make(map[types.AgentID]struct{})
	for _, item := range state.Snapshot.Evidence {
		seen[item.Agent] = struct{}{}
	}
	agents := make([]types.AgentID, 0, len(seen))
	for agent := range seen {
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].String() < agents[j].String()
	})
	return agents
}

func titleForDiff(graphDiff *diff.GraphDiff, agent *types.AgentID) string {
	if graphDiff == nil {
		return scopedTitle("baseline setup", agent)
	}
	if len(graphDiff.SemanticChanges) == 0 && len(graphDiff.RawSourceChanges) == 0 {
		return scopedTitle("unchanged setup", agent)
	}
	sorted := append([]diff.SemanticChange(nil), graphDiff.SemanticChanges...)
	sort.Slice(sorted, func(i, j int) bool {
		return priorityForChange(&sorted[i]) < priorityForChange(&sorted[j])
	})
	if len(sorted) == 0 {
		return scopedTitle("update setup files", agent)
	}
	first := sorted[0]
	switch first.Code {
	case diff.SemanticMcpAdded:
		return scopedTitle("add "+first.EntityName+" mcp", agent)
	case diff.SemanticMcpRemoved:
		return scopedTitle("remove "+first.EntityName+" mcp", agent)
	case diff.SemanticMcpChanged:
		return scopedTitle("update "+first.EntityName+" mcp", agent)
	case diff.SemanticEnvKeyAdded:
		return scopedTitle("add "+first.EntityName+" env key", agent)
	case diff.SemanticEnvKeyRemoved:
		return scopedTitle("remove "+first.EntityName+" env key", agent)
	case diff.SemanticSkillAdded:
		return scopedTitle("install "+first.EntityName+" skill", agent)
	case diff.SemanticSkillRemoved:
		return scopedTitle("remove "+first.EntityName+" skill", agent)
	case diff.SemanticSkillExecutableAppeared:
		return scopedTitle("update "+first.EntityName+" skill", agent)
	}
	title := "update setup files"
	switch first.Code {
	case diff.SemanticAgentConfigAdded, diff.SemanticAgentConfigRemoved, diff.SemanticAgentConfigChanged:
		title = "update config"
	case diff.SemanticPermissionWildcardAdded, diff.SemanticPermissionChanged:
		title = "update permissions"
	case diff.SemanticHookAdded, diff.SemanticHookRemoved, diff.SemanticHookChanged:
		title = "update hooks"
	case diff.SemanticInstructionChanged:
		title = "update project instructions"
	case diff.SemanticUnsupportedStateChanged:
		title = "update unsupported setup"
	}
	return scopedTitle(title, agent)
}

func changedSurfacesForDiff(graphDiff *diff.GraphDiff) []types.TimelineChangedSurface {
	if graphDiff == nil {
		return nil
	}
	var surfaces []types.TimelineChangedSurface
	semanticSourcePaths := make(map[string]struct{})
	for _, change := range graphDiff.SemanticChanges {
		kind := timelineSurfaceKind(change)
		path := "unknown"
		if change.Details.SourcePath != nil {
			path = *change.Details.SourcePath
		}
		semanticSourcePaths[path] = struct{}{}
		restorable := kind == "mcp_server" && strings.HasSuffix(path, ".mcp.json")
		entityName := change.EntityName
		surfaces = append(surfaces, types.TimelineChangedSurface{
			Kind:        kind,
			ChangeType:  change.Code.String(),
			Path:        path,
			EntityName:  &entityName,
			Restorable:  restorable,
			ObserveOnly: !restorable,
			Before:      change.Before,
			After:       change.After,
		})
	}
	for _, change := range graphDiff.RawSourceChanges {
		if _, seen := semanticSourcePaths[change.SourcePath]; seen {
			continue
		}
		surfaces = append(surfaces, types.TimelineChangedSurface{
			Kind:        "other",
			ChangeType:  "RAW_" + strings.ToUpper(change.Status),
			Path:        change.SourcePath,
			Restorable:  false,
			ObserveOnly: true,
		})
	}
	return surfaces
}

func eventKindFor(previous *types.TimelineEntry, graphDiff *diff.GraphDiff) types.TimelineEntryEventKind {
	if previous == nil {
		return types.TimelineEventBaseline
	}
	if graphDiff != nil && len(graphDiff.SemanticChanges) == 0 && len(graphDiff.RawSourceChanges) == 0 {
		return types.TimelineEventUnchanged
	}
	return types.TimelineEventSetupChanged
}

func timelineSurfaceKind(change diff.SemanticChange) string {
	switch change.EntityKind {
	case types.KindMcpServer:
		return "mcp_server"
	case types.KindSkill:
		return "skill"
	case types.KindPermission:
		return "permission"
	case types.KindHook:
		return "hook"
	case types.KindEnvKey:
		return "env_key"
	case types.KindUnsupported:
		return "unsupported"
	default:
		return "other"
	}
}

func restoreReadinessFor(surfaces []types.TimelineChangedSurface) types.TimelineRestoreReadiness {
	if len(surfaces) == 0 {
		return types.TimelineRestoreObserveOnly
	}
	restorable := 0
	for _, surface := range surfaces {
		if surface.Restorable {
			restorable++
		}
	}
	if restorable == len(surfaces) {
		return types.TimelineRestoreFull
	}
	if restorable > 0 {
		return types.TimelineRestorePartial
	}
	return types.TimelineRestoreObserveOnly
}

func confidenceFor(graphDiff *diff.GraphDiff, surfaces []types.TimelineChangedSurface, diffError *string) types.TimelineConfidence {
	if diffError != nil {
		return types.TimelineConfidenceLow
	}
	if graphDiff == nil {
		return types.TimelineConfidenceHigh
	}
	for _, surface := range surfaces {
		if surface.Path == "unknown" {
			return types.TimelineConfidenceMedium
		}
	}
	return types.TimelineConfidenceHigh
}

func confidenceReasonFor(graphDiff *diff.GraphDiff, surfaces []types.TimelineChangedSurface, diffError *string) string {
	if diffError != nil {
		return "previous snapshot could not be diffed: " + *diffError
	}
	if graphDiff == nil {
		return "first manual history baseline"
	}
	if len(surfaces) == 0 {
		return "no semantic or raw source changes"
	}
	for _, surface := range surfaces {
		if surface.Path == "unknown" {
			return "some changes lacked source path metadata"
		}
	}
	return "derived from snapshot graph diff"
}

func priorityForChange(change *diff.SemanticChange) string {
	code := change.Code.String()
	if strings.HasPrefix(code, "MCP_") {
		return "0-" + change.EntityName
	}
	if code == "SKILL_EXECUTABLE_APPEARED" {
		return "1-" + change.EntityName
	}
	if code == "PERMISSION_WILDCARD_ADDED" {
		return "2-" + change.EntityName
	}
	if strings.HasPrefix(code, "ENV_KEY_") {
		return "4-" + change.EntityName
	}
	return "5-" + change.EntityName
}

func scopedTitle(title string, agent *types.AgentID) string {
	if agent == nil {
		return title
	}
	return fmt.Sprintf("%s for %s", title, agents.DisplayName(*agent))
}

func highlightsForDiff(graphDiff *diff.GraphDiff) []string {
	if graphDiff == nil {
		return nil
	}
	var highlights []string
	for i, change := range graphDiff.SemanticChanges {
		if i >= 5 {
			break
		}
		highlights = append(highlights, change.Code.String()+": "+change.EntityName)
	}
	remaining := 8 - len(highlights)
	for i, change := range graphDiff.RawSourceChanges {
		if i >= remaining {
			break
		}
		highlights = append(highlights, change.Status+": "+change.SourcePath)
	}
	return highlights
}
