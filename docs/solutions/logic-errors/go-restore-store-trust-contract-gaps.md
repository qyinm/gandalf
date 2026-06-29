---
title: Go Restore/Store Trust-Contract Gaps Found in Code Review
date: 2026-06-26
last_updated: 2026-06-29
category: logic-errors
module: gandalf-go-restore-store
problem_type: logic_error
component: tooling
symptoms:
  - "Restore apply could validate `item.Dest` but still write MCP state to a different metadata-derived path."
  - "Snapshot replacement could leave updated metadata next to missing or partial content blobs after a mid-write failure."
  - "Plan/apply tests kept calling `ApplyRestoreItems` without confinement roots, so they no longer modeled real CLI apply behavior."
  - "Acceptance restore failed when the declared home root contained redundant separators but the destination path was clean."
root_cause: logic_error
resolution_type: code_fix
severity: high
tags:
  - go-rewrite
  - restore
  - snapshot-store
  - path-confinement
  - atomic-replacement
related_components:
  - development_workflow
  - testing_framework
---

# Go Restore/Store Trust-Contract Gaps Found in Code Review

## Problem

The Go rewrite had already ported the main Gate 2 restore/store path, but follow-up code review still found trust-contract holes in the write path. The remaining bugs were not in plan generation; they lived in the last mile where apply handlers and snapshot persistence turned validated intent into filesystem mutations.

Two failures mattered most: `mcp_server` apply could ignore the validated destination and follow a metadata override instead, and snapshot rewrites could tear the on-disk store if content replacement failed mid-write.

## Symptoms

- Restore apply validated `item.Dest`, but `ApplyMCPServer` still accepted an absolute `metadata["mcpPath"]` and wrote there instead.
- Rollback for the same MCP item could follow the override path rather than the path that had actually been applied.
- `WriteSnapshot` wrote fresh manifest/evidence files first, then deleted and rewrote `content/`, so a later failure could leave the snapshot half-updated.
- After `ApplyRestoreItems` was changed to fail closed without confinement roots, several plan/apply tests started needing explicit `HomeDir` and `ProjectPath` to reflect real CLI execution.
- A Gate 2 acceptance run could reject an otherwise valid restore target when `HomeDir` was spelled with redundant separators and the destination path had already been cleaned.

## What Didn't Work

- Validating only `item.Dest` was not enough. The confinement check happened before dispatch, but `ApplyMCPServer` derived the real write path from metadata afterward, which reopened the boundary.
- In-place snapshot replacement (`remove old content/`, then write new blobs, then write `content-index.json`) assumed every write would succeed. That is fine only when partial failure is impossible, which is exactly the assumption a persistence layer should not make.
- The existing plan tests exercised restore behavior through `ApplyRestoreItems`, but they omitted roots in `ApplyOptions`. Once apply correctly failed closed, those tests no longer described the real CLI path and had to be fixed.
- Comparing a cleaned destination path against uncleaned confinement roots was too literal. The filesystem target was inside the allowed root, but the string-prefix check could not prove it when the root was spelled differently.
- Session-history extraction was attempted for this run, but no relevant prior restore/store sessions were found in accessible local history. The referenced Grok session was not present in local storage, so it could not be used as evidence.

## Solution

### 1. Make apply fail closed when roots are missing

`ApplyRestoreItems` now requires confinement roots instead of silently proceeding when callers omit them:

```go
roots := pathconfinement.RootsFromPaths(options.HomeDir, options.ProjectPath)
if roots == nil {
	return failure("restore apply requires home and project roots for path confinement")
}
```

That turns missing caller context into an explicit failure instead of a silent downgrade of the trust boundary.

### 2. Bind MCP writes and rollback to the validated destination

`ApplyMCPServer` no longer trusts an absolute `metadata["mcpPath"]`. It derives the config path from `item.Dest`, which was already validated by confinement logic:

```go
func mcpConfigPathForItem(item *types.RestoreItem) string {
	dest := item.Dest
	if filepath.Base(dest) == ".mcp.json" {
		return dest
	}
	return filepath.Join(filepath.Dir(dest), ".mcp.json")
}
```

Rollback follows the same applied path by preferring the stored `filePath` from rollback state:

