/**
 * snapshot command: create, list, show snapshots.
 *
 * Subcommands:
 *   snapshot create --name <name> --metadata-only --project .
 *   snapshot list
 *   snapshot show <name>  show <name> [--json]
 */

import { auditEvidence } from "../audit.js";
import { formatSnapError } from "../errors.js";
import { buildGraph } from "../graph.js";
import { buildProvenance } from "../provenance.js";
import { scanProject } from "../scan.js";
import {
  ensureStore,
  listSnapshots,
  readSnapshot,
  writeSnapshot
} from "../store.js";
import type { AuditFinding, Snapshot, SnapshotManifest } from "../types.js";
import type { ScanResult } from "../scan.js";
import React from "react";
import { hasFlag, json, runtimeOptions, valueAfter } from "../cli-shared.js";
import { isClackMode, detectTuiMode, isInkMode, renderComponent } from "../tui/index.js";
import type { Command, CommandContext } from "./index.js";

/* ------------------------------------------------------------------ */
/*  CurrentState helper (in-memory pseudo-snapshot)                    */
/* ------------------------------------------------------------------ */

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

/* ------------------------------------------------------------------ */
/*  Command definition                                                 */
/* ------------------------------------------------------------------ */

export const snapshotCommand: Command = {
  name: "snapshot",
  description: "Create, list, and show metadata-only snapshots.",

  async execute(ctx: CommandContext): Promise<number> {
    const { args, options } = ctx;
    const sub = args[1];

    /* ---------- snapshot create ---------- */
    if (sub === "create") {
      // --tui: interactive wizard
      const tuiOpts = detectTuiMode(args);
      if (tuiOpts.mode !== "none") {
        const { snapshotCreateWizard } = await import("../tui/wizards/snapshot-create.js");
        return snapshotCreateWizard(options);
      }
      const name = valueAfter(args, "--name");
      if (!name) {
        process.stderr.write(
          formatSnapError({
            code: "SNAPTAILOR_MISSING_NAME",
            problem: "Snapshot name is required.",
            cause: "`snapshot create` was called without `--name`.",
            fix: "Run `snaptailor snapshot create --name baseline --metadata-only --project .`."
          })
        );
        return 1;
      }
      if (!hasFlag(args, "--metadata-only")) {
        process.stderr.write(
          formatSnapError({
            code: "SNAPTAILOR_METADATA_ONLY_REQUIRED",
            problem: "Snapshots are metadata-only.",
            cause: "`snapshot create` was called without `--metadata-only`.",
            fix: "Add `--metadata-only`; raw content snapshot storage is not supported."
          })
        );
        return 1;
      }

      const state = await currentState(args, name);
      await writeSnapshot(options.storeDir, state.snapshot, options.agent);
      process.stdout.write(`Created metadata-only snapshot: ${name}`);
      if (options.agent) process.stdout.write(` (agent: ${options.agent})`);
      process.stdout.write("\n");
      return 0;
    }

    /* ---------- snapshot list ---------- */
    if (sub === "list") {
      const names = await listSnapshots(options.storeDir, options.agent);
      if (isInkMode(args)) {
        const { default: SnapshotList } = await import("../tui/components/SnapshotList.js");
        return renderComponent(
          () => React.createElement(SnapshotList, { names })
        );
      }
      process.stdout.write(names.length === 0
        ? "No snapshots.\n"
        : `${names.join("\n")}\n`);
      return 0;
    }

    /* ---------- snapshot show ---------- */
    if (sub === "show") {
      const name = args[2];
      if (!name) {
        process.stderr.write(
          formatSnapError({
            code: "SNAPTAILOR_MISSING_NAME",
            problem: "Snapshot name is required.",
            cause: "`snapshot show` was called without a name.",
            fix: "Run `snaptailor snapshot list` and pass one of the listed names."
          })
        );
        return 1;
      }
      const snapshot = await readSnapshot(options.storeDir, name, options.agent);
      process.stdout.write(hasFlag(args, "--json") ? json(snapshot) : `${snapshot.manifest.name}\n`);
      return 0;
    }

    /* ---------- unknown subcommand ---------- */
    process.stderr.write(
      formatSnapError({
        code: "SNAPTAILOR_UNKNOWN_SUBCOMMAND",
        problem: `Unknown snapshot subcommand: "${sub ?? ""}".`,
        cause: "`snapshot` was called with an unrecognized subcommand.",
        fix: "Use `create`, `list`, or `show`. Run `snaptailor --help` for details."
      })
    );
    return 1;
  }
};

export default snapshotCommand;
