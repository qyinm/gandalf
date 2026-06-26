package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/scan"
	"github.com/qyinm/hem/internal/hemcore/types"
)

type piSkillTarget struct {
	absolutePath     string
	sourcePath       string
	scope            types.EvidenceScope
	precedence       uint32
	includeRootFiles bool
	source           string
}

type piExtensionTarget struct {
	absolutePath string
	sourcePath   string
	scope        types.EvidenceScope
	precedence   uint32
	source       string
}

// PiAgentScanner discovers Pi agent configuration, extensions, and skills.
type PiAgentScanner struct{}

func (PiAgentScanner) AgentID() types.AgentID   { return types.AgentPiAgent }
func (PiAgentScanner) AgentName() string        { return "Pi Agent" }
func (PiAgentScanner) Description() string {
	return "Pi coding agent configuration (settings, models, agents, extensions, skills, themes, prompts)"
}

func (p PiAgentScanner) Targets(projectPath, homeDir string) []scan.ScanTarget {
	dir := true
	return []scan.ScanTarget{
		scan.ProjectTarget(projectPath, ".pi/settings.json", types.AgentPiAgent, types.KindAgentConfig, types.ParserJSON, scan.ScanTargetOverrides{}),
		scan.ProjectTarget(projectPath, ".pi/themes", types.AgentPiAgent, types.KindUnsupported, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir, Sensitivity: stringPtr("themes")}),
		scan.ProjectTarget(projectPath, ".pi/prompts", types.AgentPiAgent, types.KindAgentInstruction, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir, Sensitivity: stringPtr("prompt_templates")}),
		scan.HomeTarget(homeDir, ".pi/agent/settings.json", types.AgentPiAgent, types.KindAgentConfig, types.ParserJSON, scan.ScanTargetOverrides{}),
		scan.HomeTarget(homeDir, ".pi/agent/models.json", types.AgentPiAgent, types.KindAgentConfig, types.ParserJSON, scan.ScanTargetOverrides{MetadataOnly: boolPtr(true), Sensitivity: stringPtr("model_registry")}),
		scan.HomeTarget(homeDir, ".pi/agents", types.AgentPiAgent, types.KindUnsupported, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir, Sensitivity: stringPtr("custom_agents")}),
		scan.HomeTarget(homeDir, ".pi/agent/themes", types.AgentPiAgent, types.KindUnsupported, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir, Sensitivity: stringPtr("themes")}),
		scan.HomeTarget(homeDir, ".pi/agent/prompts", types.AgentPiAgent, types.KindAgentInstruction, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir, Sensitivity: stringPtr("prompt_templates")}),
	}
}

func (p PiAgentScanner) Scan(context *scan.ScannerContext) []types.DiscoveredItem {
	evidence := scan.ScanTargets(p.Targets(context.ProjectPath, context.HomeDir))
	var extensionEvidence []types.DiscoveredItem
	var skillEvidence []types.DiscoveredItem
	for _, target := range piExtensionTargets(context.ProjectPath, context.HomeDir) {
		extensionEvidence = append(extensionEvidence, scanPiExtensionTarget(target)...)
	}
	for _, target := range piSkillTargets(context.ProjectPath, context.HomeDir) {
		skillEvidence = append(skillEvidence, scanPiSkillTarget(target)...)
	}
	evidence = append(evidence, dedupePiExtensions(extensionEvidence)...)
	evidence = append(evidence, dedupePiSkills(skillEvidence)...)
	return evidence
}

func boolPtr(value bool) *bool { return &value }

func piExtensionTargets(projectPath, homeDir string) []piExtensionTarget {
	targets := []piExtensionTarget{
		makePiExtensionTarget(homeDir, ".pi/agent/extensions", types.ScopeUser, 10, "auto"),
		makePiExtensionTarget(projectPath, ".pi/extensions", types.ScopeProject, 40, "auto"),
	}
	targets = append(targets, configuredExtensionTargets(projectPath, homeDir)...)
	targets = append(targets, packageExtensionTargets(projectPath, homeDir)...)
	return targets
}

