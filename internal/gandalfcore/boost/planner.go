package boost

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// CreateBoostPlan creates an actionable plan to inject the requested profile's skills.
// If profileName is empty, it plans to remove all Gandalf-injected skills (clear mode).
func CreateBoostPlan(
	m *manifest.Manifest,
	profileName string,
	scope Scope,
	projectRoot string,
	homeDir string,
) (*BoostPlan, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest is nil")
	}
	if projectRoot == "" {
		return nil, fmt.Errorf("projectRoot cannot be empty")
	}
	if scope == "" {
		scope = ScopeProject
	}

	var resolvedSkills []string
	if profileName != "" {
		var err error
		resolvedSkills, err = ResolveSkills(m, profileName)
		if err != nil {
			return nil, fmt.Errorf("resolve profile '%s': %w", profileName, err)
		}
	}

	plan := &BoostPlan{
		ProfileName:    profileName,
		Scope:          scope,
		ResolvedSkills: resolvedSkills,
		Mutations:      nil,
		Warnings:       nil,
	}

	// Build map of skill source paths from manifest
	skillSourceMap := make(map[string]string)
	for _, sk := range m.Skills {
		if sk.Source != "" {
			skillSourceMap[sk.Name] = filepath.Join(projectRoot, filepath.Clean(sk.Source))
		} else {
			skillSourceMap[sk.Name] = filepath.Join(projectRoot, ".gandalf", "skills", sk.Name)
		}
	}

	// Target agents
	targetAgents := m.Agents
	if len(targetAgents) == 0 {
		targetAgents = []types.AgentID{types.AgentClaudeCode, types.AgentCursor, types.AgentCodex}
	}

	// Set of desired skills in the new profile
	desiredSkillSet := make(map[string]bool)
	for _, s := range resolvedSkills {
		desiredSkillSet[s] = true
	}

	// Load existing boost state to know what to prune
	existingState, _ := LoadState(projectRoot)
	existingInjected := make(map[string]InjectedSkillEntry)
	if existingState != nil {
		for _, entry := range existingState.InjectedSkills {
			key := fmt.Sprintf("%s:%s", entry.Agent, entry.SkillName)
			existingInjected[key] = entry
		}
	}

	// 1. Plan removals for skills that are in existingState but NOT in desiredSkillSet
	for key, entry := range existingInjected {
		if !desiredSkillSet[entry.SkillName] {
			// Check if target exists
			fi, err := os.Lstat(entry.TargetPath)
			if err == nil {
				if fi.Mode()&os.ModeSymlink != 0 {
					plan.Mutations = append(plan.Mutations, SymlinkMutation{
						Action:     ActionRemoveSymlink,
						SkillName:  entry.SkillName,
						Agent:      entry.Agent,
						TargetPath: entry.TargetPath,
						Reason:     fmt.Sprintf("skill '%s' is not in profile '%s'", entry.SkillName, profileName),
					})
				}
			}
		}
		_ = key
	}

	// 2. Plan additions / keep / conflicts for desired skills
	for _, skillName := range resolvedSkills {
		srcPath, ok := skillSourceMap[skillName]
		if !ok {
			srcPath = filepath.Join(projectRoot, ".gandalf", "skills", skillName)
		}

		// Verify source exists
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("Skill source directory does not exist: '%s'", srcPath))
		}

		for _, agent := range targetAgents {
			targetPath := resolveTargetPath(agent, skillName, scope, projectRoot, homeDir, m.Name)
			if targetPath == "" {
				continue
			}

			// Calculate symlink destination value
			symlinkDest := srcPath
			if scope == ScopeProject {
				targetDir := filepath.Dir(targetPath)
				rel, err := filepath.Rel(targetDir, srcPath)
				if err == nil {
					symlinkDest = rel
				}
			}

			fi, err := os.Lstat(targetPath)
			if os.IsNotExist(err) {
				plan.Mutations = append(plan.Mutations, SymlinkMutation{
					Action:        ActionCreateSymlink,
					SkillName:     skillName,
					Agent:         agent,
					SourcePath:    srcPath,
					TargetPath:    targetPath,
					SymlinkTarget: symlinkDest,
					Reason:        "new symlink",
				})
			} else if err == nil {
				if fi.Mode()&os.ModeSymlink != 0 {
					// It's already a symlink. Check where it points
					existingTarget, readErr := os.Readlink(targetPath)
					if readErr == nil && (existingTarget == symlinkDest || existingTarget == srcPath) {
						plan.Mutations = append(plan.Mutations, SymlinkMutation{
							Action:        ActionKeepSymlink,
							SkillName:     skillName,
							Agent:         agent,
							SourcePath:    srcPath,
							TargetPath:    targetPath,
							SymlinkTarget: symlinkDest,
							Reason:        "already pointing to correct skill",
						})
					} else if isGandalfManagedLink(existingTarget, projectRoot) {
						// Points to another gandalf skill, safe to replace
						plan.Mutations = append(plan.Mutations, SymlinkMutation{
							Action:        ActionCreateSymlink,
							SkillName:     skillName,
							Agent:         agent,
							SourcePath:    srcPath,
							TargetPath:    targetPath,
							SymlinkTarget: symlinkDest,
							Reason:        "replace previous profile symlink",
						})
					} else {
						// Points to an external path not managed by Gandalf
						plan.Mutations = append(plan.Mutations, SymlinkMutation{
							Action:        ActionConflictSkip,
							SkillName:     skillName,
							Agent:         agent,
							SourcePath:    srcPath,
							TargetPath:    targetPath,
							SymlinkTarget: symlinkDest,
							Reason:        "existing symlink points to non-Gandalf target",
						})
						plan.Warnings = append(plan.Warnings, fmt.Sprintf("Skipping '%s': existing symlink points to external '%s'", targetPath, existingTarget))
					}
				} else {
					// Regular file or directory! Safety barrier
					plan.Mutations = append(plan.Mutations, SymlinkMutation{
						Action:        ActionConflictSkip,
						SkillName:     skillName,
						Agent:         agent,
						SourcePath:    srcPath,
						TargetPath:    targetPath,
						SymlinkTarget: symlinkDest,
						Reason:        "regular file or directory exists (safety barrier)",
					})
					plan.Warnings = append(plan.Warnings, fmt.Sprintf("Skipping '%s': regular file/directory exists. Gandalf refuses to overwrite non-symlink assets.", targetPath))
				}
			}
		}
	}

	return plan, nil
}

