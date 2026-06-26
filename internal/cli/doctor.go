package cli

import (
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local readiness for agent setup portability",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = common
			exitCode := writeError(cmd.ErrOrStderr(), notImplementedError("hem doctor"))
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	return cmd
}