func makePiExtensionTarget(root, relativePath string, scope types.EvidenceScope, precedence uint32, source string) piExtensionTarget {
	sourcePath := relativePath
	if scope == types.ScopeUser {
		sourcePath = "~/" + relativePath
	}
	return piExtensionTarget{
		absolutePath: filepath.Join(root, relativePath),
		sourcePath:   sourcePath,
		scope:        scope,
		precedence:   precedence,
		source:       source,
	}
}

func piSkillTargets(projectPath, homeDir string) []piSkillTarget {
	targets := []piSkillTarget{
		makePiSkillTarget(homeDir, ".pi/agent/skills", types.ScopeUser, 10, true, "pi"),
		makePiSkillTarget(projectPath, ".pi/skills", types.ScopeProject, 40, true, "pi"),
		makePiSkillTarget(homeDir, ".agents/skills", types.ScopeUser, 15, false, "agents"),
	}
	targets = append(targets, ancestorAgentSkillTargets(projectPath)...)
	targets = append(targets, configuredSkillTargets(projectPath, homeDir)...)
	targets = append(targets, packageSkillTargets(projectPath, homeDir)...)
	return targets
}

func makePiSkillTarget(root, relativePath string, scope types.EvidenceScope, precedence uint32, includeRootFiles bool, source string) piSkillTarget {
	sourcePath := relativePath
	if scope == types.ScopeUser {
		sourcePath = "~/" + relativePath
	}
	return piSkillTarget{
		absolutePath:     filepath.Join(root, relativePath),
		sourcePath:       sourcePath,
		scope:            scope,
		precedence:       precedence,
		includeRootFiles: includeRootFiles,
		source:           source,
	}
}

