<!-- /autoplan restore point: /Users/hippoo/.gstack/projects/hem/no-branch-autoplan-restore-20260512-000950.md -->

# Hem Plan

Source: [PRODUCT.md](PRODUCT.md)

## Final Direction

Hem is a **local Time Machine for AI coding agent setups**. It saves, compares, and restores the MCP servers, skills, permissions, hooks, prompts, and agent configurations used by Claude Code, Codex, Cursor, OpenCode, and Pi Agent.

The product wedge is:

> Let AI coding power users experiment with skills and MCPs without losing track of what changed. Save the current setup, compare it with later changes, and restore the setup that worked.

This is a deliberate pivot from the original v0.1 "read-only diagnosis" framing and the later portability-first framing. The read-only scan/diff/audit pipeline remains the trust layer, but the product is not primarily a diagnostic tool or a marketplace. It is a **local agent setup manager with Git-like history**.

Read-only diagnosis was the right first step to build trust and prove the evidence model. The current goal is local history first: inventory, save setup, compare, restore, profiles, and portable `.hem` bundles.

## Target User

AI coding power user who runs Claude Code, Codex, Cursor, OpenCode, or Pi Agent with custom MCP servers, skills, project instructions, hooks, and environment keys.

Concrete moments:

> I asked an agent to install a skill. Now I do not know what changed or what I can safely delete.

> I added a new MCP and my setup got weird. I want to go back to the setup that worked.

> I bought a new Mac. I want my existing Claude/Codex/Cursor setup without rebuilding it from memory.

> I want to see what skills and MCPs are installed across my agents right now.

## Adjacent Landscape

- **chezmoi** manages dotfiles with templates, encryption, scripts, and multi-machine sync. Hem is narrower and deeper: it targets AI agent configuration specifically, with semantic understanding of MCP servers, skill graphs, and permission rules. Source: https://www.chezmoi.io/
- **Claude Code** has hierarchical settings, MCP config, skills, subagents, memory files, and permission rules. Hem captures the full surface and can restore it. Source: https://docs.claude.com/en/docs/claude-code/settings
- **Claude Code and Cursor** expose MCP configuration as project/user state with different file locations and scope semantics. Sources: https://docs.claude.com/en/docs/claude-code/mcp and https://docs.cursor.com/advanced/model-context-protocol
- **Codex CLI** has local config and MCP support. Source: https://platform.openai.com/docs/docs-mcp
- **AGHub** is near "unified MCP and portable skills." hem differentiates on bundle portability, restore safety (rollback), and cross-agent coverage. Source: https://aghub.akr.moe/
- **Docker / Nix / Dev Containers** solve general environment reproducibility. Hem solves the agent-config layer specifically — the files and settings that live in `~/.claude/`, `.mcp.json`, `CLAUDE.md`, etc.

## Product Promise Boundaries

### v0.2 Promise (current)

- Scan and capture agent configurations from 6 agents (Claude Code, Codex, Cursor, OpenCode, Pi Agent, Project).
- Bundle entire or partial agent environments into `.hem` archives.
- Restore bundles on Macs with per-type apply handlers, rollback safety, and readiness preview.
- Read-only audit and diff between snapshots for change detection.
- Content included by default; metadata-only export is available with `--metadata-only`.
- Doctor/preflight reports missing local tools, MCP command availability, unverified remote MCP URLs, and env key gaps without installing packages or restoring secrets.
- Never execute hooks, MCP commands, scripts, plugins, or agent tools during scan.
- Never use network by default.
- Local store: `~/.hem` with `0700` permissions.

### v0.3+ Target

