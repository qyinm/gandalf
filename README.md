# Hem

Rollback safety net for Codex setup experiments.

![Hem product demo](demo/product-demo/product-demo.gif)

Hem's current wedge is narrow on purpose: save, diff, and restore your user-global Codex setup under `~/.codex/` after an agent, MCP, hook, or skill experiment goes wrong.

Use it before you let Codex or another agent change Codex config, install skills, or edit hooks. The broad multi-agent, profile, desktop, team, and cloud product is future direction, not the Gate 2 CLI path.

```bash
bun install -g @qxinm/hem

# Save a Codex user-global restore point
hem snapshot create --name baseline --agent codex --scope user --project .

# See what changed after installing skills/MCPs
hem diff baseline current --agent codex --scope user --project .

# Preview a restore before applying it
hem restore --snapshot baseline --dry-run --agent codex --scope user --project .

# Apply the rollback when the plan looks right
hem restore --snapshot baseline --apply --experimental --agent codex --scope user --project .
```

Package note: `@qxinm/hem` is the npm package because `qxinm` is the publishing account. The source repository remains `qyinm/hem`.

Hem also has broader experimental scan, TUI, restore, and bundle commands. Those are useful for dogfooding, but the current product test is Codex user-global rollback.

**Go engine:** The canonical engine lives in `internal/hemcore` with `cmd/hem` as the CLI. Build, test, and run Gate 2 with:

```bash
make build         # produces bin/hem
make test          # go test ./...
make gate2         # Codex user-global rollback demo
./bin/hem --help
```

Install from source: `go install github.com/qyinm/hem/cmd/hem@latest` (after a tagged release). Prebuilt darwin/linux binaries ship via GitHub Releases on `v*` tags (GoReleaser).

`crates/hem-core`, `crates/hem-cli`, and the Bun `packages/core` / `apps/cli` stacks are deprecated reference implementations during the phased cutover.

```bash
# Machine A: export your setup
hem bundle export --name baseline --out daily.hem --project .

# Machine B: verify, inspect, and preview it safely
hem bundle verify daily.hem
hem bundle inspect daily.hem
hem doctor --project .
hem bundle import daily.hem --dry-run --project .
```

---

## Why Hem

AI coding power users constantly change their agent environment:

- adding MCP servers
- installing skills
- editing prompts and instructions
- changing hooks and permissions
- asking agents to modify the setup for them

The problem is that agent setup changes usually have no history. After a few experiments, it is hard to know what was original, what changed, and what can be safely removed.

Hem gives the supported Codex setup a local rollback history:

- **Current setup**: what is installed right now
- **Snapshot**: a saved point in time
- **Compare**: what changed between two points
- **Restore**: go back to a saved setup
Profiles, bundles, desktop UI, team sync, and cloud sync are future product direction, not the current demand-test wedge.

---

## Trust Contract

By default Hem:

- reads local user and project agent configuration **only**
- does **not** execute MCP commands, hooks, scripts, plugins, or agent tools
- does **not** use the network unless you explicitly opt into an update check with `HEM_UPDATE_CHECK=1`
- writes **only** to `~/.hem`, unless `--out` is explicit
- omits raw secrets and raw `.env` values
- does **not** follow symlinks
- requires explicit apply flags before restoring content
- creates rollback paths for restore operations where supported
- reports missing local tools and env keys without installing packages or restoring secret values

Update notices are off by default. To run a one-off npm registry check, use `HEM_UPDATE_CHECK=1 hem --help`.

---

## Commands

### Local Setup History

```bash
# Discover agents and config files
hem scan --project .
hem scan --project . --explain
hem scan --project . --json

# Save point-in-time state
hem snapshot create --name baseline --agent codex --scope user --project .
hem snapshot create --name baseline --metadata-only --project .
hem snapshot list
hem snapshot show baseline --json

# Compare saved setup with current setup
hem diff baseline current --agent codex --scope user --project .
hem diff baseline current --agent codex --scope user --project . --json

# Restore with preview
hem restore --snapshot baseline --dry-run --agent codex --scope user --project .
hem restore --snapshot baseline --apply --experimental --agent codex --scope user --project .
hem restore --snapshot baseline --apply --rollback --experimental --agent codex --scope user --project .
```

### Local Setup Workspace

```bash
# Inspect local setup history entries, when present
hem timeline list --project .
hem timeline show <id>

# Preview undo for a timeline event without writing files
hem timeline undo <id> --dry-run --json

# Open the local setup workspace
hem tui --project .
```

