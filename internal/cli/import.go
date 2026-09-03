package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type importFlags struct {
	CommonFlags
	ProjectOnly bool
	From        string
	DryRun      bool
	Force       bool
	Output      string
}

func newImportCmd() *cobra.Command {
	var flags importFlags

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Reverse-generate gandalf.toml from existing agent configurations.",
		Long: `Import scans existing AI agent configurations (Cursor, Claude Code, OpenAI Codex, .mcp.json)
across your project and local machine to generate a standardized team manifest (gandalf.toml).

By default, import discovers both repository-level and machine-global configurations,
redacts hardcoded secrets into [env_template], and mirrors team skills into .gandalf/skills/.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runImport(cmd, &flags)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	flags.bindFlags(cmd.Flags())
	cmd.Flags().BoolVar(&flags.ProjectOnly, "project-only", false, "Restrict scan to repository files only (ignore user home directory)")
	cmd.Flags().StringVar(&flags.From, "from", "", "Import strictly from a specific configuration file or directory")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Preview generated gandalf.toml without writing to disk")
	cmd.Flags().BoolVarP(&flags.Force, "force", "f", false, "Overwrite existing gandalf.toml if present")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "gandalf.toml", "Output manifest file path")

	return cmd
}

func runImport(cmd *cobra.Command, flags *importFlags) int {
	runtime, snapErr := resolveRuntime(&flags.CommonFlags)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	opts := importer.ImportOptions{
		ProjectPath: runtime.ProjectPath,
		HomeDir:     runtime.HomeDir,
		ProjectOnly: flags.ProjectOnly,
		FromPath:    flags.From,
		DryRun:      flags.DryRun,
		Force:       flags.Force,
		OutputFile:  flags.Output,
	}

	res, err := importer.RunImport(opts)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "IMPORT_FAILED",
			Problem: err.Error(),
		})
	}

	if flags.JSON {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"manifest":       res.Manifest,
			"sources":        res.Sources,
			"extractedEnvs":  res.ExtractedEnvs,
			"mirroredSkills": res.MirroredSkills,
			"warnings":       res.Warnings,
			"dryRun":         flags.DryRun,
			"outputFile":     flags.Output,
		})
	}

	out := cmd.OutOrStdout()
	if flags.DryRun {
		_, _ = fmt.Fprintln(out, "[DRY-RUN] No files were written. Previewing generated manifest:")
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprint(out, res.FormattedTOML)
		return 0
	}

	if len(res.Warnings) > 0 {
		_, _ = fmt.Fprintln(out, "⚠️ Warnings encountered during import:")
		for _, w := range res.Warnings {
			_, _ = fmt.Fprintf(out, "  - %s\n", w)
		}
		_, _ = fmt.Fprintln(out)
	}

	_, _ = fmt.Fprintf(out, "🎉 Successfully generated %s!\n", flags.Output)
	_, _ = fmt.Fprintf(out, "🔍 Discovered %d source(s), %d MCP server(s), %d skill(s)\n",
		len(res.Sources), len(res.Manifest.MCPServers), len(res.Manifest.Skills))
	if len(res.ExtractedEnvs) > 0 {
		_, _ = fmt.Fprintf(out, "🔒 Secret protection: templated %d sensitive variable(s) into [env_template]\n", len(res.ExtractedEnvs))
	}
	if len(res.MirroredSkills) > 0 {
		_, _ = fmt.Fprintf(out, "📁 Mirrored %d skill(s) into .gandalf/skills/\n", len(res.MirroredSkills))
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Next steps:")
	_, _ = fmt.Fprintf(out, "  1. Review '%s' and configure values in [env_template].\n", flags.Output)
	_, _ = fmt.Fprintln(out, "  2. Run 'gandalf check' to verify parity locally and in CI.")
	_, _ = fmt.Fprintln(out, "  3. Team members can run 'gandalf apply' to sync their local agent setup.")

	return 0
}
