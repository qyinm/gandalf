---
title: Suppress MCP unsupported entries only when an executable config restore covers them
date: 2026-07-13
category: logic-errors
module: gandalfcore-restore-planning
problem_type: logic_error
component: tooling
symptoms:
  - "Review Changes listed a Codex TOML MCP removal as unsupported while the same plan contained a content-backed agent_config update for the identical agent and source path."
  - "The plan overstated unsupported work even though applying the config item would byte-exactly restore the MCP state."
  - "Blindly removing duplicate-looking MCP warnings would conceal a real limitation for metadata-only snapshots that cannot execute the whole-file config restore."
root_cause: logic_error
resolution_type: code_fix
severity: medium
tags:
  - restore-plan
  - unsupported-items
  - mcp-server
  - agent-config
  - content-backed
  - codex-toml
  - review-changes
  - coverage-deduplication
related_components:
  - testing_framework
---

# Suppress MCP unsupported entries only when an executable config restore covers them

## Problem

Codex represents MCP servers inside `~/.codex/config.toml`, so one physical file can produce both a raw `agent_config` change and semantic `mcp_server` changes. Restore planning walks every semantic change independently, appending unsupported actions to `UnsupportedItems` and executable actions to `Items` (`internal/gandalfcore/restore/plan.go:257`). For a content-backed snapshot, this meant the plan could contain an executable whole-file `agent_config` update and still report the TOML MCP change as unsupported, even though restoring the config bytes restores the embedded MCP definition too.

The semantic MCP branch intentionally supports create or update only for JSON MCP states, and `isJSONMCPState` recognizes `.mcp.json` and `/mcp.json`, not Codex TOML (`internal/gandalfcore/restore/plan.go:602`, `internal/gandalfcore/restore/plan.go:630`). The false warning came from two valid representations of the same physical change being reported as if they required two independent apply operations.

## Symptoms

- A Codex content-backed restore plan included an `agent_config` update and a Codex `mcp_server` entry under `UnsupportedItems`.
- The CLI rendered the redundant plan entry as a non-zero `Unsupported items` count.
- The whole-file `agent_config` path is separately covered by a byte-for-byte restore regression, so the preview overstated the remaining unsupported work.

## What Didn't Work

### Treat every semantic diff as an independent restore unit

The planner still has to classify a TOML MCP change as unsupported because the MCP-specific action is JSON-only. The missing step was reconciliation after all plan items were known, not broader MCP executability.

### Suppress every MCP warning whenever an `agent_config` item exists

Presence is not proof of coverage. `targetContentForPlanItem` returns file content for non-structured kinds only when the target value is a JSON string (`internal/gandalfcore/restore/plan.go:423`). A metadata-only config item therefore cannot reconstruct the file and must not hide the MCP warning.

### Route Codex TOML through the dedicated MCP handler

The registered MCP handler mutates JSON MCP state, while Codex stores the relevant state inside TOML. Reusing that handler would target the wrong storage format and weaken the Evidence Kind contract. Content-backed Codex TOML belongs to the whole-file `agent_config` handler; the semantic MCP entry is removed only as duplicate reporting.

## Solution

After collecting executable and unsupported items, the planner reconciles duplicate TOML MCP warnings before constructing the final restore plan (`internal/gandalfcore/restore/plan.go:340`):

```go
unsupportedItems = omitUnsupportedMCPChangesCoveredByAgentConfig(unsupportedItems, items)
```

The reconciliation first builds a coverage set from `agent_config` items. Delete actions need no target bytes; every non-delete action must yield target content through the same conversion used to create executable restore items (`internal/gandalfcore/restore/plan.go:363`):

```go
if item.Kind != types.KindAgentConfig {
	continue
}
if item.Action != types.RestoreActionDelete && len(targetContentForPlanItem(item)) == 0 {
	continue
}
coveredConfigPaths[restoreCoverageKey(item.Agent, item.SourcePath)] = struct{}{}
```

The second pass removes only unsupported `mcp_server` entries whose coverage key matches. The key includes agent identity and a cleaned source path, so one agent or file cannot hide an unrelated warning (`internal/gandalfcore/restore/plan.go:379`, `internal/gandalfcore/restore/plan.go:391`).

```go
if item.Kind == types.KindMcpServer {
	if _, covered := coveredConfigPaths[restoreCoverageKey(item.Agent, item.SourcePath)]; covered {
		continue
	}
}
filtered = append(filtered, item)
```

Paired tests encode both sides of the boundary: content-backed Codex config suppresses the duplicate warning, while metadata-only Codex config retains it (`internal/gandalfcore/restore/plan_test.go:229`, `internal/gandalfcore/restore/plan_test.go:291`).

## Why This Works

The filter asks a narrow question: will an executable plan item for this same agent and source path cover the bytes containing this semantic MCP change? It does not reinterpret TOML as JSON MCP state, add an action, or change an apply handler.

This preserves dedicated handlers and path confinement. JSON MCP evidence remains independently executable through its MCP handler, while content-backed Codex TOML remains a byte-backed agent-config restore. The reconciliation changes only which redundant unsupported descriptions are returned; destination validation and apply dispatch remain unchanged.

Metadata-only safety also remains truthful because an `agent_config` item without string target content never enters the coverage set. Its MCP warning survives, matching the fact that file bytes cannot be reconstructed from metadata alone.

## Prevention

- Reconcile overlapping evidence only after the complete executable and unsupported collections exist.
- Define coverage with the smallest stable identity that prevents cross-object suppression; here that is agent plus normalized source path.
- Require proof of executability before hiding a warning. A target state alone is insufficient.
- Keep positive and negative regression tests together: one content-backed fixture proves suppression and one metadata-only fixture proves retention.
- Preserve the distinction between presentation deduplication and execution support. New independently executable Evidence Kinds still require dedicated handlers.
- Re-run restore-safety acceptance whenever plan filtering changes so a reporting fix cannot bypass destination validation or alter apply behavior.

## Related Issues

- [Go Restore/Store Trust-Contract Gaps Found in Code Review](./go-restore-store-trust-contract-gaps.md) — applies the same path-identity discipline at the apply boundary.
- [Rust Core Restore Handler Gaps Found in Migration Code Review](./rust-core-restore-handler-review-gaps.md) — historical context for dedicated structured handlers and metadata-only limitations.
- [Keep TUI Reviews And Setup Actions Bound To Fresh Plans](../architecture-patterns/tui-fresh-review-action-plan-boundary.md) — downstream Review Changes freshness and fingerprint contract.
