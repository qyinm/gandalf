# Hem Architecture

Hem is a local-first workspace for inspecting, packaging, and restoring AI coding agent environments. It captures agent configuration surfaces such as MCP servers, skills, hooks, permissions, instructions, and project-local agent files into a normalized evidence model, then uses that model for snapshots, diffs, audits, reports, `.hem` bundles, terminal UI, and the desktop dashboard shell.

The core architectural rule is simple: scan paths are read-only and policy-aware; write paths are explicit, narrow, and reversible where possible.

## System Shape

```text
                +----------------------+
                | Hem CLI              |
                | apps/cli/src/cli.ts  |
                +----------+-----------+
                           |
                           v
                +----------------------+
                | Command handlers     |
                | apps/cli/src/commands/* |
                +----------+-----------+
                           |
        +------------------+------------------+
        |                                     |
        v                                     v
+-------------------+              +-------------------+
| Core package      |              | TUI renderers     |
| packages/core/src |              | apps/tui/src/*    |
+---------+---------+              +-------------------+
          |
          v
+-------------------+       +-------------------+
| Scanner plugins   | ----> | DiscoveredItem[]  |
| packages/core/src/scanners/* | | packages/core/src/types.ts |
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
                          | packages/core/src/store.ts |
                          | packages/core/src/report.ts |
                          +-----------+-----------+
                                      |
                    +-----------------+-----------------+
                    |                                   |
                    v                                   v
          +-------------------+              +-------------------+
          | .hem bundles     |              | Restore planner   |
          | packages/core/src/bundle.ts |      | packages/core/src/restore.ts |
          +-------------------+              +-------------------+

                +----------------------+
                | Desktop dashboard    |
                | apps/desktop         |
                +----------------------+
```

## Runtime Entry Points

- `apps/cli/src/cli.ts` is the process entry point and command registry. It maps top-level commands to implementations in `apps/cli/src/commands/*`.
- `apps/cli/src/cli-shared.ts` centralizes flag parsing and runtime options shared across command handlers.
- `apps/cli/src/commands/*` adapts CLI arguments into domain calls. These files should stay thin: parse options, call core modules, format JSON/text/TUI output, and convert errors into user-facing messages.
- `apps/tui/src/*` is a terminal presentation layer. It renders richer Ink/Clack views for supported commands without changing core behavior.
- `apps/desktop` is the Tauri v2 + Vite desktop shell. Its Rust boundary exposes small command APIs to the React frontend; core setup behavior should still live in `packages/core`.

## Core Data Model

The central object is `DiscoveredItem` in `packages/core/src/types.ts`. Scanner plugins emit discovered items with:

- `agent`, `kind`, `scope`, and `sourcePath` to describe where the state came from.
- `precedence` to resolve user-vs-project override behavior.
- `parser`, `sensitivity`, `contentPolicy`, `restorePolicy`, and `captureStatus` to preserve safety decisions alongside the evidence.
- Optional structured `value`, `checksum`, and `metadata`.

At the TypeScript boundary, `DiscoveredItem` is a `kind`-discriminated union so consumers that branch on `mcp_server`, `permission`, `hook`, `env_key`, and other evidence kinds can read known payload fields through typed optional properties. This is a compile-time contract only: serialized snapshot and bundle JSON still uses the same object shape, with `kind` as the existing discriminator and with absent `value` or `metadata` fields left absent.

The rest of the system derives from that inventory:

- `GraphNode[]` is built from evidence in `packages/core/src/graph.ts`.
- `AuditFinding[]` is produced from evidence plus graph context in `packages/core/src/audit.ts`.
- `ProvenanceEntry[]` records source, scope, precedence, confidence, and capture status in `packages/core/src/provenance.ts`.
- Snapshots persist all of the above as JSON files under `~/.hem` through `packages/core/src/store.ts`.

## Scan Pipeline

`scanProject()` in `packages/core/src/scan.ts` constructs a scanner context from `projectPath`, `homeDir`, and `storeDir`, then executes the default scanner plugin list from `packages/core/src/scanners/index.ts`.

There are two scanner styles:

- Target-based scanners declare files/directories to inspect. `packages/core/src/scanners/filesystem.ts` handles file metadata, structured parsing, checksums, redaction, directory bounds, and symlink refusal.
- Custom scanners implement `scan(context)` directly when an agent needs custom discovery logic.

Supported built-in scanner modules currently cover:

- Claude Code: `packages/core/src/scanners/claude-code.ts`
- Codex: `packages/core/src/scanners/codex.ts`
- Cursor: `packages/core/src/scanners/cursor.ts`
- OpenCode: `packages/core/src/scanners/opencode.ts`
- Pi Agent: `packages/core/src/scanners/pi.ts`
- Project-local agent files: `packages/core/src/scanners/project.ts`

