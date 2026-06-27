# Concepts

> Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## Product Identity

### Gandalf
The selected new product identity for Gandalf. Gandalf is the wizard for AI agent setup: it guides setup changes, captures working agent environments into portable setup containers, and makes those containers rollbackable across machines.

### Setup Container
A portable, rollbackable captured AI agent setup state managed by Gandalf. This is the product-level container concept behind snapshots, bundles, profile states, and cross-machine restore flows; it should not imply an OS container or remote agent runtime.

### Global Agent Setup Manager
The current product direction for Gandalf: a TUI-first manager for user-global agent skills, hooks, MCP servers, and plugins across supported agents. It excludes project-local setup surfaces from the core product scope.

### Unified Inventory
The normalized cross-agent setup inventory used by the setup console. It presents skills, hooks, MCP servers, and plugins as global/user setup rows with compact agent identity rather than forcing users through an agent picker first.

### Setup Console
The current target information structure for Gandalf's default TUI. It uses top-level setup tabs for hooks, plugins, marketplace, skills, and MCP servers while preserving cross-agent rows inside each tab.

### Agent Marketplace Source
A marketplace or plugin source exposed by an agent ecosystem and browsed through Gandalf. Gandalf can group and display source-backed entries, but install, update, uninstall, add-source, and remove-source actions are available only through agent-native providers; Gandalf does not own or certify the catalog itself.

### Setup Action Provider
The component that turns a visible setup inventory item into a concrete edit, remove, add, or dry-run action. Inventory visibility does not imply action executability; a provider must describe the target, expected effect, and execution mechanism before Gandalf can truthfully mark an action available.

## Restore

### Trust Contract
The safety boundary Gandalf promises for scan, snapshot, diff, restore, and bundle flows. In this project it means read-only discovery, confined writes under declared home/project roots, symlink refusal on write targets, and restore behavior that matches the evidence kind rather than falling back to unsafe generic file mutation.

### Evidence
A discovered configuration artifact Gandalf tracks for drift and restore planning. Each evidence record has a kind (config file, MCP server entry, permission rule, env key, etc.), a source path, and optional structured value metadata.

### Evidence Kind
The typed category of an evidence record that determines how restore planning and apply handlers treat it. Kinds with structured JSON values (MCP server, permission, env key) require dedicated apply handlers rather than whole-file replacement.

### Restore Plan
The diff-shaped output of comparing a baseline snapshot to current state. Lists planned items with actions (update, delete), risk metadata, and target state—but does not mutate the filesystem until apply.

### Restore Item
An executable unit derived from a restore plan item. Carries resolved destination path, structured `target_content`, handler `item_type`, and rollback state after apply.

### Apply Handler Registry
The dispatch table mapping restore item types to apply functions. Plan generation and apply execution share type labels; a missing registry entry surfaces as a handler error at apply time even when the plan looks valid.

### Path Confinement
The trust boundary that restricts restore and bundle writes to declared home and project roots. Confinement must be active in plan parsing, apply, rollback, and bundle import, and it only holds when the path that is actually written is the same path that was validated. Callers must supply roots or apply fails closed.

## Snapshots and Store

### Snapshot
A named capture of project and user-global evidence at a point in time. Snapshots may be metadata-only or content-backed depending on capture policy.

### Content-Backed Snapshot
A snapshot whose store entry includes captured file bytes in addition to metadata and structured evidence. Gate 2 restore depends on content-backed snapshots when byte-exact restoration of agent config files is required.

### Store
The on-disk persistence layer for snapshots, timeline entries, and related Gandalf state. CLI and TUI surfaces read the same store APIs for snapshot listing and changelog, so snapshot replacement must be atomic enough that readers never observe new metadata paired with partial or missing content blobs.
