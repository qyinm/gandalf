package cli

import (
	"strings"

	"github.com/qyinm/hem/internal/hemcore/readiness"
	"github.com/qyinm/hem/internal/hemcore/scan"
	"github.com/qyinm/hem/internal/hemcore/types"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var common CommonFlags

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local readiness for agent setup portability",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runDoctor(cmd, &common)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	return cmd
}

func runDoctor(cmd *cobra.Command, common *CommonFlags) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	scanResult := scan.ScanProject(&types.ScanOptions{
		ProjectPath: runtime.ProjectPath,
		HomeDir:     runtime.HomeDir,
		StoreDir:    runtime.StoreDir,
		Agent:       runtime.Agent,
		Scope:       runtime.Scope,
	})
	report := readiness.BuildReadinessReport(scanResult.Evidence, &types.ReadinessOptions{
		SourceHomeDir:  &runtime.HomeDir,
		TargetEvidence: scanResult.Evidence,
	})

	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), report)
	}

	lines := []string{
		"hem doctor",
		"",
		"Target platform: " + report.TargetPlatform,
		"",
	}
	lines = append(lines, readiness.FormatReadinessSummaryLines(&report, &types.ReadinessFormatOptions{
		MaxItems:       10,
		IncludeFixes:   true,
		IncludeActions: true,
	})...)
	if len(report.Items) == 0 {
		lines = append(lines, "", "No readiness issues found.")
	}

	exitCode := 0
	for _, item := range report.Items {
		if item.Category == types.ReadinessBlocked {
			exitCode = 1
			break
		}
	}
	if writeStdout(cmd.OutOrStdout(), strings.Join(lines, "\n")+"\n") != 0 {
		return 1
	}
	return exitCode
}