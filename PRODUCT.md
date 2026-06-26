# Gandalf Product Definition

Status: working product document
Last updated: 2026-06-12

## Current Gate 2 CLI Wedge

The current implementation target is not the full profile/cloud/team product described below.

Gate 2 is a Codex-only rollback demand test:

```text
snapshot -> diff -> restore
scope: --agent codex --scope user
surface: ~/.codex/ user-global setup only
distribution: CLI first
public install paths: install.sh and Homebrew tap
source repository: qyinm/gandalf
```

Gandalf is distributed as a Go binary. The supported install paths are the latest-release `install.sh` flow and the personal Homebrew tap command `brew install qyinm/tap/gandalf`. npm is no longer a supported product distribution channel; external package deletion is handled outside this repository.

In scope now:

- content-backed snapshots for supported Codex user-global files
- byte-exact restore for supported non-secret content
- explicit unsupported reasons for surfaces Gandalf cannot safely restore yet
- tests that prove rollback from a damaged `~/.codex/config.toml`

Not in scope now:

- desktop launch
- profiles and profile switching
- team/cloud sync
- broad multi-agent restore
- repo-local setup management

The larger "Git branches for Codex setups" product direction remains useful, but implementation should not expand into it until the rollback demand test clears.

## One-Line Definition

Gandalf lets developers snapshot, diff, and roll back their user-global Codex setup after risky setup experiments.

Korean:

> 내 Mac의 Codex 설정 변경을 저장하고, 비교하고, 필요하면 되돌리세요.

## Product Thesis

AI coding agents are becoming part of a developer's local operating environment. For MVP, Gandalf focuses on Codex: global config, MCP setup, instructions, permissions, and env key inventory together form a real setup layer.

That setup layer is powerful, but it is easy to lose control of:

- agents install MCP servers
- agents add skills
- agents edit instructions
- agents change hooks and permissions
- new machines require setup reconstruction
- team onboarding requires a consistent agent environment

Gandalf exists because this layer needs the same basic safety primitive developers already expect from source code:

```text
branch
diff
commit/save
checkout/switch
rollback
remote/team profile
```

Gandalf's core product idea is:

> Git branches for Codex setups.

The product should not be understood as a scanner, backup tool, audit dashboard, or marketplace. Those are implementation capabilities or adjacent surfaces. The user-facing product is a setup branch manager and safety layer for AI coding agent environments.

## Decision Log

### D1. MVP Is Global-Only

The MVP manages user-global agent setup only.

Gandalf should not manage, scan, diff, risk-score, switch, or restore repo-local setup files in the MVP.

Out of scope for MVP:

```text
.claude user-global setup
.cursor user-global setup
.mcp.json
.cursor/mcp.json inside a repo
AGENTS.md
CLAUDE.md
project-local instructions
project-local MCP config
```

In scope for MVP:

```text
~/.codex/config.toml
other supported Codex user-global config
```

Product implication:

> Gandalf profile switch should feel like changing the Codex environment on this Mac.

Safety implication:

> Every user-global write requires preview, parser confidence, an automatic snapshot, and clear risk explanation before apply.

Rationale:

> Git already owns repo-local files. Gandalf should not duplicate Git's responsibility in the MVP.

This makes the product simpler, safer, and easier to understand. Repo-local support can become a later explicit feature only if users strongly need it.

### D2. Active Profile Is Machine-Wide

Because MVP is global-only, Active Profile is machine-wide.

Gandalf should not support project-specific active profiles or machine profile + project overlay in the MVP.

User-facing language:

```text
Active Profile
Applies to this Mac
```

This keeps the first desktop dashboard and CLI status simple:

```text
Mac Setup
Default

Working Changes
3 changes since last snapshot
```

### D3. Team Profile Changes Use Lightweight Proposals

A profile behaves like a Git branch.

Users can switch to a profile, work on it locally, save changes, and continue iterating. When a teammate wants to share those changes with the team, they do not directly overwrite the approved team profile.

The team workflow should be:

```text
Switch profile
Work locally
Automatic snapshots accumulate locally
Push local snapshots
Open Profile Proposal
Review semantic diff and risk
Approve
Publish new team profile snapshot
```

Git analogy:

```text
git branch  = Gandalf profile
git commit  = Snapshot / Create Snapshot
git push    = Push profile changes
proposal    = lightweight request to publish a team profile snapshot
merge       = Approve + Publish team profile snapshot
```

Product implication:

> Team profiles are trusted distribution artifacts, not casually mutable shared documents.

Normal team members can propose, review, approve, and publish by default unless the team assigns stricter roles.

### D4. Team Profiles Are Remote Branches

Using a team profile should feel like checking out a remote branch locally.

The user is not editing the remote team profile directly. Gandalf creates or updates a local tracking profile based on the team profile.

```text
Remote team profile
= qyinm-lab/frontend

Local tracking profile
= alice/frontend tracking qyinm-lab/frontend
```

When the local agent environment changes while the user is on that profile, Gandalf should create snapshots in the local tracking profile.

```text
Team profile selected
Agent environment changes
Gandalf creates local snapshot
Local profile is now ahead of team profile
```

If the user decides the changes are good, they push/propose them back to the team.

```text
Push local snapshots
Open Profile Proposal
Review semantic diff and risk
Approve
Publish/merge into team profile
```

Git analogy:

```text
git checkout origin/frontend = Switch to Team Profile
local branch tracking remote = Local tracking profile
local commits = Automatic snapshots
git push = Push profile changes
profile proposal = lightweight publish request
merge = Publish team profile snapshot
```

Product implication:

> Fork is not required for normal team profile work. A fork is only for intentional long-lived personal divergence.

### D5. No Stash In MVP

Snapshot and restore are enough for MVP.

Gandalf should not add a separate stash feature in the first product model.

Rationale:

```text
Snapshot = commit current setup into active profile
Restore = return to any earlier snapshot
Save as New Profile = preserve current setup under a new branch
```

This covers the main reasons a user would need stash:

- avoid losing changes before switching
- preserve an experiment
- return to a previous setup

Product implication:

> Profile switching should rely on automatic snapshots and restore, not a separate temporary shelving model.

### D6. Snapshot Is Profile Save

Users are always using a profile.

There is no meaningful product state where the user is outside a profile.

```text
Active Profile
= the profile currently applied to this Mac
```

Snapshot and Save Profile are the same core action.

```text
Snapshot
= Save current supported user-global setup to the active profile
= Create immutable setup commit in the active profile history
```

Git analogy:

```text
git branch = Gandalf profile
git commit = Snapshot / Create Snapshot
```

Product implication:

```text
Create Snapshot, Save Profile, Save Setup, and Snapshot all refer to committing the current setup into the active profile.
```

Rollback uses older snapshots from the profile history.

Before risky operations, Gandalf can create an automatic snapshot in the current profile so the user can return to it later. That automatic snapshot is still a profile save, not a separate storage model.

### D7. Desktop MVP Is Dashboard Plus Lightweight Menu Bar

Desktop MVP should include both:

```text
Full dashboard
+ lightweight menu bar companion
```

The dashboard is the primary product surface.

```text
Dashboard
= profile branch manager
= create snapshot / switch / restore / compare / team proposal
```

The menu bar is the always-visible status and quick-action surface.

```text
Menu bar
= active profile
= protection/risk status
= last snapshot
= open dashboard
= quick snapshot / restore
```

Product implication:

> Gandalf desktop is not just a background watcher. It is a global AI agent setup branch manager with a lightweight always-on status surface.

### D8. High-Risk Changes Warn, But Do Not Block Snapshot

High-risk changes should warn, label, and explain. They should not block local snapshot creation.

Rationale:

```text
Snapshot = history
History must not have gaps
High-risk changes are exactly the changes Gandalf must preserve
```

If Gandalf blocks high-risk snapshots, the user loses the audit trail for the most important moments.

Product behavior:

```text
High-risk change detected
Create snapshot anyway
Mark snapshot as high risk
Explain why
Offer restore
Require review before team publish
```

High risk should affect presentation and review, not local recording.

Team implication:

> A high-risk local snapshot can be pushed/proposed, but the proposal must clearly surface the risk to reviewers.

### D9. Profile Sync Uses Git-Style Ahead/Behind

Profiles can have a cloud remote.

This applies to both personal profiles and team profiles:

```text
Personal cloud profile
= user's private remote branch

Team profile
= team's shared remote branch
```

When a local profile tracks a cloud profile, Gandalf should show Git-like sync status:

```text
up to date
ahead by 3 snapshots
behind by 2 snapshots
ahead 3, behind 2
diverged
```

Product implication:

> Cloud sync should feel like pushing and pulling profile branches, not like syncing opaque settings.

Personal subscription implication:

> Paid personal cloud sync can use the same ahead/behind model as team profiles.

Example:

```text
Personal Profile
Default
Remote: cloud/hippoo/default
Status: ahead by 2 snapshots

Team Profile
qyinm-lab/frontend
Remote: qyinm-lab/frontend
Status: behind by 1 snapshot
```

### D10. No Version Or Release Labels In MVP

Gandalf should not assign or ask for human-facing version numbers or release labels in MVP.

Git analogy:

```text
snapshot id = commit hash
profile remote = branch
```

Every snapshot should have an automatic short snapshot id:

```text
8f3a2c7
72ab91d
19df02a
```

Profiles should not show `v1`, `v2`, `v1.5.0`, `frontend-stable`, or `2026-06-safe` in MVP.

Team publish should be identified by the snapshot id that was published.

```text
qyinm-lab/frontend
Published snapshot: 8f3a2c7
```

If a human needs to discuss a team publish, they should refer to the profile and snapshot id:

```text
qyinm-lab/frontend @ 8f3a2c7
```

Named tags or release labels can be reconsidered later, but they should not be part of the first product model.

### D11. Primary Commit Action Is Create Snapshot

The primary UI action for committing current setup should be:

```text
Create Snapshot
```

Reason:

```text
Profile = branch
Snapshot = commit
Create Snapshot = commit current setup to active profile
```

`Save Profile` and `Save Setup` can remain explanatory aliases, but they should not be the primary button label in the product.

Recommended primary actions:

```text
[Create Snapshot]
[Save as New Profile]
[Restore Snapshot]
[Push Snapshots]
```

Product implication:

> Gandalf should teach users that profiles are branches and snapshots are commits.

### D12. Cloud Divergence Uses Pull-Rebase By Default

When a local profile is both ahead and behind its cloud remote, Gandalf should use a pull-rebase model by default.

Git analogy:

```text
git fetch
git pull --rebase
git push
```

User-facing language:

```text
Update from Remote
```

Behavior:

```text
Status: ahead 3, behind 2

1. Fetch remote snapshots
2. Replay local snapshots on top of latest remote snapshot
3. If clean, local profile becomes ahead 3, behind 0
4. User can push snapshots
```

Clean update UI:

```text
Default
Remote: cloud/hippoo/default
Status: ahead 3, behind 2

Remote has 2 newer snapshots.
Your 3 local snapshots will be replayed on top.

[Update from Remote] [View Details]
```

After update:

```text
Updated from remote

Status:
ahead by 3 snapshots

[Push Snapshots]
```

Conflict behavior:

```text
Update needs review

Remote changed:
~ ~/.codex/config.toml permissions

Local changed:
~ ~/.codex/config.toml permissions

[Review Conflict]
[Keep Local]
[Use Remote]
[Create New Profile]
```

Product implication:

> Default to Git-like pull-rebase, but stop for explicit review when replaying snapshots would overwrite or conflict with supported setup state.

### D13. MVP Restore Is Whole Snapshot Restore

MVP restore should restore a whole user-global setup snapshot.

```text
Restore Snapshot 8f3a2c7
= restore the supported user-global setup captured in that snapshot
```

MVP should not support partial restore by setup area.

Out of scope for MVP:

```text
Restore only MCP servers
Restore only skills
Restore only hooks
Restore only permissions
Restore only selected fields inside settings
```

Rationale:

```text
Profile = branch
Snapshot = commit
Restore Snapshot = restore commit state
```

