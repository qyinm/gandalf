# Gandalf Desktop MVP Design

Status: working design document
Last updated: 2026-06-10

This document defines the desktop MVP screen structure and ASCII wireframes.

The product definition remains in `PRODUCT.md`. This file owns layout, screen hierarchy, and UI composition.

## Design Intent

Gandalf Desktop should feel like a focused Git client for the user's Codex setup.

The UI should be organized around the active profile and its setup surfaces:

```text
Home
Setup
MCP
Skills
Hooks
```

Profiles are not a sidebar section. Profiles are the primary context switcher at the top of the sidebar.

Team spaces are not a separate sidebar section in MVP. Team profiles appear inside the profile switcher, grouped by team space.

Timeline is not a sidebar section. The timeline appears as:

```text
Home bottom changelog
Snapshot dropdown from the custom window titlebar
Snapshot detail task screen
```

## Icon Policy

ASCII wireframes use text placeholders, but the real UI should use icons for compact repeated controls and status signals.

Use lucide icons where available.

Icon mapping:

```text
<chevrons-up-down>:
use ChevronsUpDown
opens profile picker

<settings-icon>:
use Settings
opens settings mode sidebar

Home:
use Home icon in sidebar

Setup:
use SlidersHorizontal or PanelTop icon in sidebar

MCP:
use Network or Cable icon in sidebar

Skills:
use Sparkles or Wand icon in sidebar

Hooks:
use TerminalSquare, Workflow, or Zap icon in sidebar

Back:
use ArrowLeft icon

Create Snapshot:
use Save or GitCommit icon with text

View Diff:
use GitCompare icon with text

Restore:
use RotateCcw icon with text

Push:
use Upload icon with text

Update from Remote:
use RefreshCcw icon with text

Publish:
use UploadCloud or Send icon with text

Comment:
use MessageSquare icon with text

Close:
use X icon with text

Risk:
use AlertTriangle icon for high risk
use CircleAlert icon for medium risk
use CheckCircle2 icon for low/no risk

Snapshot id in titlebar:
use text short id only, optionally with Clock or GitCommit icon if needed
```

Rules:

```text
Use icon-only buttons for familiar compact controls:
settings, back, close, dropdown, refresh, copy.

Use icon + text for destructive, state-changing, or primary actions:
Create Snapshot, Restore Snapshot, Switch Profile, Push, Publish.

Do not render placeholder labels like <settings-icon> or <chevrons-up-down> in product UI.
They are ASCII documentation placeholders only.

Do not make sidebar labels icon-only in MVP.
Use icon + label for Home, Setup, MCP, Skills, Hooks.
```

## App Shell

Recommended desktop frame:

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Main Content                                      |
|--------------------------------------------------------------------------------|
| Home                                                                           |
| Setup                                                                          |
| MCP                                                                            |
| Skills                                                                         |
| Hooks                                                                          |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

Minimum useful window:

```text
1100 x 720
```

The app uses four structural zones:

```text
Custom window titlebar:
Gandalf logo on the left
current snapshot short id on the right

Profile picker:
top of left sidebar
shows active profile name
uses chevrons-up-down icon
opens profile list grouped by personal profiles and team spaces

Left sidebar:
Home, Setup, MCP, Skills, Hooks

Main content:
overall state, setup surfaces, diffs, or task screens
```

Sidebar bottom:

```text
account user profile
settings icon
```

The account user profile is not a Gandalf setup profile.

```text
Gandalf setup profile:
Default, Frontend, qyinm-lab/frontend

Account user profile:
hippoo, hippoo@example.com, signed-in user identity
```

Clicking the settings icon changes the sidebar into settings navigation.

## Custom Window Titlebar

The custom window titlebar should stay minimal.

Left:

```text
Gandalf
```

Right:

```text
8f3a2c7
```

Clicking the snapshot id opens a compact timeline dropdown.

Do not put these in the custom window titlebar:

```text
active profile name
sync status
risk status
large primary actions
search
team selector
```

Those belong in Home or the active screen, not the window chrome.

## Snapshot Dropdown

The current snapshot id in the custom window titlebar opens a compact timeline list.

Purpose:

```text
quickly inspect recent snapshots
open snapshot detail
restore from a recent snapshot
```

Dropdown:

```text
+------------------------------------------+
| Current Snapshot                         |
| 8f3a2c7                                  |
|------------------------------------------|
| Recent Timeline                          |
| 8f3a2c7  12 min ago  Manual      High    |
| 72ab91d  1h ago      Auto        Medium  |
| 19df02a  Yesterday   Initial     Low     |
|------------------------------------------|
| [Open Timeline] [Restore Snapshot]       |
+------------------------------------------+
```

