package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/store"
	"github.com/qyinm/hem/internal/hemcore/types"
	"github.com/spf13/cobra"
)

func newTimelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "List and inspect local timeline entries",
	}
	cmd.AddCommand(newTimelineListCmd())
	cmd.AddCommand(newTimelineShowCmd())
	cmd.AddCommand(newTimelineUndoCmd())
	return cmd
}

func newTimelineListCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List timeline entries",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runTimelineList(cmd, &common)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	return cmd
}

func newTimelineShowCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "show [reference]",
		Short: "Show a timeline entry by id or snapshot name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode := runTimelineShow(cmd, &common, args[0])
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	return cmd
}

func newTimelineUndoCmd() *cobra.Command {
	var common CommonFlags
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "undo [reference]",
		Short: "Build a dry-run MCP undo plan for a timeline entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode := runTimelineUndo(cmd, &common, args[0], dryRun)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview undo plan without making changes")
	return cmd
}

func runTimelineList(cmd *cobra.Command, common *CommonFlags) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	var corruptEvents []store.TimelineCorruptEvent
	entries, err := store.ListTimelineEntries(runtime.StoreDir, store.TimelineListOptions{
		Agent:       runtime.Agent,
		ProjectPath: runtime.ProjectPath,
		OnCorruptEntry: func(event store.TimelineCorruptEvent) {
			corruptEvents = append(corruptEvents, event)
		},
	})
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "HEM_TIMELINE_LIST_FAILED",
			Problem: "Failed to list timeline entries.",
			Cause:   err.Error(),
			Fix:     "Verify the store directory is readable.",
		})
	}

	for _, event := range corruptEvents {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Skipped corrupt timeline event: %s (%s)\n", event.FilePath, event.Error)
	}

	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), entries)
	}
	if len(entries) == 0 {
		return writeStdout(cmd.OutOrStdout(), "No timeline entries.\n")
	}

	lines := []string{"hem timeline", ""}
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf(
			"- %s %s (%s) -> %s",
			entry.ID,
			entry.Title,
			entry.EventKind,
			entry.AfterSnapshotName,
		))
	}
	return writeStdout(cmd.OutOrStdout(), strings.Join(lines, "\n")+"\n")
}

func runTimelineShow(cmd *cobra.Command, common *CommonFlags, reference string) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	var corruptEvents []store.TimelineCorruptEvent
	entry, err := store.FindTimelineEntry(runtime.StoreDir, reference, store.TimelineListOptions{
		Agent:       runtime.Agent,
		ProjectPath: runtime.ProjectPath,
		OnCorruptEntry: func(event store.TimelineCorruptEvent) {
			corruptEvents = append(corruptEvents, event)
		},
	})
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "HEM_TIMELINE_LOOKUP_FAILED",
			Problem: "Failed to look up timeline entry.",
			Cause:   err.Error(),
			Fix:     "Run `hem timeline list` to see available entries.",
		})
	}
	for _, event := range corruptEvents {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Skipped corrupt timeline event: %s (%s)\n", event.FilePath, event.Error)
	}
	if entry == nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "HEM_TIMELINE_NOT_FOUND",
			Problem: fmt.Sprintf("Timeline entry not found: %q.", reference),
			Cause:   "The reference does not match a timeline id or snapshot name.",
			Fix:     "Run `hem timeline list` to see available entries.",
		})
	}

	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), entry)
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "HEM_JSON_SERIALIZE_FAILED",
			Problem: "Failed to serialize timeline entry.",
			Cause:   err.Error(),
			Fix:     "This is an internal error.",
		})
	}
	return writeStdout(cmd.OutOrStdout(), string(data)+"\n")
}

func runTimelineUndo(cmd *cobra.Command, _ *CommonFlags, _ string, dryRun bool) int {
	if !dryRun {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "HEM_TIMELINE_UNDO_DRY_RUN_REQUIRED",
			Problem: "Timeline undo requires --dry-run.",
			Cause:   "`timeline undo` was called without `--dry-run`.",
			Fix:     "Run `hem timeline undo <id> --dry-run --json`.",
		})
	}
	return writeError(cmd.ErrOrStderr(), notImplementedError("hem timeline undo"))
}