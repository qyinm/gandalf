# snaptailor

Read-only drift diagnosis and security audit for AI coding agent setups.

snaptailor v0.1 does not restore, import, share, or execute anything. It reads local agent configuration, builds a metadata-only evidence inventory, explains what changed, and flags risky state.

## Trust Contract

By default snaptailor:

- reads local user and project agent configuration only
- does not execute MCP commands, hooks, scripts, plugins, or agent tools
- does not use the network
- writes only to `~/.snaptailor`, unless `--out` is explicit
- stores metadata-first snapshots
- omits raw secrets and raw `.env` values
- does not follow symlinks

## First Run

```bash
npm install -g snaptailor
snaptailor scan --project .
```

The first scan should print the trust preflight, detected agents, high-signal findings, blind spots, and the next command to create a baseline.

## Inspect What Will Be Scanned

```bash
snaptailor scan --project . --explain
```

Use this before creating a baseline if you want to see the read/write contract and supported paths.

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

This exports a human-readable report, not a restorable bundle.

## Machine-Readable Output

```bash
snaptailor scan --project . --json
snaptailor diff baseline current --project . --json
snaptailor provenance current --project . --json
snaptailor audit current --project . --json
```

Use JSON output in CI, local scripts, or agent workflows.

## Not In v0.1

- `restore --apply`
- third-party bundle import
- team sharing
- desktop UI
- cloud sync
- raw secret capture

