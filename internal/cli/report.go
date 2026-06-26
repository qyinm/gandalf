package cli

import (
	"github.com/spf13/cobra"
)

func newReportCmd() *cobra.Command {
	var common CommonFlags
	var out string

	cmd := &cobra.Command{
		Use:   "report [reference]",
		Short: "Generate a markdown report of agent state and findings",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = common
			_ = out
			exitCode := writeError(cmd.ErrOrStderr(), notImplementedError("hem report"))
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&out, "out", "", "Write report to file instead of stdout")
	return cmd
}