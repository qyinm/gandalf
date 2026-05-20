#!/usr/bin/env node

import { formatSnapError } from "./errors.js";
import { type CommandContext } from "./commands/index.js";
import { scanCommand } from "./commands/scan.js";
import { snapshotCommand } from "./commands/snapshot.js";
import { diffCommand, auditCommand, provenanceCommand } from "./commands/diff.js";
import { reportCommand } from "./commands/report.js";
import { restoreCommand } from "./commands/restore.js";
import { bundleCommand } from "./commands/bundle.js";
import { schemaCommand } from "./commands/schema.js";
import { hasFlag, runtimeOptions } from "./cli-shared.js";

const HELP = `snaptailor

Read-only drift diagnosis and security audit for AI coding agent setups.

Core v0.1 commands:
  snaptailor scan --project .
  snaptailor scan --project . --explain
  snaptailor snapshot create --name baseline --metadata-only --project .
  snaptailor snapshot list
  snaptailor snapshot show baseline --json
  snaptailor diff baseline current --project .
  snaptailor audit current --project .
  snaptailor provenance current --project . --json
  snaptailor report current --project . --out snaptailor-report.md

v0.2 commands (dry-run only):
  snaptailor restore --snapshot <name> --dry-run --project .   generate a non-mutating restore plan as JSON

v0.2+ commands (apply + rollback):
  snaptailor restore --snapshot <name> --apply --project .              apply restore items sequentially
  snaptailor restore --snapshot <name> --apply --fail-fast --project .  stop on first failure during apply
  snaptailor restore --snapshot <name> --apply --rollback --project .   apply then automatically rollback

v0.2+ bundle commands:
  snaptailor bundle export --name <snapshot> --out <file.stailor> --project .   export snapshot to .stailor bundle (includes content by default)
  snaptailor bundle export --name <snapshot> --out <file.stailor> --metadata-only --project .   export metadata-only bundle
  snaptailor bundle import <file.stailor> --project .                           import .stailor bundle into local store
  snaptailor bundle inspect <file.stailor>                                      inspect bundle metadata
  snaptailor bundle import <file.stailor> --dry-run --project .                 validate bundle without importing
`;

// ── Command Registry ───────────────────────────────────────────

const registry = new Map<string, import("./commands/index.js").Command>([
  ["scan", scanCommand],
  ["snapshot", snapshotCommand],
  ["diff", diffCommand],
  ["audit", auditCommand],
  ["provenance", provenanceCommand],
  ["report", reportCommand],
  ["restore", restoreCommand],
  ["bundle", bundleCommand],
  ["schema", schemaCommand],
]);

// ── CLI Entry Point ────────────────────────────────────────────

async function run(args: string[]): Promise<number> {
  // --help or no args: print help
  if (args.length === 0 || hasFlag(args, "--help") || hasFlag(args, "-h")) {
    process.stdout.write(HELP);
    return 0;
  }

  // Look up the command in the registry
  const commandName = args[0];
  const command = registry.get(commandName);

  if (!command) {
    process.stderr.write(formatSnapError({
      code: "SNAPTAILOR_UNKNOWN_COMMAND",
      problem: "Unknown command.",
      cause: `snaptailor does not recognize "${args.join(" ")}".`,
      fix: "Run `snaptailor --help` to see supported v0.1 commands."
    }));
    return 1;
  }

  const ctx: CommandContext = {
    args,
    options: runtimeOptions(args),
  };

  return await command.execute(ctx);
}

run(process.argv.slice(2))
  .then((code) => {
    process.exitCode = code;
  })
  .catch((error: unknown) => {
    process.stderr.write(formatSnapError({
      code: "SNAPTAILOR_UNHANDLED_ERROR",
      problem: "Command failed.",
      cause: error instanceof Error ? error.message : "Unknown error.",
      fix: "Rerun with `--help` to confirm command syntax, then inspect the reported path if present."
    }));
    process.exitCode = 1;
  });
