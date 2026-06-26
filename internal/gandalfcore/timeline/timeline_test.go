package timeline_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/timeline"
	timelineundo "github.com/qyinm/gandalf/internal/gandalfcore/timeline_undo"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func makeRuntime(t *testing.T) (string, *types.RuntimeOptions) {
	t.Helper()
	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	homeDir := filepath.Join(root, "home")
	storeDir := filepath.Join(root, "store")
	for _, dir := range []string{projectPath, homeDir, storeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root, &types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
	}
}

func writeMCP(t *testing.T, projectPath, command string) {
	t.Helper()
	payload := map[string]any{
		"mcpServers": map[string]any{
			"github": map[string]any{"transport": "stdio", "command": command},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCreatesBaselinePartialChangesAndBuildsMcpOnlyDryRunUndo(t *testing.T) {
	t.Parallel()
	root, options := makeRuntime(t)
	writeMCP(t, options.ProjectPath, "gh-mcp")

	captureID := "capture-test"
	snapshotName := "manual-baseline-capture-test"
	baseline, err := timeline.CaptureSnapshot(options, &timeline.CaptureOptions{
		CaptureID:    &captureID,
		SnapshotName: &snapshotName,
	})
	if err != nil {
		t.Fatalf("baseline capture: %v", err)
	}
	if !baseline.Written || baseline.Entry == nil {
		t.Fatal("expected baseline entry")
	}
	if baseline.Entry.EventKind != types.TimelineEventBaseline {
		t.Fatalf("event kind = %s", baseline.Entry.EventKind)
	}
	if baseline.Entry.RestoreReadiness != types.TimelineRestoreObserveOnly {
		t.Fatalf("restore readiness = %s", baseline.Entry.RestoreReadiness)
	}

	writeMCP(t, options.ProjectPath, "gh-mcp-v2")

	changed, err := timeline.CaptureSnapshot(options, &timeline.CaptureOptions{
		CaptureID:     &captureID,
		SkipUnchanged: true,
	})
	if err != nil {
		t.Fatalf("changed capture: %v", err)
	}
	if !changed.Written || changed.Entry == nil {
		t.Fatal("expected changed entry")
	}
	if changed.Entry.EventKind != types.TimelineEventSetupChanged {
		t.Fatalf("event kind = %s", changed.Entry.EventKind)
	}
	if changed.Entry.RestoreReadiness != types.TimelineRestoreFull && changed.Entry.RestoreReadiness != types.TimelineRestorePartial {
		t.Fatalf("restore readiness = %s", changed.Entry.RestoreReadiness)
	}

	plan, err := timelineundo.BuildPlan(options.StoreDir, changed.Entry.ID, timelineundo.BuildOptions{})
	if err != nil {
		t.Fatalf("undo plan: %v", err)
	}
	if !plan.DryRun || plan.WritesFiles || len(plan.WritableItems) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.WritableItems[0].Action != timelineundo.ActionUpdate || plan.WritableItems[0].ServerName != "github" {
		t.Fatalf("writable item = %#v", plan.WritableItems[0])
	}

	entries, err := store.ListTimelineEntries(options.StoreDir, store.TimelineListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("timeline entries = %d", len(entries))
	}
	_ = root
}