- **Local profiles**: keep named setup lines such as `default`, `frontend`, and `clean-baseline`.
- **Timeline-first TUI**: show current setup, filtered agent history, and Git-log-style snapshots.
- **MCP/skills manager**: add, remove, disable, and preview changes with automatic pre-change snapshots.
- **Fuller environment portability**: export → dry-run → doctor → import makes supported agent setup portable across Macs.
- **Cross-machine path remapping**: source Mac home paths resolve to target Mac home paths.
- **Content bundles as default**: supported content is included by default; metadata-only is an opt-in `--metadata-only` flag.
- **Signed bundles**: verify bundle integrity and provenance before import.
- **Partial restore**: choose which agents/skills/MCPs to restore from a bundle.
- **Env value handling**: safe, encrypted-at-rest env value storage in bundles (with explicit user opt-in).
- **Cross-OS restore**: macOS ↔ Linux path resolution.
- **CI integration**: `hem bundle export` in CI, `hem bundle validate` as a pre-merge check.

### NOT Yet

- Cloud sync / team sharing server.
- Desktop UI.
- Marketplace or skill registry.
- Remote agent execution or orchestration.

## Core Commands

### Diagnostic (v0.1, stable)

```bash
hem scan --project .
hem scan --project . --explain
hem snapshot create --name baseline --metadata-only --project .
hem snapshot list
hem snapshot show baseline --json
hem diff baseline current --project .
hem audit current --project .
hem provenance current --project . --json
hem report current --project . --out hem-report.md
```

### Reproducibility (v0.2, active development)

```bash
# Export current environment to a portable bundle
hem bundle export --name <snapshot> --out <file.hem> --project .

# Import and restore on another machine
hem bundle import <file.hem> --apply-content --project .

# Safe preview before importing
hem doctor --project .
hem bundle import <file.hem> --dry-run --project .
hem bundle inspect <file.hem>

# Snapshot-based restore with rollback
hem restore --snapshot <name> --dry-run --project .
hem restore --snapshot <name> --apply --project .
hem restore --snapshot <name> --apply --rollback --project .
```

`current` is a pseudo-snapshot generated from a fresh read-only scan. It is never stored unless the user explicitly creates a snapshot.

## First Five Minutes

The first useful moment must sell local history immediately.

```bash
bun install -g @qxinm/hem

# Save the setup that works
hem snapshot create --name baseline --metadata-only --project ~/my-project

# After agent-driven setup changes, compare and restore if needed
hem diff baseline current --project ~/my-project
hem restore --snapshot baseline --dry-run --project ~/my-project
```

Expected first-run output:

```text
hem snapshot create

Read-only during scan: yes
Network: disabled
Commands executed: none

Exported agents
  Claude Code  ✓ 12 items (settings, MCP, skills, hooks, instructions)
  Codex        ✓ 3 items (config, MCP, instructions)
  Cursor       ✓ 2 items (MCP config)
  OpenCode     ✓ 4 items (config, skills)
  Pi Agent     ✓ 8 items (extensions, skills, themes, prompts)
  Project      ✓ 5 items (AGENTS.md, .mcp.json, .env keys)

Saved setup: baseline
Captured agents: Claude Code, Codex, Cursor
Title: capture baseline

Next:
  hem diff baseline current --project .
  hem restore --snapshot baseline --dry-run --project .
```

Target time to first saved setup: under 10 seconds.

## Supported Surface

Six agents + project-local context, with a scanner plugin interface for expansion:

- **Claude Code** deep: `~/.claude/settings.json`, `~/.claude.json`, `~/.claude/agents/`, `~/.claude/skills/`, project `.claude/`, project `.mcp.json`, `CLAUDE.md`, hook commands from settings.json.
- **Codex**: `~/.codex/config.toml`, project `.codex/`, `AGENTS.md`.
- **Cursor**: `~/.cursor/mcp.json`, project `.cursor/mcp.json`.
- **OpenCode**: config, skills.
- **Pi Agent**: settings, extensions, skills, themes, prompts, agents, models.
- **Project-local**: `AGENTS.md`, `CLAUDE.md`, `CODE.md`, `.mcp.json`, `.env` (key inventory).
- **Scanner Plugin Interface**: `ScannerPlugin { agentId, agentName, description, targets() }` — add new agents without touching core.

## Evidence Inventory Contract

