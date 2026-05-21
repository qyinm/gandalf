# snaptailor

Portable diagnostics and experimental restore tooling for AI coding agent environments — MCP servers, skills, permissions, hooks, and agent configs.

snaptailor captures your agent setup (Claude Code, Codex, Cursor, OpenCode, Pi Agent) into snapshots and optional `.stailor` bundles. The safe path is inspect/verify/dry-run first; applying bundled content is experimental and project-relative only.

```bash
npm install -g @qxinm/snaptailor

# Machine A: export your setup (content included by default; use --metadata-only to opt out)
snaptailor bundle export --name my-setup --out my-setup.stailor --project .

# Machine B: verify and preview it safely
snaptailor bundle verify my-setup.stailor
snaptailor bundle import my-setup.stailor --dry-run --project .
snaptailor bundle import my-setup.stailor --apply-content --quarantine --experimental --project .
```

snaptailor also includes a full read-only diagnosis pipeline — scan, diff, audit, provenance — so you can see what changed and why before you commit to a restore.

---

## Documentation

- [Architecture overview](ARCHITECTURE.md) explains the CLI entry points, scanner plugin model, evidence graph, bundle format, restore planner, TUI layer, and trust boundaries.

---

## Trust Contract

By default snaptailor:

- reads local user and project agent configuration **only**
- does **not** execute MCP commands, hooks, scripts, plugins, or agent tools
- does **not** use the network
- writes **only** to `~/.snaptailor`, unless `--out` is explicit
- exports bundle content by default; use `--metadata-only` to opt out
- omits raw secrets and raw `.env` values
- does **not** follow symlinks
- snapshot restore requires explicit `--apply`; rollback is available with `restore --rollback`
- bundle content apply requires `--apply-content` plus `--experimental`, is project-relative only, and should be previewed with `--dry-run` or `--quarantine` first

---

## Commands

### Reproducibility (bundle + restore)

```bash
# Export current environment to a portable .stailor bundle (content included by default)
snaptailor bundle export --name <snapshot> --out <file.stailor> --project .
snaptailor bundle export --name <snapshot> --out <file.stailor> --metadata-only --project .

# Safe preview and verification before importing
snaptailor bundle verify <file.stailor>
snaptailor bundle import <file.stailor> --dry-run --project .
snaptailor bundle inspect <file.stailor>

# Experimental content inspection/apply on another machine
snaptailor bundle import <file.stailor> --apply-content --quarantine --experimental --project .
snaptailor bundle import <file.stailor> --apply-content --experimental --project .

# Snapshot-based restore with rollback safety
snaptailor restore --snapshot <name> --dry-run --project .
snaptailor restore --snapshot <name> --apply --project .
snaptailor restore --snapshot <name> --apply --fail-fast --project .
snaptailor restore --snapshot <name> --apply --rollback --project .
```

Destructive operations (`restore --apply`, `bundle import --apply-content`) require either `--experimental` or `SNAPTAILOR_EXPERIMENTAL=1`. Bundle export includes supported file content by default; pass `--metadata-only` to export metadata only. Bundle `--apply-content` refuses home-relative content paths and known sensitive prefixes; use `--quarantine` to inspect content without writing target files.

### Diagnosis (scan + diff + audit)

```bash
# Discover agents and config files
snaptailor scan --project .
snaptailor scan --project . --explain    # show paths considered
snaptailor scan --project . --json       # machine-readable output

# Capture point-in-time state
snaptailor snapshot create --name baseline --metadata-only --project .
snaptailor snapshot list
snaptailor snapshot show baseline --json

# Compare two snapshots
snaptailor diff baseline current --project .       # semantic + raw diff
snaptailor diff baseline current --project . --json

# Security/risk findings
snaptailor audit current --project .
snaptailor audit baseline --json

# Trace evidence to source
snaptailor provenance current --project .

# Export human-readable report
snaptailor report current --project . --out snaptailor-report.md
```

---

## Machine-Readable Output

Every command supports `--json` for structured output.

```bash
snaptailor scan --project . --json
snaptailor diff baseline current --project . --json
snaptailor provenance current --project . --json
snaptailor audit current --project . --json
snaptailor report current --project . --json
snaptailor bundle inspect baseline.stailor --json
```

---

## Supported Agents

| Agent | Config surface |
|---|---|
| Claude Code | settings.json, .mcp.json, CLAUDE.md, skills, hooks, agents |
| Codex | .codex/config.toml, AGENTS.md, MCP config |
| Cursor | .cursor/mcp.json |
| OpenCode | config, skills |
| Pi Agent | settings, extensions, skills, themes, prompts, agents, models |
| Project | AGENTS.md, CLAUDE.md, CODE.md, .mcp.json, .env keys |

Scanner plugin interface: add new agents by implementing `ScannerPlugin`.

---

## Roadmap

| Milestone | Status |
|---|---|
| Read-only scan, diff, audit, provenance, report | ✅ v0.1 (stable) |
| Bundle export/import (.stailor format) | ✅ v0.2 (experimental) |
| Restore engine (dry-run, apply, rollback) | ✅ v0.2 (experimental) |
| Restore policy matrix (per-kind content rules) | ✅ v0.2.1 |
| Content bundles as default | ✅ v0.2.1 |
| Cross-machine path remapping | ✅ v0.2.1 |
| Signed bundle verification | ✅ v0.2.1 |
| Windsurf / Copilot scanners | 📋 future |

---

## Development

```bash
git clone git@github.com:qyinm/snaptailor.git
cd snaptailor
npm install
npm run check        # build + test
npm run typecheck    # TypeScript only, no emit
npm test             # run tests (requires build first)
```
