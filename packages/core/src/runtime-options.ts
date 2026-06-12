import type { AgentId, EvidenceScope } from "./types.js";

export interface RuntimeOptions {
  projectPath: string;
  homeDir: string;
  storeDir: string;
  agent?: AgentId;
  scope?: EvidenceScope;
  captureContent?: boolean;
}
