---
title: Keep Setup Console interaction state in a component boundary
date: 2026-06-28
category: docs/solutions/architecture-patterns
module: Gandalf TUI
problem_type: architecture_pattern
component: tooling
severity: medium
applies_when:
  - Adding stateful keyboard behavior to the Setup Console
  - Building hierarchical Marketplace source rows
  - Preserving per-tab search, cursor, and expansion state
  - Refactoring Bubble Tea views without collapsing setup domain boundaries
tags: [tui, bubble-tea, setup-console, marketplace, component-state]
---

# Keep Setup Console interaction state in a component boundary

## Context

The Setup Console originally rendered tab rows mostly as strings, with a single cursor and search value shared across Hooks, Plugins, Marketplace, Skills, and MCP Servers.
That was enough for a flat inventory table, but it broke down once Marketplace needed source rows that expand and collapse while normal setup tabs kept standard inventory behavior.

The tempting shortcut is to push expansion flags, selected row interpretation, and action behavior into the renderer.
That makes the UI look richer quickly, but it blurs three contracts that Gandalf relies on: setup domain models, TUI interaction state, and provider-gated actions.

## Guidance

Use a Setup Console component boundary for interaction state, and keep domain interpretation in view-model construction.

The root Bubble Tea app should still own scan, store, timeline, and action execution orchestration.
Setup-specific interaction state belongs in a narrower component-like state object:

```go
type setupConsoleState struct {
    tabs            map[SetupConsoleTab]*setupConsoleTabState
    expandedSources map[string]bool
    rowsViewport    viewport.Model
}

type setupConsoleTabState struct {
    cursor      int
    search      string
    searchInput textinput.Model
}
```

The view model should then receive this state as input and produce explicit row semantics:

```go
type SetupConsoleRowModel struct {
    ID         string
    RowKind    SetupConsoleRowKind
    ParentID   string
    Depth      int
    Expanded   bool
    Toggleable bool
}
```

That lets key handling ask clear questions:

- Is the selected row a Marketplace source? Toggle expansion.
- Is it a Marketplace entry? Report provider-gated action availability.
- Is it a normal inventory row? Use the setup action planner.

For hierarchical Marketplace search, make the view model responsible for preserving source context.
When a query matches a child entry, emit both the parent source row and the matching child row.
When a query matches the source itself, emit the source and its entries.
Do not make the renderer discover parent-child relationships from string prefixes.

Use Bubbles components for stateful UI primitives where they carry behavior: `textinput` for search focus and value, `viewport` for row offset, and `help` for contextual key hints.
The renderer should receive presentation metadata and offsets, not recreate all interaction state on every render.

## Why This Matters

Terminal UIs become difficult to evolve when row strings are the only abstraction.
Marketplace needs hierarchy, inventory tabs need flat action rows, and future providers need selected-row identity that is stronger than "whatever text is highlighted."

A component boundary keeps those concerns separate:

- Domain packages define setup objects, marketplace sources, and provider availability.
- TUI view models decide visible rows, selected details, hierarchy, and labels.
- Component state decides cursor, search, expanded source IDs, and viewport offsets.
- Renderers draw rows and help text from explicit metadata.

This prevents future changes from smuggling product behavior into string formatting.
It also makes unit tests cheaper because row hierarchy, search, and key behavior can be tested without PTY snapshots.

## When to Apply

- A TUI tab needs per-tab cursor or search preservation.
- A list needs grouped or hierarchical rows.
- Pressing Enter has different semantics by row type.
- Renderer code starts parsing prefixes or indentation to infer behavior.
- A Bubble Tea app has one global cursor/search value that no longer matches the visible surface.

## Examples

Before, Marketplace entries were flattened by indentation in the row name:

```go
Name: "  " + entry.Name
```

After, hierarchy is explicit:

```go
SetupConsoleRowModel{
    RowKind:  SetupConsoleRowMarketplaceEntry,
    ParentID: entry.SourceID,
    Depth:    1,
    Name:     entry.Name,
}
```

Before, search key handling clamped the cursor against inventory rows, which is wrong on Marketplace because Marketplace rows are not `setup.InventoryItem` values:

```go
a.inventoryCursor = clampIndex(a.inventoryCursor, len(a.currentInventory()))
```

After, clamp against the visible Setup Console rows:

```go
a.setInventoryCursor(clampIndex(a.activeSetupTabState().cursor, len(a.currentSetupConsoleViewModel().Rows)))
```

## Related

- [Separate global setup inventory from executable setup actions](global-setup-inventory-action-boundary.md)
- [Unified Inventory](../../../CONCEPTS.md#unified-inventory)
- [Agent Marketplace Source](../../../CONCEPTS.md#agent-marketplace-source)
- [Setup Console](../../../CONCEPTS.md#setup-console)
