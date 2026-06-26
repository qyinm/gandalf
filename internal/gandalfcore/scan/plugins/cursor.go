package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/parsers"
	"github.com/qyinm/gandalf/internal/gandalfcore/policy"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

var (
	cursorInterpolationPattern = regexp.MustCompile(`\$\{(?:env:[^}]+|userHome|workspaceFolder|workspaceFolderBasename|pathSeparator|/)\}`)
	cursorURLAuthPattern       = regexp.MustCompile(`//([^/@:]+)(?::[^/@]*)?@`)
	cursorURLQueryPattern      = regexp.MustCompile(`([?&][^=]+)=([^&]+)`)
	cursorScopeRootPattern     = regexp.MustCompile(`^(.*?)(?:^|/)(?:\.cursor|\.agents)/skills$`)
)

// CursorScanner discovers Cursor editor configuration.
type CursorScanner struct{}

func (CursorScanner) AgentID() types.AgentID { return types.AgentCursor }
func (CursorScanner) AgentName() string      { return "Cursor" }
func (CursorScanner) Description() string {
	return "Cursor editor configuration (MCP servers, skills, hooks)"
}
func (CursorScanner) Targets(projectPath, homeDir string) []scan.ScanTarget {
	return cursorMCPTargets(projectPath, homeDir)
}

func (c CursorScanner) Scan(context *scan.ScannerContext) []types.DiscoveredItem {
	base := scan.NewScannerBase(types.AgentCursor)
	mcpEvidence := scanCursorMCPServers(context.ProjectPath, context.HomeDir, base)
	hookEvidence := scanCursorHooks(context.ProjectPath, context.HomeDir, base)
	var skillEvidence []types.DiscoveredItem
	for _, target := range cursorSkillTargets(context.ProjectPath, context.HomeDir) {
		skillEvidence = append(skillEvidence, scanCursorSkillDirectory(target)...)
	}
	if len(mcpEvidence) == 0 && len(skillEvidence) == 0 && len(hookEvidence) == 0 {
		return nil
	}
	var evidence []types.DiscoveredItem
	evidence = append(evidence, mcpEvidence...)
	evidence = append(evidence, dedupeSkillsByName(skillEvidence)...)
	evidence = append(evidence, hookEvidence...)
	evidence = append(evidence, cursorTeamHooksBlindSpot())
	return evidence
}

func cursorMCPTargets(projectPath, homeDir string) []scan.ScanTarget {
	overrides := scan.ScanTargetOverrides{
		Sensitivity:   stringPtr("command_config"),
		ContentPolicy: stringPtr("structured_safe_fields_only"),
	}
	return []scan.ScanTarget{
		scan.ProjectTarget(projectPath, ".cursor/mcp.json", types.AgentCursor, types.KindAgentConfig, types.ParserJSON, overrides),
		scan.HomeTarget(homeDir, ".cursor/mcp.json", types.AgentCursor, types.KindAgentConfig, types.ParserJSON, overrides),
	}
}

func scanCursorMCPServers(projectPath, homeDir string, base scan.ScannerBase) []types.DiscoveredItem {
	var evidence []types.DiscoveredItem
	for _, target := range cursorMCPTargets(projectPath, homeDir) {
		evidence = append(evidence, scanCursorMCPFile(target, base)...)
	}
	return evidence
}