func ancestorAgentSkillTargets(projectPath string) []piSkillTarget {
	repoRoot := findGitRepoRoot(projectPath)
	dir, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		dir = projectPath
	}
	var targets []piSkillTarget
	for {
		absolutePath := filepath.Join(dir, ".agents", "skills")
		sourcePath := ".agents/skills"
		if rel, err := filepath.Rel(projectPath, absolutePath); err == nil && rel != "." {
			sourcePath = filepath.ToSlash(rel)
		}
		targets = append(targets, piSkillTarget{
			absolutePath:     absolutePath,
			sourcePath:       sourcePath,
			scope:            types.ScopeProject,
			precedence:       35,
			includeRootFiles: false,
			source:           "agents",
		})
		if repoRoot != "" && dir == repoRoot {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return targets
}

func findGitRepoRoot(startDir string) string {
	dir, err := filepath.EvalSymlinks(startDir)
	if err != nil {
		dir = startDir
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func configuredExtensionTargets(projectPath, homeDir string) []piExtensionTarget {
	settings := []struct {
		path       string
		baseDir    string
		scope      types.EvidenceScope
		precedence uint32
	}{
		{filepath.Join(homeDir, ".pi/agent/settings.json"), filepath.Join(homeDir, ".pi/agent"), types.ScopeUser, 20},
		{filepath.Join(projectPath, ".pi/settings.json"), filepath.Join(projectPath, ".pi"), types.ScopeProject, 50},
	}
	var targets []piExtensionTarget
	for _, setting := range settings {
		value := readJSONObject(setting.path)
		if value == nil {
			continue
		}
		for _, rawPath := range scan.ArrayOfStrings(value["extensions"]) {
			absolutePath := resolveConfiguredPath(rawPath, setting.baseDir, homeDir)
			targets = append(targets, piExtensionTarget{
				absolutePath: absolutePath,
				sourcePath:   displayPath(absolutePath, homeDir, projectPath),
				scope:        setting.scope,
				precedence:   setting.precedence,
				source:       "settings",
			})
		}
	}
	return targets
}

func configuredSkillTargets(projectPath, homeDir string) []piSkillTarget {
	settings := []struct {
		path       string
		baseDir    string
		scope      types.EvidenceScope
		precedence uint32
	}{
		{filepath.Join(homeDir, ".pi/agent/settings.json"), filepath.Join(homeDir, ".pi/agent"), types.ScopeUser, 20},
		{filepath.Join(projectPath, ".pi/settings.json"), filepath.Join(projectPath, ".pi"), types.ScopeProject, 50},
	}
	var targets []piSkillTarget
	for _, setting := range settings {
		value := readJSONObject(setting.path)
		if value == nil {
			continue
		}
		for _, rawPath := range scan.ArrayOfStrings(value["skills"]) {
			absolutePath := resolveConfiguredPath(rawPath, setting.baseDir, homeDir)
			targets = append(targets, piSkillTarget{
				absolutePath:     absolutePath,
				sourcePath:       displayPath(absolutePath, homeDir, projectPath),
				scope:            setting.scope,
				precedence:       setting.precedence,
				includeRootFiles: true,
				source:           "settings",
			})
		}
	}
	return targets
}

func packageExtensionTargets(projectPath, homeDir string) []piExtensionTarget {
	return packageConfiguredTargets(projectPath, homeDir, "extensions", func(root string, scope types.EvidenceScope, precedence uint32, rawPath string) piExtensionTarget {
		absolutePath := resolveConfiguredPath(rawPath, root, homeDir)
		return piExtensionTarget{
			absolutePath: absolutePath,
			sourcePath:   displayPath(absolutePath, homeDir, projectPath),
			scope:        scope,
			precedence:   precedence,
			source:       "package",
		}
	})
}

func packageSkillTargets(projectPath, homeDir string) []piSkillTarget {
	return packageConfiguredTargets(projectPath, homeDir, "skills", func(root string, scope types.EvidenceScope, precedence uint32, rawPath string) piSkillTarget {
		absolutePath := resolveConfiguredPath(rawPath, root, homeDir)
		return piSkillTarget{
			absolutePath:     absolutePath,
			sourcePath:       displayPath(absolutePath, homeDir, projectPath),
			scope:            scope,
			precedence:       precedence,
			includeRootFiles: true,
			source:           "package",
		}
	})
}

func packageConfiguredTargets[T any](projectPath, homeDir, field string, build func(root string, scope types.EvidenceScope, precedence uint32, rawPath string) T) []T {
	settings := []struct {
		path       string
		scope      types.EvidenceScope
		precedence uint32
	}{
		{filepath.Join(homeDir, ".pi/agent/settings.json"), types.ScopeUser, 25},
		{filepath.Join(projectPath, ".pi/settings.json"), types.ScopeProject, 55},
	}
	var targets []T
	for _, setting := range settings {
		value := readJSONObject(setting.path)
		if value == nil {
			continue
		}
		for _, spec := range scan.ArrayOfStrings(value["packages"]) {
			packageRoot := resolvePackageRoot(spec)
			if packageRoot == "" {
				continue
			}
			packageJSON := readJSONObject(filepath.Join(packageRoot, "package.json"))
			if packageJSON == nil {
				continue
			}
			piConfig, _ := scan.AsRecord(packageJSON["pi"])
			rawPaths := scan.ArrayOfStrings(piConfig[field])
			if len(rawPaths) == 0 {
				rawPaths = []string{field}
			}
			for _, rawPath := range rawPaths {
				targets = append(targets, build(packageRoot, setting.scope, setting.precedence, rawPath))
			}
		}
	}
	return targets
}

func scanPiExtensionTarget(target piExtensionTarget) []types.DiscoveredItem {
	var evidence []types.DiscoveredItem
	for _, extensionFile := range findPiExtensionFiles(target.absolutePath) {
		info, err := os.Stat(extensionFile.filePath)
		if err != nil {
			continue
		}
		realPath, _ := filepath.EvalSymlinks(extensionFile.filePath)
		sourcePath := displayExtensionSourcePath(target, extensionFile)
		entrypoint := filepath.Base(extensionFile.filePath)
		style := "single_file"
		if entrypoint == "index.ts" || entrypoint == "index.js" {
			style = "directory_index"
		}
		name := extensionNameFromPath(extensionFile.filePath, extensionFile.root)
		metadata, _ := json.Marshal(map[string]any{
			"present":        true,
			"source":         target.source,
			"executable":     true,
			"entrypoint":     entrypoint,
			"extensionStyle": style,
			"sizeBytes":      info.Size(),
			"realPath":       realPath,
		})
		evidence = append(evidence, types.DiscoveredItem{
			ID:            scan.ScannerItemID(target.scope, types.AgentPiAgent, sourcePath, "extension"),
			Agent:         types.AgentPiAgent,
			Kind:          types.KindExtension,
			SourcePath:    sourcePath,
			Scope:         target.scope,
			Precedence:    target.precedence,
			Parser:        types.ParserFilesystem,
			Sensitivity:   "command_config",
			ContentPolicy: "metadata_only",
			RestorePolicy: types.RestoreFullContent,
			CaptureStatus: types.CaptureCaptured,
			Confidence:    types.ConfidenceHigh,
			Name:          &name,
			Metadata:      metadata,
		})
	}
	return evidence
}

type piExtensionFile struct {
	filePath string
	root     string
}

func findPiExtensionFiles(root string) []piExtensionFile {
	info, err := os.Stat(root)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		if isExtensionFile(root) {
			return []piExtensionFile{{filePath: root, root: root}}
		}
		return nil
	}
	return collectPiExtensionEntries(root, root)
}

func collectPiExtensionEntries(dir, root string) []piExtensionFile {
	manifestEntries := resolvePiExtensionEntries(dir, root)
	if len(manifestEntries) > 0 {
		return manifestEntries
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var discovered []piExtensionFile
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.IsDir() && isExtensionFile(path) {
			discovered = append(discovered, piExtensionFile{filePath: path, root: root})
		} else if info.IsDir() {
			discovered = append(discovered, resolvePiExtensionEntries(path, root)...)
		}
	}
	return discovered
}

func resolvePiExtensionEntries(dir, root string) []piExtensionFile {
	packageJSON := readJSONObject(filepath.Join(dir, "package.json"))
	var piConfig map[string]any
	if packageJSON != nil {
		piConfig, _ = scan.AsRecord(packageJSON["pi"])
	}
	for _, rawPath := range scan.ArrayOfStrings(piConfig["extensions"]) {
		resolved := filepath.Join(dir, rawPath)
		if files := findPiExtensionFilesFromManifestPath(resolved, root); len(files) > 0 {
			return files
		}
	}
	for _, indexFile := range []string{"index.ts", "index.js"} {
		path := filepath.Join(dir, indexFile)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return []piExtensionFile{{filePath: path, root: root}}
		}
	}
	return nil
}

func findPiExtensionFilesFromManifestPath(absolutePath, root string) []piExtensionFile {
	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		if isExtensionFile(absolutePath) {
			return []piExtensionFile{{filePath: absolutePath, root: root}}
		}
		return nil
	}
	return collectPiExtensionEntries(absolutePath, root)
}

