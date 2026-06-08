export type AgentId = "claude-code" | "codex" | "cursor" | "opencode" | "pi-agent" | "project" | "unknown";

export const EVIDENCE_KINDS = [
  "agent_config",
  "agent_instruction",
  "mcp_server",
  "permission",
  "skill",
  "extension",
  "env_key",
  "hook",
  "symlink",
  "unsupported"
] as const;

export type EvidenceKind = typeof EVIDENCE_KINDS[number];

export type RestorePolicy =
  | "full_content_supported"
  | "structured_fields_only"
  | "key_inventory_only"
  | "not_supported";

export type EvidenceScope = "user" | "project" | "managed" | "unknown";

export type CaptureStatus =
  | "captured"
  | "redacted"
  | "omitted"
  | "parse_failed"
  | "unsafe_to_export"
  | "unsupported";

export type Severity = "none" | "low" | "medium" | "high" | "critical";

export type EvidenceParser = "json" | "toml" | "markdown" | "dotenv" | "filesystem" | "unknown";
export type EvidenceConfidence = "low" | "medium" | "high";

export interface EvidenceMetadata {
  [key: string]: unknown;
}

export interface McpServerValue {
  command?: string;
  args?: string[];
  url?: string;
  type?: string;
  transport?: string;
  envKeys?: string[];
  disabled?: boolean;
  enabled?: boolean;
  [key: string]: unknown;
}

export interface PermissionValue {
  rule?: unknown;
  action?: string;
  [key: string]: unknown;
}

export interface HookValue {
  type?: string;
  command?: string;
  timeout?: number;
  matcher?: string;
  failClosed?: boolean;
  loop_limit?: number;
  [key: string]: unknown;
}

export interface EnvKeyValue {
  key?: string;
  [key: string]: unknown;
}

export interface SymlinkValue {
  target?: string;
  [key: string]: unknown;
}

export interface SkillMetadata extends EvidenceMetadata {
  builtIn?: boolean;
  description?: string;
  declaredName?: string;
  directoryName?: string;
  disableModelInvocation?: boolean;
  duplicateSources?: string[];
  entrypoint?: string;
  entrypointSizeBytes?: number;
  entrypointStatus?: string;
  executable?: boolean;
  nameMatchesDirectory?: boolean;
  realPath?: string;
  scopeRoot?: string;
  sourceRoot?: string;
}

export interface McpServerMetadata extends EvidenceMetadata {
  changed?: boolean;
  remote?: boolean;
  sourceRoot?: string;
  transport?: string;
}

export interface HookMetadata extends EvidenceMetadata {
  eventName?: string;
  executable?: boolean;
  hookCategory?: string;
  policyEvaluated?: boolean;
  sourcePriority?: number;
  sourceRoot?: string;
}

export interface UnsupportedMetadata extends EvidenceMetadata {
  error?: string;
  reason?: string;
  state?: string;
}

export interface SymlinkMetadata extends EvidenceMetadata {
  reason?: string;
  skipped?: boolean;
}

export interface DiscoveredItemBase<K extends EvidenceKind, V = unknown, M extends EvidenceMetadata = EvidenceMetadata> {
  id: string;
  agent: AgentId;
  kind: K;
  sourcePath: string;
  scope: EvidenceScope;
  precedence: number;
  parser: EvidenceParser;
  sensitivity: string;
  contentPolicy: string;
  restorePolicy: RestorePolicy;
  captureStatus: CaptureStatus;
  confidence: EvidenceConfidence;
  name?: string;
  value?: V;
  checksum?: string;
  metadata?: M;
}

export type DiscoveredItemConstruction = DiscoveredItemBase<EvidenceKind, unknown, EvidenceMetadata>;

export type AgentConfigItem = DiscoveredItemBase<"agent_config", unknown>;
export type AgentInstructionItem = DiscoveredItemBase<"agent_instruction", string | Record<string, unknown>>;
export type McpServerItem = DiscoveredItemBase<"mcp_server", McpServerValue, McpServerMetadata>;
export type PermissionItem = DiscoveredItemBase<"permission", PermissionValue>;
export type SkillItem = DiscoveredItemBase<"skill", unknown, SkillMetadata>;
export type ExtensionItem = DiscoveredItemBase<"extension", unknown, SkillMetadata>;
export type EnvKeyItem = DiscoveredItemBase<"env_key", EnvKeyValue>;
export type HookItem = DiscoveredItemBase<"hook", HookValue, HookMetadata>;
export type SymlinkItem = DiscoveredItemBase<"symlink", SymlinkValue, SymlinkMetadata>;
export type UnsupportedItem = DiscoveredItemBase<"unsupported", unknown, UnsupportedMetadata>;

