---
title: Use compact disclosure rows for dense Setup Console tabs
date: 2026-06-29
category: docs/solutions/design-patterns
module: Gandalf TUI
problem_type: design_pattern
component: tooling
severity: medium
applies_when:
  - Building dense Setup Console tabs with repeated agent and object metadata
  - Adding expandable rows for skills, hooks, marketplaces, or MCP servers
  - Replacing table-like terminal layouts with progressive disclosure
  - Keeping selected rows visible while expanded details are shown
tags: [tui, setup-console, progressive-disclosure, compact-rows, mcp]
---

# Use compact disclosure rows for dense Setup Console tabs

## Context

Setup Console tabs can contain hundreds of skills, hooks, marketplace entries, and MCP tools. A table-like layout that repeats agent markers, object kind, scope, source path, and action availability on every row becomes hard to scan, especially in a terminal where horizontal space is scarce.

The better pattern is a compact list where the primary row answers "what is this?" and a lightweight right column answers "where does this belong?" More detailed metadata should appear only when it helps the current interaction.

## Guidance

Render default rows as compact disclosure rows:

```text
› benchmark                                                                 (local)
› posthog [ready]                                                          Cursor
```

Avoid repeating metadata that is already implied by the active tab. In the Skills tab, the user already knows these rows are skills, so `CC skill benchmark` adds noise. In the MCP Servers tab, the useful right-side label is the installed agent, not the source config path.

Use Enter or Space to reveal contextual children or detail:

- Skills, hooks, and plugins reveal short metadata only while their row is selected.
- Marketplace sources reveal entries, not a repeated source metadata block.
- MCP servers reveal tool rows; tool rows reveal descriptions when selected.

Keep detail rendering tied to selection. Expanded state can remain in component state, but a stale expanded row above the cursor should not keep rendering a multi-line detail block that pushes the selected compact row out of view. For MCP servers, treat the server expansion as "show tools" and leave descriptive detail to selected tool rows.

Search should preserve the hierarchy users need to understand the result. If a query matches an MCP tool name or description, keep the parent server row and show the matching tool row under it. Do not require the parent server itself to match the query.

## Why This Matters

Dense terminal UIs fail when every row tries to be both a summary and a detail panel. The result is wide, repetitive, and visually low contrast; users have to parse prefixes like `CC skill` or source paths that are not the thing they are looking for.

Compact disclosure keeps the first scan fast and makes expansion predictable:

- The active tab supplies object type context.
- The row name supplies identity.
- The right column supplies origin or status.
- Expansion supplies child rows or selected-row details.

It also keeps viewport behavior tractable. When multi-line details are tied to the selected row instead of stale expanded rows elsewhere in the list, cursor movement and viewport offsets continue to describe the same visible surface.

## When to Apply

- A tab's row prefix duplicates the selected tab name or object kind.
- A source path is less useful than the agent or install origin.
- Rows need child items, such as Marketplace entries or MCP tools.
- Search can match child data that should remain grouped under its parent.
- Expanded metadata above the cursor can push the selected row off-screen.

## Examples

Before, the Skills tab repeated agent and kind on every row:

```text
› CC skill benchmark                                                     (local)
› CC skill design-review                                                 (local)
```

After, the active tab carries the type context:

```text
› benchmark                                                             (local)
› design-review                                                         (local)
```

Before, MCP search only matched server metadata, so searching for `dashboard-get` could hide the server that exposed that tool.

After, tool names and descriptions participate in search, and matching tools stay grouped under their server:

```text
› posthog [ready]                                                        Cursor
›   dashboard-get
```

## Related

- [Keep Setup Console interaction state in a component boundary](../architecture-patterns/setup-console-component-state-boundary.md)
- [Keep global setup inventory pure and actions provider-gated](../architecture-patterns/global-setup-inventory-action-boundary.md)
