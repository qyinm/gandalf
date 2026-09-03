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
	EnvGetter     func(string) string
	NoInterpolate bool
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

	// Pre-interpolate environment variables in text unless NoInterpolate is requested
	interpolatedText := text
	if opts == nil || !opts.NoInterpolate {
		interpolatedText = contextAwareInterpolate(text, envGetter, missingSet)
	}

	m := &Manifest{
		MCPServers:  make(map[string]MCPServerDef),
		Hooks:       make(map[string]HookDef),
		EnvTemplate: make(map[string]string),
	}

	lines := strings.Split(interpolatedText, "\n")
	var currentSection string
	var currentSubSection string
	var isEnvTable bool
	var currentSkill *SkillDef

	for i := 0; i < len(lines); i++ {
		rawLine := strings.TrimSpace(lines[i])
		line := strings.TrimSpace(stripInlineComment(rawLine))
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
			isEnvTable = false
			continue
		}

		// Check [section] table header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if currentSkill != nil {
				m.Skills = append(m.Skills, *currentSkill)
				currentSkill = nil
			}

			header := strings.TrimSpace(line[1 : len(line)-1])
			parts := splitTOMLHeader(header)

			currentSection = ""
			currentSubSection = ""
			isEnvTable = false

			if len(parts) > 0 {
				currentSection = parts[0]
			}
			if len(parts) == 2 {
				currentSubSection = parts[1]
			} else if len(parts) >= 3 {
				if parts[len(parts)-1] == "env" {
					isEnvTable = true
					currentSubSection = strings.Join(parts[1:len(parts)-1], ".")
				} else {
					currentSubSection = strings.Join(parts[1:], ".")
				}
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
				// Support nested [mcp_servers.<name>.env] tables (e.g. Codex TOML)
				if isEnvTable || strings.HasSuffix(currentSubSection, ".env") {
					parentName := currentSubSection
					if strings.HasSuffix(currentSubSection, ".env") {
						parentName = strings.TrimSuffix(currentSubSection, ".env")
					}
					parentName = strings.Trim(parentName, "\"")
					server := m.MCPServers[parentName]
					if server.Env == nil {
						server.Env = make(map[string]string)
					}
					server.Env[key] = unquote(val)
					m.MCPServers[parentName] = server
					continue
				}

				cleanSubSection := strings.Trim(currentSubSection, "\"")
				server := m.MCPServers[cleanSubSection]
				if server.Env == nil {
					server.Env = make(map[string]string)
				}
				if server.Headers == nil {
					server.Headers = make(map[string]string)
				}

				if strings.HasPrefix(key, "env.") {
					envKey := strings.TrimPrefix(key, "env.")
					server.Env[envKey] = unquote(val)
					m.MCPServers[cleanSubSection] = server
					continue
				}

				switch key {
				case "type":
					server.Type = unquote(val)
				case "command":
					server.Command = unquote(val)
				case "args":
					server.Args = parseStringArray(val)
				case "env_file":
					server.EnvFile = unquote(val)
				case "url":
					server.URL = unquote(val)
				case "description":
					server.Description = unquote(val)
				case "headers":
					server.Headers = parseInlineTable(val)
				case "env":
					server.Env = parseInlineTable(val)
				case "auth":
					trimmedVal := strings.TrimSpace(val)
					if strings.HasPrefix(trimmedVal, "{") && strings.HasSuffix(trimmedVal, "}") {
						server.Auth = parseInlineTableAny(trimmedVal)
					} else {
						server.Auth = unquote(trimmedVal)
					}
				case "disabled":
					server.Disabled = val == "true"
				case "enabled":
					if val == "false" {
						server.Disabled = true
					} else if val == "true" {
						server.Disabled = false
					}
				case "required_env":
					server.RequiredEnv = parseStringArray(val)
				}
				m.MCPServers[cleanSubSection] = server
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
	var current strings.Builder
	inQuote := false
	var quoteChar rune
	escaped := false

	for _, r := range inner {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if (r == '"' || r == '\'') && !inQuote {
			inQuote = true
			quoteChar = r
			current.WriteRune(r)
			continue
		}
		if inQuote && r == quoteChar {
			inQuote = false
			current.WriteRune(r)
			continue
		}
		if r == ',' && !inQuote {
			clean := unquote(strings.TrimSpace(current.String()))
			if clean != "" {
				items = append(items, clean)
			}
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}

	if current.Len() > 0 {
		clean := unquote(strings.TrimSpace(current.String()))
		if clean != "" {
			items = append(items, clean)
		}
	}
	return items
}

func parseInlineTable(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") || !strings.HasSuffix(raw, "}") {
		return nil
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if inner == "" {
		return make(map[string]string)
	}

	result := make(map[string]string)
	var pairs []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune
	escaped := false
	depth := 0

	for _, r := range inner {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if (r == '"' || r == '\'') && !inQuote {
			inQuote = true
			quoteChar = r
			current.WriteRune(r)
			continue
		}
		if inQuote && r == quoteChar {
			inQuote = false
			current.WriteRune(r)
			continue
		}
		if !inQuote {
			if r == '{' || r == '[' {
				depth++
			} else if r == '}' || r == ']' {
				depth--
			}
		}
		if r == ',' && !inQuote && depth == 0 {
			trimmed := strings.TrimSpace(current.String())
			if trimmed != "" {
				pairs = append(pairs, trimmed)
			}
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		trimmed := strings.TrimSpace(current.String())
		if trimmed != "" {
			pairs = append(pairs, trimmed)
		}
	}

	for _, pair := range pairs {
		k, v, ok := parseKeyValue(pair)
		if ok {
			cleanKey := unquote(k)
			result[cleanKey] = unquote(v)
		}
	}
	return result
}

func parseInlineTableAny(val string) any {
	trimmedVal := strings.TrimSpace(val)
	if strings.HasPrefix(trimmedVal, "{") && strings.HasSuffix(trimmedVal, "}") {
		inner := strings.TrimSpace(trimmedVal[1 : len(trimmedVal)-1])
		var pairs []string
		var current strings.Builder
		inQuote := false
		var quoteChar rune
		escaped := false
		depth := 0

		for _, r := range inner {
			if escaped {
				current.WriteRune(r)
				escaped = false
				continue
			}
			if r == '\\' {
				current.WriteRune(r)
				escaped = true
				continue
			}
			if (r == '"' || r == '\'') && !inQuote {
				inQuote = true
				quoteChar = r
				current.WriteRune(r)
				continue
			}
			if inQuote && r == quoteChar {
				inQuote = false
				current.WriteRune(r)
				continue
			}
			if !inQuote {
				if r == '{' || r == '[' {
					depth++
				} else if r == '}' || r == ']' {
					depth--
				}
			}
			if r == ',' && !inQuote && depth == 0 {
				trimmed := strings.TrimSpace(current.String())
				if trimmed != "" {
					pairs = append(pairs, trimmed)
				}
				current.Reset()
				continue
			}
			current.WriteRune(r)
		}
		if current.Len() > 0 {
			trimmed := strings.TrimSpace(current.String())
			if trimmed != "" {
				pairs = append(pairs, trimmed)
			}
		}

		resMap := make(map[string]any)
		for _, pair := range pairs {
			k, v, ok := parseKeyValue(pair)
			if ok {
				cleanKey := unquote(k)
				trimmedV := strings.TrimSpace(v)
				if strings.HasPrefix(trimmedV, "{") && strings.HasSuffix(trimmedV, "}") {
					resMap[cleanKey] = parseInlineTableAny(trimmedV)
				} else if strings.HasPrefix(trimmedV, "[") && strings.HasSuffix(trimmedV, "]") {
					resMap[cleanKey] = parseMixedArray(trimmedV)
				} else if trimmedV == "true" {
					resMap[cleanKey] = true
				} else if trimmedV == "false" {
					resMap[cleanKey] = false
				} else if intVal, err := strconv.Atoi(trimmedV); err == nil {
					resMap[cleanKey] = intVal
				} else if floatVal, err := strconv.ParseFloat(trimmedV, 64); err == nil {
					resMap[cleanKey] = floatVal
				} else {
					resMap[cleanKey] = unquote(trimmedV)
				}
			}
		}
		return resMap
	}
	return unquote(trimmedVal)
}

func parseMixedArray(val string) []any {
	trimmed := strings.TrimSpace(val)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return nil
	}
	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return []any{}
	}

	var elements []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune
	escaped := false
	depth := 0

	for _, r := range inner {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if (r == '"' || r == '\'') && !inQuote {
			inQuote = true
			quoteChar = r
			current.WriteRune(r)
			continue
		}
		if inQuote && r == quoteChar {
			inQuote = false
			current.WriteRune(r)
			continue
		}
		if !inQuote {
			if r == '{' || r == '[' {
				depth++
			} else if r == '}' || r == ']' {
				depth--
			}
		}
		if r == ',' && !inQuote && depth == 0 {
			t := strings.TrimSpace(current.String())
			if t != "" {
				elements = append(elements, t)
			}
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		t := strings.TrimSpace(current.String())
		if t != "" {
			elements = append(elements, t)
		}
	}

	var result []any
	for _, el := range elements {
		elTrim := strings.TrimSpace(el)
		if strings.HasPrefix(elTrim, "{") && strings.HasSuffix(elTrim, "}") {
			result = append(result, parseInlineTableAny(elTrim))
		} else if strings.HasPrefix(elTrim, "[") && strings.HasSuffix(elTrim, "]") {
			result = append(result, parseMixedArray(elTrim))
		} else if elTrim == "true" {
			result = append(result, true)
		} else if elTrim == "false" {
			result = append(result, false)
		} else if intVal, err := strconv.Atoi(elTrim); err == nil {
			result = append(result, intVal)
		} else if floatVal, err := strconv.ParseFloat(elTrim, 64); err == nil {
			result = append(result, floatVal)
		} else {
			result = append(result, unquote(elTrim))
		}
	}
	return result
}

