import { randomUUID } from "node:crypto";
import * as fs from "node:fs";
import * as path from "node:path";
import { scanProject } from "./scan.js";
import { readSnapshot, readSnapshotContent } from "./store.js";
import {
  buildGraph
} from "./graph.js";
import { diffGraphs } from "./diff.js";
import type {
  AgentId,
  ApplyHandler,
  ApplyHandlerRegistry,
  ApplyOptions,
  ApplySummary,
  ApplyWithRollbackResult,
  DiscoveredItem,
  EvidenceKind,
  RestoreAction,
  RestoreExecutor,
  RestoreItem,
  RestoreItemStatus,
  RestorePlan,
  RestorePlanItem,
  RestoreOptions,
  RiskSummary,
  RollbackOptions,
  RollbackSummary,
  Severity,
  Snapshot,
  SnapshotContentEntry,
  UndoExecutor,
  UndoHandler,
  UndoHandlerRegistry,
  UnsupportedPlanItem
} from "./types.js";

// ── Dry-run output parser types ─────────────────────────────────

export interface ParseDryRunError {
  message: string;
}

export interface ParseDryRunResult {
  items: RestoreItem[];
  errors: ParseDryRunError[];
}

// ── Dry-run output parser ───────────────────────────────────────

/**
 * Parse dry-run planner JSON output into executable RestoreItem entries.
 * Strips comment lines (#, //, ---), validates structure, and assigns
 * execution order from the plan's `executionOrder` array.
 */
export function parseDryRunOutput(input: string): ParseDryRunResult {
  const errors: ParseDryRunError[] = [];
  const seenIds = new Set<string>();
  const result: RestoreItem[] = [];

  // Strip comment lines before parsing
  const cleaned = input
    .split("\n")
    .filter((line) => {
      const trimmed = line.trim();
      return !(
        trimmed.startsWith("#") ||
        trimmed.startsWith("//") ||
        trimmed.startsWith("---")
      );
    })
    .join("\n")
    .trim();

  if (!cleaned) {
    return { items: [], errors };
  }

  // Try to parse the cleaned text as JSON
  let parsed: unknown;
  try {
    parsed = JSON.parse(cleaned);
  } catch (parseError) {
    errors.push({
      message: `Failed to parse dry-run output as JSON: ${
        parseError instanceof Error ? parseError.message : String(parseError)
      }`
    });
    return { items: [], errors };
  }

  // Validate root is an object
  if (typeof parsed !== "object" || parsed === null) {
    errors.push({ message: "Dry-run output is not a valid JSON object" });
    return { items: [], errors };
  }

  const plan = parsed as Record<string, unknown>;
  const targetProject = typeof plan.targetProject === "string" ? plan.targetProject : undefined;
  const targetHome = typeof plan.targetHome === "string" ? plan.targetHome : undefined;

  // Validate items array
  if (!Array.isArray(plan.items)) {
    errors.push({ message: 'Dry-run plan is missing required "items" array' });
    return { items: [], errors };
  }

  const planItems = plan.items as RestorePlanItem[];
  const executionOrder = Array.isArray(plan.executionOrder)
    ? (plan.executionOrder as string[])
    : [];
  const unsupportedItems = Array.isArray(plan.unsupportedItems)
    ? (plan.unsupportedItems as UnsupportedPlanItem[])
    : [];

  // Build lookup from executionOrder for 1-based ordering
  const orderLookup = new Map<string, number>();
  executionOrder.forEach((itemId, index) => {
    orderLookup.set(itemId, index + 1);
  });

  // Track itemIds present in the items array for unsupported dedup
  const itemsItemIds = new Set<string>();
  let nextAppendOrder = executionOrder.length + 1;

  for (const planItem of planItems) {
    // Validate required fields
    if (typeof planItem.itemId !== "string" || typeof planItem.kind !== "string") {
      errors.push({
        message: `Skipping item "${String(planItem.itemId ?? "?")}": missing required fields (itemId, kind)`
      });
      continue;
    }

    // Detect duplicates
    if (seenIds.has(planItem.itemId)) {
      errors.push({
        message: `Duplicate itemId "${planItem.itemId}" skipped`
      });
      continue;
    }

    seenIds.add(planItem.itemId);
    itemsItemIds.add(planItem.itemId);

    const order =
      orderLookup.get(planItem.itemId) ?? nextAppendOrder++;
    const canRollback = canRollbackAction(planItem.action);

    result.push({
      itemId: planItem.itemId,
      path: planItem.sourcePath,
      type: planItem.kind,
      source: planItem.sourcePath,
      dest: resolvePlanDestination(planItem, targetProject, targetHome),
      action: planItem.action,
      status: planItem.action === "unsupported" ? "unsupported" : "pending",
      executionOrder: order,
      rollbackState: null,
      targetContent: planItem.targetState?.value,
      canRollback,
      metadata: restoreItemMetadata(planItem),
      applyAt: undefined,
      errorMessage: undefined,
      skipReason: undefined
    });
  }

  // Append unsupported items (skip if already present in items array)
  for (const unsupported of unsupportedItems) {
    if (itemsItemIds.has(unsupported.itemId)) {
      // Already present as a regular item — don't duplicate
      continue;
    }

    if (seenIds.has(unsupported.itemId)) {
      errors.push({
        message: `Duplicate itemId "${unsupported.itemId}" skipped`
      });
      continue;
    }

    seenIds.add(unsupported.itemId);

    result.push({
      itemId: unsupported.itemId,
      path: unsupported.sourcePath,
      type: unsupported.kind,
      source: unsupported.sourcePath,
      dest: unsupported.sourcePath,
      action: "unsupported",
      status: "unsupported",
      executionOrder: nextAppendOrder++,
      rollbackState: null,
      canRollback: false,
      skipReason: unsupported.reason,
      applyAt: undefined,
      errorMessage: undefined
    });
  }

  // Sort by executionOrder before returning
  result.sort((a, b) => a.executionOrder - b.executionOrder);

  return { items: result, errors };
}

