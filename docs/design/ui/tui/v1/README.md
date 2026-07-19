# Gandalf TUI v1 — Trust-Visible Console

Status: implemented in v0.6.0
Last updated: 2026-07-19
Supersedes: `docs/design/ui/tui/v0/README.md` (navigation and terminology sections)
Related: `docs/plans/2026-07-19-001-refactor-rust-runtime-tui-migration-plan.md` (this document is the UX layer the migration plan's U8 references; the Go implementation of v1 is the behavioral reference for the Rust TUI)

## Problem Statement

The v0 TUI realized the setup console and review-gated mutation loop, but shipped with structural UX debt:

1. **Dead navigation architecture.** A full sidebar navigation model exists in code but is never rendered and never drives screen transitions. Four completed screens (Agent Detail, Profile, Compare, Save Setup) are unreachable.
2. **Asymmetric, one-way screen transitions.** No screen can return to Home. Timeline and Snapshots are reachable only through Inventory. Uppercase jump keys (`H`/`S`/`E`) exist on some screens and not others, and are not advertised in footers.
3. **Inconsistent key grammar.** Case-sensitivity carries meaning (`i` vs `I`), the same key means different things per screen (`R`, `u`, `s`), and no global keymap is discoverable.
4. **Trust is backend-only.** The product's strongest properties — provider-gated mutation, review-before-apply, restore evidence — are experienced as constraints, not visible reasons to trust the tool (see `docs/ideation/2026-07-18-open-ideation.html`). Worse, the MCP toggle writes immediately, bypassing Review Changes.
5. **Terminology overload.** baseline / snapshot / saved setup / restore point / capture all name the same underlying operation.
6. **Fragmented feedback.** Three error/notice channels exist; one of them (`actionError`) renders on only one screen, so feedback can be invisible.

v1 fixes these with one organizing idea:

> **The trust machinery is the interface.** Every row shows what Gandalf can safely do to it and why. Every mutation flows through one Review Changes surface. Every applied change leaves a visible receipt.

## Information Architecture

Five fixed destinations in a persistent left sidebar. The sidebar is always visible (collapsing to a one-row strip on narrow terminals).

| # | Destination | Contents | Replaces (v0) |
|---|---|---|---|
| 1 | **Home** | Changes-first summary: drift since last Save, top changes, quick actions | ScreenHome |
| 2 | **Console** | Tabbed setup console: Hooks / Plugins / Marketplace / Skills / MCP Servers | ScreenInventory |
| 3 | **Changes** | Environment diff review: agents → surfaces → side-by-side/unified diff | ScreenEnvironments |
| 4 | **Timeline** | Local history events, event detail, undo preview | ScreenTimeline |
| 5 | **Saves** | Saved setups list, restore review | ScreenSnapshots |

Removed from the reachable-screen inventory (dead code deleted, concepts merged):

- **Agent Detail** → agent context lives in Console rows and the Changes agent pane.
- **Profile** → out of MVP scope (per PLAN.md); stub deleted.
- **Compare** → folded into Changes (From/To selection is a Changes concern) and Saves (restore review); standalone screen deleted until a real entry point exists.
- **Save Setup screen** → Save is a modal action (`s`), not a destination.

### Layout

```text
┌ gandalf ──────────────────────────── ● claude-code ▲3 · ● codex ✓ ┐
│                 │                                                  │
│  1 Home         │  ▲ 3 setup objects changed since last save      │
│  2 Console      │                                                  │
│ ›3 Changes      │  ~ mcp    context7        modified  [reviewable]│
│  4 Timeline     │  + skill  graphify        added     [reviewable]│
│  5 Saves        │  - hook   pre-commit      removed   [restore]   │
│                 │                                                  │
│                 │                                                  │
│                 │  enter review · s save setup · ? keys           │
├─────────────────┴──────────────────────────────────────────────────┤
│ ✓ saved "add context7 mcp" · restore point 61b8                    │
└─────────────────────────────────────────────────────────────────────┘
```

- Header: app name + per-agent drift chips (unchanged from v0 frame).
- Sidebar: fixed 5 items, `›` and brand color mark the active destination. Width ~17 cols.
- Body: the active destination, full remaining width.
- Status line: single unified feedback region (see Feedback).

Narrow terminals (< 88 cols): sidebar collapses to a horizontal strip under the header:

```text
1 Home  2 Console  ›3 Changes  4 Timeline  5 Saves
```

## Key Grammar

One grammar, enforced everywhere. Case never carries meaning for navigation.

### Global (every screen)

```text
1–5      jump to destination
esc      close overlay → else back to Home (from Home: no-op)
?        keymap overlay (full reference, grouped by scope)
/        search (where a list is focused)
r        rescan
q        quit (ctrl+c always quits)
```

### Structural

```text
↑↓ / jk  move selection
tab      cycle focus within a screen (tabs, panes)
enter    primary action on selection (open / review)
```

### Contextual actions (lowercase, shown in footer per screen)

```text
s        save setup            (Home, Console, Changes)
u        preview undo          (Timeline)
n / p    next / previous hunk  (Changes diff)
v        toggle diff layout    (Changes diff)
e        expand row            (Console)
```

Retired: uppercase jumps `H`/`S`/`E`/`B`/`R`/`I`. `B` (create baselines) becomes part of Save flow / Home quick action. `I` (install) becomes `enter` on a reviewed marketplace entry inside the Review modal. `R` (rollback) becomes `enter` on a Save row.

### Keymap overlay (`?`)

A dismissible overlay listing global, structural, and current-screen keys. This is the single source of key discoverability; footers show only the top 3–5 contextual keys.

## Terminology: one word — Save

| v0 terms | v1 term |
|---|---|
| baseline, snapshot, saved setup, restore point, capture | **Save** |

- Destination 5 is **Saves**.
- "Create baseline" → "Save current setup" (auto-titled, per v0 deterministic titles).
- "Rollback to snapshot" → "Restore from save".
- Home drift line reads "changed since last save".
- Storage formats, CLI command names (`snapshot`, `restore`), and on-disk store layout are **unchanged** — this is user-facing copy only, preserving the migration plan's contract freeze.

## Trust-Visible Surfaces

### 1. Capability badges on every row

Every Console and Changes row renders its mutation capability as a first-class column, sourced from provider availability (the same `setup.PlanItemAction` gate that already exists):

```text
~ mcp    context7      modified   [reviewable]
+ skill  graphify      added      [reviewable]
- hook   pre-commit    removed    [restore-only]
  plugin github        installed  [read-only · no provider]
```

- `[reviewable]` — a provider can preview and execute a change; `enter` opens Review Changes.
- `[restore-only]` — no forward mutation, but restore evidence exists; `enter` opens restore review.
- `[read-only · <reason>]` — visible but not executable; the reason is the concrete unavailable reason the engine already produces. `enter` shows detail with the reason prominent.

This implements "Mutation-Capability-Is-Inventory" from the 2026-07-18 ideation memo: visibility ≠ executability stops being a silent rule and becomes information on the row.

### 2. One Review Changes modal

All mutations flow through a single modal component with identical structure and keys:

```text
┌ Review Changes ─────────────────────────────────────┐
│ Install plugin "github" (Claude Code)               │
│                                                      │
│ Will change                                          │
│   ~/.claude/plugins/…                                │
│                                                      │
│ Before applying                                      │
│   Current setup will be saved automatically.         │
│                                                      │
│ enter apply · esc cancel                             │
└──────────────────────────────────────────────────────┘
```

Consolidates the v0 trio: pending-action confirmation, marketplace review, rollback review. Fingerprint staleness checks (already implemented) stay; a stale plan re-renders the modal with a "changes are stale — re-review" state instead of a hidden error.

**The MCP enable/disable toggle routes through this modal.** The v0 immediate-write path is removed. This closes the one known violation of the review-before-apply principle (also pre-implements migration plan R8/AE5 so the Rust port inherits correct behavior).

### 3. Receipts in the status line and Timeline

After any apply, the status line shows the durable outcome, not a transient toast:

```text
✓ applied "install github plugin" · saved restore point 61b8
```

Timeline events for applied actions show the same identity so the status line claim is verifiable one keystroke away. (The full Action Receipt artifact is migration-plan scope R11; v1 surfaces what the Go engine already records — the auto-save + timeline event.)

## Feedback: one channel

`notice`, `undoError`, `actionError` merge into a single frame-level status model:

```text
status = { level: info | success | warn | error, text, context }
```

- Rendered in the status line on **every** screen by the frame, never by individual views.
- `error` uses the removed/danger token, `warn` amber, `success` green, `info` muted.
- Loading and boot-error get real framed states (header + message), replacing the bare string and the reused History-empty view.

## Visual Tokens

Fixes to `views/styles.go`, keeping the v0 restraint (color only where it means something):

| Token | v0 | v1 |
|---|---|---|
| `colorBrand` | 203 (== removed) | **205** — distinct from removed; used only for active sidebar item + focus |
| `colorRemoved` | 203 | 203 (unchanged) |
| `activeStyle` | hardcoded 86 (cyan) | uses `colorBrand` — one accent, not two |
| selected rows | inline literals per view | shared `selectedRow` token |
| overlay chrome | inline 235/240 | shared tokens |

Every safety distinction keeps a non-color redundancy (dots/markers/badge text), consistent with migration plan R22.

## Screen Specs

### 1 Home

v0 changes-first summary, plus:

- Footer advertises the real key set: `enter review · s save · 1-5 screens · ? keys`.
- Quick actions: `enter` on a change row → Changes destination focused on that item; `s` → Save flow (covers missing-baseline creation: if an agent has no save yet, the Save modal says so and creates the first one).
- Empty states per v0 copy ("No supported agent setup found", "Current setup matches latest save").

### 2 Console

v0 setup console (tabs, search, master-detail, skill overlay) with:

- Capability badge column (above).
- `enter` semantics unified: expand groups, review reviewable rows, show-reason for read-only rows. No separate `I`.
- Selected-row styling from shared tokens.

### 3 Changes

v0 Environments (3-pane, 4 responsive layouts, hunk nav) with:

- Renamed from "Environments" — the destination answers "what changed?".
- `s` save stays; `R` removed (restore lives in Saves; a changed agent row offers `enter → restore review` where evidence exists).
- Status/notice rendering moves to the frame (fixes invisible `actionError`).

### 4 Timeline

v0 History with:

- Hard-clip overflow replaced by scrollable regions (current-setup panel must not push the list off-screen).
- `u` preview undo unchanged; the preview clearly labels itself **preview-only** ("writes files: no") until an apply path ships. No key that implies apply.

### 5 Saves

v0 Snapshots rebuilt as a real view (currently raw string assembly in app code):

- List of saves with deterministic titles, agent markers, age.
- `enter` → restore review in the unified Review Changes modal.
- Empty state per v0 copy ("No saved setups yet… s save setup").

## Deletions

- Dead sidebar machinery is replaced by the live one: `NavSection`/`BuildNavigationModel`/`SelectNavItem` become the actual navigation model (rewired, not deleted).
- `ScreenAgentDetail`, `ScreenProfile`, `ScreenCompare`, `ScreenSaveSetup` and their renderers: deleted.
- `BuildSetupInventoryViewModel` (superseded pre-tab model): deleted.
- Dead baseline-rows render branch in setup console: deleted.
- Uppercase jump-key handlers: deleted.

## Acceptance Criteria

1. From any destination, `1`–`5` reaches every other destination; `esc` (with no overlay open) returns to Home. No dead ends.
2. `?` shows a complete, accurate keymap on every screen.
3. Every Console row shows exactly one capability badge; read-only rows show a concrete reason in detail.
4. No mutation path writes files without the Review Changes modal (including MCP toggle).
5. A status message set on any screen is visible on that screen.
6. The words "baseline" and "snapshot" do not appear in TUI-rendered copy (CLI and storage unaffected).
7. All previously reachable v0 functionality remains reachable (setup console tabs, search, skill overlay, marketplace review/install, diff layouts, undo preview, restore).
8. Existing TUI tests pass or are updated to the new grammar; new tests cover navigation symmetry and badge rendering.

## Out of Scope (deferred to migration plan)

- Mouse support, semantic action registry as a formal subsystem (R19)
- Virtualized diffs, grapheme-aware measurement (R21)
- Truecolor/256/16/NO_COLOR degradation matrix (R22)
- Action Receipt artifact + write-ahead ordering (R11)
- CLI parity surfaces (R13)