Snapshots follow a metadata-first model with opt-in content inclusion.

```text
snapshot/
  manifest.json
  evidence.json
  graph.json
  audit-findings.json
  provenance.json
  checksums.json
  redactions.json
```

### Discovered Item Model

Every scanner emits a policy-aware intermediate item:

```json
{
  "id": "claude.project.mcp.github",
  "agent": "claude-code",
  "kind": "mcp_server",
  "sourcePath": ".mcp.json",
  "scope": "project",
  "precedence": 40,
  "parser": "json",
  "sensitivity": "command_config",
  "contentPolicy": "structured_safe_fields_only",
  "restorePolicy": "full_content_supported",
  "captureStatus": "captured",
  "confidence": "high"
}
```

Implementation status: TypeScript model in `src/types.ts`. **restorePolicy is the active development surface — currently defaulting to `"not_supported_v0_1"`, needs per-kind policy mapping.**

### Capture Status Values

- `captured`: safe structured value captured.
- `redacted`: value exists but content omitted.
- `omitted`: value intentionally not stored.
- `parse_failed`: file existed but could not be parsed.
- `unsafe_to_export`: evidence can be used locally but must not leave the machine.
- `unsupported`: detected, but semantics are unknown.

### Restore Policy Values (v0.2+)

- `full_content_supported`: item can be fully captured and restored (e.g., CLAUDE.md, skill files).
- `structured_fields_only`: only structured fields are captured; raw values omitted (e.g., MCP server URLs but not env secrets).
- `key_inventory_only`: only key names, not values (e.g., `.env` keys).
- `not_supported`: item cannot be restored (e.g., remote MCP behavior, provider-side routing).

## Architecture

```text
                 +-------------------------+
                 | Hem CLI          |
                 +------------+------------+
                              |
                              v
                 +-------------------------+
                 | Trust preflight         |
                 | paths, no network, RO   |
                 +------------+------------+
                              |
                              v
    +----------------+   +----------------+   +----------------+
    | Claude scanner |   | Project scanner|   | Plugin scanners|
    +--------+-------+   +-------+--------+   +-------+--------+
             \                 |                    /
              \                v                   /
               +----------------------------------+
               | Evidence inventory               |
               | DiscoveredItem + restore policy  |
               +----------------+-----------------+
                                |
                                v
               +----------------------------------+
               | Normalized agent-state graph     |
               | source/scope/precedence/confidence|
               +---------+------------+-----------+
                         |            |
              +----------+----------+ +----------+-----------+
              |                     | |                      |
              v                     v v                      v
    +------------------+  +------------------+  +----------------------+
    | Semantic diff    |  | Audit rules      |  | Bundle export/import |
    +------------------+  +------------------+  +----------------------+
              |                     |                      |
              v                     v                      v
    +------------------+  +------------------+  +----------------------+
    | Report + JSON    |  | Restore planner  |  | .hem tar bundle  |
    +------------------+  +------------------+  +----------------------+
                                     |
                                     v
                          +----------------------+
                          | Apply + Rollback     |
                          +----------------------+
```

## Semantic Diff Model

Diff both raw source changes and effective resolved state.

Identity fields:

- `agent`, `scope`, `sourcePath`, `entityKind`, `entityName`
- `effectiveValue`, `overriddenBy`, `confidence`

High-signal diffs:

- MCP server added, removed, command changed, transport changed, URL host changed.
- Permission allow/deny changed, wildcard added, dangerous command newly allowed.
- Skill added, removed, source changed, executable file appeared.
- Agent instruction file changed, project instruction now overrides user instruction.
- Model or reasoning config changed.
- Env key added or removed, value omitted.
- Hook or wrapper script appeared, became executable, or changed checksum.
- Unsupported state appeared or disappeared.

## Audit Rules

Initial audit findings:

- `EXECUTABLE_CONFIG_ADDED`: config adds a command or executable hook.
- `REMOTE_MCP_CHANGED`: remote MCP URL or host changed.
- `PERMISSION_WILDCARD_ADDED`: broad allow rule added.
- `SECRET_LIKE_VALUE_OMITTED`: sensitive value detected and omitted.
- `PROJECT_OVERRIDES_USER_POLICY`: project config overrides user-level policy.
- `PARSE_FAILED`: a relevant config file cannot be parsed.
- `SYMLINK_SKIPPED`: symlink found and not followed.
- `UNSUPPORTED_AGENT_STATE`: detected state exists but cannot be interpreted.
- `WORLD_WRITABLE_STORE`: local Hem store permissions are unsafe.

Each finding includes `code`, `severity`, `problem`, `cause`, `fix`, `path`, `evidenceId`.

## Trust And Security Model

- Local only by default. No telemetry.
- No command execution while scanning. No network access while scanning.
- Store path: `~/.hem`, created with `0700`.
- Write only to `~/.hem` unless `--out` is explicitly provided.
- Never follow symlinks by default. Record symlink metadata only.
- Reject world/group-writable snapshot stores.
- Limit scan size, file count, depth, and parse time.
- Ignore common large or irrelevant directories: `node_modules`, `.git`, build outputs, caches, logs, model weights.
- Bundles are metadata-only by default. Content inclusion requires explicit `--include-content` flag.
- Restore is opt-in: requires `--apply` flag. Dry-run is always available.
- Rollback is automatic when `--rollback` is requested; items are undone in reverse execution order.
- Signed bundles and provenance verification planned for v0.3.

## Error Contract

Every user-facing error must include:

- `code`, `problem`, `cause`, `fix`, `path` (when relevant)

Example:

```text
HEM_PARSE_FAILED
Problem: Could not parse Codex config.
Cause: TOML syntax error at ~/.codex/config.toml:12.
Fix: Run `hem scan --skip codex` or fix the TOML file.
```

## Implementation Plan

### Milestone 0: Validation And Repo Scaffold

- [done] TypeScript CLI scaffold, lockfile, build, and Node test harness.
- [done] Shared user-facing error contract implemented and tested.
- [done] Validation incident template and seed backlog documented.
- [done] Replace seed incident patterns with 10 real target-operator incidents.
- [done] Choose TypeScript for fastest parser iteration and CLI distribution.

### Milestone 1: Read-Only Scan

- [done] Implement trust preflight and `scan --explain`.
- [done] Implement path discovery for all supported agents.
- [done] Emit evidence inventory with capture status.
- [done] Enforce no symlink following, size caps, parse timeouts, no network/command execution.

### Milestone 2: Metadata Snapshot Store

- [done] Create `~/.hem` with `0700`.
- [done] Store metadata-only snapshots.
- [done] Snapshot store helpers for create, list, and show.
- [done] Checksums for observed files and safe structured fields.

### Milestone 3: Normalized Graph And Diff

- [done] Build agent-state graph with scope, precedence, confidence.
- [done] Implement `current` pseudo-snapshot.
- [done] Semantic diff plus raw source-change summary.
- [done] `--json` on scan, snapshot show, diff, audit, provenance, report.

### Milestone 4: Audit And Provenance

- [done] 9 audit rules implemented.
- [done] Provenance report showing where every effective value came from.
- [done] Reproducibility gap inputs via capture statuses and blind spots.
- [done] Markdown report renderer.

### Milestone 5: Docs And Dogfood

- [done] Copy-paste workflows in `README.md`.
- [done] Dogfood on three real agent setups.

### Milestone 6: Bundle Format (v0.2)

- [done] `.hem` tar-based bundle format: export, import, inspect.
- [done] Bundle manifest with format version, security metadata, checksums.
- [done] Content inclusion with `--include-content` flag.
- [done] Path traversal hardening: reject `..`, `~/`, absolute paths, `.ssh`.
- [done] Tar security: symlink/hardlink rejection, size limits, checksum validation.

### Milestone 7: Restore Engine (v0.2)

