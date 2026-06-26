#!/usr/bin/env node

import { formatSnapError } from "@qxinm/gandalf-core/errors.js";
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
import { timelineCommand } from "./commands/timeline.js";
import { hasFlag, runtimeOptions } from "./cli-shared.js";
import { maybePrintUpdateNotice } from "./update-check.js";

const HELP = [
  "gandalf",
  "",
  "Save, compare, and restore Codex user-global setup experiments.",
  "",
  "TUI command:",
  "  gandalf tui                              launch interactive TUI dashboard",
  "",
  "Local history commands:",
  '  gandalf timeline list --project .',
  '  gandalf timeline show <id>',
  '  gandalf timeline undo <id> --dry-run --json',
  "",
  "Diagnosis commands:",
  '  gandalf scan --project .',
  '  gandalf scan --project . --explain',
  '  gandalf snapshot create --name baseline --agent codex --scope user --project .',
  '  gandalf snapshot create --name baseline --metadata-only --project .',
  '  gandalf snapshot create --name baseline --metadata-only --project . --agent claude-code',
  '  gandalf snapshot list',
  '  gandalf snapshot list --agent codex',
  '  gandalf snapshot show baseline --json',
  '  gandalf diff baseline current --agent codex --scope user --project .',
  '  gandalf diff baseline current --project .',
  '  gandalf audit current --project .',
  '  gandalf provenance current --project . --json',
  '  gandalf report current --project . --out gandalf-report.md',
  '  gandalf doctor --project .',
  "",
  "Restore commands:",
  '  gandalf restore --snapshot <name> --dry-run --agent codex --scope user --project . preview a non-mutating restore plan',
  '  gandalf restore --snapshot <name> --apply --experimental --agent codex --scope user --project . apply restore items sequentially',
  '  gandalf restore --snapshot <name> --apply --fail-fast --project . stop on first failure during apply',
  '  gandalf restore --snapshot <name> --apply --rollback --project . apply then automatically rollback',
  "",
  "Bundle commands:",
  '  gandalf bundle export --name <snapshot> --out <file.gandalf> --project . export snapshot to .gandalf bundle (content included by default)',
  '  gandalf bundle export --name <snapshot> --out <file.gandalf> --metadata-only --project . export metadata-only bundle',
  '  gandalf bundle verify <file.gandalf>                             verify format, checksums, and signature metadata',
  '  gandalf bundle inspect <file.gandalf>                            inspect bundle metadata',
  '  gandalf bundle import <file.gandalf> --dry-run --project .       validate bundle without importing',
  '  gandalf bundle import <file.gandalf> --apply-content --quarantine --experimental --project . inspect content without writing targets',
  '  gandalf bundle import <file.gandalf> --apply-content --experimental --project . apply project-relative content (experimental)',
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
  ["timeline", timelineCommand],
]);

// ── CLI Entry Point ────────────────────────────────────────────

async function run(args: string[]): Promise<number> {
  await maybePrintUpdateNotice({
    args,
    homeDir: process.env.HOME ?? process.cwd(),
    stderrIsTty: process.stderr.isTTY
  });

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
      code: "GANDALF_UNKNOWN_COMMAND",
      problem: "Unknown command.",
      cause: `gandalf does not recognize "${args.join(" ")}".`,
      fix: "Run `gandalf --help` to see supported commands."
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
      code: "GANDALF_UNHANDLED_ERROR",
      problem: "Command failed.",
      cause: error instanceof Error ? error.message : "Unknown error.",
      fix: "Rerun with `--help` to confirm command syntax, then inspect the reported path if present."
    }));
    process.exitCode = 1;
  });
