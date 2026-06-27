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
	if len(pi.Entries) != 1 || pi.Entries[0].Name != "cmux-session" {
		t.Fatalf("pi entries = %#v", pi.Entries)
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
