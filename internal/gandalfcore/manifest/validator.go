package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
)

// Validate checks the manifest for schema compliance, semantic validity, and path security.
func Validate(m *Manifest, projectRoot string) []ValidationError {
	var errors []ValidationError

	if m.Version == "" {
		errors = append(errors, ValidationError{
			Field:   "version",
			Problem: "Manifest version is missing",
			Fix:     "Add 'version = \"1.0\"' to the top of gandalf.toml",
		})
	}

	if m.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Problem: "Project/team name is missing",
			Fix:     "Add 'name = \"your-team-project\"' to gandalf.toml",
		})
	}

	if len(m.Agents) == 0 {
		errors = append(errors, ValidationError{
			Field:   "agents",
			Problem: "No target agents specified",
			Fix:     fmt.Sprintf("Specify agents from the supported set: %s", strings.Join(agents.CurrentSupportedNames(), ", ")),
		})
	} else {
		for _, agent := range m.Agents {
			if !agents.IsCurrentSupported(agent) {
				errors = append(errors, ValidationError{
					Field:   "agents",
					Problem: fmt.Sprintf("Agent '%s' is not in the currently supported agent set (supported: %s)", agent, strings.Join(agents.CurrentSupportedNames(), ", ")),
					Fix:     fmt.Sprintf("Use supported agent IDs: %s", strings.Join(agents.CurrentSupportedNames(), ", ")),
				})
			}
		}
	}

	for name, srv := range m.MCPServers {
		if strings.TrimSpace(name) == "" {
			errors = append(errors, ValidationError{
				Field:   "mcp_servers",
				Problem: "MCP server name cannot be empty",
				Fix:     "Provide a valid identifier for the MCP server",
			})
		}
		if srv.Command == "" && srv.URL == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("mcp_servers.%s", name),
				Problem: fmt.Sprintf("MCP server '%s' must specify either 'command' or 'url'", name),
				Fix:     "Add a command (e.g. 'npx') or URL (e.g. 'https://...') to the server definition",
			})
		}
	}

	for i, skill := range m.Skills {
		if skill.Name == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("skills[%d].name", i),
				Problem: "Skill name is missing",
				Fix:     "Provide a name for the skill",
			})
		}

		if skill.Source != "" && projectRoot != "" {
			cleanSource := filepath.Clean(skill.Source)
			fullPath := filepath.Join(projectRoot, cleanSource)

			// Security check: Must not escape project root
			if pathconfinement.PathHasTraversal(skill.Source) || !pathconfinement.IsStrictlyUnder(fullPath, filepath.Clean(projectRoot)) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("skills[%d].source", i),
					Problem: fmt.Sprintf("Skill '%s' source path '%s' escapes project root", skill.Name, skill.Source),
					Fix:     "Place skill inside the project root (e.g. './.gandalf/skills/...')",
				})
			}
		}
	}

	for name, hook := range m.Hooks {
		if hook.Event == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("hooks.%s.event", name),
				Problem: fmt.Sprintf("Hook '%s' is missing 'event'", name),
				Fix:     "Specify an event (e.g. 'before_save', 'on_start')",
			})
		}
		if hook.Command == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("hooks.%s.command", name),
				Problem: fmt.Sprintf("Hook '%s' is missing 'command'", name),
				Fix:     "Specify a command to execute for the hook",
			})
		}
	}

	// Profiles validation
	for profName, prof := range m.Profiles {
		if strings.TrimSpace(profName) == "" {
			errors = append(errors, ValidationError{
				Field:   "profiles",
				Problem: "Profile name cannot be empty",
				Fix:     "Provide a valid identifier for the profile (e.g. [profiles.frontend])",
			})
		} else if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(profName) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("profiles.%s", profName),
				Problem: fmt.Sprintf("Profile name '%s' contains invalid characters", profName),
				Fix:     "Use alphanumeric characters, dashes, and underscores only",
			})
		}

		for _, inc := range prof.Includes {
			if _, exists := m.Profiles[inc]; !exists {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("profiles.%s.includes", profName),
					Problem: fmt.Sprintf("Profile '%s' includes unknown profile '%s'", profName, inc),
					Fix:     fmt.Sprintf("Declare [profiles.%s] before referencing it", inc),
				})
			}
		}

		for _, sk := range prof.Skills {
			found := false
			for _, def := range m.Skills {
				if def.Name == sk {
					found = true
					break
				}
			}
			if !found && projectRoot != "" {
				skillPath := filepath.Join(projectRoot, ".gandalf", "skills", sk)
				if info, err := os.Stat(skillPath); err == nil && info.IsDir() {
					found = true
				}
			}
			if !found {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("profiles.%s.skills", profName),
					Problem: fmt.Sprintf("Profile '%s' references undeclared skill '%s'", profName, sk),
					Fix:     fmt.Sprintf("Declare [[skills]] with name = %q or add it to .gandalf/skills/%s", sk, sk),
				})
			}
		}
	}

	// Cycle detection for profile includes
	if cycle := detectProfileCycle(m.Profiles); len(cycle) > 0 {
		errors = append(errors, ValidationError{
			Field:   "profiles.includes",
			Problem: fmt.Sprintf("Circular inheritance detected in profiles: %s", strings.Join(cycle, " -> ")),
			Fix:     "Break the inheritance cycle between profiles",
		})
	}

	return errors
}

func detectProfileCycle(profiles map[string]ProfileDef) []string {
	visited := make(map[string]int) // 0: unvisited, 1: visiting, 2: visited
	var path []string

	var dfs func(node string) []string
	dfs = func(node string) []string {
		visited[node] = 1
		path = append(path, node)

		if prof, ok := profiles[node]; ok {
			for _, inc := range prof.Includes {
				if visited[inc] == 1 {
					cycleStart := -1
					for i, p := range path {
						if p == inc {
							cycleStart = i
							break
						}
					}
					if cycleStart >= 0 {
						res := append([]string{}, path[cycleStart:]...)
						res = append(res, inc)
						return res
					}
					return []string{node, inc}
				}
				if visited[inc] == 0 {
					if c := dfs(inc); len(c) > 0 {
						return c
					}
				}
			}
		}

		visited[node] = 2
		path = path[:len(path)-1]
		return nil
	}

	for name := range profiles {
		if visited[name] == 0 {
			if c := dfs(name); len(c) > 0 {
				return c
			}
		}
	}
	return nil
}
