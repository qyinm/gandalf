# Gandalf TUI v0

## Product Frame

Gandalf v0 is a global setup manager for AI coding agent setups.

The TUI should help a power user answer four questions quickly:

1. What user-global skills, hooks, MCP servers, and plugins are installed right now?
2. Which agent does each setup object belong to?
3. Can I add, remove, or edit an item through the agent's native setup path?
4. Can I still use snapshots and history as a rollback safety layer?

The app is not primarily a marketplace, security dashboard, or brand page. It is a user-global setup manager with history and restore as secondary safety workflows.

## Core Model

| Concept | Git Analogy | User-Facing Term |
|---|---|---|
| Current user-global agent setup | working tree | Current setup |
| Named environment line | branch | Profile |
| Saved state in a profile | commit | Snapshot / saved setup |
| Local setup history event | log entry | Timeline event / change |
| Change list between states | diff | Compare |
| Revert to saved state | checkout/reset | Restore |
| Portable bundle | archive | `.gandalf` file |

### Profile

A profile is a named line of agent environment history.

Examples:

- `default`
- `daily`
- `frontend`
- `clean-baseline`
- `work-mac`
- `claude-only`

MVP starts with one default profile. Users can add more profiles later.

### Snapshot

A snapshot is one saved point inside a profile. It captures user-global setup, not only one agent.

The snapshot contains:

- Claude Code state
- Codex state
- Cursor state
- OpenCode/Pi Agent state when supported

Agent screens show filtered history, but the saved unit is the global setup. Project-local setup files are outside the current product scope.

### Deterministic Snapshot Titles

Users should not write save messages in v0. Snapshot titles are generated from structured diffs.

Priority order:

1. MCP added or removed
2. Skill added or removed
3. Hook or permission changed
4. Plugin changed
5. Other global settings changed

Examples:

- `add github mcp to Claude Code`
- `remove playwright mcp from Cursor`
- `install react-review skill`
- `update Claude Code permissions`
- `change 2 MCPs and 1 skill`
- `capture baseline`
- `before restore to 61b8`

## Layout

The TUI uses a persistent left nav and a main workspace. The Global Setup inventory is the first screen. It shows one cross-agent list of user-global skills, hooks, MCP servers, and plugins with a compact agent marker on each row. Timeline and snapshots remain available from History.

```text
┌──────────────────────┬────────────────────────────────────────────────────────┐
│ Inventory            │ Global setup inventory                                 │
│ ▸ Global setup       │ skills 570  mcp 3  hooks 13  plugins 4                │
│                      │                                                        │
│ Profiles             │ > CC  skill   autoplan              ~/.claude/skills   │
│   default            │   CX  skill   review                ~/.codex/skills    │
│                      │   CU  mcp     github                ~/.cursor/mcp.json │
│ Agents               │   PI  plugin  browser               ~/.pi/agent/...    │
│   Claude Code   71   │                                                        │
│   Codex         377  │ Confirm setup action                                   │
│   OpenCode      126  │ edit skill: review                                     │
│   Pi Agent      27   │ target: ~/.codex/skills/review                        │
│                      │                                                        │
│ History              │                                                        │
│   All changes        │                                                        │
│   Snapshots          │                                                        │
├──────────────────────┴────────────────────────────────────────────────────────┤
│ ↑↓ move  Enter action  r=rescan  u=preview undo in history  q=quit           │
└───────────────────────────────────────────────────────────────────────────────┘
```

Do not show a large `Gandalf` brand header. The selected inventory row, agent marker, and global setup object are the primary context.

The nav frame and inventory workspace should share the same overall height. The first screen should not require choosing an agent before inspecting setup.

## Left Navigation

```text
Inventory
  Global setup

Profiles
  default

Agents
  Claude Code
  Codex
  OpenCode
  Pi Agent

History
  All changes
  Snapshots
```

Profiles appear first because they define the active environment line. MVP shows only the `default` profile.

Agents are shown only when detected in the current scan. Do not list supported-but-absent agents with zero counts. Project evidence is not an agent nav item and project-scoped setup is outside the current TUI product scope.

History can be viewed globally. `All changes` opens the Timeline screen with `Filter: All agents`; selecting an agent while Timeline is open keeps the Timeline screen and changes the filter.

## Main Screens

### Global Setup Inventory

Shown first. It lists user-global skills, hooks, MCP servers, and plugins across agents.

