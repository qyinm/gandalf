export type AgentId = "claude-code" | "codex" | "cursor" | "project" | "unknown";

export type EvidenceKind =
  | "agent_config"
  | "agent_instruction"
  | "mcp_server"
  | "permission"
  | "skill"
  | "env_key"
  | "hook"
  | "symlink"
  | "unsupported";

export type EvidenceScope = "user" | "project" | "managed" | "unknown";

export type CaptureStatus =
  | "captured"
  | "redacted"
  | "omitted"
  | "parse_failed"
  | "unsafe_to_export"
  | "unsupported";

export type Severity = "none" | "low" | "medium" | "high" | "critical";

export interface DiscoveredItem {
  id: string;
  agent: AgentId;
  kind: EvidenceKind;
  sourcePath: string;
  scope: EvidenceScope;
  precedence: number;
  parser: "json" | "toml" | "markdown" | "dotenv" | "filesystem" | "unknown";
  sensitivity: string;
  contentPolicy: string;
  restorePolicy: "not_supported_v0_1";
  captureStatus: CaptureStatus;
  confidence: "low" | "medium" | "high";
  name?: string;
  value?: unknown;
  checksum?: string;
  metadata?: Record<string, unknown>;
}

export interface SnapshotManifest {
  schemaVersion: "0.1";
  name: string;
  createdAt: string;
  projectPath: string;
  security: {
    rawSecretsIncluded: false;
    redactionPolicy: "metadata-only";
  };
}

export interface Snapshot {
  manifest: SnapshotManifest;
  evidence: DiscoveredItem[];
  graph: GraphNode[];
  auditFindings: AuditFinding[];
  provenance: ProvenanceEntry[];
}

export interface GraphNode {
  id: string;
  agent: AgentId;
  scope: EvidenceScope;
  sourcePath: string;
  entityKind: EvidenceKind;
  entityName: string;
  effectiveValue: unknown;
  overriddenBy?: string;
  confidence: "low" | "medium" | "high";
  evidenceId: string;
}

export interface AuditFinding {
  code: string;
  severity: Severity;
  problem: string;
  cause: string;
  fix: string;
  path?: string;
  evidenceId?: string;
}

export interface ProvenanceEntry {
  nodeId: string;
  evidenceId: string;
  sourcePath: string;
  scope: EvidenceScope;
  precedence: number;
  confidence: "low" | "medium" | "high";
  captureStatus: CaptureStatus;
}

export interface ScanOptions {
  projectPath: string;
  homeDir: string;
  storeDir: string;
  explain?: boolean;
}

// ── Restore dry-run types (v0.2) ──────────────────────────────

export type RestoreAction = "create" | "update" | "skip" | "conflict" | "unsupported";

export interface ItemDiff {
  changes: string[];
  additions: string[];
  removals: string[];
}

export interface RestorePlanItem {
  itemId: string;
  agent: AgentId;
  kind: EvidenceKind;
  sourcePath: string;
  dependsOn: string[];
  action: RestoreAction;
  currentState: DiscoveredItem | null;
  targetState: DiscoveredItem | null;
  diff: ItemDiff;
  riskLevel: Severity;
  riskReason: string;
  needsConfirmation: boolean;
  confirmationPrompt: string;
  rollbackInstruction: string;
}

export interface UnsupportedPlanItem {
  itemId: string;
  agent: AgentId;
  kind: EvidenceKind;
  sourcePath: string;
  reason: string;
}

export interface RollbackStep {
  itemId: string;
  action: string;
  instruction: string;
}

export interface RollbackPlan {
  steps: RollbackStep[];
}

export interface RiskSummary {
  none: number;
  low: number;
  medium: number;
  high: number;
  critical: number;
}

export interface RestorePlanMetadata {
  plannerVersion: string;
  generatedBy: string;
}

export interface RestorePlan {
  planId: string;
  sourceSnapshot: string;
  targetProject: string;
  createdAt: string;
  itemCount: number;
  riskSummary: RiskSummary;
  items: RestorePlanItem[];
  rollbackPlan: RollbackPlan;
  executionOrder: string[];
  unsupportedItems: UnsupportedPlanItem[];
  planMetadata: RestorePlanMetadata;
}

export interface RestoreOptions {
  sourceSnapshot: string;
  projectPath: string;
  homeDir: string;
  storeDir: string;
  dryRun: boolean;
}
