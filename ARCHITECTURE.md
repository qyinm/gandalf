# Gandalf Architecture

Gandalf is a local control console for AI agent setup. It inspects installed capabilities, browses agent-native marketplace/source entries, and runs reviewed provider-backed changes for current Codex and Claude Code user-global setup.

The console presents skills, hooks, MCP servers, plugins, agent-native marketplace/source entries, and baseline status in a top-tab terminal setup console, then uses the same normalized evidence model for snapshots, diffs, audits, reports, `.gandalf` bundles, and restore safety workflows.

The core architectural rule is simple: scan paths are read-only and policy-aware; write paths are explicit, narrow, and reversible where possible.

## Canonical Runtime

`internal/gandalfcore` is the canonical engine. `cmd/gandalf` is the supported CLI entrypoint and `internal/tui` is the Bubble Tea terminal workspace. The default command opens the TUI.

New CLI, engine, and TUI behavior lands in Go. The old JavaScript CLI, TUI, and core packages have been removed from the supported architecture, and the deprecated Rust engine, CLI, and Tauri desktop transition path are no longer active repository surfaces.

## System Shape

```text
                +----------------------+
                | gandalf Go CLI       |
                | cmd/gandalf          |
                | internal/cli         |
                +----------+-----------+
                           |
                           v
                +----------------------+
                | gandalfcore Go       |
                | internal/gandalfcore |
                | scan/store/restore/  |
                | bundle/timeline/...  |
                +----------+-----------+
                           |
                           v
                +----------------------+
                | Bubble Tea TUI       |
                | internal/tui         |
                +----------------------+

```

## Active Architecture

The active architecture has five boundaries:

- **Read-only global setup discovery**: scanners inspect current Codex and Claude Code user-global setup without executing MCP commands, hooks, scripts, plugins, agent tools, or network calls.
- **Normalized setup console**: discovered setup becomes unified inventory rows for skills, hooks, MCP servers, plugins, and agent-native marketplace/source entries before the TUI renders it.
- **Provider-backed action boundary**: a visible row is not automatically editable or installable. Mutating actions are available only when a setup action provider can produce a concrete preview and execution path.
- **Review Changes before mutation**: restore-backed applies and provider-backed setup actions must show reviewed changes before writing and must refresh or revalidate the underlying plan before apply.
- **Restore safety backing workflow**: content-backed snapshots, restore planning, path confinement, symlink refusal, rollback, and unsupported-item reporting preserve the trust contract behind the console.

## Runtime Entry Points

- `cmd/gandalf` is the primary CLI (`go build -o bin/gandalf ./cmd/gandalf`). Command wiring lives in `internal/cli`; it exposes scan, snapshot, diff, restore, doctor, report, timeline, bundle, and TUI subcommands.
- `internal/gandalfcore` holds engine logic: scanners, setup inventory and agent-native marketplace/source models, setup action providers, store, snapshot, graph, diff, audit, provenance, restore, bundle, timeline, readiness, and report rendering.
- `internal/tui` is the Bubble Tea presentation layer over typed Go engine APIs. It owns the top-tab setup console interaction state and Review Changes screens but must not own scan, restore, setup action, baseline, or bundle business logic.
- Release binaries for darwin/linux amd64/arm64 are built with GoReleaser (`.goreleaser.yaml`) on `v*` tags via `.github/workflows/release.yml`.
- `install.sh` installs the latest stable release binary from GitHub Releases.
- Homebrew installs use the `qyinm/tap/gandalf` formula generated from GoReleaser into `qyinm/homebrew-tap`.
- The release workflow needs `GORELEASER_GITHUB_TOKEN` with write access to `qyinm/homebrew-tap`; the default repository token is only enough for same-repo release assets.

## Core Data Model

The central Go evidence object is `DiscoveredItem` in `internal/gandalfcore/types`. Scanner plugins emit discovered items with agent, kind, scope, source path, precedence, parser, sensitivity, content policy, restore policy, capture status, and optional structured value metadata.

The rest of the system derives from that inventory:

- Graph construction lives in `internal/gandalfcore/graph`.
- Audit findings live in `internal/gandalfcore/audit`.
- Provenance entries live in `internal/gandalfcore/provenance`.
- Snapshot and timeline persistence live in `internal/gandalfcore/store`.
- Bundle import/export lives in `internal/gandalfcore/bundle`.
- Restore planning and apply logic live in `internal/gandalfcore/restore`.

## Scan Pipeline

The scan package builds a scanner context from project path, home directory, and store directory, then executes the registered scanner plugins. The active default scan returns user-global and managed evidence for the current supported agent set; project-scoped evidence is excluded from the default product path. Target-based scanners declare files or directories to inspect; custom scanners implement direct discovery when an agent needs more than static file targets.

The current product-visible scanner set covers Claude Code and Codex user-global setup surfaces. Legacy scanner implementations for other agents can remain in the repository for compatibility or direct parser tests, but they are not registered in the active default product scan.

Scanner plugins should emit typed evidence without executing referenced MCP commands, hooks, scripts, plugins, or agent tools. Agent-native marketplace/source discovery is browse/inspect inventory unless a provider-backed action explicitly supports install, update, uninstall, add-source, or remove-source.

## Setup Action Boundary

Setup action providers are the only layer that can mark an inventory action available. A provider-backed action must describe the target, expected effect, Review Changes preview, and execution mechanism before the TUI or CLI can offer it as executable.

Rows without providers can still appear in inventory and source browsing. Their edit, remove, install, update, uninstall, add-source, or remove-source actions must be unavailable with a concrete reason rather than falling through to a descriptive no-op.

## Restore Pipeline

Restore remains conservative:

- Review Changes compares an agent-scoped baseline snapshot to current evidence;
- apply creates a pre-apply restore point before user-global restore writes;
- apply requires explicit apply and experimental flags where the command contract requires them;
- writes are path-confined to declared home and project roots;
- symlink write targets are refused;
- rollback paths are created where supported;
- unsupported evidence kinds stay observe-only rather than falling back to generic mutation.

Restore is the backing safety workflow for the local control console. It should not be used as the sole product definition, and the current Gate 2 acceptance script should remain a restore-safety regression until a follow-up code PR renames or splits it.

The Trust Contract in `README.md` is the user-facing version of these rules.

## Distribution

Supported CLI distribution channels are:

- `install.sh`, which downloads the latest stable GitHub Release binary;
- Homebrew, through `brew install qyinm/tap/gandalf`;
- source builds for contributors via `go install` or `make build`.

npm is no longer a supported distribution channel in this repository. External npm package removal is a registry operation handled outside this codebase.

## Test and CI Posture

CI must keep the supported runtime green:

- Go tests: `go test ./...`
- Go build: `go build -o bin/gandalf ./cmd/gandalf`
- install script smoke: `./scripts/install-smoke.sh`
- Gate 2 restore-safety regression: `./scripts/gate2-acceptance.sh` (current script name retained until a follow-up code PR renames or splits it)

Landing-site checks live in the dedicated landing repository, not in this CLI/TUI repository.
