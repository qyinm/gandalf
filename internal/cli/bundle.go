package cli

import (
	"fmt"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/bundle"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
	"github.com/spf13/cobra"
)

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Export, import, inspect, and verify .gandalf bundle archives",
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
		Short: "Export a snapshot to a .gandalf bundle",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runBundleExport(cmd, &common, name, out, metadataOnly)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	common.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&name, "name", "", "Snapshot name to export")
	_ = cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&out, "out", "", "Output .gandalf bundle path")
	_ = cmd.MarkFlagRequired("out")
	cmd.Flags().BoolVar(&metadataOnly, "metadata-only", false, "Export metadata only (no content)")
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
		Short: "Import a .gandalf bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode := runBundleImport(cmd, &common, args[0], dryRun, applyContent, quarantine, experimental, trust)
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
	return cmd
}

func newBundleInspectCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "inspect [bundle-path]",
		Short: "Inspect bundle metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode := runBundleInspect(cmd, args[0], jsonOut)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON output")
	return cmd
}

func newBundleVerifyCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "verify [bundle-path]",
		Short: "Verify bundle format, checksums, and signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode := runBundleVerify(cmd, args[0], jsonOut)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON output")
	return cmd
}

func runBundleExport(cmd *cobra.Command, common *CommonFlags, name, out string, metadataOnly bool) int {
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}
	includeContent := !metadataOnly
	result, err := bundle.Export(&types.BundleExportOptions{
		SnapshotName:   name,
		OutputPath:     out,
		StoreDir:       runtime.StoreDir,
		ProjectPath:    runtime.ProjectPath,
		HomeDir:        runtime.HomeDir,
		IncludeContent: &includeContent,
		Agent:          runtime.Agent,
	})
	if err != nil {
		return writeError(cmd.ErrOrStderr(), bundleSnapError("GANDALF_BUNDLE_EXPORT_FAILED", "Failed to export bundle.", err))
	}
	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	lines := []string{
		"gandalf bundle export",
		"",
		"Bundle: " + result.BundlePath,
		"Checksum: " + result.Checksum,
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	return writeStdout(cmd.OutOrStdout(), strings.Join(lines, "\n")+"\n")
}

func runBundleImport(cmd *cobra.Command, common *CommonFlags, bundlePath string, dryRun, applyContent, quarantine, experimental, trust bool) int {
	if applyContent && !experimental {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "GANDALF_EXPERIMENTAL_REQUIRED",
			Problem: "Bundle content apply requires --experimental.",
			Cause:   "`bundle import --apply-content` was called without `--experimental`.",
			Fix:     "Run `gandalf bundle import <path> --apply-content --experimental`.",
		})
	}
	runtime, snapErr := resolveRuntime(common)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}
	result, err := bundle.Import(&types.BundleImportOptions{
		BundlePath:   bundlePath,
		StoreDir:     runtime.StoreDir,
		ProjectPath:  runtime.ProjectPath,
		HomeDir:      runtime.HomeDir,
		ApplyContent: &applyContent,
		DryRun:       &dryRun,
		Quarantine:   &quarantine,
		Trust:        &trust,
		Agent:        runtime.Agent,
	})
	if err != nil {
		return writeError(cmd.ErrOrStderr(), bundleSnapError("GANDALF_BUNDLE_IMPORT_FAILED", "Failed to import bundle.", err))
	}
	if common.JSON {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	lines := []string{
		"gandalf bundle import",
		"",
		"Snapshot: " + result.SnapshotName,
		fmt.Sprintf("Evidence items: %d", result.EvidenceCount),
		fmt.Sprintf("Content applied: %v", result.ContentApplied),
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	return writeStdout(cmd.OutOrStdout(), strings.Join(lines, "\n")+"\n")
}

func runBundleInspect(cmd *cobra.Command, bundlePath string, jsonOut bool) int {
	result, err := bundle.Inspect(bundlePath)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), bundleSnapError("GANDALF_BUNDLE_INSPECT_FAILED", "Failed to inspect bundle.", err))
	}
	if jsonOut {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	lines := []string{
		"gandalf bundle inspect",
		"",
		"Bundle: " + result.BundlePath,
		fmt.Sprintf("Format version: %d", result.FormatVersion),
		"Snapshot: " + result.SnapshotName,
		fmt.Sprintf("Includes content: %v", result.IncludesContent),
		"Checksum: " + result.BundleChecksum,
	}
	return writeStdout(cmd.OutOrStdout(), strings.Join(lines, "\n")+"\n")
}

func runBundleVerify(cmd *cobra.Command, bundlePath string, jsonOut bool) int {
	result, err := bundle.Verify(&types.BundleVerifyOptions{BundlePath: bundlePath})
	if err != nil {
		return writeError(cmd.ErrOrStderr(), bundleSnapError("GANDALF_BUNDLE_VERIFY_FAILED", "Failed to verify bundle.", err))
	}
	if jsonOut {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	status := "invalid"
	if result.Valid {
		status = "valid"
	}
	lines := []string{
		"gandalf bundle verify",
		"",
		"Bundle: " + result.BundlePath,
		"Status: " + status,
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	for _, errText := range result.Errors {
		lines = append(lines, "Error: "+errText)
	}
	exitCode := 0
	if !result.Valid {
		exitCode = 1
	}
	if writeStdout(cmd.OutOrStdout(), strings.Join(lines, "\n")+"\n") != 0 {
		return 1
	}
	return exitCode
}

func bundleSnapError(code, problem string, err error) *types.SnapError {
	return &types.SnapError{
		Code:    code,
		Problem: problem,
		Cause:   err.Error(),
		Fix:     "Verify bundle path, store directory, and snapshot name.",
	}
}
