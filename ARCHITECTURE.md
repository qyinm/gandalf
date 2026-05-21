# snaptailor Architecture

snaptailor is a local-first CLI for inspecting, packaging, and restoring AI coding agent environments. It captures agent configuration surfaces such as MCP servers, skills, hooks, permissions, instructions, and project-local agent files into a normalized evidence model, then uses that model for snapshots, diffs, audits, reports, `.stailor` bundles, and experimental restore operations.

The core architectural rule is simple: scan paths are read-only and policy-aware; write paths are explicit, narrow, and reversible where possible.

## System Shape

```text
                +----------------------+
                | snaptailor CLI       |
                | src/cli.ts           |
                +----------+-----------+
                           |
                           v
                +----------------------+
                | Command handlers     |
                | src/commands/*       |
                +----------+-----------+
                           |
        +------------------+------------------+
        |                                     |
        v                                     v
+-------------------+              +-------------------+
| Scan pipeline     |              | TUI renderers     |
| src/scan.ts       |              | src/tui/*         |
+---------+---------+              +-------------------+
          |
          v
+-------------------+       +-------------------+
| Scanner plugins   | ----> | DiscoveredItem[]  |
| src/scanners/*    |       | src/types.ts      |
+---------+---------+       +---------+---------+
          |                           |
          |                           v
          |                 +-------------------+
          |                 | Graph builder     |
          |                 | src/graph.ts      |
          |                 +---------+---------+
          |                           |
          +---------------------------+---------------------------+
                                      |
             +------------------------+------------------------+
             |                        |                        |
             v                        v                        v
      +--------------+         +--------------+          +--------------+
      | Diff         |         | Audit        |          | Provenance   |
      | src/diff.ts  |         | src/audit.ts |          | src/provenance.ts |
      +------+-------+         +------+-------+          +------+-------+
             |                        |                        |
             +------------------------+------------------------+
                                      |
                                      v
                          +-----------------------+
                          | Snapshot/report/store |
                          | src/store.ts          |
                          | src/report.ts         |
                          +-----------+-----------+
                                      |
                    +-----------------+-----------------+
                    |                                   |
                    v                                   v
          +-------------------+              +-------------------+
          | .stailor bundles  |              | Restore planner   |
          | src/bundle.ts     |              | src/restore.ts    |
          +-------------------+              +-------------------+
```

## Runtime Entry Points

- `src/cli.ts` is the process entry point and command registry. It maps top-level commands to implementations in `src/commands/*`.
- `src/cli-shared.ts` centralizes flag parsing and runtime options shared across command handlers.
- `src/commands/*` adapts CLI arguments into domain calls. These files should stay thin: parse options, call core modules, format JSON/text/TUI output, and convert errors into user-facing messages.
- `src/tui/*` is a presentation layer. It renders richer Ink/Clack views for supported commands without changing core behavior.

## Core Data Model

The central object is `DiscoveredItem` in `src/types.ts`. Scanner plugins emit discovered items with:

- `agent`, `kind`, `scope`, and `sourcePath` to describe where the state came from.
- `precedence` to resolve user-vs-project override behavior.
- `parser`, `sensitivity`, `contentPolicy`, `restorePolicy`, and `captureStatus` to preserve safety decisions alongside the evidence.
- Optional structured `value`, `checksum`, and `metadata`.

The rest of the system derives from that inventory:

- `GraphNode[]` is built from evidence in `src/graph.ts`.
- `AuditFinding[]` is produced from evidence plus graph context in `src/audit.ts`.
- `ProvenanceEntry[]` records source, scope, precedence, confidence, and capture status in `src/provenance.ts`.
- Snapshots persist all of the above as JSON files under `~/.snaptailor` through `src/store.ts`.

## Scan Pipeline

`scanProject()` in `src/scan.ts` constructs a scanner context from `projectPath`, `homeDir`, and `storeDir`, then executes the default scanner plugin list from `src/scanners/index.ts`.

There are two scanner styles:

- Target-based scanners declare files/directories to inspect. `src/scanners/filesystem.ts` handles file metadata, structured parsing, checksums, redaction, directory bounds, and symlink refusal.
- Custom scanners implement `scan(context)` directly when an agent needs custom discovery logic.

Supported built-in scanner modules currently cover:

- Claude Code: `src/scanners/claude-code.ts`
- Codex: `src/scanners/codex.ts`
- Cursor: `src/scanners/cursor.ts`
- OpenCode: `src/scanners/opencode.ts`
- Pi Agent: `src/scanners/pi.ts`
- Project-local agent files: `src/scanners/project.ts`

The plugin contract lives in `src/scanners/scanner-plugin.ts`. New agent support should generally enter through this interface instead of adding special cases to the core scan pipeline.

## Policy Layer

`src/policy.ts` is the safety map for capture and restore behavior.

Important policy defaults:

- Agent instructions, agent configs, skills, and extensions can be full-content restore candidates.
- MCP servers, permissions, and hooks are structured-fields-only restore candidates.
- Env files are key-inventory-only.
- Symlinks and unsupported surfaces are not restorable.
- Secret-like keys are redacted or omitted.
- Large or noisy directories such as `.git`, `node_modules`, `dist`, and caches are ignored.

This policy layer is intentionally separate from scanners so every feature downstream sees the same capture and restore decisions.

## Snapshot Store

`src/store.ts` owns the local snapshot store. The default store path is `~/.snaptailor`, created with `0700` permissions.

A snapshot directory contains:

```text
manifest.json
evidence.json
graph.json
audit-findings.json
provenance.json
checksums.json
redactions.json
```

Snapshot writes use atomic JSON writes. Snapshot names and agent-scoped store paths are validated to avoid path traversal.

## Graph, Diff, Audit, Provenance, Report

The read-only diagnosis flow is:

1. `scanProject()` returns `DiscoveredItem[]`.
2. `buildGraph()` converts evidence into effective graph nodes and marks lower-precedence nodes as overridden.
3. `diffGraphs()` compares two graph states for semantic and raw changes.
4. `auditEvidence()` flags risky executable config, wildcard permissions, parse failures, skipped symlinks, unsupported state, secret-like values, unsafe store permissions, and project overrides of user policy.
5. `buildProvenance()` traces graph nodes back to evidence sources.
6. `buildReport()` turns the snapshot state into Markdown output.

This path should remain non-mutating except for explicit snapshot/report output.

## Bundle Architecture

`src/bundle.ts` implements `.stailor` export, import, inspect, and verify.

A `.stailor` file is a tar archive with metadata under `.stailor/`, normalized snapshot JSON under `snapshot/`, and optional captured file content under `content/`.

Bundle responsibilities include:

- Normalizing home paths to `{home}/...` for cross-machine portability.
- Resolving `{home}` back to the target machine during import.
- Recording source machine metadata such as home directory, platform, and hostname.
- Detecting cross-OS differences and MCP binary availability on import.
- Enforcing size limits for bundle and content entries.
- Computing SHA-256 checksums for archive entries.
- Supporting optional HMAC-SHA256 signatures via `SNAPTAILOR_BUNDLE_KEY` or an explicit key.
- Supporting quarantine import so content can be staged for inspection before writing target files.

Bundle import is not the same as blind restore. Dry-run, verification, quarantine, explicit content apply flags, and experimental gates are part of the trust boundary.

## Restore Architecture

`src/restore.ts` has two separate responsibilities:

- Build restore plans by diffing a stored snapshot against a fresh scan of the current project.
- Execute restore items through typed apply handlers and rollback handlers.

Restore planning produces structured `RestorePlanItem` records with action, risk level, current state, target state, diff details, rollback instruction, and confirmation prompt. Unsupported items are carried explicitly instead of disappearing.

Restore apply is sequential and can be configured with `failFast` and rollback behavior. Restore should stay conservative: when policy or content support is unclear, the right behavior is to skip or mark unsupported rather than guessing.

## TUI Architecture

The TUI layer is optional and presentation-only.

- `src/tui/tui-mode.ts` detects when rich output is appropriate.
- `src/tui/wizards/*` contains Clack-based interactive flows for bundle export/import, restore confirmation, and snapshot creation.
- `src/tui/components/*` contains Ink views for scans, audits, diffs, snapshots, provenance, reports, dashboard, errors, tables, and navigation.

Command handlers decide whether to render text/JSON or delegate to the TUI layer. Core scan, diff, audit, bundle, and restore logic should not depend on React or Ink.

## Trust Boundaries

snaptailor's safety model depends on keeping these boundaries intact:

- Scanning reads known local paths and does not execute MCP commands, hooks, scripts, plugins, or agent tools.
- Scanning does not use the network.
- Symlinks are detected but not followed.
- Raw secrets are not stored; secret-like values are redacted or omitted.
- Snapshot store writes are confined to `~/.snaptailor` unless the user gives an explicit output path.
- Content application requires explicit import/restore flags and is constrained by restore policy.
- Restore and bundle import paths must remain project-relative or home-token-aware; path traversal and home-relative bundle writes should be rejected.

## Extension Points

The preferred extension points are:

- Add an agent scanner through `ScannerPlugin`.
- Add parser behavior in `src/parsers.ts` or filesystem scanning when a new file type needs structured capture.
- Add or adjust restore policy in `src/policy.ts`.
- Add command wiring in `src/commands/*` and register it in `src/cli.ts`.
- Add TUI rendering in `src/tui/*` after the core command works in text and JSON mode.

Avoid coupling new features directly to CLI output. The durable contract should be typed data in `src/types.ts`, then command output can format it for humans, JSON consumers, or the TUI.

## Development Checks

Use the standard verification path before shipping architecture-sensitive changes:

```bash
npm run check
```

This runs the TypeScript build and Node test suite. For CLI behavior changes, add a focused test under `tests/*.test.ts` and keep command output compatible with `--json` where applicable.