export type DiscoveredItem =
  | AgentConfigItem
  | AgentInstructionItem
  | McpServerItem
  | PermissionItem
  | SkillItem
  | ExtensionItem
  | EnvKeyItem
  | HookItem
  | SymlinkItem
  | UnsupportedItem;

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

export type TimelineEntrySource = "daemon";
export type TimelineEntryEventKind = "baseline" | "setup_changed" | "unchanged";
export type TimelineRestoreReadiness = "full" | "partial" | "observe-only";
export type TimelineConfidence = "low" | "medium" | "high";
export type TimelineSurfaceKind = "mcp_server" | "skill" | "permission" | "hook" | "env_key" | "unsupported" | "other";

export interface TimelineChangeSummary {
  previousEntryId?: string;
  previousSnapshotName?: string;
  hasChanges: boolean;
  semanticChangeCount: number;
  rawSourceChangeCount: number;
  highlights: string[];
}

export interface TimelineChangedSurface {
  kind: TimelineSurfaceKind;
  changeType: string;
  path: string;
  entityName?: string;
  restorable: boolean;
  observeOnly: boolean;
  before?: unknown;
  after?: unknown;
}

export interface TimelineEntry {
  schemaVersion: "0.1";
  id: string;
  source: TimelineEntrySource;
  eventKind: TimelineEntryEventKind;
  title: string;
  projectPath: string;
  agent?: AgentId;
  agents: AgentId[];
  beforeSnapshotName?: string;
  afterSnapshotName: string;
  daemonRunId: string;
  createdAt: string;
  observedAt: string;
  changedSurfaces: TimelineChangedSurface[];
  restoreReadiness: TimelineRestoreReadiness;
  confidence: TimelineConfidence;
  confidenceReason: string;
  evidenceCount: number;
  graphNodeCount: number;
  auditFindingCount: number;
  changes: TimelineChangeSummary;
}

export interface TimelineCaptureResult {
  written: boolean;
  entry?: TimelineEntry;
  snapshot: Snapshot;
  reason?: "unchanged";
}

export interface DaemonStatus {
  running: boolean;
  pid?: number;
  pidAlive?: boolean;
  identityHash?: string;
  identityVerified: boolean;
  identityError?: string;
  startedAt?: string;
  lastHeartbeatAt?: string;
  lastEventAt?: string;
  runId?: string;
  projectPath: string;
  storeDir: string;
  watchedPaths: string[];
  stale: boolean;
  staleReason?: string;
  errors: string[];
}

export type DaemonStatusReadResult =
  | { ok: true; status: DaemonStatus }
  | { ok: false; error: string; status: DaemonStatus };

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

export type ReadinessCategory =
  | "ready"
  | "needs_manual_action"
  | "warning"
  | "unverified"
  | "unsupported"
  | "blocked";

export interface ReadinessAction {
  label: string;
  command?: string;
  url?: string;
}

export interface ReadinessItem {
  id: string;
  category: ReadinessCategory;
  severity: Severity;
  code: string;
  problem: string;
  cause: string;
  fix: string;
  path?: string;
  evidenceId?: string;
  command?: string;
  actions?: ReadinessAction[];
}

export interface ReadinessReport {
  targetPlatform: NodeJS.Platform;
  summary: Record<ReadinessCategory, number>;
  items: ReadinessItem[];
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

export type RestoreAction = "create" | "update" | "delete" | "skip" | "conflict" | "unsupported";

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
  /** Optional agent scope for snapshot lookup. */
  agent?: AgentId;
}

// ── Restore apply types (v0.2+ Phase-1) ──────────────────────────

export type RestoreItemStatus = "pending" | "applied" | "failed" | "skipped" | "unsupported";

export interface RestoreItem {
  /** Unique identifier linking back to the plan item */
  itemId: string;

  /** Target path/file/resource being restored */
  path: string;

  /** Type of restore item (e.g. claude, codex, mcp_server, skill, env) */
  type: string;

  /** Source reference path or identifier for the restore data */
  source: string;

  /** Destination path where the restore is applied */
  dest: string;

  /** Restore operation to execute. */
  action?: RestoreAction;

  /** Current execution status */
  status: RestoreItemStatus;

  /** Error message if execution failed */
  errorMessage?: string;

  /** Reason if skipped or unsupported */
  skipReason?: string;

  /** Execution order (1-based, derived from topological sort) */
  executionOrder: number;

  /** Previous state saved for potential rollback */
  rollbackState: Record<string, unknown> | null;

  /** Content to restore from the snapshot (from targetState.value) */
  targetContent?: unknown;