func isExtensionFile(filePath string) bool {
	ext := filepath.Ext(filePath)
	return ext == ".ts" || ext == ".js"
}

func extensionNameFromPath(filePath, root string) string {
	entrypoint := filepath.Base(filePath)
	if entrypoint == "index.ts" || entrypoint == "index.js" {
		if _, err := os.Stat(filepath.Join(root, "package.json")); err == nil {
			return filepath.Base(root)
		}
		return filepath.Base(filepath.Dir(filePath))
	}
	return strings.TrimSuffix(strings.TrimSuffix(entrypoint, ".ts"), ".js")
}

func scanPiSkillTarget(target piSkillTarget) []types.DiscoveredItem {
	var evidence []types.DiscoveredItem
	for _, skillFile := range findPiSkillFiles(target.absolutePath, target.includeRootFiles) {
		frontmatter := readPiSkillFrontmatter(skillFile.filePath)
		if frontmatter == nil || frontmatter.Description == nil || strings.TrimSpace(*frontmatter.Description) == "" {
			continue
		}
		description := *frontmatter.Description
		name := filepath.Base(skillFile.skillDir)
		if frontmatter.Name != nil {
			name = *frontmatter.Name
		}
		sourcePath := displaySkillSourcePath(target, skillFile)
		entrypoint := filepath.Base(skillFile.filePath)
		if strings.HasSuffix(skillFile.filePath, "/SKILL.md") {
			entrypoint = "SKILL.md"
		}
		metadata, _ := json.Marshal(map[string]any{
			"present":              true,
			"source":               target.source,
			"entrypoint":           entrypoint,
			"entrypointStatus":     skillEntrypointStatus(target.absolutePath, skillFile.filePath),
			"entrypointSizeBytes":  frontmatter.SizeBytes,
			"declaredName":         frontmatter.Name,
			"directoryName":        filepath.Base(skillFile.skillDir),
			"nameMatchesDirectory": name == filepath.Base(skillFile.skillDir),
			"description":          description,
			"disableModelInvocation": frontmatter.DisableModelInvocation != nil && *frontmatter.DisableModelInvocation,
		})
		evidence = append(evidence, types.DiscoveredItem{
			ID:            scan.ScannerItemID(target.scope, types.AgentPiAgent, sourcePath, "skill"),
			Agent:         types.AgentPiAgent,
			Kind:          types.KindSkill,
			SourcePath:    sourcePath,
			Scope:         target.scope,
			Precedence:    target.precedence,
			Parser:        types.ParserFilesystem,
			Sensitivity:   "metadata",
			ContentPolicy: "metadata_only",
			RestorePolicy: types.RestoreFullContent,
			CaptureStatus: types.CaptureCaptured,
			Confidence:    types.ConfidenceHigh,
			Name:          &name,
			Metadata:      metadata,
		})
	}
	return evidence
}

