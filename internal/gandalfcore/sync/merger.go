package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
)

// MergeClaudeSettingsJSON merges manifest MCP servers into an existing Claude settings.json without removing existing user keys.
func MergeClaudeSettingsJSON(existingJSON string, m *manifest.Manifest) (string, error) {
	var settings map[string]any

	trimmed := strings.TrimSpace(existingJSON)
	if trimmed == "" {
		settings = make(map[string]any)
	} else {
		if err := json.Unmarshal([]byte(trimmed), &settings); err != nil {
			return "", fmt.Errorf("invalid json in claude settings: %w", err)
		}
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	var mcpServers map[string]any
	if raw, ok := settings["mcpServers"]; ok {
		if m, ok := raw.(map[string]any); ok {
			mcpServers = m
		}
	}
	if mcpServers == nil {
		mcpServers = make(map[string]any)
	}

	for name, srv := range m.MCPServers {
		serverEntry := make(map[string]any)
		if srv.Command != "" {
			serverEntry["command"] = srv.Command
		}
		if len(srv.Args) > 0 {
			serverEntry["args"] = srv.Args
		}
		if len(srv.Env) > 0 {
			serverEntry["env"] = srv.Env
		}
		if srv.URL != "" {
			serverEntry["url"] = srv.URL
		}
		if len(srv.Headers) > 0 {
			serverEntry["headers"] = srv.Headers
		}
		if srv.Disabled {
			serverEntry["disabled"] = true
		}
		mcpServers[name] = serverEntry
	}

	settings["mcpServers"] = mcpServers

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(settings); err != nil {
		return "", fmt.Errorf("encode claude settings: %w", err)
	}

	return strings.TrimSpace(buf.String()) + "\n", nil
}

// MergeCursorMCPJSON merges manifest MCP servers into an existing Cursor mcp.json without removing existing user keys.
func MergeCursorMCPJSON(existingJSON string, m *manifest.Manifest) (string, error) {
	var root map[string]any

	trimmed := strings.TrimSpace(existingJSON)
	if trimmed == "" {
		root = make(map[string]any)
	} else {
		if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
			return "", fmt.Errorf("invalid json in cursor mcp.json: %w", err)
		}
	}
	if root == nil {
		root = make(map[string]any)
	}

	var mcpServers map[string]any
	if raw, ok := root["mcpServers"]; ok {
		if m, ok := raw.(map[string]any); ok {
			mcpServers = m
		}
	}
	if mcpServers == nil {
		mcpServers = make(map[string]any)
	}

	for name, srv := range m.MCPServers {
		serverEntry := make(map[string]any)
		if srv.Type != "" {
			serverEntry["type"] = srv.Type
		}
		if srv.Command != "" {
			serverEntry["command"] = srv.Command
		}
		if len(srv.Args) > 0 {
			serverEntry["args"] = srv.Args
		}
		if len(srv.Env) > 0 {
			serverEntry["env"] = srv.Env
		}
		if srv.EnvFile != "" {
			serverEntry["envFile"] = srv.EnvFile
		}
		if srv.URL != "" {
			serverEntry["url"] = srv.URL
		}
		if len(srv.Headers) > 0 {
			serverEntry["headers"] = srv.Headers
		}
		if srv.Disabled {
			serverEntry["disabled"] = true
		}
		mcpServers[name] = serverEntry
	}

	root["mcpServers"] = mcpServers

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(root); err != nil {
		return "", fmt.Errorf("encode cursor mcp.json: %w", err)
	}

	return strings.TrimSpace(buf.String()) + "\n", nil
}

// MergeCodexConfigTOML merges manifest MCP servers and hooks into an existing Codex config.toml without destroying user config.
func MergeCodexConfigTOML(existingTOML string, m *manifest.Manifest) (string, error) {
	lines := strings.Split(existingTOML, "\n")
	var resultLines []string

	// Track existing mcp_servers sections to replace
	inMCPServerSection := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			header := trimmed[1 : len(trimmed)-1]
			parts := strings.Split(header, ".")
			if len(parts) >= 2 && parts[0] == "mcp_servers" {
				srvName := strings.Trim(strings.Join(parts[1:], "."), "\"")
				// If this server is managed by manifest, we will replace it later
				if _, exists := m.MCPServers[srvName]; exists {
					inMCPServerSection = true
					continue
				}
			}
			inMCPServerSection = false
		}

		if inMCPServerSection {
			// Skip old lines for this manifest-managed server
			continue
		}

		resultLines = append(resultLines, line)
	}

	// Clean trailing blank lines
	content := strings.TrimRight(strings.Join(resultLines, "\n"), "\r\n")

	// Append all manifest MCP servers
	var srvNames []string
	for k := range m.MCPServers {
		srvNames = append(srvNames, k)
	}
	sort.Strings(srvNames)

	var tomlAdditions []string
	for _, name := range srvNames {
		srv := m.MCPServers[name]
		var section strings.Builder
		section.WriteString(fmt.Sprintf("\n[mcp_servers.%s]\n", formatTOMLKey(name)))
		if srv.Command != "" {
			section.WriteString(fmt.Sprintf("command = %q\n", srv.Command))
		}
		if len(srv.Args) > 0 {
			var argsFormatted []string
			for _, a := range srv.Args {
				argsFormatted = append(argsFormatted, fmt.Sprintf("%q", a))
			}
			section.WriteString(fmt.Sprintf("args = [%s]\n", strings.Join(argsFormatted, ", ")))
		}
		if srv.URL != "" {
			section.WriteString(fmt.Sprintf("url = %q\n", srv.URL))
		}
		if srv.Disabled {
			section.WriteString("disabled = true\n")
		}
		if len(srv.Env) > 0 {
			var envKeys []string
			for k := range srv.Env {
				envKeys = append(envKeys, k)
			}
			sort.Strings(envKeys)
			for _, k := range envKeys {
				section.WriteString(fmt.Sprintf("env.%s = %q\n", formatTOMLKey(k), srv.Env[k]))
			}
		}
		tomlAdditions = append(tomlAdditions, strings.TrimSpace(section.String()))
	}

	if len(tomlAdditions) > 0 {
		if content != "" {
			content += "\n\n"
		}
		content += strings.Join(tomlAdditions, "\n\n")
	}

	return content + "\n", nil
}

func formatTOMLKey(k string) string {
	if strings.ContainsAny(k, " .") {
		return fmt.Sprintf("%q", k)
	}
	return k
}