Whole snapshot restore keeps the first product model simple, predictable, and safer to implement.

Partial restore can be reconsidered later as an advanced feature:

```text
Restore selected changes
Restore MCP servers only
Restore hooks only
```

But partial restore requires stronger parser confidence, field-level merge logic, and more complex conflict UI.

### D14. Cloud Sync Does Not Upload Raw Secrets

MVP cloud sync must not upload raw secret values.

Allowed in cloud snapshots:

```text
config structure
MCP server names
command paths
permission rules
hook metadata
skill content when supported
env key names
masked token-like fields
```

Not allowed in MVP cloud snapshots:

```text
raw env values
API tokens
OAuth tokens
provider secrets
unknown secret-shaped values
private key material
```

Cloud sync should preserve secret inventory without preserving secret values.

Example:

```text
FIGMA_TOKEN=<redacted>
OPENAI_API_KEY=<redacted>
```

Future option:

> Gandalf may later support end-to-end encrypted secret sync, similar in spirit to 1Password, where Gandalf's cloud cannot read the secret values.

That future feature should be explicit, opt-in, and separate from default profile sync.

### D15. MVP Agent Support Is Codex-Only

Productized MVP should support Codex only.

MVP in scope:

```text
~/.codex/config.toml
Codex MCP/global config when supported
Codex instructions/settings when supported
Codex-related env key inventory
```

MVP out of scope:

```text
Claude Code
Cursor
OpenCode
Pi Agent
multi-agent setup switching
```

Rationale:

```text
Codex-only MVP
= narrower parser surface
= safer restore semantics
= faster desktop MVP
= clearer product dogfooding
```

Claude Code and Cursor remain important expansion targets, but they should not block the first productized MVP.

### D16. Automatic Snapshots Use Stable Debounced Groups

Automatic snapshots should use debounced change groups, not one snapshot per file write.

Default behavior:

```text
First Codex setup file changes
Start snapshot group
Wait for quiet period
Create one snapshot for the grouped changes
```

Recommended defaults:

```text
Quiet period: 30 seconds after last change
Maximum group window: 2 minutes after first change
High-risk change: shorten quiet period to 5 seconds
```

Gandalf must flush pending snapshot groups before important actions:

```text
before switch
before restore
before push
before update from remote
before app quit
before system shutdown when detectable
```

Stability requirements:

```text
Never lose a detected pending change.
Persist pending snapshot metadata to local state.
If the app crashes, recover and create a snapshot on next launch.
If parsing fails, create a raw/low-confidence snapshot marker and explain the parser failure.
Do not let debounce delay block explicit user actions.
```

Product implication:

> Automatic snapshots should feel meaningful, not noisy, while preserving history even under crash or shutdown conditions.

### D17. Team Permissions Are Role-Based, Open By Default

Team Space permissions should be role/capability based.

Teams can create roles and assign capabilities to members.

Default team policy should be open:

```text
All members can create proposals.
All members can review proposals.
All members can publish snapshots.
Approve is the final confirmation performed by the member who publishes.
```

This means `approve` is not a separate privileged gate by default. It is the publishing member's explicit confirmation that the snapshot should become the team remote's current published snapshot.

Default publish flow:

```text
Member pushes local snapshots
Member creates Profile Proposal
Team members review/comment
Publishing member approves
Publishing member publishes snapshot
Team remote moves to that snapshot
```

Teams can still define stricter custom roles later:

```text
reviewer
publisher
admin
security reviewer
```

But those are optional team policy choices, not MVP defaults.

### D18. MVP Ships With Member Role Only

MVP should expose one visible team role:

```text
Member
```

Visible MVP behavior:

```text
Member can create proposals.
Member can review proposals.
Member can publish snapshots.
Member approves their own publish action.
```

Internal model should still be capability-based:

```text
role:
  name: "Member"
  capabilities:
    - proposal:create
    - proposal:review
    - snapshot:publish
    - team:invite
```

Reason:

```text
Simple team UI now.
Flexible permission model later.
No premature enterprise governance surface.
```

Future optional presets:

```text
Reviewer
Publisher
Admin
Security Reviewer
```

These should not be visible in the first MVP unless the team explicitly enables advanced permissions later.

### D19. MVP Team Access Uses Invites Only

MVP Team Space access should use:

```text
email invite
invite link
```

Gandalf should not use GitHub organization mapping in the MVP.

Reason:

```text
Gandalf's team model is about shared Codex setup profiles.
It is not primarily about repositories, GitHub organizations, or source control permissions.
GitHub org mapping would add unnecessary identity and organization complexity.
Invite-based access is enough to share team profile remotes.
```

MVP team access UI:

```text
Team Settings

Members
- jiyoon@example.com        Member
- minsu@example.com         Member

Actions:
[Invite by Email]
[Create Invite Link]
```

Invite behavior:

```text
Email invite grants Member access to the Team Space.
Invite link grants Member access to the Team Space.
Members can invite other members by default.
Invite links should be revocable.
Invite links should have an expiration policy.
```

Out of MVP:

```text
GitHub organization mapping
domain-based auto join
SCIM
SAML
enterprise identity sync
```

### D20. Invite Link Safety Uses Hybrid Policy

MVP invite policy should be hybrid:

```text
Email invites are email-bound.
Generic invite links are allowed.
Generic invite links expire, are revocable, and only grant Member access.
```

Expected results:

```text
Email invite:
- Sent to a specific email address.
- Only that email identity can accept it.
- User joins as Member.
- Team member list shows the invited email before acceptance.

Generic invite link:
- Anyone with the link can open the join page.
- Link always grants Member access.
- Link has an expiration date.
- Link can be revoked by any Member.
- Link acceptance is recorded in team activity.
```

MVP UI:

```text
Invite Members

[ Email ]
teammate@example.com

[Send Invite]

Invite Link
gandalf.dev/join/qyinm-lab/7k3f...

Expires in: 7 days
Role: Member

[Copy Link] [Revoke Link]
```

Join UI:

```text
Join qyinm-lab

You will join as:
Member

This gives access to:
- shared team profiles
- team profile remotes
- profile proposals
- review and publish actions

[Join Team]
```

Safety behavior:

```text
Expired link:
This invite link has expired.

Revoked link:
This invite link has been revoked.

Already member:
You are already a member of this Team Space.

Wrong email for email-bound invite:
This invite was sent to a different email address.
```

### D21. Cloud Login Uses Browser Device Code

MVP Gandalf Cloud login should use browser-based device code approval.

Core model:

```text
Desktop or CLI requests device login.
Gandalf shows a short device code.
User opens Gandalf Cloud in the browser.
User signs in with email code.
User approves this Mac or CLI session.
Gandalf desktop/CLI receives a device session token.
```

This should be the default for:

```text
desktop app login
CLI login
team invite acceptance
personal cloud remotes
team profile remotes
subscription ownership
```

Desktop UI:

```text
Sign in to Gandalf Cloud

Code:
K7Q9-L2

Open in your browser:
gandalf.dev/activate

[Open Browser]

Waiting for approval...
```

CLI UI:

```text
$ gandalf login

Open this URL in your browser:
https://gandalf.dev/activate

Enter code:
K7Q9-L2

Waiting for browser approval...
Signed in as hippoo@example.com
```

Browser UI:

```text
Activate Gandalf on this device

Device:
MacBook Pro

Requested by:
Gandalf Desktop

Signed in as:
hippoo@example.com

[Approve Device]
```

After approval:

```text
Signed in as hippoo@example.com
Cloud remotes enabled.
Team Spaces available.
```

Reason:

```text
Works for both desktop and CLI.
Avoids fragile localhost callback and custom URL scheme flows.
Avoids password management in MVP.
Keeps account creation inside the browser.
Allows passkeys, Apple/Google sign-in, and enterprise SSO later.
Creates a clear device approval moment.
```

Underlying MVP browser sign-in method:

```text
email code
```

Out of MVP:

```text
password login
magic-link-only desktop login
local callback login
custom URL scheme login
passkey-first login
Sign in with Apple
Sign in with Google
enterprise SSO
```

### D22. Device Sessions Use Opaque Revocable Tokens

MVP device sessions should use long-lived opaque device session tokens, not JWT-only authentication.

Core policy:

```text
Device session remains valid until sign out or revoke.
Gandalf periodically rechecks the session with the server.
Server can revoke a device immediately.
Normal cloud sync should not require repeated login.
Sensitive cloud/account actions can require browser reapproval.
```

Token model:

```text
Local Mac:
- stores opaque device session token in macOS Keychain
- uses token for cloud sync, push, pull, proposal, and team access

Server:
- stores device session record
- stores token hash, not raw token
- tracks user_id
- tracks device_id
- tracks device name
- tracks created_at
- tracks last_seen_at
- tracks revoked_at
- tracks scopes/capabilities
```

JWT can still be used later as a short-lived access token, but not as the primary device session source of truth.

Recommended future extension:

```text
Opaque device session token
  -> exchanged for short-lived API access token
  -> access token may be JWT
```

MVP should avoid this unless needed for infrastructure reasons.

Device management UI:

```text
Account Settings

Devices
- MacBook Pro        Active now        Gandalf Desktop
- Mac Studio         Last seen 2d ago  Gandalf CLI

[Revoke Device]
```

Sensitive actions that may require browser reapproval:

```text
delete Team Space
remove all members
change billing owner
export team cloud history
enable future secret sync
revoke other devices
```

Why opaque token:

```text
Easy immediate revoke.
Simple device audit.
Simple last_seen tracking.
No JWT revocation complexity in MVP.
Better fit for desktop/CLI local-first sessions.
```

### D23. Device Naming Uses System Name With Rename Support

MVP should name approved devices with a hybrid model:

```text
Default name:
Use the macOS system/computer name.

Fallback name:
Use a generated name if system name is unavailable.

User control:
Allow rename in Account Settings.
```

Device naming should be display-only. Renaming a Gandalf device should not rename the actual Mac.

Device record:

```text
device_id
display_name
system_name
platform
client_type
created_at
last_seen_at
revoked_at
```

Examples:

```text
display_name: Work Mac
system_name: Hippoo's MacBook Pro
platform: macOS
client_type: Gandalf Desktop
```

Approval UI:

```text
Activate Gandalf on this device

Device:
Hippoo's MacBook Pro

App:
Gandalf Desktop

[Approve Device]
```

Account settings UI:

```text
Devices

Work Mac              Active now        Gandalf Desktop
Home Mac              Last seen 3d ago  Gandalf CLI
Hippoo's Mac Studio   Last seen 12d ago Gandalf Desktop

[Rename] [Revoke]
```

Why:

```text
System name is immediately recognizable.
Rename support handles duplicate or unclear Mac names.
Device revoke UI stays understandable.
No extra naming step during first login.
```

### D24. Remote Device Revoke Is Sign Out Only

MVP remote device revoke should be an access-control action, not a remote wipe action.

When a device is revoked:

```text
Cloud sync stops.
Push/pull/update from remote stops.
Profile proposal actions stop.
Team remote access stops.
Device session token becomes invalid.
Local profiles remain.
Local snapshots remain.
Current Codex setup remains untouched.
Local tracking profiles remain, but show cloud disconnected.
```

UI state after revoke:

```text
Cloud disconnected

This device was revoked from Gandalf Cloud.
Local profiles and snapshots are still available.
Sign in again to use cloud remotes and Team Spaces.

[Sign In Again]
```

Local tracking profile state:

```text
Profile:
qyinm-lab/frontend

Status:
Cloud disconnected

Local snapshots:
Available

Remote actions:
Disabled until sign-in
```

Important product constraint:

```text
Gandalf must not promise remote wipe.
If a Mac is offline, Gandalf cannot remove local data from it.
Remote revoke means cloud access is revoked.
```

Reason:

```text
Gandalf is local-first.
Remote revoke should not destroy local work.
Accidental revoke should be recoverable.
Offline devices make reliable wipe impossible.
Cloud access control is still enforceable at the server.
```

Future enterprise option:

```text
Managed teams may later request wipe-on-next-online for cached cloud/team metadata.
This should be explicit, opt-in, and not part of the MVP default.
```

