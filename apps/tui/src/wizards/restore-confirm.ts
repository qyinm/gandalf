/**
 * Clack wizard for `hem restore`.
 *
 * Walks the user through:
 *   1. Selecting a snapshot
 *   2. Running dry-run to preview the plan
 *   3. Confirming apply (with experimental opt-in)
 *   4. Executing with optional rollback
 */

import * as clack from "@clack/prompts";

import {
  applyWithRollback,
  buildRestorePlan,
  createDefaultApplyExecutor,
  createDefaultUndoExecutor,
  defaultApplyHandlerRegistry,
  defaultUndoHandlerRegistry,
  formatApplySummary,
  formatRollbackSummary,
  parseDryRunOutput,
} from "@qxinm/hem-core/restore.js";
import { ensureStore, listSnapshots } from "@qxinm/hem-core/store.js";
import type { RuntimeOptions } from "@qxinm/hem-core";
import { formatSnapError } from "@qxinm/hem-core/errors.js";

/**
 * Run the restore wizard interactively.
 * Returns exit code (0 = success, 1 = cancelled/error).
 */
export async function restoreWizard(
  options: RuntimeOptions
): Promise<number> {
  clack.intro("hem restore");

  await ensureStore(options.storeDir);
  const snapshots = await listSnapshots(options.storeDir, options.agent);

  if (snapshots.length === 0) {
    clack.log.error(
      "No snapshots found. Create one first with `hem snapshot create`."
    );
    clack.outro("Restore cancelled.");
    return 1;
  }

  // ── Step 1: Pick a snapshot ──────────────────────────────
  const snapshotName = await clack.select({
    message: "Select a snapshot to restore from:",
    options: snapshots.map((name) => ({ label: name, value: name })),
  });

  if (clack.isCancel(snapshotName)) {
    clack.cancel("Restore cancelled.");
    return 1;
  }

  // ── Step 2: Run dry-run ──────────────────────────────────
  const drySpinner = clack.spinner();
  drySpinner.start("Generating restore plan (dry-run)...");

  let plan;
  try {
    plan = await buildRestorePlan({
      sourceSnapshot: snapshotName as string,
      storeDir: options.storeDir,
      projectPath: options.projectPath,
      homeDir: options.homeDir,
      dryRun: true,
      agent: options.agent,
      scope: options.scope,
    });
    drySpinner.stop("Restore plan generated");
  } catch (err) {
    drySpinner.stop("Plan generation failed");
    process.stderr.write(
      formatSnapError({
        code: "HEM_RESTORE_PLAN_FAILED",
        problem: `Failed to build restore plan: ${err instanceof Error ? err.message : String(err)}`,
        cause: "The snapshot could not be compared with the current state.",
        fix: "Verify the snapshot exists and is compatible with this project.",
      })
    );
    return 1;
  }

  // ── Step 3: Show plan summary ────────────────────────────
  const summaryParts: string[] = [];

  const actions = plan.items ?? [];
  const skipItems = plan.unsupportedItems ?? [];

  if (actions.length > 0) {
    summaryParts.push(`Actions to perform: ${actions.length}`);
    for (const action of actions.slice(0, 10)) {
      summaryParts.push(`  • ${action.action}: ${action.kind} @ ${action.sourcePath}`);
    }
    if (actions.length > 10) {
      summaryParts.push(`  ... and ${actions.length - 10} more`);
    }
  }

  if (skipItems.length > 0) {
    summaryParts.push(`Skipped items: ${skipItems.length}`);
  }

  clack.note(summaryParts.join("\n") || "No changes needed.", "Restore Plan");

  if (actions.length === 0) {
    clack.log.info("No restore actions needed — snapshot and current state match.");
    clack.outro("Restore complete (no-op).");
    return 0;
  }

  // ── Step 4: Experimental warning ─────────────────────────
  clack.log.warn(
    "Restore --apply is experimental. It modifies files in your project."
  );

  // ── Step 5: Confirm apply ────────────────────────────────
  const confirmApply = await clack.confirm({
    message: "Proceed with restore?",
    active: "Yes, run restore",
    inactive: "Cancel",
    initialValue: false,
  });

  if (clack.isCancel(confirmApply) || !confirmApply) {
    clack.cancel("Restore cancelled.");
    return 1;
  }

  // ── Step 6: Rollback option ──────────────────────────────
  const useRollback = await clack.confirm({
    message: "Enable automatic rollback on failure?",
    active: "Yes, rollback on failure",
    inactive: "No, stop on failure",
    initialValue: true,
  });

  if (clack.isCancel(useRollback)) {
    clack.cancel("Restore cancelled.");
    return 1;
  }

  // ── Step 7: Execute restore ──────────────────────────────
  const execSpinner = clack.spinner();
  execSpinner.start("Executing restore...");

  try {
    // Parse the plan into RestoreItem[]
    const planJson = JSON.stringify(plan);
    const parsed = parseDryRunOutput(planJson);

    if (parsed.errors && parsed.errors.length > 0) {
      execSpinner.stop("Plan parsing failed");
      clack.log.error(`Plan parsing errors: ${parsed.errors.length}`);
      return 1;
    }

    const applyRegistry = defaultApplyHandlerRegistry();
    const undoRegistry = defaultUndoHandlerRegistry();
    const executor = createDefaultApplyExecutor(applyRegistry);
    const undoExecutor = createDefaultUndoExecutor(undoRegistry);

    const result = await applyWithRollback(
      parsed.items,
      executor,
      {
        rollback: useRollback as boolean,
        failFast: false,
        undoExecutor,
      }
    );

    execSpinner.stop("Restore complete");

    const summaryLines: string[] = [];
    if (result.applySummary) {
      summaryLines.push(formatApplySummary(result.applySummary));
    }
    if (result.rollbackSummary) {
      summaryLines.push(formatRollbackSummary(result.rollbackSummary));
    }

    clack.note(summaryLines.join("\n") || "Done.", "Result");
    clack.outro("Restore complete!");
    return 0;
  } catch (err) {
    execSpinner.stop("Restore failed");
    process.stderr.write(
      formatSnapError({
        code: "HEM_RESTORE_EXECUTION_FAILED",
        problem: `Restore execution failed: ${err instanceof Error ? err.message : String(err)}`,
        cause: "An error occurred during restore.",
        fix: "Check the logs and try again. Use --dry-run to preview before applying.",
      })
    );
    return 1;
  }
}
