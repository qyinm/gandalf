package manifest

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// Manifest represents the top-level declaration in gandalf.toml.
type Manifest struct {
	Version     string                   `json:"version" toml:"version"`
	Name        string                   `json:"name" toml:"name"`
	Description string                   `json:"description,omitempty" toml:"description,omitempty"`
	Agents      []types.AgentID          `json:"agents" toml:"agents"`
	MCPServers  map[string]MCPServerDef  `json:"mcp_servers,omitempty" toml:"mcp_servers,omitempty"`
	Skills      []SkillDef               `json:"skills,omitempty" toml:"skills,omitempty"`
	Hooks       map[string]HookDef       `json:"hooks,omitempty" toml:"hooks,omitempty"`
	EnvTemplate map[string]string        `json:"env_template,omitempty" toml:"env_template,omitempty"`
}

// MCPServerDef defines an MCP server in the manifest.
type MCPServerDef struct {
	Type        string            `json:"type,omitempty" toml:"type,omitempty"`
	Command     string            `json:"command,omitempty" toml:"command,omitempty"`
	Args        []string          `json:"args,omitempty" toml:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty" toml:"env,omitempty"`
	EnvFile     string            `json:"env_file,omitempty" toml:"env_file,omitempty"`
	URL         string            `json:"url,omitempty" toml:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
	RequiredEnv []string          `json:"required_env,omitempty" toml:"required_env,omitempty"`
	Description string            `json:"description,omitempty" toml:"description,omitempty"`
	Disabled    bool              `json:"disabled,omitempty" toml:"disabled,omitempty"`
}

// SkillDef defines a team skill in the manifest.
type SkillDef struct {
	Name        string `json:"name" toml:"name"`
	Source      string `json:"source,omitempty" toml:"source,omitempty"`
	Git         string `json:"git,omitempty" toml:"git,omitempty"`
	Path        string `json:"path,omitempty" toml:"path,omitempty"`
	Ref         string `json:"ref,omitempty" toml:"ref,omitempty"`
	Description string `json:"description,omitempty" toml:"description,omitempty"`
}

// HookDef defines an agent hook in the manifest.
type HookDef struct {
	Event       string `json:"event" toml:"event"`
	Command     string `json:"command" toml:"command"`
	Description string `json:"description,omitempty" toml:"description,omitempty"`
}

// ValidationError represents a validation problem in a manifest.
type ValidationError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
	Fix     string `json:"fix"`
}

func (v ValidationError) Error() string {
	return v.Field + ": " + v.Problem
}
