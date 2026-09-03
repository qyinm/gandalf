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
	anthropicKeyRegex = regexp.MustCompile(`sk-ant-api03-[A-Za-z0-9_-]{16,}`)
	openAIKeyRegex    = regexp.MustCompile(`sk-(proj-)?[A-Za-z0-9_-]{16,}`)
	githubTokenRegex  = regexp.MustCompile(`(ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,})`)
	bearerTokenRegex  = regexp.MustCompile(`Bearer\s+([A-Za-z0-9_\-\.]{10,})`)
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

func sanitizeSecretPlaceholder(key, sampleType string) string {
	lowerKey := strings.ToLower(key)
	switch sampleType {
	case "database_url":
		return "postgres://user:password@localhost:5432/dbname"
	case "anthropic":
		return "sk-ant-api03-sample-key"
	case "openai":
		return "sk-sample-key"
	case "github":
		return "ghp_sample_token"
	case "bearer":
		return "sample-auth-token"
	default:
		if strings.Contains(lowerKey, "url") || strings.Contains(lowerKey, "uri") {
			return "https://api.example.com"
		}
		return fmt.Sprintf("your-%s-here", lowerKey)
	}
}

// RedactAndTemplatizeServer inspects an MCPServerDef, redacting hardcoded secrets
// into ${ENV_VAR} references and updating the server's RequiredEnv and safe [env_template] placeholders.
func RedactAndTemplatizeServer(serverName string, srv *manifest.MCPServerDef, envTemplate map[string]string) {
	requiredEnvMap := make(map[string]bool)
	for _, req := range srv.RequiredEnv {
		requiredEnvMap[req] = true
	}

	cleanName := strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(serverName, "_"))

	// 1. Process URL
	if srv.URL != "" {
		srv.URL = redactStringValue(srv.URL, cleanName, envTemplate, requiredEnvMap)
	}

	// 2. Process Command
	if srv.Command != "" {
		srv.Command = redactStringValue(srv.Command, cleanName, envTemplate, requiredEnvMap)
	}

	// 3. Process Args
	for i, arg := range srv.Args {
		srv.Args[i] = redactStringValue(arg, cleanName, envTemplate, requiredEnvMap)
	}

	// 4. Process Headers
	for k, v := range srv.Headers {
		normalized := redactStringValue(v, cleanName, envTemplate, requiredEnvMap)
		lowerK := strings.ToLower(k)
		isAuthHeader := lowerK == "authorization" || strings.Contains(lowerK, "token") || strings.Contains(lowerK, "key") || strings.Contains(lowerK, "secret")
		if isAuthHeader && !strings.HasPrefix(normalized, "${") {
			cleanK := strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(k, "_"))
			varKey := fmt.Sprintf("%s_%s", cleanName, cleanK)
			normalized = fmt.Sprintf("${%s}", varKey)
			requiredEnvMap[varKey] = true
			if _, exists := envTemplate[varKey]; !exists {
				envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "bearer")
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
					// If another server already registered this env key with a conflicting context,
					// use a server-scoped key name.
					if _, exists := envTemplate[varKey]; exists {
						varKey = fmt.Sprintf("%s_%s", cleanName, k)
					}

					srv.Env[k] = fmt.Sprintf("${%s}", varKey)
					requiredEnvMap[varKey] = true
					if _, exists := envTemplate[varKey]; !exists {
						envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "default")
					}
					continue
				}
			}
			srv.Env[k] = normalized
		}
	}

	// 6. Process Auth (string or map)
	if srv.Auth != nil {
		switch a := srv.Auth.(type) {
		case string:
			normalized := NormalizeInterpolation(a)
			for _, e := range ExtractExistingRequiredEnvs(normalized) {
				requiredEnvMap[e] = true
			}
			if !strings.HasPrefix(normalized, "${") {
				varKey := fmt.Sprintf("%s_AUTH_TOKEN", cleanName)
				srv.Auth = fmt.Sprintf("${%s}", varKey)
				requiredEnvMap[varKey] = true
				if _, exists := envTemplate[varKey]; !exists {
					envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "bearer")
				}
			} else {
				srv.Auth = normalized
			}
		case map[string]any:
			redactedMap := make(map[string]any)
			for k, v := range a {
				strVal, isStr := v.(string)
				if isStr {
					normalized := NormalizeInterpolation(strVal)
					for _, e := range ExtractExistingRequiredEnvs(normalized) {
						requiredEnvMap[e] = true
					}
					lowerK := strings.ToLower(k)
					if !strings.HasPrefix(normalized, "${") &&
						(strings.Contains(lowerK, "token") || strings.Contains(lowerK, "secret") || strings.Contains(lowerK, "key") || strings.Contains(lowerK, "password")) {
						varKey := fmt.Sprintf("%s_%s", cleanName, strings.ToUpper(k))
						redactedMap[k] = fmt.Sprintf("${%s}", varKey)
						requiredEnvMap[varKey] = true
						if _, exists := envTemplate[varKey]; !exists {
							envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "bearer")
						}
					} else {
						redactedMap[k] = normalized
					}
				} else {
					redactedMap[k] = v
				}
			}
			srv.Auth = redactedMap
		}
	}

	// Rebuild RequiredEnv list
	var finalRequired []string
	for req := range requiredEnvMap {
		finalRequired = append(finalRequired, req)
	}
	srv.RequiredEnv = finalRequired
}

