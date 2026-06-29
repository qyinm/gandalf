package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func tempStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func testSnapshot(name string) types.Snapshot {
	checksum := "sha256:observed-config"
	effectiveValue := json.RawMessage(`{"permissions":["Bash(bun test)"]}`)
	evidenceID := "claude.project.settings"
	path := ".claude/settings.json"

	return types.Snapshot{
		Manifest: types.SnapshotManifest{
			SchemaVersion: "0.1",
			Name:          name,
			CreatedAt:     "2026-05-12T00:00:00.000Z",
			ProjectPath:   "/tmp/project",
			Security: types.SnapshotSecurity{
				RawSecretsIncluded: false,
				RedactionPolicy:    "metadata-only",
			},
		},
		Evidence: []types.DiscoveredItem{{
			ID:            evidenceID,
			Agent:         types.AgentClaudeCode,
			Kind:          types.KindAgentConfig,
			SourcePath:    path,
			Scope:         types.ScopeProject,
			Precedence:    40,
			Parser:        types.ParserJSON,
			Sensitivity:   "command_config",
			ContentPolicy: "structured_safe_fields_only",
			RestorePolicy: types.RestoreNotSupported,
			CaptureStatus: types.CaptureCaptured,
			Confidence:    types.ConfidenceHigh,
			Checksum:      &checksum,
		}},
		Graph: []types.GraphNode{{
			ID:             "node.claude.project.settings",
			Agent:          types.AgentClaudeCode,
			Scope:          types.ScopeProject,
			SourcePath:     path,
			EntityKind:     types.KindAgentConfig,
			EntityName:     "settings",
			EffectiveValue: effectiveValue,
			Confidence:     types.ConfidenceHigh,
			EvidenceID:     evidenceID,
		}},
		AuditFindings: []types.AuditFinding{{
			Code:       "EXECUTABLE_CONFIG_ADDED",
			Severity:   types.SeverityMedium,
			Problem:    "Project config allows an executable command.",
			Cause:      ".claude/settings.json allows Bash(bun test).",
			Fix:        "Review the allowed command before sharing the project config.",
			Path:       &path,
			EvidenceID: &evidenceID,
		}},
		Provenance: []types.ProvenanceEntry{{
			NodeID:        "node.claude.project.settings",
			EvidenceID:    evidenceID,
			SourcePath:    path,
			Scope:         types.ScopeProject,
			Precedence:    40,
			Confidence:    types.ConfidenceHigh,
			CaptureStatus: types.CaptureCaptured,
		}},
	}
}

func snapshotsEqual(t *testing.T, got, want types.Snapshot) bool {
	t.Helper()
	if got.Manifest != want.Manifest {
		return false
	}
	if len(got.Evidence) != len(want.Evidence) {
		return false
	}
	for i := range got.Evidence {
		g := got.Evidence[i]
		w := want.Evidence[i]
		if g.ID != w.ID || g.Agent != w.Agent || g.Kind != w.Kind || g.SourcePath != w.SourcePath ||
			g.Scope != w.Scope || g.Precedence != w.Precedence || g.Parser != w.Parser ||
			g.Sensitivity != w.Sensitivity || g.ContentPolicy != w.ContentPolicy ||
			g.RestorePolicy != w.RestorePolicy || g.CaptureStatus != w.CaptureStatus ||
			g.Confidence != w.Confidence || !pointerEqual(g.Name, w.Name) ||
			!pointerEqual(g.Checksum, w.Checksum) || !jsonEqual(g.Value, w.Value) ||
			!jsonEqual(g.Metadata, w.Metadata) {
			return false
		}
	}
	if len(got.AuditFindings) != len(want.AuditFindings) {
		return false
	}
	for i := range got.AuditFindings {
		g := got.AuditFindings[i]
		w := want.AuditFindings[i]
		if g.Code != w.Code || g.Severity != w.Severity || g.Problem != w.Problem ||
			g.Cause != w.Cause || g.Fix != w.Fix || !pointerEqual(g.Path, w.Path) ||
			!pointerEqual(g.EvidenceID, w.EvidenceID) {
			return false
		}
	}
	if len(got.Provenance) != len(want.Provenance) {
		return false
	}
	for i := range got.Provenance {
		if got.Provenance[i] != want.Provenance[i] {
			return false
		}
	}
	if len(got.Graph) != len(want.Graph) {
		return false
	}
	for i := range got.Graph {
		g := got.Graph[i]
		w := want.Graph[i]
		if g.ID != w.ID || g.Agent != w.Agent || g.Scope != w.Scope || g.SourcePath != w.SourcePath ||
			g.EntityKind != w.EntityKind || g.EntityName != w.EntityName || g.Confidence != w.Confidence ||
			g.EvidenceID != w.EvidenceID || !pointerEqual(g.OverriddenBy, w.OverriddenBy) ||
			!jsonEqual(g.EffectiveValue, w.EffectiveValue) {
			return false
		}
	}
	return len(got.Content) == len(want.Content)
}

func pointerEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func jsonEqual(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var av any
	var bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func timelineEntry(id, observedAt, afterSnapshotName string) types.TimelineEntry {
	entityName := "github"
	return types.TimelineEntry{
		SchemaVersion:     "0.1",
		ID:                id,
		Source:            types.TimelineSourceManual,
		EventKind:         types.TimelineEventSetupChanged,
		Title:             "update github mcp",
		ProjectPath:       "/tmp/project",
		Agents:            []types.AgentID{types.AgentClaudeCode},
		AfterSnapshotName: afterSnapshotName,
		CaptureID:         "capture-test",
		CreatedAt:         observedAt,
		ObservedAt:        observedAt,
		ChangedSurfaces: []types.TimelineChangedSurface{{
			Kind:        "mcp_server",
			ChangeType:  "MCP_CHANGED",
			Path:        ".mcp.json",
			EntityName:  &entityName,
			Restorable:  true,
			ObserveOnly: false,
		}},
		RestoreReadiness:  types.TimelineRestoreFull,
		Confidence:        types.TimelineConfidenceHigh,
		ConfidenceReason:  "test",
		EvidenceCount:     1,
		GraphNodeCount:    1,
		AuditFindingCount: 0,
		Changes: types.TimelineChangeSummary{
			HasChanges:           true,
			SemanticChangeCount:  1,
			RawSourceChangeCount: 0,
			Highlights:           []string{"MCP_CHANGED: github"},
		},
	}
}

func TestCreatesStoreDirectoryWith0700Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions test")
	}
	root := tempStore(t)
	storeDir := filepath.Join(root, "store")
	findings, err := EnsureStore(storeDir)
	if err != nil {
		t.Fatalf("EnsureStore: %v", err)
	}
	info, err := os.Stat(storeDir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o, want 0700", info.Mode().Perm())
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want empty", findings)
	}
}

func TestReturnsAuditFindingWhenStoreIsWorldWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions test")
	}
	root := tempStore(t)
	storeDir := filepath.Join(root, "store")
	if _, err := EnsureStore(storeDir); err != nil {
		t.Fatalf("EnsureStore: %v", err)
	}
	if err := os.Chmod(storeDir, 0o777); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	findings, err := EnsureStore(storeDir)
	if err != nil {
		t.Fatalf("EnsureStore: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings len = %d, want 1", len(findings))
	}
	if findings[0].Code != "WORLD_WRITABLE_STORE" {
		t.Fatalf("code = %q, want WORLD_WRITABLE_STORE", findings[0].Code)
	}
	if findings[0].Severity != types.SeverityHigh {
		t.Fatalf("severity = %q, want high", findings[0].Severity)
	}
	if findings[0].Path == nil || *findings[0].Path != storeDir {
		t.Fatalf("path = %v, want %q", findings[0].Path, storeDir)
	}
}

func TestRejectsUnsafeSnapshotNames(t *testing.T) {
	root := tempStore(t)
	for _, name := range []string{"", "../baseline", "base/line", `base\line`, "safe/../unsafe", ".."} {
		err := WriteSnapshot(root, StoreSnapshotFrom(testSnapshot(name)), nil)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsafe snapshot name") {
			t.Fatalf("WriteSnapshot(%q) err = %v, want unsafe snapshot name", name, err)
		}
		_, err = ReadSnapshot(root, name, nil)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsafe snapshot name") {
			t.Fatalf("ReadSnapshot(%q) err = %v, want unsafe snapshot name", name, err)
		}
	}
}

