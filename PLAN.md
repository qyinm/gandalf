<!-- /autoplan restore point: /Users/hippoo/.gstack/projects/snaptailor/no-branch-autoplan-restore-20260512-000950.md -->

# snaptailor Plan

Source: [PRODUCT.md](PRODUCT.md)

## Final Direction

snaptailor v0.1 is a local-first, read-only drift diagnosis and security audit tool for AI coding agent setups.

The product should not start as "Docker image for agents" or "restore my whole agent environment." That promise is too broad, too risky, and too close to dotfile backup. The sharp wedge is:

> Tell me exactly what changed in my AI agent setup, where it came from, why it matters, and what might be risky.

Snapshot and restore remain part of the long-term product, but v0.1 proves the differentiated layer first: semantic diff, provenance, reproducibility gaps, and risk findings.

## Target User

Initial operator: a developer who runs Claude Code, Codex, Cursor, MCP servers, custom skills, project instructions, hooks, and local scripts across multiple repos.

Concrete painful moment:

> Yesterday this repo worked well with my coding agent. Today the output is worse or riskier. I changed prompts, MCPs, skills, permissions, and local config, and I need to know what changed.

## Adjacent Landscape

- chezmoi already manages dotfiles with templates, encryption, scripts, and multi-machine sync. snaptailor should not compete as generic dotfile management. Source: https://www.chezmoi.io/
- Claude Code has hierarchical settings, MCP config, skills, subagents, memory files, and permission rules. Agent state is already multi-layered. Source: https://docs.claude.com/en/docs/claude-code/settings
- Claude Code and Cursor expose MCP configuration as project/user state with different file locations and scope semantics. Sources: https://docs.claude.com/en/docs/claude-code/mcp and https://docs.cursor.com/advanced/model-context-protocol
- Codex CLI has local config and MCP support, so the long-term product should compare across agents. Source: https://platform.openai.com/docs/docs-mcp
- AGHub is already near "unified MCP and portable skills." snaptailor must differentiate on read-only drift explanation, provenance, and security audit, not MCP toggles. Source: https://aghub.akr.moe/

## Product Promise Boundaries

### v0.1 Promise

- Read local user and project agent config.
- Never execute hooks, MCP commands, scripts, plugins, or agent tools.
- Never mutate user/project config.
- Never use network by default.
- Produce an evidence inventory, semantic diff, provenance report, and risk audit.
- Make unsupported or non-reproducible state visible instead of pretending it was captured.

### Not v0.1

- `restore --apply`
- Importing or applying bundles from other people.
- Sharing complete snapshots.
- Raw file-content bundles.
- Cloud sync.
- Desktop UI.
- "Complete agent state" claims.

## Core Commands

```bash
snaptailor scan --project .
snaptailor scan --project . --explain
snaptailor snapshot create --name baseline --metadata-only --project .
snaptailor snapshot list
snaptailor snapshot show baseline --json
snaptailor diff baseline current --project .
snaptailor audit current --project .
snaptailor provenance current --project . --json
snaptailor report current --project . --out snaptailor-report.md
```

`current` is a pseudo-snapshot generated from a fresh read-only scan. It is never stored unless the user explicitly creates a snapshot.

## First Five Minutes

The first useful moment must not require a before/after diff.

```bash
npm install -g snaptailor
snaptailor scan --project .
```

Expected first-run output:

```text
snaptailor scan

Read-only: yes
Network: disabled
Commands executed: none
Writes: ~/.snaptailor/index only

Detected agents
  Claude Code  user config found, project config found
  Codex        user config found, AGENTS.md missing
  Cursor       project MCP config found

High-signal findings
  HIGH   Claude Code permission wildcard added in project settings
  MED    MCP server "github" command changed since last baseline
  MED    Skill folder contains executable hook-like script

Blind spots
  Remote MCP server behavior cannot be captured
  Provider-side model routing cannot be verified
  Raw env values are omitted by policy

Next
  snaptailor snapshot create --name baseline --metadata-only --project .
```

