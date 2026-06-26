# Dogfood Runs

Date: 2026-05-12 (initial), 2026-05-15 (re-run after symlink noise fix and TOML parser fix)

Command shape:

```bash
make build
GANDALF_STORE=/tmp/gandalf-dogfood/store ./bin/gandalf scan --project <project> --json
GANDALF_STORE=/tmp/gandalf-dogfood/store ./bin/gandalf report current --project <project> --out /tmp/gandalf-dogfood/reports/<name>-report.md
```

The store was redirected to `/tmp/gandalf-dogfood/store` so dogfood did not write to the user's real `~/.gandalf` store.

## Results (2026-05-15, post-fix)

| Project | Scan JSON | Markdown report | Evidence | Findings | Blind spots |
|---|---|---|---:|---:|---:|
| Gandalf | `/tmp/gandalf-dogfood/gandalf-scan.json` | `/tmp/gandalf-dogfood/reports/gandalf-report.md` | 283 | 1 | 3 |
| DuckDocs | `/tmp/gandalf-dogfood/duckdocs-scan.json` | `/tmp/gandalf-dogfood/reports/DuckDocs-report.md` | 283 | 1 | 3 |
| HyprDuck (replaced MirrorNote — workspace no longer present) | `/tmp/gandalf-dogfood/hyprduck-scan.json` | `/tmp/gandalf-dogfood/reports/HyprDuck-report.md` | 283 | 1 | 3 |

All three projects share the same user-level agent configuration and lack project-level agent config files, so they produce identical evidence.

Only finding: `MEDIUM SECRET_LIKE_VALUE_OMITTED` — expected, from a skill directory containing secret-like key names.

### Comparison with 2026-05-12 baseline

| Metric | Before | After | Delta |
|---|---:|---:|---:|
| Evidence items | 243-247 | 283 | +40 (better TOML parsing captures more keys) |
| Audit findings | 108-111 | **1** | -107 (symlink noise suppressed, parse failure fixed) |
| Blind spots | 3 | 3 | unchanged |

## Observations

- The symlink noise fix reduced findings from ~108 to 1, making the default scan output useful.
- The TOML parser fix allowed `~/.codex/config.toml` to be fully parsed (27 key-value pairs extracted), adding 40 more evidence items.
- Reports are still large (79KB) because the provenance section lists every evidence item. This is acceptable for v0.1 but could be summarized in a future iteration.
- No restore/import/share path was needed for dogfood.

## Cross-machine bundle dogfood

Command:

```bash
bun run dogfood:cross-machine
```

This builds a disposable macOS-side snapshot/bundle, then runs `bundle import --dry-run --json` inside a Linux `node:22-bookworm` Docker container with separate `/home/gandalf`, `/linux/project`, and `/linux/store` paths.

Validation checks:

- import remains non-mutating (`--dry-run`, no content applied)
- machine diff reports `darwin → linux`
- `crossOS=true`
- source-local MCP binary paths are reported unavailable on the Linux target

## Local history / TUI dogfood

Command shape:

```bash
HOME=/tmp/gandalf-dogfood/home \
GANDALF_STORE=/tmp/gandalf-dogfood/store \
./bin/gandalf timeline list --project /tmp/gandalf-dogfood/project --json

HOME=/tmp/gandalf-dogfood/home \
GANDALF_STORE=/tmp/gandalf-dogfood/store \
./bin/gandalf timeline undo <id> --project /tmp/gandalf-dogfood/project --dry-run --json
```

Validation checks:

- local history entries remain inspectable when present
- timeline undo remains non-mutating with `writesFiles=false`
- MCP surfaces appear as writable preview items
- skills, hooks, permissions, env keys, and unsupported surfaces remain observe-only
- corrupt timeline files do not hide valid timeline history
- TUI opens on `History > All changes`, keeps Current Setup above Timeline, and exposes `u=preview undo` without writing files

See `docs/dogfood-reports/2026-06-08-main-daemon-timeline-dogfood.md` for the earlier daemon/timeline matrix that informed deferring background capture.

## Gate 2 Codex rollback demo

Command:

```bash
bun run dogfood:gate2
```

This records the launch-path rollback story in a disposable environment:

- creates separate temp `HOME`, `GANDALF_STORE`, and project directories
- writes a clean synthetic `~/.codex/config.toml`
- saves a content-backed Codex user snapshot
- simulates a bad harness install by zero-filling config and adding a synthetic Codex skill
- runs diff, human dry-run restore preview, and restore apply
- verifies the original config is restored and the synthetic skill is removed

This is the primary Gate 2 demo path because it is deterministic and does not depend on a third-party installer changing behavior. A real LazyCodex/harness recording can be added later as a non-blocking appendix.
