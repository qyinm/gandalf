package timelineundo

import (
	"encoding/json"
	"fmt"

	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/timeline"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// Action identifies an undo operation.
type Action string

const (
	ActionAdd    Action = "add"
	ActionRemove Action = "remove"
	ActionUpdate Action = "update"
)

func (a Action) String() string { return string(a) }

// Item is a single dry-run undo change.
type Item struct {
	Action       Action          `json:"action"`
	Kind         string          `json:"kind"`
	Path         string          `json:"path"`
	ServerName   string          `json:"serverName"`
	TargetValue  json.RawMessage `json:"targetValue,omitempty"`
	CurrentValue json.RawMessage `json:"currentValue,omitempty"`
}

// Plan is a dry-run MCP undo plan for a timeline entry.
type Plan struct {
	EntryID             string                         `json:"entryId"`
	Title               string                         `json:"title"`
	DryRun              bool                           `json:"dryRun"`
	WritesFiles         bool                           `json:"writesFiles"`
	RestoreReadiness    types.TimelineRestoreReadiness `json:"restoreReadiness"`
	TargetSnapshotName  *string                        `json:"targetSnapshotName,omitempty"`
	CurrentSnapshotName string                         `json:"currentSnapshotName"`
	WritableItems       []Item                         `json:"writableItems"`
	ObserveOnlySurfaces []types.TimelineChangedSurface `json:"observeOnlySurfaces"`
}

// BuildOptions configures undo plan construction.
type BuildOptions struct {
	OnCorruptEntry func(store.TimelineCorruptEvent)
}

// BuildPlan constructs a dry-run MCP undo plan for a timeline entry reference.
func BuildPlan(storeDir, reference string, options BuildOptions) (*Plan, error) {
	listOptions := store.TimelineListOptions{
		OnCorruptEntry: options.OnCorruptEntry,
	}
	entry, err := store.FindTimelineEntry(storeDir, reference, listOptions)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, &timeline.Error{Message: "Timeline entry not found: " + reference}
	}

	var writableItems []Item
	var observeOnly []types.TimelineChangedSurface
	for _, surface := range entry.ChangedSurfaces {
		if surface.Restorable && surface.Kind == "mcp_server" {
			writableItems = append(writableItems, undoItemForMcpSurface(&surface))
		} else {
			observeOnly = append(observeOnly, surface)
		}
	}

	return &Plan{
		EntryID:             entry.ID,
		Title:               fmt.Sprintf("dry-run MCP undo: %s", entry.Title),
		DryRun:              true,
		WritesFiles:         false,
		RestoreReadiness:    entry.RestoreReadiness,
		TargetSnapshotName:  entry.BeforeSnapshotName,
		CurrentSnapshotName: entry.AfterSnapshotName,
		WritableItems:       writableItems,
		ObserveOnlySurfaces: observeOnly,
	}, nil
}

func undoItemForMcpSurface(surface *types.TimelineChangedSurface) Item {
	serverName := "unknown"
	if surface.EntityName != nil {
		serverName = *surface.EntityName
	}
	if surface.ChangeType == "MCP_ADDED" {
		return Item{
			Action:       ActionRemove,
			Kind:         "mcp_server",
			Path:         surface.Path,
			ServerName:   serverName,
			CurrentValue: surface.After,
		}
	}
	if surface.ChangeType == "MCP_REMOVED" {
		return Item{
			Action:      ActionAdd,
			Kind:        "mcp_server",
			Path:        surface.Path,
			ServerName:  serverName,
			TargetValue: surface.Before,
		}
	}
	return Item{
		Action:       ActionUpdate,
		Kind:         "mcp_server",
		Path:         surface.Path,
		ServerName:   serverName,
		TargetValue:  surface.Before,
		CurrentValue: surface.After,
	}
}
