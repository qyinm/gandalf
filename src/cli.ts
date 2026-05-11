#!/usr/bin/env node

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
`;

function main(argv: string[]): number {
  if (argv.length === 0 || argv.includes("--help") || argv.includes("-h")) {
    process.stdout.write(HELP);
    return 0;
  }

  process.stderr.write(`SNAPTAILOR_UNKNOWN_COMMAND
Problem: Unknown command.
Cause: snaptailor does not recognize "${argv.join(" ")}".
Fix: Run \`snaptailor --help\` to see supported v0.1 commands.
`);
  return 1;
}

process.exitCode = main(process.argv.slice(2));
