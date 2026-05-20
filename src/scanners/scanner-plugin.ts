/**
 * Scanner Plugin Interface.
 * Each AI coding agent implements this to declare which files to scan.
 */
import type { AgentId } from "../types.js";
import type { DiscoveredItem } from "../types.js";
import type { ScanTarget } from "./index.js";

export interface ScannerContext {
  projectPath: string;
  homeDir: string;
  storeDir: string;
  explain?: boolean;
}

export interface ScannerPlugin {
  readonly agentId: AgentId;
  readonly agentName: string;
  readonly description: string;
  targets(projectPath: string, homeDir: string): ScanTarget[];
  scan?(context: ScannerContext): Promise<DiscoveredItem[]>;
}
