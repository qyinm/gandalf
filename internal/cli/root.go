package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// Execute runs the gandalf CLI and returns an exit code.
func Execute() int {
	cmd := NewRootCmd()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	if err := cmd.Execute(); err != nil {
		if code, ok := IsExitError(err); ok {
			return code
		}
		return 1
	}
	return 0
}

// NewRootCmd builds the root Cobra command tree.
func NewRootCmd() *cobra.Command {
	var common CommonFlags

	root := &cobra.Command{
		Use:           "gandalf",
		Short:         "Manage global agent skills, hooks, MCP servers, and plugins.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUICommand(cmd, &common)
		},
	}

	common.bindFlags(root.Flags())
	root.AddCommand(newScanCmd())
	root.AddCommand(newSnapshotCmd())
	root.AddCommand(newDiffCmd())
	root.AddCommand(newRestoreCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newReportCmd())
	root.AddCommand(newTimelineCmd())
	root.AddCommand(newBundleCmd())
	root.AddCommand(newTuiCmd())

	return root
}
