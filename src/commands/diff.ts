/**
 * Command-pattern implementations for `diff`, `audit`, and `provenance`.
 *
 * Each command is a standalone exported const matching the Command interface
 * from ./index.ts. Internal helpers (currentState, snapshotByRef, renderers)
 * are defined locally to keep cli.ts decoupled.
 */

import React from "react";
import { diffGraphs, type GraphDiff } from "../diff.js";
import { hasFlag, json, runtimeOptions } from "../cli-shared.js";
import { isInkMode, renderComponent } from "../tui/index.js";
import { readSnapshot } from "../store.js";
import { captureCurrentState } from "../current-state.js";
import { formatSnapError } from "../errors.js";
import type { AuditFinding, Snapshot } from "../types.js";
import type { Command, CommandContext } from "./index.js";

// ── Internal helpers ───────────────────────────────────────────

async function snapshotByRef(ref: string, args: string[]): Promise<Snapshot> {
  if (ref === "current") {
    return (await captureCurrentState(runtimeOptions(args))).snapshot;
  }
  const opts = runtimeOptions(args);
  return await readSnapshot(opts.storeDir, ref, opts.agent);
}

function renderDiffText(diff: GraphDiff): string {
  const lines = ["hem diff", "", "Semantic changes"];
  if (diff.semanticChanges.length === 0) {
    lines.push("  none");
  } else {
    for (const change of diff.semanticChanges) {
      lines.push(`  ${change.severity.toUpperCase()}  ${change.code}: ${change.entityName}`);
    }
  }

  lines.push("", "Raw source changes");
  if (diff.rawSourceChanges.length === 0) {
    lines.push("  none");
  } else {
    for (const change of diff.rawSourceChanges) {
      lines.push(`  ${change.status}: ${change.sourcePath}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

function renderFindingsText(findings: AuditFinding[]): string {
  if (findings.length === 0) {
    return "No findings.\n";
  }

  const MAX_DISPLAY = 12;
  const display = findings.slice(0, MAX_DISPLAY);
  const remaining = findings.length - display.length;

  const lines = display.map(
    (f) => `${f.severity.toUpperCase()} ${f.code}: ${f.problem}`
  );

  if (remaining > 0) {
    lines.push(`  ... and ${remaining} more finding(s). Use --json to see all.`);
  }

  return `${lines.join("\n")}\n`;
}

// ── diff command ───────────────────────────────────────────────

export const diffCommand: Command = {
  name: "diff",
  description: "Show semantic and raw-source changes between two snapshots",
  async execute(ctx: CommandContext): Promise<number> {
    const baseline = ctx.args[1];
    const target = ctx.args[2];
    if (!baseline || !target) {
      process.stderr.write(
        formatSnapError({
          code: "HEM_DIFF_REFS_REQUIRED",
          problem: "Two snapshot references are required.",
          cause: "`diff` was called without baseline and target references.",
          fix: "Run `hem diff baseline current --project .`."
        })
      );
      return 1;
    }

    const before = await snapshotByRef(baseline, ctx.args);
    const after = await snapshotByRef(target, ctx.args);
    const diff = diffGraphs(before.graph, after.graph);

    if (hasFlag(ctx.args, "--json")) {
      process.stdout.write(json(diff));
      return 0;
    }
    if (isInkMode(ctx.args)) {
      const { default: DiffView } = await import("../tui/components/DiffView.js");
      return renderComponent(
        () => React.createElement(DiffView, {
          semanticChanges: diff.semanticChanges,
          rawSourceChanges: diff.rawSourceChanges,
        })
      );
    }
    process.stdout.write(renderDiffText(diff));
    return 0;
  }
};

// ── audit command ──────────────────────────────────────────────

export const auditCommand: Command = {
  name: "audit",
  description: "Run audit rules on a snapshot and print findings",
  async execute(ctx: CommandContext): Promise<number> {
    const ref = ctx.args[1] ?? "current";
    const snapshot = await snapshotByRef(ref, ctx.args);

    if (hasFlag(ctx.args, "--json")) {
      process.stdout.write(json(snapshot.auditFindings));
      return 0;
    }
    if (isInkMode(ctx.args)) {
      const { default: AuditView } = await import("../tui/components/AuditView.js");
      return renderComponent(
        () => React.createElement(AuditView, { findings: snapshot.auditFindings })
      );
    }
    process.stdout.write(renderFindingsText(snapshot.auditFindings));
    return 0;
  }
};

// ── provenance command ─────────────────────────────────────────

export const provenanceCommand: Command = {
  name: "provenance",
  description: "Show provenance for every node in a snapshot graph",
  async execute(ctx: CommandContext): Promise<number> {
    const ref = ctx.args[1] ?? "current";
    const snapshot = await snapshotByRef(ref, ctx.args);

    // Provenance always outputs JSON (structured data is the primary format)
    process.stdout.write(json(snapshot.provenance));
    return 0;
  }
};
