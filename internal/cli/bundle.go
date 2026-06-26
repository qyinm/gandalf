package cli

import (
	"github.com/spf13/cobra"
)

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Export, import, inspect, and verify .hem bundle archives",
	}
	cmd.AddCommand(newBundleExportCmd())
	cmd.AddCommand(newBundleImportCmd())
	cmd.AddCommand(newBundleInspectCmd())
	cmd.AddCommand(newBundleVerifyCmd())
	return cmd
}

func newBundleExportCmd() *cobra.Command {
	var common CommonFlags
	var name string
	var out string
	var metadataOnly bool

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a snapshot to a .hem bundle",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := writeError(cmd.ErrOrStderr(), notImplementedError("hem bundle export"))
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&name, "name", "", "Snapshot name to export")
	_ = cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&out, "out", "", "Output .hem bundle path")
	_ = cmd.MarkFlagRequired("out")
	cmd.Flags().BoolVar(&metadataOnly, "metadata-only", false, "Export metadata only (no content)")
	_ = common
	_ = metadataOnly
	return cmd
}

func newBundleImportCmd() *cobra.Command {
	var common CommonFlags
	var dryRun bool
	var applyContent bool
	var quarantine bool
	var experimental bool
	var trust bool

	cmd := &cobra.Command{
		Use:   "import [bundle-path]",
		Short: "Import a .hem bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := writeError(cmd.ErrOrStderr(), notImplementedError("hem bundle import"))
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview import without writing")
	cmd.Flags().BoolVar(&applyContent, "apply-content", false, "Apply content-backed files from bundle")
	cmd.Flags().BoolVar(&quarantine, "quarantine", false, "Quarantine suspicious bundle content")
	cmd.Flags().BoolVar(&experimental, "experimental", false, "Enable experimental import features")
	cmd.Flags().BoolVar(&trust, "trust", false, "Trust bundle signature without verification prompt")
	_ = common
	_ = dryRun
	_ = applyContent
	_ = quarantine
	_ = experimental
	_ = trust
	return cmd
}

func newBundleInspectCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "inspect [bundle-path]",
		Short: "Inspect bundle metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := writeError(cmd.ErrOrStderr(), notImplementedError("hem bundle inspect"))
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON output")
	_ = jsonOut
	return cmd
}

func newBundleVerifyCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "verify [bundle-path]",
		Short: "Verify bundle format, checksums, and signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := writeError(cmd.ErrOrStderr(), notImplementedError("hem bundle verify"))
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON output")
	_ = jsonOut
	return cmd
}