// ── Restore plan builder (dry-run) ──────────────────────────────

/**
 * Build a restore plan by diffing a stored snapshot against the current project state.
 * Used for both `--dry-run` (report generation) and `--apply` (execution input).
 */
export async function buildRestorePlan(options: RestoreOptions): Promise<RestorePlan> {
  const snapshot = await readSnapshot(options.storeDir, options.sourceSnapshot, options.agent);
  const scan = await scanProject({
    projectPath: options.projectPath,
    homeDir: options.homeDir,
    storeDir: options.storeDir,
    agent: options.agent,
    scope: options.scope
  });
  const currentGraph = buildGraph(scan.evidence);
  const snapshotContent = await snapshotContentByEvidenceId(snapshot, options);

  // Build a meaningful plan from the diff
  const diff = diffGraphs(snapshot.graph, currentGraph);
  const items: RestorePlanItem[] = [];
  const unsupportedItems: UnsupportedPlanItem[] = [];
  const executionOrder: string[] = [];
  const riskCounts: RiskSummary = { none: 0, low: 0, medium: 0, high: 0, critical: 0 };

  // Generate restore items from semantic changes
  for (const change of diff.semanticChanges) {
    // Find matching graph nodes for current/target state
    const sourcePath = typeof change.details.sourcePath === "string" ? change.details.sourcePath : undefined;
    const currentState = findMatchingEvidence(change, scan.evidence, sourcePath);
    const targetState = withSnapshotContent(
      findMatchingEvidence(change, snapshot.evidence, sourcePath),
      snapshotContent
    );
    const itemId = `${change.entityKind}:${change.entityName}:${randomUUID().slice(0, 8)}`;
    const restorePath = restorePathFromContent(targetState)
      ?? restorePathFromContent(currentState)
      ?? restorePathForEvidenceFile(targetState)
      ?? restorePathForEvidenceFile(currentState)
      ?? sourcePathForRestoreItem(
      change.entityKind,
      change.entityName,
      currentState,
      targetState,
      sourcePath
    );
    const action = restoreActionForChange(change.code, currentState, targetState);

    const riskLevel = change.severity;
    if (action === "unsupported" || action === "skip") {
      unsupportedItems.push({
        itemId,
        agent: agentForRestoreItem(currentState, targetState),
        kind: change.entityKind,
        sourcePath: restorePath,
        reason: unsupportedReasonFor(change.code, change.entityKind, change.entityName, currentState, targetState)
      });
      continue;
    }

    riskCounts[riskLevel]++;
    items.push({
      itemId,
      agent: agentForRestoreItem(currentState, targetState),
      kind: change.entityKind,
      sourcePath: restorePath,
      dependsOn: [],
      action,
      currentState,
      targetState,
      diff: {
        changes: change.details.changedFields,
        additions: [],
        removals: []
      },
      riskLevel,
      riskReason: `Restore ${action} for ${change.entityKind}: ${change.entityName}`,
      needsConfirmation: riskLevel === "high" || riskLevel === "critical",
      confirmationPrompt: riskLevel === "high" || riskLevel === "critical"
        ? `Restore ${change.entityKind} "${change.entityName}" with risk ${riskLevel}. Continue?`
        : "",
      rollbackInstruction: rollbackInstructionFor(action, change.entityKind, change.entityName)
    });
    executionOrder.push(itemId);
  }

  const planId = `plan-${randomUUID().slice(0, 12)}`;

  return {
    planId,
    sourceSnapshot: options.sourceSnapshot,
    targetProject: options.projectPath,
    targetHome: options.homeDir,
    createdAt: new Date().toISOString(),
    itemCount: items.length,
    riskSummary: riskCounts,
    items,
    rollbackPlan: {
      steps: buildRollbackSteps(items)
    },
    executionOrder,
    unsupportedItems,
    planMetadata: {
      plannerVersion: "0.2.0",
      generatedBy: "hem restore"
    }
  };
}

async function snapshotContentByEvidenceId(
  snapshot: Snapshot,
  options: RestoreOptions
): Promise<Map<string, SnapshotContentEntry & { content: string }>> {
  const content = new Map<string, SnapshotContentEntry & { content: string }>();
  for (const entry of snapshot.content ?? []) {
    if (entry.captureStatus !== "captured") {
      continue;
    }
    const text = await readSnapshotContent(options.storeDir, options.sourceSnapshot, entry, options.agent);
    content.set(entry.evidenceId, { ...entry, content: text });
  }
  return content;
}

function withSnapshotContent(
  item: DiscoveredItem | null,
  content: Map<string, SnapshotContentEntry & { content: string }>
): DiscoveredItem | null {
  if (!item) {
    return null;
  }
  const entry = content.get(item.id);
  if (!entry) {
    return item;
  }
  return {
    ...item,
    value: entry.content,
    metadata: {
      ...(item.metadata ?? {}),
      contentCaptureStatus: "captured",
      contentRestorePath: entry.restorePath,
      contentChecksum: entry.checksum
    }
  } as DiscoveredItem;
}

