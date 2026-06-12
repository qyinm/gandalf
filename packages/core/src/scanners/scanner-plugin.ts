/**
 * Scanner Plugin Interface.
 * Each AI coding agent implements this to declare which files to scan.
 */
import type { AgentId, DiscoveredItem, DiscoveredItemConstruction, EvidenceScope } from "../types.js";
import type { ScanTarget } from "./index.js";

export interface ScannerContext {
  projectPath: string;
  homeDir: string;
  storeDir: string;
  explain?: boolean;
  scope?: EvidenceScope;
}

export interface ScannerPlugin {
  readonly agentId: AgentId;
  readonly agentName: string;
  readonly description: string;
  targets(projectPath: string, homeDir: string): ScanTarget[];
  scan?(context: ScannerContext): Promise<Array<DiscoveredItem | DiscoveredItemConstruction>>;
}