```text
Global setup inventory
skills 570  mcp 3  hooks 13  plugins 4

> CC  skill   autoplan              ~/.claude/skills           edit remove
  CX  skill   review                ~/.codex/skills            edit remove
  CU  mcp     github                ~/.cursor/mcp.json         edit remove
  PI  plugin  browser               ~/.pi/agent/extensions     edit remove
```

Each row shows agent marker, object kind, name, source, and available actions. Selecting a row opens a short confirmation that shows the target, agent, operation or command, and global config target.

### Current Setup Panel

Shown on the secondary Timeline screen. It summarizes the currently scanned global setup for `All agents` or the selected agent filter.

```text
Current Setup
  Scope: Codex
  Agents 1  Skills 368  MCP Servers 3  Hooks 5  Permissions 0  Env Keys 0

  Skills 368  MCP Servers 3  Hooks 5
  Spreadsheets
  ads-explorer
  agent-browser
  agent-browser-verify
  showing 1-4 of 351
```

The section rows are skills, MCP servers, hooks, and plugins. Project-local setup is not shown in the active product path.

Do not render `Instructions none`. Missing instructions should simply be absent from the Timeline Current Setup panel. Agent Detail can still show instruction counts and paths when they exist.

### Agent Detail

Shown when an agent is selected.

```text
Claude Code
Profile: default

Current Setup
  Skills        8
  MCP Servers   3
  Hooks         2
  Permissions   4
  Instructions  ~/.claude/CLAUDE.md

Skills
  react-review
  debugging
  product-review

MCP Servers
  github        enabled
  linear        enabled
  playwright    disabled

Instructions
  ~/.claude/CLAUDE.md

History
* 9f2a  Today 14:22  add github mcp to Claude Code
* 61b8  Today 13:50  install react-review skill
```

Agent detail is inventory-first. History is attached below the current setup. Add/remove/edit actions are available from the global inventory first screen.

### All Changes / Timeline

Shown from History. The initial view is global (`All agents`), and selecting an agent filters both Current Setup and Timeline instead of leaving the Timeline screen.

The Timeline screen includes Current Setup above the timeline list. Timeline is below setup, not a separate top-level tab above it.

```text
Current Setup
  Scope: All agents
  Agents 4  Skills 570  MCP Servers 3  Hooks 13  Permissions 1  Env Keys 0

  Skills 570  MCP Servers 3  Hooks 13
  Claude Code: autoplan
  Claude Code: benchmark
  Claude Code: benchmark-models
  Claude Code: better-auth-best-practices
  showing 1-4 of 553

Timeline
Filter: All agents

event     observed                 kind           readiness     agent        title
9f2a      Today 14:22              setup_changed  partial       Claude Code  add github mcp
61b8      Today 13:50              setup_changed  observe-only  Claude Code  install react-review skill
b102      Yesterday                baseline       observe-only  all          capture baseline

Selected: 9f2a

Changed
  + Claude Code MCP: github          writable preview
  + Claude Code Skill: react-review  observe-only

Actions
  u preview undo
```

This should feel like `git log`, but without using `commit`, `branch`, `checkout`, or `reset` as user-facing labels.

Timeline preview is non-mutating in P0: it calls the same dry-run planner as `gandalf timeline undo <id> --dry-run`, renders `writes files: no`, and separates MCP preview items from observe-only surfaces.

### Profiles

Shown when a profile is selected from the Profiles section.

```text
Profiles

● default
  12 snapshots
  Claude Code, Codex, Cursor
  changed Today 14:22

  frontend
  5 snapshots
  Claude Code, Cursor
  changed Yesterday

  clean-baseline
  1 snapshot
  Claude Code
  changed May 28

Actions
  Enter switch   s save   c compare
```

`.gandalf` export is not a primary global action. It lives inside Save Setup as a destination option.

## Actions

### Save Setup

Save creates a local snapshot. Exporting `.gandalf` is a save destination, not a separate top-level mental model.

```text
Save current setup?

Detected changes
  + github mcp in Claude Code
  ~ ~/.claude/settings.json

This will be saved as
  add github mcp to Claude Code

Destinations
  ✓ Local history
  □ Export as .gandalf
  □ Cloud profile        Pro

[Save] [Cancel]
```

MVP only needs:

- Local history
- Export as `.gandalf`

Cloud profile is future Pro behavior.

### Compare

Compare must always show the two points being compared.

Default:

- From: selected snapshot, or latest saved snapshot
- To: current setup

