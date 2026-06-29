---
title: Keep TUI Reviews And Setup Actions Bound To Fresh Plans
date: 2026-06-29
category: docs/solutions/architecture-patterns
module: Gandalf TUI restore and setup actions
problem_type: architecture_pattern
component: tooling
severity: high
applies_when:
  - Applying rollback or restore changes from a TUI review screen
  - Rendering environment diffs from both semantic evidence and raw source files
  - Executing setup changes such as MCP toggles from the TUI
  - Routing user-visible setup mutations through shared action-plan execution
tags: [tui, restore, rollback, environments, action-plan, mcp, diff-surface, stale-review]
---

# Keep TUI Reviews And Setup Actions Bound To Fresh Plans

## Context

Gandalf's TUI environment, restore, and setup-action flows share a trust
boundary: the user reviews a proposed state or action in the interface, then
applies it later. Code review found three related gaps around that boundary.

First, rollback apply trusted the previously built review/restore plan. If the
underlying config changed after review but before apply, the TUI could apply a
stale restore plan. Second, the Environments view-model only rendered semantic
diff surfaces, so a baseline comparison with raw-only source changes could look
clean even though checksums or evidence IDs changed. Third, MCP toggles from
the TUI called the low-level MCP mutator directly instead of flowing through
the setup action executor boundary used by the rest of the setup UI.

## Guidance

Treat preview/review output as a convenience, not as authority at apply time.
Immediately before applying a rollback review, rebuild the review from the same
snapshot reference and compare a deterministic fingerprint of the current plan
with the user's reviewed plan. Abort if the rebuilt plan differs.

```go
fresh, err := a.buildRollbackReview(snapshotRef{Name: review.SnapshotName, Agent: review.Agent})
if err != nil {
	return rollbackApplyMsg{err: fmt.Errorf("failed to refresh rollback review before apply: %w", err)}
}
if !rollbackReviewMatches(review, fresh) {
	return rollbackApplyMsg{err: fmt.Errorf("review changes are stale; reopen Review Changes before applying")}
}
```

The fingerprint should include supported items, unsupported items, and evidence
fields that affect restore semantics. Canonicalize raw JSON before
fingerprinting so equivalent JSON formatting does not create false stale-review
failures.

Account for both semantic and raw diff channels when building TUI diff
surfaces. A semantic diff answers "what named object changed"; a raw source
diff answers "what source artifact changed." Either channel is enough to make a
baseline comparison non-clean.

```go
totalSurfaces := len(focus.Diff.SemanticChanges) + len(focus.Diff.RawSourceChanges)
surfaceIndex := clampIndex(input.SelectedSurfaceIndex, totalSurfaces)
model.Surfaces = buildEnvironmentSurfaces(focus.Diff, surfaceIndex, input.CurrentHunkIndex)
```

For raw-only changes, render a source-level surface with a stable marker,
source path, change count, and rows for the raw fields that changed, such as
evidence ID, checksum, and status. Do not rely on an aggregate raw-change count
alone; the view-model needs actual surfaces so navigation and detail panes show
the change.

Route TUI setup actions through the setup action executor boundary. MCP toggles
are file mutations rather than shell commands, but they still belong inside the
same action-plan execution path so the TUI has one orchestration point and
tests can replace the executor.

```go
func (a *App) defaultSetupActionExecutor(ctx context.Context, plan setup.ActionPlan) error {
	_, err := setup.ExecuteActionPlan(ctx, plan, nil, setup.WithHomeDir(a.runtime.HomeDir))
	return err
}
```

The action plan can carry an execution spec for built-in file mutations, and
the executor can require the roots or context needed for that mutation.

```go
if plan.Action == ActionToggle {
	if plan.MCPToggle == nil {
		return ActionResult{}, errors.New("toggle action requires an MCP toggle plan")
	}
	if strings.TrimSpace(options.HomeDir) == "" {
		return ActionResult{}, errors.New("toggle action requires home directory")
	}
	if _, err := ExecuteMCPToggle(plan, options.HomeDir, plan.MCPToggle.ServerName, plan.MCPToggle.ConfigPath); err != nil {
		return ActionResult{}, err
	}
	return ActionResult{ExecutedCommand: false}, nil
}
```

## Why This Matters

A review screen is not a capability boundary unless it is revalidated at the
moment of mutation. Without the rollback refresh and fingerprint check, a user
could review one restore plan, have the environment change underneath it, and
then apply a different effective operation.

Diff surfaces are part of the user's trust model. If the UI only displays
semantic changes, raw-only config drift can make a changed environment look
like it matches the baseline. That erodes confidence in restore and scan
workflows because the underlying evidence graph knows about the change but the
visible TUI does not.

The TUI should orchestrate setup actions through the same setup-layer contract
that plans and executes actions elsewhere. Direct calls to low-level mutators
split behavior across paths, make it easier to miss shared requirements like
home-directory handling, and prevent tests from substituting a single executor
boundary.

## When to Apply

- A TUI flow separates review/preview from mutation, especially for restore,
  rollback, install, toggle, or delete actions over files or environment state.
- A diff model has multiple channels, such as semantic object changes and raw
  source changes.
- An action can be represented as a setup action plan, even if execution is a
  built-in file mutation rather than an external command.
- Tests need to substitute a single executor boundary for all TUI setup
  mutations.

## Examples

Stale rollback reviews should fail closed. Build a review, change the config
again, apply the original review, and assert that the stale apply errors while
leaving the newer config untouched:

```go
msg := app.applyRollbackReview(review)
if msg.err == nil || !strings.Contains(msg.err.Error(), "stale") {
	t.Fatalf("expected stale review error, got %v", msg.err)
}
```

Raw source changes should produce visible surfaces instead of a clean empty
state:

```go
if model.ChangesEmpty != "" {
	t.Fatalf("raw-only diff should not be reported clean: %q", model.ChangesEmpty)
}
if !hasEnvironmentPair(model.Diff.Rows, "checksum: sha256:old", "checksum: sha256:new") {
	t.Fatalf("expected raw checksum diff: %#v", model.Diff.Rows)
}
```

Built-in setup mutations should still go through the generic action executor
and require the context they need:

```go
plan := PlanItemAction(item, ActionToggle)
if plan.MCPToggle == nil {
	t.Fatalf("toggle plan missing execution spec: %#v", plan)
}

if _, err := ExecuteActionPlan(context.Background(), plan, nil, WithHomeDir(home)); err != nil {
	t.Fatal(err)
}
```

## Related

- [Separate global setup inventory from executable setup actions](global-setup-inventory-action-boundary.md)
- [Keep Setup Console interaction state in a component boundary](setup-console-component-state-boundary.md)
- [Go Restore/Store Trust-Contract Gaps Found in Code Review](../logic-errors/go-restore-store-trust-contract-gaps.md)
- [Review Changes](../../../CONCEPTS.md#review-changes)
- [Environment Diff Surface](../../../CONCEPTS.md#environment-diff-surface)
- [Setup Action Provider](../../../CONCEPTS.md#setup-action-provider)
