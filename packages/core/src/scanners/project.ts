import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { projectTarget } from "./index.js";

export const projectScanner: ScannerPlugin = {
  agentId: "project",
  agentName: "Project",
  description: "Project-level environment variable inventory",
  targets(projectPath: string, _homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, ".env", "project", "env_key", "dotenv", {
        sensitivity: "env_key_inventory",
        contentPolicy: "key_inventory_only"
      }),
    ];
  }
};