func TestListsSnapshotsAndRoundTrips(t *testing.T) {
	root := tempStore(t)
	if err := WriteSnapshot(root, StoreSnapshotFrom(testSnapshot("current")), nil); err != nil {
		t.Fatalf("WriteSnapshot current: %v", err)
	}
	if err := WriteSnapshot(root, StoreSnapshotFrom(testSnapshot("baseline")), nil); err != nil {
		t.Fatalf("WriteSnapshot baseline: %v", err)
	}

	names, err := ListSnapshots(root, nil)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	want := []string{"baseline", "current"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}

	exists, err := SnapshotExists(root, "baseline", nil)
	if err != nil || !exists {
		t.Fatalf("SnapshotExists baseline = %v, %v, want true", exists, err)
	}
	exists, err = SnapshotExists(root, "missing", nil)
	if err != nil || exists {
		t.Fatalf("SnapshotExists missing = %v, %v, want false", exists, err)
	}

	read, err := ReadSnapshot(root, "baseline", nil)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if !snapshotsEqual(t, read, testSnapshot("baseline")) {
		t.Fatalf("read snapshot mismatch:\n got %#v\nwant %#v", read, testSnapshot("baseline"))
	}
}

func TestWritesMetadataOnlySnapshotFileSet(t *testing.T) {
	root := tempStore(t)
	if err := WriteSnapshot(root, StoreSnapshotFrom(testSnapshot("baseline")), nil); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	snapshotDir := filepath.Join(root, "baseline")
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var files []string
	for _, entry := range entries {
		files = append(files, entry.Name())
	}
	want := []string{
		"audit-findings.json",
		"checksums.json",
		"evidence.json",
		"graph.json",
		"manifest.json",
		"provenance.json",
		"redactions.json",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}

	var checksums map[string]ChecksumRecord
	if err := readJSON(filepath.Join(snapshotDir, "checksums.json"), &checksums); err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	wantChecksums := map[string]ChecksumRecord{
		"claude.project.settings": {
			SourcePath: ".claude/settings.json",
			Checksum:   "sha256:observed-config",
		},
	}
	if !reflect.DeepEqual(checksums, wantChecksums) {
		t.Fatalf("checksums = %#v, want %#v", checksums, wantChecksums)
	}

	var redactions []json.RawMessage
	if err := readJSON(filepath.Join(snapshotDir, "redactions.json"), &redactions); err != nil {
		t.Fatalf("read redactions: %v", err)
	}
	if len(redactions) != 0 {
		t.Fatalf("redactions = %#v, want empty array", redactions)
	}
}

func TestWritesAndReadsContentBackedSnapshotEntries(t *testing.T) {
	root := tempStore(t)
	contentSnapshot := StoreSnapshotFrom(testSnapshot("codex-baseline"))
	contentSnapshot.Manifest.Security = types.SnapshotSecurity{
		RawSecretsIncluded: false,
		RedactionPolicy:    "content-backed",
	}
	content := "model = \"gpt-5\""
	reason := "secret_like_assignment"
	contentSnapshot.Content = []types.SnapshotContentEntry{
		{
			EvidenceID:    "claude.project.settings",
			SourcePath:    "~/.codex/config.toml",
			RestorePath:   "~/.codex/config.toml",
			Checksum:      "sha256:codex-config",
			ByteLength:    14,
			Encoding:      "utf8",
			StoragePath:   "content/claude.project.settings.txt",
			CaptureStatus: "captured",
			Content:       &content,
		},
		{
			EvidenceID:    "secret",
			SourcePath:    "~/.codex/config.toml",
			RestorePath:   "~/.codex/config.toml",
			Checksum:      "sha256:secret",
			ByteLength:    18,
			Encoding:      "utf8",
			StoragePath:   "content/secret.txt",
			CaptureStatus: "omitted",
			Reason:        &reason,
		},
	}
	agent := types.AgentCodex
	if err := WriteSnapshot(root, contentSnapshot, &agent); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	read, err := ReadSnapshot(root, "codex-baseline", &agent)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if len(read.Content) != 2 {
		t.Fatalf("content len = %d, want 2", len(read.Content))
	}
	if read.Content[0].Content != nil {
		t.Fatalf("content index should omit inline content")
	}
	if read.Content[1].CaptureStatus != "omitted" {
		t.Fatalf("captureStatus = %q, want omitted", read.Content[1].CaptureStatus)
	}
	if read.Content[1].Reason == nil || *read.Content[1].Reason != "secret_like_assignment" {
		t.Fatalf("reason = %v, want secret_like_assignment", read.Content[1].Reason)
	}

	text, err := ReadSnapshotContent(root, "codex-baseline", read.Content[0], &agent)
	if err != nil {
		t.Fatalf("ReadSnapshotContent: %v", err)
	}
	if text != `model = "gpt-5"` {
		t.Fatalf("content = %q, want model = \"gpt-5\"", text)
	}

	badEntry := read.Content[0]
	badEntry.StoragePath = "../escape"
	_, err = ReadSnapshotContent(root, "codex-baseline", badEntry, &agent)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsafe snapshot content path") {
		t.Fatalf("ReadSnapshotContent bad path err = %v, want unsafe snapshot content path", err)
	}
}

