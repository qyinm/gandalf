---
title: "feat: Add inline source root labels"
type: "feat"
status: "completed"
date: "2026-06-08"
---

# feat: Add inline source root labels

## Summary

Show short source-root labels beside Skills, MCP Servers, and Hooks inventory rows in the TUI. This makes setup counts auditable in place without adding a separate Sources screen.

## Problem Frame

Users compare Hem's setup counts against agent UIs and need to see which local config surface produced each row. The current TUI shows item names and counts, but it often hides the provenance needed to explain why a row is present.

## Requirements

- R1. Skills, MCP Servers, and Hooks rows include a short source-root suffix in parentheses.
- R2. Source-root labels use compact roots, not full item paths, so terminal rows stay readable.
- R3. Existing status labels such as `enabled`, `disabled`, and `parse_failed` remain visible after the source suffix.
- R4. Agent Detail renders Hooks rows, not only the Hooks count.
- R5. Env Keys, Permissions, and Instructions keep their current display behavior.

## Key Technical Decisions

- **Derive source roots in the TUI model:** Use existing `DiscoveredItem.sourcePath` and optional metadata instead of changing scanner contracts.
- **Prefer model-level formatting:** Keep Ink views simple by giving them already-formatted row data or a small row model with a display label.
- **Use one inline suffix:** Project-scoped rows should avoid stacked parentheticals by folding scope and source into a single readable suffix when needed.

## Scope Boundaries

- No dedicated Sources tab, Sources screen, or left-nav item.
- No inline source labels for Env Keys, Permissions, or Instructions in this pass.
- No scanner changes unless implementation discovers missing source data for the targeted row types.

## Implementation Units

### U1. Source-root display helper

- **Goal:** Add a reusable helper that derives compact source labels from evidence paths.
- **Requirements:** R1, R2, R5
- **Dependencies:** None
- **Files:** `src/tui/components/TuiFormatters.ts`, `tests/tui.test.tsx`
- **Approach:** Create a formatter that accepts an evidence kind, name, source path, scope, and optional metadata. For skills, strip the item name from the end when it matches the skill directory so `review (~/.claude/skills/review)` becomes `review (~/.claude/skills)`. For MCP servers and hooks, keep compact config-file paths such as `.mcp.json`, `~/.cursor/mcp.json`, or `~/.cursor/hooks.json`.
- **Patterns to follow:** Existing `formatAgentLabel`, `truncateText`, and `padDisplay` helpers in `src/tui/components/TuiFormatters.ts`.
- **Test scenarios:**
  - Skill source path ending in the skill name returns the parent skill root.
  - MCP server source path that is already a config file remains that config path.
  - Hook source path that is a hooks config file remains that hooks config path.
  - Absolute managed source paths are compacted enough to avoid full machine-specific path noise.
- **Verification:** Formatter tests pin compact output for home, project, and managed examples.

### U2. Current Setup row labels

- **Goal:** Show inline source-root labels in Timeline Current Setup rows for Skills, MCP Servers, and Hooks.
- **Requirements:** R1, R2, R3, R5
- **Dependencies:** U1
- **Files:** `src/tui/components/TimelineViewModel.ts`, `tests/tui.test.tsx`
- **Approach:** Update Current Setup row construction to append the source-root suffix only for `skill`, `mcp_server`, and `hook`. Preserve the existing all-agents prefix (`Claude Code: review`) and agent-filtered shape (`review`) while adding source information after the item name.
- **Patterns to follow:** Existing `rowsForKind()` and `displayNameForItem()` behavior in `src/tui/components/TimelineViewModel.ts`.
- **Test scenarios:**
  - All-agents Current Setup renders `Agent: item (source-root)` for each targeted kind.
  - Agent-filtered Current Setup renders `item (source-root)` without duplicating the agent label.
  - Project-scoped evidence remains visibly project-scoped while also showing its source.
  - Env Key rows remain unchanged.
- **Verification:** Current Setup model tests pass with source labels and unchanged non-target rows.

### U3. Agent Detail inventory labels and Hooks section

- **Goal:** Show source-root labels in Agent Detail inventory rows and add the missing Hooks list.
- **Requirements:** R1, R2, R3, R4, R5
- **Dependencies:** U1
- **Files:** `src/tui/components/AgentDetailViewModel.ts`, `src/tui/components/AgentDetailView.tsx`, `tests/tui.test.tsx`
- **Approach:** Extend `AgentInventoryRow` with a display label or source-root field, then render Skills, MCP Servers, and Hooks using the inline source suffix. Add `hooks` rows to the view model and render a Hooks section between MCP Servers and Env Keys.
- **Patterns to follow:** Existing `skills`, `mcpServers`, `envKeys`, and `instructions` model/view flow in `src/tui/components/AgentDetailViewModel.ts` and `src/tui/components/AgentDetailView.tsx`.
- **Test scenarios:**
  - Agent Detail skill row includes a source-root suffix.
  - Agent Detail MCP row keeps `enabled` or `disabled` status after the source label.
  - Agent Detail hook row is present and includes a source-root suffix.
  - Existing Env Key and Instruction output remains unchanged.
- **Verification:** Agent Detail model tests cover the new Hooks rows and source labels.

### U4. Display safety and regression pass

- **Goal:** Keep row rendering readable after adding source labels.
- **Requirements:** R2, R3
- **Dependencies:** U2, U3
- **Files:** `src/tui/components/TimelineView.tsx`, `src/tui/components/AgentDetailView.tsx`, `tests/tui.test.tsx`
- **Approach:** Reuse existing truncation behavior where possible. If rows become too wide in fixed panels, truncate only the source suffix rather than dropping the item name or status.
- **Patterns to follow:** Existing `CurrentSetupRows` windowing in `src/tui/components/TimelineView.tsx` and `truncateText` in `src/tui/components/TuiFormatters.ts`.
- **Test scenarios:**
  - Long source labels do not remove the item name.
  - Overflow row counts and scrolling behavior remain unchanged.
  - Disabled MCP status remains visible with a long source label.
- **Verification:** TUI model tests pass and a built TUI smoke check shows readable rows in the Current Setup panel.

## Risks & Dependencies

- Source-root derivation is heuristic because scanner evidence does not have a universal `sourceRoot` field. Keep the helper conservative and prefer `sourcePath` over scanner-specific branches.
- Row labels may grow long in narrow terminals. The implementation should preserve item names and statuses before truncating source detail.

## Sources & Research

- `src/tui/components/TimelineViewModel.ts` is the Current Setup row construction choke point.
- `src/tui/components/AgentDetailViewModel.ts` already carries `path: item.sourcePath`, but does not render Hooks rows.
- `tests/tui.test.tsx` is the primary regression surface for TUI view models.
- `docs/design/ui/tui/v0/README.md` keeps Current Setup as the inline inventory surface rather than a separate source browser.
