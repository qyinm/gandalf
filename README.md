# snaptailor

Read-only drift diagnosis and security audit for AI coding agent setups.

snaptailor reads local agent configuration, builds a metadata-only evidence inventory, explains what changed, and flags risky state. v0.2+ adds restore planning and execution.

## Trust Contract

By default snaptailor:

- reads local user and project agent configuration only
- does not execute MCP commands, hooks, scripts, plugins, or agent tools
- does not use the network
- writes only to `~/.snaptailor`, unless `--out` is explicit
- stores metadata-first snapshots
- omits raw secrets and raw `.env` values
- does not follow symlinks

## Install

```bash
npm install -g @qxinm/snaptailor
snaptailor scan --project .
```

First scan prints the trust preflight, detected agents, high-signal findings, blind spots, and the next command to create a baseline.

## Inspect What Will Be Scanned

```bash
snaptailor scan --project . --explain
```

Shows the read/write contract and supported paths before creating a baseline.

## Create First Baseline

```bash
snaptailor snapshot create --name baseline --metadata-only --project .
snaptailor snapshot list
snaptailor snapshot show baseline --json
```

Snapshots are metadata-only in v0.1.

## See What Changed Since Baseline

```bash
snaptailor diff baseline current --project .
```

`current` is a fresh read-only scan. It is not stored unless you explicitly create a snapshot.

## Audit Current Setup

```bash
snaptailor audit current --project .
```

Findings include executable config additions, remote MCP changes, permission wildcards, parse failures, skipped symlinks, unsupported state, and reproducibility gaps.

## Export A Redacted Report

```bash
snaptailor report current --project . --out snaptailor-report.md
```

Exports a human-readable report, not a restorable bundle.

## Restore Planning (v0.2)

```bash
# Generate a non-mutating restore plan
snaptailor restore --snapshot baseline --dry-run --project .

# Apply restore items sequentially (best-effort by default)
snaptailor restore --snapshot baseline --apply --project .

# Stop on first failure
snaptailor restore --snapshot baseline --apply --fail-fast --project .
```

## Machine-Readable Output

```bash
snaptailor scan --project . --json
snaptailor diff baseline current --project . --json
snaptailor provenance current --project . --json
snaptailor audit current --project . --json
```

## Not In v0.1/v0.2

- `restore --apply` with real per-type file executors (currently no-op stub)
- `.stailor` bundle import/export
- team sharing
- desktop UI
- cloud sync
- raw secret capture
