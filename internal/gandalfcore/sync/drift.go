package sync

import (
	"os"
	"path/filepath"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// DetectDrift compares the team manifest against the discovered local environment items.
func DetectDrift(m *manifest.Manifest, projectRoot, homeDir string, discovered []types.DiscoveredItem) (*DriftReport, error) {
	report := &DriftReport{
		InSync:       true,
		ProjectName:  m.Name,
		TargetAgents: m.Agents,
		Items:        nil,
	}

	// Map existing discovered items by Agent -> Kind -> Name
	existingMCP := make(map[types.AgentID]map[string]bool)
	existingSkills := make(map[types.AgentID]map[string]bool)

	for _, a := range m.Agents {
		existingMCP[a] = make(map[string]bool)
		existingSkills[a] = make(map[string]bool)
	}

	for _, item := range discovered {
		if item.Name == nil {
			continue
		}
		name := *item.Name
		if item.Kind == types.KindMcpServer {
			if m, ok := existingMCP[item.Agent]; ok {
				m[name] = true
			}
		} else if item.Kind == types.KindSkill {
			if m, ok := existingSkills[item.Agent]; ok {
				m[name] = true
			}
		}
	}

	// Check MCP servers for each target agent
	for srvName, srv := range m.MCPServers {
		for _, agent := range m.Agents {
			if !existingMCP[agent][srvName] {
				report.InSync = false
				targetFile := ""
				switch agent {
				case types.AgentClaudeCode:
					targetFile = filepath.Join(homeDir, ".claude", "settings.json")
				case types.AgentCodex:
					targetFile = filepath.Join(homeDir, ".codex", "config.toml")
				case types.AgentCursor:
					targetFile = filepath.Join(homeDir, ".cursor", "mcp.json")
				}

				report.Items = append(report.Items, DriftItem{
					Agent:       agent,
					Kind:        DriftMissingMCPServer,
					Name:        srvName,
					Description: srv.Description,
					TargetFile:  targetFile,
					Details:     "MCP server is not configured in local agent setup",
				})
			}
		}
	}

	// Check Skills for each target agent
	for _, skill := range m.Skills {
		for _, agent := range m.Agents {
			var destSkillDir string
			switch agent {
			case types.AgentClaudeCode:
				destSkillDir = filepath.Join(homeDir, ".claude", "skills", skill.Name)
			case types.AgentCodex:
				destSkillDir = filepath.Join(homeDir, ".codex", "skills", skill.Name)
			case types.AgentCursor:
				destSkillDir = filepath.Join(homeDir, ".cursor", "skills", skill.Name)
			}

			if _, err := os.Stat(destSkillDir); os.IsNotExist(err) {
				report.InSync = false
				report.Items = append(report.Items, DriftItem{
					Agent:       agent,
					Kind:        DriftMissingSkill,
					Name:        skill.Name,
					Description: skill.Description,
					TargetFile:  destSkillDir,
					Details:     "Skill directory is missing in local agent home",
				})
			}
		}
	}

	return report, nil
}
