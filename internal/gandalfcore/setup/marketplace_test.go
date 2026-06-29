package setup

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestBuildMarketplaceGroupsObservedAgentSources(t *testing.T) {
	reviewName := "review"
	cmuxName := "cmux-session"
	projectName := "project-only"
	evidence := []types.DiscoveredItem{
		{
			ID:         "codex-skill-review",
			Agent:      types.AgentCodex,
			Kind:       types.KindSkill,
			Name:       &reviewName,
			SourcePath: "~/.codex/plugins/cache/openai-codex/skills/review",
			Scope:      types.ScopeManaged,
			Metadata: json.RawMessage(`{
				"source": "plugin",
				"sourceRoot": "~/.codex/plugins/cache/openai-codex",
				"description": "Review code",
				"author": "OpenAI",
				"category": "development",
				"version": "1.0.5",
				"provides": ["skills", "hooks"]
			}`),
		},
		{
			ID:         "pi-extension-cmux",
			Agent:      types.AgentPiAgent,
			Kind:       types.KindExtension,
			Name:       &cmuxName,
			SourcePath: "~/.pi/agent/extensions/cmux-session.ts",
			Scope:      types.ScopeUser,
			Metadata:   json.RawMessage(`{"source":"package","version":"0.1.0"}`),
		},
		{
			ID:         "project-skill",
			Agent:      types.AgentCodex,
			Kind:       types.KindSkill,
			Name:       &projectName,
			SourcePath: ".codex/skills/project-only",
			Scope:      types.ScopeProject,
			Metadata:   json.RawMessage(`{"source":"plugin"}`),
		},
	}

	sources := BuildMarketplace(evidence)
	if len(sources) != 2 {
		t.Fatalf("sources = %#v", sources)
	}

	codex := findMarketplaceSource(t, sources, types.AgentCodex)
	if codex.Label != "openai-codex" {
		t.Fatalf("codex source label = %q", codex.Label)
	}
	if codex.Kind != MarketplaceSourcePlugin {
		t.Fatalf("codex source kind = %q", codex.Kind)
	}
	if len(codex.Entries) != 1 {
		t.Fatalf("codex entries = %#v", codex.Entries)
	}
	entry := codex.Entries[0]
	if !entry.Installed || entry.Status != "installed" {
		t.Fatalf("installed state = %#v", entry)
	}
	if entry.Description != "Review code" || entry.Author != "OpenAI" || entry.Version != "1.0.5" {
		t.Fatalf("metadata not captured: %#v", entry)
	}
	if len(entry.Provides) != 2 || entry.Provides[0] != "skills" {
		t.Fatalf("provides = %#v", entry.Provides)
	}
	if len(entry.Actions) == 0 || entry.Actions[0].Action != MarketplaceActionReview || !entry.Actions[0].Available {
		t.Fatalf("entry review action should be available: %#v", entry.Actions)
	}
	if entry.Actions[1].Available {
		t.Fatalf("mutating entry actions should remain unavailable: %#v", entry.Actions)
	}
	if len(codex.Actions) == 0 || codex.Actions[0].Available {
		t.Fatalf("source actions should be unavailable: %#v", codex.Actions)
	}

	pi := findMarketplaceSource(t, sources, types.AgentPiAgent)
	if pi.Kind != MarketplaceSourceExtension {
		t.Fatalf("pi source kind = %q", pi.Kind)
	}
	if len(pi.Entries) != 1 || pi.Entries[0].Name != "cmux-session" {
		t.Fatalf("pi entries = %#v", pi.Entries)
	}
}

func TestMarketplaceReviewActionPlansNonMutatingGuidance(t *testing.T) {
	name := "codex"
	sources := BuildMarketplace([]types.DiscoveredItem{{
		ID:         "marketplace-plugin",
		Agent:      types.AgentClaudeCode,
		Kind:       types.KindSkill,
		Name:       &name,
		SourcePath: "~/.claude/plugins/marketplaces/openai-codex/codex/1.0.2/skills/codex",
		Scope:      types.ScopeUser,
		Metadata: json.RawMessage(`{
			"source": "plugin",
			"sourceRoot": "~/.claude/plugins/marketplaces/openai-codex",
			"description": "Use Codex from Claude Code",
			"author": "OpenAI",
			"version": "1.0.5",
			"provides": ["skills", "hooks"]
		}`),
	}})
	if len(sources) != 1 || len(sources[0].Entries) != 1 {
		t.Fatalf("sources = %#v", sources)
	}

	plan := PlanMarketplaceEntryAction(sources[0], sources[0].Entries[0], MarketplaceActionReview)
	if !plan.Available {
		t.Fatalf("plan unavailable: %#v", plan)
	}
	if !plan.NonMutating || plan.ExpectedEffect == "" {
		t.Fatalf("plan should be non-mutating guidance: %#v", plan)
	}
	if !containsAll(plan.Instructions, "Use Codex from Claude Code", "write files", "Source path:") {
		t.Fatalf("instructions = %q", plan.Instructions)
	}

	result, err := ExecuteMarketplaceReviewPlan(plan, sources)
	if err != nil {
		t.Fatal(err)
	}
	if !result.NonMutating || result.ChangedFiles || result.ExecutedTools {
		t.Fatalf("result should not mutate or execute: %#v", result)
	}
}