func resolveTargetPath(agent types.AgentID, skillName string, scope Scope, projectRoot, homeDir, projectName string) string {
	if scope == ScopeProject {
		switch agent {
		case types.AgentClaudeCode:
			return filepath.Join(projectRoot, ".claude", "skills", skillName)
		case types.AgentCursor:
			return filepath.Join(projectRoot, ".cursor", "skills", skillName)
		case types.AgentCodex:
			return filepath.Join(projectRoot, ".codex", "skills", skillName)
		}
	} else {
		// Global scope
		prefix := ""
		if projectName != "" {
			prefix = projectName + "--"
		}
		switch agent {
		case types.AgentClaudeCode:
			return filepath.Join(homeDir, ".claude", "skills", prefix+skillName)
		case types.AgentCursor:
			return filepath.Join(homeDir, ".cursor", "skills", prefix+skillName)
		case types.AgentCodex:
			return filepath.Join(homeDir, ".codex", "skills", prefix+skillName)
		}
	}
	return ""
}

func isGandalfManagedLink(linkTarget, projectRoot string) bool {
	cleanTarget := filepath.Clean(linkTarget)
	if strings.Contains(cleanTarget, ".gandalf/skills") {
		return true
	}
	if strings.HasPrefix(cleanTarget, filepath.Join(projectRoot, ".gandalf", "skills")) {
		return true
	}
	return false
}