- [done] Restore planner: dry-run plan generation with risk assessment.
- [done] Per-type apply handlers with fail-fast support.
- [done] Rollback engine: LIFO reverse-iteration undo with status tracking.
- [done] `applyWithRollback` orchestration: apply → rollback on failure.
- [done] Status registry for runtime item state queries.
- [done] Human-readable apply/rollback summary formatting.

### Milestone 8: Restore Policy Matrix (v0.3)

- [ ] Map `restorePolicy` per evidence kind: which items are `full_content_supported`, `structured_fields_only`, `key_inventory_only`, or `not_supported`.
- [ ] Wire restore policies into the evidence pipeline so snapshots carry accurate restore metadata.
- [ ] Implement per-kind content capture: read actual file contents for full-content items, structured fields for MCP config, key names only for env.
- [ ] Add restore policy validation: fail bundle export if `not_supported` items would silently lose data.

### Milestone 9: Content Bundles as Default (v0.3)

- [ ] Flip default: `bundle export` includes content by default; `--metadata-only` becomes the opt-in flag.
- [ ] Per-kind content capture: actual file bytes for CLAUDE.md, skill files, settings.json.
- [ ] Bundle size reporting and warnings for large bundles.

### Milestone 10: Cross-Machine Restore (v0.3)

- [ ] Home directory abstraction: store `~/.claude/settings.json` as `{home}/.claude/settings.json` in bundle; resolve to `$HOME` on restore.
- [ ] MCP binary path remapping: detect `npx`, `uvx`, local binary paths and warn if they differ between export and import machines.
- [ ] OS-aware path normalization: macOS `/Users/` ↔ Linux `/home/`.
- [ ] Restore dry-run with machine-specific diff: "on this machine, these items will be different."
- [ ] Cross-machine dogfood: export on macOS, import on Linux (or vice versa).

### Milestone 11: Bundle Security (v0.3)

- [ ] Signed bundles: HMAC or Ed25519 signature on bundle manifest + content.
- [ ] Bundle verification before import: `hem bundle verify <file.hem>`.
- [ ] Trust-on-first-use key management for bundle signing.
- [ ] Quarantine mode: imported bundles are inspected before content is applied.

### Milestone 12: Scanner Expansion & CI

- [ ] Windsurf scanner (if API/surface is stable).
- [ ] Copilot scanner (if config surface is inspectable).
- [ ] CI recipe: `hem bundle export --validate` as pre-merge check.
- [ ] CI recipe: `hem diff baseline current` to detect agent config drift in PRs.

## Test Diagram

```text
bundle export --include-content --project .
  |
  +-- content capture per kind
  |     +-- CLAUDE.md full content          [golden]
  |     +-- MCP config structured fields    [golden]
  |     +-- skill files full content        [fixture]
  |     +-- .env key inventory only         [fixture]
  |
  +-- bundle import --apply-content
  |     +-- cross-machine path resolution   [temp-home]
  |     +-- MCP binary path mismatch warn   [unit]
  |     +-- permission-preserving restore   [temp-home]
  |     +-- idempotent re-import            [unit]
  |
  +-- bundle security
  |     +-- signed bundle verification      [unit]
  |     +-- unsigned bundle warning         [unit]
  |     +-- quarantine mode enforce         [unit]
  |
  +-- restore policy matrix
        +-- per-kind policy mapping         [unit]
        +-- missing policy → safe default   [unit]
        +-- not_supported items → export warn [unit]
```

## Failure Modes Registry

| Failure mode | Severity | Response |
|---|---:|---|
| Bundle leaks raw secret | Critical | Central capture policy, per-kind content rules, fixture tests |
| Scanner follows symlink into secret tree | Critical | Do not follow symlinks by default |
| Restore overwrites user's manual config | High | Dry-run required before apply, rollback available |
| Cross-machine paths don't resolve | High | Path abstraction layer, dry-run machine-diff report |
| MCP binary missing on target machine | High | Pre-import check, warning on missing `npx`/`uvx`/local bins |
| Semantic diff misses project override | High | Graph includes scope, precedence, source, confidence |
| Bundle is tampered with in transit | High | Signed bundles, checksum verification |
| Malformed config crashes scan | High | Structured parser diagnostics and `parse_failed` evidence |
| User thinks env values are included | High | Explicit `--include-content` flag, bundle manifest states content policy |

