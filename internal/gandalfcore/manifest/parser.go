package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::-([^}]*))?\}`)

// ParseOptions controls how the manifest is parsed and interpolated.
type ParseOptions struct {
	EnvGetter func(string) string
}

// ParseResult holds the parsed manifest and any missing required environment variables.
type ParseResult struct {
	Manifest    *Manifest
	MissingEnvs []string
}

// LoadManifest reads and parses a gandalf.toml file from disk.
func LoadManifest(manifestPath string, opts *ParseOptions) (*ParseResult, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest file '%s': %w", manifestPath, err)
	}
	return Parse(string(data), opts)
}

// FindManifestFile searches for a gandalf.toml (or fallback formats) in the given directory.
func FindManifestFile(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "gandalf.toml"),
		filepath.Join(dir, "Agentfile.toml"),
		filepath.Join(dir, "gandalf.yaml"),
		filepath.Join(dir, "gandalf.yml"),
		filepath.Join(dir, "gandalf.jsonc"),
		filepath.Join(dir, "gandalf.json"),
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	return "", fmt.Errorf("no manifest file found in '%s' (checked gandalf.toml, gandalf.yaml, etc.)", dir)
}

// Parse parses a TOML string into a Manifest with environment variable interpolation.
func Parse(text string, opts *ParseOptions) (*ParseResult, error) {
	envGetter := os.Getenv
	if opts != nil && opts.EnvGetter != nil {
		envGetter = opts.EnvGetter
	}

	missingSet := make(map[string]struct{})

	// Pre-interpolate environment variables in text
	interpolatedText := envVarPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := envVarPattern.FindStringSubmatch(match)
		if len(submatches) >= 2 {
			varName := submatches[1]
			val := envGetter(varName)
			if val != "" {
				return val
			}
			// check default value ${VAR:-default}
			if len(submatches) >= 3 && submatches[2] != "" {
				return submatches[2]
			}
			missingSet[varName] = struct{}{}
		}
		return match
	})

	m := &Manifest{
		MCPServers:  make(map[string]MCPServerDef),
		Hooks:       make(map[string]HookDef),
		EnvTemplate: make(map[string]string),
	}

	lines := strings.Split(interpolatedText, "\n")
	var currentSection string
	var currentSubSection string
	var currentSkill *SkillDef

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check [[skills]] array of tables
		if strings.TrimSpace(line) == "[[skills]]" {
			if currentSkill != nil {
				m.Skills = append(m.Skills, *currentSkill)
			}
			currentSkill = &SkillDef{}
			currentSection = "skills"
			currentSubSection = ""
			continue
		}

		// Check [section] table header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if currentSkill != nil {
				m.Skills = append(m.Skills, *currentSkill)
				currentSkill = nil
			}

			header := strings.TrimSpace(line[1 : len(line)-1])
			parts := strings.Split(header, ".")

			currentSection = parts[0]
			if len(parts) > 1 {
				currentSubSection = strings.Trim(strings.Join(parts[1:], "."), "\"")
			} else {
				currentSubSection = ""
			}
			continue
		}

		key, val, ok := parseKeyValue(line)
		if !ok {
			continue
		}

		// Handle multiline arrays if needed
		if strings.HasPrefix(val, "[") && !strings.HasSuffix(val, "]") {
			var arrayLines []string
			arrayLines = append(arrayLines, val)
			for i+1 < len(lines) {
				i++
				nextLine := strings.TrimSpace(lines[i])
				if nextLine == "" || strings.HasPrefix(nextLine, "#") {
					continue
				}
				arrayLines = append(arrayLines, nextLine)
				if strings.HasSuffix(nextLine, "]") {
					break
				}
			}
			val = strings.Join(arrayLines, " ")
		}

		switch currentSection {
		case "":
			// Top-level fields
			switch key {
			case "version":
				m.Version = unquote(val)
			case "name":
				m.Name = unquote(val)
			case "description":
				m.Description = unquote(val)
			case "agents":
				rawList := parseStringArray(val)
				for _, a := range rawList {
					m.Agents = append(m.Agents, types.ParseAgentID(a))
				}
			}

		case "mcp_servers":
			if currentSubSection != "" {
				server := m.MCPServers[currentSubSection]
				if server.Env == nil {
					server.Env = make(map[string]string)
				}
				if server.Headers == nil {
					server.Headers = make(map[string]string)
				}

				switch key {
				case "command":
					server.Command = unquote(val)
				case "args":
					server.Args = parseStringArray(val)
				case "url":
					server.URL = unquote(val)
				case "description":
					server.Description = unquote(val)
				case "disabled":
					server.Disabled = val == "true"
				case "required_env":
					server.RequiredEnv = parseStringArray(val)
				}
				m.MCPServers[currentSubSection] = server
			}

		case "skills":
			if currentSkill != nil {
				switch key {
				case "name":
					currentSkill.Name = unquote(val)
				case "source":
					currentSkill.Source = unquote(val)
				case "git":
					currentSkill.Git = unquote(val)
				case "path":
					currentSkill.Path = unquote(val)
				case "ref":
					currentSkill.Ref = unquote(val)
				case "description":
					currentSkill.Description = unquote(val)
				}
			}

		case "hooks":
			if currentSubSection != "" {
				hook := m.Hooks[currentSubSection]
				switch key {
				case "event":
					hook.Event = unquote(val)
				case "command":
					hook.Command = unquote(val)
				case "description":
					hook.Description = unquote(val)
				}
				m.Hooks[currentSubSection] = hook
			}

		case "env_template":
			m.EnvTemplate[key] = unquote(val)
		}
	}

	if currentSkill != nil {
		m.Skills = append(m.Skills, *currentSkill)
	}

	// Validate required_env from MCP definitions
	for _, server := range m.MCPServers {
		for _, req := range server.RequiredEnv {
			if envGetter(req) == "" {
				missingSet[req] = struct{}{}
			}
		}
	}

	var missingEnvs []string
	for k := range missingSet {
		missingEnvs = append(missingEnvs, k)
	}

	return &ParseResult{
		Manifest:    m,
		MissingEnvs: missingEnvs,
	}, nil
}

func parseKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, "=")
	if idx == -1 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	val := strings.TrimSpace(line[idx+1:])
	return key, val, true
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if unquoted, err := strconv.Unquote(s); err == nil {
		return unquoted
	}
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		if len(s) >= 2 {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if inner == "" {
		return nil
	}

	var items []string
	for _, piece := range strings.Split(inner, ",") {
		clean := unquote(piece)
		if clean != "" {
			items = append(items, clean)
		}
	}
	return items
}
