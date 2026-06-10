/**
 * Command-pattern implementation of the `report` CLI command.
 *
 * hem report [ref] [--out <path>] [--json] --project .
 *
 * Outputs a markdown report (to stdout or --out file).
 * Shows diff when comparing against a stored snapshot.
 */

import { writeFile } from "node:fs/promises";
import path from "node:path";

import { captureCurrentState } from "@qxinm/hem-core/current-state.js";
import { diffGraphs, type GraphDiff } from "@qxinm/hem-core/diff.js";
import { renderMarkdownReport } from "@qxinm/hem-core/report.js";
import { scanProject, type ScanResult } from "@qxinm/hem-core/scan.js";
import { readSnapshot } from "@qxinm/hem-core/store.js";
import type { Snapshot } from "@qxinm/hem-core/types.js";
import { hasFlag, json, runtimeOptions, valueAfter, type RuntimeOptions } from "../cli-shared.js";
import type { Command, CommandContext } from "./index.js";

async function snapshotByRef(ref: string, args: string[]): Promise<Snapshot> {
  if (ref === "current") {
    return (await captureCurrentState(runtimeOptions(args))).snapshot;
  }
  const opts = runtimeOptions(args);
  return await readSnapshot(opts.storeDir, ref, opts.agent);
}

function trustForReport(scan: ScanResult): { readOnly: boolean; network: "disabled"; commandsExecuted: number } {
  return {
    readOnly: scan.trust.readOnly,
    network: scan.trust.network,
    commandsExecuted: scan.trust.commandsExecuted.length
  };
}

// ── Command definition ───────────────────────────────────────────────────────

export const reportCommand: Command = {
  name: "report",
  description:
    "Generate a markdown report of agent state, findings, and (optionally) diff. " +
    "Usage: hem report [ref] [--out <path>] [--json] --project .",

  async execute(ctx: CommandContext): Promise<number> {
    const { args } = ctx;

    // ref is the first positional argument; default to "current"
    const ref = args[1] ?? "current";
    const options: RuntimeOptions = runtimeOptions(args);
    const snapshot = await snapshotByRef(ref, args);

    // Compute diff only when comparing against a stored snapshot
    const diff: GraphDiff | undefined =
      ref === "current"
        ? undefined
        : diffGraphs(snapshot.graph, (await captureCurrentState(options)).snapshot.graph);

    // Scan is needed for trust info and blind spots
    const scan: ScanResult =
      ref === "current"
        ? (await captureCurrentState(options)).scan
        : await scanProject(options);

    const markdown = renderMarkdownReport({
      snapshotName: snapshot.manifest.name,
      trust: trustForReport(scan),
      evidence: snapshot.evidence,
      graph: snapshot.graph,
      findings: snapshot.auditFindings,
      provenance: snapshot.provenance,
      blindSpots: scan.blindSpots,
      diffs: diff
    });

    if (hasFlag(args, "--json")) {
      process.stdout.write(json({ snapshot, markdown }));
      return 0;
    }

    const out = valueAfter(args, "--out");
    if (out) {
      await writeFile(path.resolve(out), markdown);
      process.stdout.write(`Wrote report: ${path.resolve(out)}\n`);
    } else {
      process.stdout.write(markdown);
    }

    return 0;
  }
};