```go
mcpPath, _ := state["filePath"].(string)
if mcpPath == "" {
	mcpPath, _ = state["mcpPath"].(string)
}
```

The same patch also added a symlink refusal before writes:

```go
if info, err := os.Lstat(filePath); err == nil && info.Mode()&os.ModeSymlink != 0 {
	return fmt.Errorf("refusing to write through symlink destination: %s", filePath)
}
```

### 3. Stage snapshot replacements in a temporary directory

`WriteSnapshot` now materializes the replacement snapshot under a temp directory and swaps it into place only after all metadata, content blobs, and `content-index.json` are ready:

```go
tempDir, err := os.MkdirTemp(parentDir, "."+name+".tmp-")
...
if err := writeJSONAtomic(filepath.Join(tempDir, "manifest.json"), snapshot.Manifest); err != nil { ... }
...
return replaceSnapshotDir(tempDir, dir)
```

`replaceSnapshotDir` uses a backup rename so a failed swap can restore the previous snapshot directory:

```go
if err := os.Rename(targetDir, backupDir); err != nil { ... }
if err := os.Rename(tempDir, targetDir); err != nil {
	_ = os.Rename(backupDir, targetDir)
	return err
}
```

### 4. Bring restore plan tests in line with the production contract

The plan/apply tests that call `ApplyRestoreItems` directly now pass `HomeDir` and `ProjectPath` explicitly, matching the CLI path and preserving the fail-closed behavior under test.

### 5. Normalize confinement roots before comparisons

`RootsFromPaths` now cleans the declared roots before they are used by `ValidateConstrainedWritePath`:

```go
case homeDir != nil && projectPath != nil:
	return &Roots{HomeDir: filepath.Clean(*homeDir), ProjectPath: filepath.Clean(*projectPath)}
```

This keeps the root spelling and destination spelling in the same canonical form before the strict prefix check runs.

## Why This Works

The trust contract only holds if the **actual write target** is the same one that was validated. By deriving MCP writes from `item.Dest` and making rollback prefer the path that was actually applied, the code removes the metadata escape hatch that existed after validation.

The snapshot-store fix works for the same reason at a different layer: readers should either see the old snapshot or the fully-written new snapshot, never a mixture. Staging under a temp directory and swapping at the end restores that all-or-nothing property.

Root normalization preserves the same trust boundary without making it looser. It does not authorize new paths; it makes the allowed roots comparable to the already-cleaned destination path so equivalent filesystem spellings do not produce false rejections.

The test updates matter because they keep the regression net aligned with production semantics. Once apply requires roots, tests that omit roots are no longer realistic; they are accidental bypasses.

## Prevention

- Any apply handler that derives a secondary path from metadata must either:
  - derive it from the already-validated `item.Dest`, or
  - run the same confinement validation again on the derived path.
- Persistence code for the snapshot store should stage complete replacements and swap them into place, rather than mutating live snapshot directories in place.
- Direct `ApplyRestoreItems` tests must pass `HomeDir` and `ProjectPath` so they exercise the same confinement contract as `internal/cli/restore.go`.
- Normalize both sides of any path-confinement comparison before applying strict prefix checks. Acceptance scripts should canonicalize temporary roots too, so path-spelling drift fails in the smallest possible place.
- Keep regression tests that prove:
  - metadata path overrides do not redirect writes,
  - rollback restores the applied path rather than an override path,
  - redundant separators in declared roots do not reject valid in-root destinations,
  - failed snapshot replacement leaves the previous version readable.
- Treat `ARCHITECTURE.md`'s Gate 2 trust contract as a concrete checklist for write-path reviews: fail-closed roots, symlink refusal, validated destinations, and atomic persistence.

## Related Issues

- [Rust Core Restore Handler Gaps Found in Migration Code Review](./rust-core-restore-handler-review-gaps.md) — same problem space in the Rust rewrite; overlap is moderate rather than exact because the Go fixes target a later trust-boundary regression.
- [Go full rewrite plan](../../plans/2026-06-26-001-refactor-go-full-rewrite-plan.md) — R3/R4 and U5 explicitly require path confinement, symlink refusal, and Gate 2 restore safety.
- [ARCHITECTURE.md](../../../ARCHITECTURE.md) — Gate 2 and restore/store trust expectations.