func TestWriteSnapshotKeepsPreviousVersionWhenReplacementFails(t *testing.T) {
	root := tempStore(t)
	agent := types.AgentCodex

	baseline := StoreSnapshotFrom(testSnapshot("codex-baseline"))
	content := "model = \"gpt-5\""
	baseline.Content = []types.SnapshotContentEntry{{
		EvidenceID:    "claude.project.settings",
		SourcePath:    "~/.codex/config.toml",
		RestorePath:   "~/.codex/config.toml",
		Checksum:      "sha256:codex-config",
		ByteLength:    14,
		Encoding:      "utf8",
		StoragePath:   "content/claude.project.settings.txt",
		CaptureStatus: "captured",
		Content:       &content,
	}}
	if err := WriteSnapshot(root, baseline, &agent); err != nil {
		t.Fatalf("WriteSnapshot baseline: %v", err)
	}

	broken := baseline
	broken.Content = []types.SnapshotContentEntry{{
		EvidenceID:    "claude.project.settings",
		SourcePath:    "~/.codex/config.toml",
		RestorePath:   "~/.codex/config.toml",
		Checksum:      "sha256:codex-config-broken",
		ByteLength:    14,
		Encoding:      "utf8",
		StoragePath:   "../escape",
		CaptureStatus: "captured",
		Content:       &content,
	}}
	if err := WriteSnapshot(root, broken, &agent); err == nil {
		t.Fatal("expected replacement to fail")
	}

	read, err := ReadSnapshot(root, "codex-baseline", &agent)
	if err != nil {
		t.Fatalf("ReadSnapshot after failed replacement: %v", err)
	}
	if len(read.Content) != 1 || read.Content[0].StoragePath != "content/claude.project.settings.txt" {
		t.Fatalf("content index = %#v", read.Content)
	}
	text, err := ReadSnapshotContent(root, "codex-baseline", read.Content[0], &agent)
	if err != nil {
		t.Fatalf("ReadSnapshotContent after failed replacement: %v", err)
	}
	if text != content {
		t.Fatalf("content = %q, want %q", text, content)
	}
}

func TestAgentStoreDirReturnsScopedPaths(t *testing.T) {
	agent := types.AgentClaudeCode
	if got := AgentStoreDir("/store", &agent); got != "/store/claude-code" {
		t.Fatalf("claude-code path = %q", got)
	}
	codex := types.AgentCodex
	if got := AgentStoreDir("/store", &codex); got != "/store/codex" {
		t.Fatalf("codex path = %q", got)
	}
	if got := AgentStoreDir("/store", nil); got != "/store" {
		t.Fatalf("default path = %q", got)
	}
}

