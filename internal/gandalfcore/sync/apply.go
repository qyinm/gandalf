package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/fsutil"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ApplySyncPlan applies the planned changes to local agent configurations and skill directories.
func ApplySyncPlan(plan *SyncPlan, roots *pathconfinement.Roots, storeDir string) (*SyncApplyResult, error) {
	if len(plan.Items) == 0 {
		return &SyncApplyResult{
			Success:      true,
			AppliedItems: nil,
		}, nil
	}

	// 1. Verify Path Confinement for all targets
	for _, item := range plan.Items {
		if roots != nil {
			if _, err := pathconfinement.ValidateConstrainedWritePath(item.TargetFile, roots); err != nil {
				return nil, fmt.Errorf("confinement check failed for '%s': %w", item.TargetFile, err)
			}
		}
	}

	// 2. Create Pre-apply backup snapshot
	backupName := fmt.Sprintf("preapply-manifest-%s", time.Now().Format("20060102-150405"))
	var backupAgent *types.AgentID
	if len(plan.Manifest.Agents) == 1 {
		backupAgent = &plan.Manifest.Agents[0]
	}
	userScope := types.ScopeUser
	if storeDir != "" {
		state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
			ProjectPath:    plan.ProjectRoot,
			HomeDir:        plan.HomeDir,
			StoreDir:       storeDir,
			Agent:          backupAgent,
			Scope:          &userScope,
			CaptureContent: true,
		}, backupName)
		if err == nil && state != nil {
			_ = store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), backupAgent)
		} else {
			backupName = ""
		}
	} else {
		backupName = ""
	}

	var applied []SyncPlanItem
	var errors []string

	// 3. Apply items
	for _, item := range plan.Items {
		switch item.Action {
		case "update":
			// Ensure parent directory exists
			dir := filepath.Dir(item.TargetFile)
			if err := os.MkdirAll(dir, 0755); err != nil {
				errors = append(errors, fmt.Sprintf("create dir '%s': %v", dir, err))
				continue
			}

			if err := fsutil.WriteTextAtomically(item.TargetFile, item.Content, 0644); err != nil {
				errors = append(errors, fmt.Sprintf("write file '%s': %v", item.TargetFile, err))
				continue
			}
			applied = append(applied, item)

		case "copy":
			if item.SourceFile == "" {
				continue
			}
			if err := copyDirOrFile(item.SourceFile, item.TargetFile); err != nil {
				errors = append(errors, fmt.Sprintf("copy '%s' to '%s': %v", item.SourceFile, item.TargetFile, err))
				continue
			}
			applied = append(applied, item)
		}
	}

	success := len(errors) == 0
	return &SyncApplyResult{
		Success:        success,
		BackupSnapshot: backupName,
		AppliedItems:   applied,
		Errors:         errors,
	}, nil
}

func copyDirOrFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(src)
	if err == nil {
		_ = os.Chmod(dst, sourceInfo.Mode())
	}
	return nil
}

func copyDir(src, dst string) error {
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, sourceInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}
