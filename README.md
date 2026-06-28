<h1 align="center">Gandalf</h1>

<p align="center">
  <img alt="Gandalf - AI agent environment management" src="https://shieldcn.dev/header/surface.svg?title=Gandalf&subtitle=AI%20agent%20environment%20management&align=center">
</p>

<p align="center">
  <strong>AI agent environment management.</strong>
</p>

<p align="center">
  Manage the user-global setup layer Git does not track:
  MCP servers, skills, hooks, prompts, permissions, and agent config.
</p>

<p align="center">
  <a href="https://github.com/qyinm/gandalf/actions/workflows/ci.yml"><img alt="CI" src="https://shieldcn.dev/github/qyinm/gandalf/ci.svg"></a>
  <a href="https://github.com/qyinm/gandalf/releases"><img alt="Release" src="https://shieldcn.dev/github/qyinm/gandalf/release.svg"></a>
  <a href="https://github.com/qyinm/gandalf/blob/main/LICENSE"><img alt="License" src="https://shieldcn.dev/github/qyinm/gandalf/license.svg"></a>
  <a href="https://github.com/qyinm/homebrew-tap/blob/main/Formula/gandalf.rb"><img alt="Homebrew tap" src="https://shieldcn.dev/badge/homebrew-qyinm%2Ftap%2Fgandalf-2ea44f.svg"></a>
</p>

---

## Contents

