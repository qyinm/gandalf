import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { projectTarget, homeTarget } from "./index.js";

export const piAgentScanner: ScannerPlugin = {
  agentId: "pi-agent",
  agentName: "Pi Agent",
  description: "Pi coding agent configuration (settings, models, custom agents, project config)",

  targets(projectPath: string, homeDir: string): ScanTarget[] {
    const targets: ScanTarget[] = [];

    // Project-level: .pi/settings.json
    targets.push(
      projectTarget(projectPath, ".pi/settings.json", "pi-agent", "agent_config", "json", {
        sensitivity: "project_settings",
      }),
    );

    // User-level: agent runtime settings
    targets.push(
      homeTarget(homeDir, ".pi/agent/settings.json", "pi-agent", "agent_config", "json"),
      homeTarget(homeDir, ".pi/agent/models.json", "pi-agent", "agent_config", "json", {
        metadataOnly: true,
        sensitivity: "model_registry",
      }),
    );

    // User-level: custom agent definitions (markdown frontmatter + system prompt)
    targets.push(
      homeTarget(homeDir, ".pi/agents", "pi-agent", "agent_instruction", "filesystem", {
        directory: true,
      }),
    );

    return targets;
  },
};