### D25. Local Snapshots Redact Secrets And Prompt On Restore

MVP local snapshots should use strict secret redaction plus restore-time prompting.

Snapshot policy:

```text
Do not store raw secret-like values in local snapshots.
Do not store reversible encrypted secret blobs in local snapshots.
Do not store raw secret hashes in local snapshots by default.
Store secret key name, location, redacted marker, and detection confidence.
```

Snapshot example:

```text
OPENAI_API_KEY=<redacted>
FIGMA_TOKEN=<redacted>
```

Metadata example:

```text
file: ~/.codex/config.toml
field: mcp_servers.figma.env.FIGMA_TOKEN
kind: env_secret
value: <redacted>
confidence: high
```

Restore policy:

```text
Restore recreates the config structure.
Restore does not silently write raw secret values from history.
If a restored snapshot needs secret values, Gandalf prompts the user.
User can enter values for this restore.
User can restore without secrets.
Entered values are written to the target config only if the user confirms.
Entered values are not saved into the snapshot.
```

Restore UI:

```text
Restore needs secrets

This snapshot contains redacted secret fields.
Gandalf did not store their values.

Required:
- FIGMA_TOKEN
- OPENAI_API_KEY

[Enter Values] [Restore Without Secrets] [Cancel]
```

After restoring without secrets:

```text
Restored with missing secrets

2 secret values were not restored.
Codex setup may need these values before related MCP servers work.

[Add Secrets]
```

Why:

```text
Snapshot history should be safe to keep.
Local-first does not justify storing tokens forever.
Restore UX should still explain what is missing.
This preserves the future path to Keychain vault or E2EE secret sync.
```

Future options:

```text
Local Keychain vault
Opt-in encrypted local secret capture
Future end-to-end encrypted secret sync
```

### D26. Restore Redacted Secrets Uses Smart Default

When restoring a snapshot with redacted secrets, MVP should use a smart default.

Default behavior:

```text
If current config has a non-empty value at the same key and same field path:
  default to preserving the current value.

If no current value exists:
  prompt the user to enter a value or restore without it.

If the field path changed:
  prompt the user.

If parser confidence is low:
  prompt the user.

If restoring a team profile:
  show an explicit secret review step before apply.
```

Gandalf should never treat a redacted snapshot value as a secret source.

Preserve-current example:

```text
Snapshot:
~/.codex/config.toml
mcp_servers.figma.env.FIGMA_TOKEN = <redacted>

Current config:
~/.codex/config.toml
mcp_servers.figma.env.FIGMA_TOKEN = current-local-value

Restore default:
Keep current-local-value
```

Restore UI:

```text
Restore secrets

FIGMA_TOKEN
Current value found in the same location.

Default:
Keep current value

[Keep Current Value] [Enter New Value] [Restore Blank]
```

Missing value UI:

```text
Restore secrets

FIGMA_TOKEN
No current value found.

[Enter Value] [Restore Without This Secret]
```

Team profile restore UI:

```text
Secret review

This team profile references secrets that Gandalf does not store.

FIGMA_TOKEN       current value found
OPENAI_API_KEY   missing

[Review Secrets] [Restore Without Missing Secrets] [Cancel]
```

Reason:

```text
Avoids breaking working local MCP setup unnecessarily.
Keeps snapshot history secret-safe.
Avoids silently moving secrets to uncertain locations.
Makes team profile restore explicit when secrets are involved.
```

### D27. MVP Codex Scope Includes Extensions With Safe Apply

MVP product scope should use Config Plus Extensions.

Gandalf should manage supported user-global Codex setup surfaces including:

```text
Codex global config
user-global instructions
MCP configuration
tools configuration
hooks
skills or skill-like directories
prompt/template directories if Codex exposes them globally
other supported Codex extension surfaces
```

This makes Gandalf a real Codex setup branch manager, not just a `config.toml` backup tool.

However, implementation must separate storage from apply.

Recommended implementation model:

```text
~/.gandalf/
  store.git
  manifests/
  profiles/
  cache/

~/.codex/
  config.toml
  hooks/
  skills/
  prompts/
  ...
```

Flow:

```text
scan ~/.codex
normalize supported surfaces
redact secrets
commit snapshot into ~/.gandalf/store.git
move profile branches inside ~/.gandalf store
generate restore/switch preview
apply changes to ~/.codex through Gandalf safe apply engine
```

Gandalf may use Git-like storage internally for:

```text
snapshot hashes
profile branches
history
diffs
ahead/behind
pull-rebase mental model
```

Gandalf must not treat `~/.codex` itself as a raw Git working tree for profile switching.

Do not:

```text
cd ~/.codex
git checkout profile/frontend
```

Reason:

```text
Git does not know Gandalf's secret redaction policy.
Git does not know supported vs unsafe executable surfaces.
Git checkout can silently restore hooks, skills, symlinks, permissions, or unknown files.
Git does not provide Codex-aware semantic merge/conflict UI.
Git history can accidentally retain raw secrets forever.
```

Required apply protections:

```text
preview before writing
automatic snapshot before apply
secret redaction and restore prompts
symlink refusal or explicit review
executable hook/skill risk labeling
atomic writes where possible
rollback point before switch/restore
unsupported files blocked from write unless explicitly supported
clear conflict UI
```

MVP UI:

```text
Codex Setup Coverage

Managed:
- ~/.codex/config.toml
- Codex instructions
- MCP configuration
- hooks
- skills

Storage:
Gandalf snapshots are stored in ~/.gandalf

Apply:
Changes are previewed and written by Gandalf
```

### D28. Executable Surface Changes Are Warn-Only In MVP

MVP should not block executable or behavior-changing Codex surface changes by default.

Executable or behavior-changing surfaces include:

```text
hooks
tools
skills
MCP commands
permissions that allow command execution
unknown executable paths
```

Default MVP behavior:

```text
Do not show a blocking confirmation modal just because executable surfaces changed.
Do not require trust settings.
Do not require per-profile trust.
Do not require per-source trust.
Do not block high-risk executable changes by default.
Show a macOS notification.
Record a persistent timeline event.
Show risk labels in dashboard and diff.
Keep one-click restore visible.
```

macOS notification:

```text
Codex setup changed

High risk: new hook can run shell commands

[View Diff]
```

Dashboard/timeline record:

```text
Today 14:22
Codex hook changed

Risk:
High

Snapshot:
8f3a2c7

[View Diff] [Restore Previous]
```

Switch/restore/profile apply behavior:

```text
Preview still appears before writing.
Executable changes are labeled in the preview.
The primary action remains Apply / Restore / Switch.
No extra confirmation modal appears only because the change is executable.
```

External input behavior:

```text
Imported bundles and team profile applies still require the normal preview/apply flow.
Risk labels are shown in preview.
Warn-only means no additional trust dialog beyond the preview.
```

Reason:

```text
Automatic snapshots already preserve rollback points.
Blocking modals make profile switching feel heavy.
The product value is awareness plus undo, not permission prompts.
macOS notifications match the desktop safety-layer experience.
Timeline keeps the warning durable after the notification disappears.
```

### D29. Notifications Are Risk-Based And User Configurable

MVP macOS notifications should default to risk-based triggers.

Default notification policy:

```text
Low risk:
Record in dashboard and timeline only.

Medium risk:
Show macOS notification.
Record in dashboard and timeline.

High risk:
Show macOS notification immediately.
Record in dashboard and timeline.
Keep restore action prominent.
```

User setting:

```text
Notification level:
[All Changes] [Medium + High] [High Only] [Digest] [Off]

Default:
Medium + High
```

Notification examples:

```text
Medium risk:
Codex setup changed
MCP server added: figma

High risk:
Codex setup changed
New hook can run shell commands
```

Digest behavior:

```text
Codex setup summary

Today:
- 4 low-risk changes
- 1 medium-risk change

[View Timeline]
```

Dashboard behavior:

```text
All detected changes are recorded in timeline even when notification is off.
Notification settings do not disable automatic snapshots.
Notification settings do not disable risk labels.
Notification settings do not disable restore.
```

Reason:

```text
All-change notifications are too noisy.
High-risk-only notifications may hide meaningful setup drift.
Medium/high default gives protection without constant interruption.
User control is necessary because teams and individuals have different tolerance.
```

### D30. Background Watching Starts Through Menu Bar Protection

MVP should start background watching through an onboarding opt-in to Gandalf Protection.

User-facing model:

```text
Gandalf Protection
= menu bar app watches Codex setup changes
= automatic snapshots
= macOS notifications
= timeline/risk records
```

It should not be presented as:

```text
daemon
background service
agent process
```

Onboarding UI:

```text
Keep Gandalf Protection running?

Gandalf can watch your Codex setup, create snapshots when it changes,
and notify you about risky changes.

[Enable Protection] [Not Now]

[x] Launch Gandalf at login
```

Default behavior:

```text
User must explicitly choose Enable Protection during onboarding.
If enabled, the menu bar app runs the watcher.
If Launch at Login is enabled, the menu bar app starts after login.
If not enabled, watcher runs only when Gandalf is open.
```

Settings UI:

```text
Protection

Status:
Watching Codex setup

Launch at Login:
On

Notifications:
Medium + High

[Pause Protection] [Open Timeline]
```

Menu bar UI:

```text
Gandalf: Protected
Active Profile: Default
Last snapshot: 12 min ago
Watching: Codex setup

[Open Dashboard]
[Create Snapshot]
[Pause Protection]
```

Important product rule:

```text
Daemon can exist as an implementation detail if needed.
Daemon must not be the product concept or main user-facing feature.
```

Reason:

```text
Gandalf needs always-on protection to deliver desktop value.
Opt-in onboarding avoids silently installing background behavior.
Menu bar presence makes the watcher visible and controllable.
Launch at Login gives always-on protection without daemon-first UX.
```

### D31. First-Run Onboarding Is Local-First

MVP first-run onboarding should prove local value before asking for cloud, team, or background behavior.

Default first-run order:

```text
1. Welcome
2. Scan current user-global Codex setup
3. Create Default profile from current setup
4. Create first snapshot in Default
5. Show dashboard with active profile, snapshot, risk, and covered surfaces
6. Ask whether to enable Gandalf Protection
7. Offer optional Gandalf Cloud sign-in
8. Offer optional Team Space join if user has an invite
```

First-run must not require cloud sign-in.

First-run must not require enabling Protection.

First-run must not modify current Codex setup. It should only read Codex setup and write Gandalf's local profile store.

Onboarding UI:

```text
Gandalf found your Codex setup

Default profile created.
First snapshot created.

Detected:
- 3 MCP servers
- 2 hooks
- 12 skills
- 1 high-risk item

[Open Dashboard]
```

Dashboard after first run:

```text
Active Profile:
Default

Last Snapshot:
just now

Protection:
Off

Cloud:
Not signed in

Primary Actions:
[Enable Protection] [Create Snapshot] [Switch Profile]
```

Cloud sign-in should be offered after local value is clear:

```text
Sync profiles across devices?

Sign in to Gandalf Cloud to push/pull personal profile snapshots
across your own devices and use team profile remotes.

[Sign In] [Not Now]
```

Reason:

```text
The user should see Gandalf understand their existing Codex setup first.
Local-first builds trust before asking for account, team, or launch-at-login permissions.
Default profile and first snapshot teach the profile/snapshot model immediately.
Cloud and team become upgrades to an already useful local product.
```

### D32. Team Invite First-Run Is Invite-Aware Local-First

If a user opens Gandalf from a Team Space invite before they have a local Default profile, Gandalf should keep the invite context but still protect the local setup first.

Invite-aware first-run order:

```text
1. Show invite context
2. Scan current user-global Codex setup
3. Create Default profile from current setup
4. Create first snapshot in Default
5. Ask user to sign in with browser device code
6. Join Team Space
7. Show available team profiles
8. Switch to team profile only through preview/apply
```

Invite context UI:

```text
You were invited to qyinm-lab

Before joining, Gandalf will save your current Codex setup
as a local Default profile.

[Continue]
```

After local snapshot:

```text
Local setup saved

Default profile created.
First snapshot: 8f3a2c7

Now you can join qyinm-lab safely.

[Sign In & Join Team]
```

Reason:

```text
Team onboarding should not overwrite or confuse the user's existing Codex setup.
Invite context should not be lost.
Local rollback point must exist before applying any team profile.
This keeps team onboarding fast while preserving Gandalf's safety promise.
```

### D33. Local-First Still Allows Cloud Sign-In

Local-first means Gandalf is useful without an account. It does not mean local users cannot sign in.

Users should be able to sign in from:

```text
first-run onboarding after local setup is saved
dashboard cloud status
settings
team invite flow
CLI via gandalf login
```

Personal cloud profile behavior for subscribed users:

```text
Personal profiles can track personal cloud remotes.
Personal snapshots can be pushed to the user's cloud remote.
Personal profiles show ahead/behind status against their cloud remote.
Personal cloud profiles can sync across the user's devices.
Personal cloud profiles are private to the user's account by default.
```

Example:

```text
Profile:
Default

Remote:
cloud/hippoo/default

Status:
ahead by 2 snapshots

Actions:
[Push] [Update from Remote]
```

Sharing with other people should use Team Spaces, not personal cloud remotes:

```text
Team profile:
owned by Team Space
reviewed through Profile Proposal
published for team members

Personal cloud profile:
owned by one user
synced across that user's own devices
not shared with other people in MVP
```

Important:

```text
Cloud sign-in is optional for local use.
Cloud sign-in unlocks personal sync, personal cloud backup, and team remotes.
Free local functionality should not require sign-in.
```

### D34. Cloud Sign-In Auto-Attaches Personal Profiles

After a user signs in to Gandalf Cloud, Gandalf should automatically attach local personal profiles to personal cloud remotes.

This applies to:

```text
Default
all local personal profiles
```

This does not apply to:

```text
team tracking profiles
imported profiles that have not been saved as personal profiles
debug/internal profiles
```

Remote naming:

```text
Local profile:
Default

Cloud remote:
cloud/hippoo/default

Local profile:
Frontend

Cloud remote:
cloud/hippoo/frontend
```

After sign-in UI:

```text
Gandalf Cloud connected

Personal profiles attached:
- Default          cloud/hippoo/default
- Frontend         cloud/hippoo/frontend
- Experimental     cloud/hippoo/experimental

Status:
Syncing 3 profiles...

Icon:
refresh-ccw spinning

[View Sync Activity]
```

Important distinction:

```text
Attach remote:
Create or connect the local profile to a cloud remote.

Push:
Upload local snapshots to that cloud remote.
```

Cloud sign-in should automatically attach remotes and run the initial personal sync bootstrap when there is no conflict.

Conflict behavior:

```text
If cloud remote already exists:
Fetch remote state.
Show ahead/behind/diverged status.
Do not overwrite remote blindly.
Use Update from Remote when needed.
```

Privacy behavior:

```text
Auto-attach does not share profiles with other people.
Auto-attach does not bypass secret redaction.
Auto-attach does not upload raw secrets.
Team sharing still requires Team Space.
```

Reason:

```text
A user who signs in expects personal cloud sync to become available.
Manual remote attachment is unnecessary product friction.
Git-like ahead/behind can still explain sync state after attachment.
Visible sync status is enough for the normal no-conflict path.
```

### D35. First Personal Cloud Sync Bootstraps Automatically

After sign-in auto-attaches personal profiles to cloud remotes, Gandalf should run an initial sync bootstrap automatically when the remote state is empty or non-conflicting.

MVP should not block the initial sign-in bootstrap with a summary confirmation screen.

This does not mean continuous auto-push.

Default behavior:

```text
Sign in succeeds.
Personal profiles attach to cloud remotes.
Gandalf runs initial cloud sync bootstrap automatically.
UI shows a spinning refresh-ccw sync indicator.
Timeline records sync events.
User can open sync activity details if they want.
```

Primary UI:

```text
Cloud
Syncing...

Icon:
refresh-ccw spinning
```

Detailed UI:

```text
Sync Activity

Default          uploading 12 snapshots
Frontend         uploading 4 snapshots
Experimental     waiting

Secrets:
Raw secret values are not uploaded.
```

Complete state:

```text
Cloud
Synced just now

Default          up to date
Frontend         up to date
Experimental     up to date
```

Exception behavior:

```text
If remote is empty:
Upload existing local snapshots during initial bootstrap.

If remote is up to date:
Show synced.

If remote is behind local only:
Push existing local snapshots during initial bootstrap.

If remote is ahead or diverged:
Stop and show Needs Review.
Do not overwrite blindly.
Use Update from Remote / pull-rebase flow.
```

After initial bootstrap:

```text
New local snapshots stay local.
Profile shows ahead by N snapshots.
User clicks Push to upload.
Gandalf does not continuously auto-push future snapshots.
```

Post-bootstrap UI:

```text
Profile:
Default

Status:
ahead by 2 snapshots

Action:
[Push]
```

Needs review UI:

```text
Cloud sync needs review

Default is ahead 2 and behind 1.
Gandalf needs to update from remote before pushing.

[Review] [Later]
```

Reason:

```text
Sign-in is enough intent to enable personal cloud sync.
A blocking summary makes sync feel unnecessarily heavy.
The user still sees sync state through the refresh-ccw indicator.
Risk is controlled by secret redaction and conflict review.
After bootstrap, manual Push preserves the Git-like profile workflow.
```

### D36. Existing Personal Remotes Use Auto Pull-Rebase

When a signed-in user already has a personal cloud remote with the same profile name, Gandalf should auto pull-rebase if it can do so cleanly.

Example:

```text
Current Mac:
Default

Cloud:
cloud/hippoo/default
```

Default behavior:

```text
Fetch cloud remote snapshots.
Compare local and remote history.
If cloud has snapshots local does not have, replay local snapshots on top.
If replay succeeds cleanly, sync automatically.
If replay conflicts, stop and show Needs Review.
Never overwrite local setup blindly.
```

Syncing UI:

```text
Syncing Default

Remote found:
cloud/hippoo/default

Remote:
8 snapshots

Local:
2 new snapshots

Status:
Updating...
```

Success UI:

```text
Default synced

Status:
up to date
```

Conflict UI:

```text
Sync needs review

Default has local and cloud changes that touch the same Codex setup fields.

[Review Conflict]
[Keep Local as New Profile]
[Use Cloud Version]
[Cancel]
```

Safety rules:

```text
Create a local snapshot before applying remote state.
Stop on semantic conflicts.
Stop on file-level conflicts.
Stop when parser confidence is low.
Offer Keep Local as New Profile.
Do not prefer cloud automatically.
Do not create duplicate profiles unless needed.
```

Reason:

```text
Users expect second-machine sign-in to sync automatically.
Auto pull-rebase matches Gandalf's Git-like profile model.
Local setup is still protected by snapshots and conflict stop.
Prefer-cloud would be too destructive.
Ask-every-time would make sync feel fragile.
```

### D37. MVP Conflict Resolution Uses Simple Whole-Profile Choices

MVP conflict resolution should not expose file-level or semantic field-level merge UI.

When sync, switch, restore, or pull-rebase hits a conflict, Gandalf should show the conflict summary and offer simple whole-profile choices.

Conflict UI:

```text
Sync needs review

Default has local and cloud changes that touch the same Codex setup fields.

Local:
2 snapshots only on this Mac

Cloud:
8 snapshots from MacBook Pro

Risk:
1 high-risk executable change

Actions:
[Use Local]
[Use Cloud]
[Keep Local as New Profile]
[Cancel]
```

Action behavior:

```text
Use Local:
Keep the local profile state and do not apply remote changes.

Use Cloud:
Create a rollback snapshot locally, then apply the cloud profile state.

Keep Local as New Profile:
Create a new personal profile from the local state, then allow the original profile to update from cloud.

Cancel:
Leave everything unchanged.
```

Conflict review can show:

```text
conflicting files
conflicting Codex surfaces
snapshot ids
risk labels
device/source names
timestamps
```

But MVP should not allow:

```text
choose local/remote per file
choose local/remote per field
manual merge editor
partial restore as conflict resolution
```

Reason:

```text
Whole-profile choices match the branch/profile mental model.
File-level and field-level merges require stronger parser confidence.
Simple choices reduce accidental partial environment corruption.
Keep Local as New Profile preserves user work without complex merge UI.
```

Future:

```text
file-level resolution
semantic field-level resolution
partial restore
manual TOML/editor-assisted merge
```

### D38. Auto-Created Profile Names Are Reason-Based And Editable

When Gandalf creates a new profile automatically, it should use a readable reason-based name and allow the user to edit it before creation when the flow is interactive.

Default naming pattern:

```text
<Original Profile> local copy
```

Conflict naming examples:

```text
Default local copy
Frontend local copy
Backend local copy
```

If the name already exists, add a small numeric suffix:

```text
Default local copy 2
Default local copy 3
```

Interactive UI:

```text
Keep Local as New Profile

Name:
[Default local copy]

[Create Profile]
```

Non-interactive fallback:

```text
Default local copy 2
```

Metadata should still record the reason and source:

```text
created_reason: conflict_keep_local
source_profile: Default
source_device: MacBook Pro
source_snapshot: 8f3a2c7
created_at: 2026-06-10T14:22:00Z
```

Do not use these as the visible primary name by default:

```text
timestamp-only names
short hash names
device-only names
```

Reason:

```text
Users need to understand profile lists later.
Reason-based names explain why the profile exists.
Numeric suffixes are simpler than timestamp noise.
Editable names avoid forcing Gandalf's generated name on the user.
Detailed metadata can still preserve precise provenance.
```

### D39. Profile Rename Changes Display Name Only

MVP profile rename should change the profile display name only.

It should not automatically rename:

```text
profile_id
cloud remote slug
team remote slug
snapshot history
```

Profile identity model:

```text
profile_id:
prf_abc123

display_name:
Web App Setup

remote_slug:
frontend

cloud_remote:
cloud/hippoo/frontend
```

Rename example:

```text
Before:
Display name: Frontend
Remote: cloud/hippoo/frontend

After:
Display name: Web App Setup
Remote: cloud/hippoo/frontend
```

Rename UI:

```text
Rename Profile

Name:
[Web App Setup]

Cloud remote will stay:
cloud/hippoo/frontend

[Rename]
```

Remote rename should be a separate explicit action, not part of basic profile rename.

Future action:

```text
Rename Cloud Remote
```

Reason:

```text
Display rename should be safe and reversible.
Cloud remote identity should not move unexpectedly.
Other devices should keep tracking the same remote.
Stable profile ids keep history and sync reliable.
Remote rename has Git-like consequences and deserves explicit handling.
```

### D40. Profile Delete Is Local-Only In MVP

MVP profile delete should delete the local profile first and preserve cloud history by default.

Default behavior:

```text
Delete local profile.
Detach from cloud remote on this device.
Keep cloud remote.
Keep cloud snapshot history.
Do not delete cloud data by default.
Do not break other devices tracking the same remote.
```

Delete UI:

```text
Delete Profile

Profile:
Frontend

Cloud remote:
cloud/hippoo/frontend

Default:
Delete local profile only.
Cloud history will be kept.

[Delete Local Profile]
[Cancel]
```

Do not offer cloud archive in MVP.

Do not offer destructive cloud history deletion as a primary MVP action.

Future advanced action:

```text
Permanently Delete Cloud Remote
```

This should require stronger confirmation and may be account/billing/retention-policy dependent.

Reason:

```text
Profile delete should not unexpectedly destroy cloud history.
Other devices may still depend on the cloud remote.
Accidental local cleanup should be recoverable.
Manual Push means there is no continuous auto-sync list to clean up.
Cloud history deletion has stronger data-loss and compliance implications.
Archive adds product state without enough MVP value.
```

### D41. Cloud-Only Personal Profiles Are Managed In Web Dashboard

If a local profile is deleted but its personal cloud remote remains, recovery and reattach should be managed primarily from the Gandalf web dashboard.

Web dashboard behavior:

```text
Gandalf Web Dashboard

Personal Cloud Profiles
- Default
- Frontend
- Backend

Frontend
Remote: cloud/hippoo/frontend
Last pushed: 3 days ago
Snapshots: 18

[Attach to This Mac]
```

Desktop behavior:

```text
Cloud
Signed in as hippoo@example.com

[Open Web Dashboard]
```

Attach flow:

```text
User clicks [Attach to This Mac] in web dashboard.
Browser device approval confirms the Mac.
Gandalf desktop receives attach request.
Gandalf creates local personal profile tracking the cloud remote.
Gandalf fetches remote snapshots.
Gandalf shows preview before applying the profile to current Codex setup.
```

CLI can still expose an explicit attach command for power users:

```bash
gandalf profile attach cloud/hippoo/frontend
```

Do not put cloud-only deleted profiles in the primary desktop profile picker by default.

Reason:

```text
Cloud-only profile management is account/cloud state, not daily local switching.
The desktop profile picker should stay focused on local active profiles.
The web dashboard is a better place to browse cloud history and recover deleted local profiles.
Attach still requires local preview before writing Codex setup.
```

### D42. Web Dashboard MVP Covers Account And Cloud Profiles

MVP web dashboard should cover cloud/account management, not local Codex setup application.

In scope:

```text
account
subscription and billing
approved devices
personal cloud profiles
personal cloud profile history
Attach to This Mac
basic Team Space entry
team members and invites
```

Out of scope for web MVP:

```text
local profile switching
local restore/apply
editing ~/.codex
full desktop dashboard replacement
complex team proposal review UI
full audit dashboard
field-level diff/merge
```

Web dashboard UI:

```text
Gandalf Cloud

Account
Billing
Devices
Personal Cloud Profiles
Team Spaces
```

Personal cloud profile UI:

```text
Personal Cloud Profiles

Default
cloud/hippoo/default
Last pushed: 8 min ago
Snapshots: 42

[View History] [Attach to This Mac]
```

Device UI:

```text
Devices

Work Mac        Active now
Home Mac        Last seen 3d ago

[Revoke]
```

Role split:

```text
Desktop:
local profile switching, restore, apply, timeline, protection, local risk review

Web:
account, billing, devices, personal cloud history, cloud-only profile attach, team membership
```

Reason:

```text
Web is the natural place for account and cloud state.
Desktop is the natural place for local file writes and Codex setup apply.
Keeping web smaller avoids duplicating the desktop dashboard.
Attach to This Mac bridges cloud state back into local preview/apply.
```

### D43. Team Proposal Review Is Desktop-Only In MVP

MVP should not split team Profile Proposal review between web and desktop.

Team proposal review, approval, and publish should happen in the desktop app.

Desktop owns:

```text
team proposal list
proposal diff
risk labels
snapshot ids
review status
publish action
local switch/apply preview
restore/rollback context
```

Web owns:

```text
team members
team invites
billing
devices
personal cloud profiles
account settings
```

Desktop proposal list UI:

```text
Team Proposals

qyinm-lab/frontend

#42 Frontend setup update
Proposed by: alice
Snapshot: 8f3a2c7
Risk: High

[Review]
```

Desktop proposal review UI:

```text
Profile Proposal #42

Profile:
qyinm-lab/frontend

Proposed snapshot:
8f3a2c7

Changes:
+ MCP server: figma
+ hook: postToolUse
~ Codex permissions

Risk:
High - new hook can run shell commands

Actions:
[View Diff]
[Preview Switch]
[Publish]
[Close]
```

Reason:

```text
Team proposal review is about Codex setup diff and risk.
The desktop app already owns local preview, switch, restore, and risk review.
Web proposal UI would duplicate desktop product surface too early.
Keeping proposal review in desktop makes the MVP simpler and safer.
```

### D44. Team Proposal Publish Is Lightweight And Non-Blocking

MVP team Profile Proposal flow should be a lightweight publish review, not a full code-review product.

Default behavior:

```text
Any member can create a Profile Proposal.
Any member can review the diff.
Any member can leave top-level comments.
Any member can publish by default.
Review is encouraged but not mandatory.
Dry-run is available but not mandatory.
Local apply is available but not mandatory.
High-risk labels warn but do not block publish.
```

MVP flow:

```text
Local snapshot
Push snapshots
Create Profile Proposal
Review semantic diff + risk label
Optional top-level comments
Publish
```

Publish is a team remote update, not a local file write.

```text
Publish:
Move qyinm-lab/frontend to snapshot 8f3a2c7.

Not publish:
Do not apply that snapshot to every member's Mac.
Do not write ~/.codex on reviewer machines.
Do not bypass each user's switch/apply preview later.
```

Desktop proposal UI:

```text
Profile Proposal #42

Diff:
+ MCP server: figma
+ hook: postToolUse
~ Codex permissions

Risk:
High - new hook can run shell commands

Actions:
[View Diff]
[Preview Switch]
[Publish]
[Comment]
```

Conversation UI:

```text
Conversation

alice:
Added Figma MCP for design review workflow.

minsu:
Can we confirm this hook only runs after Codex tool use?

[Write a comment...]
[Comment]
```

Publish warning for high-risk proposals:

```text
High-risk proposal

This proposal adds executable Codex behavior.
Publishing updates the team profile remote.
Members will still review/apply locally before switching.

[Publish Anyway] [Cancel]
```

Reason:

```text
The team remote is a branch-like distribution point.
Proposal objects preserve team intent and audit history.
Full PR-style review UI is too much surface area for the MVP.
Forcing dry-run or local apply makes team publish too heavy.
Actual local safety happens when each user switches/applies the profile.
Diff and risk labels should inform, not gate, MVP publishing.
```

Future team policy:

```text
inline comments
request changes
resolved/unresolved threads
required reviews
required dry-run
required publisher role
high-risk publish restrictions
security reviewer requirement
```

### D45. Team Proposal Comments Are Top-Level Only In MVP

MVP team Profile Proposals should support top-level Markdown comments only.

Comment surfaces:

```text
top-level proposal conversation
```

Comment behavior:

```text
Any member can comment.
Comments are visible to all team members.
Comments do not block publish by default.
Publishing remains allowed by default.
```

Proposal conversation UI:

```text
Profile Proposal #42

Conversation

alice
Added Figma MCP and updated frontend review skill.

minsu
Please explain why the new hook needs shell access.

[Write a comment...]
[Comment]
```

Review actions:

```text
[Comment]
[Publish]
```

Reason:

```text
Teams need discussion more than strict gates in the MVP.
Top-level comments capture the main decision context.
Inline comments, threads, and request changes make proposals feel like a full code review product.
Comments preserve context for why a profile changed.
```

Future:

```text
inline comments on semantic diff entries
inline comments on raw file diff lines
request changes
resolved/unresolved comment threads
required review policy
```

### D46. Team Proposal Diff Defaults To Semantic

MVP team Profile Proposal review should show semantic diff and risk first.

Default view:

```text
Semantic Diff

+ MCP server: figma
+ hook: postToolUse
~ Codex permissions
- MCP server: old-docs

[View Snapshot]
```

Snapshot detail can expose lower-level file context later, but raw line-by-line review should not be a core proposal UI in MVP.

Future raw diff view:

```text
Raw File Diff

~ ~/.codex/config.toml
+ [mcp_servers.figma]
+ command = "..."

~ ~/.codex/hooks/post-tool-use.sh
+ ...

[View Semantic Diff]
```

Proposal list should stay compact:

```text
#42 Frontend setup update
+ Figma MCP, + hook, ~ permissions
Risk: High
```

Reason:

```text
Semantic diff explains product meaning faster than raw file changes.
The MVP proposal should answer what changed, why it matters, and whether it is risky.
Raw file diff is useful later, but it pulls Gandalf toward code-review complexity too early.
```

### D47. Inline Review Features Are Later

MVP team Profile Proposals should not include inline review features.

Do not ship in MVP:

```text
inline comments on semantic diff entries
inline comments on raw file diff lines
resolved/unresolved threads
request changes
review states
required approvals
```

Later PR-lite model:

```text
Lightweight proposal
+ inline comments
+ request changes
+ resolved threads
+ optional required review policy
```

Reason:

```text
The MVP should prove that teams need proposal-based profile publishing.
Inline review is a second product surface.
If teams start using comments heavily, Gandalf can graduate to PR-lite later.
```

### D48. Published Proposals Keep Lightweight Audit History

Published team Profile Proposals should remain readable after publish, but the retained record should match the lightweight MVP scope.

Published proposal page should preserve:

```text
proposal title
proposal description
semantic diff
risk label
top-level comments
published snapshot id
publisher
published_at
target team profile
```

Published proposal UI:

```text
Profile Proposal #42
Published

Profile:
qyinm-lab/frontend

Published snapshot:
8f3a2c7

Published by:
minsu

Diff:
+ MCP server: figma
+ hook: postToolUse
~ Codex permissions

[View Comments]
[View Snapshot]
```

Team profile timeline should link to the proposal:

```text
Today 16:40
qyinm-lab/frontend published snapshot 8f3a2c7
via Proposal #42

[Open Proposal]
```

Reason:

```text
Team setup decisions should remain auditable.
Comments explain why a profile changed.
Published snapshot id connects team history to exact setup state.
Keeping published proposals readable is useful without cloning a full code-review history model.
```

### D49. Closed Unpublished Proposals Are Minimal In MVP

Closed unpublished team Profile Proposals should be preserved as lightweight records, but MVP should not build a full closed-proposal review surface.

Default behavior:

```text
Move closed proposals out of the default open list.
Preserve proposal title and description.
Preserve semantic diff and risk label.
Preserve top-level comments.
Preserve close reason.
Preserve closed_by and closed_at.
Clearly show that no team profile was published.
```

Closed proposal minimal UI:

```text
Profile Proposal #41
Closed

Status:
Not published

Closed by:
alice

Reason:
Replaced by Proposal #42

[View Comments]
```

Reason:

```text
Abandoned setup discussions can explain future decisions.
Closed proposals should not become a major navigation surface in MVP.
Deleting proposal history would weaken team auditability.
Closed tabs, rich filters, and full closed review pages can come later if teams need them.
```

### D50. Team Proposals Do Not Have Drafts In MVP

MVP team Profile Proposals should not support draft state.

Creating a proposal immediately opens it for team review.

Flow:

```text
Local snapshots
Push snapshots
Create Profile Proposal
Proposal opens immediately
Team can review/comment/publish
```

UI:

```text
Create Profile Proposal

Profile:
qyinm-lab/frontend

Snapshot:
8f3a2c7

Title:
[Frontend setup update]

Description:
[Optional]

[Create Proposal]
```

After creation:

```text
Profile Proposal #42
Open
```

Reason:

```text
Proposal creation should stay lightweight.
Preparing changes already happens in local snapshots.
Draft state adds extra lifecycle surface without enough MVP value.
Draft/ready workflow can come later if teams need it.
```

Future:

```text
draft proposals
mark ready for review
private proposal drafts
team-visible drafts
```

### D51. Proposal Titles Are Auto-Generated And Editable

MVP team Profile Proposal creation should generate a title from the semantic diff and let the user edit it before creation.

Description should be optional.

Title generation examples:

```text
Changes:
+ MCP server: figma
~ Codex permissions

Generated title:
Add Figma MCP and update Codex permissions
```

```text
Changes:
+ hook: postToolUse
- MCP server: old-docs

Generated title:
Add postToolUse hook and remove old-docs MCP
```

Create proposal UI:

```text
Create Profile Proposal

Title:
[Add Figma MCP and update Codex permissions]

Description:
[Optional]

Snapshot:
8f3a2c7

[Create Proposal]
```

Fallback title:

```text
Update Frontend profile
```

Reason:

```text
Proposal creation should stay lightweight.
Readable titles make proposal history useful.
Users should be able to correct Gandalf's generated wording.
Description can stay optional because diff/comments carry most context.
```

### D52. Proposal Text Supports Markdown

MVP proposal descriptions and comments should support Markdown.

Supported:

```text
paragraphs
lists
inline code
code blocks
links
quotes
```

