package hemcore_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/qyinm/hem/internal/hemcore/audit"
	"github.com/qyinm/hem/internal/hemcore/diff"
	"github.com/qyinm/hem/internal/hemcore/graph"
	"github.com/qyinm/hem/internal/hemcore/provenance"
	"github.com/qyinm/hem/internal/hemcore/types"
)

type itemOverrides struct {
	agent         *types.AgentID
	name          *string
	value         json.RawMessage
	checksum      *string
	captureStatus *types.CaptureStatus
	metadata      json.RawMessage
}

func makeItem(
	id string,
	kind types.EvidenceKind,
	sourcePath string,
	scope types.EvidenceScope,
	precedence uint32,
	overrides itemOverrides,
) types.DiscoveredItem {
	agent := types.AgentClaudeCode
	if overrides.agent != nil {
		agent = *overrides.agent
	}
	captureStatus := types.CaptureCaptured
	if overrides.captureStatus != nil {
		captureStatus = *overrides.captureStatus
	}
	return types.DiscoveredItem{
		ID:            id,
		Agent:         agent,
		Kind:          kind,
		SourcePath:    sourcePath,
		Scope:         scope,
		Precedence:    precedence,
		Parser:        types.ParserJSON,
		Sensitivity:   "command_config",
		ContentPolicy: "structured_safe_fields_only",
		RestorePolicy: types.RestoreNotSupported,
		CaptureStatus: captureStatus,
		Confidence:    types.ConfidenceHigh,
		Name:          overrides.name,
		Value:         overrides.value,
		Checksum:      overrides.checksum,
		Metadata:      overrides.metadata,
	}
}

func strPtr(value string) *string { return &value }

func TestBuildsGraphWithProjectOverrideAndProvenance(t *testing.T) {
	t.Parallel()
	evidence := []types.DiscoveredItem{
		makeItem("user-permission-bash", types.KindPermission, "~/.claude/settings.json", types.ScopeUser, 10, itemOverrides{
			name:  strPtr("Bash(git status)"),
			value: json.RawMessage(`{"action":"allow","rule":"Bash(git status)"}`),
		}),
		makeItem("project-permission-bash", types.KindPermission, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name:  strPtr("Bash(git status)"),
			value: json.RawMessage(`{"action":"deny","rule":"Bash(git status)"}`),
		}),
	}

	graphNodes := graph.BuildGraph(evidence)
	var userNode, projectNode *types.GraphNode
	for i := range graphNodes {
		node := &graphNodes[i]
		switch node.EvidenceID {
		case "user-permission-bash":
			userNode = node
		case "project-permission-bash":
			projectNode = node
		}
	}
	if userNode == nil || projectNode == nil {
		t.Fatal("expected user and project nodes")
	}
	if userNode.OverriddenBy == nil || *userNode.OverriddenBy != projectNode.ID {
		t.Fatalf("overridden_by = %#v, want %q", userNode.OverriddenBy, projectNode.ID)
	}
	if string(projectNode.EffectiveValue) != `{"action":"deny","rule":"Bash(git status)"}` {
		t.Fatalf("effective value = %s", projectNode.EffectiveValue)
	}

	entries := provenance.BuildProvenance(graphNodes, evidence)
	var projectEntry *types.ProvenanceEntry
	for i := range entries {
		if entries[i].EvidenceID == "project-permission-bash" {
			projectEntry = &entries[i]
			break
		}
	}
	if projectEntry == nil {
		t.Fatal("expected project provenance entry")
	}
	want := types.ProvenanceEntry{
		NodeID:        projectNode.ID,
		EvidenceID:    "project-permission-bash",
		SourcePath:    ".claude/settings.json",
		Scope:         types.ScopeProject,
		Precedence:    40,
		Confidence:    types.ConfidenceHigh,
		CaptureStatus: types.CaptureCaptured,
	}
	if *projectEntry != want {
		t.Fatalf("provenance entry = %#v, want %#v", *projectEntry, want)
	}
}