type piSkillFile struct {
	filePath string
	skillDir string
	root     string
}

func findPiSkillFiles(root string, includeRootFiles bool) []piSkillFile {
	var files []piSkillFile
	seen := make(map[string]struct{})
	walkPiSkillFiles(root, root, includeRootFiles, &files, seen)
	return files
}

func walkPiSkillFiles(dir, root string, includeRootFiles bool, files *[]piSkillFile, seen map[string]struct{}) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return
	}
	if _, ok := seen[resolved]; ok {
		return
	}
	seen[resolved] = struct{}{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() == "SKILL.md" {
			filePath := filepath.Join(dir, "SKILL.md")
			if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
				*files = append(*files, piSkillFile{filePath: filePath, skillDir: dir, root: root})
			}
			return
		}
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.IsDir() {
			walkPiSkillFiles(path, root, false, files, seen)
			continue
		}
		if includeRootFiles && strings.HasSuffix(name, ".md") {
			*files = append(*files, piSkillFile{filePath: path, skillDir: filepath.Dir(path), root: root})
		}
	}
}

type piFrontmatter struct {
	Name                   *string
	Description            *string
	DisableModelInvocation *bool
	SizeBytes              uint64
}

func readPiSkillFrontmatter(filePath string) *piFrontmatter {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil
	}
	text, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	matches := skillFrontmatterPattern.FindStringSubmatch(string(text))
	frontmatter := &piFrontmatter{SizeBytes: uint64(info.Size())}
	if len(matches) < 2 {
		return frontmatter
	}
	fieldRe := regexp.MustCompile(`^(name|description|disable-model-invocation):\s*(.*)$`)
	for _, line := range strings.Split(matches[1], "\n") {
		if caps := fieldRe.FindStringSubmatch(strings.TrimSpace(line)); len(caps) == 3 {
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
		}
	}
	return frontmatter
}

func dedupePiExtensions(evidence []types.DiscoveredItem) []types.DiscoveredItem {
	seen := make(map[string]struct{})
	var result []types.DiscoveredItem
	for _, item := range evidence {
		realPath := ""
		if len(item.Metadata) > 0 {
			var metadata map[string]any
			_ = json.Unmarshal(item.Metadata, &metadata)
			if value, ok := metadata["realPath"].(string); ok {
				realPath = value
			}
		}
		if realPath != "" {
			if _, ok := seen[realPath]; ok {
				continue
			}
			seen[realPath] = struct{}{}
		}
		result = append(result, item)
	}
	return result
}

