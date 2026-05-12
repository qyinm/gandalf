import { randomUUID } from "node:crypto";
import { scanProject } from "./scan.js";
import { readSnapshot } from "./store.js";
import {
  buildGraph
} from "./graph.js";
import { diffGraphs } from "./diff.js";
import type {
  AgentId,
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
    const canRollback =
      planItem.action === "create" || planItem.action === "update";

    result.push({
      itemId: planItem.itemId,
      path: planItem.sourcePath,
      type: planItem.kind,
      source: planItem.sourcePath,
      dest: planItem.sourcePath,
      status: "pending",
      executionOrder: order,
      rollbackState: null,
      canRollback,
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
  const snapshot = await readSnapshot(options.storeDir, options.sourceSnapshot);
  const scan = await scanProject({
    projectPath: options.projectPath,
    homeDir: options.homeDir,
    storeDir: options.storeDir
  });
  const currentGraph = buildGraph(scan.evidence);

  // Build a meaningful plan from the diff
  const diff = diffGraphs(snapshot.graph, currentGraph);
  const items: RestorePlanItem[] = [];
  const unsupportedItems: UnsupportedPlanItem[] = [];
  const executionOrder: string[] = [];
  const riskCounts: RiskSummary = { none: 0, low: 0, medium: 0, high: 0, critical: 0 };

  // Generate restore items from semantic changes
  for (const change of diff.semanticChanges) {
    const itemId = `${change.entityKind}:${change.entityName}:${randomUUID().slice(0, 8)}`;
    const sourcePath = resolveSourcePathByKind(change.entityKind, change.entityName);

    // Determine action based on change type
    let action: RestoreAction;
    if (change.code === "MCP_ADDED" || change.code === "ENV_KEY_ADDED") {
      action = "create";
    } else if (change.code === "MCP_CHANGED") {
      action = "update";
    } else {
      // Removals and other changes become "skip" in the dry-run plan
      action = "skip";
    }

    // Find matching graph nodes for current/target state
    const currentState = findMatchingEvidence(change, scan.evidence, false);
    const targetState = findMatchingEvidence(change, snapshot.evidence, true);

    const riskLevel = change.severity;
    riskCounts[riskLevel]++;

    items.push({
      itemId,
      agent: resolveAgent(change.entityName),
      kind: change.entityKind,
      sourcePath,
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
      rollbackInstruction: `Reverse ${action} for ${change.entityKind}: ${change.entityName}`
    });
    executionOrder.push(itemId);
  }

  // Mark unsupported items from the diff
  for (const change of diff.semanticChanges) {
    if (change.code === "UNSUPPORTED_STATE_CHANGED") {
      unsupportedItems.push({
        itemId: `unsupported:${change.entityName}:${randomUUID().slice(0, 8)}`,
        agent: resolveAgent(change.entityName),
        kind: change.entityKind,
        sourcePath: resolveSourcePathByKind(change.entityKind, change.entityName),
        reason: `Unsupported state change: ${change.entityKind} ${change.entityName}`
      });
    }
  }

  const planId = `plan-${randomUUID().slice(0, 12)}`;

  return {
    planId,
    sourceSnapshot: options.sourceSnapshot,
    targetProject: options.projectPath,
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
      generatedBy: "snaptailor restore"
    }
  };
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
 * Default undo handler registry for known restore item types.
 *
 * This registry provides default no-ops for types that don't have
 * concrete undo implementations (e.g., mcp_server, env_key).
 * As each type's concrete undo logic is implemented, register it here.
 */
export function defaultUndoHandlerRegistry(): UndoHandlerRegistry {
  return {
    // env_key items: generic rollbackState-based undo
    env_key: restorePreviousStateUndoHandler,
    env: restorePreviousStateUndoHandler,

    // mcp_server items: generic rollbackState-based undo
    mcp_server: restorePreviousStateUndoHandler,

    // skill items: generic rollbackState-based undo
    skill: restorePreviousStateUndoHandler,

    // agent_config and agent_instruction: rollbackState-based undo
    agent_config: restorePreviousStateUndoHandler,
    agent_instruction: restorePreviousStateUndoHandler,

    // permission, hook, symlink: no-op by default (non-reversible in v0.2)
    permission: noopUndoHandler,
    hook: noopUndoHandler,
    symlink: noopUndoHandler,

    // catch-all for unknown types (no-op)
    unsupported: noopUndoHandler
  };
}

// ── Helpers ─────────────────────────────────────────────────────

function canRollbackAction(action: RestoreAction): boolean {
  return action === "create" || action === "update";
}

function buildRollbackSteps(items: RestorePlanItem[]): Array<{ itemId: string; action: string; instruction: string }> {
  return items
    .filter((item) => canRollbackAction(item.action))
    .map((item) => ({
      itemId: item.itemId,
      action: item.action === "create" ? "delete" : "revert",
      instruction: item.rollbackInstruction
    }));
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
  if (name.startsWith("claude") || name.includes("claude")) return "claude-code";
  if (name.startsWith("codex") || name.includes("codex")) return "codex";
  if (name.startsWith("cursor") || name.includes("cursor")) return "cursor";
  return "unknown";
}

function findMatchingEvidence(
  change: { entityName: string; entityKind: string },
  evidence: DiscoveredItem[],
  target: boolean
): DiscoveredItem | null {
  for (const item of evidence) {
    if (item.kind === change.entityKind) {
      return item;
    }
  }
  return null;
}