func TestAuditsProjectOverrideWildcardParseFailureSymlinkAndSecretLikeValues(t *testing.T) {
	t.Parallel()
	evidence := []types.DiscoveredItem{
		makeItem("user-policy", types.KindPermission, "~/.claude/settings.json", types.ScopeUser, 10, itemOverrides{
			name:  strPtr("Bash(git status)"),
			value: json.RawMessage(`{"action":"allow","rule":"Bash(git status)"}`),
		}),
		makeItem("project-policy", types.KindPermission, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name:  strPtr("Bash(git status)"),
			value: json.RawMessage(`{"action":"deny","rule":"Bash(git status)"}`),
		}),
		makeItem("project-wildcard", types.KindPermission, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name:  strPtr("Bash(*)"),
			value: json.RawMessage(`{"action":"allow","rule":"Bash(*)"}`),
		}),
		makeItem("bad-json", types.KindAgentConfig, ".mcp.json", types.ScopeProject, 40, itemOverrides{
			captureStatus: captureStatusPtr(types.CaptureParseFailed),
			metadata:      json.RawMessage(`{"error":"Unexpected token"}`),
		}),
		makeItem("skipped-link", types.KindSymlink, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			captureStatus: captureStatusPtr(types.CaptureOmitted),
			metadata:      json.RawMessage(`{"skipped":true,"target":"../private/settings.json"}`),
		}),
		makeItem("env-token", types.KindEnvKey, ".env", types.ScopeProject, 40, itemOverrides{
			captureStatus: captureStatusPtr(types.CaptureOmitted),
			name:          strPtr("OPENAI_API_KEY"),
			metadata:      json.RawMessage(`{"secretLike":true}`),
		}),
	}

	findings := audit.AuditEvidence(evidence, graph.BuildGraph(evidence))
	codes := make([]string, len(findings))
	for i, finding := range findings {
		codes[i] = finding.Code
	}
	want := []string{
		"PARSE_FAILED",
		"PERMISSION_WILDCARD_ADDED",
		"PROJECT_OVERRIDES_USER_POLICY",
		"SYMLINK_SKIPPED",
		"SECRET_LIKE_VALUE_OMITTED",
	}
	if len(codes) != len(want) {
		t.Fatalf("codes = %#v, want %#v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("codes = %#v, want %#v", codes, want)
		}
	}
	for _, finding := range findings {
		if finding.Problem == "" || finding.Cause == "" || finding.Fix == "" {
			t.Fatalf("finding missing text: %#v", finding)
		}
		if finding.Path == nil || finding.EvidenceID == nil {
			t.Fatalf("finding missing path/evidence id: %#v", finding)
		}
	}
}

func captureStatusPtr(status types.CaptureStatus) *types.CaptureStatus { return &status }

func TestDiffsSemanticMcpChangesAndRawSourceChanges(t *testing.T) {
	t.Parallel()
	baselineGraph := graph.BuildGraph([]types.DiscoveredItem{
		makeItem("github-old", types.KindMcpServer, ".mcp.json", types.ScopeProject, 40, itemOverrides{
			name:     strPtr("github"),
			value:    json.RawMessage(`{"transport":"stdio","command":"mcp-github","args":["--read-only"]}`),
			checksum: strPtr("old"),
		}),
	})
	currentGraph := graph.BuildGraph([]types.DiscoveredItem{
		makeItem("github-new", types.KindMcpServer, ".mcp.json", types.ScopeProject, 40, itemOverrides{
			name:     strPtr("github"),
			value:    json.RawMessage(`{"transport":"http","url":"https://mcp.example.com/github"}`),
			checksum: strPtr("new"),
		}),
	})

	graphDiff := diff.DiffGraphs(baselineGraph, currentGraph)
	if len(graphDiff.RawSourceChanges) != 1 {
		t.Fatalf("raw changes = %#v", graphDiff.RawSourceChanges)
	}
	raw := graphDiff.RawSourceChanges[0]
	if raw.SourcePath != ".mcp.json" {
		t.Fatalf("source path = %q", raw.SourcePath)
	}
	if raw.BeforeEvidenceID == nil || *raw.BeforeEvidenceID != "github-old" {
		t.Fatalf("before evidence id = %#v", raw.BeforeEvidenceID)
	}
	if raw.AfterEvidenceID == nil || *raw.AfterEvidenceID != "github-new" {
		t.Fatalf("after evidence id = %#v", raw.AfterEvidenceID)
	}
	if raw.Status != "changed" {
		t.Fatalf("status = %q", raw.Status)
	}
	if len(graphDiff.SemanticChanges) != 1 {
		t.Fatalf("semantic changes = %#v", graphDiff.SemanticChanges)
	}
	change := graphDiff.SemanticChanges[0]
	if change.Code != diff.SemanticMcpChanged {
		t.Fatalf("code = %q", change.Code)
	}
	if change.EntityName != "github" {
		t.Fatalf("entity name = %q", change.EntityName)
	}
	fields := append([]string(nil), change.Details.ChangedFields...)
	sort.Strings(fields)
	wantFields := []string{"command", "transport", "urlHost"}
	if len(fields) != len(wantFields) {
		t.Fatalf("changed fields = %#v", fields)
	}
	for i := range wantFields {
		if fields[i] != wantFields[i] {
			t.Fatalf("changed fields = %#v, want %#v", fields, wantFields)
		}
	}
}

