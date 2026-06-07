/**
 * Command-pattern implementation of the `restore` CLI command.
 *
 * v0.2 dry-run mode:
 *   restore --snapshot <name> --dry-run --project .   output a non-mutating restore plan as JSON
 *
 * v0.2+ apply mode (requires --experimental):
 *   restore --snapshot <name> --apply --project .              apply restore items sequentially
 *   restore --snapshot <name> --apply --fail-fast --project .  stop on first failure
 *   restore --snapshot <name> --apply --rollback --project .   apply then auto-rollback on failure
 */

import { hasFlag, json, runtimeOptions, valueAfter } from "../cli-shared.js";
import { formatSnapError } from "../errors.js";
import { detectTuiMode } from "../tui/index.js";
import {
  applyWithRollback,
  buildRestorePlan,
  createDefaultApplyExecutor,
  createDefaultUndoExecutor,
  defaultApplyHandlerRegistry,
  defaultUndoHandlerRegistry,
  formatApplySummary,
  formatRollbackSummary,
  parseDryRunOutput
} from "../restore.js";
import { ensureStore, snapshotExists } from "../store.js";
import type { Command, CommandContext } from "./index.js";

// ── Command export ─────────────────────────────────────────────

export const restoreCommand: Command = {
  name: "restore",
  description: "Generate a restore plan (dry-run) or apply a snapshot (experimental)",

  async execute(ctx: CommandContext): Promise<number> {
    const { args, options } = ctx;

    // ── --tui: interactive wizard ────────────────────────────
    if (detectTuiMode(args).mode !== "none" && !hasFlag(args, "--json")) {
      const { restoreWizard } = await import("../tui/wizards/restore-confirm.js");
      return restoreWizard(options);
    }

    // ── Required: --snapshot ────────────────────────────────
    const snapshotName = valueAfter(args, "--snapshot");
    if (!snapshotName) {
      process.stderr.write(
        formatSnapError({
          code: "HEM_RESTORE_SNAPSHOT_REQUIRED",
          problem: "Snapshot name is required for restore.",
          cause: "`restore` was called without `--snapshot`.",
          fix: "Run `hem restore --snapshot <name> --dry-run --project .`."
        })
      );
      return 1;
    }

    const isDryRun = hasFlag(args, "--dry-run");
    const isApply = hasFlag(args, "--apply");

    // ── Mode selection ──────────────────────────────────────
    if (!isDryRun && !isApply) {
      process.stderr.write(
        formatSnapError({
          code: "HEM_RESTORE_MODE_REQUIRED",
          problem: "Either --dry-run or --apply is required for restore.",
          cause: "`restore` was called without `--dry-run` or `--apply`.",
          fix: "Add `--dry-run` to generate a non-mutating restore plan, or `--apply` to execute restore items."
        })
      );
      return 1;
    }

    if (isDryRun && isApply) {
      process.stderr.write(
        formatSnapError({
          code: "HEM_RESTORE_MODE_CONFLICT",
          problem: "--dry-run and --apply are mutually exclusive.",
          cause: "Both `--dry-run` and `--apply` were passed.",
          fix: "Use `--dry-run` to preview changes, or `--apply` to execute them."
        })
      );
      return 1;
    }

    // ── Validate snapshot exists ────────────────────────────
    await ensureStore(options.storeDir);

    const exists = await snapshotExists(options.storeDir, snapshotName, options.agent);
    if (!exists) {
      process.stderr.write(
        formatSnapError({
          code: "HEM_SNAPSHOT_NOT_FOUND",
          problem: `Snapshot "${snapshotName}" not found.`,
          cause: "The named snapshot does not exist in the store.",
          fix: "Run `hem snapshot list` to see available snapshots."
        })
      );
      return 1;
    }

    // ── Dry-run mode: build plan and output as JSON ─────────
    if (isDryRun) {
      const plan = await buildRestorePlan({
        sourceSnapshot: snapshotName,
        projectPath: options.projectPath,
        homeDir: options.homeDir,
        storeDir: options.storeDir,
        dryRun: true,
        agent: options.agent
      });

      process.stdout.write(json(plan));
      return 0;
    }

    // ── Apply mode: --experimental gate ─────────────────────
    if (!process.env.HEM_EXPERIMENTAL && !hasFlag(args, "--experimental")) {
      process.stderr.write(
        formatSnapError({
          code: "HEM_EXPERIMENTAL_REQUIRED",
          problem: "Restore --apply requires --experimental.",
          cause: "--apply was used without HEM_EXPERIMENTAL=1 or --experimental.",
          fix: "Set HEM_EXPERIMENTAL=1 or pass --experimental to enable experimental features."
        })
      );
      return 1;
    }

    // ── Apply mode: build plan ──────────────────────────────
    const plan = await buildRestorePlan({
      sourceSnapshot: snapshotName,
      projectPath: options.projectPath,
      homeDir: options.homeDir,
      storeDir: options.storeDir,
      dryRun: true,
      agent: options.agent
    });

    // Serialize the plan and parse it into executable RestoreItems
    const planJson = json(plan);
    const parsed = parseDryRunOutput(planJson);
    if (parsed.errors.length > 0) {
      process.stderr.write(
        formatSnapError({
          code: "HEM_RESTORE_PARSE_ERROR",
          problem: "Failed to parse restore plan for execution.",
          cause: parsed.errors[0]?.message ?? "Unknown parse error",
          fix: "This is an internal error. Verify the snapshot is valid and try again."
        })
      );
      return 1;
    }

    // ── Apply mode: execute ────────────────────────────────
    const isFailFast = hasFlag(args, "--fail-fast");
    const isRollback = hasFlag(args, "--rollback");

    // Create the default undo executor from the built-in registry
    const undoExecutor = createDefaultUndoExecutor(defaultUndoHandlerRegistry());

    // Create the default apply executor — dispatches to per-type handlers
    const applyExecutor = createDefaultApplyExecutor(defaultApplyHandlerRegistry());

    const result = await applyWithRollback(parsed.items, applyExecutor, {
      failFast: isFailFast,
      rollback: isRollback,
      undoExecutor
    });

    // Render apply summary
    process.stdout.write(formatApplySummary(result.applySummary));

    // Render rollback summary if rollback was triggered
    if (result.rollbackSummary) {
      process.stdout.write("\n");
      process.stdout.write(formatRollbackSummary(result.rollbackSummary));
    }

    // Exit with non-zero if there were failures
    const hasFailures =
      result.applySummary.failed > 0 ||
      (result.rollbackSummary?.failed ?? 0) > 0;
    return hasFailures ? 1 : 0;
  }
};

export default restoreCommand;
