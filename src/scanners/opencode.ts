import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { homeTarget } from "./index.js";

export const opencodeScanner: ScannerPlugin = {
  agentId: "opencode",
  agentName: "OpenCode",
  description: "OpenCode CLI configuration (MCP servers, plugins, providers, skills)",

  targets(_projectPath: string, homeDir: string): ScanTarget[] {
    return [
      // Main config: MCP servers, plugins, providers
      homeTarget(homeDir, ".config/opencode/opencode.json", "opencode", "agent_config", "json"),

      // Skills directory
      homeTarget(homeDir, ".config/opencode/skills", "opencode", "skill", "filesystem", {
        directory: true,
      }),
    ];
  },
};