Do not promise full GitHub-flavored Markdown in MVP:

```text
task lists
tables
mentions
issue references
advanced extensions
```

Reason:

```text
Markdown is enough for technical discussion.
Full GitHub-flavored Markdown expands compatibility promises unnecessarily.
Rich text editing is too much UI for MVP.
```

## Positioning

### Primary Positioning

```text
Gandalf is the branch manager for your Codex setup.
```

### Secondary Positioning

```text
Create snapshots of different Codex setups.
Switch between them locally.
Compare what changed.
Rollback when an agent breaks your setup.
Sync approved profiles with your team.
```

### Safety Positioning

```text
Gandalf watches your AI agent setup and gives you an undo button.
```

### What Gandalf Is

- A local-first agent setup manager.
- A profile switcher for Codex setup in the MVP.
- A safety layer for agent-driven setup changes.
- A Time Machine / Git-like history model for AI agent configuration.
- A future desktop dashboard for profile state, drift, risk, timeline, and restore.
- For MVP, a user-global setup manager, not a repo-local config manager.

### What Gandalf Is Not

- Not primarily a marketplace.
- Not primarily a security dashboard.
- Not primarily a generic dotfile manager.
- Not primarily a TUI.
- Not primarily a graph/provenance explorer.
- Not an agent runtime or orchestration layer.
- Not a tool that executes MCP servers, hooks, skills, or agent commands during scan.

## Core Product Model

Gandalf should be modeled around these objects:

```text
Space
Global Profile
Active Profile
Remote Team Profile
Local Tracking Profile
Cloud Remote
Working Changes
Snapshot
Timeline
Switch
Create Snapshot
Fork
Profile Proposal
Review
Publish
Bundle
Guard
```

### Space

A Space is the ownership boundary for profiles.

```text
Personal Space
Team Space
```

Personal Space contains profiles owned by one developer.

Team Space contains profiles published by a team or organization owner.

Examples:

```text
Personal Space
- Default
- Minimal Safe
- Codex Power
- Frontend
- Backend
- Experimental

Team Space: qyinm-lab
- Frontend Global Setup
- Backend Global Setup
- Security Review Setup
- Minimal Team Safe Setup
```

### Global Profile

A Global Profile is a selectable user-global agent setup state.

This is the central product object.

```text
Global Profile = branch-like user-global AI agent setup state
```

A profile contains the desired/current state of supported agent setup surfaces:

- MCP servers
- skills
- hooks
- permissions
- user-global instructions/settings
- supported agent configs
- safe env key inventory

A profile is not just a label, folder, or export artifact. Switching profile should be able to change real local configuration files, subject to preview, safety policy, and rollback.

### Active Profile

The Active Profile is the profile currently applied to this Mac's user-global agent setup.

Example:

```text
Active Profile: Default
Working Changes: 3
Risk: 1 high
Last Snapshot: 12 min ago
```

The user should always start inside a profile. On first run:

```text
Gandalf found your current setup.
Default profile created from your current setup.
```

Default is not "no profile." Default is the user's first personal branch.

### Remote Team Profile

A Remote Team Profile is a team-owned profile that behaves like a remote Git branch.

Users do not mutate it directly. They switch to it locally, build snapshots on top of it, and propose changes back when useful.

Example:

```text
Remote Team Profile:
qyinm-lab/frontend
```

### Local Tracking Profile

A Local Tracking Profile is the user's local profile that tracks a Remote Team Profile.

Example:

```text
Local Tracking Profile:
alice/frontend

Tracks:
qyinm-lab/frontend

Status:
Ahead by 3 snapshots
```

This is the normal state after using a team profile locally.

### Cloud Remote

A Cloud Remote is a hosted profile branch that a local profile can track.

Cloud remotes can be personal or team-owned.

Examples:

```text
cloud/hippoo/default
qyinm-lab/frontend
```

When a local profile has a cloud remote, Gandalf should show sync status:

```text
up to date
ahead by 3 snapshots
behind by 2 snapshots
ahead 3, behind 2
```

When the profile is behind or diverged, the primary action should be:

```text
Update from Remote
```

Internally this behaves like pull-rebase: fetch remote snapshots, then replay local snapshots on top.

### Working Changes

Working Changes are local setup changes detected before they are recorded as a snapshot in the active profile.

Examples:

```text
Changes since last snapshot:
+ MCP server: figma
+ Skill: react-review
~ Codex permissions changed
+ Hook: postToolUse
```

Working Changes are the explanatory state between "files changed" and "snapshot recorded."

In the ideal desktop/guard experience, Gandalf should automatically create snapshots when it detects environment changes. Automatic snapshotting records what happened; it does not mean the change is trusted, pushed, or published.

When the user has working changes, Gandalf should offer:

```text
[Create Snapshot]
[Save as New Profile]
[Discard Changes]
[Switch Profile]
```

### Snapshot

A Snapshot is a saved setup commit in the active profile.

Snapshot and Create Snapshot are the same core action.

The user can create snapshots manually:

```text
Create Snapshot
```

Gandalf can also create snapshots automatically before risky operations:

- before switching profiles
- before restoring
- before applying a team profile
- before importing a bundle
- when recording a high-risk guard event

Example:

```text
Before switching from Default to Frontend,
Gandalf saved a snapshot in Default.
```

Snapshots are the restore points in profile history.

### Timeline

Timeline is the chronological history of setup changes, saves, switches, restores, and reviewed events.

Timeline is useful because users forget when and why setup changed.

Example timeline:

```text
Today 14:22  Codex added MCP server: figma
Today 14:18  Created snapshot in Default
Today 13:02  Switched from Minimal Safe to Frontend
Yesterday    Restored to snapshot before hook change
```

Timeline should be written in user language, not internal engine language.

Avoid first-class labels like:

```text
graph node changed
provenance changed
audit finding
raw source diff
```

Prefer:

```text
MCP server added
Skill installed
Hook changed
Permission widened
Global agent settings changed
```

### Switch

Switch applies another profile to the user's global local setup, like Git checkout.

Included surfaces can include:

```text
~/.codex/config.toml
supported Codex global config
```

Example:

```text
Switching to Profile:
Frontend Global Setup

Will change:
+ Figma MCP
+ GitHub MCP
~ ~/.codex/config.toml
- Unknown local MCP: old-docs

[Review & Switch]
```

Switch must always have a dry-run/preview state before writing files.

Switch must create a snapshot in the current profile before applying changes.

Switch must detect pending working changes that have not yet been snapshotted and require an explicit decision.

Switch must not apply unsupported or low-confidence executable state silently.

Switch must not restore raw secret values.

### Create Snapshot

Create Snapshot records current working changes into the active profile.

In Git terms, Create Snapshot is commit-to-branch.

Create Snapshot, Save Profile, and Snapshot are the same core action: all create an immutable setup commit in the active profile history.

Snapshot creation can be user-triggered or automatic.

```text
Manual snapshot
= user clicks Create Snapshot

Automatic snapshot
= Gandalf detects agent setup changed and records a snapshot
```

Automatic snapshot does not push to a team, approve the change, or mark it trusted. It only records local history.

Example:

```text
Create Snapshot in Default

Changes:
+ MCP server: figma
+ Skill: react-review
~ Codex permissions changed

[Create Snapshot]
```

Primary product language should be:

```text
Create Snapshot
Restore Snapshot
Push Snapshots
```

### Fork

Fork creates a separate personal profile from an existing profile.

Fork is not required for normal team profile usage. Normal team usage creates a local tracking profile, accumulates local snapshots, and proposes changes back to the team.

Fork is for intentional long-lived personal divergence.

Example:

```text
Team Profile:
qyinm-lab/frontend @ 72ab91d

You added:
+ Personal MCP: browser-control

Actions:
[Keep on Local Tracking Profile]
[Fork as Separate Personal Profile]
[Revert to Team Profile]
```

Fork lets users preserve local experimentation when they do not intend to push it back to the team.

### Profile Proposal

A Profile Proposal is a lightweight request to update a team profile.

This is how team members share profile changes without directly mutating the approved team setup.

Example:

```text
Propose Profile Update

From:
alice/frontend

To:
qyinm-lab/frontend

Changes:
+ Figma MCP
~ Updated frontend review skill
- Removed old docs MCP

Status:
Ahead by 3 snapshots

Risk:
1 medium

[Push & Create Proposal]
```

Reviewers should see:

```text
Semantic diff
Risk explanation
Required env keys
Affected global files
Automatic snapshot history
```

### Review

Review is the lightweight inspection and comment surface before a proposed profile snapshot becomes the new published team snapshot.

By default, review is open to all members and is not a mandatory gate. Approve is the final confirmation made by the member who publishes the snapshot.

Review should answer:

```text
What changed?
Why was it changed?
Is it risky?
Did any high-risk snapshot occur?
Who approved it?
Which snapshot will be published?
```

Review actions:

```text
[Publish]
[Comment]
[Close]
```

Review comments:

```text
Top-level proposal comments
```

In MVP, comments are advisory by default. They do not block publish. Inline comments, request changes, resolved threads, and required review rules are later PR-lite features.

### Publish

Publish updates a Team Space profile remote to an approved snapshot.

Publish is not the same as Create Snapshot. Create Snapshot creates a local snapshot. Publish moves the team remote profile to an approved snapshot.

By default, every team member can publish. The member who publishes performs the final approve action. Review, dry-run, and local apply are not mandatory gates in the MVP. Teams may assign stricter publish roles later.

Example:

```text
Publish Approved Profile

Snapshot: 8f3a2c7
Profile: qyinm-lab/frontend
Changes:
+ Figma MCP
~ Updated frontend review skill
- Removed old docs MCP

[Publish Snapshot]
```

### Bundle

Bundle is the portable file format used for moving a profile/setup between machines.

Bundle should be treated as implementation/export surface, not the center of the product.

User-facing language:

```text
Export Profile
Import Profile
Move setup to another Mac
```

Implementation language:

```text
.gandalf bundle
bundle verify
bundle inspect
bundle import
```

### Guard

Guard is the product behavior of detecting risky changes.

Daemon is an implementation detail. Guard is a user-facing safety feature.

Guard answers:

```text
Is my AI agent setup protected?
What changed?
Is it risky?
Can I review or undo it?
```

Guard should eventually power desktop notifications:

```text
High risk change detected

Codex added a hook that can execute shell commands:
~/.codex/config.toml

[View Diff] [Restore Previous Setup] [Acknowledge]
```

## Git Analogy

| Git Concept | Gandalf Concept | Meaning |
|---|---|---|
| repository | local profile store | saved profile history for global agent setup |
| branch | profile | selectable setup state |
| current branch | active profile | profile currently applied locally |
| working tree | current global setup files | real files on disk |
| unstaged changes | working changes | drift from active profile |
| commit | create snapshot | record current setup into active profile |
| checkout | switch profile | apply another setup state |
| reflog | timeline/snapshots | rollback history |
| remote branch | team profile | shared profile owned by team |

This analogy is valuable but should not be overexposed in the UI. Use it to guide behavior and mental model, not to force Git vocabulary everywhere.

## Primary User Segments

### AI Coding Power User

Uses Codex daily and experiments with global Codex setup.

Pain:

- does not know which MCPs/skills are installed
- agents mutate setup
- setup breaks unexpectedly
- new machine setup is manual and error-prone

Value:

- save known-good setup
- see what changed
- switch work modes
- rollback quickly

### Agent Experimenter

Frequently tries MCP servers, skills, hooks, permissions, prompts.

Pain:

- experiments pollute default setup
- hard to return to clean baseline
- risky hooks or permissions are easy to forget

Value:

- Experimental profile
- Minimal Safe profile
- Save as New Profile
- Discard experiment

### Team Lead / DevEx

Wants consistent global AI coding setup across team members.

Pain:

- global MCP setup differs by engineer
- security-sensitive tools spread informally
- hard to know who has which setup

Value:

- team profiles
- approved MCPs/skills
- published profile snapshots
- profile drift visibility
- onboarding profile for new engineers

### Security-Conscious Developer

