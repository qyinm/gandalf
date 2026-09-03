package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// CreateSyncPlan generates an execution plan to synchronize the team manifest to local agents.
func CreateSyncPlan(m *manifest.Manifest, projectRoot, homeDir string, discovered []types.DiscoveredItem) (*SyncPlan, error) {
	drift, err := DetectDrift(m, projectRoot, homeDir, discovered)
	if err != nil {
		return nil, fmt.Errorf("detect drift: %w", err)
	}

	plan := &SyncPlan{
		Manifest:    m,
		ProjectRoot: projectRoot,
		HomeDir:     homeDir,
		Drift:       drift,
	}

	for _, agent := range m.Agents {
		switch agent {
		case types.AgentClaudeCode:
			claudeFile := filepath.Join(homeDir, ".claude", "settings.json")
			existingContent := ""
			if data, err := os.ReadFile(claudeFile); err == nil {
				existingContent = string(data)
			}
			mergedJSON, err := MergeClaudeSettingsJSON(existingContent, m)
			if err != nil {
				return nil, fmt.Errorf("merge claude settings: %w", err)
			}

			plan.Items = append(plan.Items, SyncPlanItem{
				Agent:       types.AgentClaudeCode,
				Kind:        types.KindAgentConfig,
				Name:        "settings.json",
				Action:      "update",
				TargetFile:  claudeFile,
				Content:     mergedJSON,
				Description: fmt.Sprintf("Inject %d MCP server(s) into Claude Code settings", len(m.MCPServers)),
			})

			for _, skill := range m.Skills {
				if skill.Source != "" {
					srcPath := filepath.Join(projectRoot, filepath.Clean(skill.Source))
					destPath := filepath.Join(homeDir, ".claude", "skills", skill.Name)
					plan.Items = append(plan.Items, SyncPlanItem{
						Agent:       types.AgentClaudeCode,
						Kind:        types.KindSkill,
						Name:        skill.Name,
						Action:      "copy",
						SourceFile:  srcPath,
						TargetFile:  destPath,
						Description: fmt.Sprintf("Copy team skill '%s' to Claude Code", skill.Name),
					})
				}
			}

		case types.AgentCodex:
			codexFile := filepath.Join(homeDir, ".codex", "config.toml")
			existingContent := ""
			if data, err := os.ReadFile(codexFile); err == nil {
				existingContent = string(data)
			}
			mergedTOML, err := MergeCodexConfigTOML(existingContent, m)
			if err != nil {
				return nil, fmt.Errorf("merge codex config: %w", err)
			}

			plan.Items = append(plan.Items, SyncPlanItem{
				Agent:       types.AgentCodex,
				Kind:        types.KindAgentConfig,
				Name:        "config.toml",
				Action:      "update",
				TargetFile:  codexFile,
				Content:     mergedTOML,
				Description: fmt.Sprintf("Inject %d MCP server(s) into Codex config", len(m.MCPServers)),
			})

			for _, skill := range m.Skills {
				if skill.Source != "" {
					srcPath := filepath.Join(projectRoot, filepath.Clean(skill.Source))
					destPath := filepath.Join(homeDir, ".codex", "skills", skill.Name)
					plan.Items = append(plan.Items, SyncPlanItem{
						Agent:       types.AgentCodex,
						Kind:        types.KindSkill,
						Name:        skill.Name,
						Action:      "copy",
						SourceFile:  srcPath,
						TargetFile:  destPath,
						Description: fmt.Sprintf("Copy team skill '%s' to Codex", skill.Name),
					})
				}
			}

		case types.AgentCursor:
			cursorFile := filepath.Join(homeDir, ".cursor", "mcp.json")
			existingContent := ""
			if data, err := os.ReadFile(cursorFile); err == nil {
				existingContent = string(data)
			}
			mergedJSON, err := MergeCursorMCPJSON(existingContent, m)
			if err != nil {
				return nil, fmt.Errorf("merge cursor mcp settings: %w", err)
			}

			plan.Items = append(plan.Items, SyncPlanItem{
				Agent:       types.AgentCursor,
				Kind:        types.KindAgentConfig,
				Name:        "mcp.json",
				Action:      "update",
				TargetFile:  cursorFile,
				Content:     mergedJSON,
				Description: fmt.Sprintf("Inject %d MCP server(s) into Cursor config", len(m.MCPServers)),
			})

			for _, skill := range m.Skills {
				if skill.Source != "" {
					srcPath := filepath.Join(projectRoot, filepath.Clean(skill.Source))
					destPath := filepath.Join(homeDir, ".cursor", "skills", skill.Name)
					plan.Items = append(plan.Items, SyncPlanItem{
						Agent:       types.AgentCursor,
						Kind:        types.KindSkill,
						Name:        skill.Name,
						Action:      "copy",
						SourceFile:  srcPath,
						TargetFile:  destPath,
						Description: fmt.Sprintf("Copy team skill '%s' to Cursor", skill.Name),
					})
				}
			}
		}
	}

	return plan, nil
}
