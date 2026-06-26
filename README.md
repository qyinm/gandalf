# Gandalf

Rollback safety net for Codex setup experiments.

Gandalf's current wedge is narrow on purpose: save, diff, and restore your user-global Codex setup under `~/.codex/` after an agent, MCP, hook, or skill experiment goes wrong.

Use it before you let Codex or another agent change Codex config, install skills, or edit hooks. The broad multi-agent, profile, desktop, team, and cloud product is future direction, not the Gate 2 CLI path.

```bash
curl -fsSL https://raw.githubusercontent.com/qyinm/gandalf/main/install.sh | sh

# or, on macOS
brew install qyinm/tap/gandalf

# Save a Codex user-global restore point
gandalf snapshot create --name baseline --agent codex --scope user --project .

# See what changed after installing skills/MCPs
gandalf diff baseline current --agent codex --scope user --project .

# Preview a restore before applying it
gandalf restore --snapshot baseline --dry-run --agent codex --scope user --project .

# Apply the rollback when the plan looks right
gandalf restore --snapshot baseline --apply --experimental --agent codex --scope user --project .
```

Distribution note: Gandalf ships as a Go binary. npm is no longer a supported install path for this repository.

Gandalf also has broader experimental scan, TUI, restore, and bundle commands. Those are useful for dogfooding, but the current product test is Codex user-global rollback.

**Go engine:** The canonical engine lives in `internal/gandalfcore` with `cmd/gandalf` as the CLI. Build, test, and run Gate 2 with:

```bash
make build         # produces bin/gandalf
make test          # go test ./...
make gate2         # Codex user-global rollback acceptance check
./bin/gandalf --help
```

Install from source: `go install github.com/qyinm/gandalf/cmd/gandalf@latest` (after a tagged release). Prebuilt darwin/linux binaries ship via GitHub Releases on `v*` tags (GoReleaser), and are consumed by `install.sh` and the Homebrew tap.

`crates/gandalf-core` and `crates/gandalf-cli` are deprecated Rust reference implementations kept for the desktop transition path. The old Bun/TypeScript CLI, TUI, and core packages are no longer supported distribution paths.

```bash
# Machine A: export your setup
gandalf bundle export --name baseline --out daily.gandalf --project .

# Machine B: verify, inspect, and preview it safely
gandalf bundle verify daily.gandalf
gandalf bundle inspect daily.gandalf
gandalf doctor --project .
gandalf bundle import daily.gandalf --dry-run --project .
```

---

## Why Gandalf

AI coding power users constantly change their agent environment:

- adding MCP servers
- installing skills
- editing prompts and instructions
- changing hooks and permissions
- asking agents to modify the setup for them

The problem is that agent setup changes usually have no history. After a few experiments, it is hard to know what was original, what changed, and what can be safely removed.

Gandalf gives the supported Codex setup a local rollback history:

- **Current setup**: what is installed right now
- **Snapshot**: a saved point in time
- **Compare**: what changed between two points
- **Restore**: go back to a saved setup
Profiles, bundles, desktop UI, team sync, and cloud sync are future product direction, not the current demand-test wedge.

---

## Trust Contract

By default Gandalf:

- reads local user and project agent configuration **only**
- does **not** execute MCP commands, hooks, scripts, plugins, or agent tools
- does **not** use the network unless you explicitly opt into an update check with `GANDALF_UPDATE_CHECK=1`
- writes **only** to `~/.gandalf`, unless `--out` is explicit
- omits raw secrets and raw `.env` values
- does **not** follow symlinks
- requires explicit apply flags before restoring content
- creates rollback paths for restore operations where supported
- reports missing local tools and env keys without installing packages or restoring secret values

Update notices are off by default.

---

## Commands

### Local Setup History

