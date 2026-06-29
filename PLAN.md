<!-- /autoplan restore point: /Users/hippoo/.gstack/projects/gandalf/no-branch-autoplan-restore-20260512-000950.md -->

# Gandalf Plan

Source: [PRODUCT.md](PRODUCT.md)

Status: superseded plan, current direction summarized here
Last updated: 2026-06-29

## Current Direction

Gandalf is a local control console for AI agent setup: it inspects installed capabilities, browses agent-native marketplace/source entries, and runs reviewed provider-backed changes.

The previous local-history/profile and broad multi-agent plan is no longer the active product plan. Restore, diff, bundle, and snapshot flows remain the backing trust workflows, but the first product surface is the Unified Agent Setup Console.

## Gate 2: Unified Agent Setup Console

Gate 2 now means:

- installed inventory for current Codex and Claude Code support
- user-global setup tabs for skills, hooks, MCP servers, plugins, and agent-native marketplace/source entries
- agent-native marketplace/source browsing rather than a Gandalf-owned marketplace or registry
- truthful action availability, including unavailable reasons when no provider-backed action exists
- Review Changes before mutation
- at least one provider-backed safe action
- restore safety as a backing regression, not the product definition

The current `scripts/gate2-acceptance.sh` still protects restore safety. It should be renamed or split in a later code PR; this docs plan does not rename scripts.

## Current Support Boundary

| Surface | Current plan |
|---|---|
| Codex | User-global discovery, normalized inventory, source-backed plugin/skill browsing where discovered, content-backed restore safety where supported |
| Claude Code | User-global discovery, normalized inventory, source metadata browsing where discovered, content-backed restore safety where supported |
| Cursor, OpenCode, Pi Agent | Legacy parser or scanner code may remain, but these are not current product surfaces |
| Project-local files | Out of current product scope; Git already owns repo-local setup files |
| Marketplace/source | Agent-native source browsing only unless an agent-native provider can preview and execute a concrete action |
| Actions | Provider-backed only; visible rows do not imply edit/install/remove support |

## Product Promise Boundaries

Current:

- read-only global setup discovery for current Codex and Claude Code support
- normalized Setup Console inventory across skills, hooks, MCP servers, plugins, and agent-native marketplace/source entries
- Review Changes before any mutating provider-backed action or restore-backed apply
- explicit unavailable reasons for unsupported actions and unsupported restore items
- content-backed snapshots and restore safety for supported Codex and Claude Code user-global files
- Go CLI and Bubble Tea TUI distributed through GitHub Releases, `install.sh`, Homebrew, and source builds

Not current:

- scanning and capturing additional agents as the product promise
- project-local setup management
- Gandalf-owned marketplace, skill registry, or trust-certified catalog
- new marketplace install/update/uninstall actions without concrete provider-backed implementation
- local profiles, profile switching, team profiles, cloud sync, desktop launch, or background daemon
- release automation, script renames, or v0.5.0 tagging as part of this alignment work

## Architecture Translation

The current implementation should preserve these boundaries:

- Discovery is read-only and global/user-scoped by default.
- Inventory is normalized before the TUI renders it.
- Marketplace rows are source-backed inventory, not catalog ownership.
- Action availability is determined by setup action providers.
- Review Changes is the user-facing mutation preview and must be refreshed or revalidated before apply.
- Restore planning, path confinement, symlink refusal, rollback, and content-backed snapshots remain the trust layer behind apply flows.

## Near-Term Follow-Ups

- Rename or split `scripts/gate2-acceptance.sh` so restore-safety regression and setup-console acceptance are separate code checks.
- Split helper boundaries only when code work requires it; do not do it as part of docs alignment.
- Add new provider-backed actions only with concrete previews, execution paths, unavailable reasons, and tests.
- Revisit release planning after docs and code boundaries are aligned.

## Retained Historical Context

Older versions of this plan described Gandalf as a local-history profile product, a broad multi-agent scanner, and future cloud/team profile product. Those ideas are historical context, not active implementation direction.