This dropdown should stay small. It is not the full timeline view.

## Profile Picker

The top of the sidebar is the profile picker.

It replaces the old `Personal Space` label.

Collapsed:

```text
Default <chevrons-up-down>
```

`<chevrons-up-down>` is documentation shorthand. The actual UI should render the ChevronsUpDown icon, not that text.

Expanded:

```text
+------------------------------------------+
| Profiles                                 |
|------------------------------------------|
| Personal                                 |
| * Default                 local only     |
|   Frontend                ahead 2        |
|   Backend                 up to date     |
|------------------------------------------|
| qyinm-lab                                |
|   qyinm-lab/frontend      behind 1       |
|   qyinm-lab/backend       up to date     |
|------------------------------------------|
| acme                                     |
|   acme/security           up to date     |
|------------------------------------------|
| [Create Profile] [Join Team]             |
+------------------------------------------+
```

Rules:

```text
Personal profiles appear first.
Each team space appears as a grouped section.
Team profiles are listed inside their team space.
The active profile uses a selected marker.
Rows show sync status when relevant.
Selecting a profile opens Switch Preview before writing files.
```

Do not show `Personal Space` as a static heading in the sidebar.

## Left Navigation

Primary sidebar sections:

```text
Home
Setup
MCP
Skills
Hooks
```

Removed from sidebar:

```text
Profiles
Team
Current Setup
Timeline
```

Where removed sections went:

```text
Profiles:
profile picker at top of sidebar

Team:
team profiles grouped inside profile picker
team proposal actions appear from team-tracking profile context

Current Setup:
renamed and split into Setup, MCP, Skills, Hooks

Timeline:
Home bottom changelog
snapshot dropdown
snapshot detail task screen
```

Task screens opened from relevant objects:

```text
Switch Preview
Diff
Snapshot Detail
Restore Preview
Cloud Sync
Team Proposal Review
Profile Proposal Create
```

Do not add these as primary sidebar sections in MVP:

```text
Scan
Audit
Graph
Provenance
Bundle
Admin
Marketplace
Compare
Restore
Profiles
Team
Timeline
Current Setup
Settings as a main nav item
```

## Account User Profile

The bottom of the sidebar should show the signed-in account user profile.

This is separate from Gandalf setup profiles.

Collapsed account row:

```text
hippoo                                      <settings-icon>
```

`<settings-icon>` is documentation shorthand. The actual UI should render the Settings icon, not that text.

Signed-out account row:

```text
Local user                                  <settings-icon>
```

Rules:

```text
Account row stays at the bottom of the sidebar.
Account row is not a profile switcher.
Clicking the account name can open account details.
Clicking the settings icon switches the sidebar into settings mode.
Protection, Cloud, and Device status should not be permanently visible in the sidebar footer.
```

## Settings Mode Sidebar

Clicking the account row settings icon changes the left navbar to settings-specific items.

Settings mode:

```text
< Back

Account
Cloud
Device
Protection
Notifications
Local Paths
Privacy
About

hippoo                                      <settings-icon>
```

Rules:

```text
Settings are not a main app nav item.
Settings mode replaces the main nav until the user goes Back.
Settings mode keeps the account row at the bottom.
Protection, Cloud, and Device are settings sections, not persistent footer labels.
```

## Home Screen Contract

Home has two main vertical zones:

```text
Top:
Overall

Bottom:
Changelog / timeline
```

Overall should answer:

```text
Which profile is active?
What is the current snapshot?
Is the setup protected?
Are there working changes?
What is the highest risk?
Is this profile local-only, ahead, behind, or up to date?
What is the next safe action?
```

Changelog should behave like the TUI timeline:

```text
recent snapshots
automatic setup changes
risk events
restore events
push/update events
team proposal events
```

Primary actions on Home:

```text
[Create Snapshot]
[View Diff]
[Restore Previous]
[Push]
[Update from Remote]
```

Home should not become a full team dashboard, graph view, or analytics screen.

## Setup Screen Contract

Setup is the high-level current setup overview.

Purpose:

```text
show all supported Codex setup surfaces in one place
summarize MCP, skills, hooks, permissions, env keys, and unsupported state
route users to detailed setup surface screens
```

Sections:

```text
Overall
MCP summary
Skills summary
Hooks summary
Permissions
Env key inventory
Unsupported / low-confidence items
Affected files
```

Setup is read-only in MVP. Direct editing comes later.

## MCP Screen Contract

MCP shows configured MCP servers for the active profile.

Must show:

```text
server name
transport/command summary
risk
env keys required
source file
status
```

Actions:

```text
[View Diff]
[Create Snapshot]
```

Do not execute MCP commands during scan.

## Skills Screen Contract

Skills shows supported Codex skills/tools/prompt surfaces when available.

Must show:

```text
skill name
source path
status
risk
last changed snapshot
```

If skills are not supported yet:

```text
Skills
No supported Codex skills found.

Gandalf will show supported skill surfaces here when Codex support is available.
```

## Hooks Screen Contract

Hooks shows executable or automation-related Codex setup.

Must show:

```text
hook name
trigger
command/path summary
risk
source file
last changed snapshot
```

Hooks should be visually treated as the riskiest setup surface.

Actions:

```text
[View Diff]
[Restore Previous]
[Create Snapshot]
```

Do not execute hooks during scan.

## Settings Screen Contract

Settings should stay narrow in MVP and should be reached from the account row settings icon.

Settings mode sidebar sections:

```text
Back
Account
Cloud
Device
Protection
Notifications
Local Paths
Privacy
About
```

Default settings screen:

```text
Account
```

## ASCII Wireframes

These wireframes are intentionally plain ASCII.

They are not visual design. They define layout, hierarchy, and product weight.

### Home

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Home                                              |
|--------------------------------------------------------------------------------|
| > Home                    Overall                                              |
|   Setup                   ---------------------------------------------------  |
|   MCP                     Active Profile                                       |
|   Skills                  Default                                              |
|   Hooks                   Local only                                           |
|                           Current snapshot: 8f3a2c7                            |
|                           Last snapshot: 12 min ago                            |
|                                                                                |
|                           Setup Status                                          |
|                           ---------------------------------------------------  |
|                           3 changes since last snapshot                        |
|                           Risk: High                                           |
|                                                                                |
|                           Changed                                              |
|                           + MCP server: figma                                  |
|                           + Hook: postToolUse                                  |
|                           ~ Codex permissions                                  |
|                                                                                |
|                           Actions                                              |
|                           [Create Snapshot] [View Diff] [Restore Previous]     |
|                                                                                |
|                           Changelog                                            |
|                           ---------------------------------------------------  |
|                           8f3a2c7  12 min ago  Manual snapshot      High       |
|                           72ab91d  1h ago      Codex config changed Medium     |
|                           19df02a  Yesterday   First snapshot       Low        |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Profile Picker Dropdown

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Home                                              |
| +------------------------------------------+                                   |
| | Profiles                                 |                                   |
| |------------------------------------------|                                   |
| | Personal                                 |                                   |
| | * Default                 local only     |                                   |
| |   Frontend                ahead 2        |                                   |
| |   Backend                 up to date     |                                   |
| |------------------------------------------|                                   |
| | qyinm-lab                                |                                   |
| |   qyinm-lab/frontend      behind 1       |                                   |
| |   qyinm-lab/backend       up to date     |                                   |
| |------------------------------------------|                                   |
| | [Create Profile] [Join Team]             |                                   |
| +------------------------------------------+                                   |
| > Home                                                                         |
|   Setup                                                                        |
|   MCP                                                                          |
|   Skills                                                                       |
|   Hooks                                                                        |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Snapshot Dropdown

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|                                                        +---------------------+ |
|                                                        | Current Snapshot    | |
|                                                        | 8f3a2c7             | |
|                                                        |---------------------| |
|                                                        | 8f3a2c7  12m High   | |
|                                                        | 72ab91d  1h  Medium | |
|                                                        | 19df02a  1d  Low    | |
|                                                        |---------------------| |
|                                                        | [Open Timeline]     | |
|                                                        | [Restore Snapshot]  | |
|                                                        +---------------------+ |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Home                                              |
|--------------------------------------------------------------------------------|
| > Home                    Overall                                              |
|   Setup                                                                        |
|   MCP                                                                          |
|   Skills                                                                       |
|   Hooks                                                                        |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Setup

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Setup                                             |
|--------------------------------------------------------------------------------|
|   Home                    Setup Overview                                       |
| > Setup                   ---------------------------------------------------  |
|   MCP                     Active Profile: Default                              |
|   Skills                  Snapshot: 8f3a2c7                                    |
|   Hooks                                                                        |
|                           MCP                                                  |
|                           2 servers configured                 Medium          |
|                                                                                |
|                           Skills                                               |
|                           No supported Codex skills found                       |
|                                                                                |
|                           Hooks                                                |
|                           1 hook configured                    High            |
|                                                                                |
|                           Permissions                                          |
|                           Bash allowed                         Medium          |
|                                                                                |
|                           Env Keys                                             |
|                           FIGMA_TOKEN, GITHUB_TOKEN            values hidden   |
|                                                                                |
|                           Unsupported                                          |
|                           1 unknown config block               Needs review    |
|                                                                                |
|                           [View Diff] [Create Snapshot]                        |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### MCP

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   MCP                                               |
|--------------------------------------------------------------------------------|
|   Home                    MCP Servers                                          |
|   Setup                   ---------------------------------------------------  |
| > MCP                     Name        Status       Risk      Source            |
|   Skills                  ---------------------------------------------------  |
|   Hooks                   figma       configured   Medium    config.toml       |
|                           github      configured   Medium    config.toml       |
|                                                                                |
|                           Selected                                             |
|                           ---------------------------------------------------  |
|                           figma                                                |
|                           Command configured                                   |
|                           Env key: FIGMA_TOKEN                                 |
|                           Gandalf did not execute this MCP command.                |
|                                                                                |
|                           [View Diff] [Create Snapshot]                        |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Skills

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Skills                                            |
|--------------------------------------------------------------------------------|
|   Home                    Skills                                               |
|   Setup                   ---------------------------------------------------  |
|   MCP                     No supported Codex skills found.                     |
| > Skills                                                                       |
|   Hooks                   Gandalf will show supported skill surfaces here when      |
|                           Codex support is available.                          |
|                                                                                |
|                           [Create Snapshot]                                    |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Hooks

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Hooks                                             |
|--------------------------------------------------------------------------------|
|   Home                    Hooks                                                |
|   Setup                   ---------------------------------------------------  |
|   MCP                     Name          Trigger       Risk       Source        |
|   Skills                  ---------------------------------------------------  |
| > Hooks                   postToolUse   after tool    High       config.toml   |
|                                                                                |
|                           Selected                                             |
|                           ---------------------------------------------------  |
|                           postToolUse                                          |
|                           Can execute shell commands during Codex use.         |
|                           Gandalf did not execute this hook.                       |
|                                                                                |
|                           [View Diff] [Restore Previous] [Create Snapshot]     |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Diff

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Diff                                              |
|--------------------------------------------------------------------------------|
|   Home                    Changes Since Last Snapshot                          |
| > Setup                   ---------------------------------------------------  |
|   MCP                     Added                                                |
|   Skills                  + MCP server: figma                                  |
|   Hooks                   + Hook: postToolUse                                  |
|                                                                                |
|                           Changed                                              |
|                           ~ Codex permissions                                  |
|                                                                                |
|                           Removed                                              |
|                           - MCP server: old-docs                               |
|                                                                                |
|                           Risk                                                 |
|                           High - new hook can execute shell commands.          |
|                                                                                |
|                           Affected Files                                       |
|                           ~/.codex/config.toml                                 |
|                                                                                |
|                           [Create Snapshot] [Restore Previous] [Back]          |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Switch Preview

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Switch Profile                                    |
|--------------------------------------------------------------------------------|
|   Home                    Switch Profile                                       |
|   Setup                   ---------------------------------------------------  |
|   MCP                     From                                                 |
|   Skills                  Default @ 8f3a2c7                                    |
|   Hooks                                                                        |
|                           To                                                   |
|                           Frontend @ 19df02a                                   |
|                                                                                |
|                           Will Change                                           |
|                           + MCP server: figma                                   |
|                           ~ Codex permissions                                  |
|                           - MCP server: old-docs                               |
|                                                                                |
|                           Before Switching                                     |
|                           Gandalf will create a rollback snapshot first.           |
|                                                                                |
|                           [Switch Profile] [Cancel]                            |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Restore Preview

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Default <chevrons-up-down>   Restore Snapshot                                  |
|--------------------------------------------------------------------------------|
|   Home                    Restore Snapshot                                     |
|   Setup                   ---------------------------------------------------  |
|   MCP                     Snapshot: 8f3a2c7                                    |
|   Skills                                                                       |
|   Hooks                   Will Change                                          |
|                           + MCP server: figma                                  |
|                           ~ Codex permissions                                  |
|                           - MCP server: old-docs                               |
|                                                                                |
|                           Secrets                                              |
|                           FIGMA_TOKEN value is not stored by Gandalf.              |
|                           Current local value will be preserved.               |
|                                                                                |
|                           Before Restoring                                     |
|                           Gandalf will create a rollback snapshot first.           |
|                                                                                |
|                           [Restore Snapshot] [Cancel]                          |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Team Proposal Review