The plugin contract lives in `packages/core/src/scanners/scanner-plugin.ts`. New agent support should generally enter through this interface instead of adding special cases to the core scan pipeline.

Cursor, Codex, OpenCode, and Pi Agent use custom scanners when their runtime needs more than a fixed file list. Cursor's scanner reads `.cursor/mcp.json`, Cursor-recognized skill roots, nested project skill roots, and Cursor hook configuration, then emits standard `mcp_server`, `skill`, `hook`, and `unsupported` evidence without executing any referenced commands or scripts.

## Policy Layer

`packages/core/src/policy.ts` is the safety map for capture and restore behavior.

Important policy defaults:

- Agent instructions, agent configs, skills, and extensions can be full-content restore candidates.
- MCP servers, permissions, and hooks are structured-fields-only restore candidates.
- Env files are key-inventory-only.
- Symlinks and unsupported surfaces are not restorable.
- Secret-like keys are redacted or omitted.
- Large or noisy directories such as `.git`, `node_modules`, `dist`, and caches are ignored.

This policy layer is intentionally separate from scanners so every feature downstream sees the same capture and restore decisions.

## Snapshot Store

`packages/core/src/store.ts` owns the local snapshot store. The default store path is `~/.hem`, created with `0700` permissions.

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

`packages/core/src/bundle.ts` implements `.hem` export, import, inspect, and verify.

A `.hem` file is a tar archive with metadata under `.hem/`, normalized snapshot JSON under `snapshot/`, and optional captured file content under `content/`.

Bundle responsibilities include:

- Normalizing home paths to `{home}/...` for cross-machine portability.
- Resolving `{home}` back to the target machine during import.
- Recording source machine metadata such as home directory, platform, and hostname.
- Detecting cross-OS differences and MCP binary availability on import.
- Enforcing size limits for bundle and content entries.
- Computing SHA-256 checksums for archive entries.
- Supporting optional HMAC-SHA256 signatures via `HEM_BUNDLE_KEY` or an explicit key.
- Supporting quarantine import so content can be staged for inspection before writing target files.

Bundle import is not the same as blind restore. Dry-run, verification, quarantine, explicit content apply flags, and experimental gates are part of the trust boundary.

## Restore Architecture

`packages/core/src/restore.ts` has two separate responsibilities:

- Build restore plans by diffing a stored snapshot against a fresh scan of the current project.
- Execute restore items through typed apply handlers and rollback handlers.

Restore planning produces structured `RestorePlanItem` records with action, risk level, current state, target state, diff details, rollback instruction, and confirmation prompt. Unsupported items are carried explicitly instead of disappearing.

Restore apply is sequential and can be configured with `failFast` and rollback behavior. Restore should stay conservative: when policy or content support is unclear, the right behavior is to skip or mark unsupported rather than guessing.

## TUI Architecture

The TUI layer is optional and presentation-only.

- `apps/tui/src/tui-mode.ts` detects when rich output is appropriate.
- `apps/tui/src/wizards/*` contains Clack-based interactive flows for bundle export/import, restore confirmation, and snapshot creation.
- `apps/tui/src/components/*` contains Ink views for scans, audits, diffs, snapshots, provenance, reports, dashboard, errors, tables, and navigation.

Command handlers decide whether to render text/JSON or delegate to the TUI layer. Core scan, diff, audit, bundle, and restore logic should not depend on React or Ink.

## Trust Boundaries

Hem's safety model depends on keeping these boundaries intact:

- Scanning reads known local paths and does not execute MCP commands, hooks, scripts, plugins, or agent tools.
- Scanning does not use the network.
- Symlinks are detected but not followed.
- Raw secrets are not stored; secret-like values are redacted or omitted.
- Snapshot store writes are confined to `~/.hem` unless the user gives an explicit output path.
- Content application requires explicit import/restore flags and is constrained by restore policy.
- Restore and bundle import paths must remain project-relative or home-token-aware; path traversal and home-relative bundle writes should be rejected.

## Extension Points

The preferred extension points are:

- Add an agent scanner through `ScannerPlugin`.
- Add parser behavior in `packages/core/src/parsers.ts` or filesystem scanning when a new file type needs structured capture.
- Add or adjust restore policy in `packages/core/src/policy.ts`.
- Add command wiring in `apps/cli/src/commands/*` and register it in `apps/cli/src/cli.ts`.
- Add TUI rendering in `apps/tui/src/*` after the core command works in text and JSON mode.

Avoid coupling new features directly to CLI output. The durable contract should be typed data in `packages/core/src/types.ts`, then command output can format it for humans, JSON consumers, the TUI, or the desktop app.

## Development Checks

Use the standard verification path before shipping architecture-sensitive changes:

```bash
bun run check
```

This runs the TypeScript build and Bun test suites for the workspace. For CLI behavior changes, add a focused test under `apps/cli/tests/*.test.ts` and keep command output compatible with `--json` where applicable.
