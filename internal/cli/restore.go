package cli

import (
	"fmt"
	"os"

	"github.com/qyinm/gandalf/internal/gandalfcore/restore"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
	"github.com/spf13/cobra"
)

func newRestoreCmd() *cobra.Command {
	var common CommonFlags
	var snapshotName string
	var dryRun bool
	var apply bool
	var experimental bool
	var failFast bool
	var rollback bool

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Generate a restore plan (dry-run) or apply a snapshot (experimental)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runRestore(cmd, &common, restoreFlags{
				Snapshot:     snapshotName,
				DryRun:       dryRun,
				Apply:        apply,
				Experimental: experimental,
				FailFast:     failFast,
				Rollback:     rollback,
			})
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&snapshotName, "snapshot", "", "Snapshot to restore from")
	_ = cmd.MarkFlagRequired("snapshot")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview restore plan without making changes (default when --apply is omitted)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply restore items (requires --experimental)")
	cmd.Flags().BoolVar(&experimental, "experimental", false, "Enable experimental apply mode")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop on first failure during apply")
	cmd.Flags().BoolVar(&rollback, "rollback", false, "Apply then automatically rollback on failure")
	cmd.MarkFlagsMutuallyExclusive("dry-run", "apply")
	return cmd
}

type restoreFlags struct {
	Snapshot     string
	DryRun       bool
	Apply        bool
	Experimental bool
	FailFast     bool
	Rollback     bool
}

func runRestore(cmd *cobra.Command, common *CommonFlags, flags restoreFlags) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	if _, err := store.EnsureStore(runtime.StoreDir); err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_STORE_INIT_FAILED",
			Problem: "Failed to initialize store.",
			Cause:   err.Error(),
			Fix:     "Verify the store directory is writable.",
		})
	}

	exists, err := store.SnapshotExists(runtime.StoreDir, flags.Snapshot, runtime.Agent)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_SNAPSHOT_LOOKUP_FAILED",
			Problem: fmt.Sprintf("Failed to look up snapshot %q.", flags.Snapshot),
			Cause:   err.Error(),
			Fix:     "Run `gandalf snapshot list` to see available snapshots.",
		})
	}
	if !exists {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_SNAPSHOT_NOT_FOUND",
			Problem: fmt.Sprintf("Snapshot %q not found.", flags.Snapshot),
			Cause:   "The named snapshot does not exist in the store.",
			Fix:     "Run `gandalf snapshot list` to see available snapshots.",
		})
	}

	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: flags.Snapshot,
		ProjectPath:    runtime.ProjectPath,
		HomeDir:        runtime.HomeDir,
		StoreDir:       runtime.StoreDir,
		DryRun:         true,
		Agent:          runtime.Agent,
		Scope:          runtime.Scope,
	})
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_RESTORE_PLAN_FAILED",
			Problem: "Failed to build restore plan.",
			Cause:   err.Error(),
			Fix:     "Verify snapshot, project, and scope options.",
		})
	}

	if !flags.Apply {
		if common.JSON {
			return writeJSON(cmd.OutOrStdout(), plan)
		}
		return writeStdout(cmd.OutOrStdout(), formatRestorePlanPreview(plan))
	}

	experimental := flags.Experimental || os.Getenv("GANDALF_EXPERIMENTAL") != ""
	if !experimental {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_EXPERIMENTAL_REQUIRED",
			Problem: "Restore --apply requires --experimental.",
			Cause:   "--apply was used without GANDALF_EXPERIMENTAL=1 or --experimental.",
			Fix:     "Set GANDALF_EXPERIMENTAL=1 or pass --experimental to enable experimental features.",
		})
	}

	parsed := restore.RestoreItemsFromPlan(plan)
	if len(parsed.Errors) > 0 {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_RESTORE_PARSE_ERROR",
			Problem: "Failed to parse restore plan for execution.",
			Cause:   parsed.Errors[0].Message,
			Fix:     "This is an internal error. Verify the snapshot is valid and try again.",
		})
	}

	items := parsed.Items
	applyExecutor := restore.CreateDefaultApplyExecutor()
	undoExecutor := restore.CreateDefaultUndoExecutor()
	homeDir := runtime.HomeDir
	projectPath := runtime.ProjectPath
	result := restore.ApplyWithRollback(
		items,
		applyExecutor,
		undoExecutor,
		&types.ApplyOptions{
			FailFast:    flags.FailFast,
			Rollback:    &flags.Rollback,
			HomeDir:     &homeDir,
			ProjectPath: &projectPath,
		},
	)

	output := restore.FormatApplySummary(&result.ApplySummary)
	if result.RollbackSummary != nil {
		output += "\n" + formatRollbackSummary(result.RollbackSummary)
	}

	if result.ApplySummary.Failed > 0 ||
		(result.RollbackSummary != nil && result.RollbackSummary.Failed > 0) {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), output)
		return 1
	}
	return writeStdout(cmd.OutOrStdout(), output)
}
