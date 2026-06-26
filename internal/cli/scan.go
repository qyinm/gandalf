package cli

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var common CommonFlags
	var explain bool

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan project for agent configuration and emit evidence inventory",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runScan(cmd, &common, explain)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	cmd.Flags().BoolVar(&explain, "explain", false, "Include paths considered during the scan")
	return cmd
}

func runScan(cmd *cobra.Command, common *CommonFlags, explain bool) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	scanResult := scan.ScanProject(&types.ScanOptions{
		ProjectPath: runtime.ProjectPath,
		HomeDir:     runtime.HomeDir,
		StoreDir:    runtime.StoreDir,
		Explain:     explain,
		Agent:       runtime.Agent,
		Scope:       runtime.Scope,
	})

	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), scanResult)
	}

	output := renderScanText(&scanResult)
	if explain {
		output = renderScanExplainText(&scanResult)
	}
	return writeStdout(cmd.OutOrStdout(), output)
}
