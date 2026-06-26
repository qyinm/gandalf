---
title: Rust Core Restore Handler Gaps Found in Migration Code Review
date: 2026-06-26
category: logic-errors
module: hem-core-restore
problem_type: logic_error
component: tooling
symptoms:
  - "MCP, permission, and env restore apply handlers missing from Rust core migration"
  - "Restore writes not confined to home/project roots in CLI apply path"
  - "Permission apply parsed item_id incorrectly and wrote rule wrapper verbatim into settings.json"
  - "Undo handler matched generic filePath before mcp/env-specific branches"
  - "Verification plan step 2 CLI/bundle artifacts missing until verify driver added"
root_cause: logic_error
resolution_type: code_fix
severity: high
tags:
  - rust-core-migration
  - restore-handlers
  - path-confinement
  - hem-core
  - code-review
related_components:
  - testing_framework
  - development_workflow
---

# Rust Core Restore Handler Gaps Found in Migration Code Review

## Problem

The Rust-core migration landed restore/apply, bundle import, CLI restore, and desktop home state, but a code review found gaps between plan generation and execution: structured evidence kinds (MCP servers, permissions, env keys) were planned correctly yet could not be applied or rolled back; path confinement was absent at apply time in some entrypoints; the desktop UI showed placeholder home state instead of store-backed snapshots; and there was no repeatable verification harness for reviewers.

Without these fixes, `hem restore --dry-run` and real apply would emit handler errors, permission restores would write malformed JSON, undo would corrupt MCP/env files, and out-of-root writes were possible.

## Symptoms

| Area | Observable failure |
|------|-------------------|
| Apply registry | `No apply handler for type "mcp_server"`, `"permission"`, `"env_key"`, `"env"` |
| Plan → apply content | `target_content` was `None` for MCP/permission/env plan items |
| Permission apply | Scan ids like `home.claude_code.~/.claude/settings.json.perm-bash` did not resolve to `bash`; `{rule: {...}}` wrapper was written into `settings.json` |
| Undo rollback | Generic `filePath` + `previousContent` ran before MCP/env typed branches |
| Path confinement | Destinations outside home/project were not rejected at CLI apply |
| Desktop home | `currentSnapshotId`, `workingChanges`, and `changelog` were not store-backed |
| Verification | No single script produced `cli-restore-dryrun.out` and `bundle-verify-*.out` |

## What Didn't Work

- **Filtering the e2e test to only three item types** proved handlers in isolation but hid full-plan regressions. The accepted test applies all parsed plan items and allows only expected `agent_config` metadata-only failures. (session history)
- **`cargo run` redirect for CLI evidence** mixed compiler warnings with stdout. Fix: build once, then invoke `target/debug/hem` directly in `scripts/verify-review-patches.sh`. (session history)
- **`item.item_id.split('.').next_back()` for permission names** produced wrong keys on real scan ids. Replaced with metadata-first resolution and `.perm-` marker parsing.

## Solution

### 1. Register apply/undo handlers for structured kinds

`default_apply_handler_registry` maps `mcp_server`, `permission`, `env_key`, and `env` to dedicated handlers; undo registry uses `restore_previous_content_undo_handler` for the same kinds.

### 2. Structured `target_content` in plan parsing

`target_content_for_plan_item` clones full JSON values for `McpServer`, `Permission`, and `EnvKey` evidence—not only string bodies.

### 3. Permission name resolution and rule unwrapping

```rust
fn permission_name_for_item(item: &RestoreItem) -> Result<String, String> {
    // metadata.permissionName → metadata.permissionKey → ".perm-" suffix on id/path
}

fn permission_rule_value_for_apply(value: Value) -> Value {
    if let Some(rule) = value.as_object().and_then(|o| o.get("rule")) {
        return rule.clone();
    }
    value
}
```

### 4. Undo handler: typed branches before generic `filePath`

Check `mcp_server` (`mcpPath` + `mcpConfig`) and `env_key`/`env` (`envPath`) before the generic file-backed branch.

### 5. Path confinement at parse, apply, and bundle import

`path_confinement.rs` provides `confinement_roots_from_paths` and `validate_constrained_write_path`. Wire into `apply_restore_items`, `parse_dry_run_output`, and bundle content apply. CLI restore must pass `home_dir` and `project_path` in `ApplyOptions`.

### 6. Desktop home state from store APIs

`build_desktop_home_state` uses `list_snapshots`, graph diff, and timeline entries for `currentSnapshotId`, `workingChanges`, and `changelog`.

### 7. Verification capture driver

`scripts/verify-review-patches.sh` runs all six verification-plan steps and checks artifact existence plus expected strings (no `No apply handler`, bundle verify exit codes, `currentSnapshotId` in desktop JSON).

**Commits:** `9a7e65f` → `a2b6810` on `refactor/rust-core-u1`

## Why This Works

Registry completeness closes the plan→apply dispatch gap. Structured `target_content` preserves JSON shapes scanners already produce. Permission metadata and `.perm-` parsing match evidence encoding; rule unwrapping aligns on-disk JSON with pre-migration behavior. Ordered undo branches respect derived paths (`mcpPath`, `envPath`) that differ from `item.dest`. Confinement at parse, apply, and import provides defense in depth. Full-plan tests catch registration bugs subset tests miss.

## Prevention

| Guard | Location |
|-------|----------|
| Handler smoke + structured apply tests | `crates/hem-core/tests/restore_test.rs` |
| Full-plan pipeline test | `restore_plan_pipeline_applies_mcp_permission_and_env_with_confinement` |
| Desktop snapshot integration test | `populates_current_snapshot_id_from_store` |
| Review verification driver | `scripts/verify-review-patches.sh` |

Permission assertion to keep:

```rust
assert!(bash_rule.get("rule").is_none(), "permission apply must unwrap rule wrapper");
```

Run before merge:

```bash
HEM_SCRATCH_DIR=/tmp/hem-review-verify ./scripts/verify-review-patches.sh
```

### Code review checklist for new restore kinds

1. Add `apply_*` and register in both default registries.
2. Extend `target_content_for_plan_item` if the kind uses non-string values.
3. Add typed undo branch before generic `filePath` when rollback is partial-file.
4. Ensure callers pass `home_dir` and `project_path` in `ApplyOptions`.
5. Add unit test and extend full-plan test.
6. Re-run `scripts/verify-review-patches.sh`.

## Related Issues

- [Migration plan](../../plans/2026-06-25-001-refactor-rust-core-migration-plan.md) — U6/U7 intent; refresh recommended to document handler parity and confinement wiring
- [ARCHITECTURE.md](../../../ARCHITECTURE.md) — restore architecture section
- TypeScript reference: `packages/core/src/restore.ts`