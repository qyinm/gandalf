package baseline

import (
	"sort"

	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/graph"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	_ "github.com/qyinm/gandalf/internal/gandalfcore/scan/plugins"
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
	scanResult := scan.ScanProject(&types.ScanOptions{
		ProjectPath: options.ProjectPath,
		HomeDir:     options.HomeDir,
		StoreDir:    options.StoreDir,
	})
	return BuildStatusFromEvidence(options, scanResult.Evidence)
}

// BuildStatusFromEvidence builds baseline drift from an already-scanned inventory.
// Callers that already ran ScanProject should use this to avoid a second walk.
func BuildStatusFromEvidence(options types.RuntimeOptions, evidence []types.DiscoveredItem) (Status, error) {
	out := Status{Agents: make([]AgentStatus, 0, len(agents.CurrentSupportedIDs()))}

	for _, agent := range agents.CurrentSupportedIDs() {
		filtered := filterAgentUserEvidence(evidence, agent)
		status := AgentStatus{
			Agent:            agent,
			UnsupportedCount: countUnsupported(filtered),
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
			status.Diff = diff.DiffGraphs(latest.Graph, graph.BuildGraph(filtered))
			status.SemanticChangeCount = len(status.Diff.SemanticChanges)
			status.RawChangeCount = len(status.Diff.RawSourceChanges)
		}
		out.Agents = append(out.Agents, status)
	}

	return out, nil
}

func filterAgentUserEvidence(evidence []types.DiscoveredItem, agent types.AgentID) []types.DiscoveredItem {
	filtered := make([]types.DiscoveredItem, 0, len(evidence))
	for _, item := range evidence {
		if item.Agent != agent || item.Scope != types.ScopeUser {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func latestSnapshot(storeDir string, agent types.AgentID) (*types.Snapshot, error) {
	names, err := store.ListSnapshots(storeDir, &agent)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}

	type ranked struct {
		name      string
		createdAt string
	}
	candidates := make([]ranked, 0, len(names))
	for _, name := range names {
		if store.IsRestorePointSnapshotName(name) {
			continue
		}
		manifest, err := store.ReadSnapshotManifest(storeDir, name, &agent)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, ranked{name: name, createdAt: manifest.CreatedAt})
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].createdAt > candidates[j].createdAt
	})
	snap, err := store.ReadSnapshotDiffState(storeDir, candidates[0].name, &agent)
	if err != nil {
		return nil, err
	}
	return &snap, nil
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