func dedupePiSkills(evidence []types.DiscoveredItem) []types.DiscoveredItem {
	result := make([]types.DiscoveredItem, 0, len(evidence))
	indexes := make(map[string]int)
	realPaths := make(map[string]struct{})
	for _, item := range evidence {
		if item.Name == nil {
			result = append(result, item)
			continue
		}
		if existingIndex, ok := indexes[*item.Name]; ok {
			existing := result[existingIndex]
			duplicateSources := append(scan.MetadataStringArray(existing.Metadata, "duplicateSources"), item.SourcePath)
			result[existingIndex] = mergeDuplicateSources(existing, duplicateSources)
			continue
		}
		indexes[*item.Name] = len(result)
		result = append(result, item)
	}
	_ = realPaths
	return result
}

func readJSONObject(filePath string) map[string]any {
	text, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(text, &value); err != nil {
		return nil
	}
	return value
}

func resolveConfiguredPath(rawPath, baseDir, homeDir string) string {
	if rawPath == "~" {
		return homeDir
	}
	if rest, ok := strings.CutPrefix(rawPath, "~/"); ok {
		return filepath.Join(homeDir, rest)
	}
	if filepath.IsAbs(rawPath) {
		return rawPath
	}
	return filepath.Join(baseDir, rawPath)
}

func displayPath(absolutePath, homeDir, projectPath string) string {
	resolved, _ := filepath.EvalSymlinks(absolutePath)
	resolvedHome, _ := filepath.EvalSymlinks(homeDir)
	resolvedProject, _ := filepath.EvalSymlinks(projectPath)
	if resolved == resolvedHome || strings.HasPrefix(resolved, resolvedHome+string(os.PathSeparator)) {
		if rel, err := filepath.Rel(resolvedHome, resolved); err == nil {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	if resolved == resolvedProject {
		return "."
	}
	if rel, err := filepath.Rel(resolvedProject, resolved); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(resolved)
}

func displayExtensionSourcePath(target piExtensionTarget, extensionFile piExtensionFile) string {
	rel, _ := filepath.Rel(target.absolutePath, extensionFile.filePath)
	if rel == "." || rel == "" {
		return target.sourcePath
	}
	return target.sourcePath + "/" + filepath.ToSlash(rel)
}

func displaySkillSourcePath(target piSkillTarget, skillFile piSkillFile) string {
	rel, _ := filepath.Rel(target.absolutePath, skillFile.filePath)
	if rel == "." || rel == "SKILL.md" {
		return target.sourcePath
	}
	if strings.HasSuffix(filepath.ToSlash(rel), "/SKILL.md") {
		return target.sourcePath + "/" + strings.TrimSuffix(filepath.ToSlash(rel), "/SKILL.md")
	}
	return target.sourcePath + "/" + filepath.ToSlash(rel)
}

func resolvePackageRoot(spec string) string {
	packageName := packageNameFromSpec(spec)
	if packageName == "" {
		return ""
	}
	for _, root := range nodeModuleRoots() {
		packageRoot := filepath.Join(root, packageName)
		if _, err := os.Stat(filepath.Join(packageRoot, "package.json")); err == nil {
			return packageRoot
		}
	}
	return ""
}

func packageNameFromSpec(spec string) string {
	value := spec
	if rest, ok := strings.CutPrefix(value, "npm:"); ok {
		value = rest
	}
	if strings.HasPrefix(value, "@") {
		parts := strings.Split(value, "/")
		if len(parts) < 2 {
			return ""
		}
		scope := parts[0]
		name := strings.Split(parts[1], "@")[0]
		return scope + "/" + name
	}
	return strings.Split(value, "@")[0]
}

func nodeModuleRoots() []string {
	var roots []string
	if execPath, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Join(filepath.Dir(execPath), "..", "lib", "node_modules"))
	}
	roots = append(roots, "/opt/homebrew/lib/node_modules", "/usr/local/lib/node_modules")
	seen := make(map[string]struct{})
	var deduped []string
	for _, root := range roots {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		deduped = append(deduped, root)
	}
	return deduped
}