func TestWritesAndReadsSnapshotsPerAgent(t *testing.T) {
	root := tempStore(t)
	cc := types.AgentClaudeCode
	codex := types.AgentCodex
	if err := WriteSnapshot(root, StoreSnapshotFrom(testSnapshot("baseline")), &cc); err != nil {
		t.Fatalf("WriteSnapshot claude-code: %v", err)
	}
	if err := WriteSnapshot(root, StoreSnapshotFrom(testSnapshot("codex-baseline")), &codex); err != nil {
		t.Fatalf("WriteSnapshot codex: %v", err)
	}

	if info, err := os.Stat(filepath.Join(root, "claude-code", "baseline")); err != nil || !info.IsDir() {
		t.Fatalf("claude-code/baseline missing: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "codex", "codex-baseline")); err != nil || !info.IsDir() {
		t.Fatalf("codex/codex-baseline missing: %v", err)
	}

	names, err := ListSnapshots(root, nil)
	if err != nil || len(names) != 0 {
		t.Fatalf("root snapshots = %v, %v, want empty", names, err)
	}
	ccNames, err := ListSnapshots(root, &cc)
	if err != nil || !reflect.DeepEqual(ccNames, []string{"baseline"}) {
		t.Fatalf("claude-code snapshots = %v, %v", ccNames, err)
	}
	codexNames, err := ListSnapshots(root, &codex)
	if err != nil || !reflect.DeepEqual(codexNames, []string{"codex-baseline"}) {
		t.Fatalf("codex snapshots = %v, %v", codexNames, err)
	}

	readCC, err := ReadSnapshot(root, "baseline", &cc)
	if err != nil || !snapshotsEqual(t, readCC, testSnapshot("baseline")) {
		t.Fatalf("read claude-code snapshot mismatch: %v", err)
	}
	readCodex, err := ReadSnapshot(root, "codex-baseline", &codex)
	if err != nil || readCodex.Manifest.Name != "codex-baseline" {
		t.Fatalf("read codex snapshot mismatch: %v", err)
	}

	exists, err := SnapshotExists(root, "baseline", &cc)
	if err != nil || !exists {
		t.Fatalf("baseline exists for claude-code = %v, %v", exists, err)
	}
	exists, err = SnapshotExists(root, "baseline", &codex)
	if err != nil || exists {
		t.Fatalf("baseline exists for codex = %v, %v, want false", exists, err)
	}
}

func TestListSnapshotsSkipsAgentScopedDirsWithoutManifest(t *testing.T) {
	root := tempStore(t)
	agent := types.AgentCodex
	if err := WriteSnapshot(root, StoreSnapshotFrom(testSnapshot("baseline")), &agent); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "codex", "pre-apply-tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "codex", "baseline.bak"), 0o755); err != nil {
		t.Fatal(err)
	}

	names, err := ListSnapshots(root, &agent)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"baseline"}) {
		t.Fatalf("names = %v, want [baseline]", names)
	}
}

func TestListAgentsReturnsAgentsWithSnapshots(t *testing.T) {
	root := tempStore(t)
	cc := types.AgentClaudeCode
	codex := types.AgentCodex
	cursor := types.AgentCursor
	for _, agent := range []*types.AgentID{&cc, &codex, &cursor} {
		if err := WriteSnapshot(root, StoreSnapshotFrom(testSnapshot("v1")), agent); err != nil {
			t.Fatalf("WriteSnapshot %s: %v", agent.String(), err)
		}
	}

	agents, err := ListAgents(root)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	want := []types.AgentID{types.AgentClaudeCode, types.AgentCodex, types.AgentCursor}
	if !reflect.DeepEqual(agents, want) {
		t.Fatalf("agents = %v, want %v", agents, want)
	}
}

func TestListAgentsReturnsEmptyForEmptyStore(t *testing.T) {
	root := tempStore(t)
	if _, err := EnsureStore(root); err != nil {
		t.Fatalf("EnsureStore: %v", err)
	}
	agents, err := ListAgents(root)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("agents = %v, want empty", agents)
	}
}

