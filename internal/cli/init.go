package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qyinm/gandalf/internal/gandalfcore/fsutil"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type initFlags struct {
	CommonFlags
	Name        string
	FromCurrent bool
	Force       bool
}

func newInitCmd() *cobra.Command {
	var flags initFlags

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a team agent manifest (gandalf.toml) in current repository.",
		Long: `Init creates a starter gandalf.toml and .gandalf/skills/ directory
to standardize AI agent setup across your engineering team.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runInit(cmd, &flags)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	flags.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&flags.Name, "name", "", "Project name (default: directory name)")
	cmd.Flags().BoolVar(&flags.FromCurrent, "from-current", false, "Scaffold manifest by scanning current local agent setup")
	cmd.Flags().BoolVarP(&flags.Force, "force", "f", false, "Overwrite existing gandalf.toml if present")

	return cmd
}

func runInit(cmd *cobra.Command, flags *initFlags) int {
	runtime, snapErr := resolveRuntime(&flags.CommonFlags)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	targetPath := filepath.Join(runtime.ProjectPath, "gandalf.toml")
	if _, err := os.Stat(targetPath); err == nil && !flags.Force {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "MANIFEST_EXISTS",
			Problem: "gandalf.toml already exists in this repository",
			Fix:     "Use --force to overwrite existing manifest",
		})
	}

	projectName := flags.Name
	if projectName == "" {
		projectName = filepath.Base(runtime.ProjectPath)
		if projectName == "." || projectName == "/" || projectName == "" {
			projectName = "team-project"
		}
	}

	skillsDir := filepath.Join(runtime.ProjectPath, ".gandalf", "skills")
	_ = os.MkdirAll(skillsDir, 0755)

	var tomlContent string

	if flags.FromCurrent {
		scanOptions := &types.ScanOptions{
			ProjectPath: runtime.ProjectPath,
			HomeDir:     runtime.HomeDir,
			StoreDir:    runtime.StoreDir,
		}
		baseScan := scan.ScanProject(scanOptions)

		var mcpSections []string
		seenMCP := make(map[string]bool)

		for _, item := range baseScan.Evidence {
			if item.Kind == types.KindMcpServer && item.Name != nil {
				name := *item.Name
				if !seenMCP[name] {
					seenMCP[name] = true
					mcpSections = append(mcpSections, fmt.Sprintf(`[mcp_servers.%s]
description = "Imported from local %s setup"
# command = "npx"
# args = ["-y", "@mcp/server"]
`, name, item.Agent))
				}
			}
		}

		mcpBlock := strings.Join(mcpSections, "\n")
		if mcpBlock == "" {
			mcpBlock = `[mcp_servers.example-db]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres", "${DATABASE_URL}"]
required_env = ["DATABASE_URL"]
description = "Staging database MCP server"
`
		}

		tomlContent = fmt.Sprintf(`# gandalf.toml - AI Agent Environment Declaration
version = "1.0"
name = "%s"
description = "Standardized AI agent environment for %s"
agents = ["claude-code", "codex", "cursor"]

# Team MCP Servers
%s
# Team Skills (placed in .gandalf/skills/)
# [[skills]]
# name = "team-reviewer"
# source = "./.gandalf/skills/team-reviewer"

# Hooks
# [hooks.pre-save-lint]
# event = "before_save"
# command = "./scripts/lint.sh"

[env_template]
# DATABASE_URL = "postgres://user:pass@localhost:5432/db"
`, projectName, projectName, mcpBlock)

	} else {
		// Default template
		tomlContent = fmt.Sprintf(`# gandalf.toml - AI Agent Environment Declaration
version = "1.0"
name = "%s"
description = "Standardized AI agent environment for %s"
agents = ["claude-code", "codex", "cursor"]

# 1. Team MCP Servers
[mcp_servers.postgres-db]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres", "${DATABASE_URL}"]
required_env = ["DATABASE_URL"]
description = "Shared staging database MCP server"

# [mcp_servers.api-docs]
# url = "https://mcp.internal.myteam.com/sse"
# headers = { Authorization = "Bearer ${INTERNAL_API_KEY}" }
# required_env = ["INTERNAL_API_KEY"]

# 2. Team Skills (stored under .gandalf/skills/)
# [[skills]]
# name = "code-reviewer"
# source = "./.gandalf/skills/code-reviewer"
# description = "Automated PR review skill"

# 3. Agent Hooks
# [hooks.pre-save-lint]
# event = "before_save"
# command = "./scripts/lint.sh"

# 4. Required Environment Variables Template
[env_template]
DATABASE_URL = "postgres://user:password@localhost:5432/db"
`, projectName, projectName)
	}

	if err := fsutil.WriteTextAtomically(targetPath, tomlContent, 0644); err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "WRITE_MANIFEST_ERROR",
			Problem: "Failed to write gandalf.toml",
			Cause:   err.Error(),
		})
	}

	if flags.JSON {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"manifestPath": targetPath,
			"projectName":  projectName,
			"skillsDir":    skillsDir,
		})
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "🎉 Successfully initialized %s!\n", targetPath)
	_, _ = fmt.Fprintf(out, "📁 Team skills directory created: %s\n", skillsDir)
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Next steps:")
	_, _ = fmt.Fprintln(out, "  1. Edit 'gandalf.toml' to declare team MCP servers and skills.")
	_, _ = fmt.Fprintln(out, "  2. Commit 'gandalf.toml' and '.gandalf/skills/' to your Git repository.")
	_, _ = fmt.Fprintln(out, "  3. Team members can run 'gandalf apply' to sync their local agent setup.")

	return 0
}
