/**
 * Command-pattern implementation of the `report` CLI command.
 *
 * snaptailor report [ref] [--out <path>] [--json] --project .
 *
 * Outputs a markdown report (to stdout or --out file).
 * Shows diff when comparing against a stored snapshot.
 */

import { writeFile } from "node:fs/promises";
import path from "node:path";

import { auditEvidence } from "../audit.js";
import { diffGraphs, type GraphDiff } from "../diff.js";
import { formatSnapError } from "../errors.js";
import { buildGraph } from "../graph.js";
import { buildProvenance } from "../provenance.js";
import { renderMarkdownReport } from "../report.js";
import { scanProject, type ScanResult } from "../scan.js";
import { ensureStore, readSnapshot } from "../store.js";
import type { AuditFinding, Snapshot, SnapshotManifest } from "../types.js";
import { hasFlag, json, runtimeOptions, valueAfter, type RuntimeOptions } from "../cli-shared.js";
import type { Command, CommandContext } from "./index.js";

// ── Local helpers (mirrored from cli.ts until all commands are migrated) ─────

interface CurrentState {
  scan: ScanResult;
  snapshot: Snapshot;
  storeFindings: AuditFinding[];
}

async function currentState(args: string[], name = "current"): Promise<CurrentState> {
  const options = runtimeOptions(args);
  const storeFindings = await ensureStore(options.storeDir);
  const scan = await scanProject(options);
  const graph = buildGraph(scan.evidence);
  const auditFindings = [...storeFindings, ...auditEvidence(scan.evidence, graph)];
  const provenance = buildProvenance(graph, scan.evidence);
  const manifest: SnapshotManifest = {
    schemaVersion: "0.1",
    name,
    createdAt: new Date().toISOString(),
    projectPath: options.projectPath,
    security: {
      rawSecretsIncluded: false,
      redactionPolicy: "metadata-only"
    }
  };
  return {
    scan,
    storeFindings,
    snapshot: {
      manifest,
      evidence: scan.evidence,
      graph,
      auditFindings,
      provenance
    }
  };
}

async function snapshotByRef(ref: string, args: string[]): Promise<Snapshot> {
  if (ref === "current") {
    return (await currentState(args)).snapshot;
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
    "Usage: snaptailor report [ref] [--out <path>] [--json] --project .",

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
        : diffGraphs(snapshot.graph, (await currentState(args)).snapshot.graph);

    // Scan is needed for trust info and blind spots
    const scan: ScanResult =
      ref === "current"
        ? (await currentState(args)).scan
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