Team proposal review is opened from a team-tracking profile context, not from a permanent Team sidebar item.

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| qyinm-lab/frontend <chevrons-up-down>   Proposal #42                           |
|--------------------------------------------------------------------------------|
|   Home                    Profile Proposal #42                                 |
|   Setup                   ---------------------------------------------------  |
|   MCP                     Title                                                |
|   Skills                  Add Figma MCP and update permissions                 |
|   Hooks                                                                        |
|                           Profile                                              |
|                           qyinm-lab/frontend                                   |
|                                                                                |
|                           Snapshot                                             |
|                           8f3a2c7                                               |
|                                                                                |
|                           Semantic Diff                                        |
|                           + MCP server: figma                                  |
|                           ~ Codex permissions                                  |
|                                                                                |
|                           Risk                                                 |
|                           High - hook can execute commands.                    |
|                                                                                |
|                           Comments                                             |
|                           alice: Needed for frontend design review.            |
|                                                                                |
|                           [Publish] [Comment] [Close]                          |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

### Settings

```text
+--------------------------------------------------------------------------------+
| Gandalf                                                          8f3a2c7            |
|--------------------------------------------------------------------------------|
| Settings                         Account                                       |
|--------------------------------------------------------------------------------|
| < Back                    Account                                              |
|                                                                                |
| > Account                 User                                                 |
|   Cloud                   ---------------------------------------------------  |
|   Device                  hippoo@example.com                                   |
|   Protection                                                                   |
|   Notifications           Plan                                                 |
|   Local Paths             Local only                                           |
|   Privacy                                                                      |
|   About                   Device                                               |
|                           Work Mac                                             |
|                                                                                |
|                           Protection                                           |
|                           On                                                    |
|                                                                                |
|                           Notifications                                        |
|                           Medium and high risk                                 |
|                                                                                |
|                           Privacy                                              |
|                           Raw secrets are not uploaded.                        |
|                           Commands, hooks, and skills are not executed.        |
|                                                                                |
|                           [Rename Device] [Notification Settings] [Sign Out]   |
|                                                                                |
| hippoo                                      <settings-icon>                    |
+--------------------------------------------------------------------------------+
```