func redactStringValue(val, cleanName string, envTemplate map[string]string, requiredEnvMap map[string]bool) string {
	normalized := NormalizeInterpolation(val)
	for _, e := range ExtractExistingRequiredEnvs(normalized) {
		requiredEnvMap[e] = true
	}

	// 1. Redact DB URLs iteratively
	for {
		match := dbURLRegex.FindString(normalized)
		if match == "" {
			break
		}
		varKey := "DATABASE_URL"
		if _, exists := envTemplate["DATABASE_URL"]; exists && cleanName != "DATABASE" && cleanName != "POSTGRES" && cleanName != "DB" {
			varKey = fmt.Sprintf("%s_DATABASE_URL", cleanName)
		}
		normalized = strings.Replace(normalized, match, fmt.Sprintf("${%s}", varKey), 1)
		requiredEnvMap[varKey] = true
		if _, exists := envTemplate[varKey]; !exists {
			envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "database_url")
		}
	}

	// 2. Redact Anthropic API Keys iteratively
	for {
		match := anthropicKeyRegex.FindString(normalized)
		if match == "" {
			break
		}
		varKey := "ANTHROPIC_API_KEY"
		if _, exists := envTemplate["ANTHROPIC_API_KEY"]; exists {
			varKey = fmt.Sprintf("%s_ANTHROPIC_API_KEY", cleanName)
		}
		normalized = strings.Replace(normalized, match, fmt.Sprintf("${%s}", varKey), 1)
		requiredEnvMap[varKey] = true
		if _, exists := envTemplate[varKey]; !exists {
			envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "anthropic")
		}
	}

	// 3. Redact OpenAI API Keys iteratively
	for {
		match := openAIKeyRegex.FindString(normalized)
		if match == "" {
			break
		}
		varKey := "OPENAI_API_KEY"
		if _, exists := envTemplate["OPENAI_API_KEY"]; exists {
			varKey = fmt.Sprintf("%s_OPENAI_API_KEY", cleanName)
		}
		normalized = strings.Replace(normalized, match, fmt.Sprintf("${%s}", varKey), 1)
		requiredEnvMap[varKey] = true
		if _, exists := envTemplate[varKey]; !exists {
			envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "openai")
		}
	}

	// 4. Redact GitHub Tokens iteratively
	for {
		match := githubTokenRegex.FindString(normalized)
		if match == "" {
			break
		}
		varKey := "GITHUB_TOKEN"
		if _, exists := envTemplate["GITHUB_TOKEN"]; exists {
			varKey = fmt.Sprintf("%s_GITHUB_TOKEN", cleanName)
		}
		normalized = strings.Replace(normalized, match, fmt.Sprintf("${%s}", varKey), 1)
		requiredEnvMap[varKey] = true
		if _, exists := envTemplate[varKey]; !exists {
			envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "github")
		}
	}

	// 5. Redact Bearer tokens in string
	for {
		match := bearerTokenRegex.FindStringSubmatch(normalized)
		if len(match) <= 1 {
			break
		}
		token := match[1]
		varKey := fmt.Sprintf("%s_AUTH_TOKEN", cleanName)
		normalized = strings.Replace(normalized, token, fmt.Sprintf("${%s}", varKey), 1)
		requiredEnvMap[varKey] = true
		if _, exists := envTemplate[varKey]; !exists {
			envTemplate[varKey] = sanitizeSecretPlaceholder(varKey, "bearer")
		}
	}

	return normalized
}
