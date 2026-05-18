import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { projectTarget, homeTarget } from "./index.js";

export const cursorScanner: ScannerPlugin = {
  agentId: "cursor",
  agentName: "Cursor",
  description: "Cursor editor MCP server configuration",
  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, ".cursor/mcp.json", "cursor", "agent_config", "json"),
      homeTarget(homeDir, ".cursor/mcp.json", "cursor", "agent_config", "json"),
    ];
  }
};
