package importer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
)

var (
	// Cursor environment interpolation pattern: ${env:VAR_NAME}
	cursorEnvRegex = regexp.MustCompile(`\$\{env:([A-Za-z0-9_]+)\}`)

	// Database URL patterns
	dbURLRegex = regexp.MustCompile(`(postgres|postgresql|mysql|mongodb|mongodb\+srv|redis|rediss)://[^:\s]+:[^@\s]+@[^\s"']+`)

	// Sensitive token patterns
	anthropicKeyRegex = regexp.MustCompile(`sk-ant-api03-[A-Za-z0-9_-]{80,}`)
	openAIKeyRegex    = regexp.MustCompile(`sk-(proj-)?[A-Za-z0-9_-]{32,}`)
	githubTokenRegex  = regexp.MustCompile(`(ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{82})`)
	bearerTokenRegex  = regexp.MustCompile(`Bearer\s+([A-Za-z0-9_\-\.]{20,})`)
)

// NormalizeInterpolation converts vendor-specific env interpolation (like Cursor's ${env:VAR})
// to Gandalf's standard ${VAR} format.
func NormalizeInterpolation(s string) string {
	return cursorEnvRegex.ReplaceAllString(s, `${$1}`)
}

// ExtractExistingRequiredEnvs inspects strings for ${VAR} syntax and collects variable names.
func ExtractExistingRequiredEnvs(s string) []string {
	var envs []string
	re := regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::-([^}]+))?\}`)
	matches := re.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		if len(m) > 1 && m[1] != "" {
			envs = append(envs, m[1])
		}
	}
	return envs
}

// RedactAndTemplatizeServer inspects an MCPServerDef, redacting hardcoded secrets
// into ${ENV_VAR} references and updating the server's RequiredEnv and the global envTemplate.
func RedactAndTemplatizeServer(serverName string, srv *manifest.MCPServerDef, envTemplate map[string]string) {
	requiredEnvMap := make(map[string]bool)
	for _, req := range srv.RequiredEnv {
		requiredEnvMap[req] = true
	}

	cleanName := strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(serverName, "_"))

	// 1. Process URL
	if srv.URL != "" {
		srv.URL = NormalizeInterpolation(srv.URL)
		for _, e := range ExtractExistingRequiredEnvs(srv.URL) {
			requiredEnvMap[e] = true
		}

		if match := dbURLRegex.FindString(srv.URL); match != "" {
			varKey := "DATABASE_URL"
			if existing, exists := envTemplate["DATABASE_URL"]; exists && existing != match {
				varKey = fmt.Sprintf("%s_DATABASE_URL", cleanName)
			}
			srv.URL = strings.Replace(srv.URL, match, fmt.Sprintf("${%s}", varKey), 1)
			requiredEnvMap[varKey] = true
			if _, exists := envTemplate[varKey]; !exists {
				envTemplate[varKey] = match
			}
		}
	}

	// 2. Process Command
	srv.Command = NormalizeInterpolation(srv.Command)
	for _, e := range ExtractExistingRequiredEnvs(srv.Command) {
		requiredEnvMap[e] = true
	}

	// 3. Process Args
	for i, arg := range srv.Args {
		normalized := NormalizeInterpolation(arg)
		for _, e := range ExtractExistingRequiredEnvs(normalized) {
			requiredEnvMap[e] = true
		}

		// Detect DB URL in args (e.g. pg://user:pass@host/db)
		if match := dbURLRegex.FindString(normalized); match != "" {
			varKey := "DATABASE_URL"
			if existing, exists := envTemplate["DATABASE_URL"]; exists && existing != match {
				varKey = fmt.Sprintf("%s_DATABASE_URL", cleanName)
			}
			normalized = strings.Replace(normalized, match, fmt.Sprintf("${%s}", varKey), 1)
			requiredEnvMap[varKey] = true
			if _, exists := envTemplate[varKey]; !exists {
				envTemplate[varKey] = match
			}
		}

		// Detect API keys in args
		if match := anthropicKeyRegex.FindString(normalized); match != "" {
			varKey := "ANTHROPIC_API_KEY"
			normalized = strings.Replace(normalized, match, fmt.Sprintf("${%s}", varKey), 1)
			requiredEnvMap[varKey] = true
			if _, exists := envTemplate[varKey]; !exists {
				envTemplate[varKey] = match
			}
		} else if match := openAIKeyRegex.FindString(normalized); match != "" {
			varKey := "OPENAI_API_KEY"
			normalized = strings.Replace(normalized, match, fmt.Sprintf("${%s}", varKey), 1)
			requiredEnvMap[varKey] = true
			if _, exists := envTemplate[varKey]; !exists {
				envTemplate[varKey] = match
			}
		} else if match := githubTokenRegex.FindString(normalized); match != "" {
			varKey := "GITHUB_TOKEN"
			normalized = strings.Replace(normalized, match, fmt.Sprintf("${%s}", varKey), 1)
			requiredEnvMap[varKey] = true
			if _, exists := envTemplate[varKey]; !exists {
				envTemplate[varKey] = match
			}
		}

		srv.Args[i] = normalized
	}

	// 4. Process Headers
	for k, v := range srv.Headers {
		normalized := NormalizeInterpolation(v)
		for _, e := range ExtractExistingRequiredEnvs(normalized) {
			requiredEnvMap[e] = true
		}

		if match := bearerTokenRegex.FindStringSubmatch(normalized); len(match) > 1 {
			token := match[1]
			varKey := fmt.Sprintf("%s_AUTH_TOKEN", cleanName)
			normalized = strings.Replace(normalized, token, fmt.Sprintf("${%s}", varKey), 1)
			requiredEnvMap[varKey] = true
			if _, exists := envTemplate[varKey]; !exists {
				envTemplate[varKey] = token
			}
		}
		srv.Headers[k] = normalized
	}

	// 5. Process Env map
	if srv.Env != nil {
		for k, v := range srv.Env {
			normalized := NormalizeInterpolation(v)
			for _, e := range ExtractExistingRequiredEnvs(normalized) {
				requiredEnvMap[e] = true
			}

			// If the env value is a raw secret key and not already a ${VAR}
			if !strings.HasPrefix(normalized, "${") {
				isSecret := anthropicKeyRegex.MatchString(normalized) ||
					openAIKeyRegex.MatchString(normalized) ||
					githubTokenRegex.MatchString(normalized) ||
					dbURLRegex.MatchString(normalized) ||
					strings.HasSuffix(strings.ToUpper(k), "_KEY") ||
					strings.HasSuffix(strings.ToUpper(k), "_TOKEN") ||
					strings.HasSuffix(strings.ToUpper(k), "_SECRET") ||
					strings.HasSuffix(strings.ToUpper(k), "_PASSWORD")

				if isSecret {
					varKey := k
					srv.Env[k] = fmt.Sprintf("${%s}", varKey)
					requiredEnvMap[varKey] = true
					if _, exists := envTemplate[varKey]; !exists {
						envTemplate[varKey] = normalized
					}
					continue
				}
			}
			srv.Env[k] = normalized
		}
	}

	// Rebuild RequiredEnv list
	var finalRequired []string
	for req := range requiredEnvMap {
		finalRequired = append(finalRequired, req)
	}
	srv.RequiredEnv = finalRequired
}