func scanCursorMCPFile(target scan.ScanTarget, base scan.ScannerBase) []types.DiscoveredItem {
	text, err := os.ReadFile(target.AbsolutePath)
	if err != nil {
		return nil
	}
	parsed := parsers.ParseJSON(string(text))
	if parsed.Ok == nil {
		errText := "invalid JSON"
		if parsed.Err != nil {
			errText = parsed.Err.Error
		}
		item := base.ParseFailed(scan.EvidenceBaseTargetFromScanTarget(target), types.KindAgentConfig, errText)
		return []types.DiscoveredItem{item}
	}
	root, ok := scan.AsRecord(parsed.Ok.Value)
	if !ok {
		return nil
	}
	servers, _ := scan.AsRecord(root["mcpServers"])
	if servers == nil {
		item := base.Captured(scan.EvidenceBaseTargetFromScanTarget(target), types.KindAgentConfig, nil, root)
		return []types.DiscoveredItem{item}
	}
	var evidence []types.DiscoveredItem
	for name, serverValue := range servers {
		serverRecord, ok := scan.AsRecord(serverValue)
		if !ok {
			continue
		}
		sanitized := sanitizeMCPServer(serverRecord)
		transport := transportForMCPServer(sanitized)
		remote := transport != "stdio"
		if url, ok := sanitized["url"].(string); ok && url != "" {
			remote = true
		}
		metadata := map[string]any{
			"transport":           transport,
			"remote":              remote,
			"source":              target.Scope.String(),
			"authConfigured":      sanitized["auth"] != nil,
			"interpolationFields": interpolationFieldsForMCPServer(sanitized),
		}
		if envFile, ok := sanitized["envFile"]; ok {
			metadata["envFile"] = envFile
		}
		evidenceTarget := scan.EvidenceBaseTargetFromScanTarget(target)
		evidenceTarget.Sensitivity = "command_config"
		evidenceTarget.ContentPolicy = "structured_safe_fields_only"
		item := base.Captured(evidenceTarget, types.KindMcpServer, metadata, sanitized)
		item.ID = base.ItemID(scan.ItemIDTarget{Agent: target.Agent, SourcePath: target.SourcePath, Scope: target.Scope}, "mcp-"+name)
		item.Name = stringPtr(name)
		evidence = append(evidence, item)
	}
	return evidence
}

func sanitizeMCPServer(value map[string]any) map[string]any {
	sanitized := make(map[string]any, len(value))
	for key, nestedValue := range value {
		if key == "url" {
			if url, ok := nestedValue.(string); ok {
				sanitized[key] = redactURL(url)
				continue
			}
		}
		sanitized[key] = nestedValue
	}
	return sanitized
}

func transportForMCPServer(value map[string]any) string {
	typeValue := ""
	if raw, ok := value["type"].(string); ok {
		typeValue = strings.ToLower(raw)
	}
	switch typeValue {
	case "stdio":
		return "stdio"
	case "sse":
		return "sse"
	case "streamable-http", "streamable_http", "http":
		return "streamable-http"
	}
	if _, ok := value["command"]; ok {
		return "stdio"
	}
	if _, ok := value["url"]; ok {
		return "streamable-http"
	}
	return "unknown"
}

func interpolationFieldsForMCPServer(value map[string]any) []string {
	fields := []string{"command", "args", "env", "url", "headers"}
	var result []string
	for _, field := range fields {
		if nested, ok := value[field]; ok && containsInterpolation(nested) {
			result = append(result, field)
		}
	}
	return result
}

