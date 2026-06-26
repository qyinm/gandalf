package readiness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

var readinessCategories = []types.ReadinessCategory{
	types.ReadinessReady,
	types.ReadinessNeedsManualAction,
	types.ReadinessWarning,
	types.ReadinessUnverified,
	types.ReadinessUnsupported,
	types.ReadinessBlocked,
}

var sensitiveQueryPattern = regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)`)

// CurrentPlatform returns the gandalf target platform identifier.
func CurrentPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

// ClassifyMcpBinary classifies an MCP command path.
func ClassifyMcpBinary(command string, homeDir *string) types.McpBinaryKind {
	if command == "npx" || command == "uvx" {
		return types.McpBinaryPackageRunner
	}
	if filepath.IsAbs(command) {
		if homeDir != nil && isStrictlyUnder(command, *homeDir) {
			return types.McpBinarySourceLocalPath
		}
		return types.McpBinaryPathBinary
	}
	return types.McpBinaryCommand
}

// ExtractMcpBinaries collects MCP binary references from evidence.
func ExtractMcpBinaries(evidence []types.DiscoveredItem, sourceHomeDir *string) []types.McpBinaryInfo {
	var binaries []types.McpBinaryInfo
	for _, item := range evidence {
		if item.Kind != types.KindMcpServer {
			continue
		}
		value, _ := scanRecord(item.Value)
		if value == nil {
			continue
		}
		command, _ := value["command"].(string)
		url, _ := value["url"].(string)
		if command == "" && url == "" {
			continue
		}
		var args []string
		if rawArgs, ok := value["args"].([]any); ok {
			for _, arg := range rawArgs {
				if s, ok := arg.(string); ok {
					args = append(args, s)
				}
			}
		}
		var safeURL *string
		if url != "" {
			sanitized := sanitizeRemoteURL(url)
			safeURL = &sanitized
		}
		commandText := command
		if commandText == "" && safeURL != nil {
			commandText = *safeURL
		}
		if commandText == "" {
			commandText = "unknown"
		}
		var kind types.McpBinaryKind
		if url != "" {
			kind = types.McpBinaryRemoteURL
		} else {
			kind = ClassifyMcpBinary(command, sourceHomeDir)
		}
		binaries = append(binaries, types.McpBinaryInfo{
			EvidenceID: item.ID,
			Command:    commandText,
			Args:       args,
			URL:        safeURL,
			BinaryKind: &kind,
		})
	}
	return binaries
}

// CheckMcpBinaryAvailability reports whether MCP commands exist on PATH.
func CheckMcpBinaryAvailability(sourceBinaries []types.McpBinaryInfo) []types.McpBinaryReport {
	var reports []types.McpBinaryReport
	for _, bin := range sourceBinaries {
		if bin.URL != nil {
			warning := "Remote URL — availability cannot be verified locally"
			kind := types.McpBinaryRemoteURL
			reports = append(reports, types.McpBinaryReport{
				EvidenceID:        bin.EvidenceID,
				Command:           *bin.URL,
				AvailableOnTarget: true,
				BinaryKind:        &kind,
				Warning:           &warning,
			})
			continue
		}
		if bin.BinaryKind != nil && *bin.BinaryKind == types.McpBinarySourceLocalPath {
			warning := "MCP command points to a source machine local binary path (" + bin.Command + "); install or remap it on this machine."
			reports = append(reports, types.McpBinaryReport{
				EvidenceID:        bin.EvidenceID,
				Command:           bin.Command,
				AvailableOnTarget: false,
				BinaryKind:        bin.BinaryKind,
				Warning:           &warning,
			})
			continue
		}
		resolved := findExecutableOnPath(bin.Command, nil)
		available := resolved != ""
		var warning *string
		if bin.BinaryKind != nil && *bin.BinaryKind == types.McpBinaryPackageRunner {
			if available {
				msg := "Package runner " + bin.Command + " is available at " + resolved + "; package arguments may still differ on this machine."
				warning = &msg
			} else {
				msg := "Package runner " + bin.Command + " not found on this machine; MCP package cannot be launched."
				warning = &msg
			}
		} else if !available {
			msg := `Binary "` + bin.Command + `" not found on this machine`
			warning = &msg
		}
		var resolvedPath *string
		if resolved != "" {
			resolvedPath = &resolved
		}
		reports = append(reports, types.McpBinaryReport{
			EvidenceID:        bin.EvidenceID,
			Command:           bin.Command,
			AvailableOnTarget: available,
			BinaryKind:        bin.BinaryKind,
			ResolvedPath:      resolvedPath,
			Warning:           warning,
		})
	}
	return reports
}

// BuildReadinessReport analyzes portability readiness for evidence on this machine.
func BuildReadinessReport(sourceEvidence []types.DiscoveredItem, options *types.ReadinessOptions) types.ReadinessReport {
	targetPlatform := CurrentPlatform()
	if options.TargetPlatform != nil {
		targetPlatform = *options.TargetPlatform
	}
	sourceBinaries := ExtractMcpBinaries(sourceEvidence, options.SourceHomeDir)
	mcpReports := CheckMcpBinaryAvailability(sourceBinaries)
	var items []types.ReadinessItem

	if targetPlatform != "darwin" {
		category := types.ReadinessUnsupported
		severity := types.SeverityMedium
		fix := "Dry-run and inspect remain available here; apply the bundle on macOS."
		if options.ApplyContent {
			category = types.ReadinessBlocked
			severity = types.SeverityHigh
			fix = "Run dry-run or inspect here, or apply the bundle on macOS."
		}
		items = append(items, types.ReadinessItem{
			ID:       "platform.apply-content-macos-only",
			Category: category,
			Severity: severity,
			Code:     "GANDALF_MACOS_APPLY_ONLY",
			Problem:  "Bundle content apply is Mac-only in this release.",
			Cause:    "Target platform is " + targetPlatform + ".",
			Fix:      fix,
		})
	}

	for i := range mcpReports {
		items = append(items, readinessItemForMcpReport(&mcpReports[i]))
	}

	targetEnvKeys := envKeySet(options.TargetEvidence, false)
	sourceEnvKeys := envKeySet(sourceEvidence, true)
	sortedKeys := make([]string, 0, len(sourceEnvKeys))
	for key := range sourceEnvKeys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	for _, key := range sortedKeys {
		if targetEnvKeys[key] {
			continue
		}
		if options.ProcessEnv != nil {
			if _, ok := options.ProcessEnv[key]; ok {
				continue
			}
		}
		path := ".env"
		items = append(items, types.ReadinessItem{
			ID:       "env." + key,
			Category: types.ReadinessNeedsManualAction,
			Severity: types.SeverityMedium,
			Code:     "GANDALF_ENV_VALUE_REQUIRED",
			Problem:  "Environment key " + key + " needs a value on this machine.",
			Cause:    "The bundle records the key name only; raw env values are omitted by policy.",
			Fix:      "Add the value manually or through your preferred secret manager before running tools that need it.",
			Path:     &path,
			Actions: []types.ReadinessAction{{
				Label: "Set env value manually",
			}},
		})
	}

	return types.ReadinessReport{
		TargetPlatform: targetPlatform,
		Summary:        summarize(items),
		Items:          items,
	}
}

// ReadinessItemForMcpReport maps an MCP binary report to a readiness item.
func ReadinessItemForMcpReport(report *types.McpBinaryReport) types.ReadinessItem {
	return readinessItemForMcpReport(report)
}

func readinessItemForMcpReport(report *types.McpBinaryReport) types.ReadinessItem {
	if report.BinaryKind != nil && *report.BinaryKind == types.McpBinaryRemoteURL {
		cause := "Remote MCP availability depends on network and provider state."
		if report.Warning != nil {
			cause = *report.Warning
		}
		return types.ReadinessItem{
			ID:         "mcp." + report.EvidenceID + ".remote",
			Category:   types.ReadinessUnverified,
			Severity:   types.SeverityLow,
			Code:       "GANDALF_REMOTE_MCP_UNVERIFIED",
			Problem:    "Remote MCP URL cannot be verified locally.",
			Cause:      cause,
			Fix:        "Verify the remote endpoint outside gandalf if this MCP server is required.",
			EvidenceID: &report.EvidenceID,
			Command:    &report.Command,
		}
	}
	if report.AvailableOnTarget {
		cause := "The command is available on PATH."
		if report.ResolvedPath != nil {
			cause = "Resolved to " + *report.ResolvedPath + "."
		}
		return types.ReadinessItem{
			ID:         "mcp." + report.EvidenceID + ".available",
			Category:   types.ReadinessReady,
			Severity:   types.SeverityNone,
			Code:       "GANDALF_MCP_COMMAND_AVAILABLE",
			Problem:    "MCP command " + report.Command + " is available.",
			Cause:      cause,
			Fix:        "No action needed.",
			EvidenceID: &report.EvidenceID,
			Command:    &report.Command,
		}
	}
	if report.BinaryKind != nil && *report.BinaryKind == types.McpBinarySourceLocalPath {
		cause := "The source command path is " + report.Command + "."
		if report.Warning != nil {
			cause = *report.Warning
		}
		return types.ReadinessItem{
			ID:         "mcp." + report.EvidenceID + ".source-local-path",
			Category:   types.ReadinessNeedsManualAction,
			Severity:   types.SeverityMedium,
			Code:       "GANDALF_SOURCE_LOCAL_MCP_PATH",
			Problem:    "MCP command points to a source-machine local path.",
			Cause:      cause,
			Fix:        "Install the MCP server on this Mac and update the command path if needed.",
			EvidenceID: &report.EvidenceID,
			Command:    &report.Command,
			Actions: []types.ReadinessAction{{
				Label: "Install or remap local MCP binary",
			}},
		}
	}
	cause := `The command ` + report.Command + ` was not found on PATH.`
	if report.Warning != nil {
		cause = *report.Warning
	}
	return types.ReadinessItem{
		ID:         "mcp." + report.EvidenceID + ".missing",
		Category:   types.ReadinessNeedsManualAction,
		Severity:   types.SeverityMedium,
		Code:       "GANDALF_MCP_COMMAND_MISSING",
		Problem:    "MCP command " + report.Command + " is missing on this machine.",
		Cause:      cause,
		Fix:        installHintForCommand(report.Command, report.BinaryKind),
		EvidenceID: &report.EvidenceID,
		Command:    &report.Command,
		Actions:    installActionsForCommand(report.Command, report.BinaryKind),
	}
}

// FormatReadinessSummaryLines renders human-readable readiness summary lines.
func FormatReadinessSummaryLines(report *types.ReadinessReport, options *types.ReadinessFormatOptions) []string {
	maxItems := 5
	if options != nil && options.MaxItems > 0 {
		maxItems = options.MaxItems
	}
	includeFixes := options == nil || options.IncludeFixes
	includeActions := options == nil || options.IncludeActions

	lines := []string{
		"Readiness:",
		"  ready: " + countSummary(report.Summary, types.ReadinessReady),
		"  needs manual action: " + countSummary(report.Summary, types.ReadinessNeedsManualAction),
		"  warnings: " + countSummary(report.Summary, types.ReadinessWarning),
		"  unverified: " + countSummary(report.Summary, types.ReadinessUnverified),
		"  unsupported: " + countSummary(report.Summary, types.ReadinessUnsupported),
		"  blocked: " + countSummary(report.Summary, types.ReadinessBlocked),
	}

	var actionable []types.ReadinessItem
	for _, item := range report.Items {
		if item.Category == types.ReadinessBlocked || item.Category == types.ReadinessNeedsManualAction {
			actionable = append(actionable, item)
		}
	}
	for i, item := range actionable {
		if i >= maxItems {
			break
		}
		lines = append(lines, "  - "+item.Problem)
		if includeFixes {
			lines = append(lines, "    fix: "+item.Fix)
		}
		if includeActions {
			for _, action := range item.Actions {
				label := action.Label
				if action.Command != nil {
					label = *action.Command
				}
				lines = append(lines, "    action: "+label)
			}
		}
	}
	if len(actionable) > maxItems {
		lines = append(lines, "  ... and "+itoa(len(actionable)-maxItems)+" more action item(s)")
	}
	return lines
}

func countSummary(summary map[types.ReadinessCategory]uint32, category types.ReadinessCategory) string {
	return itoa(int(summary[category]))
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

func envKeySet(evidence []types.DiscoveredItem, includeMcpEnvKeys bool) map[string]bool {
	keys := make(map[string]bool)
	for _, item := range evidence {
		if item.Kind == types.KindEnvKey {
			key := ""
			if item.Name != nil {
				key = *item.Name
			} else if value, ok := scanRecord(item.Value); ok {
				if k, ok := value["key"].(string); ok {
					key = k
				}
			}
			if key != "" {
				keys[key] = true
			}
		}
		if includeMcpEnvKeys && item.Kind == types.KindMcpServer {
			if value, ok := scanRecord(item.Value); ok {
				if envKeys, ok := value["envKeys"].([]any); ok {
					for _, envKey := range envKeys {
						if k, ok := envKey.(string); ok {
							keys[k] = true
						}
					}
				}
			}
		}
	}
	return keys
}

func scanRecord(raw json.RawMessage) (map[string]any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	return value, true
}

func summarize(items []types.ReadinessItem) map[types.ReadinessCategory]uint32 {
	summary := make(map[types.ReadinessCategory]uint32, len(readinessCategories))
	for _, category := range readinessCategories {
		summary[category] = 0
	}
	for _, item := range items {
		summary[item.Category]++
	}
	return summary
}

func installHintForCommand(command string, kind *types.McpBinaryKind) string {
	switch command {
	case "npx":
		return "Install Node.js on this Mac, then rerun the dry-run."
	case "uvx":
		return "Install uv on this Mac, then rerun the dry-run."
	case "gh":
		return "Install GitHub CLI on this Mac and authenticate it if the MCP server needs GitHub access."
	}
	if kind != nil && *kind == types.McpBinaryPackageRunner {
		return "Install package runner " + command + " on this Mac, then rerun the dry-run."
	}
	return "Install " + command + " on this Mac or update the MCP command to a local path that exists."
}

func installActionsForCommand(command string, kind *types.McpBinaryKind) []types.ReadinessAction {
	switch command {
	case "npx":
		cmd := "brew install node"
		return []types.ReadinessAction{{Label: "Install Node.js", Command: &cmd}}
	case "uvx":
		cmd := "brew install uv"
		return []types.ReadinessAction{{Label: "Install uv", Command: &cmd}}
	case "gh":
		cmd := "brew install gh"
		return []types.ReadinessAction{{Label: "Install GitHub CLI", Command: &cmd}}
	}
	if kind != nil && *kind == types.McpBinaryPackageRunner {
		return []types.ReadinessAction{{Label: "Install " + command}}
	}
	return []types.ReadinessAction{{Label: "Install " + command}}
}

func findExecutableOnPath(command string, pathEnv *string) string {
	pathValue := os.Getenv("PATH")
	if pathEnv != nil {
		pathValue = *pathEnv
	}
	if filepath.IsAbs(command) {
		if executablePath(command) {
			return command
		}
		return ""
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, command)
		if executablePath(candidate) {
			return candidate
		}
	}
	return ""
}

func executablePath(candidate string) bool {
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func isStrictlyUnder(resolved, root string) bool {
	if resolved == root {
		return true
	}
	return strings.HasPrefix(resolved, root+string(os.PathSeparator))
}

func sanitizeRemoteURL(rawURL string) string {
	parsed, ok := parseURL(rawURL)
	if !ok {
		return "[remote-url]"
	}
	parsed.username = ""
	parsed.password = ""
	parsed.fragment = nil
	for key := range parsed.query {
		if sensitiveQueryPattern.MatchString(key) {
			parsed.query[key] = "[redacted]"
		}
	}
	return parsed.String()
}

type parsedURL struct {
	scheme   string
	username string
	password string
	host     string
	path     string
	query    map[string]string
	fragment *string
}

func (p parsedURL) String() string {
	var b strings.Builder
	b.WriteString(p.scheme)
	b.WriteString("://")
	if p.username != "" || p.password != "" {
		b.WriteString(p.username)
		if p.password != "" {
			b.WriteByte(':')
			b.WriteString(p.password)
		}
		b.WriteByte('@')
	}
	b.WriteString(p.host)
	b.WriteString(p.path)
	if len(p.query) > 0 {
		b.WriteByte('?')
		keys := make([]string, 0, len(p.query))
		for key := range p.query {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i > 0 {
				b.WriteByte('&')
			}
			b.WriteString(key)
			b.WriteByte('=')
			b.WriteString(p.query[key])
		}
	}
	if p.fragment != nil {
		b.WriteByte('#')
		b.WriteString(*p.fragment)
	}
	return b.String()
}

func parseURL(raw string) (parsedURL, bool) {
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return parsedURL{}, false
	}
	fragment := (*string)(nil)
	if before, frag, ok := strings.Cut(rest, "#"); ok {
		fragment = &frag
		rest = before
	}
	query := map[string]string{}
	if before, q, ok := strings.Cut(rest, "?"); ok {
		for _, pair := range strings.Split(q, "&") {
			if pair == "" {
				continue
			}
			key, value, _ := strings.Cut(pair, "=")
			query[key] = value
		}
		rest = before
	}
	authority, path, _ := strings.Cut(rest, "/")
	if path != "" {
		path = "/" + path
	}
	username := ""
	password := ""
	host := authority
	if creds, h, ok := strings.Cut(authority, "@"); ok {
		host = h
		username, password, _ = strings.Cut(creds, ":")
	}
	return parsedURL{
		scheme:   scheme,
		username: username,
		password: password,
		host:     host,
		path:     path,
		query:    query,
		fragment: fragment,
	}, true
}
