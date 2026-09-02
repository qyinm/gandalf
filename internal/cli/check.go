package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/sync"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type checkFlags struct {
	CommonFlags
	ManifestPath string
	CI           bool
}

func newCheckCmd() *cobra.Command {
	var flags checkFlags

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check for drift between team agent manifest and local setup.",
		Long: `Check compares the declarative team manifest (gandalf.toml) with your
local agent environment and reports missing MCP servers, skills, or hooks.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runCheck(cmd, &flags)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	flags.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&flags.ManifestPath, "manifest", "", "Path to gandalf.toml (default: search project root)")
	cmd.Flags().BoolVar(&flags.CI, "ci", false, "Exit with non-zero status code if drift or errors are detected")

	return cmd
}

func runCheck(cmd *cobra.Command, flags *checkFlags) int {
	runtime, snapErr := resolveRuntime(&flags.CommonFlags)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	manifestPath := flags.ManifestPath
	if manifestPath == "" {
		found, err := manifest.FindManifestFile(runtime.ProjectPath)
		if err != nil {
			return writeError(cmd.ErrOrStderr(), &types.SnapError{
				Code:    "MANIFEST_NOT_FOUND",
				Problem: "No team manifest file found in project",
				Cause:   err.Error(),
				Fix:     "Run 'gandalf init' to create a gandalf.toml in this repository",
			})
		}
		manifestPath = found
	}

	res, err := manifest.LoadManifest(manifestPath, nil)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "MANIFEST_PARSE_ERROR",
			Problem: "Failed to parse manifest file",
			Cause:   err.Error(),
			Fix:     "Check syntax of gandalf.toml",
		})
	}

	validationErrs := manifest.Validate(res.Manifest, runtime.ProjectPath)
	if len(validationErrs) > 0 {
		var msgs []string
		for _, v := range validationErrs {
			msgs = append(msgs, v.Error())
		}
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "MANIFEST_VALIDATION_ERROR",
			Problem: "Manifest validation failed",
			Cause:   strings.Join(msgs, "; "),
			Fix:     validationErrs[0].Fix,
		})
	}

	scanOptions := &types.ScanOptions{
		ProjectPath: runtime.ProjectPath,
		HomeDir:     runtime.HomeDir,
		StoreDir:    runtime.StoreDir,
	}
	baseScan := scan.ScanProject(scanOptions)

	drift, err := sync.DetectDrift(res.Manifest, runtime.ProjectPath, runtime.HomeDir, baseScan.Evidence)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "DRIFT_CHECK_ERROR",
			Problem: "Failed to perform drift check",
			Cause:   err.Error(),
		})
	}

	if flags.JSON {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"manifest": res.Manifest,
			"drift":    drift,
		})
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "📦 Team Manifest: %s (version: %s)\n", res.Manifest.Name, res.Manifest.Version)
	_, _ = fmt.Fprintf(out, "🎯 Target Agents: %s\n", formatAgentsList(res.Manifest.Agents))
	_, _ = fmt.Fprintln(out)

	if drift.InSync {
		_, _ = fmt.Fprintln(out, "✅ [IN SYNC] Local agent setup matches the team manifest perfectly!")
		return 0
	}

	_, _ = fmt.Fprintln(out, "⚠️  [DRIFT DETECTED] The following items are missing or out of sync:")
	for i, item := range drift.Items {
		_, _ = fmt.Fprintf(out, "  [%d] [%s] %s: %s (%s)\n", i+1, item.Agent, item.Kind, item.Name, item.TargetFile)
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "💡 Run 'gandalf apply' to synchronize your agent environment.")

	if flags.CI {
		return 1
	}

	return 0
}
