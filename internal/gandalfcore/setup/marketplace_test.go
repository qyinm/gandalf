package setup

import (
	"encoding/json"
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
	if len(entry.Actions) == 0 || entry.Actions[0].Available {
		t.Fatalf("entry actions should be unavailable: %#v", entry.Actions)
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