Target time to first useful report: under 60 seconds.

## Supported Surface In v0.1

Support two high-signal surfaces deeply, with a scanner plugin interface for later expansion:

- Claude Code deep support: `~/.claude/settings.json`, `~/.claude.json` metadata only, `~/.claude/agents/`, `~/.claude/skills/`, project `.claude/`, project `.mcp.json`, `CLAUDE.md`.
- Project-local agent context: `AGENTS.md`, `CLAUDE.md`, `.mcp.json`, `.cursor/mcp.json`, `.claude/settings.json`, `.codex/` if present.
- Codex and Cursor v0.1 support is metadata-level unless the scanner can resolve semantics safely.
- Generic include paths are opt-in only.
- `.env` handling is key inventory only by default: key names, file presence, and capture status. No raw values and no unsalted raw hashes.

## Evidence Inventory Contract

v0.1 snapshots are metadata-first. Raw content is captured only for known-safe structured fields.

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
  "restorePolicy": "not_supported_v0_1",
  "captureStatus": "captured",
  "confidence": "high"
}
```

### Capture Status Values

- `captured`: safe structured value captured.
- `redacted`: value exists but content omitted.
- `omitted`: value intentionally not stored.
- `parse_failed`: file existed but could not be parsed.
- `unsafe_to_export`: evidence can be used locally but must not leave the machine.
- `unsupported`: detected, but semantics are unknown.

## Architecture

```text
                 +-------------------------+
                 | snaptailor CLI          |
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
               | DiscoveredItem + capture policy  |
               +----------------+-----------------+
                                |
                                v
               +----------------------------------+
               | Normalized agent-state graph     |
               | source/scope/precedence/confidence|
               +---------+------------+-----------+
                         |            |
                         v            v
          +---------------------+  +----------------------+
          | Semantic diff engine|  | Audit rules engine   |
          +----------+----------+  +----------+-----------+
                     \                       /
                      v                     v
                 +-----------------------------+
                 | Report + JSON outputs       |
                 +-----------------------------+
