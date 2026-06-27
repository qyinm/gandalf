package cli

import (
	"github.com/qyinm/gandalf/internal/tui"
	"github.com/spf13/cobra"
)

var launchTUI = tui.Run

func newTuiCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive global setup console",
		Long:  "Open the Bubble Tea TUI with the top-tab setup console as the first screen.",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runTUICommand(cmd, &common) },
	}

	common.bindFlags(cmd.Flags())
	return cmd
}

func runTUICommand(cmd *cobra.Command, common *CommonFlags) error {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return errExit(writeError(cmd.ErrOrStderr(), snapErr))
	}
	exitCode := launchTUI(runtime)
	if exitCode != 0 {
		return errExit(exitCode)
	}
	return nil
}
