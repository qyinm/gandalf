#!/usr/bin/env node

import { formatSnapError } from "./errors.js";
import { type CommandContext } from "./commands/index.js";
import { scanCommand } from "./commands/scan.js";
import { snapshotCommand } from "./commands/snapshot.js";
import { diffCommand, auditCommand, provenanceCommand } from "./commands/diff.js";
import { reportCommand } from "./commands/report.js";
import { restoreCommand } from "./commands/restore.js";
import { bundleCommand } from "./commands/bundle.js";
import { doctorCommand } from "./commands/doctor.js";
import { schemaCommand } from "./commands/schema.js";
import { tuiCommand } from "./commands/tui.js";
import { hasFlag, runtimeOptions } from "./cli-shared.js";

const HELP = [
  "hem",
  "",
  "Save, compare, and restore AI coding agent setups.",
  "",
  "TUI command:",
  "  hem tui                              launch interactive TUI dashboard",
  "",
  "Diagnosis commands:",
  '  hem scan --project .',
  '  hem scan --project . --explain',
  '  hem snapshot create --name baseline --metadata-only --project .',
  '  hem snapshot create --name baseline --metadata-only --project . --agent claude-code',
  '  hem snapshot list',
  '  hem snapshot list --agent codex',
  '  hem snapshot show baseline --json',
  '  hem diff baseline current --project .',
  '  hem audit current --project .',
  '  hem provenance current --project . --json',
  '  hem report current --project . --out hem-report.md',
  '  hem doctor --project .',
  "",
  "Restore commands:",
  '  hem restore --snapshot <name> --dry-run --project .          generate a non-mutating restore plan as JSON',
  '  hem restore --snapshot <name> --apply --project .            apply restore items sequentially (experimental)',
  '  hem restore --snapshot <name> --apply --fail-fast --project . stop on first failure during apply',
  '  hem restore --snapshot <name> --apply --rollback --project . apply then automatically rollback',
  "",
  "Bundle commands:",
  '  hem bundle export --name <snapshot> --out <file.hem> --project . export snapshot to .hem bundle (content included by default)',
  '  hem bundle export --name <snapshot> --out <file.hem> --metadata-only --project . export metadata-only bundle',
  '  hem bundle verify <file.hem>                             verify format, checksums, and signature metadata',
  '  hem bundle inspect <file.hem>                            inspect bundle metadata',
  '  hem bundle import <file.hem> --dry-run --project .       validate bundle without importing',
  '  hem bundle import <file.hem> --apply-content --quarantine --experimental --project . inspect content without writing targets',
  '  hem bundle import <file.hem> --apply-content --experimental --project . apply project-relative content (experimental)',
].join("\n");

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
  ["doctor", doctorCommand],
  ["schema", schemaCommand],
  ["tui", tuiCommand],
]);

// ── CLI Entry Point ────────────────────────────────────────────

async function run(args: string[]): Promise<number> {
  // --help or no args: print help
  if (args.length === 0 || hasFlag(args, "--help") || hasFlag(args, "-h")) {
    process.stdout.write(HELP + "\n");
    return 0;
  }

  // Look up the command in the registry
  const commandName = args[0];
  const command = registry.get(commandName);

  if (!command) {
    process.stderr.write(formatSnapError({
      code: "HEM_UNKNOWN_COMMAND",
      problem: "Unknown command.",
      cause: `hem does not recognize "${args.join(" ")}".`,
      fix: "Run `hem --help` to see supported commands."
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
      code: "HEM_UNHANDLED_ERROR",
      problem: "Command failed.",
      cause: error instanceof Error ? error.message : "Unknown error.",
      fix: "Rerun with `--help` to confirm command syntax, then inspect the reported path if present."
    }));
    process.exitCode = 1;
  });