Timeline undo is P0 dry-run preview only for stored history entries. It reports `writesFiles=false`, shows MCP changes that could be reversed, and keeps skills, hooks, permissions, env keys, and unsupported surfaces as observe-only.

`hem tui` opens a local setup-history workspace with persistent `Profiles`, `Agents`, and `History` navigation. The first screen is `History > All changes` with Current Setup above local history and an `All agents` filter. The `Agents` nav lists detected agents only. Project-scoped evidence appears in Current Setup as `Project` or `(project)`, not as an agent. Agent screens show current setup inventory, snapshots are full setup save points, Save Setup previews deterministic titles before writing, and Compare shows explicit From / To / Scope before side-by-side setup changes.

### Bundle And Move Setups

```bash
# Export current environment to a portable .hem bundle
hem bundle export --name baseline --out daily.hem --project .
hem bundle export --name baseline --out daily.hem --metadata-only --project .

# Safe preview and verification before importing
hem bundle verify daily.hem
hem bundle inspect daily.hem
hem doctor --project .
hem bundle import daily.hem --dry-run --project .

# Experimental content inspection/apply on another machine
hem bundle import daily.hem --apply-content --quarantine --experimental --project .
hem bundle import daily.hem --apply-content --experimental --project .
```

Destructive operations require either `--experimental` or `HEM_EXPERIMENTAL=1`. Bundle content apply refuses known sensitive prefixes and should be previewed with `--dry-run` or `--quarantine` first.

### Diagnosis

```bash
# Security/risk notes
hem audit current --project .
hem audit baseline --json

# Trace evidence to source
hem provenance current --project .

# Export human-readable report
hem report current --project . --out hem-report.md
```

Every command supports `--json` where structured output is useful.

---

## Supported Setup Surfaces

| Surface | Config surface |
|---|---|
| Claude Code | settings.json, .mcp.json, CLAUDE.md, skills, hooks, agents |
| Codex | Current Gate 2 path: user-global `~/.codex/config.toml`, user hooks, user skills, managed plugin skill inventory |
| Cursor | .cursor/mcp.json, skills, hooks |
| OpenCode | config, skills |
| Pi Agent | settings, extensions, skills, themes, prompts, agents, models |
| Project | AGENTS.md, CLAUDE.md, CODE.md, .mcp.json, .env keys |

Scanner plugin interface: add new agents by implementing `ScannerPlugin`. `Project` is a shared setup surface, not an agent in the TUI navigation.

---

## Roadmap

| Milestone | Status |
|---|---|
| Read-only scan, diff, audit, provenance, report | ✅ v0.1 |
| Bundle export/import (`.hem` format) | ✅ v0.2 experimental |
| Restore engine (dry-run, apply, rollback) | ✅ v0.2 experimental |
| TUI setup-history workspace | ✅ v0.3 preview |
| Codex user-global content-backed rollback | current Gate 2 wedge |
| Local multi-profile persistence | future |
| MCP/skills add-remove manager | future |
| Background setup-change daemon | future |
| Cloud profiles and multi-machine sync | future |

---

## Development

### Go (canonical)

```bash
git clone git@github.com:qyinm/hem.git
cd hem
make test            # go test ./...
make build           # bin/hem
make gate2           # Gate 2 acceptance demo
./bin/hem scan --project .
```

### Legacy Bun / TypeScript

```bash
bun install
bun run check        # build + test
bun run typecheck    # TypeScript only, no emit
bun run test         # run tests (requires build first)
bun run desktop:dev  # run the Tauri desktop app in development
```

### Legacy Rust

```bash
cargo test --workspace
cargo run -p hem-cli -- snapshot list
```

## Repository Layout

| Path | Purpose |
|---|---|
| `cmd/hem` | Go CLI entrypoint (`bin/hem`) |
| `internal/hemcore` | Canonical Go engine: scan, store, snapshot, diff, restore, bundle, timeline |
| `internal/cli` | Cobra command handlers |
| `crates/hem-core` | **Deprecated** Rust engine (desktop transition) |
| `crates/hem-cli` | **Deprecated** Rust CLI |
| `packages/core` | **Deprecated** TypeScript engine reference |
| `apps/cli` | **Deprecated** npm `hem` shim (`@qxinm/hem`) |
| `apps/tui` | **Deprecated** Ink/Clack terminal workspace |
| `apps/desktop` | Tauri v2 + Vite desktop dashboard shell |
