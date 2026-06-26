package plugins

import (
	"encoding/json"
	"path/filepath"

	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// OpenCodeScanner discovers OpenCode configuration and skills.
type OpenCodeScanner struct{}

func (OpenCodeScanner) AgentID() types.AgentID { return types.AgentOpencode }
func (OpenCodeScanner) AgentName() string      { return "OpenCode" }
func (OpenCodeScanner) Description() string {
	return "OpenCode CLI configuration (MCP servers, plugins, providers, skills)"
}

func (OpenCodeScanner) Targets(_ string, homeDir string) []scan.ScanTarget {
	return []scan.ScanTarget{
		scan.HomeTarget(homeDir, ".config/opencode/opencode.json", types.AgentOpencode, types.KindAgentConfig, types.ParserJSON, scan.ScanTargetOverrides{}),
	}
}

func (o OpenCodeScanner) Scan(context *scan.ScannerContext) []types.DiscoveredItem {
	evidence := scan.ScanTargets(o.Targets(context.ProjectPath, context.HomeDir))
	skillEvidence := []types.DiscoveredItem{builtinCustomizeOpencodeSkill()}
	for _, target := range opencodeSkillTargets(context.ProjectPath, context.HomeDir) {
		skillEvidence = append(skillEvidence, scanOpencodeSkillDirectory(target)...)
	}
	evidence = append(evidence, dedupeSkillsByName(skillEvidence)...)
	return evidence
}

func opencodeSkillTargets(projectPath, homeDir string) []scan.ScanTarget {
	dir := true
	return []scan.ScanTarget{
		scan.ProjectTarget(projectPath, ".opencode/skills", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".config/opencode/skills", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.ProjectTarget(projectPath, ".opencode/skill", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".config/opencode/skill", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.ProjectTarget(projectPath, ".claude/skills", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".claude/skills", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.ProjectTarget(projectPath, ".agents/skills", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".agents/skills", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
		scan.HomeTarget(homeDir, ".cache/opencode/packages", types.AgentOpencode, types.KindSkill, types.ParserFilesystem, scan.ScanTargetOverrides{Directory: &dir}),
	}
}

func scanOpencodeSkillDirectory(target scan.ScanTarget) []types.DiscoveredItem {
	var evidence []types.DiscoveredItem
	for _, skillFile := range findSkillFiles(target.AbsolutePath).files {
		namePtr, descriptionPtr, sizeBytes := readAgentSkillFrontmatter(skillFile)
		if namePtr == nil || descriptionPtr == nil {
			continue
		}
		name := *namePtr
		description := *descriptionPtr
		if !validAgentSkillName(name) {
			continue
		}
		skillDir := filepath.Dir(skillFile)
		directoryName := filepath.Base(skillDir)
		relSkillDir, _ := filepath.Rel(target.AbsolutePath, skillDir)
		sourcePath := target.SourcePath
		if relSkillDir != "." {
			sourcePath = target.SourcePath + "/" + filepath.ToSlash(relSkillDir)
		}
		metadata, _ := json.Marshal(map[string]any{
			"present":              true,
			"entrypoint":           "SKILL.md",
			"entrypointStatus":     skillEntrypointStatus(target.AbsolutePath, skillFile),
			"entrypointSizeBytes":  sizeBytes,
			"declaredName":         name,
			"directoryName":        directoryName,
			"nameMatchesDirectory": name == directoryName,
			"description":          description,
		})
		evidence = append(evidence, types.DiscoveredItem{
			ID:            scan.ScannerItemID(target.Scope, target.Agent, sourcePath, "skill"),
			Agent:         target.Agent,
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
			Metadata:      metadata,
		})
	}
	return evidence
}

func builtinCustomizeOpencodeSkill() types.DiscoveredItem {
	name := "customize-opencode"
	metadata, _ := json.Marshal(map[string]any{
		"present":      true,
		"builtIn":      true,
		"declaredName": name,
		"description":  "Use when editing or creating opencode configuration, agents, skills, plugins, MCP servers, or permission rules.",
	})
	return types.DiscoveredItem{
		ID:            "managed.opencode.built-in.customize-opencode.skill",
		Agent:         types.AgentOpencode,
		Kind:          types.KindSkill,
		SourcePath:    "<built-in>",
		Scope:         types.ScopeManaged,
		Precedence:    100,
		Parser:        types.ParserFilesystem,
		Sensitivity:   "metadata",
		ContentPolicy: "metadata_only",
		RestorePolicy: types.RestoreNotSupported,
		CaptureStatus: types.CaptureCaptured,
		Confidence:    types.ConfidenceHigh,
		Name:          &name,
		Metadata:      metadata,
	}
}