func splitTOMLHeader(header string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune
	escaped := false

	for _, r := range header {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && inQuote && quoteChar == '"' {
			escaped = true
			current.WriteRune(r)
			continue
		}
		if inQuote {
			if r == quoteChar {
				inQuote = false
				current.WriteRune(r)
			} else {
				current.WriteRune(r)
			}
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
				current.WriteRune(r)
			} else if r == '.' {
				parts = append(parts, unquote(current.String()))
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		}
	}
	if current.Len() > 0 || len(parts) > 0 {
		parts = append(parts, unquote(current.String()))
	}
	return parts
}

func stripInlineComment(line string) string {
	inSingleQuote := false
	inDoubleQuote := false
	var escaped bool

	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
		} else if r == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
		} else if r == '#' && !inSingleQuote && !inDoubleQuote {
			return strings.TrimRight(line[:i], " \t")
		}
	}
	return line
}

func escapeTOMLStringValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

func contextAwareInterpolate(text string, envGetter func(string) string, missingSet map[string]struct{}) string {
	var sb strings.Builder
	runes := []rune(text)
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(runes); {
		r := runes[i]
		if escaped {
			sb.WriteRune(r)
			escaped = false
			i++
			continue
		}
		if r == '\\' && inDoubleQuote {
			escaped = true
			sb.WriteRune(r)
			i++
			continue
		}
		if r == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			sb.WriteRune(r)
			i++
			continue
		}
		if r == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			sb.WriteRune(r)
			i++
			continue
		}

		// Check for ${VAR...}
		if r == '$' && i+1 < len(runes) && runes[i+1] == '{' {
			end := -1
			for j := i + 2; j < len(runes); j++ {
				if runes[j] == '}' {
					end = j
					break
				}
				if runes[j] == '\n' {
					break
				}
			}
			if end != -1 {
				matchStr := string(runes[i : end+1])
				submatches := envVarPattern.FindStringSubmatch(matchStr)
				if len(submatches) >= 2 {
					varName := submatches[1]
					val := envGetter(varName)
					if val == "" && len(submatches) >= 3 && submatches[2] != "" {
						val = submatches[2]
					}
					if val != "" {
						if inSingleQuote {
							// Inside literal single-quoted strings: preserve raw value without escaping
							sb.WriteString(val)
						} else {
							// Inside double quotes or outside quotes: escape safely
							sb.WriteString(escapeTOMLStringValue(val))
						}
						i = end + 1
						continue
					}
					missingSet[varName] = struct{}{}
					sb.WriteString(matchStr)
					i = end + 1
					continue
				}
			}
		}

		sb.WriteRune(r)
		i++
	}
	return sb.String()
}