## What Already Exists

- Dotfile managers solve generic config sync.
- Agent vendors expose some settings export/import inside their own tools.
- MCP managers solve server toggling and install flow.
- Docker/Nix/Dev Containers solve general environment reproducibility.
- Hem's unique work: **Git-like local history for cross-agent AI coding setups, with restore safety and portable profiles**.

## NOT In Scope

- Mutating config files without explicit user intent (always requires `--apply`).
- Raw secret capture without explicit user opt-in.
- Cloud-hosted team management.
- Desktop UI.
- Marketplace or skill registry.

## Decision Audit Trail

| # | Phase | Decision | Classification | Principle | Rationale | Rejected |
|---|---|---|---|---|---|---|
| 1 | CEO | Reframe v0.1 from restore/share to read-only drift diagnosis and audit | Superseded | See #8 | Original decision: both external voices flagged restore-first as strategically risky. Read-only diagnosis proved the evidence model was sound before adding write paths. | Backup/restore-first MVP |
| 2 | CEO | Narrow initial operator to developers running coding agents across repos | Retained | P5 | A concrete operator makes the first workflow and copy-paste docs testable | Broad "AI power user" segment |
| 3 | Eng | Use metadata-only snapshots in v0.1 | Retained | P1 | Avoids raw secret and supply-chain exposure. Content inclusion is now opt-in via `--include-content`. | Raw file-content bundles as default in v0.1 |
| 4 | Eng | Add policy-aware `DiscoveredItem` before manifest/diff/report | Retained | P5 | Redaction and capture rules must be central, not downstream cleanup | Scanner emits raw bytes directly |
| 5 | Eng | Defer import/export/restore/share | Superseded | See #8 | Original decision: these require write safety before core value is proven. Bundle and restore are now implemented and becoming the core workflow. | Implement `.hem` bundles in v0.1 |
| 6 | DX | Make `hem scan --project .` the first useful report | Retained | P5 | First run must create value and trust. Now supplemented by `bundle export` as the primary first-run workflow. | Install, create snapshot, then empty diff |
| 7 | DX | Add explicit error contract and stable JSON outputs | Retained | P1 | Developer tool adoption depends on automation and fixable errors | Human-only prose output |
| 8 | CEO | **Pivot to local setup history**: Hem's core value is "Time Machine for AI agent setups" | Active (2026-06-07) | P1 + P2 | Read-only diagnosis proved the evidence model. Bundle + restore are useful, but the repeat-use wedge is local history: save, compare, restore, and profile agent setups while experimenting with MCPs and skills. | Portability-only framing as the primary product |

## Review Scores

- CEO: Originally high concern on restore-first plan. Now: **accepted the pivot to reproducibility** — the read-only foundation proved the evidence model, and bundle/restore is already implemented.
- Design: Skipped. No UI surface.
- Eng: Original plan had critical restore/share/security risks. Revised plan addresses these with per-kind restore policies, dry-run gating, signed bundles, and rollback safety.
- DX: First-use flow now offers both `scan` (diagnosis) and `bundle export` (portability) as entry points.

## Cross-Phase Themes

- **Reproducibility is the product.** Diagnosis is the trust layer that makes reproducibility safe.
- **Trust is still the product.** No network, no command execution during scan, no mutation without `--apply`, rollback on failure.
- **"Complete state" remains an unsafe promise.** Evidence inventory and reproducibility gaps are explicit. Some things (remote MCP behavior, provider routing) are inherently non-reproducible.
- **Content bundles will become the default** once per-kind restore policies and signed verification are in place.