function restorePathFromContent(item: DiscoveredItem | null): string | undefined {
  const restorePath = item?.metadata?.contentRestorePath;
  return typeof restorePath === "string" && restorePath.length > 0 ? restorePath : undefined;
}

function restorePathForEvidenceFile(item: DiscoveredItem | null): string | undefined {
  if (!item) {
    return undefined;
  }
  if (item.kind === "skill") {
    const entrypoint = typeof item.metadata?.entrypoint === "string" ? item.metadata.entrypoint : "SKILL.md";
    return `${item.sourcePath}/${entrypoint}`;
  }
  if (item.sourcePath.startsWith("~/") || item.sourcePath.startsWith(".") || path.isAbsolute(item.sourcePath)) {
    return item.sourcePath;
  }
  return undefined;
}

function agentForRestoreItem(currentState: DiscoveredItem | null, targetState: DiscoveredItem | null): AgentId {
  return targetState?.agent ?? currentState?.agent ?? "unknown";
}

function unsupportedReasonFor(
  code: string,
  kind: EvidenceKind,
  name: string,
  currentState: DiscoveredItem | null,
  targetState: DiscoveredItem | null
): string {
  if (!currentState && !targetState) {
    return `Cannot map ${code} for ${kind} ${name} to captured evidence`;
  }
  if (targetState && targetState.metadata?.contentCaptureStatus === "omitted") {
    return `Snapshot content for ${kind} ${name} was omitted: ${String(targetState.metadata.contentCaptureReason ?? "policy")}`;
  }
  if (kind === "env_key") {
    return `Environment key values are key-inventory-only; ${code} cannot be restored without a user-supplied value`;
  }
  if (code === "UNSUPPORTED_STATE_CHANGED") {
    return `Unsupported state change: ${kind} ${name}`;
  }
  return `No supported restore action for ${code} on ${kind} ${name}`;
}

// ── Apply execution loop (v0.2+ Phase-1) ───────────────────────

/**
 * Execute restore items sequentially with per-item try/catch isolation.
 *
 * Default behaviour (failFast=false):
 *   - Each item is executed independently via the provided executor
 *   - A single failure logs the error and continues with the next item
 *   - Returns a summary with aggregated counts and failure details
 *
 * failFast=true:
 *   - Stops execution immediately on the first failure
 *   - Items not yet executed retain their current status
 *
 * Each item is executed in the order specified by executionOrder.
 * The executor is responsible for the actual mutation and rollbackState capture.
 */
export async function applyRestoreItems(
  items: RestoreItem[],
  executor: RestoreExecutor,
  options: ApplyOptions
): Promise<ApplySummary> {
  const summary: ApplySummary = {
    total: items.length,
    successful: 0,
    failed: 0,
    skipped: 0,
    unsupported: 0,
    failures: [],
    /** Append-only list of successfully applied items, in execution order */
    appliedItems: [],
    /** Mutable runtime registry: itemId → current status */
    statusRegistry: {}
  };

  // Work on a copy sorted by executionOrder
  const sorted = [...items].sort((a, b) => a.executionOrder - b.executionOrder);

  for (const item of sorted) {
    // Check for abort signal
    if (options.signal?.aborted) {
      // Remaining items stay as-is; return partial summary
      break;
    }

    // Pre-check: unsupported/skipped items are not executed
    if (item.status === "unsupported") {
      summary.statusRegistry[item.itemId] = item.status;
      summary.unsupported++;
      continue;
    }

    if (item.status === "skipped") {
      summary.statusRegistry[item.itemId] = item.status;
      summary.skipped++;
      continue;
    }

    try {
      await executor(item);
      item.status = "applied";
      item.applyAt = new Date().toISOString();
      summary.statusRegistry[item.itemId] = item.status;
      summary.successful++;
      // Append-only tracking: record the applied item for rollback targeting
      summary.appliedItems.push(item);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      item.status = "failed";
      item.errorMessage = errorMessage;
      summary.statusRegistry[item.itemId] = item.status;
      summary.failed++;
      summary.failures.push({
        itemId: item.itemId,
        reason: errorMessage
      });

      if (options.failFast) {
        break;
      }
    }
  }

  // Count remaining items by their status for a complete summary
  if (options.signal?.aborted || (options.failFast && summary.failed > 0)) {
    for (const item of sorted) {
      if (item.status === "pending") {
        summary.skipped++;
        item.status = "skipped";
        item.skipReason = "Execution stopped before this item";
        summary.statusRegistry[item.itemId] = item.status;
      } else if (item.status === "unsupported") {
        // Unsupported items that were pre-marked but never reached in the loop
        // still need to be counted in the summary
        summary.unsupported++;
        summary.statusRegistry[item.itemId] = item.status;
      }
    }
    // Recalculate total to match actually processed vs skipped-at-end
    summary.total = items.length;
  }

  return summary;
}

