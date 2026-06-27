---
title: Separate global setup inventory from executable setup actions
date: 2026-06-27
category: docs/solutions/architecture-patterns
module: Gandalf global setup manager
problem_type: architecture_pattern
component: tooling
severity: high
applies_when:
  - Building a TUI-first setup manager over global agent configuration
  - Showing editable setup objects before concrete mutation providers exist
  - Changing scan defaults from project-aware discovery to global-only discovery
tags: [global-setup, tui, inventory, actions, scan-scope, agent-setup]
---

# Separate global setup inventory from executable setup actions

## Context

Gandalf moved from a history-first setup safety tool to a TUI-first global
setup manager. The first screen now presents user-global skills, hooks, MCP
servers, and plugins across supported agents. Project-local setup is outside
the active product scope.

That product shift creates two separate boundaries that are easy to collapse:

- The inventory boundary decides what objects are visible in the global setup
  manager.
- The action boundary decides which visible objects can actually be edited,
  removed, or added by Gandalf.

Treating a visible row as automatically executable caused a review finding:
edit/remove confirmations could report success even when no concrete file or
command mutation had run.

## Guidance

Keep global inventory, scan scope, and executable action providers as separate
layers.

The scan layer should enforce product scope before expensive or invasive
reads. If default product behavior is global-only, custom scanners must use the
same scope predicate before walking project-local targets. Filtering project
evidence after reading it avoids output leakage, but it still leaves startup
cost and scope confusion.

The inventory layer should answer only: "Which setup objects belong in the
global manager?" It can show unavailable actions with reasons, but it should
not imply executability just because an item is user-scoped.

The action layer should own executability. A setup action is available only
when an action provider can produce a concrete effect such as:

- a file mutation with target path, object key, and expected change;
- an agent-native command plan with runner requirements;
- a dry-run preview that is explicitly labeled non-mutating.

Until a provider exists, the row can remain visible while actions are marked
unavailable:

```go
return []setup.ActionAvailability{
    {Action: setup.ActionEdit, Available: false, Reason: "edit action provider is not implemented yet"},
    {Action: setup.ActionRemove, Available: false, Reason: "remove action provider is not implemented yet"},
}
```

The TUI should report success only after a real executor completes. If action
execution succeeds but the follow-up rescan fails, clear the pending action so
the user cannot press Enter and re-run the same mutation. Show the rescan
failure as stale-data state, not as an executable confirmation.

## Why This Matters

A global setup manager is trust-sensitive because users expect it to change
their actual agent environment. A row that says "edit" or "remove" but only
executes a descriptive no-op teaches the wrong safety model. It also blocks
future agent-native automation because agents need structured, auditable
effects, not human-readable operation strings.

Separating inventory from providers allows the product to ship useful discovery
and navigation first while preserving truthful action semantics. It also gives
future CLI or agent surfaces a reusable contract: list objects, list available
actions, plan an action, then execute only concrete effects.

## When to Apply

- Introducing a management UI before every mutation path is implemented.
- Adding support for a new agent whose skills, hooks, MCP servers, or plugins
  have different storage formats.
- Refactoring scans so nil or default scope means global-only product behavior.
- Adding CLI or agent automation on top of TUI-visible setup objects.

## Examples

Before:

```go
if plan.Command == nil {
    return setup.ActionResult{OperationOnly: true}, nil
}
```

This lets a confirmation succeed without changing anything.

After:

```go
if plan.Command == nil {
    return setup.ActionResult{}, errors.New("setup action requires an executable command plan")
}
```

Pair that with unavailable inventory actions until a provider can emit a
concrete command or file mutation.

For scan scope, do not let custom scanners interpret nil scope as "read
everything" when the product default is global-only:

```go
if !scan.ScopeEnabled(target.Scope, context.Scope) {
    continue
}
```

## Related

- [Preserve Go verification when removing runtime surfaces](../workflow-issues/go-verification-after-runtime-surface-removal.md)
- [Global Agent Setup Manager](../../../CONCEPTS.md#global-agent-setup-manager)
- [Unified Inventory](../../../CONCEPTS.md#unified-inventory)
