package baseline

import (
	"encoding/json"
	"sort"

	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// Status summarizes baseline coverage for the current supported agent set.
type Status struct {
	Agents []AgentStatus
}

// AgentStatus summarizes the latest baseline and current drift for one agent.
type AgentStatus struct {
	Agent               types.AgentID
	HasBaseline         bool
	BaselineName        string
	BaselineCreatedAt   string
	ContentBacked       bool
	SemanticChangeCount int
	RawChangeCount      int
	UnsupportedCount    int
	OmittedContentCount int
	Diff                diff.GraphDiff
}

// ChangeCount returns the total observed graph/source changes since baseline.
func (s AgentStatus) ChangeCount() int {
	return s.SemanticChangeCount + s.RawChangeCount
}

// BuildStatus compares each current supported agent's latest user baseline with current state.
func BuildStatus(options types.RuntimeOptions) (Status, error) {
	scope := types.ScopeUser
	out := Status{Agents: make([]AgentStatus, 0, len(agents.CurrentSupportedIDs()))}

	for _, agent := range agents.CurrentSupportedIDs() {
		currentRuntime := options
		currentRuntime.Agent = &agent
		currentRuntime.Scope = &scope
		currentRuntime.CaptureContent = agents.SupportsContentBackedUserSnapshot(agent, scope)

		current, err := snapshot.CaptureCurrentState(&currentRuntime, "current")
		if err != nil {
			return Status{}, err
		}

		status := AgentStatus{
			Agent:               agent,
			UnsupportedCount:    countUnsupported(current.Snapshot.Evidence),
			OmittedContentCount: countOmittedContent(current.Snapshot),
		}

		latest, err := latestSnapshot(options.StoreDir, agent)
		if err != nil {
			return Status{}, err
		}
		if latest != nil {
			status.HasBaseline = true
			status.BaselineName = latest.Manifest.Name
			status.BaselineCreatedAt = latest.Manifest.CreatedAt
			status.ContentBacked = hasCapturedContent(*latest)
			status.Diff = diff.DiffGraphs(latest.Graph, current.Snapshot.Graph)
			status.SemanticChangeCount = len(status.Diff.SemanticChanges)
			status.RawChangeCount = len(status.Diff.RawSourceChanges)
		}
		out.Agents = append(out.Agents, status)
	}

	return out, nil
}

func latestSnapshot(storeDir string, agent types.AgentID) (*types.Snapshot, error) {
	names, err := store.ListSnapshots(storeDir, &agent)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}

	snapshots := make([]types.Snapshot, 0, len(names))
	for _, name := range names {
		snap, err := store.ReadSnapshot(storeDir, name, &agent)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Manifest.CreatedAt > snapshots[j].Manifest.CreatedAt
	})
	return &snapshots[0], nil
}

func hasCapturedContent(snapshot types.Snapshot) bool {
	for _, entry := range snapshot.Content {
		if entry.CaptureStatus == "captured" {
			return true
		}
	}
	return false
}

func countUnsupported(evidence []types.DiscoveredItem) int {
	count := 0
	for _, item := range evidence {
		if item.Kind == types.KindUnsupported || item.RestorePolicy == types.RestoreNotSupported {
			count++
		}
	}
	return count
}

func countOmittedContent(snapshot types.Snapshot) int {
	seen := map[string]struct{}{}
	for _, entry := range snapshot.Content {
		if entry.CaptureStatus == "omitted" {
			seen[entry.EvidenceID] = struct{}{}
		}
	}
	for _, item := range snapshot.Evidence {
		if contentCaptureStatus(item) == "omitted" {
			seen[item.ID] = struct{}{}
		}
	}
	return len(seen)
}

func contentCaptureStatus(item types.DiscoveredItem) string {
	if len(item.Metadata) == 0 {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal(item.Metadata, &meta); err != nil {
		return ""
	}
	status, _ := meta["contentCaptureStatus"].(string)
	return status
}
