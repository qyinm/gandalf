package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type parsedCandidateResult struct {
	Source     DiscoveredSource
	MCPServers map[string]manifest.MCPServerDef
	Skills     []manifest.SkillDef
}

// ReconcileSources loads candidate files, parses them, resolves conflicts
// (project > global), applies secret templatization, and builds a consolidated Manifest.
func ReconcileSources(opts ImportOptions, candidates []DetectedCandidate) (*ImportResult, error) {
	var results []parsedCandidateResult
	var warnings []string

	for _, cand := range candidates {
		res := parsedCandidateResult{
			Source: DiscoveredSource{
				Agent: cand.Agent,
				Scope: cand.Scope,
				Path:  cand.Path,
			},
		}

		switch cand.Kind {
		case "mcp_json":
			data, err := os.ReadFile(cand.Path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to read %s: %v", cand.Path, err))
				continue
			}
			servers, err := ParseStandardJSONMCPServers(data)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to parse %s: %v", cand.Path, err))
				continue
			}
			res.MCPServers = servers
			res.Source.ServerCount = len(servers)

		case "claude_json":
			data, err := os.ReadFile(cand.Path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to read %s: %v", cand.Path, err))
				continue
			}
			servers, err := ParseClaudeConfigJSON(data, opts.ProjectPath)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to parse %s: %v", cand.Path, err))
				continue
			}
			res.MCPServers = servers
			res.Source.ServerCount = len(servers)

		case "codex_toml":
			data, err := os.ReadFile(cand.Path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to read %s: %v", cand.Path, err))
				continue
			}
			servers, err := ParseCodexConfigTOML(data)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to parse %s: %v", cand.Path, err))
				continue
			}
			res.MCPServers = servers
			res.Source.ServerCount = len(servers)

		case "skills_dir":
			skills, err := ScanSkillsDirectory(cand.Path, ".gandalf/skills")
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to scan skills in %s: %v", cand.Path, err))
				continue
			}
			res.Skills = skills
			res.Source.SkillCount = len(skills)
		}

		results = append(results, res)
	}

	if len(results) == 0 && len(candidates) > 0 {
		return nil, fmt.Errorf("all discovered candidate sources failed to parse: %s", strings.Join(warnings, "; "))
	}

	// Separate results into project and global
	var projectResults []parsedCandidateResult
	var globalResults []parsedCandidateResult

	for _, r := range results {
		if r.Source.Scope == "project" {
			projectResults = append(projectResults, r)
		} else {
			globalResults = append(globalResults, r)
		}
	}

	finalMCPServers := make(map[string]manifest.MCPServerDef)
	envTemplate := make(map[string]string)

	// 1. First add Global MCP servers
	for _, gr := range globalResults {
		for name, srv := range gr.MCPServers {
			finalMCPServers[name] = srv
		}
	}

	// 2. Project MCP servers take priority over Global MCP servers
	for _, pr := range projectResults {
		for name, srv := range pr.MCPServers {
			finalMCPServers[name] = srv
		}
	}

	// 3. Final Skills: Project takes precedence over Global
	finalSkillsMap := make(map[string]manifest.SkillDef)
	for _, gr := range globalResults {
		for _, sk := range gr.Skills {
			finalSkillsMap[sk.Name] = sk
		}
	}
	for _, pr := range projectResults {
		for _, sk := range pr.Skills {
			finalSkillsMap[sk.Name] = sk
		}
	}

	var skillNames []string
	for name := range finalSkillsMap {
		skillNames = append(skillNames, name)
	}
	sort.Strings(skillNames)
	var finalSkills []manifest.SkillDef
	for _, name := range skillNames {
		finalSkills = append(finalSkills, finalSkillsMap[name])
	}

	// 3. Templatize secrets across all servers deterministically (sort server keys first)
	var serverNames []string
	for name := range finalMCPServers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)
	for _, name := range serverNames {
		srv := finalMCPServers[name]
		copied := srv
		RedactAndTemplatizeServer(name, &copied, envTemplate)
		finalMCPServers[name] = copied
	}

	// 4. Determine Project Name
	projectName := filepath.Base(opts.ProjectPath)
	if projectName == "" || projectName == "." || projectName == "/" {
		projectName = "my-project"
	}

	// 5. Detect distinct agents from discovered sources
	agentSeen := make(map[types.AgentID]bool)
	for _, r := range results {
		switch r.Source.Agent {
		case "cursor":
			agentSeen[types.AgentCursor] = true
		case "claude_code", "claude-code", "claude":
			agentSeen[types.AgentClaudeCode] = true
		case "codex":
			agentSeen[types.AgentCodex] = true
		}
	}
	var targetAgents []types.AgentID
	for _, a := range []types.AgentID{types.AgentClaudeCode, types.AgentCodex, types.AgentCursor} {
		if agentSeen[a] {
			targetAgents = append(targetAgents, a)
		}
	}
	if len(targetAgents) == 0 {
		targetAgents = []types.AgentID{types.AgentClaudeCode, types.AgentCodex, types.AgentCursor}
	}

	// 6. Build final Manifest
	m := &manifest.Manifest{
		Version:     "1.0",
		Name:        projectName,
		Description: fmt.Sprintf("Standardized AI agent environment for %s", projectName),
		Agents:      targetAgents,
		MCPServers:  finalMCPServers,
		Skills:      finalSkills,
		EnvTemplate: envTemplate,
	}

	var discoveredSources []DiscoveredSource
	for _, r := range results {
		discoveredSources = append(discoveredSources, r.Source)
	}

	formattedTOML := FormatManifestTOML(m)

	return &ImportResult{
		Manifest:      m,
		Sources:       discoveredSources,
		ExtractedEnvs: envTemplate,
		Warnings:      warnings,
		FormattedTOML: formattedTOML,
	}, nil
}