```bash
# Discover agents and config files
gandalf scan --project .
gandalf scan --project . --explain
gandalf scan --project . --json

# Save point-in-time state
gandalf snapshot create --name baseline --agent codex --scope user --project .
gandalf snapshot create --name baseline --metadata-only --project .
gandalf snapshot list
gandalf snapshot show baseline --json

# Compare saved setup with current setup
gandalf diff baseline current --agent codex --scope user --project .
gandalf diff baseline current --agent codex --scope user --project . --json

# Restore with preview
gandalf restore --snapshot baseline --dry-run --agent codex --scope user --project .
gandalf restore --snapshot baseline --apply --experimental --agent codex --scope user --project .
gandalf restore --snapshot baseline --apply --rollback --experimental --agent codex --scope user --project .
```

### Local Setup Workspace

```bash
# Inspect local setup history entries, when present
gandalf timeline list --project .
gandalf timeline show <id>

# Preview undo for a timeline event without writing files
gandalf timeline undo <id> --dry-run --json

# Open the local setup workspace
gandalf tui --project .
```

Timeline undo is P0 dry-run preview only for stored history entries. It reports `writesFiles=false`, shows MCP changes that could be reversed, and keeps skills, hooks, permissions, env keys, and unsupported surfaces as observe-only.

`gandalf tui` opens a local setup-history workspace with persistent `Profiles`, `Agents`, and `History` navigation. The first screen is `History > All changes` with Current Setup above local history and an `All agents` filter. The `Agents` nav lists detected agents only. Project-scoped evidence appears in Current Setup as `Project` or `(project)`, not as an agent. Agent screens show current setup inventory, snapshots are full setup save points, Save Setup previews deterministic titles before writing, and Compare shows explicit From / To / Scope before side-by-side setup changes.

### Bundle And Move Setups

```bash
# Export current environment to a portable .gandalf bundle
gandalf bundle export --name baseline --out daily.gandalf --project .
gandalf bundle export --name baseline --out daily.gandalf --metadata-only --project .

# Safe preview and verification before importing
gandalf bundle verify daily.gandalf
gandalf bundle inspect daily.gandalf
gandalf doctor --project .
gandalf bundle import daily.gandalf --dry-run --project .

# Experimental content inspection/apply on another machine
gandalf bundle import daily.gandalf --apply-content --quarantine --experimental --project .
gandalf bundle import daily.gandalf --apply-content --experimental --project .
```

Destructive operations require either `--experimental` or `GANDALF_EXPERIMENTAL=1`. Bundle content apply refuses known sensitive prefixes and should be previewed with `--dry-run` or `--quarantine` first.

### Diagnosis

```bash
# Security/risk notes
gandalf audit current --project .
gandalf audit baseline --json

# Trace evidence to source
gandalf provenance current --project .

# Export human-readable report
gandalf report current --project . --out gandalf-report.md
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
| Bundle export/import (`.gandalf` format) | ✅ v0.2 experimental |
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
git clone git@github.com:qyinm/gandalf.git
cd gandalf
make test            # go test ./...
make build           # bin/gandalf
make gate2           # Gate 2 acceptance check
./bin/gandalf scan --project .
```

### Frontend and Desktop

```bash
bun install
bun run check        # landing + desktop checks
bun run typecheck    # landing + desktop type checks
bun run test         # desktop tests
bun run desktop:dev  # run the Tauri desktop app in development
```

### Legacy Rust

```bash
cargo test --workspace
cargo run -p gandalf-cli -- snapshot list
```

## Repository Layout

| Path | Purpose |
|---|---|
| `cmd/gandalf` | Go CLI entrypoint (`bin/gandalf`) |
| `internal/gandalfcore` | Canonical Go engine: scan, store, snapshot, diff, restore, bundle, timeline |
| `internal/cli` | Cobra command handlers |
| `internal/tui` | Bubble Tea terminal workspace |
| `crates/gandalf-core` | **Deprecated** Rust engine (desktop transition) |
| `crates/gandalf-cli` | **Deprecated** Rust CLI |
| `apps/desktop` | Tauri v2 + Vite desktop dashboard shell |
| `apps/landing` | Astro landing and docs site |