Worries about hooks, permissions, shell commands, remote MCPs, and secret exposure.

Pain:

- agents can add executable setup
- agent config is scattered
- secret-like values must not be captured raw

Value:

- risk explanation
- restore preview
- trust contract
- no command execution during scan

## User Journey Map

### 1. First Discovery

User state:

```text
I use Codex and my setup is getting messy.
```

Gandalf job:

```text
Show what exists.
Create a Default profile.
Make the user feel safe.
```

Ideal first screen:

```text
Gandalf found your Codex setup

Codex config   3 items
MCP servers    7 items
Env keys       4 items

Default profile created from current setup.

[Create Snapshot] [View Current Setup]
```

### 2. First Snapshot

User state:

```text
This setup works. I want to save it.
```

Gandalf job:

```text
Turn current setup into a snapshot in the active profile.
```

UI:

```text
Create Snapshot in Default

Captured:
- 7 MCP servers
- 23 skills
- 2 hooks
- 4 global settings files

[Create Snapshot]
```

Outcome:

```text
Saved snapshot in Default.
```

### 3. Agent Changes Setup

User state:

```text
I asked Codex to install or configure something.
```

Gandalf job:

```text
Detect and explain working changes.
```

UI:

```text
Working Changes in Default

+ MCP server: figma
+ Skill: react-review
~ Codex permissions changed

[Create Snapshot] [Save as New Profile] [Discard]
```

### 4. Risk Explanation

User state:

```text
Is this safe?
```

Gandalf job:

```text
Explain the risk in simple language.
```

Risk examples:

```text
Low
- instruction text changed
- markdown skill added

Medium
- MCP server added
- permission changed
- remote MCP URL added

High
- hook can execute shell commands
- wildcard permission added
- unknown executable path added
- sensitive env key required
```

UI:

```text
Risk: High

Reason:
New hook can execute shell commands before Codex actions.

Changed:
~/.codex/config.toml

[View Diff] [Restore Previous Setup] [Acknowledge]
```

### 5. Compare

User state:

```text
What exactly changed?
```

Gandalf job:

```text
Show domain-level diff first, raw file diff second.
```

UI:

```text
Added
+ MCP server: figma
+ Skill: react-review

Changed
~ Permission: Bash commands now allowed
~ Codex config.toml

Removed
- MCP server: old-docs
```

### 6. Snapshot / Branch / Discard Decision

User state:

```text
Do I keep this setup change?
```

Gandalf job:

```text
Offer safe choices based on profile model.
```

If active profile is personal:

```text
[Create Snapshot in Default]
[Save as New Profile]
[Discard Changes]
```

If active profile is team:

```text
[Create Local Snapshot]
[Push / Propose Update]
[Revert to Team Profile]
```

### 7. Switch Profile

User state:

```text
I want a different agent setup for this work mode.
```

Gandalf job:

```text
Preview, snapshot, apply, rollback if needed.
```

UI:

```text
Switch Profile

From: Default
To: Frontend

Will change:
+ Figma MCP
+ Playwright skill
- Old backend MCP

Before switching:
Gandalf will create a snapshot in Default.

[Review & Switch]
```

### 8. Conflict / Unsaved Changes

User state:

```text
I have local changes but want to switch profile.
```

Gandalf job:

```text
Prevent accidental loss.
```

UI:

```text
You have unsaved changes in Default.

Changed:
+ MCP server: figma
+ hook: postToolUse

Before switching to Backend:

[Create Snapshot in Default]
[Save as New Profile]
[Discard Changes]
[Cancel]
```

This is a critical trust moment. Gandalf must not silently overwrite local setup.

### 9. Restore / Rollback

User state:

```text
Something broke. Take me back.
```

Gandalf job:

```text
Restore the whole snapshot safely and explain what will happen.
```

UI:

```text
Restore Snapshot 8f3a2c7

This restores the whole supported user-global setup captured in this snapshot.

Will change:
~ ~/.codex/config.toml
- Codex MCP server: figma

Will not restore:
- raw .env values
- unsupported symlinks

[Preview] [Restore Snapshot]
```

After restore:

```text
Restored setup.
Snapshot saved: before-rollback-2026-06-10
```

### 10. Team Profile Adoption

User state:

```text
My team published an approved setup.
```

Gandalf job:

```text
Make adoption feel like switching branches, not downloading files.
```

UI:

```text
Team Profile
qyinm-lab/frontend @ 72ab91d

Published by: qyinm-lab
Snapshot: 72ab91d
Status: behind by 1 snapshot

Includes:
- Figma MCP
- GitHub MCP
- frontend review skill

[Review & Switch Locally]
[View History]
```

### 11. Team Profile Contribution

User state:

```text
I improved this profile and want my team to use it.
```

Gandalf job:

```text
Make sharing feel like pushing a branch and opening a lightweight proposal.
```

UI:

```text
Propose Update to Team Profile

Base:
qyinm-lab/frontend @ 72ab91d

Local tracking profile:
alice/frontend

Changes:
+ Figma MCP
~ Updated frontend review skill
- Removed old docs MCP

Status:
Ahead by 3 snapshots

Risk:
1 medium

Reviewers:
@devex

[Push & Create Proposal]
```

Reviewer UI:

```text
Profile Proposal
qyinm-lab/frontend

Snapshot:
8f3a2c7

Proposed by: alice

Changes:
+ Figma MCP
~ Updated frontend review skill

Risk:
1 medium

Actions:
[Publish]
[Comment]
[Close]
```

### 12. New Machine

User state:

```text
I bought a new Mac.
```

Gandalf job:

```text
Apply profile with readiness checks.
```

UI:

```text
Import Profile

Profile: Default
Source: old Mac

Ready:
- Codex config
- Codex MCP config

Needs action:
- gh command missing
- FIGMA_TOKEN env key missing

[Apply Ready Items]
[Show Setup Instructions]
```

## Desktop Product Direction

Gandalf should become a desktop app, but not by simply porting the TUI.

The desktop app should be dashboard-first with a lightweight menu bar companion.

### Desktop Role

Desktop Gandalf should answer:

```text
Which profile is active?
What changed?
Is it risky?
Can I save, switch, or rollback?
Is my team setup in sync?
```

Dashboard owns:

```text
Overall status
Active profile context
Setup overview
MCP inventory
Skills inventory
Hooks inventory
Snapshot changelog
Switch/restore/diff task screens
Cloud sync status
Team proposal task screens
Account settings mode
```

Menu bar owns:

```text
Active profile status
Protection/risk status
Last snapshot time
Open Dashboard
Create Snapshot
Restore Last Snapshot
```

### First Dashboard

```text
Overall
Active Profile: Default
Current Snapshot: 8f3a2c7

Remote
local only

Status
3 changes since last snapshot, 1 high risk

Primary Actions
[Create Snapshot] [View Diff] [Restore Previous]

Changelog
8f3a2c7  12 min ago  Manual snapshot
72ab91d  1h ago      Codex config changed
```

### Navigation

Recommended desktop sections:

```text
Home
Setup
MCP
Skills
Hooks
```

Profiles are selected from the top-left profile picker, not from a sidebar section.

Team profiles are grouped by team space inside the profile picker, not from a permanent Team sidebar section.

Timeline appears as the Home changelog and the snapshot dropdown from the custom window titlebar.

The sidebar bottom should show the account user profile, not Protection/Cloud/Device status. This account user profile is different from Gandalf setup profiles.

Clicking the account row settings icon should switch the sidebar into settings mode with Account, Cloud, Device, Protection, Notifications, Local Paths, Privacy, and About.

The desktop UI should use icons for compact controls and repeated status signals. Text placeholders in ASCII design docs, such as chevrons or settings markers, are documentation only and should become real icons in product UI.

Compare, restore, switch preview, cloud sync, and proposal review should be task screens opened from the relevant object. They should not be primary sidebar sections in MVP.

Avoid making engine concepts top-level:

```text
Scan
Audit
Graph
Provenance
Bundle
Profiles
Team
Timeline
Current Setup
Settings as a main nav item
```

These can exist under advanced/debug views.

## Desktop MVP Design

Detailed desktop screen composition and ASCII wireframes live in [docs/design/desktop-mvp.md](docs/design/desktop-mvp.md).

This product document keeps only the desktop product direction and menu bar/protection requirements.

### Menu Bar / Guard

Desktop MVP should include lightweight menu bar status.

```text
Gandalf: Protected
Active Profile: Default
2 changes since last snapshot
1 high-risk change
```

Menu bar should stay thin. It should not become the main profile management UI.

Guard mode should be framed as protection, not daemon management.

Protection should be started through onboarding and controlled through the menu bar app.

Commands should eventually be:

```bash
gandalf guard enable
gandalf guard status
gandalf guard disable
```

not:

```bash
gandalf daemon start
```

## CLI Product Direction

The CLI remains important as the engine and automation surface.

Current CLI language can use snapshot because snapshot and profile save are the same core action. Future CLI can expose profile language as clearer aliases.

Current:

```bash
gandalf snapshot create --name baseline --agent codex --scope user --project .
gandalf diff baseline current --agent codex --scope user --project .
gandalf restore --snapshot baseline --dry-run --agent codex --scope user --project .
```

Future:

```bash
gandalf profile init
gandalf profile save
gandalf profile status
gandalf profile switch frontend --dry-run
gandalf profile switch frontend --apply
gandalf profile fork team/frontend personal/frontend-custom
gandalf profile push team/frontend
gandalf profile update team/frontend
gandalf profile proposal create team/frontend
gandalf profile review team/frontend/proposals/42
gandalf profile publish team/frontend 8f3a2c7
```

Compatibility commands can remain aliases.

## Trust Contract

Gandalf's trust contract is a core product feature.

By default Gandalf:

- reads local user-global agent configuration only
- does not execute MCP commands
- does not execute hooks
- does not execute skill scripts
- does not execute plugins or agent tools
- does not use the network during local scan
- does not store raw `.env` values
- does not upload raw secret values to cloud sync
- does not follow symlinks
- previews restore/switch/import before applying
- creates automatic snapshots before destructive operations
- reports missing tools and env keys without installing packages or restoring secrets

This contract must remain visible near risky features.

## Apply / Restore Policy

Not every setup surface should be equally restorable.

### Restore MVP Scope

MVP restore is whole snapshot restore only.

```text
Restore Snapshot
= restore the supported user-global setup captured in that snapshot
```

MVP should not expose partial restore by setup area.

Out of scope for MVP:

```text
Restore only MCP servers
Restore only skills
Restore only hooks
Restore only permissions
Restore selected settings fields
```

Restore must still show a preview before writing files.

### Profile Switch MVP Scope

Profile switch must support user-global agent setup files only in the MVP.

Included examples:

```text
~/.codex/config.toml
supported Codex global config
```

This is what makes Gandalf a global AI agent environment switcher.

The product must treat user-global writes as first-class supported behavior, but never casual behavior. A global write needs a preview, an automatic snapshot, parser confidence, and an obvious rollback path.

Repo-local setup files are out of scope for MVP and should not be scanned, diffed, risk-scored, switched, or restored by Gandalf.

Examples of out-of-scope repo-local files:

```text
.mcp.json
.cursor/mcp.json inside a repo
AGENTS.md
CLAUDE.md
```

### Safe to Restore Earlier

- user-global structured settings files
- known user-global skill files when full content was captured
- supported structured settings fields

### Requires More Caution

- user-global Codex settings such as `~/.codex/config.toml`
- hooks
- permissions
- files that can execute commands
- files with unknown parser confidence

### Never Restore Automatically

- raw env values
- unsupported symlinks
- unknown executable state
- provider-side remote behavior
- remote MCP server behavior

## Risk Model

Risk is not the same as security audit. It is a product explanation layer.

### Low Risk

- instruction markdown changed
- skill markdown added
- non-executable metadata changed

### Medium Risk

- MCP server added/changed
- remote MCP URL added
- permission changed
- env key inventory changed

### High Risk

- hook added or changed
- wildcard permission added
- executable command path changed
- unsupported state changed near agent execution path

Risk UI must always explain:

```text
What changed?
Why does it matter?
What can I do?
```