func containsInterpolation(value any) bool {
	switch v := value.(type) {
	case string:
		return cursorInterpolationPattern.MatchString(v)
	case []any:
		for _, item := range v {
			if containsInterpolation(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if containsInterpolation(item) {
				return true
			}
		}
	}
	return false
}

func redactURL(value string) string {
	withoutAuth := cursorURLAuthPattern.ReplaceAllString(value, "//[redacted]:[redacted]@")
	return cursorURLQueryPattern.ReplaceAllString(withoutAuth, "$1=[redacted]")
}

func cursorSkillTargets(projectPath, homeDir string) []scan.ScanTarget {
	dir := true
	explicit := []scan.ScanTarget{
		scan.ProjectTarget(projectPath, ".cursor/skills", types.AgentCursor, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.ProjectTarget(projectPath, ".agents/skills", types.AgentCursor, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.ProjectTarget(projectPath, ".claude/skills", types.AgentCursor, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.ProjectTarget(projectPath, ".codex/skills", types.AgentCursor, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".cursor/skills", types.AgentCursor, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".agents/skills", types.AgentCursor, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".claude/skills", types.AgentCursor, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".codex/skills", types.AgentCursor, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
	}
	targets := make(map[string]scan.ScanTarget)
	for _, target := range append(explicit, nestedCursorSkillTargets(projectPath)...) {
		targets[target.SourcePath] = target
	}
	result := make([]scan.ScanTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, target)
	}
	return result
}

func nestedCursorSkillTargets(projectPath string) []scan.ScanTarget {
	var roots []scan.ScanTarget
	walkNestedSkillRoots(projectPath, projectPath, &roots, 0)
	return roots
}

func walkNestedSkillRoots(projectPath, absoluteDir string, targets *[]scan.ScanTarget, depth int) {
	if depth > maxSkillDepth {
		return
	}
	entries, err := os.ReadDir(absoluteDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || policy.IgnoredDirectory(entry.Name()) {
			continue
		}
		absolutePath := filepath.Join(absoluteDir, entry.Name())
		info, err := os.Lstat(absolutePath)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if entry.Name() == ".cursor" || entry.Name() == ".agents" {
			skillsPath := filepath.Join(absolutePath, "skills")
			if skillInfo, err := os.Stat(skillsPath); err == nil && skillInfo.IsDir() {
				*targets = append(*targets, scan.ScanTarget{
					AbsolutePath:  skillsPath,
					SourcePath:    scan.NormalizeSourcePath(projectPath, skillsPath),
					Scope:         types.ScopeProject,
					Precedence:    40,
					Agent:         types.AgentCursor,
					Kind:          types.KindSkill,
					Parser:        types.ParserFilesystem,
					Sensitivity:   "metadata",
					ContentPolicy: "metadata_only",
					Directory:     true,
				})
			}
		}
		walkNestedSkillRoots(projectPath, absolutePath, targets, depth+1)
	}
}

func scanCursorSkillDirectory(target scan.ScanTarget) []types.DiscoveredItem {
	var evidence []types.DiscoveredItem
	scopeRoot := scopeRootForSkillTarget(target)
	for _, skillFile := range findSkillFiles(target.AbsolutePath).files {
		frontmatter := readCursorSkillFrontmatter(skillFile)
		if frontmatter == nil || frontmatter.Name == nil || frontmatter.Description == nil {
			continue
		}
		name := *frontmatter.Name
		description := *frontmatter.Description
		skillDir := filepath.Dir(skillFile)
		directoryName := filepath.Base(skillDir)
		if !validAgentSkillName(name) || name != directoryName {
			continue
		}
		relSkillDir, _ := filepath.Rel(target.AbsolutePath, skillDir)
		sourcePath := target.SourcePath
		if rel := filepath.ToSlash(relSkillDir); rel != "" && rel != "." {
			sourcePath = target.SourcePath + "/" + rel
		}
		metadata := map[string]any{
			"present":              true,
			"entrypoint":           filepath.Base(skillFile),
			"entrypointStatus":     "captured",
			"entrypointSizeBytes":  frontmatter.SizeBytes,
			"declaredName":         name,
			"directoryName":        directoryName,
			"nameMatchesDirectory": true,
			"description":          description,
			"sourceRoot":           target.SourcePath,
		}
		if scopeRoot != nil {
			metadata["scopeRoot"] = *scopeRoot
		}
		if len(frontmatter.Paths) > 0 {
			metadata["paths"] = frontmatter.Paths
		}
		if frontmatter.DisableModelInvocation != nil {
			metadata["disableModelInvocation"] = *frontmatter.DisableModelInvocation
		}
		if frontmatter.SkillMetadata != nil {
			metadata["skillMetadata"] = frontmatter.SkillMetadata
		}
		rawMetadata, _ := json.Marshal(metadata)
		evidence = append(evidence, types.DiscoveredItem{
			ID:            scan.ScannerItemID(target.Scope, types.AgentCursor, sourcePath, "skill"),
			Agent:         types.AgentCursor,
			Kind:          types.KindSkill,
			SourcePath:    sourcePath,
			Scope:         target.Scope,
			Precedence:    target.Precedence,
			Parser:        types.ParserFilesystem,
			Sensitivity:   target.Sensitivity,
			ContentPolicy: target.ContentPolicy,
			RestorePolicy: types.RestoreFullContent,
			CaptureStatus: types.CaptureCaptured,
			Confidence:    types.ConfidenceHigh,
			Name:          &name,
			Metadata:      rawMetadata,
		})
	}
	return evidence
}

type cursorSkillFrontmatter struct {
	Name                   *string
	Description            *string
	Paths                  []string
	DisableModelInvocation *bool
	SkillMetadata          map[string]any
	SizeBytes              uint64
}

func readCursorSkillFrontmatter(filePath string) *cursorSkillFrontmatter {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil
	}
	text, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	matches := skillFrontmatterPattern.FindStringSubmatch(string(text))
	frontmatter := &cursorSkillFrontmatter{SizeBytes: uint64(info.Size())}
	if len(matches) < 2 {
		return frontmatter
	}
	lines := strings.Split(matches[1], "\n")
	scalarRe := regexp.MustCompile(`^(name|description|disable-model-invocation):\s*(.*)$`)
	metadataRe := regexp.MustCompile(`^\s+([A-Za-z0-9_.-]+):\s*(.*)$`)
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if caps := scalarRe.FindStringSubmatch(line); len(caps) == 3 {
			value := scan.UnquoteYAMLScalar(caps[2])
			switch caps[1] {
			case "name":
				frontmatter.Name = &value
			case "description":
				frontmatter.Description = &value
			case "disable-model-invocation":
				disable := value == "true"
				frontmatter.DisableModelInvocation = &disable
			}
			continue
		}
		if line == "paths:" {
			var values []string
			for index+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[index+1]), "- ") {
				index++
				values = append(values, scan.UnquoteYAMLScalar(strings.TrimPrefix(strings.TrimSpace(lines[index]), "- ")))
			}
			frontmatter.Paths = values
			continue
		}
		if line == "metadata:" {
			metadata := make(map[string]any)
			for index+1 < len(lines) && strings.HasPrefix(lines[index+1], "  ") {
				index++
				if caps := metadataRe.FindStringSubmatch(lines[index]); len(caps) == 3 {
					metadata[caps[1]] = scan.UnquoteYAMLScalar(caps[2])
				}
			}
			frontmatter.SkillMetadata = metadata
		}
	}
	return frontmatter
}

