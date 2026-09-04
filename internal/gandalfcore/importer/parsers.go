package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
)

// rawMCPServerJSON represents the generic JSON structure used in .cursor/mcp.json, .mcp.json, and ~/.claude.json.
type rawMCPServerJSON struct {
	Type        string            `json:"type,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	EnvFile     string            `json:"envFile,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Auth        any               `json:"auth,omitempty"`
	Disabled    bool              `json:"disabled,omitempty"`
	Description string            `json:"description,omitempty"`
}

// ParseStandardJSONMCPServers parses a JSON document containing {"mcpServers": { ... }}.
func ParseStandardJSONMCPServers(data []byte) (map[string]manifest.MCPServerDef, error) {
	var root struct {
		MCPServers map[string]rawMCPServerJSON `json:"mcpServers"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("unmarshal mcpServers json: %w", err)
	}

	result := make(map[string]manifest.MCPServerDef)
	for name, raw := range root.MCPServers {
		srv := manifest.MCPServerDef{
			Type:        raw.Type,
			Command:     raw.Command,
			Args:        raw.Args,
			Env:         raw.Env,
			EnvFile:     sanitizeEnvFilePath(raw.EnvFile),
			URL:         raw.URL,
			Headers:     raw.Headers,
			Auth:        raw.Auth,
			Disabled:    raw.Disabled,
			Description: raw.Description,
		}
		result[name] = srv
	}

	return result, nil
}

// ParseClaudeConfigJSON parses ~/.claude.json, extracting both root-level user servers
// and project-specific servers for the given project path.
func ParseClaudeConfigJSON(data []byte, projectPath string) (map[string]manifest.MCPServerDef, error) {
	var root struct {
		MCPServers map[string]rawMCPServerJSON `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers map[string]rawMCPServerJSON `json:"mcpServers"`
		} `json:"projects"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("unmarshal claude config json: %w", err)
	}

	result := make(map[string]manifest.MCPServerDef)

	// 1. User-scoped servers
	for name, raw := range root.MCPServers {
		result[name] = manifest.MCPServerDef{
			Type:        raw.Type,
			Command:     raw.Command,
			Args:        raw.Args,
			Env:         raw.Env,
			EnvFile:     sanitizeEnvFilePath(raw.EnvFile),
			URL:         raw.URL,
			Headers:     raw.Headers,
			Auth:        raw.Auth,
			Disabled:    raw.Disabled,
			Description: raw.Description,
		}
	}

	// 2. Project-scoped servers (match cleaned projectPath)
	cleanProj := filepath.Clean(projectPath)
	for pPath, proj := range root.Projects {
		if filepath.Clean(pPath) == cleanProj {
			for name, raw := range proj.MCPServers {
				result[name] = manifest.MCPServerDef{
					Type:        raw.Type,
					Command:     raw.Command,
					Args:        raw.Args,
					Env:         raw.Env,
					EnvFile:     sanitizeEnvFilePath(raw.EnvFile),
					URL:         raw.URL,
					Headers:     raw.Headers,
					Auth:        raw.Auth,
					Disabled:    raw.Disabled,
					Description: raw.Description,
				}
			}
		}
	}

	return result, nil
}

// ParseCodexConfigTOML parses Codex config.toml containing [mcp_servers.<name>] tables.
func ParseCodexConfigTOML(data []byte) (map[string]manifest.MCPServerDef, error) {
	parsed, err := manifest.Parse(string(data), &manifest.ParseOptions{NoInterpolate: true})
	if err != nil {
		return nil, fmt.Errorf("parse codex toml config: %w", err)
	}

	// Clean up any remaining .env virtual servers and fold them into the parent server
	servers := parsed.Manifest.MCPServers
	for name, srv := range servers {
		if strings.HasSuffix(name, ".env") {
			parentName := strings.TrimSuffix(name, ".env")
			if parent, exists := servers[parentName]; exists {
				if parent.Env == nil {
					parent.Env = make(map[string]string)
				}
				for k, v := range srv.Env {
					parent.Env[k] = v
				}
				servers[parentName] = parent
			}
			delete(servers, name)
		}
	}

	return servers, nil
}

// ScanSkillsDirectory scans a directory (e.g. .cursor/skills/ or .claude/skills/)
// for subdirectories containing SKILL.md and returns SkillDefs.
func ScanSkillsDirectory(skillsDir, targetSourcePrefix string) ([]manifest.SkillDef, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []manifest.SkillDef
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		skillMD := filepath.Join(skillsDir, skillName, "SKILL.md")
		if _, err := os.Stat(skillMD); err == nil {
			sourceRel := filepath.Join(targetSourcePrefix, skillName)
			skills = append(skills, manifest.SkillDef{
				Name:        skillName,
				Source:      sourceRel,
				Description: fmt.Sprintf("Imported skill from %s", filepath.Base(skillsDir)),
			})
		}
	}

	return skills, nil
}

// sanitizeEnvFilePath validates that an envFile path is safe and does not escape the project boundary.
func sanitizeEnvFilePath(envFile string) string {
	envFile = strings.TrimSpace(envFile)
	if envFile == "" {
		return ""
	}
	if strings.Contains(envFile, "${workspaceFolder}") {
		rest := strings.Replace(envFile, "${workspaceFolder}", "", 1)
		clean := filepath.Clean(strings.TrimPrefix(filepath.ToSlash(rest), "/"))
		if strings.Contains(rest, "..") || strings.HasPrefix(clean, "..") || clean == "." || clean == "" {
			return ""
		}
		return envFile
	}
	if filepath.IsAbs(envFile) {
		return ""
	}
	clean := filepath.Clean(envFile)
	if strings.HasPrefix(clean, "..") {
		return ""
	}
	return clean
}