  /** Whether this item supports rollback */
  canRollback: boolean;

  /** Structured planner metadata needed by per-type handlers. */
  metadata?: Record<string, unknown>;

  /** ISO timestamp when the item was applied */
  applyAt?: string;
}

/** Options for `applyRestoreItems` execution loop */
export interface ApplyOptions {
  /** When true, abort execution on the first failure */
  failFast: boolean;
  /** Optional signal to abort execution */
  signal?: AbortSignal;
  /**
   * When true, trigger automatic rollback of applied items after
   * the apply loop completes. Uses the undoExecutor from RollbackOptions.
   */
  rollback?: boolean;
}

/** Summary of an apply execution */
export interface ApplySummary {
  total: number;
  successful: number;
  failed: number;
  skipped: number;
  unsupported: number;
  /** Per-item failure details */
  failures: Array<{ itemId: string; reason: string }>;
  /**
   * Ordered list of successfully-applied restore items (append-only).
   * Items are appended in execution order as they are applied.
   * Used as the authoritative source for rollback targeting.
   */
  appliedItems: RestoreItem[];
  /**
   * Mutable runtime registry mapping itemId → current status.
   * Accumulated inline during execution so the restore loop can query
   * any item's completion/failure state at any point without iterating
   * the full items array or summary structures.
   *
   * Populated on every status transition: pending → applied/failed/skipped.
   * Updated immediately in the apply loop — never stale.
   */
  statusRegistry: Record<string, RestoreItemStatus>;
}

/** Executor function signature for applying a single restore item */
export type RestoreExecutor = (item: RestoreItem) => Promise<void>;

// ── Rollback / undo types (v0.2+ Phase-1) ─────────────────────────

/** Executor function signature for undoing a single restore item's apply() side effects */
export type UndoExecutor = (item: RestoreItem) => Promise<void>;

export type UndoStatus = "undone" | "skipped" | "failed";

/** Per-item undo result */
export interface UndoResult {
  itemId: string;
  status: UndoStatus;
  reason?: string;
}

// ── Per-item undo handler types ────────────────────────────────

/** Per-item undo handler function — reverses a single item's apply() side effects */
export type UndoHandler = (item: RestoreItem) => Promise<void>;

/** Registry mapping item type strings to their undo handlers */
export type UndoHandlerRegistry = Record<string, UndoHandler>;

/** Executor function for applying a single restore item (mutates files) */
export type ApplyHandler = (item: RestoreItem) => Promise<void>;

/** Registry mapping item types to their apply handlers */
export type ApplyHandlerRegistry = Record<string, ApplyHandler>;

/** Summary of a rollback/undo execution */
export interface RollbackSummary {
  total: number;
  undone: number;
  skipped: number;
  failed: number;
  results: UndoResult[];
}

/**
 * Options for rolling back applied restore items.
 * Provides the undo executor and optional signal for cancellation.
 */
export interface RollbackOptions {
  /** The undo executor that reverses side effects for each item type */
  undoExecutor: UndoExecutor;
  /** Optional signal to abort rollback execution */
  signal?: AbortSignal;
  /**
   * When true, abort rollback execution on the first per-item failure.
   * Default: false (best-effort continuation — remaining items are still undone).
   */
  failFast?: boolean;
}

/**
 * Combined result of an apply-with-rollback execution.
 * Contains both the apply and rollback summaries when rollback is triggered.
 */
export interface ApplyWithRollbackResult {
  /** Summary of the apply execution */
  applySummary: ApplySummary;
  /** Summary of the rollback execution (undefined if rollback was not requested) */
  rollbackSummary?: RollbackSummary;
}

// ── Bundle types (v0.2+) ────────────────────────────────────────

/**
 * Bundle manifest stored in .hem/manifest.json inside every .hem archive.
 */
export interface BundleManifest {
  formatVersion: 1;
  snapshotName: string;
  createdAt: string;
  projectPath: string;
  includesContent: boolean;
  contentFileCount: number;
  contentTotalBytes: number;
  /** Information about the machine that created this bundle. */
  sourceMachine: SourceMachine;
  security: {
    rawSecretsIncluded: false;
    redactionPolicy: "metadata-only" | "structured_safe_fields_only";
    signed: boolean;
    signatureAlgorithm?: string;
    signature?: string;
  };
}

/**
 * Machine metadata captured at bundle export time.
 * Used during import to detect path/binary differences.
 */
export interface SourceMachine {
  homeDir: string;
  platform: NodeJS.Platform;
  hostname: string;
}

/**
 * Checksums for every tar entry inside a .hem bundle.
 * Stored in .hem/checksums.json.
 */