func scopeRootForSkillTarget(target scan.ScanTarget) *string {
	if target.Scope != types.ScopeProject {
		return nil
	}
	matches := cursorScopeRootPattern.FindStringSubmatch(target.SourcePath)
	if len(matches) < 2 {
		return nil
	}
	prefix := matches[1]
	if prefix == "" {
		dot := "."
		return &dot
	}
	return &prefix
}

func cursorHookTargets(projectPath, homeDir string) []scan.ScanTarget {
	overrides := scan.ScanTargetOverrides{
		Sensitivity:   stringPtr("command_config"),
		ContentPolicy: stringPtr("structured_safe_fields_only"),
	}
	targets := []scan.ScanTarget{
		scan.ProjectTarget(projectPath, ".cursor/hooks.json", types.AgentCursor, types.KindHook, types.ParserJSON, overrides),
		scan.HomeTarget(homeDir, ".cursor/hooks.json", types.AgentCursor, types.KindHook, types.ParserJSON, overrides),
	}
	if runtime.GOOS == "darwin" {
		targets = append(targets, scan.ScanTarget{
			AbsolutePath:  "/Library/Application Support/Cursor/hooks.json",
			SourcePath:    "/Library/Application Support/Cursor/hooks.json",
			Scope:         types.ScopeManaged,
			Precedence:    80,
			Agent:         types.AgentCursor,
			Kind:          types.KindHook,
			Parser:        types.ParserJSON,
			Sensitivity:   "command_config",
			ContentPolicy: "structured_safe_fields_only",
		})
	}
	return targets
}

func scanCursorHooks(projectPath, homeDir string, base scan.ScannerBase) []types.DiscoveredItem {
	var evidence []types.DiscoveredItem
	for _, target := range cursorHookTargets(projectPath, homeDir) {
		evidence = append(evidence, scanCursorHookFile(target, base)...)
	}
	return evidence
}

