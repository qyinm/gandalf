import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { projectTarget, homeTarget } from "./index.js";

export const claudeCodeScanner: ScannerPlugin = {
  agentId: "claude-code",
  agentName: "Claude Code",
  description: "Claude Code agent configuration (prompts, MCP servers, settings, skills)",
  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, "CLAUDE.md", "claude-code", "agent_instruction", "markdown"),
      projectTarget(projectPath, ".mcp.json", "claude-code", "agent_config", "json"),
      projectTarget(projectPath, ".claude/settings.json", "claude-code", "agent_config", "json"),
      homeTarget(homeDir, ".claude/settings.json", "claude-code", "agent_config", "json"),
      homeTarget(homeDir, ".claude.json", "claude-code", "agent_config", "json", {
        metadataOnly: true,
        sensitivity: "metadata"
      }),
      homeTarget(homeDir, ".claude/agents", "claude-code", "unsupported", "filesystem", { directory: true }),
      homeTarget(homeDir, ".claude/skills", "claude-code", "skill", "filesystem", { directory: true }),
    ];
  }
};