```

## Semantic Diff Model

Diff both raw source changes and effective resolved state.

Identity fields:

- `agent`
- `scope`
- `sourcePath`
- `entityKind`
- `entityName`
- `effectiveValue`
- `overriddenBy`
- `confidence`

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
- `WORLD_WRITABLE_STORE`: local snaptailor store permissions are unsafe.

Each finding includes:

```json
{
  "code": "PERMISSION_WILDCARD_ADDED",
  "severity": "high",
  "problem": "Project settings added a broad shell permission.",
  "cause": ".claude/settings.json contains Bash(*)",
  "fix": "Replace with explicit allowed commands.",
  "path": ".claude/settings.json",
  "evidenceId": "claude.project.permissions.shell"
}
```

## Trust And Security Model

- Local only by default.
- No telemetry in v0.1.
- No command execution while scanning.
- No network access while scanning.
- Store path: `~/.snaptailor`, created with `0700`.
- Write only to `~/.snaptailor` unless `--out` is explicitly provided.
- Never follow symlinks by default. Record symlink metadata only.
- Reject world/group-writable snapshot stores.
- Limit scan size, file count, depth, and parse time.
- Ignore common large or irrelevant directories: `node_modules`, `.git`, build outputs, caches, logs, model weights.
- Use HMAC or omit fingerprints for sensitive values. Do not store unsalted hashes of secrets.
- Export, import, restore, and share are not implemented in v0.1.

## Error Contract

Every user-facing error must include:

- `code`
- `problem`
- `cause`
- `fix`
- `path` when relevant

Example:

```text
SNAPTAILOR_PARSE_FAILED
Problem: Could not parse Codex config.
Cause: TOML syntax error at ~/.codex/config.toml:12.
Fix: Run `snaptailor scan --skip codex` or fix the TOML file.
```

## Implementation Plan

### Milestone 0: Validation And Repo Scaffold

- Collect 10 real "agent behavior changed" incidents from the target operator.
- Manually reconstruct what changed and classify whether snaptailor could detect it.
- Choose TypeScript for fastest parser iteration and CLI distribution, with minimal dependencies and a lockfile.
- Add CLI scaffold, tests, lint/typecheck, fixture harness, and temp-home integration test harness.

### Milestone 1: Read-Only Scan

- Implement trust preflight and `scan --explain`.
- Implement path discovery for Claude Code and project-local agent files.
- Emit evidence inventory with capture status.
- Enforce no symlink following, size caps, parse timeouts, and no network/command execution.

### Milestone 2: Metadata Snapshot Store

- Create `~/.snaptailor` with `0700`.
- Store metadata-only snapshots.
- Add `snapshot create --metadata-only`, `list`, and `show --json`.
- Add checksums for observed files and safe structured fields.

### Milestone 3: Normalized Graph And Diff

- Build agent-state graph with scope, precedence, confidence, and source references.
- Implement `current` pseudo-snapshot.
- Implement semantic diff plus raw source-change summary.
- Add `--json` to scan, snapshot show, diff, audit, provenance, and report.

### Milestone 4: Audit And Provenance

- Implement audit rules listed above.
- Implement provenance report showing where every effective value came from.
- Implement reproducibility gap report: captured, redacted, omitted, remote, unsupported.
- Generate `snaptailor-report.md`.

### Milestone 5: Docs And Dogfood

- Add copy-paste workflows:
  - inspect what will be scanned
  - create first baseline
  - see what changed since baseline
  - audit current setup
  - export a redacted report, not a restorable bundle
  - use JSON in CI or agent workflows
- Dogfood on at least three real agent setups before adding write features.

## Test Diagram

```text
scan --project .
  |
  +-- trust preflight
  |     +-- store path exists? permissions 0700?          [unit + temp-home]
  |     +-- no network / no commands contract visible     [CLI snapshot]
  |
  +-- discover paths
  |     +-- existing Claude user/project config           [fixture]
  |     +-- missing config                                [fixture]
  |     +-- unreadable file                               [temp-home]
  |     +-- symlink / broken symlink / symlink loop        [temp-home]
  |     +-- huge folder / ignored dirs                    [temp-home]
  |
  +-- parse evidence
  |     +-- valid JSON/TOML/Markdown frontmatter          [golden]
  |     +-- malformed JSON/TOML                           [golden]
  |     +-- binary/large file skipped                     [fixture]
  |     +-- secret-like value omitted                     [unit + golden]
  |
  +-- normalize graph
  |     +-- user vs project precedence                    [unit]
  |     +-- unsupported state retained as unsupported     [unit]
  |
  +-- diff baseline current
  |     +-- MCP added/removed/changed                     [golden]
  |     +-- permission wildcard added                     [golden]
  |     +-- skill executable appeared                     [golden]
  |
  +-- audit current
        +-- finding has code/problem/cause/fix/path       [unit]
        +-- JSON schema stable                            [schema test]
        +-- markdown report includes blind spots          [golden]
