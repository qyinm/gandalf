# Gandalf Architecture

Gandalf is a local-first workspace for managing user-global AI coding agent setup. It presents skills, hooks, MCP servers, and plugins in a unified terminal inventory, then uses the same normalized evidence model for snapshots, diffs, audits, reports, `.gandalf` bundles, and restore safety workflows.

The core architectural rule is simple: scan paths are read-only and policy-aware; write paths are explicit, narrow, and reversible where possible.

## Canonical Runtime

`internal/gandalfcore` is the canonical engine. `cmd/gandalf` is the supported CLI entrypoint and `internal/tui` is the Bubble Tea terminal workspace. The default command opens the TUI.

New CLI, engine, and TUI behavior lands in Go. The old Bun/TypeScript CLI, TUI, and core packages have been removed from the supported architecture, and the deprecated Rust engine, CLI, and Tauri desktop transition path are no longer active repository surfaces.

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

## Runtime Entry Points

- `cmd/gandalf` is the primary CLI (`go build -o bin/gandalf ./cmd/gandalf`). Command wiring lives in `internal/cli`; it exposes scan, snapshot, diff, restore, doctor, report, timeline, bundle, and TUI subcommands.
- `internal/gandalfcore` holds engine logic: scanners, store, snapshot, graph, diff, audit, provenance, restore, bundle, timeline, readiness, and report rendering.
- `internal/tui` is the Bubble Tea presentation layer over typed Go engine APIs. It must not own scan, restore, or bundle business logic.
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

The scan package builds a scanner context from project path, home directory, and store directory, then executes the registered scanner plugins. The active default scan returns user-global and managed evidence; project-scoped evidence is excluded from the default product path. Target-based scanners declare files or directories to inspect; custom scanners implement direct discovery when an agent needs more than static file targets.

Supported built-in scanner modules currently cover Claude Code, Codex, Cursor, OpenCode, and Pi Agent user-global setup surfaces.

Scanner plugins should emit typed evidence without executing referenced MCP commands, hooks, scripts, plugins, or agent tools.

## Restore Pipeline

Restore remains conservative:

- planning compares a baseline snapshot to current evidence;
- apply requires explicit apply and experimental flags where the command contract requires them;
- writes are path-confined to declared home and project roots;
- symlink write targets are refused;
- rollback paths are created where supported;
- unsupported evidence kinds stay observe-only rather than falling back to generic mutation.

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
- Gate 2 acceptance: `node scripts/gate2-acceptance.mjs`

Landing-site checks remain for `apps/landing`, but they are not CLI distribution paths.
