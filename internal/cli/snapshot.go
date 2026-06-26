package cli

import (
	"fmt"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
	"github.com/spf13/cobra"
)

func newSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Create, list, and show snapshots",
	}
	cmd.AddCommand(newSnapshotCreateCmd())
	cmd.AddCommand(newSnapshotListCmd())
	cmd.AddCommand(newSnapshotShowCmd())
	return cmd
}

func newSnapshotCreateCmd() *cobra.Command {
	var common CommonFlags
	var name string
	var metadataOnly bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Capture and persist a snapshot",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runSnapshotCreate(cmd, &common, name, metadataOnly)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&name, "name", "", "Snapshot name")
	_ = cmd.MarkFlagRequired("name")
	cmd.Flags().BoolVar(&metadataOnly, "metadata-only", false, "Store metadata only (no content capture)")
	return cmd
}

func newSnapshotListCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List snapshots in the store",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runSnapshotList(cmd, &common)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	return cmd
}

func newSnapshotShowCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show snapshot metadata (or full JSON with --json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode := runSnapshotShow(cmd, &common, args[0])
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	return cmd
}

func runSnapshotCreate(cmd *cobra.Command, common *CommonFlags, name string, metadataOnly bool) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	contentBackedCodexUser := runtime.Agent != nil &&
		*runtime.Agent == types.AgentCodex &&
		runtime.Scope != nil &&
		*runtime.Scope == types.ScopeUser

	if !metadataOnly && !contentBackedCodexUser {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_METADATA_ONLY_REQUIRED",
			Problem: "Snapshots are metadata-only.",
			Cause:   "`snapshot create` was called without `--metadata-only`.",
			Fix:     "Add `--metadata-only`, or use `--agent codex --scope user` for the Codex rollback safety-net path.",
		})
	}

	captureRuntime := runtime
	captureRuntime.CaptureContent = !metadataOnly && contentBackedCodexUser

	state, err := snapshot.CaptureCurrentState(&captureRuntime, name)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_SNAPSHOT_CAPTURE_FAILED",
			Problem: "Failed to capture current state.",
			Cause:   err.Error(),
			Fix:     "Verify project and store paths are accessible.",
		})
	}

	if err := store.WriteSnapshot(runtime.StoreDir, store.StoreSnapshotFrom(state.Snapshot), runtime.Agent); err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_SNAPSHOT_WRITE_FAILED",
			Problem: "Failed to write snapshot.",
			Cause:   err.Error(),
			Fix:     "Verify the store directory is writable.",
		})
	}

	kind := "content-backed"
	if metadataOnly {
		kind = "metadata-only"
	}
	line := fmt.Sprintf("Created %s snapshot: %s", kind, name)
	if runtime.Agent != nil {
		line += fmt.Sprintf(" (agent: %s)", runtime.Agent.String())
	}
	if runtime.Scope != nil {
		line += fmt.Sprintf(" (scope: %s)", runtime.Scope.String())
	}
	line += "\n"
	return writeStdout(cmd.OutOrStdout(), line)
}

func runSnapshotList(cmd *cobra.Command, common *CommonFlags) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	names, err := store.ListSnapshots(runtime.StoreDir, runtime.Agent)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_SNAPSHOT_LIST_FAILED",
			Problem: "Failed to list snapshots.",
			Cause:   err.Error(),
			Fix:     "Verify the store directory exists and is readable.",
		})
	}

	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), names)
	}

	if len(names) == 0 {
		return writeStdout(cmd.OutOrStdout(), "No snapshots.\n")
	}
	return writeStdout(cmd.OutOrStdout(), strings.Join(names, "\n")+"\n")
}

func runSnapshotShow(cmd *cobra.Command, common *CommonFlags, name string) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	snap, err := store.ReadSnapshot(runtime.StoreDir, name, runtime.Agent)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_SNAPSHOT_NOT_FOUND",
			Problem: fmt.Sprintf("Snapshot %q not found.", name),
			Cause:   err.Error(),
			Fix:     "Run `gandalf snapshot list` to see available snapshots.",
		})
	}

	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), snap)
	}
	return writeStdout(cmd.OutOrStdout(), snap.Manifest.Name+"\n")
}
