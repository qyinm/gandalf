---
title: Preserve Go verification when removing runtime surfaces
date: 2026-06-26
category: docs/solutions/workflow-issues
module: Gandalf verification workflow
problem_type: workflow_issue
component: development_workflow
severity: medium
applies_when:
  - Removing a runtime surface that previously owned CI or review evidence
  - Replacing cargo or desktop verification with Go CLI verification
  - Running repeated Go tests as review artifacts
tags: [go-verification, ci, review-artifacts, runtime-removal, test-cache]
---

# Preserve Go verification when removing runtime surfaces

## Context

Removing the desktop/Tauri and Rust workspace surfaces also removed the old
Cargo-based verification path. The replacement had to prove the supported Go
CLI/TUI path without preserving false dependencies on `cargo`,
`target/debug/gandalf`, or desktop-specific artifacts.

The first Go rewrite of review verification was directionally right, but code
review caught two workflow traps that are easy to miss during a surface
removal:

- A clean CI checkout may not have the local output directory used by
  `go build -o bin/gandalf`.
- Repeated `go test` evidence can be satisfied by Go's test cache unless the
  command explicitly disables caching.

## Guidance

When deleting an old runtime stack, replace its checks with fresh verification
for the new supported path and audit those checks as if they run in a clean
environment.

For CI build outputs, create the output directory in the workflow before
writing the binary:

```yaml
- name: Build Go CLI
  run: |
    mkdir -p bin
    go build -o bin/gandalf ./cmd/gandalf
```

For repeated Go test evidence, use `-count=1` when the second run is meant to
prove fresh execution rather than "the package was green once":

```bash
go test -count=1 ./internal/gandalfcore/restore >"$SCRATCH/restore-test-1.log" 2>&1 &
RESTORE_1_PID=$!
go test -count=1 ./internal/gandalfcore/restore >"$SCRATCH/restore-test-2.log" 2>&1 &
RESTORE_2_PID=$!
```

Keep the review script focused on the canonical binary path. Build the Go CLI,
exercise CLI restore dry-run, verify valid and invalid bundles, and run the
full Go test suite. Do not leave behind cargo-built binary paths or
desktop-specific artifact expectations.

## Why This Matters

Runtime-surface removal can accidentally weaken verification even while the
repository gets simpler. If the replacement checks assume local directories
that CI does not have, the new canonical path fails on a clean checkout. If a
"run twice" test uses cached results, the evidence looks stronger than it is.

Explicit directory creation and cache-bypassing repeated tests keep the review
artifacts honest while allowing obsolete Rust/Tauri checks to disappear.

## When to Apply

- Removing a language/runtime stack from a repository.
- Replacing CI jobs with checks for a new canonical implementation.
- Building binaries into ignored local output directories.
- Capturing repeated Go test logs as flake or determinism evidence.

## Examples

Before:

```yaml
- name: Build Go CLI
  run: go build -o bin/gandalf ./cmd/gandalf
```

After:

```yaml
- name: Build Go CLI
  run: |
    mkdir -p bin
    go build -o bin/gandalf ./cmd/gandalf
```

Before:

```bash
go test ./internal/gandalfcore/restore >"$SCRATCH/restore-test-1.log" 2>&1
go test ./internal/gandalfcore/restore >"$SCRATCH/restore-test-2.log" 2>&1
```

After:

```bash
go test -count=1 ./internal/gandalfcore/restore >"$SCRATCH/restore-test-1.log" 2>&1
go test -count=1 ./internal/gandalfcore/restore >"$SCRATCH/restore-test-2.log" 2>&1
```

## Related

- [Go restore store trust contract gaps](../logic-errors/go-restore-store-trust-contract-gaps.md)
- [Remove desktop and Rust stack plan](../../plans/2026-06-26-004-refactor-remove-desktop-rust-plan.md)
