# snaptailor

Reproducible AI coding agent environment — like a Docker image for your MCP servers, skills, and agent configs.

snaptailor captures your entire agent setup (Claude Code, Codex, Cursor, OpenCode, Pi Agent) into a single `.stailor` bundle. Import it on any machine and get the exact same environment — MCP servers, skills, permissions, hooks, and all.

```bash
npm install -g @qxinm/snaptailor

# Machine A: export your setup
snaptailor bundle export --name my-setup --out my-setup.stailor --include-content --project .

# Machine B: restore it
snaptailor bundle import my-setup.stailor --apply-content --project .
```

snaptailor also includes a full read-only diagnosis pipeline — scan, diff, audit, provenance — so you can see what changed and why before you commit to a restore.

---

## Trust Contract

By default snaptailor:

- reads local user and project agent configuration **only**
- does **not** execute MCP commands, hooks, scripts, plugins, or agent tools
- does **not** use the network
- writes **only** to `~/.snaptailor`, unless `--out` is explicit
- stores **metadata-first** snapshots (content requires explicit `--include-content`)
- omits raw secrets and raw `.env` values
- does **not** follow symlinks
- restores **only** with explicit `--apply` flag; rollback available with `--rollback`

---

## Commands

### Reproducibility (bundle + restore)

```bash
# Export current environment to a portable .stailor bundle
snaptailor bundle export --name <snapshot> --out <file.stailor> --include-content --project .

# Import and restore on another machine
snaptailor bundle import <file.stailor> --apply-content --project .

# Safe preview
snaptailor bundle import <file.stailor> --dry-run --project .
snaptailor bundle inspect <file.stailor>

# Snapshot-based restore with rollback safety
snaptailor restore --snapshot <name> --dry-run --project .
snaptailor restore --snapshot <name> --apply --project .
snaptailor restore --snapshot <name> --apply --fail-fast --project .
snaptailor restore --snapshot <name> --apply --rollback --project .
```

Destructive operations (`--apply`, `--apply-content`, `--include-content`) require either `--experimental` or `SNAPTAILOR_EXPERIMENTAL=1`.

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
| Restore policy matrix (per-kind content rules) | 🚧 v0.3 |
| Content bundles as default | 🚧 v0.3 |
| Cross-machine path remapping | 📋 v0.3 |
| Signed bundle verification | 📋 v0.3 |
| Windsurf / Copilot scanners | 📋 v0.3+ |

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