func TestDiffsSetupInventoryChangesForSaveTitles(t *testing.T) {
	t.Parallel()
	baselineGraph := graph.BuildGraph([]types.DiscoveredItem{
		makeItem("instructions-old", types.KindAgentInstruction, "AGENTS.md", types.ScopeProject, 40, itemOverrides{
			name: strPtr("AGENTS.md"), value: json.RawMessage(`{"checksum":"old"}`), checksum: strPtr("old"),
		}),
		makeItem("skill-old", types.KindSkill, ".claude/skills/legacy-review/SKILL.md", types.ScopeProject, 40, itemOverrides{
			name: strPtr("legacy-review"), value: json.RawMessage(`{"installed":true}`), checksum: strPtr("legacy"),
		}),
		makeItem("hook-old", types.KindHook, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name: strPtr("pre-tool-use"), value: json.RawMessage(`{"command":"old-hook"}`), checksum: strPtr("hook-old"),
		}),
		makeItem("permission-old", types.KindPermission, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name: strPtr("Bash(git status)"), value: json.RawMessage(`{"action":"allow","rule":"Bash(git status)"}`), checksum: strPtr("permission-old"),
		}),
		makeItem("permission-removed", types.KindPermission, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name: strPtr("Bash(bun test)"), value: json.RawMessage(`{"action":"allow","rule":"Bash(bun test)"}`), checksum: strPtr("permission-removed"),
		}),
	})
	currentGraph := graph.BuildGraph([]types.DiscoveredItem{
		makeItem("instructions-new", types.KindAgentInstruction, "AGENTS.md", types.ScopeProject, 40, itemOverrides{
			name: strPtr("AGENTS.md"), value: json.RawMessage(`{"checksum":"new"}`), checksum: strPtr("new"),
		}),
		makeItem("skill-new", types.KindSkill, ".claude/skills/react-review/SKILL.md", types.ScopeProject, 40, itemOverrides{
			name: strPtr("react-review"), value: json.RawMessage(`{"installed":true}`), checksum: strPtr("skill"),
		}),
		makeItem("hook-new", types.KindHook, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name: strPtr("pre-tool-use"), value: json.RawMessage(`{"command":"new-hook"}`), checksum: strPtr("hook-new"),
		}),
		makeItem("hook-added", types.KindHook, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name: strPtr("post-tool-use"), value: json.RawMessage(`{"command":"notify"}`), checksum: strPtr("hook-added"),
		}),
		makeItem("permission-new", types.KindPermission, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name: strPtr("Bash(git status)"), value: json.RawMessage(`{"action":"deny","rule":"Bash(git status)"}`), checksum: strPtr("permission-new"),
		}),
		makeItem("permission-added", types.KindPermission, ".claude/settings.json", types.ScopeProject, 40, itemOverrides{
			name: strPtr("Bash(bun run build)"), value: json.RawMessage(`{"action":"allow","rule":"Bash(bun run build)"}`), checksum: strPtr("permission-added"),
		}),
	})

	graphDiff := diff.DiffGraphs(baselineGraph, currentGraph)
	codes := make([]string, len(graphDiff.SemanticChanges))
	for i, change := range graphDiff.SemanticChanges {
		codes[i] = change.Code.String()
	}
	sort.Strings(codes)
	want := []string{
		"HOOK_ADDED",
		"HOOK_CHANGED",
		"INSTRUCTION_CHANGED",
		"PERMISSION_CHANGED",
		"PERMISSION_CHANGED",
		"PERMISSION_CHANGED",
		"SKILL_ADDED",
		"SKILL_REMOVED",
	}
	if len(codes) != len(want) {
		t.Fatalf("codes = %#v, want %#v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("codes = %#v, want %#v", codes, want)
		}
	}

	foundRemoved := false
	for _, change := range graphDiff.SemanticChanges {
		if change.Code != diff.SemanticPermissionChanged {
			continue
		}
		removed, ok := change.Details.Extra["removed"]
		if !ok {
			continue
		}
		var value bool
		if json.Unmarshal(removed, &value) == nil && value {
			foundRemoved = true
			break
		}
	}
	if !foundRemoved {
		t.Fatal("expected permission removed semantic change")
	}
}