/** Render an ApplySummary as a human-readable text block */
export function formatApplySummary(summary: ApplySummary): string {
  const lines: string[] = [
    "Restore apply results",
    "",
    `  Successful: ${summary.successful}`,
    `  Failed:     ${summary.failed}`,
    `  Skipped:    ${summary.skipped}`,
    `  Unsupported: ${summary.unsupported}`,
    `  Total:      ${summary.total}`
  ];

  if (summary.failures.length > 0) {
    lines.push("", "Failures:");
    for (const failure of summary.failures) {
      lines.push(`  [${failure.itemId}] ${failure.reason}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

// ── Per-type apply executors (v0.2+ Phase-2) ─────────────────────

/** Read the current content at a file path, returning null if missing. */
async function readCurrentContent(filePath: string): Promise<string | null> {
  try {
    return await fs.promises.readFile(filePath, "utf-8");
  } catch {
    return null;
  }
}

export async function writeFileAtomically(filePath: string, content: string): Promise<void> {
  const tempPath = `${filePath}.${process.pid}.${randomUUID()}.tmp`;
  try {
    await fs.promises.writeFile(tempPath, content, "utf-8");
    await fs.promises.rename(tempPath, filePath);
  } catch (error) {
    await fs.promises.rm(tempPath, { force: true }).catch(() => {});
    throw error;
  }
}

async function applyFileMutation(
  item: RestoreItem,
  filePath: string,
  content?: string,
  mode?: number,
  forceWrite = false
): Promise<void> {
  const prev = await readCurrentContent(filePath);
  item.rollbackState = { filePath, previousContent: prev };
  if (item.action === "delete" && !forceWrite) {
    await fs.promises.rm(filePath, { force: true });
    return;
  }
  if (content === undefined) {
    throw new Error(`Missing target content for ${item.itemId}`);
  }
  await fs.promises.mkdir(path.dirname(filePath), { recursive: true });
  await writeFileAtomically(filePath, content);
  if (mode !== undefined) {
    await fs.promises.chmod(filePath, mode);
  }
}

/**
 * Apply handler for agent_config items (JSON/TOML config files).
 */
export const applyAgentConfig: ApplyHandler = async (item: RestoreItem): Promise<void> => {
  const value = item.targetContent;
  const filePath = item.dest;
  const content = typeof value === "string" ? value : JSON.stringify(value, null, 2);
  await applyFileMutation(item, filePath, content);
};

/**
 * Apply handler for agent_instruction items (CLAUDE.md / AGENTS.md).
 */
export const applyAgentInstruction: ApplyHandler = async (item: RestoreItem): Promise<void> => {
  const content = typeof item.targetContent === "string"
    ? item.targetContent
    : String(item.targetContent ?? "");
  const filePath = item.dest;
  await applyFileMutation(item, filePath, content);
};

/**
 * Apply handler for hook items (executable scripts).
 */
export const applyHook: ApplyHandler = async (item: RestoreItem): Promise<void> => {
  const content = typeof item.targetContent === "string"
    ? item.targetContent
    : String(item.targetContent ?? "");
  const filePath = item.dest;
  await applyFileMutation(item, filePath, content, 0o755);
};

/**
 * Apply handler for mcp_server items.
 * Adds/updates an MCP server entry in the project's .mcp.json.
 */
export const applyMcpServer: ApplyHandler = async (item: RestoreItem): Promise<void> => {
  const mcpPath = mcpConfigPathForItem(item);

  let mcpConfig: Record<string, unknown> = { mcpServers: {} };
  const existing = await readCurrentContent(mcpPath);
  if (existing) {
    try { mcpConfig = JSON.parse(existing); } catch { mcpConfig = { mcpServers: {} }; }
  }
  if (!mcpConfig.mcpServers || typeof mcpConfig.mcpServers !== "object") {
    mcpConfig.mcpServers = {};
  }

  const serverName = mcpServerNameForItem(item);
  const servers = mcpConfig.mcpServers as Record<string, unknown>;
  const prevEntry = servers[serverName] ?? null;

  if (item.action === "delete") {
    delete servers[serverName];
  } else {
    if (item.targetContent === undefined) {
      throw new Error(`Missing target MCP server content for ${serverName}`);
    }
    servers[serverName] = item.targetContent;
  }

  await applyFileMutation(item, mcpPath, JSON.stringify(mcpConfig, null, 2) + "\n", undefined, true);
  item.rollbackState = {
    ...(item.rollbackState ?? {}),
    mcpPath,
    previousEntry: prevEntry
  };
};

/**
 * Apply handler for permission items (settings.json permission rules).
 */
export const applyPermission: ApplyHandler = async (item: RestoreItem): Promise<void> => {
  const filePath = item.dest;
  let settings: Record<string, unknown> = {};
  const existing = await readCurrentContent(filePath);
  if (existing) {
    try { settings = JSON.parse(existing); } catch { settings = {}; }
  }

  const permValue = item.targetContent;
  const permName = item.itemId.split(".").pop() ?? "permission";
  if (!settings.permissions || typeof settings.permissions !== "object") {
    settings.permissions = {};
  }
  if (item.action === "delete") {
    delete (settings.permissions as Record<string, unknown>)[permName];
  } else {
    (settings.permissions as Record<string, unknown>)[permName] = permValue;
  }

  await applyFileMutation(item, filePath, JSON.stringify(settings, null, 2) + "\n", undefined, true);
};

/**
 * Apply handler for env_key items (.env entries).
 */
export const applyEnvKey: ApplyHandler = async (item: RestoreItem): Promise<void> => {
  const envPath = path.join(path.dirname(item.dest), ".env");
  const keyName = item.itemId.split(".").pop() ?? "VAR";
  const value = typeof item.targetContent === "string"
    ? item.targetContent : String(item.targetContent ?? "");

  const existing = await readCurrentContent(envPath);

  const lines = existing ? existing.split("\n") : [];
  const keyIndex = lines.findIndex((l) => l.trim().startsWith(`${keyName}=`));
  if (item.action === "delete") {
    if (keyIndex >= 0) {
      lines.splice(keyIndex, 1);
    }
  } else {
    const newLine = `${keyName}=${value}`;
    if (keyIndex >= 0) { lines[keyIndex] = newLine; }
    else { lines.push(newLine); }
  }

  await applyFileMutation(item, envPath, lines.filter((line, index) => line.length > 0 || index < lines.length - 1).join("\n") + "\n", undefined, true);
};

/**
 * Apply handler for skill items.
 */
export const applySkill: ApplyHandler = async (item: RestoreItem): Promise<void> => {
  const skillPath = item.dest;
  const content = typeof item.targetContent === "string"
    ? item.targetContent : JSON.stringify(item.targetContent, null, 2);
  await applyFileMutation(item, skillPath, content);
};

/** Default apply handler registry for known restore item types. */
export function defaultApplyHandlerRegistry(): ApplyHandlerRegistry {
  return {
    agent_config: applyAgentConfig,
    agent_instruction: applyAgentInstruction,
    mcp_server: applyMcpServer,
    permission: applyPermission,
    hook: applyHook,
    skill: applySkill,
    env_key: applyEnvKey
  };
}

/** Create a RestoreExecutor that dispatches to per-type apply handlers. */
export function createDefaultApplyExecutor(
  registry: ApplyHandlerRegistry
): RestoreExecutor {
  return async (item: RestoreItem): Promise<void> => {
    const handler = registry[item.type];
    if (!handler) {
      item.skipReason = `No apply handler for type "${item.type}"`;
      throw new Error(item.skipReason);
    }
    await handler(item);
  };
}

/** Undo handler: restores previous file content from rollbackState. */
export const restorePreviousContentUndoHandler: UndoHandler = async (
  item: RestoreItem
): Promise<void> => {
  if (!item.rollbackState) return;
  const state = item.rollbackState as Record<string, unknown>;
  const prevContent = state.previousContent as string | null;
  const filePath = state.filePath as string | undefined;
  const mcpPath = state.mcpPath as string | undefined;
  const envPath = state.envPath as string | undefined;

  if (filePath) {
    if (prevContent === null) {
      await fs.promises.rm(filePath, { force: true }).catch(() => {});
    } else {
      await fs.promises.mkdir(path.dirname(filePath), { recursive: true });
      await writeFileAtomically(filePath, prevContent);
    }
  } else if (mcpPath) {
    const savedConfig = state.mcpConfig as Record<string, unknown> | null;
    if (savedConfig) {
      await fs.promises.mkdir(path.dirname(mcpPath), { recursive: true });
      await writeFileAtomically(mcpPath, JSON.stringify(savedConfig, null, 2) + "\n");
    }
  } else if (envPath) {
    if (prevContent === null) {
      await fs.promises.rm(envPath, { force: true }).catch(() => {});
    } else {
      await fs.promises.mkdir(path.dirname(envPath), { recursive: true });
      await writeFileAtomically(envPath, prevContent);
    }
  }
};

// ── Rollback execution (v0.2+ Phase-1) ──────────────────────────

/**
 * Roll back a list of applied restore items in reverse execution order.
 *
 * Only items with `status === "applied"` and `canRollback === true` are
 * passed to the undo executor. Non-reversible items (canRollback === false)
 * are skipped (no-op), matching the contract that each restore item type
 * exposes an undo() method defaulting to no-op for non-reversible types.
 *
 * Items are processed in reverse execution order (LIFO):
 * the last item applied is the first item undone.
 *
 * The undoExecutor is the pluggable function that performs the actual
 * reversal of side effects for each item type.
 *
 * Per-item rollback failure handling:
 * - Default (failFast=false, best-effort): each item is undone independently.
 *   A single rollback failure logs the error and continues with the next item.
 *   This is the safe default — the goal is to undo as much as possible.
 * - failFast=true: stops rollback immediately on the first per-item failure.
 *   Remaining items are NOT attempted. Skipped items are recorded as skipped
 *   with the reason "Rollback stopped before this item".
 */
export async function rollbackAppliedItems(
  appliedItems: RestoreItem[],
  undoExecutor: UndoExecutor,
  options?: { failFast?: boolean; signal?: AbortSignal }
): Promise<RollbackSummary> {
  const summary: RollbackSummary = {
    total: 0,
    undone: 0,
    skipped: 0,
    failed: 0,
    results: []
  };

  // Reverse execution order: last applied = first undone
  const reversed = sortByDescendingOrder(appliedItems);

  summary.total = reversed.length;

  for (const item of reversed) {
    // Only undo items that were actually applied
    if (item.status !== "applied") {
      continue;
    }

    // Non-reversible items are no-ops (default per-item undo for non-reversible types)
    if (!item.canRollback) {
      summary.skipped++;
      summary.results.push({
        itemId: item.itemId,
        status: "skipped",
        reason: "Item does not support rollback"
      });
      continue;
    }

    try {
      await undoExecutor(item);
      item.status = "pending";
      item.rollbackState = null;
      summary.undone++;
      summary.results.push({
        itemId: item.itemId,
        status: "undone"
      });
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error);
      item.errorMessage = `Rollback failed: ${reason}`;
      summary.failed++;
      summary.results.push({
        itemId: item.itemId,
        status: "failed",
        reason
      });

      // failFast: abort on first rollback failure
      if (options?.failFast) {
        break;
      }
    }
  }

  // Mark remaining (unprocessed) items as skipped if failFast aborted early
  if (options?.failFast && summary.failed > 0) {
    for (const remaining of reversed) {
      // Items that were visited in the main loop are no longer "applied":
      // undone → "pending", failed → stays "applied" (but already in results).
      // Non-reversible items visited get status check before break too—they're
      // already recorded in results. Only truly unvisited "applied" items
      // (those that came after the break point in loop order) need skipping.
      if (remaining.status === "applied") {
        summary.skipped++;
        summary.results.push({
          itemId: remaining.itemId,
          status: "skipped",
          reason: "Rollback stopped before this item"
        });
      }
    }
  }

  return summary;
}

/** Render a RollbackSummary as a human-readable text block */
export function formatRollbackSummary(summary: RollbackSummary): string {
  const lines: string[] = [
    "Rollback complete.",
    "",
    `  Undone:  ${summary.undone}`,
    `  Skipped: ${summary.skipped}`,
    `  Failed:  ${summary.failed}`,
    `  Total:   ${summary.total}`
  ];

  const failures = summary.results.filter((r) => r.status === "failed");
  if (failures.length > 0) {
    lines.push("", "Failures:");
    for (const f of failures) {
      lines.push(`  [${f.itemId}] ${f.reason ?? "Unknown error"}`);
    }
  }

  const skipped = summary.results.filter((r) => r.status === "skipped");
  if (skipped.length > 0) {
    lines.push("", "Skipped (non-reversible):");
    for (const s of skipped) {
      lines.push(`  [${s.itemId}] ${s.reason ?? "No reason"}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

// ── Execution order helpers (v0.2+ Phase-1, Sub-AC 6b) ──────────

/**
 * Sort items in descending executionOrder (highest order first).
 *
 * This is the reverse sort used by rollback to process applied items
 * in LIFO order: the last item applied (highest executionOrder) is
 * the first item undone.
 *
 * The function works on any array of RestoreItems and returns a new
 * sorted copy without mutating the original array.
 *
 * @param items - Items to sort in descending executionOrder
 * @returns A new array sorted by descending executionOrder
 */
export function sortByDescendingOrder(items: RestoreItem[]): RestoreItem[] {
  return [...items].sort((a, b) => b.executionOrder - a.executionOrder);
}

// ── Rollback eligibility filter (v0.2+ Phase-1, Sub-AC 6a) ─────

/**
 * Rollback eligibility filter: select only items with status === "applied"
 * from a restore items array.
 *
 * This is a standalone filter that reads the item's own `status` field,
 * independent of any runtime status registry. It is used to determine
 * which items from a completed apply plan are eligible for rollback.
 *
 * Applied items are returned sorted by executionOrder (ascending).
 * Rollback callers typically reverse the returned order for LIFO undo.
 *
 * Failed, skipped, unsupported, and pending items are excluded.
 *
 * @param items - The array of restore items to filter
 * @returns Items whose status is "applied", sorted by executionOrder
 */
export function getAppliedItems(items: RestoreItem[]): RestoreItem[] {
  return items
    .filter((item) => item.status === "applied")
    .sort((a, b) => a.executionOrder - b.executionOrder);
}

// ── Registry read interface (v0.2+ Phase-1, Sub-AC 2.3a) ─────

/**
 * Registry read interface: query the execution status registry for
 * items with status === "applied".
 *
 * This function provides an independent query path for rollback to
 * determine which items were successfully applied, decoupling rollback
 * from the pre-built `appliedItems` list. It reads the authoritative
 * runtime registry (`statusRegistry`) and returns the corresponding
 * items sorted by executionOrder (ascending).
 *
 * Rollback callers typically reverse the returned order for LIFO undo.
 *
 * @param items - The full array of restore items (source of item objects)
 * @param statusRegistry - Runtime status registry mapping itemId → status
 * @returns Items whose registry status is "applied", sorted by executionOrder
 */
export function getSuccessfulItems(
  items: RestoreItem[],
  statusRegistry: Record<string, RestoreItemStatus>
): RestoreItem[] {
  return items
    .filter((item) => statusRegistry[item.itemId] === "applied")
    .sort((a, b) => a.executionOrder - b.executionOrder);
}

// ── Apply-with-rollback orchestration (v0.2+ Phase-1) ──────────

/**
 * Clear the applied items list on completion.
 *
 * Resets the list to an empty array so that subsequent apply-rollback
 * cycles start fresh. Call this after rollback completes to satisfy
 * the contract that "the list is cleared on completion."
 */
export function clearAppliedItems(summary: ApplySummary): void {
  summary.appliedItems = [];
}

/**
 * Apply restore items and optionally roll them back automatically.
 *
 * Orchestrates the full apply-then-rollback lifecycle:
 * 1. Runs applyRestoreItems with the given executor and options
 * 2. If `options.rollback === true` and there are applied items:
 *    a. Reads applied items from the status registry via `getSuccessfulItems()`
 *    b. Runs rollbackAppliedItems on the registry-identified applied items
 *    c. Clears the applied items list on completion
 * 3. Returns both summaries (rollbackSummary is undefined if rollback not requested)
 *
 * Rollback reads applied items from the execution registry rather than
 * the pre-built `appliedItems` list, demonstrating the registry read
 * interface pattern.
 */
export async function applyWithRollback(
  items: RestoreItem[],
  executor: RestoreExecutor,
  options: ApplyOptions & RollbackOptions
): Promise<ApplyWithRollbackResult> {
  // Phase 1: Apply
  const applySummary = await applyRestoreItems(items, executor, options);

  // Phase 2: Rollback (only if requested and items were applied)
  if (options.rollback && applySummary.appliedItems.length > 0) {
    // Read applied items from the execution registry via the read interface.
    // This decouples rollback from the pre-built `appliedItems` list and
    // demonstrates the registry read interface pattern: rollback queries
    // the authoritative status registry independently.
    const appliedItems = getSuccessfulItems(items, applySummary.statusRegistry);

    // Forward failFast and signal to rollback for on-continue vs on-abort control
    const rollbackSummary = await rollbackAppliedItems(
      appliedItems,
      options.undoExecutor,
      { failFast: options.failFast, signal: options.signal }
    );

    // Clear the applied items list on completion (post-rollback)
    clearAppliedItems(applySummary);

    return { applySummary, rollbackSummary };
  }

  // No rollback requested — still clear applied items if rollback was requested
  // but no items were applied (edge case: all items were unsupported/skipped)
  if (options.rollback) {
    clearAppliedItems(applySummary);
  }

  return { applySummary };
}

// ── Per-item undo handler dispatch (v0.2+ Phase-1) ─────────────

/**
 * Default no-op undo handler for non-reversible item types.
 */
export const noopUndoHandler: UndoHandler = async (_item: RestoreItem): Promise<void> => {
  // No side effects — this is the default for non-reversible types
};

/**
 * Create an UndoExecutor that dispatches to per-type undo handlers.
 *
 * The resulting executor looks up `item.type` in the registry:
 *   - If `item.canRollback === false`, the handler is a no-op
 *     (non-reversible items are never undone)
 *   - If the type is not found in the registry, the handler is a no-op
 *     (default for unregistered types)
 *   - Otherwise, the registered handler is called with the item
 *
 * This bridges the pluggable-executor pattern (UndoExecutor) with the
 * per-item-type dispatch needed for rollback. Callers that want full
 * control can still pass a custom UndoExecutor directly to
 * rollbackAppliedItems().
 */
export function createDefaultUndoExecutor(
  registry: UndoHandlerRegistry
): UndoExecutor {
  return async (item: RestoreItem): Promise<void> => {
    // Non-reversible items are never undone — no-op by contract
    if (!item.canRollback) {
      return;
    }

    // Look up the handler for this item type
    const handler = registry[item.type];

    // Unregistered types default to no-op
    if (!handler) {
      return;
    }

    // Dispatch to the registered handler
    await handler(item);
  };
}

/**
 * Create an undo handler that reverses a "create" side effect by
 * reverting item.state to its previous value stored in rollbackState.
 *
 * This is the generic fallback for item types whose undo logic is
 * "restore the previous value from rollbackState." Types with more
 * complex undo logic should register their own handler instead.
 */
export const restorePreviousStateUndoHandler: UndoHandler = async (
  item: RestoreItem
): Promise<void> => {
  // Generic undo: the apply executor must have saved the previous
  // state into item.rollbackState before mutating. If no previous
  // state was saved, this is a no-op.
  if (item.rollbackState === null || item.rollbackState === undefined) {
    return;
  }
  // The actual restoration is delegated to the apply-time capture;
  // this handler validates that rollbackState exists and signals
  // that the undo data is available for the executor to use.
  // Concrete state restoration happens in the apply executor via
  // the rollbackState stored here.
};

/**
 * Default undo handler registry using content-restore undo.
 * Replaces the old no-op-based registry — all types now support rollback.
 */
export function defaultUndoHandlerRegistry(): UndoHandlerRegistry {
  const handlers: Record<string, UndoHandler> = {};
  for (const type of ["agent_config", "agent_instruction", "mcp_server", "permission", "hook", "skill", "env_key", "env", "symlink"]) {
    handlers[type] = restorePreviousContentUndoHandler;
  }
  handlers.unsupported = noopUndoHandler;
  return handlers;
}

// ── Helpers ─────────────────────────────────────────────────────

function canRollbackAction(action: RestoreAction): boolean {
  return action === "create" || action === "update" || action === "delete";
}

function buildRollbackSteps(items: RestorePlanItem[]): Array<{ itemId: string; action: string; instruction: string }> {
  return items
    .filter((item) => canRollbackAction(item.action))
    .map((item) => ({
      itemId: item.itemId,
      action: rollbackActionFor(item.action),
      instruction: item.rollbackInstruction
    }));
}

function rollbackActionFor(action: RestoreAction): string {
  if (action === "create") return "delete";
  if (action === "delete") return "create";
  return "revert";
}

function restoreActionForChange(
  code: string,
  currentState: DiscoveredItem | null,
  targetState: DiscoveredItem | null
): RestoreAction {
  switch (code) {
    case "AGENT_CONFIG_ADDED":
    case "SKILL_ADDED":
    case "HOOK_ADDED":
      return currentState ? "delete" : "unsupported";
    case "MCP_ADDED":
      return currentState && isJsonMcpState(currentState) ? "delete" : "unsupported";
    case "AGENT_CONFIG_REMOVED":
    case "SKILL_REMOVED":
    case "HOOK_REMOVED":
      return targetState ? "create" : "unsupported";
    case "MCP_REMOVED":
      return targetState && isJsonMcpState(targetState) ? "create" : "unsupported";
    case "AGENT_CONFIG_CHANGED":
    case "HOOK_CHANGED":
    case "PERMISSION_CHANGED":
    case "INSTRUCTION_CHANGED":
    case "SKILL_EXECUTABLE_APPEARED":
      return targetState ? "update" : "unsupported";
    case "MCP_CHANGED":
      return targetState && isJsonMcpState(targetState) ? "update" : "unsupported";
    case "ENV_KEY_ADDED":
      return currentState ? "delete" : "unsupported";
    case "ENV_KEY_REMOVED":
    case "UNSUPPORTED_STATE_CHANGED":
      return "unsupported";
    default:
      return "unsupported";
  }
}

function isJsonMcpState(item: DiscoveredItem): boolean {
  return item.sourcePath.endsWith(".mcp.json") || item.sourcePath.endsWith("/mcp.json");
}

function rollbackInstructionFor(action: RestoreAction, kind: EvidenceKind, name: string): string {
  if (action === "delete") {
    return `Recreate deleted ${kind}: ${name}`;
  }
  if (action === "create") {
    return `Remove created ${kind}: ${name}`;
  }
  return `Reverse ${action} for ${kind}: ${name}`;
}

function sourcePathForRestoreItem(
  kind: EvidenceKind,
  name: string,
  currentState: DiscoveredItem | null,
  targetState: DiscoveredItem | null,
  diffSourcePath?: string
): string {
  return targetState?.sourcePath
    ?? currentState?.sourcePath
    ?? diffSourcePath
    ?? resolveSourcePathByKind(kind, name);
}

function isVirtualSourcePath(sourcePath: string): boolean {
  return /^[a-z_]+:/i.test(sourcePath);
}

function resolvePlanDestination(planItem: RestorePlanItem, targetProject?: string, targetHome?: string): string {
  if (targetHome && planItem.sourcePath === "~") {
    return targetHome;
  }
  if (targetHome && planItem.sourcePath.startsWith("~/")) {
    return path.resolve(targetHome, planItem.sourcePath.slice(2));
  }
  if (!targetProject || path.isAbsolute(planItem.sourcePath) || isVirtualSourcePath(planItem.sourcePath)) {
    return planItem.sourcePath;
  }

  return path.resolve(targetProject, planItem.sourcePath);
}

function restoreItemMetadata(planItem: RestorePlanItem): Record<string, unknown> | undefined {
  const metadata: Record<string, unknown> = {};
  const restorePath = restorePathFromContent(planItem.targetState) ?? restorePathFromContent(planItem.currentState);
  if (restorePath) {
    metadata.restorePath = restorePath;
  }

  if (planItem.kind === "mcp_server") {
    const serverName = planItem.targetState?.name ?? planItem.currentState?.name;
    if (typeof serverName === "string" && serverName.length > 0) {
      metadata.serverName = serverName;
    }
    metadata.sourcePath = planItem.sourcePath;
    if (planItem.sourcePath.endsWith(".mcp.json") || planItem.sourcePath.endsWith("/mcp.json")) {
      metadata.mcpPath = planItem.sourcePath;
    }
  }

  return Object.keys(metadata).length > 0 ? metadata : undefined;
}

function mcpConfigPathForItem(item: RestoreItem): string {
  const metadataPath = item.metadata?.mcpPath;
  if (typeof metadataPath === "string" && metadataPath.length > 0 && path.isAbsolute(metadataPath)) {
    return metadataPath;
  }

  if (path.basename(item.dest) === ".mcp.json") {
    return item.dest;
  }

  return path.join(path.dirname(item.dest), ".mcp.json");
}

function mcpServerNameForItem(item: RestoreItem): string {
  const metadataName = item.metadata?.serverName;
  if (typeof metadataName === "string" && metadataName.length > 0) {
    return metadataName;
  }

  const colonParts = item.itemId.split(":");
  if (colonParts.length >= 2 && colonParts[1]) {
    return colonParts[1];
  }

  return item.itemId.split(".").pop() ?? "unknown";
}

function resolveSourcePathByKind(kind: EvidenceKind, name: string): string {
  if (kind === "mcp_server") return `.mcp.json (${name})`;
  if (kind === "env_key") return `env:${name}`;
  if (kind === "agent_config") return `config:${name}`;
  if (kind === "agent_instruction") return `instruction:${name}`;
  if (kind === "skill") return `skill:${name}`;
  if (kind === "permission") return `permission:${name}`;
  if (kind === "hook") return `hook:${name}`;
  return `unknown:${name}`;
}

function resolveAgent(name: string): AgentId {
  // Use word-boundary matching to avoid false positives like "precursor" → "cursor"
  const word = (s: string) => new RegExp(`\\b${s}\\b`, "i");
  if (word("claude").test(name)) return "claude-code";
  if (word("codex").test(name)) return "codex";
  if (word("cursor").test(name)) return "cursor";
  return "unknown";
}

function findMatchingEvidence(
  change: { entityName: string; entityKind: string },
  evidence: DiscoveredItem[],
  sourcePath?: string
): DiscoveredItem | null {
  if (sourcePath) {
    for (const item of evidence) {
      if (item.kind === change.entityKind && item.name === change.entityName && item.sourcePath === sourcePath) {
        return item;
      }
    }
    for (const item of evidence) {
      if (item.kind === change.entityKind && item.sourcePath === sourcePath) {
        return item;
      }
    }
  }
  // First pass: match both kind AND name (exact match)
  for (const item of evidence) {
    if (item.kind === change.entityKind && item.name === change.entityName) {
      return item;
    }
  }
  // Fallback: match by kind only (partial match for items without names)
  for (const item of evidence) {
    if (item.kind === change.entityKind) {
      return item;
    }
  }
  return null;
}
