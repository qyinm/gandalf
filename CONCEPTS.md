# Concepts

> Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## Product Identity

### Gandalf
The selected product identity for Gandalf. Gandalf is a local control console for AI agent setup: it inspects installed capabilities, browses agent-native marketplace/source entries, and runs reviewed provider-backed changes.

### Setup Container
A historical/future portability concept for captured AI agent setup state. It can describe snapshots or bundles, but it is not the current product identity and should not imply an OS container, remote agent runtime, or active profile system.

### Local Control Console
The current product direction for Gandalf: a local TUI-first console for user-global AI agent setup. It separates installed inventory, agent-native marketplace/source browsing, and provider-backed actions.

### Global Agent Setup Manager
Older wording for the current local control console direction. Prefer "local control console" in new product docs; the scope remains user-global agent skills, hooks, MCP servers, plugins, and agent-native marketplace/source entries across currently supported agents.

### Current Supported Agent Set
The product-visible agent boundary for the current Gandalf loop. Gandalf may keep legacy scanners or type constants for compatibility, but the default TUI, CLI help, active scan path, and documentation should only present current Codex and Claude Code support until broader support is intentionally reintroduced.

### Unified Inventory
The normalized cross-agent setup inventory used by the setup console. It presents skills, hooks, MCP servers, and plugins as global/user setup rows with compact agent identity rather than forcing users through an agent picker first.

### Setup Console
The current target information structure for Gandalf's default TUI. It uses top-level setup tabs for hooks, plugins, agent-native marketplace/source browsing, skills, and MCP servers while preserving cross-agent rows inside each tab.

### Changes-First Home
The default Gandalf TUI surface that summarizes drift from the latest supported baselines before users enter inventory browsing or recovery flows.

It is read-only: Review opens the detailed environment diff, while rollback must enter Review Changes before apply.

### Environment Diff Surface
A TUI-visible unit of environment drift for one semantic setup object or raw source artifact. It exists so semantic object changes and raw source changes both remain navigable and cannot be hidden behind a clean summary.

### Agent-Native Marketplace/Source
A marketplace, registry, plugin repository, or source exposed by an agent ecosystem and browsed through Gandalf. Gandalf can group and display source-backed entries, but install, update, uninstall, add-source, and remove-source actions are available only through agent-native provider-backed actions; Gandalf does not own or certify the catalog itself.

### Provider-Backed Action
A setup action backed by a provider that can describe the target, expected effect, Review Changes preview, and execution mechanism. Inventory visibility does not imply action executability; Gandalf can truthfully mark an action available only when a provider-backed action exists.

### Marketplace-Originated Review Action
A Review Changes-style flow that starts from an agent-native marketplace/source entry. The first safe version can produce non-mutating setup instructions or source-backed guidance, but install, update, uninstall, add-source, and remove-source remain unavailable until an agent-native provider can preview and execute a concrete effect.

### Setup Action Provider
The component that turns a visible setup inventory item into a provider-backed edit, remove, add, install, update, uninstall, or dry-run action.

### Skill Markdown Overlay Viewer
A read-only Setup Console overlay that opens from a selected skill and renders its `SKILL.md` entrypoint as terminal markdown. It makes inspection the primary Skills tab `Enter` behavior while keeping setup mutations behind explicit provider-backed actions.

## Restore

### Trust Contract
The safety boundary Gandalf promises for scan, snapshot, diff, restore, and bundle flows. In this project it means read-only discovery, confined writes under declared home/project roots, symlink refusal on write targets, and restore behavior that matches the evidence kind rather than falling back to unsafe generic file mutation.

### Evidence
A discovered configuration artifact Gandalf tracks for drift and restore planning. Each evidence record has a kind (config file, MCP server entry, permission rule, env key, etc.), a source path, and optional structured value metadata.

### Evidence Kind
The typed category of an evidence record that determines how restore planning and apply handlers treat it. Kinds with structured JSON values (MCP server, permission, env key) require dedicated apply handlers rather than whole-file replacement.

### Restore Plan
The diff-shaped output of comparing a baseline snapshot to current state. Lists planned items with actions (update, delete), risk metadata, and target state—but does not mutate the filesystem until apply.

### Review Changes
The user-facing preview step before a mutating action applies. Internally it can be backed by a restore plan or action preview, but product language should describe the concrete changes, unsupported items, rollback availability, and required apply confirmation rather than asking users to learn a separate plan concept.

A Review Changes surface is not itself apply authority. Mutating flows that depend on it must refresh or revalidate the underlying plan at apply time so the action still matches what the user reviewed.

### Restore Item
An executable unit derived from a restore plan item. Carries resolved destination path, structured `target_content`, handler `item_type`, and rollback state after apply.

### Apply Handler Registry
The dispatch table mapping restore item types to apply functions. Plan generation and apply execution share type labels; a missing registry entry surfaces as a handler error at apply time even when the plan looks valid.

### Path Confinement
The trust boundary that restricts restore and bundle writes to declared home and project roots. Confinement must be active in plan parsing, apply, rollback, and bundle import, and it only holds when the path that is actually written is the same path that was validated. Callers must supply roots or apply fails closed.

## Snapshots and Store

### Baseline Coverage
The per-agent completeness state of the Changes-First Home, which may be empty, partial, or complete across the Current Supported Agent Set.

Capturing missing baselines preserves existing agent baselines and fills only uncovered agents so established comparison points do not move silently.

### Snapshot
A named capture of project and user-global evidence at a point in time. Snapshots may be metadata-only or content-backed depending on capture policy.

### Content-Backed Snapshot
A snapshot whose store entry includes captured file bytes in addition to metadata and structured evidence. Restore safety depends on content-backed snapshots when byte-exact restoration of agent config files is required.

### Store
The on-disk persistence layer for snapshots, timeline entries, and related Gandalf state. CLI and TUI surfaces read the same store APIs for snapshot listing and changelog, so snapshot replacement must be atomic enough that readers never observe new metadata paired with partial or missing content blobs.
