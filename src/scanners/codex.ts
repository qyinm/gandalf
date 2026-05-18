import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { projectTarget, homeTarget } from "./index.js";

export const codexScanner: ScannerPlugin = {
  agentId: "codex",
  agentName: "Codex",
  description: "Codex agent configuration (prompts, config, skills)",
  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, "AGENTS.md", "codex", "agent_instruction", "markdown"),
      projectTarget(projectPath, ".codex", "codex", "unsupported", "filesystem", { directory: true }),
      homeTarget(homeDir, ".codex/config.toml", "codex", "agent_config", "toml"),
      homeTarget(homeDir, ".codex/skills", "codex", "skill", "filesystem", { directory: true }),
    ];
  }
};