// FormatManifestTOML generates a clean, readable gandalf.toml string from a Manifest.
func FormatManifestTOML(m *manifest.Manifest) string {
	var sb strings.Builder

	sb.WriteString("# gandalf.toml - AI Agent Environment Declaration\n")
	sb.WriteString(fmt.Sprintf("version = %q\n", m.Version))
	sb.WriteString(fmt.Sprintf("name = %q\n", m.Name))
	if m.Description != "" {
		sb.WriteString(fmt.Sprintf("description = %q\n", m.Description))
	}

	// Agents
	sb.WriteString("agents = [")
	for i, a := range m.Agents {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%q", a))
	}
	sb.WriteString("]\n\n")

	// MCP Servers (sorted by key for deterministic output)
	if len(m.MCPServers) > 0 {
		sb.WriteString("# Team MCP Servers\n")
		var serverNames []string
		for name := range m.MCPServers {
			serverNames = append(serverNames, name)
		}
		sort.Strings(serverNames)

		for _, name := range serverNames {
			srv := m.MCPServers[name]
			sb.WriteString(fmt.Sprintf("[mcp_servers.%s]\n", formatTOMLKey(name)))
			if srv.Type != "" {
				sb.WriteString(fmt.Sprintf("type = %q\n", srv.Type))
			}
			if srv.Command != "" {
				sb.WriteString(fmt.Sprintf("command = %q\n", srv.Command))
			}
			if len(srv.Args) > 0 {
				sb.WriteString("args = [")
				for j, arg := range srv.Args {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(fmt.Sprintf("%q", arg))
				}
				sb.WriteString("]\n")
			}
			if srv.EnvFile != "" {
				sb.WriteString(fmt.Sprintf("env_file = %q\n", srv.EnvFile))
			}
			if srv.URL != "" {
				sb.WriteString(fmt.Sprintf("url = %q\n", srv.URL))
			}
			if len(srv.Headers) > 0 {
				var hKeys []string
				for k := range srv.Headers {
					hKeys = append(hKeys, k)
				}
				sort.Strings(hKeys)
				sb.WriteString("headers = { ")
				for j, k := range hKeys {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(fmt.Sprintf("%s = %q", k, srv.Headers[k]))
				}
				sb.WriteString(" }\n")
			}
			if srv.Auth != nil {
				switch a := srv.Auth.(type) {
				case string:
					sb.WriteString(fmt.Sprintf("auth = %q\n", a))
				case map[string]any:
					var aKeys []string
					for k := range a {
						aKeys = append(aKeys, k)
					}
					sort.Strings(aKeys)
					sb.WriteString("auth = { ")
					for j, k := range aKeys {
						if j > 0 {
							sb.WriteString(", ")
						}
						sb.WriteString(fmt.Sprintf("%s = %s", k, formatTOMLValue(a[k])))
					}
					sb.WriteString(" }\n")
				}
			}
			if len(srv.Env) > 0 {
				var envKeys []string
				for k := range srv.Env {
					envKeys = append(envKeys, k)
				}
				sort.Strings(envKeys)
				sb.WriteString("env = { ")
				for j, k := range envKeys {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(fmt.Sprintf("%s = %q", k, srv.Env[k]))
				}
				sb.WriteString(" }\n")
			}
			if len(srv.RequiredEnv) > 0 {
				sort.Strings(srv.RequiredEnv)
				sb.WriteString("required_env = [")
				for j, req := range srv.RequiredEnv {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(fmt.Sprintf("%q", req))
				}
				sb.WriteString("]\n")
			}
			if srv.Description != "" {
				sb.WriteString(fmt.Sprintf("description = %q\n", srv.Description))
			}
			if srv.Disabled {
				sb.WriteString("disabled = true\n")
			}
			sb.WriteString("\n")
		}
	}

	// Skills
	if len(m.Skills) > 0 {
		sb.WriteString("# Team Skills\n")
		for _, sk := range m.Skills {
			sb.WriteString("[[skills]]\n")
			sb.WriteString(fmt.Sprintf("name = %q\n", sk.Name))
			if sk.Source != "" {
				sb.WriteString(fmt.Sprintf("source = %q\n", sk.Source))
			}
			if sk.Description != "" {
				sb.WriteString(fmt.Sprintf("description = %q\n", sk.Description))
			}
			sb.WriteString("\n")
		}
	}

	// Env Template
	if len(m.EnvTemplate) > 0 {
		sb.WriteString("# Required Environment Variables Template\n")
		sb.WriteString("[env_template]\n")
		var envKeys []string
		for k := range m.EnvTemplate {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		for _, k := range envKeys {
			sb.WriteString(fmt.Sprintf("%s = %q\n", k, m.EnvTemplate[k]))
		}
	}

	return sb.String()
}

func formatTOMLKey(key string) string {
	if regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(key) && !strings.Contains(key, ".") {
		return key
	}
	return fmt.Sprintf("%q", key)
}

func formatTOMLValue(v any) string {
	if v == nil {
		return `""`
	}
	switch val := v.(type) {
	case bool:
		return fmt.Sprintf("%t", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%f", val)
	case string:
		return fmt.Sprintf("%q", val)
	case []any:
		var items []string
		for _, item := range val {
			items = append(items, formatTOMLValue(item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case []string:
		var items []string
		for _, item := range val {
			items = append(items, fmt.Sprintf("%q", item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", val))
	}
}
