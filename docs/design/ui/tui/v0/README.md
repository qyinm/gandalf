# Hem TUI v0

## Product Frame

Hem v0 is a local Time Machine for AI coding agent setups.

The TUI should help a power user answer four questions quickly:

1. What is installed in each agent right now?
2. What changed since the last saved setup?
3. What does this setup look like compared with another saved point?
4. Can I restore a previous setup without guessing which files changed?

The app is not primarily a marketplace, security dashboard, or brand page. It is a local environment manager with Git-like history using setup-focused language.

## Core Model

| Concept | Git Analogy | User-Facing Term |
|---|---|---|
| Current local agent files | working tree | Current setup |
| Named environment line | branch | Profile |
| Saved state in a profile | commit | Snapshot / saved setup |
| Change list between states | diff | Compare |
| Revert to saved state | checkout/reset | Restore |
| Portable bundle | archive | `.hem` file |

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

A snapshot is one saved point inside a profile. It captures the full setup, not only one agent.

The snapshot contains:

- Claude Code state
- Codex state
- Cursor state
- OpenCode/Pi Agent state when supported
- Project instructions
- Shared MCP files
- Env key inventory

Agent screens show filtered history, but the saved unit is the full setup. This avoids confusing restores when shared files affect multiple agents.

### Deterministic Snapshot Titles

Users should not write save messages in v0. Snapshot titles are generated from structured diffs.

Priority order:

1. MCP added or removed
2. Skill added or removed
3. Hook or permission changed
4. Instruction changed
5. Env key inventory changed
6. Other settings changed

Examples:

- `add github mcp to Claude Code`
- `remove playwright mcp from Cursor`
- `install react-review skill`
- `update Claude Code permissions`
- `update project instructions`
- `change 2 MCPs and 1 skill`
- `capture baseline`
- `before restore to 61b8`

## Layout

The TUI uses a persistent left nav and a main detail pane.

```text
┌──────────────────────┬────────────────────────────────────────────────────────┐
│ Profiles             │ Claude Code                                            │
│ ● default            │ Profile: default                                       │
│   frontend           │                                                        │
│   clean-baseline     │ Current Setup                                          │
│                      │   Skills        8                                      │
│ Agents               │   MCP Servers   3                                      │
│ ● Claude Code        │   Hooks         2                                      │
│   Codex              │   Permissions   4                                      │
│   Cursor             │   Instructions  CLAUDE.md, AGENTS.md                  │
│   OpenCode           │                                                        │
│   Pi Agent           │ Skills                                                 │
│                      │   react-review                         installed       │
│ History              │   debugging                            installed       │
│   All changes        │   product-review                        installed       │
│   Snapshots          │                                                        │
│                      │ MCP Servers                                            │
│                      │   github                               enabled         │
│                      │   linear                               enabled         │
│                      │   playwright                           disabled        │
│                      │                                                        │
│                      │ History                                                │
│                      │ * 9f2a  Today 14:22  add github mcp to Claude Code     │
│                      │ * 61b8  Today 13:50  install react-review skill        │
│                      │ * b102  Yesterday    capture baseline                  │
├──────────────────────┴────────────────────────────────────────────────────────┤
│ ↑↓ move  Enter open  s save  c compare  r restore  p profile  q quit         │
└───────────────────────────────────────────────────────────────────────────────┘
```

Do not show a large `Hem` brand header. The selected profile, selected agent, and current setup are the primary context.

## Left Navigation

```text
Profiles
  default
  frontend
  clean-baseline

Agents
  Claude Code
  Codex
  Cursor
  OpenCode
  Pi Agent

History
  All changes
  Snapshots
```

Profiles appear first because they define the active environment line. Agents are shown inside the selected profile. History can be viewed globally.

## Main Screens

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
  Instructions  CLAUDE.md, AGENTS.md

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
  ./AGENTS.md

History
* 9f2a  Today 14:22  add github mcp to Claude Code
* 61b8  Today 13:50  install react-review skill
```

Agent detail is inventory-first. History is attached below the current setup. Add/remove actions are available from section context, not from the global first screen.

### All Changes

Shown when `History > All changes` is selected.

```text
All Changes
Profile: default

* 9f2a  Today 14:22  add github mcp to Claude Code
* 61b8  Today 13:50  install react-review skill
* b102  Yesterday    capture baseline
* 8aa1  May 28       import work mac profile

Selected: 9f2a

Changed
  + Claude Code MCP: github
  ~ ~/.claude/settings.json

Actions
  c compare   r restore
```

This should feel like `git log`, but without using `commit`, `branch`, `checkout`, or `reset` as user-facing labels.

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

`.hem` export is not a primary global action. It lives inside Save Setup as a destination option.

## Actions

### Save Setup

Save creates a local snapshot. Exporting `.hem` is a save destination, not a separate top-level mental model.

```text
Save current setup?

Detected changes
  + github mcp in Claude Code
  ~ ~/.claude/settings.json

This will be saved as
  add github mcp to Claude Code

Destinations
  ✓ Local history
  □ Export as .hem
  □ Cloud profile        Pro

[Save] [Cancel]
```

MVP only needs:

- Local history
- Export as `.hem`

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
│                               │ + allow bash: npm test          │
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

Original TUI v0 restore targets full setup restore. Daemon P0 deliberately narrows this:

- daemon timeline restore is MCP-only dry-run undo
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
r        restore
p        switch profile
/        search
q        quit
```

Section-specific actions can appear in the footer when focused:

```text
Skills focused:    a add skill     d remove
MCP focused:       a add mcp       e enable/disable   d remove
History focused:   c compare       r restore
```

## Empty States

### No Snapshots

```text
No saved setups yet.

Save your current setup to create a restore point.

s save setup
```

### No Detected Agents

```text
No supported agent setup found.

Hem looks for Claude Code, Codex, Cursor, OpenCode, Pi Agent,
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

- Left nav with profiles, agents, history
- Agent detail inventory
- Full setup snapshots
- Deterministic snapshot titles
- Git-log-style history
- Compare with explicit From/To
- Structured side-by-side compare
- Full setup restore
- Save Setup with local history and optional `.hem` export

Defer:

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
