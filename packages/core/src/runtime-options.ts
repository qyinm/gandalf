import type { AgentId } from "./types.js";

export interface RuntimeOptions {
  projectPath: string;
  homeDir: string;
  storeDir: string;
  agent?: AgentId;
}
