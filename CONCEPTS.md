# Concepts

> Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## Restore

### Evidence
A discovered configuration artifact Hem tracks for drift and restore planning. Each evidence record has a kind (config file, MCP server entry, permission rule, env key, etc.), a source path, and optional structured value metadata.

### Evidence Kind
The typed category of an evidence record that determines how restore planning and apply handlers treat it. Kinds with structured JSON values (MCP server, permission, env key) require dedicated apply handlers rather than whole-file replacement.

### Restore Plan
The diff-shaped output of comparing a baseline snapshot to current state. Lists planned items with actions (update, delete), risk metadata, and target state—but does not mutate the filesystem until apply.

### Restore Item
An executable unit derived from a restore plan item. Carries resolved destination path, structured `target_content`, handler `item_type`, and rollback state after apply.

### Apply Handler Registry
The dispatch table mapping restore item types to apply functions. Plan generation and apply execution share type labels; a missing registry entry surfaces as a handler error at apply time even when the plan looks valid.

### Path Confinement
The trust boundary that restricts restore and bundle writes to declared home and project roots. Confinement must be active in plan parsing, apply, and bundle import; callers must supply roots or checks are skipped.

## Snapshots and Store

### Snapshot
A named capture of project and user-global evidence at a point in time. Snapshots may be metadata-only or content-backed depending on capture policy.

### Store
The on-disk persistence layer for snapshots, timeline entries, and related Hem state. Desktop and CLI clients read the same store APIs for snapshot listing and changelog.