```

## Failure Modes Registry

| Failure mode | Severity | v0.1 response |
|---|---:|---|
| Tool leaks raw secret in snapshot/report | Critical | Central capture policy, omit unknown sensitive values, fixture tests |
| Scanner follows symlink into unrelated secret tree | Critical | Do not follow symlinks by default, record metadata only |
| Semantic diff misses project override | High | Graph includes scope, precedence, source, confidence |
| Snapshot store is tampered with | High | `0700` store, checksums, unsafe permission finding |
| Malformed config crashes scan | High | Structured parser diagnostics and `parse_failed` evidence |
| Huge folder makes scan unusable | Medium | Size/count/depth caps, ignored dirs, explicit exclusions |
| User thinks "complete state" was captured | High | Reproducibility gap report and no complete-state claim |
| Audit report is too noisy | Medium | Severity levels, confidence, high-signal initial rules |

## What Already Exists

- Dotfile managers solve generic config sync.
- Agent vendors expose some settings export/import inside their own tools.
- MCP managers solve server toggling and install flow.
- snaptailor's unique work is cross-scope, cross-agent explanation: semantic drift, provenance, risk, and unsupported-state visibility.

## NOT In Scope

- Mutating config files.
- Applying snapshots.
- Importing third-party bundles.
- Sharing restorable environments.
- Raw secret capture.
- Cloud-hosted team management.
- Desktop UI.
- Marketplace or skill registry.

## Decision Audit Trail

| # | Phase | Decision | Classification | Principle | Rationale | Rejected |
|---|---|---|---|---|---|---|
| 1 | CEO | Reframe v0.1 from restore/share to read-only drift diagnosis and audit | User Challenge | P1 + P2 | Both external voices flagged restore-first as strategically and technically risky; read-only diagnosis proves the differentiated wedge first | Backup/restore-first MVP |
| 2 | CEO | Narrow initial operator to developers running coding agents across repos | Auto | P5 | A concrete operator makes the first workflow and copy-paste docs testable | Broad "AI power user" segment |
| 3 | Eng | Use metadata-only snapshots in v0.1 | Auto | P1 | Avoids raw secret and supply-chain exposure while preserving diff/provenance value | Raw file-content bundles |
| 4 | Eng | Add policy-aware `DiscoveredItem` before manifest/diff/report | Auto | P5 | Redaction and capture rules must be central, not downstream cleanup | Scanner emits raw bytes directly |
| 5 | Eng | Defer import/export/restore/share | User Challenge | P1 + P3 | These require write safety and malicious bundle handling before the core value is proven | Implement `.stailor` bundles in v0.1 |
| 6 | DX | Make `snaptailor scan --project .` the first useful report | Auto | P5 | First run must create value and trust before asking for baselines | Install, create snapshot, then empty diff |
| 7 | DX | Add explicit error contract and stable JSON outputs | Auto | P1 | Developer tool adoption depends on automation and fixable errors | Human-only prose output |

## Review Scores

- CEO: High concern on original plan. Revised direction accepted as the stronger wedge.
- Design: Skipped. No UI surface in v0.1.
- Eng: Original plan had critical restore/share/security risks. Revised plan reduces v0.1 blast radius to read-only scanning, graph, diff, audit, and report.
- DX: Original first-use flow could be empty. Revised flow makes `scan` valuable under 60 seconds and adds trust preflight.

## Cross-Phase Themes

- Restore/share before diagnosis is the wrong first move. CEO, Eng, and DX all flagged it.
- "Complete state" is an unsafe promise. The plan now uses evidence inventory and reproducibility gaps.
- Trust is the product. No network, no command execution, no mutation, explicit capture status, and local-only defaults are v0.1 requirements.

## GSTACK REVIEW REPORT

| Review | Command | Scope | Runs | Status | Findings |
|---|---|---|---:|---|---|
| CEO Review | `/plan-ceo-review` via `/autoplan` | Strategy and scope | 1 | issues_resolved_in_plan | Restore-first challenged; wedge reframed to drift diagnosis |
| Design Review | `/plan-design-review` via `/autoplan` | UI/UX | 0 | skipped | No UI scope in v0.1 |
| Eng Review | `/plan-eng-review` via `/autoplan` | Architecture and tests | 1 | issues_resolved_in_plan | Secret handling, symlinks, store trust, restore risk addressed |
| DX Review | `/plan-devex-review` via `/autoplan` | Developer experience | 1 | issues_resolved_in_plan | First useful report and trust preflight added |
| Dual Voices | `/autoplan` | CEO, Eng, DX outside voices | 3 | issues_resolved_in_plan | Strong consensus on read-only diagnostic MVP |