## Onboarding Screens

```text
+--------------------------------------------------------------------------------+
| Welcome to Gandalf                                                                |
|-------------------------------------------------------------------------------|
| Gandalf will scan your user-global Codex setup and create a local Default profile. |
|                                                                                |
| [Continue]                                                                     |
+--------------------------------------------------------------------------------+
```

```text
+--------------------------------------------------------------------------------+
| Current Codex Setup Found                                                     |
|-------------------------------------------------------------------------------|
| Detected                                                                       |
| ~/.codex/config.toml                                                           |
| 2 MCP servers                                                                  |
| 1 permission change                                                            |
| 3 env keys                                                                     |
|                                                                                |
| [Create Default Profile]                                                       |
+--------------------------------------------------------------------------------+
```

```text
+--------------------------------------------------------------------------------+
| Default Profile Created                                                       |
|-------------------------------------------------------------------------------|
| First snapshot                                                                 |
| 8f3a2c7                                                                        |
|                                                                                |
| [Open Dashboard]                                                               |
+--------------------------------------------------------------------------------+
```

```text
+--------------------------------------------------------------------------------+
| Enable Protection?                                                            |
|-------------------------------------------------------------------------------|
| Gandalf can watch Codex setup changes and notify you when risky changes happen.    |
|                                                                                |
| [Enable Protection] [Not Now]                                                  |
+--------------------------------------------------------------------------------+
```

```text
+--------------------------------------------------------------------------------+
| Sync Across Devices                                                           |
|-------------------------------------------------------------------------------|
| Sign in to attach this local profile to your personal cloud remote.            |
|                                                                                |
| [Sign In] [Keep Local]                                                         |
+--------------------------------------------------------------------------------+
```

If the app was opened from a team invite, show invite context before local setup, but still create the local Default profile and first snapshot before applying any team profile.

## Desktop MVP Cut Line

Must ship:

```text
Home
Setup
MCP
Skills
Hooks
Profile picker
Snapshot dropdown
Switch Preview
Diff
Snapshot Detail
Restore Preview
Cloud sync status
Team lightweight proposals
Protection status/settings
```

Later:

```text
raw file editor
inline proposal comments
request changes
required approvals
team drift dashboard
advanced graph/provenance explorer
profile tags/releases
partial restore
full timeline sidebar
```

Never as primary MVP UI:

```text
daemon management
security score dashboard
analytics dashboard
marketplace
repo browser
GitHub organization mapping
Profiles sidebar item
Team sidebar item
Timeline sidebar item
Current Setup sidebar item
```
