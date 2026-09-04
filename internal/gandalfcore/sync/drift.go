package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

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

var envVarRegex = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::-([^}]*))?\}`)

func extractEnvsFromString(s string) []string {
	var vars []string
	matches := envVarRegex.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		if len(m) >= 2 {
			vars = append(vars, m[1])
		}
	}
	return vars
}

// DetectProjectDrift checks for drift strictly within project repository files,
// suitable for CI runners where user home directory configs do not exist.
func DetectProjectDrift(m *manifest.Manifest, projectRoot string) (*DriftReport, error) {
	report := &DriftReport{
		InSync:       true,
		ProjectName:  m.Name,
		TargetAgents: m.Agents,
		Items:        nil,
	}

	// 1. Verify that all referenced ${VAR} in MCPServers are declared in EnvTemplate
	seenMissingEnvs := make(map[string]bool)
	for srvName, srv := range m.MCPServers {
		var referencedVars []string
		referencedVars = append(referencedVars, extractEnvsFromString(srv.Command)...)
		for _, arg := range srv.Args {
			referencedVars = append(referencedVars, extractEnvsFromString(arg)...)
		}
		for _, v := range srv.Env {
			referencedVars = append(referencedVars, extractEnvsFromString(v)...)
		}
		for _, v := range srv.Headers {
			referencedVars = append(referencedVars, extractEnvsFromString(v)...)
		}
		if authStr, ok := srv.Auth.(string); ok {
			referencedVars = append(referencedVars, extractEnvsFromString(authStr)...)
		}

		for _, v := range referencedVars {
			if _, exists := m.EnvTemplate[v]; !exists && !seenMissingEnvs[v] {
				seenMissingEnvs[v] = true
				report.InSync = false
				report.MissingEnvs = append(report.MissingEnvs, v)
				report.Items = append(report.Items, DriftItem{
					Kind:        DriftMissingEnvTemplate,
					Name:        v,
					TargetFile:  "gandalf.toml",
					Description: fmt.Sprintf("Missing [env_template.%s] definition", v),
					Details:     fmt.Sprintf("Environment variable '${%s}' is referenced in MCP server '%s' but missing from [env_template]", v, srvName),
				})
			}
		}
	}

	// 2. Verify that all declared skills exist in project and contain SKILL.md
	for _, skill := range m.Skills {
		relPath := skill.Source
		if relPath == "" {
			relPath = filepath.Join(".gandalf", "skills", skill.Name)
		}
		skillDir := filepath.Join(projectRoot, filepath.Clean(relPath))
		fi, err := os.Stat(skillDir)
		if err != nil || !fi.IsDir() {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkill,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  relPath,
				Details:     fmt.Sprintf("Skill directory '%s' does not exist in repository", relPath),
			})
			continue
		}

		skillMD := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillMD); os.IsNotExist(err) {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkillFile,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  filepath.Join(relPath, "SKILL.md"),
				Details:     fmt.Sprintf("SKILL.md is missing inside skill directory '%s'", relPath),
			})
		}
	}

	// 3. Check project-level .mcp.json and .cursor/mcp.json if they exist
	projectMCPFiles := []string{".mcp.json", filepath.Join(".cursor", "mcp.json")}
	for _, relMCP := range projectMCPFiles {
		mcpPath := filepath.Join(projectRoot, relMCP)
		data, err := os.ReadFile(mcpPath)
		if err == nil {
			var parsed struct {
				MCPServers map[string]any `json:"mcpServers"`
			}
			if err := json.Unmarshal(data, &parsed); err == nil && parsed.MCPServers != nil {
				// Check for missing servers in project file that are declared in gandalf.toml
				for srvName := range m.MCPServers {
					if _, ok := parsed.MCPServers[srvName]; !ok {
						report.InSync = false
						report.Items = append(report.Items, DriftItem{
							Kind:        DriftUnsyncedProjectConfig,
							Name:        srvName,
							TargetFile:  relMCP,
							Description: fmt.Sprintf("MCP server '%s' declared in gandalf.toml is missing from project '%s'", srvName, relMCP),
							Details:     "Project agent configuration is out of sync with gandalf.toml (run 'gandalf apply' to sync)",
						})
					}
				}
				// Check for unmanaged extra servers in project file
				for srvName := range parsed.MCPServers {
					if _, ok := m.MCPServers[srvName]; !ok {
						report.InSync = false
						report.Items = append(report.Items, DriftItem{
							Kind:        DriftUnsyncedProjectConfig,
							Name:        srvName,
							TargetFile:  relMCP,
							Description: fmt.Sprintf("Unmanaged MCP server '%s' in project '%s' is not declared in gandalf.toml", srvName, relMCP),
							Details:     "Project file contains extra MCP servers not tracked by team manifest (run 'gandalf import' or update gandalf.toml)",
						})
					}
				}
			}
		}
	}

	return report, nil
}