- [Why Gandalf](#why-gandalf)
- [Highlights](#highlights)
- [Install](#install)
- [Quick Start](#quick-start)
- [Commands](#commands)
- [Trust Contract](#trust-contract)
- [Development](#development)

## Why Gandalf

Agent power users constantly change their local environment:

- add MCP servers
- install skills and plugins
- edit prompts, instructions, hooks, and permissions
- let an agent modify the setup on their behalf

Those changes usually have no clean management layer. Gandalf opens as a TUI-first setup console for skills, hooks, MCP servers, plugins, and agent marketplace sources across supported agents. Snapshot, diff, bundle, and restore remain the safety layer behind that workflow:

```bash
gandalf
gandalf snapshot create --name baseline --agent codex --scope user --project .
gandalf diff baseline current --agent codex --scope user --project .
```

Use it before you let an agent change config, install skills, edit hooks, or rewrite setup instructions. User-global setup is the active product scope; project-local setup files are outside the current management workflow.

## Highlights

- **Local history** for AI agent environment experiments.
- **Top-tab TUI setup console** for user-global skills, hooks, MCP servers, plugins, and agent marketplace sources.
- **Human-readable diffs** for config, skills, hooks, and MCP servers.
- **Dry-run first restores** with explicit apply flags before writing content.
- **Content-backed snapshots** for the current Codex user-global launch path.
- **Portable bundles** for exporting, verifying, inspecting, and previewing setup state on another machine.
- **Go CLI and Bubble Tea TUI** shipped as a single binary.
- **No npm distribution path**. Gandalf ships through GitHub Releases, `install.sh`, and Homebrew.

## Install

### Homebrew

```bash
brew install qyinm/tap/gandalf
gandalf --help
```

### install.sh

```bash
curl -fsSL https://raw.githubusercontent.com/qyinm/gandalf/main/install.sh | sh
gandalf --help
```

### From Source

```bash
go install github.com/qyinm/gandalf/cmd/gandalf@latest
```

Prebuilt darwin/linux binaries are published on `v*` tags with GoReleaser. The npm package path is no longer supported for this repository.

## Quick Start

Open the global setup console:

```bash
gandalf
```

Create a safe baseline before changing your agent environment. The current safety path uses user-global setup:

```bash
gandalf snapshot create --name clean-codex --agent codex --scope user --project .
```

Compare the baseline with your current setup:

```bash
gandalf diff clean-codex current --agent codex --scope user --project .
```

Preview the rollback plan:

```bash
gandalf restore --snapshot clean-codex --dry-run --agent codex --scope user --project .
```

Apply only after the plan is correct:

```bash
gandalf restore --snapshot clean-codex --apply --experimental --agent codex --scope user --project .
```

## What Gandalf Tracks

| Surface | Supported setup inventory |
|---|---|
| Codex | user-global `~/.codex/config.toml`, user hooks, user skills, managed plugin skill inventory |
| Claude Code | user-global settings, skills, hooks, agents |
| Cursor | user-global MCP servers, skills, hooks |
| OpenCode | user-global config and skills |
| Pi Agent | user-global settings, extensions, skills, themes, prompts, agents, models |

Project-local files such as repo `.mcp.json`, `AGENTS.md`, and `.env` are not part of the current product scope. Broader team sync and cloud workflows are future direction.

## Commands

### Setup History

```bash
# Discover agent environment files
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

### Terminal Workspace

```bash
gandalf timeline list --project .
gandalf timeline show <id>
gandalf timeline undo <id> --dry-run --json
gandalf tui --project .
```

`gandalf` and `gandalf tui` open the global setup console first. The console uses top tabs for Hooks, Plugins, Marketplace, Skills, and MCP Servers. History, snapshots, and timeline undo remain available as secondary safety workflows; timeline undo is dry-run preview only for stored history entries and reports `writesFiles=false`.

### Bundles

```bash
# Export current environment to a portable .gandalf bundle
gandalf bundle export --name baseline --out daily.gandalf --project .
gandalf bundle export --name baseline --out daily.gandalf --metadata-only --project .

# Verify and preview before importing
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
gandalf audit current --project .
gandalf audit baseline --json
gandalf provenance current --project .
gandalf report current --project . --out gandalf-report.md
```

Every command supports `--json` where structured output is useful.

## Trust Contract

By default Gandalf:

- reads local user-global agent configuration only
- does not execute MCP commands, hooks, scripts, plugins, or agent tools
- does not use the network unless `GANDALF_UPDATE_CHECK=1` is set
- writes only to `~/.gandalf`, unless `--out` is explicit
- omits raw secrets and does not manage project `.env` values
- does not follow symlinks
- requires explicit apply flags before restoring content
- creates rollback paths for restore operations where supported
- reports missing local tools and env keys without installing packages or restoring secret values

Update notices are off by default.

## Tech Stack

| Area | Stack |
|---|---|
| CLI | Go, Cobra |
| TUI | Bubble Tea, Bubbles, Lip Gloss |
| Engine | Go packages under `internal/gandalfcore` |
| Release | GoReleaser, GitHub Releases, Homebrew tap |

## Development

### Go

```bash
git clone git@github.com:qyinm/gandalf.git
cd gandalf
make test
make build
make gate2
./bin/gandalf scan --project .
```

### Landing Site

랜딩 페이지는 별도 저장소에서 관리합니다.
랜딩 페이지의 빌드/배포/기여는 분리된 랜딩 레포에서 진행하세요.
[랜딩 레포](https://github.com/qyinm/gandalf-landing)에서 소스 관리하세요.

## Repository Layout

| Path | Purpose |
|---|---|
| `cmd/gandalf` | Go CLI entrypoint |
| `internal/cli` | Cobra command handlers |
| `internal/gandalfcore` | Canonical Go engine: scan, store, snapshot, diff, restore, bundle, timeline |
| `internal/tui` | Bubble Tea terminal workspace |
| `install.sh` | Latest GitHub Release binary installer |
| `.goreleaser.yaml` | Release assets and Homebrew tap formula generation |

## Roadmap

| Milestone | Status |
|---|---|
| Read-only scan, diff, audit, provenance, report | v0.1 |
| Bundle export/import (`.gandalf` format) | v0.2 experimental |
| Restore engine: dry-run, apply, rollback | v0.2 experimental |
| TUI setup console | v0.4 preview |
| Codex user-global content-backed restore | current launch path |
| Local multi-profile persistence | future |
| MCP/skills add-remove manager | future |
| Background setup-change daemon | future |
| Cloud profiles and multi-machine sync | future |

## Contributing

Issues and focused pull requests are welcome. For code changes, run the checks that match the surface you touched:

```bash
make test
make gate2
bun run check
```

For release or installer changes, also run:

```bash
./scripts/install-smoke.sh
```

## License

MIT. See [LICENSE](LICENSE).

## Star History

<a href="https://www.star-history.com/?repos=qyinm%2Fgandalf&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=qyinm/gandalf&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=qyinm/gandalf&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=qyinm/gandalf&type=date&legend=top-left" />
</picture>
</a>
