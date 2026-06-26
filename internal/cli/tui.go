package cli

import (
	"github.com/qyinm/gandalf/internal/tui"
	"github.com/spf13/cobra"
)

func newTuiCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive setup-history workspace",
		Long:  "Open the Bubble Tea TUI with History > All changes as the first screen.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, snapErr := resolveRuntime(&common)
			if snapErr != nil {
				return errExit(writeError(cmd.ErrOrStderr(), snapErr))
			}
			exitCode := tui.Run(runtime)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	return cmd
}