func scanCursorHookFile(target scan.ScanTarget, base scan.ScannerBase) []types.DiscoveredItem {
	text, err := os.ReadFile(target.AbsolutePath)
	if err != nil {
		return nil
	}
	parsed := parsers.ParseJSON(string(text))
	if parsed.Ok == nil {
		errText := "invalid JSON"
		if parsed.Err != nil {
			errText = parsed.Err.Error
		}
		item := base.ParseFailed(scan.EvidenceBaseTargetFromScanTarget(target), types.KindHook, errText)
		return []types.DiscoveredItem{item}
	}
	root, ok := scan.AsRecord(parsed.Ok.Value)
	if !ok {
		return nil
	}
	hooks, ok := scan.AsRecord(root["hooks"])
	if !ok {
		return nil
	}
	var evidence []types.DiscoveredItem
	for eventName, definitions := range hooks {
		items, ok := definitions.([]any)
		if !ok {
			continue
		}
		for hookIndex, definitionValue := range items {
			definition, ok := scan.AsRecord(definitionValue)
			if !ok {
				continue
			}
			hookType := "command"
			if raw, ok := definition["type"].(string); ok {
				hookType = raw
			}
			command, _ := definition["command"].(string)
			hookValue := cursorHookValue(definition, hookType, command)
			metadata := map[string]any{
				"executable":      hookType == "command" && command != "",
				"policyEvaluated": hookType == "prompt",
				"eventName":       eventName,
				"hookIndex":       hookIndex,
				"hookCategory":    hookCategory(eventName),
				"source":          cursorHookSource(target.Scope),
				"sourcePriority":  cursorHookSourcePriority(target.Scope),
			}
			evidenceTarget := scan.EvidenceBaseTargetFromScanTarget(target)
			evidenceTarget.Sensitivity = "command_config"
			evidenceTarget.ContentPolicy = "structured_safe_fields_only"
			item := base.Captured(evidenceTarget, types.KindHook, metadata, hookValue)
			item.ID = base.ItemID(scan.ItemIDTarget{Agent: target.Agent, SourcePath: target.SourcePath, Scope: target.Scope}, "hook-"+eventName+"-"+itoa(hookIndex))
			item.Name = stringPtr(eventName + "." + itoa(hookIndex))
			evidence = append(evidence, item)
		}
	}
	return evidence
}

func cursorHookValue(definition map[string]any, hookType, command string) map[string]any {
	value := map[string]any{"type": hookType}
	if command != "" {
		value["command"] = command
	}
	for _, field := range []string{"timeout", "loop_limit", "failClosed", "matcher"} {
		if nested, ok := definition[field]; ok {
			value[field] = nested
		}
	}
	return value
}

func hookCategory(eventName string) string {
	switch eventName {
	case "beforeTabFileRead", "afterTabFileEdit":
		return "tab"
	case "workspaceOpen":
		return "app_lifecycle"
	default:
		return "agent"
	}
}

func cursorHookSource(scope types.EvidenceScope) string {
	if scope == types.ScopeManaged {
		return "enterprise"
	}
	return scope.String()
}

func cursorHookSourcePriority(scope types.EvidenceScope) uint32 {
	switch scope {
	case types.ScopeManaged:
		return 40
	case types.ScopeProject:
		return 30
	case types.ScopeUser:
		return 10
	default:
		return 0
	}
}

func cursorTeamHooksBlindSpot() types.DiscoveredItem {
	name := "Cursor team hooks"
	metadata, _ := json.Marshal(map[string]any{
		"reason":         "cloud_distributed_hooks_not_locally_readable",
		"source":         "team",
		"sourcePriority": 35,
	})
	return types.DiscoveredItem{
		ID:            "managed.cursor.cursor-team-hooks.unsupported",
		Agent:         types.AgentCursor,
		Kind:          types.KindUnsupported,
		SourcePath:    "<cursor-team-hooks>",
		Scope:         types.ScopeManaged,
		Precedence:    70,
		Parser:        types.ParserUnknown,
		Sensitivity:   "metadata",
		ContentPolicy: "metadata_only",
		RestorePolicy: types.RestoreNotSupported,
		CaptureStatus: types.CaptureUnsupported,
		Confidence:    types.ConfidenceMedium,
		Name:          &name,
		Metadata:      metadata,
	}
}
