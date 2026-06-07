/**
 * snapshot command: create, list, show snapshots.
 *
 * Subcommands:
 *   snapshot create --name <name> --metadata-only --project .
 *   snapshot list
 *   snapshot show <name>  show <name> [--json]
 */

import { formatSnapError } from "../errors.js";
import { captureCurrentState } from "../current-state.js";
import {
  listSnapshots,
  readSnapshot,
  writeSnapshot,
} from "../store.js";
import React from "react";
import { hasFlag, json, valueAfter } from "../cli-shared.js";
import { detectTuiMode, isInkMode, renderComponent } from "../tui/index.js";
import type { Command, CommandContext } from "./index.js";

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
            code: "HEM_MISSING_NAME",
            problem: "Snapshot name is required.",
            cause: "`snapshot create` was called without `--name`.",
            fix: "Run `hem snapshot create --name baseline --metadata-only --project .`."
          })
        );
        return 1;
      }
      if (!hasFlag(args, "--metadata-only")) {
        process.stderr.write(
          formatSnapError({
            code: "HEM_METADATA_ONLY_REQUIRED",
            problem: "Snapshots are metadata-only.",
            cause: "`snapshot create` was called without `--metadata-only`.",
            fix: "Add `--metadata-only`; raw content snapshot storage is not supported."
          })
        );
        return 1;
      }

      const state = await captureCurrentState(options, name);
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
            code: "HEM_MISSING_NAME",
            problem: "Snapshot name is required.",
            cause: "`snapshot show` was called without a name.",
            fix: "Run `hem snapshot list` and pass one of the listed names."
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
        code: "HEM_UNKNOWN_SUBCOMMAND",
        problem: `Unknown snapshot subcommand: "${sub ?? ""}".`,
        cause: "`snapshot` was called with an unrecognized subcommand.",
        fix: "Use `create`, `list`, or `show`. Run `hem --help` for details."
      })
    );
    return 1;
  }
};

export default snapshotCommand;