```text
Compare

From  61b8  Yesterday 13:50  clean baseline
To    Current                 unsaved changes
Scope Full setup

Summary
  + Claude Code MCP: github
  + Claude Code Skill: react-review
  ~ Claude Code permissions

Side-by-side
┌───────────────────────────────┬────────────────────────────────┐
│ 61b8 · clean baseline          │ Current · unsaved changes       │
├───────────────────────────────┼────────────────────────────────┤
│ Claude Code                    │ Claude Code                     │
│ Skills                         │ Skills                          │
│   debugging                    │   debugging                     │
│                               │ + react-review                  │
│ MCP Servers                    │ MCP Servers                     │
│   linear                       │   linear                        │
│                               │ + github                        │
│ Permissions                    │ Permissions                     │
│   allow read                   │   allow read                    │
│                               │ + allow bash: bun run test          │
├───────────────────────────────┼────────────────────────────────┤
│ Files                          │ Files                           │
│ ~/.claude/settings.json        │ ~/.claude/settings.json         │
│                               │ ~/.claude/skills/react-review/  │
└───────────────────────────────┴────────────────────────────────┘

r restore from left   s save current   f change from   t change to   esc back
```

Structured side-by-side comparison is primary. Raw file diff can be a secondary view later.

Supported scopes:

- Full setup
- This agent

MVP can skip file-level raw diff if structured diff is clear.

### Restore

Restore returns to a saved snapshot. Before restore, current setup is saved automatically.

```text
Restore to 61b8?

Target
  Yesterday 13:50  clean baseline

This will restore
  Claude Code settings
  Claude Code skills
  MCP configuration
  Project instructions

Current setup will be saved first as
  before restore to 61b8

[Restore] [Cancel]
```

Original TUI v0 restore targets full setup restore. Local history preview deliberately narrows this:

- timeline restore preview is MCP-only dry-run undo
- skills, hooks, and permissions appear as observe-only surfaces
- full setup restore remains post-P0 until restore handlers are audited and covered by tests

Agent-only restore is still a future advanced action.

### Add / Remove MCP or Skill

Add/remove is available from agent detail section context, not from the first-screen global action row.

Example MCP add preview:

```text
Add GitHub MCP to Claude Code?

Will change
  ~/.claude/settings.json

Will add
  mcpServers.github

Needs
  GITHUB_TOKEN

Before applying
  Current setup will be saved automatically.

[Apply] [Cancel]
```

Example remove preview:

```text
Remove GitHub MCP from Claude Code?

Will change
  ~/.claude/settings.json

Will remove
  mcpServers.github

Before applying
  Current setup will be saved automatically.

[Remove] [Cancel]
```

## Keyboard

```text
↑↓       move selection
Enter    open/select
Esc      back
s        save setup
c        compare
u        preview timeline undo
r        rescan / refresh
p        switch profile
/        search
q        quit
```

Section-specific actions can appear in the footer when focused:

```text
Skills focused:    a add skill     d remove
MCP focused:       a add mcp       e enable/disable   d remove
History focused:   u preview undo  c compare
```

## Empty States

### No Snapshots

```text
No saved setups yet.

Save your current setup to create a restore point.

s save setup
```

### No Timeline Events

```text
No timeline entries yet.

Save a setup to start local history.
```

### No Detected Agents

```text
No supported agent setup found.

Gandalf looks for Claude Code, Codex, Cursor, OpenCode, Pi Agent,
and project instruction files.

Run from a project directory or install one supported agent first.
```

### No Changes Since Last Save

```text
Current setup matches latest saved setup.

Latest
  61b8  Today 13:50  install react-review skill
```

## MVP Scope

Build now:

- Left nav with detected agents and an `All agents` timeline filter
- Timeline-first main screen
- Current Setup panel above Timeline with Skills, MCP Servers, Hooks, and Project tabs
- Agent detail inventory
- Full setup snapshots
- Deterministic snapshot titles
- Git-log-style history
- Compare with explicit From/To
- Structured side-by-side compare
- MCP-only dry-run timeline undo preview
- Save Setup with local history and optional `.gandalf` export

Defer:

- Full setup restore from timeline events
- Cloud profiles
- Team dashboard
- Marketplace
- Security finding dashboard
- Manual snapshot messages
- Agent-only restore
- Raw file diff tabs
- Policy enforcement

## Design Principle

The first useful moment is not installing a new MCP or skill.

The first useful moment is:

> I can see what is installed, see what changed, and restore the setup that worked.