func TestMarketplaceReviewActionKeepsMutatingActionsUnavailable(t *testing.T) {
	entry := MarketplaceEntry{
		ID:         "entry",
		SourceID:   "source",
		Agent:      types.AgentClaudeCode,
		Name:       "codex",
		Kind:       types.KindSkill,
		SourcePath: "~/.claude/plugins/marketplaces/openai-codex/codex/skills/codex",
		Actions:    marketplaceEntryActions(MarketplaceEntry{ID: "entry", SourceID: "source", Name: "codex", SourcePath: "path"}),
	}
	source := MarketplaceSource{ID: "source", Label: "openai-codex", Kind: MarketplaceSourceMarketplace, Agent: types.AgentClaudeCode}

	plan := PlanMarketplaceEntryAction(source, entry, MarketplaceActionInstall)
	if plan.Available {
		t.Fatalf("install action should be unavailable: %#v", plan)
	}
	if plan.UnavailableReason == "" {
		t.Fatalf("missing unavailable reason: %#v", plan)
	}
}

func TestMarketplaceReviewActionRejectsSparseOrStaleSourceData(t *testing.T) {
	source := MarketplaceSource{ID: "source", Label: "openai-codex", Kind: MarketplaceSourceMarketplace, Agent: types.AgentClaudeCode}
	sparse := MarketplaceEntry{
		ID:       "entry",
		SourceID: "source",
		Agent:    types.AgentClaudeCode,
		Name:     "codex",
		Kind:     types.KindSkill,
	}
	if plan := PlanMarketplaceEntryAction(source, sparse, MarketplaceActionReview); plan.Available {
		t.Fatalf("sparse entry should be unavailable: %#v", plan)
	}

	available := sparse
	available.SourcePath = "~/.claude/plugins/marketplaces/openai-codex/codex/skills/codex"
	plan := PlanMarketplaceEntryAction(source, available, MarketplaceActionReview)
	if !plan.Available {
		t.Fatalf("expected available plan: %#v", plan)
	}
	if _, err := ExecuteMarketplaceReviewPlan(plan, []MarketplaceSource{source}); err == nil {
		t.Fatal("expected stale marketplace review error")
	}
}

func TestMarketplaceReviewActionSanitizesTerminalControls(t *testing.T) {
	source := MarketplaceSource{ID: "source", Label: "openai-codex", Kind: MarketplaceSourceMarketplace, Agent: types.AgentClaudeCode}
	entry := MarketplaceEntry{
		ID:          "entry",
		SourceID:    "source",
		Agent:       types.AgentClaudeCode,
		Name:        "codex",
		Kind:        types.KindSkill,
		SourcePath:  "~/.claude/plugins/marketplaces/openai-codex/codex/skills/codex",
		Description: "\x1b[31mred\x1b[0m\x07 plugin",
	}

	plan := PlanMarketplaceEntryAction(source, entry, MarketplaceActionReview)
	if !plan.Available {
		t.Fatalf("plan unavailable: %#v", plan)
	}
	if strings.Contains(plan.Instructions, "\x1b") || strings.Contains(plan.Instructions, "\x07") || strings.Contains(plan.Instructions, "[31m") {
		t.Fatalf("instructions contain terminal controls: %q", plan.Instructions)
	}
	if !strings.Contains(plan.Instructions, "red plugin") {
		t.Fatalf("sanitized content missing: %q", plan.Instructions)
	}
}

func TestBuildMarketplaceInfersAgentNativeSourceKinds(t *testing.T) {
	codexName := "review"
	opencodeName := "superpowers"
	evidence := []types.DiscoveredItem{
		{
			ID:         "codex-plugin-cache-skill",
			Agent:      types.AgentCodex,
			Kind:       types.KindSkill,
			Name:       &codexName,
			SourcePath: "~/.codex/plugins/cache/openai-codex/codex/1.0.4/skills/review",
			Scope:      types.ScopeManaged,
		},
		{
			ID:         "opencode-package-skill",
			Agent:      types.AgentOpencode,
			Kind:       types.KindSkill,
			Name:       &opencodeName,
			SourcePath: "~/.cache/opencode/packages/superpowers/skills/superpowers",
			Scope:      types.ScopeUser,
		},
	}

	sources := BuildMarketplace(evidence)
	if len(sources) != 2 {
		t.Fatalf("sources = %#v", sources)
	}
	codex := findMarketplaceSource(t, sources, types.AgentCodex)
	if codex.Kind != MarketplaceSourcePlugin || codex.Label != "codex" {
		t.Fatalf("codex inferred source = %#v", codex)
	}
	opencode := findMarketplaceSource(t, sources, types.AgentOpencode)
	if opencode.Kind != MarketplaceSourcePlugin || opencode.Label != "superpowers" {
		t.Fatalf("opencode inferred source = %#v", opencode)
	}
}

func findMarketplaceSource(t *testing.T, sources []MarketplaceSource, agent types.AgentID) MarketplaceSource {
	t.Helper()
	for _, source := range sources {
		if source.Agent == agent {
			return source
		}
	}
	t.Fatalf("missing source for agent %s: %#v", agent, sources)
	return MarketplaceSource{}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