export interface BundleChecksums {
  algorithm: "SHA-256";
  entries: Record<string, string>; // tar entry path → hex digest
}

/**
 * Options for `bundleExport`.
 */
export interface BundleExportOptions {
  snapshotName: string;
  outputPath: string;
  storeDir: string;
  projectPath: string;
  homeDir: string;
  includeContent?: boolean;
  /** Optional HMAC key. Defaults to HEM_BUNDLE_KEY when present. */
  signatureKey?: string;
  /** Optional agent scope for snapshot lookup. */
  agent?: AgentId;
}

/**
 * Options for `bundleImport`.
 */
export interface BundleImportOptions {
  bundlePath: string;
  storeDir: string;
  projectPath: string;
  homeDir: string;
  applyContent?: boolean;
  dryRun?: boolean;
  /** Stage content under the snapshot store instead of applying it to target paths. */
  quarantine?: boolean;
  trust?: boolean;
  /** Optional HMAC key. Defaults to HEM_BUNDLE_KEY when present. */
  signatureKey?: string;
  /** Optional agent scope for snapshot storage on import. */
  agent?: AgentId;
  /** Test-only override for target platform. Defaults to os.platform(). */
  targetPlatform?: NodeJS.Platform;
}

/**
 * Result of a bundle import operation.
 */
export interface BundleImportResult {
  snapshotName: string;
  evidenceCount: number;
  includesContent: boolean;
  contentApplied: boolean;
  /** Directory where content was staged when quarantine mode is enabled. */
  quarantinedContentDir?: string;
  warnings: string[];
  /** Machine compatibility report (always present, even on dry-run). */
  machineDiff?: MachineDiff;
  /** Structured readiness preview shared by CLI, JSON, and TUI. */
  readiness: ReadinessReport;
}

/**
 * Cross-machine compatibility report.
 * Shows path remapping, binary availability, and OS differences.
 */
export interface MachineDiff {
  sourceHome: string;
  targetHome: string;
  sourcePlatform: string;
  targetPlatform: string;
  /** Hostname of the source machine that created the bundle. */
  sourceHostname: string;
  /** Hostname of the target machine running import. */
  targetHostname: string;
  /** True when source and target OS differ (e.g., darwin → linux). */
  crossOS: boolean;
  /** OS-specific differences between source and target. */
  osDifferences: string[];
  /** Paths that were remapped from source home to target home. */
  remappedPaths: string[];
  /** MCP binaries detected on source machine. */
  sourceMcpBinaries: McpBinaryInfo[];
  /** MCP binary availability on target machine. */
  mcpBinaryReport: McpBinaryReport[];
}

export interface McpBinaryInfo {
  evidenceId: string;
  command: string;
  args?: string[];
  url?: string;
  binaryKind?: "package_runner" | "source_local_path" | "path_binary" | "command" | "remote_url";
}

export interface McpBinaryReport {
  evidenceId: string;
  command: string;
  availableOnTarget: boolean;
  binaryKind?: "package_runner" | "source_local_path" | "path_binary" | "command" | "remote_url";
  resolvedPath?: string;
  warning?: string;
}

/**
 * Result of a bundle inspect operation.
 */
export interface BundleInspectResult {
  bundlePath: string;
  formatVersion: number;
  snapshotName: string;
  createdAt: string;
  projectPath: string;
  includesContent: boolean;
  contentFileCount: number;
  contentTotalBytes: number;
  checksumAlgorithm: string;
  bundleChecksum: string;
  isSigned: boolean;
  signatureAlgorithm?: string;
  /** Machine that created this bundle. */
  sourceMachine?: SourceMachine;
}

export interface BundleVerifyOptions {
  bundlePath: string;
  /** Optional HMAC key. Defaults to HEM_BUNDLE_KEY when present. */
  signatureKey?: string;
}

export interface BundleVerifyResult {
  bundlePath: string;
  valid: boolean;
  formatVersion?: number;
  snapshotName?: string;
  checksums: {
    checked: boolean;
    ok: boolean;
    entriesChecked: number;
  };
  signature: {
    signed: boolean;
    checked: boolean;
    ok: boolean;
    algorithm?: string;
  };
  warnings: string[];
  errors: string[];
}

// ── Tar types ───────────────────────────────────────────────────

/**
 * A single entry in a tar archive, held in memory.
 */
export interface TarEntry {
  /** Entry path inside the archive (POSIX, relative) */
  path: string;
  /** File content as Buffer */
  content: Buffer;
  /** File mode (default 0o644) */
  mode: number;
  /** File modification time (Unix timestamp, default Date.now()) */
  mtime: number;
  /** Entry type: 'file' or 'directory' */
  type: "file" | "directory";
}
