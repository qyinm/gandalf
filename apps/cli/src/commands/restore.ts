/**
 * Command-pattern implementation of the `restore` CLI command.
 *
 * v0.2 dry-run mode:
 *   restore --snapshot <name> --dry-run --project .          output a human restore preview
 *   restore --snapshot <name> --dry-run --project . --json   output the restore plan as JSON
 *
 * v0.2+ apply mode (requires --experimental):
 *   restore --snapshot <name> --apply --project .              apply restore items sequentially
 *   restore --snapshot <name> --apply --fail-fast --project .  stop on first failure
 *   restore --snapshot <name> --apply --rollback --project .   apply then auto-rollback on failure
 */

import { hasFlag, json, valueAfter } from "../cli-shared.js";
import { formatSnapError } from "@qxinm/hem-core/errors.js";
import { detectTuiMode } from "@qxinm/hem-tui";
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
} from "@qxinm/hem-core/restore.js";
import { ensureStore, snapshotExists } from "@qxinm/hem-core/store.js";
import type { RestorePlan, RestorePlanItem, RiskSummary, UnsupportedPlanItem } from "@qxinm/hem-core/types.js";
import type { Command, CommandContext } from "./index.js";

// ── Command export ─────────────────────────────────────────────

export const restoreCommand: Command = {
  name: "restore",
  description: "Generate a restore plan (dry-run) or apply a snapshot (experimental)",

  async execute(ctx: CommandContext): Promise<number> {
    const { args, options } = ctx;

    // ── --tui: interactive wizard ────────────────────────────
    if (detectTuiMode(args).mode !== "none" && !hasFlag(args, "--json")) {
      const { restoreWizard } = await import("@qxinm/hem-tui/wizards/restore-confirm.js");
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

    // ── Dry-run mode: build plan and output preview ─────────
    if (isDryRun) {
      const plan = await buildRestorePlan({
        sourceSnapshot: snapshotName,
        projectPath: options.projectPath,
        homeDir: options.homeDir,
        storeDir: options.storeDir,
        dryRun: true,
        agent: options.agent,
        scope: options.scope
      });

      process.stdout.write(hasFlag(args, "--json") ? json(plan) : formatRestorePlanPreview(plan));
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
      agent: options.agent,
      scope: options.scope
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

function formatRestorePlanPreview(plan: RestorePlan): string {
  const lines = [
    "hem restore dry-run",
    "",
    `Snapshot: ${plan.sourceSnapshot}`,
    `Target project: ${plan.targetProject}`,
    `Target home: ${plan.targetHome}`,
    `Writable changes: ${plan.items.length}`,
    `Unsupported items: ${plan.unsupportedItems.length}`,
    `Risk: ${formatRiskSummary(plan.riskSummary)}`,
    ""
  ];

  if (plan.items.length === 0 && plan.unsupportedItems.length === 0) {
    lines.push("No restore actions needed.");
  }

  if (plan.items.length > 0) {
    lines.push("Plan:");
    plan.items.forEach((item, index) => {
      lines.push(formatRestorePlanItem(item, index + 1));
    });
  }

  if (plan.unsupportedItems.length > 0) {
    lines.push("", "Unsupported:");
    plan.unsupportedItems.forEach((item, index) => {
      lines.push(formatUnsupportedPlanItem(item, index + 1));
    });
  }

  lines.push(
    "",
    "No files were changed.",
    "Use --apply --experimental to apply this plan.",
    "Use --json for the machine-readable restore plan."
  );

  return `${lines.join("\n")}\n`;
}

function formatRestorePlanItem(item: RestorePlanItem, index: number): string {
  const fields = [
    `${index}. ${item.action} ${item.kind} at ${item.sourcePath}`,
    `   risk: ${item.riskLevel}${item.needsConfirmation ? " (confirmation required)" : ""}`,
    `   rollback: ${item.rollbackInstruction}`
  ];
  const changedFields = [
    ...item.diff.changes,
    ...item.diff.additions.map((field) => `+${field}`),
    ...item.diff.removals.map((field) => `-${field}`)
  ];
  if (changedFields.length > 0) {
    fields.push(`   fields: ${changedFields.join(", ")}`);
  }
  return fields.join("\n");
}

function formatUnsupportedPlanItem(item: UnsupportedPlanItem, index: number): string {
  return `${index}. ${item.kind} at ${item.sourcePath}\n   reason: ${item.reason}`;
}

function formatRiskSummary(riskSummary: RiskSummary): string {
  const ordered: Array<keyof RiskSummary> = ["critical", "high", "medium", "low", "none"];
  const nonZero = ordered
    .filter((risk) => riskSummary[risk] > 0)
    .map((risk) => `${risk} ${riskSummary[risk]}`);
  return nonZero.length > 0 ? nonZero.join(", ") : "none";
}

export default restoreCommand;
