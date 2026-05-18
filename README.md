# snaptailor

Read-only drift diagnosis and security audit for AI coding agent setups.

snaptailor reads local agent configuration (Claude Code, Codex, Cursor),
builds a metadata-only evidence inventory, explains what changed, flags risky
state, and can **restore** or **bundle/export** snapshots.

```bash
npm install -g @qxinm/snaptailor
snaptailor scan --project .
```

---

## Trust Contract

By default snaptailor:

- reads local user and project agent configuration **only**
- does **not** execute MCP commands, hooks, scripts, plugins, or agent tools
- does **not** use the network
- writes **only** to `~/.snaptailor`, unless `--out` is explicit
- stores **metadata-first** snapshots
- omits raw secrets and raw `.env` values
- does **not** follow symlinks

---

## Commands

### `scan` — Discover agents and config files

```bash
snaptailor scan --project .
snaptailor scan --project . --explain    # show paths considered
snaptailor scan --project . --json       # machine-readable output
```

### `snapshot` — Capture point-in-time state

```bash
snaptailor snapshot create --name baseline --metadata-only --project .
snaptailor snapshot list
snaptailor snapshot show baseline --json
```

Snapshots are **metadata-only** in v0.1 (`--metadata-only` is required).

### `diff` — Compare two snapshots

```bash
snaptailor diff baseline current --project .       # semantic + raw diff
snaptailor diff baseline current --project . --json
```

`current` is a fresh read-only scan. It is not stored unless you explicitly
create a snapshot.

### `audit` — Security/risk findings

```bash
snaptailor audit current --project .
snaptailor audit baseline --json
```

Detects: executable config additions, remote MCP changes, permission wildcards,
parse failures, skipped symlinks, secret-like keys, unsupported state, and
project-overrides-user policy conflicts.

### `provenance` — Trace evidence to source

```bash
snaptailor provenance current --project .
snaptailor provenance baseline --json
```

### `report` — Export human-readable report

```bash
snaptailor report current --project . --out snaptailor-report.md
snaptailor report baseline --project . --json
```

### `restore` — Plan and apply rollbacks (v0.2+)

```bash
# Dry-run: generate a non-mutating restore plan (JSON)
snaptailor restore --snapshot baseline --dry-run --project .

# Apply: execute restore items (requires --experimental in v0.1)
snaptailor restore --snapshot baseline --apply --experimental --project .

# Apply with fail-fast: stop on first failure
snaptailor restore --snapshot baseline --apply --fail-fast --experimental --project .

# Apply then auto-rollback on failure
snaptailor restore --snapshot baseline --apply --rollback --experimental --project .
```

### `bundle` — Export/import .stailor bundles (v0.2+)

```bash
# Export a snapshot to a .stailor bundle
snaptailor bundle export --name baseline --out baseline.stailor --project .

# Export with raw file contents (requires --experimental)
snaptailor bundle export --name baseline --out baseline.stailor \
  --include-content --experimental --project .

# Import a bundle
snaptailor bundle import baseline.stailor --project .

# Dry-run import
snaptailor bundle import baseline.stailor --dry-run --project .

# Import and apply content files (requires --experimental)
snaptailor bundle import baseline.stailor --apply-content --experimental --project .

# Inspect bundle metadata without unpacking
snaptailor bundle inspect baseline.stailor
```

### SNAPTAILOR_EXPERIMENTAL

Destructive or write-path features (`restore --apply`, `bundle export
--include-content`, `bundle import --apply-content`) require either the
`--experimental` flag or the `SNAPTAILOR_EXPERIMENTAL=1` environment variable.

```bash
SNAPTAILOR_EXPERIMENTAL=1 snaptailor restore --snapshot baseline --apply --project .
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

## Deferred

The following are planned but **not yet implemented**:

- Team sharing / cloud sync
- Desktop UI (TUI/GUI)
- HMAC-based secret fingerprinting
- Signed bundle verification
- Scanner plugin interface (Windsurf, Copilot)
- Cross-OS restore
