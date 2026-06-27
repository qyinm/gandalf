package setup

import (
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestBuildInventoryIncludesGlobalSetupObjects(t *testing.T) {
	skillName := "review"
	mcpName := "github"
	hookName := "Stop.*"
	pluginName := "browser"

	items := BuildInventory([]types.DiscoveredItem{
		{
			ID:         "skill-review",
			Agent:      types.AgentCodex,
			Kind:       types.KindSkill,
			Name:       &skillName,
			SourcePath: "~/.codex/skills/review",
			Scope:      types.ScopeUser,
		},
		{
			ID:         "mcp-github",
			Agent:      types.AgentClaudeCode,
			Kind:       types.KindMcpServer,
			Name:       &mcpName,
			SourcePath: "~/.claude/settings.json",
			Scope:      types.ScopeUser,
		},
		{
			ID:         "hook-stop",
			Agent:      types.AgentCursor,
			Kind:       types.KindHook,
			Name:       &hookName,
			SourcePath: "~/.cursor/hooks.json",
			Scope:      types.ScopeUser,
		},
		{
			ID:         "plugin-browser",
			Agent:      types.AgentPiAgent,
			Kind:       types.KindExtension,
			Name:       &pluginName,
			SourcePath: "~/.pi/agent/extensions/browser",
			Scope:      types.ScopeUser,
		},
	})

	if len(items) != 4 {
		t.Fatalf("items = %#v", items)
	}

	byID := make(map[string]InventoryItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	assertInventoryItem(t, byID["skill-review"], types.AgentCodex, ObjectSkill, "review")
	assertInventoryItem(t, byID["mcp-github"], types.AgentClaudeCode, ObjectMCPServer, "github")
	assertInventoryItem(t, byID["hook-stop"], types.AgentCursor, ObjectHook, "Stop.*")
	assertInventoryItem(t, byID["plugin-browser"], types.AgentPiAgent, ObjectPlugin, "browser")
}

func TestBuildInventoryExcludesProjectScopeAndProjectAgent(t *testing.T) {
	projectSkill := "project-skill"
	projectMCP := "project-mcp"
	userSkill := "user-skill"

	items := BuildInventory([]types.DiscoveredItem{
		{
			ID:         "project-skill",
			Agent:      types.AgentCodex,
			Kind:       types.KindSkill,
			Name:       &projectSkill,
			SourcePath: ".codex/skills/project-skill",
			Scope:      types.ScopeProject,
		},
		{
			ID:         "project-mcp",
			Agent:      types.AgentProject,
			Kind:       types.KindMcpServer,
			Name:       &projectMCP,
			SourcePath: ".mcp.json",
			Scope:      types.ScopeProject,
		},
		{
			ID:         "user-skill",
			Agent:      types.AgentCodex,
			Kind:       types.KindSkill,
			Name:       &userSkill,
			SourcePath: "~/.codex/skills/user-skill",
			Scope:      types.ScopeUser,
		},
	})

	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].ID != "user-skill" {
		t.Fatalf("unexpected item = %#v", items[0])
	}
}

func TestBuildInventoryMarksManagedActionsUnavailable(t *testing.T) {
	name := "customize-opencode"
	items := BuildInventory([]types.DiscoveredItem{
		{
			ID:         "managed-skill",
			Agent:      types.AgentOpencode,
			Kind:       types.KindSkill,
			Name:       &name,
			SourcePath: "<built-in>",
			Scope:      types.ScopeManaged,
		},
	})

	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	for _, action := range items[0].Actions {
		if action.Available {
			t.Fatalf("managed action should be unavailable: %#v", action)
		}
		if action.Reason == "" {
			t.Fatalf("managed action should explain why unavailable: %#v", action)
		}
	}
}

func TestBuildInventoryIgnoresNonSetupEvidence(t *testing.T) {
	configName := "config"
	items := BuildInventory([]types.DiscoveredItem{
		{
			ID:         "config",
			Agent:      types.AgentCodex,
			Kind:       types.KindAgentConfig,
			Name:       &configName,
			SourcePath: "~/.codex/config.toml",
			Scope:      types.ScopeUser,
		},
	})

	if len(items) != 0 {
		t.Fatalf("items = %#v", items)
	}
}

func assertInventoryItem(t *testing.T, item InventoryItem, agent types.AgentID, kind ObjectKind, name string) {
	t.Helper()
	if item.Agent != agent {
		t.Fatalf("agent = %q", item.Agent)
	}
	if item.ObjectKind != kind {
		t.Fatalf("kind = %q", item.ObjectKind)
	}
	if item.Name != name {
		t.Fatalf("name = %q", item.Name)
	}
	if len(item.Actions) != 2 {
		t.Fatalf("actions = %#v", item.Actions)
	}
	for _, action := range item.Actions {
		if !action.Available {
			t.Fatalf("user action should be available: %#v", action)
		}
	}
}
