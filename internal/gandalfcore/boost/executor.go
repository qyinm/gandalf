package boost

import (
	"fmt"
	"os"
	"path/filepath"
)

// ApplyBoostPlan executes the mutations specified in a BoostPlan.
func ApplyBoostPlan(plan *BoostPlan, projectRoot string) (*BoostApplyResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan cannot be nil")
	}

	result := &BoostApplyResult{
		Success:       true,
		ActiveProfile: plan.ProfileName,
		Scope:         plan.Scope,
		Created:       nil,
		Removed:       nil,
		Skipped:       nil,
		Errors:        nil,
	}

	var currentlyInjected []InjectedSkillEntry

	for _, m := range plan.Mutations {
		switch m.Action {
		case ActionRemoveSymlink:
			fi, err := os.Lstat(m.TargetPath)
			if err == nil {
				if fi.Mode()&os.ModeSymlink != 0 {
					if err := os.Remove(m.TargetPath); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("remove symlink '%s': %v", m.TargetPath, err))
						continue
					}
					result.Removed = append(result.Removed, m)
				} else {
					result.Skipped = append(result.Skipped, m)
				}
			}

		case ActionCreateSymlink:
			dir := filepath.Dir(m.TargetPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create parent dir '%s': %v", dir, err))
				continue
			}

			// Atomic symlink creation
			tmpPath := fmt.Sprintf("%s.tmp.%d", m.TargetPath, os.Getpid())
			_ = os.Remove(tmpPath)

			if err := os.Symlink(m.SymlinkTarget, tmpPath); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create symlink '%s' -> '%s': %v", tmpPath, m.SymlinkTarget, err))
				continue
			}

			// If target already exists and is a symlink, remove it or let Rename replace it
			if fi, err := os.Lstat(m.TargetPath); err == nil {
				if fi.Mode()&os.ModeSymlink != 0 {
					_ = os.Remove(m.TargetPath)
				} else {
					_ = os.Remove(tmpPath)
					result.Errors = append(result.Errors, fmt.Sprintf("target '%s' is regular file/dir, refusing to overwrite", m.TargetPath))
					continue
				}
			}

			if err := os.Rename(tmpPath, m.TargetPath); err != nil {
				_ = os.Remove(tmpPath)
				result.Errors = append(result.Errors, fmt.Sprintf("rename symlink to '%s': %v", m.TargetPath, err))
				continue
			}

			result.Created = append(result.Created, m)
			currentlyInjected = append(currentlyInjected, InjectedSkillEntry{
				SkillName:  m.SkillName,
				Agent:      m.Agent,
				TargetPath: m.TargetPath,
			})

		case ActionKeepSymlink:
			currentlyInjected = append(currentlyInjected, InjectedSkillEntry{
				SkillName:  m.SkillName,
				Agent:      m.Agent,
				TargetPath: m.TargetPath,
			})

		case ActionConflictSkip:
			result.Skipped = append(result.Skipped, m)
		}
	}

	if len(result.Errors) > 0 {
		result.Success = false
	}

	// Update persisted boost state
	newState := &BoostState{
		ActiveProfile:  plan.ProfileName,
		Scope:          plan.Scope,
		ProjectRoot:    projectRoot,
		InjectedSkills: currentlyInjected,
	}

	if plan.ProfileName == "" && len(currentlyInjected) == 0 {
		_ = RemoveState(projectRoot)
	} else {
		_ = SaveState(projectRoot, newState)
	}

	return result, nil
}
