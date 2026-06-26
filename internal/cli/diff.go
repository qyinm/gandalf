package cli

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "diff [baseline] [target]",
		Short: "Show semantic and raw-source changes between two snapshots",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode := runDiff(cmd, &common, args[0], args[1])
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	return cmd
}

func runDiff(cmd *cobra.Command, common *CommonFlags, baselineRef, targetRef string) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	before, snapErr := snapshotByRef(baselineRef, &runtime)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}
	after, snapErr := snapshotByRef(targetRef, &runtime)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	graphDiff := diff.DiffGraphs(before.Graph, after.Graph)
	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), graphDiff)
	}
	return writeStdout(cmd.OutOrStdout(), renderDiffText(&graphDiff))
}