Risk UI must not prevent local snapshot creation.

The correct behavior is:

```text
Record first.
Warn clearly.
Make restore obvious.
Require team review before publish.
```

For local usage, risk is a warning and explanation layer. For team usage, risk becomes a review signal.

## Pricing / Packaging Direction

### Free

- local CLI
- desktop dashboard
- personal profiles
- create snapshot/switch/compare/restore
- local snapshots
- manual `.gandalf` export/import
- local guard for personal setup

### Pro

- cloud profile sync
- personal cloud remotes
- ahead/behind status for personal profiles
- push/pull personal profile snapshots
- multi-machine sync
- encrypted cloud backup
- secret-redacted cloud snapshots
- future opt-in end-to-end encrypted secret sync
- cloud profile history

### Team

- Team Spaces
- shared approved profiles
- team cloud remotes
- ahead/behind status for team profiles
- push/pull team profile snapshots
- profile proposals
- review and approval workflow
- role-based team permissions
- member/device drift visibility
- onboarding profiles
- approved MCP/skills catalog

## MVP Sequence

The product should not jump straight to a large desktop app before core profile semantics are reliable.

Recommended sequence:

```text
1. Current setup -> Default profile
2. Create Snapshot
3. Working Changes detection
4. Profile status screen
5. Switch Profile dry-run for user-global files
6. Safe apply with automatic snapshot
7. Save as New Profile
8. Switch Team Profile as local tracking profile
9. Automatic local snapshots while using a team profile
10. Push local snapshots
11. Profile Proposal create/review/publish flow
12. Desktop dashboard + lightweight menu bar
13. Personal cloud remote sync
14. Guard notifications
15. Team review/publish
```

## Near-Term Implementation Translation

Current implementation objects map to future product concepts like this:

| Current Implementation | Product Concept |
|---|---|
| setup commit | immutable captured user-global setup state |
| snapshot | create snapshot / setup commit |
| profile save | snapshot in the active profile |
| timeline event | local history event |
| bundle | export/import format |
| scan | inventory engine |
| diff | compare engine |
| audit | risk signal input |
| restore | rollback/apply engine |
| doctor | readiness check |
| daemon | should not be product concept; replaced by guard |

## Product Principles

### 1. Profile First

Users should always be inside a profile. Create Snapshot is the act of saving current setup into that profile.

### 2. Preview Before Write

Any operation that writes user-global local agent setup files must have a preview.

### 3. Snapshot Before Risk

Before switch, restore, import, or high-risk apply, Gandalf creates an automatic snapshot in the current profile.

### 4. Domain Language First

Say:

```text
Codex added Figma MCP
```

not:

```text
Graph node changed
```

### 5. Trust Is a Feature

The product must be conservative about file writes, execution, secrets, symlinks, and unknown state.

### 6. Personal Freedom + Team Standard

Team profiles should not eliminate personal customization. They should provide a safe base with clear override/fork semantics.

### 7. Team Changes Need Lightweight Review

Team profile updates should use a lightweight proposal, diff, comment, and publish flow. By default all members can review and publish, and review is not a mandatory gate. Inline comments, request changes, stricter roles, and required checks are optional future team policy.

### 8. Team Profiles Are Remote Branches

Using a team profile creates or updates a local tracking profile. Local environment changes become local snapshots. Sharing them requires push plus Profile Proposal.

### 9. Desktop Is Dashboard Plus Lightweight Menu Bar

Desktop should center active profile, working changes, risk, timeline, switch, save, and rollback in the dashboard. Menu bar should provide status, notifications, and quick actions only.

### 10. Cloud Sync Uses Pull-Rebase

When a profile is ahead and behind its cloud remote, Gandalf should update from remote by replaying local snapshots on top of the latest remote snapshot. Conflicts stop for explicit review.

### 11. Restore Whole Snapshots First

MVP restore should restore the whole supported user-global setup from a snapshot. Partial restore can come later only after parser confidence and conflict UI are strong enough.

### 12. No Raw Secrets In Cloud Sync

MVP cloud sync should upload config structure and env key names only. Raw secret values must be redacted or blocked. End-to-end encrypted secret sync can be considered later as an explicit opt-in feature.

### 13. Automatic Snapshots Prefer Stable Groups

Automatic snapshots should use debounced groups: 30 seconds after the last change, 2 minute max window, faster flush for high-risk changes, and mandatory flush before switch/restore/push/update/quit.

### 14. MVP Team Role Is Member Only

MVP should expose only `Member` as a visible team role. Internally, permissions should be capability-based so stricter roles can be added later without changing the product model.

### 15. Team Access Starts With Invites

MVP Team Space access should be based on email invites and revocable invite links. Gandalf should not depend on GitHub organizations, repository ownership, or enterprise identity systems in the first version.

### 16. Invite Links Are Lightweight But Bounded

Generic invite links are allowed for easy onboarding, but they should expire, be revocable, grant only Member access, and be recorded in team activity.

### 17. Cloud Login Is Device Approval

Gandalf Cloud login should treat each Mac or CLI installation as an approved device session. The browser owns account sign-in; desktop and CLI receive approved device access.

### 18. Device Sessions Are Server-Revocable

MVP should store an opaque device session token locally and keep server-side device session state. JWT may be used later for short-lived API access, but the durable device session should remain revocable by the server.

### 19. Device Names Should Be Recognizable

Gandalf should default to the macOS system name for approved devices and allow users to rename devices later. Device naming should optimize approval, audit, and revoke clarity.

### 20. Remote Revoke Does Not Mean Local Wipe

Remote device revoke should stop cloud access but preserve local profiles, snapshots, and the current Codex setup. Gandalf should not promise remote wipe in the MVP.

### 21. Local Snapshot History Must Be Secret-Safe

MVP local snapshots should redact secret-like values and prompt for missing values during restore. Gandalf should not store raw secrets in snapshot history by default.

### 22. Preserve Current Secrets Only When Confident

When restoring redacted secrets, Gandalf may preserve the current local secret value only when the key, field path, and parser confidence match. Otherwise it should prompt the user.

### 23. Manage The Full Codex Setup, Apply Safely

MVP should manage Codex config plus supported extension surfaces such as instructions, MCP, tools, hooks, and skills. Gandalf may use Git-like storage inside `~/.gandalf`, but must apply changes to `~/.codex` through a safe apply engine rather than raw Git checkout.

### 24. Warn, Record, And Make Restore Easy

MVP should warn about executable Codex setup changes through macOS notifications, dashboard risk labels, and timeline records. It should not block local changes by default; it should make review and restore obvious.

### 25. Notify By Risk, Record Everything

MVP should notify by default for medium and high risk changes, but record every detected Codex setup change in the dashboard and timeline.

### 26. Protection Is Opt-In And Visible

Background watching should start through onboarding opt-in and be visible through the menu bar app. Gandalf should say Protection, not daemon, in user-facing UI.

### 27. Onboarding Proves Local Value First

MVP onboarding should scan the current Codex setup, create the Default profile, and create the first snapshot before asking for Protection, cloud sign-in, or Team Space setup.

### 28. Team Invites Preserve Local Safety

If first launch comes from a Team Space invite, Gandalf should show the invite context but still create the local Default profile and first snapshot before joining the team or applying team profiles.

### 29. Local-First Does Not Block Sign-In

Gandalf should be usable without an account, but local users can sign in at any time to enable personal cloud remotes, multi-device sync, personal cloud backup, and Team Spaces.

### 30. Sharing Between People Uses Team Spaces

Personal cloud profiles are for one user's own devices. Sharing profiles with other people should happen through Team Spaces and team profile remotes, not personal cloud sharing, in the MVP.

### 31. Sign-In Attaches Personal Cloud Remotes

When a user signs in, Gandalf should automatically attach local personal profiles to personal cloud remotes. This enables sync state immediately without making personal profile sharing a product concept.

### 32. Initial Personal Cloud Sync Should Feel Automatic

After sign-in, personal profiles should run an automatic initial bootstrap sync in the no-conflict path with a visible sync indicator. After that bootstrap, new local snapshots should stay local until the user pushes them.

### 33. Personal Sync Rebases Local Work On Cloud History

When local and personal cloud profiles share a name, Gandalf should fetch remote history and replay local snapshots on top when it can do so cleanly. Conflicts stop for review.

### 34. MVP Conflicts Resolve At Profile Level

MVP should resolve conflicts with whole-profile choices: use local, use cloud, keep local as new profile, or cancel. File-level and field-level merge UI should come later.

### 35. Auto-Created Profiles Need Human Names

When Gandalf creates profiles during conflict resolution, names should be reason-based and editable, while precise timestamps, devices, and snapshot ids live in metadata.

### 36. Profile Names Are Display Labels

Profile rename should update the display label, not the stable profile id or cloud remote slug. Remote rename should be a separate explicit action.

### 37. Profile Delete Is Local-Only In MVP

Deleting a local personal profile should delete local state only and preserve cloud history. MVP should not add archived cloud remote state.

### 38. Cloud-Only Profiles Live In Web Dashboard

When a personal cloud remote exists without a local profile, users should recover or attach it from the Gandalf web dashboard, not from the primary desktop profile picker.

### 39. Web Owns Cloud State, Desktop Owns Local Apply

The web dashboard should manage account, devices, billing, personal cloud profiles, and basic team membership. The desktop app should own local profile switching, restore, protection, and Codex setup writes.

### 40. Team Proposal Review Belongs In Desktop

MVP team proposal review and publish should happen in the desktop app, because proposals are evaluated through setup diff, risk, preview, and local rollback context.

### 41. Team Publish Is Lightweight By Default

MVP team proposal publish should be lightweight: diff, risk, and comments inform the publisher, but dry-run, local apply, and required review are not mandatory gates by default.

### 42. Team Proposals Show Diff, Not Checklists

MVP team proposals should primarily show the profile diff and risk label. Automatic checklists should not be a main proposal UI concept in the MVP.

### 43. Team Proposal Comments Are Top-Level

MVP team proposals should support top-level Markdown comments. Inline comments, request changes, resolved threads, and required reviews are later PR-lite features.

### 44. Proposal Diff Is Semantic First

Team proposal review should default to semantic setup diff and risk labels. Raw line-by-line review is not a core proposal UI in MVP.

### 45. Inline Review Features Are Later

Team proposal comments should not attach to semantic diff entries or raw file diff lines in MVP. Top-level comments are enough for the first team workflow.

### 46. Published Proposals Keep Lightweight Audit History

Published team proposals should remain readable with semantic diff, risk label, top-level comments, publisher, timestamp, and published snapshot id. Team profile timeline should link back to published proposals.

### 47. Closed Proposals Are Minimal

Closed unpublished proposals should be preserved as lightweight records with title, semantic diff, top-level comments, close reason, and clear Not Published status. Rich closed proposal filters can come later.

### 48. Proposals Open Immediately

MVP team proposals should not have draft state. Creating a proposal opens it for team review immediately; preparation happens through local snapshots before proposal creation.

### 49. Proposal Titles Are Generated But Editable

Gandalf should generate proposal titles from semantic diff, allow editing before creation, and keep description optional.

### 50. Proposal Text Uses Markdown

Proposal descriptions and comments should support basic Markdown, not a full rich text editor or full GitHub-flavored Markdown promise in the MVP.

## Open Questions

These remaining product decisions are not MVP blockers unless explicitly promoted.

### Q51. Later Team Review Expansion

If teams heavily use proposal comments, should Gandalf expand lightweight proposals into PR-lite review later?

MVP decision:

```text
Lightweight proposal:
Proposal has title, optional Markdown description, semantic diff, risk label, top-level comments, and publish.
```

Deferred options:

```text
PR-lite:
Lightweight proposal plus inline comments, closed/published history, and request changes.

Full PR-like:
Inline comments on semantic/raw diff, resolved threads, closed/published pages, request changes, and future required review policies.
```

## Immediate Next Product Decision

The team proposal MVP decision is now resolved:

> Ship Lightweight Proposal in MVP.

The next product decision should be about desktop MVP sequencing, not deeper PR-style review mechanics.
