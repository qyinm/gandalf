# Hem

Time Machine for your AI coding agent setup.

Hem helps you view, save, compare, and restore the MCP servers, skills, hooks, permissions, instructions, and agent configs used by Claude Code, Codex, Cursor, OpenCode, and Pi Agent.

Use it when you let agents change their own setup, experiment with MCPs and skills, or move your agent environment to a new machine.

```bash
npm install -g @qxinm/hem

# Save a restore point
hem snapshot create --name baseline --metadata-only --project .

# See what changed after installing skills/MCPs
hem diff baseline current --project .

# Keep a local daemon timeline of setup changes
hem daemon start --project . --json
hem tui

# Preview a restore before applying it
hem restore --snapshot baseline --dry-run --project .
```

Hem can also export a saved setup as a portable `.hem` bundle.

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

Hem gives that setup a local history:

- **Current setup**: what is installed right now
- **Snapshot**: a saved point in time
- **Compare**: what changed between two points
- **Restore**: go back to a saved setup
- **Profile**: a named setup line, like `default`, `frontend`, or `clean-baseline`
- **Bundle**: a portable `.hem` file for moving a setup between machines

---

## Trust Contract

By default Hem:

- reads local user and project agent configuration **only**
- does **not** execute MCP commands, hooks, scripts, plugins, or agent tools
- does **not** use the network
- writes **only** to `~/.hem`, unless `--out` is explicit
- omits raw secrets and raw `.env` values
- does **not** follow symlinks
- requires explicit apply flags before restoring content
- creates rollback paths for restore operations where supported
- reports missing local tools and env keys without installing packages or restoring secret values

---

## Commands

### Local Setup History

```bash
# Discover agents and config files
hem scan --project .
hem scan --project . --explain
hem scan --project . --json

# Save point-in-time state
hem snapshot create --name baseline --metadata-only --project .
hem snapshot list
hem snapshot show baseline --json

# Compare saved setup with current setup
hem diff baseline current --project .
hem diff baseline current --project . --json

# Restore with preview
hem restore --snapshot baseline --dry-run --project .
hem restore --snapshot baseline --apply --experimental --project .
hem restore --snapshot baseline --apply --rollback --experimental --project .
```

### Daemon Timeline

```bash
# Start local setup history capture
hem daemon start --project . --json

# Check daemon trust/status metadata
hem daemon status --project . --json

# Inspect captured setup changes
hem timeline list --project .
hem timeline show <id>

# Preview undo for a timeline event without writing files
hem timeline undo <id> --dry-run --json

# Open the Timeline-first TUI
hem tui --project .
```

Timeline undo is P0 dry-run preview only. It reports `writesFiles=false`, shows MCP changes that could be reversed, and keeps skills, hooks, permissions, env keys, and unsupported surfaces as observe-only.

`hem tui` opens a local setup-history workspace with persistent `Profiles`, `Agents`, and `History` navigation. The first screen is `History > All changes` with Current Setup above Timeline and an `All agents` filter. The `Agents` nav lists detected agents only. Project-scoped evidence appears in Current Setup as `Project` or `(project)`, not as an agent. Agent screens show current setup inventory, snapshots are full setup save points, Save Setup previews deterministic titles before writing, and Compare shows explicit From / To / Scope before side-by-side setup changes.

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
| Codex | .codex/config.toml, AGENTS.md, MCP config |
| Cursor | .cursor/mcp.json |
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
| Daemon timeline and Timeline-first TUI | ✅ v0.3 preview |
| Local multi-profile persistence | 📋 next |
| MCP/skills add-remove manager | 📋 future |
| Cloud profiles and multi-machine sync | 📋 Pro |

---

## Development

```bash
git clone git@github.com:qyinm/hem.git
cd hem
npm install
npm run check        # build + test
npm run typecheck    # TypeScript only, no emit
npm test             # run tests (requires build first)
```