func TestPersistsTimelineEventsSortedByObservedTime(t *testing.T) {
	root := tempStore(t)
	older := timelineEntry("older", "2026-06-07T00:00:00.000Z", "after-older")
	newer := timelineEntry("newer", "2026-06-07T00:01:00.000Z", "after-newer")
	if err := AppendTimelineEntry(root, &older); err != nil {
		t.Fatalf("AppendTimelineEntry older: %v", err)
	}
	if err := AppendTimelineEntry(root, &newer); err != nil {
		t.Fatalf("AppendTimelineEntry newer: %v", err)
	}

	entries, err := ListTimelineEntries(root, TimelineListOptions{})
	if err != nil {
		t.Fatalf("ListTimelineEntries: %v", err)
	}
	var ids []string
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	if !reflect.DeepEqual(ids, []string{"newer", "older"}) {
		t.Fatalf("ids = %v, want [newer older]", ids)
	}

	found, err := FindTimelineEntry(root, "after-older", TimelineListOptions{})
	if err != nil {
		t.Fatalf("FindTimelineEntry: %v", err)
	}
	if found == nil || found.ID != "older" {
		t.Fatalf("found = %#v, want older", found)
	}
}

func TestSkipsCorruptTimelineEvents(t *testing.T) {
	root := tempStore(t)
	valid := timelineEntry("valid", "2026-06-07T00:00:00.000Z", "after-valid")
	if err := AppendTimelineEntry(root, &valid); err != nil {
		t.Fatalf("AppendTimelineEntry: %v", err)
	}
	eventsDir := filepath.Join(root, timelineEventsDir)
	if err := os.MkdirAll(eventsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(eventsDir, "bad.json"), []byte("{bad json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var corruptEvents []TimelineCorruptEvent
	entries, err := ListTimelineEntries(root, TimelineListOptions{
		OnCorruptEntry: func(event TimelineCorruptEvent) {
			corruptEvents = append(corruptEvents, event)
		},
	})
	if err != nil {
		t.Fatalf("ListTimelineEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "valid" {
		t.Fatalf("entries = %#v, want valid only", entries)
	}
	if len(corruptEvents) != 1 {
		t.Fatalf("corruptEvents len = %d, want 1", len(corruptEvents))
	}
	if !strings.HasSuffix(corruptEvents[0].FilePath, "bad.json") {
		t.Fatalf("corrupt file = %q", corruptEvents[0].FilePath)
	}
	if !strings.Contains(strings.ToLower(corruptEvents[0].Error), "json") {
		t.Fatalf("corrupt error = %q", corruptEvents[0].Error)
	}
}

func TestNormalizesLegacyDaemonTimelineEvents(t *testing.T) {
	root := tempStore(t)
	if _, err := EnsureStore(root); err != nil {
		t.Fatalf("EnsureStore: %v", err)
	}
	eventsDir := filepath.Join(root, timelineEventsDir)
	if err := os.MkdirAll(eventsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	legacy := `{
  "schemaVersion": "0.1",
  "id": "legacy-event",
  "source": "daemon",
  "eventKind": "baseline",
  "title": "legacy daemon baseline",
  "projectPath": "/tmp/project",
  "agents": ["claude-code"],
  "afterSnapshotName": "legacy-baseline",
  "daemonRunId": "run-legacy",
  "createdAt": "2026-06-07T00:00:00.000Z",
  "observedAt": "2026-06-07T00:00:00.000Z",
  "changedSurfaces": [],
  "restoreReadiness": "observe-only",
  "confidence": "high",
  "confidenceReason": "legacy",
  "evidenceCount": 0,
  "graphNodeCount": 0,
  "auditFindingCount": 0,
  "changes": {
    "hasChanges": false,
    "semanticChangeCount": 0,
    "rawSourceChangeCount": 0,
    "highlights": []
  },
  "captureId": ""
}`
	if err := os.WriteFile(filepath.Join(eventsDir, "legacy.json"), []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entries, err := ListTimelineEntries(root, TimelineListOptions{})
	if err != nil {
		t.Fatalf("ListTimelineEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Source != types.TimelineSourceManual {
		t.Fatalf("source = %q, want manual", entry.Source)
	}
	if entry.CaptureID != "run-legacy" {
		t.Fatalf("captureId = %q, want run-legacy", entry.CaptureID)
	}
	if entry.AfterSnapshotName != "legacy-baseline" {
		t.Fatalf("afterSnapshotName = %q, want legacy-baseline", entry.AfterSnapshotName)
	}
}
