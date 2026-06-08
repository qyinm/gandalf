---
title: "refactor: Type DiscoveredItem by evidence kind"
type: "refactor"
status: "completed"
date: "2026-06-08"
---

# refactor: Type DiscoveredItem by evidence kind

## Summary

Refactor `DiscoveredItem` from one flat interface with `unknown` payloads into a `kind`-discriminated TypeScript union. The runtime JSON shape stays compatible with existing scans, snapshots, bundles, CLI JSON, and `hem schema`.

---

## Problem Frame

`DiscoveredItem` is the durable evidence contract that scanners emit and the rest of Hem consumes. It currently carries precise common fields, but `value?: unknown` and `metadata?: Record<string, unknown>` force audit, readiness, restore, graph, and TUI consumers to repeat ad hoc shape checks. That weakens type safety around the highest-traffic data contract without changing the fact that imported snapshots and bundles remain untrusted JSON at runtime.

---

## Requirements

- R1. Keep `DiscoveredItem` as the exported shared evidence contract used by scanners, snapshots, bundles, graph, audit, restore, readiness, report, and TUI flows.
- R2. Preserve the existing serialized evidence shape: no new runtime discriminants, no renamed fields, and no empty `value` or `metadata` objects added when the current code omits them.
- R3. Model each `EvidenceKind` as a TypeScript union variant with typed `value` and `metadata` where the repo already relies on known shapes.
- R4. Keep imported or legacy evidence defensive at JSON boundaries; stricter TypeScript types must not make old snapshots or `.hem` bundles crash because optional payload fields are absent.
- R5. Fix the existing `hem schema` evidence-kind drift so `extension` is represented wherever `EvidenceKind` is externally described.
- R6. Preserve current graph, diff, timeline, readiness, restore, audit, and TUI behavior while reducing casts and repeated `Record<string, unknown>` probing.

---

## Key Technical Decisions

- **Make the union type-only:** Use existing `kind` as the discriminator and do not add runtime tags. This preserves snapshot and bundle compatibility while improving compile-time narrowing.
- **Keep metadata extensible by kind:** Define known metadata keys for rich kinds such as `skill`, `hook`, `mcp_server`, `symlink`, and `unsupported`, but leave room for scanner-specific metadata where current scanners intentionally record extra facts.
- **Do not over-tighten raw config:** Keep `agent_config.value` broad because agent config files can be arbitrary JSON/TOML and are not a stable domain payload.
- **Separate public evidence from construction evidence:** Keep shared aliases for common fields such as parser, capture status, and common metadata, then use typed factories for known evidence kinds. Dynamic scanner paths may use a deliberately named loose construction type before returning the public `DiscoveredItem` union.
- **Preserve defensive external boundaries:** `src/store.ts`, `src/bundle.ts`, and `src/restore.ts` read serialized JSON from disk or archives, so they should validate or defensively inspect data instead of trusting the union blindly.
- **Keep `hem schema` compatible first:** Fix enum drift and document payload object shapes without switching to a breaking `oneOf` schema unless implementation proves external consumers can tolerate it.

---

## High-Level Technical Design

```mermaid
flowchart TB
  scanners[Scanner modules] --> evidence[DiscoveredItem union]
  evidence --> store[Snapshot store]
  evidence --> graph[Graph and diff]
  evidence --> audit[Audit and report]
  evidence --> readiness[Readiness and restore]
  evidence --> tui[TUI view models]
  store --> bundle[Bundle export/import]
  schema[hem schema] -.describes.-> evidence

  legacy[Legacy JSON evidence] --> store
  legacy --> bundle
  store -.defensive read.-> evidence
  bundle -.defensive read.-> evidence
```

The refactor changes the TypeScript shape of `DiscoveredItem`, not the serialized evidence object. Producer code should emit the same JSON fields as today, while consumers gain safer narrowing when they branch on `item.kind`.

---

## Scope Boundaries

- In scope: `DiscoveredItem` union variants, typed value/metadata payloads, scanner producer alignment, consumer compile fixes, schema enum drift, and compatibility tests.
- In scope: documentation updates that clarify the evidence contract and runtime compatibility rule.
- Out of scope: changing `EvidenceKind` names, adding agent-specific evidence kinds, changing snapshot or bundle file layout, or redesigning restore policies.
- Out of scope: runtime schema validation for every imported evidence item beyond the defensive reads needed to preserve current behavior.

### Deferred to Follow-Up Work

- Full generated JSON Schema from TypeScript source, if manual schema drift becomes recurring.
- A deeper restore-policy matrix refactor that changes restore behavior rather than typing the existing contract.

---

## System-Wide Impact

This refactor touches a shared contract with external consumers. The affected surfaces include scanner output, snapshot `evidence.json`, `.hem` bundle `snapshot/evidence.json`, CLI JSON output, `hem schema`, graph/diff semantics, restore dry-run JSON, and TUI inventory rendering.

---

## Implementation Units

### U1. Evidence contract matrix and characterization coverage

- **Goal:** Pin the current serialized evidence contract before tightening TypeScript types.
- **Requirements:** R1, R2, R4, R5
- **Dependencies:** None
- **Files:** `tests/scan.test.ts`, `tests/store.test.ts`, `tests/bundle.test.ts`, `tests/doctor.test.ts`, `tests/analysis.test.ts`, `src/commands/schema.ts`
- **Approach:** Add targeted characterization assertions for representative evidence kinds: `mcp_server`, `permission`, `hook`, `skill`, `extension`, `env_key`, `symlink`, and `unsupported`. Assert that omitted `value` and `metadata` fields remain omitted when current scanners omit them. Add schema coverage that catches `EvidenceKind` enum drift, including the existing missing `extension` case.
- **Execution note:** Start with characterization coverage before changing `src/types.ts`.
- **Patterns to follow:** Existing evidence fixture style in `tests/analysis.test.ts`, scanner assertions in `tests/scan.test.ts`, and bundle compatibility checks in `tests/bundle.test.ts`.
- **Test scenarios:**
  - A scanner result with an env key that intentionally omits raw secret values still has no `value` field.
  - A skill item with known metadata keeps current metadata keys and does not gain empty payload fields.
  - A representative `.hem` bundle still contains `snapshot/evidence.json` as an array of evidence objects with existing field names.
  - `hem schema` includes every `EvidenceKind`, including `extension`.
- **Verification:** Existing behavior is pinned before the union refactor and failing tests identify accidental runtime shape drift.

### U2. Discriminated union and payload type aliases

- **Goal:** Replace the flat `DiscoveredItem` interface with a `kind`-discriminated union that preserves the exported contract name.
- **Requirements:** R1, R2, R3
- **Dependencies:** U1
- **Files:** `src/types.ts`, `tests/analysis.test.ts`, `tests/doctor.test.ts`, `tests/tui.test.tsx`
- **Approach:** Introduce shared aliases for common fields that consumers index today, including evidence parser, capture status, common metadata, and a base item shape. Define variants for each current `EvidenceKind` on top of that base. Add value aliases for known payloads such as MCP servers, permissions, hooks, env keys, and unsupported states. Add metadata aliases for scanner-known facts while keeping kind metadata extensible where current scanner behavior is intentionally open.
- **Patterns to follow:** Existing stable domain types in `src/types.ts`; avoid moving policy decisions out of `src/policy.ts`.
- **Test scenarios:**
  - Test fixture builders can still create each evidence kind without unsafe casts.
  - Narrowing on `item.kind === "mcp_server"` exposes MCP payload fields as optional typed fields.
  - Narrowing on `item.kind === "env_key"` does not require a secret value payload.
  - `agent_config` can still carry arbitrary parsed config without pretending it has a fixed domain schema.
  - Existing `Pick<DiscoveredItem, ...>` and indexed aliases such as parser/capture status continue to compile through shared common-field aliases.
- **Verification:** Typecheck failures move from broad `unknown` casts toward intentional optional-field handling.

### U3. Scanner producer alignment

- **Goal:** Update scanner constructors and helpers so emitted evidence conforms to the new union without changing emitted JSON.
- **Requirements:** R1, R2, R3
- **Dependencies:** U2
- **Files:** `src/scanners/base.ts`, `src/scanners/filesystem.ts`, `src/scanners/codex.ts`, `src/scanners/cursor.ts`, `src/scanners/opencode.ts`, `src/scanners/pi.ts`, `tests/scan.test.ts`
- **Approach:** Add typed construction helpers for the known high-value kinds (`mcp_server`, `permission`, `hook`, `skill`, `extension`, `env_key`, `symlink`, `unsupported`). Keep a clearly named loose construction helper for dynamic `ScanTarget.kind` paths and JSON passthrough cases, with the cast localized at the helper boundary rather than repeated across scanner modules. Do not try to make dynamic target construction fully generic if it makes scanner code harder to read.
- **Patterns to follow:** `createScannerBase()` in `src/scanners/base.ts`, `baseItem()` in `src/scanners/filesystem.ts`, and the standard-kind preference from `docs/plans/2026-06-08-003-refactor-cursor-scanner-docs-plan.md`.
- **Test scenarios:**
  - Filesystem MCP server extraction emits typed MCP values while preserving existing JSON fields.
  - Cursor hook extraction keeps hook metadata such as event name, category, priority, and executable status.
  - Pi extension evidence compiles as the `extension` variant and appears in schema-backed tests.
  - Parse failures and unsupported team/cloud hook blind spots still produce explicit evidence items.
  - Dynamic scanner target helpers keep their compatibility behavior while localizing any unavoidable union cast to one helper.
- **Verification:** Scanner tests pass without broad casts at construction sites except intentional generic fallbacks.

### U4. Consumer narrowing and defensive reads

- **Goal:** Replace repeated ad hoc casts in core consumers with safe kind narrowing while preserving behavior for absent or legacy payloads.
- **Requirements:** R3, R4, R6
- **Dependencies:** U2, U3
- **Files:** `src/audit.ts`, `src/diff.ts`, `src/graph.ts`, `src/readiness.ts`, `src/report.ts`, `src/provenance.ts`, `tests/analysis.test.ts`, `tests/doctor.test.ts`
- **Approach:** Update consumers that branch on `item.kind` to use union narrowing for known payloads. Keep defensive `typeof` checks anywhere data may come from imported snapshots, bundles, or older store files. Preserve `src/graph.ts` behavior where captured evidence with `value` keeps that value as the graph effective value.
- **Patterns to follow:** Current guard-heavy style in `src/audit.ts` and `src/readiness.ts`; do not replace boundary guards with non-null assertions.
- **Test scenarios:**
  - Permission wildcard audit still detects wildcard rules from permission payloads.
  - MCP readiness still classifies `command`, `url`, `args`, and `envKeys` from optional MCP values.
  - Graph/diff output is unchanged for captured evidence values and unsupported fallback states.
  - Legacy evidence with missing MCP `value` does not crash audit or readiness.
- **Verification:** Analysis and doctor tests pass with fewer unsafe casts and no behavior deltas.

### U5. External contract and restore/bundle compatibility

- **Goal:** Keep persisted and exported evidence compatible after the type refactor.
- **Requirements:** R2, R4, R5, R6
- **Dependencies:** U2, U3, U4
- **Files:** `src/store.ts`, `src/bundle.ts`, `src/restore.ts`, `src/commands/schema.ts`, `tests/store.test.ts`, `tests/bundle.test.ts`, `tests/restore.test.ts`, `tests/cli.test.ts`
- **Approach:** Treat store reads, bundle imports, and restore dry-run parsing as JSON boundaries. Keep tolerant parsing and validation where required fields are checked separately. Update `hem schema` to include `extension` and to describe current payload object compatibility without making a breaking schema-form change.
- **Patterns to follow:** Existing bundle import and readiness flow in `src/bundle.ts`; existing restore plan parsing in `src/restore.ts`.
- **Test scenarios:**
  - Existing-style snapshot evidence JSON can still be read and used after the refactor.
  - Bundle import of loose legacy evidence still produces readiness output instead of trusting payload fields blindly.
  - Restore dry-run parsing accepts current `currentState` and `targetState` evidence payloads.
  - `hem schema` emits the corrected evidence kind enum and remains compatible with existing object-shaped `value` and `metadata`.
- **Verification:** Store, bundle, restore, and CLI tests cover persisted JSON, archive JSON, dry-run JSON, and schema JSON.

### U6. TUI model cleanup and documentation

- **Goal:** Update UI-facing projections and docs so typed evidence is understandable without leaking scanner internals into Ink views.
- **Requirements:** R1, R3, R6
- **Dependencies:** U4, U5
- **Files:** `src/tui/components/AgentDetailViewModel.ts`, `src/tui/components/TimelineViewModel.ts`, `src/tui/components/TuiFormatters.ts`, `src/tui/components/Sidebar.tsx`, `src/tui/components/ProfileViewModel.ts`, `tests/tui.test.tsx`, `ARCHITECTURE.md`, `docs/bundle-format.md`
- **Approach:** Keep Ink views receiving view models or narrow `Pick<DiscoveredItem, ...>` shapes. Update model helpers to use typed metadata where they currently inspect keys such as `builtIn`, `sourceRoot`, `disabled`, and `enabled`. Document that `DiscoveredItem` is a discriminated TypeScript contract while the serialized JSON format remains stable.
- **Patterns to follow:** Existing TUI model boundary in `src/tui/components/*ViewModel.ts`; `docs/plans/2026-06-08-004-feat-inline-source-root-labels-plan.md` keeps source-root handling in model/formatter code rather than scanner-specific UI code.
- **Test scenarios:**
  - Agent Detail still renders Skills, MCP Servers, Hooks, Env Keys, and Instructions with existing status labels.
  - Timeline Current Setup still counts each evidence kind correctly through `Pick<DiscoveredItem, "kind">` style inputs.
  - Source-root formatting still handles optional metadata without requiring every item to carry that metadata.
  - Documentation states that snapshots and bundles remain compatible at the JSON field level.
- **Verification:** TUI tests pass and docs describe the new type boundary without implying a runtime format migration.

---

## Acceptance Examples

- AE1. Given a Cursor MCP server item with a command and args, when readiness analyzes evidence, then it reads typed optional MCP fields and emits the same availability report as before.
- AE2. Given a `.env` key evidence item with no raw secret value, when the scanner serializes evidence, then `value` remains absent and audit/readiness still identify the key by `name` or safe metadata.
- AE3. Given an old `.hem` bundle whose MCP evidence lacks a typed payload, when import or dry-run reads it, then Hem handles it defensively instead of assuming the stricter union shape.
- AE4. Given `hem schema`, when an external consumer reads the evidence kind enum, then `extension` is present and the schema remains compatible with object-shaped `value` and `metadata`.

---

## Risks & Dependencies

- **Runtime shape drift:** Adding empty payload objects or new tags would break compatibility assumptions for snapshots, bundles, and CLI JSON. Characterization tests in U1 mitigate this.
- **False type safety at JSON boundaries:** Imported evidence remains untrusted even after the union exists. U4 and U5 preserve defensive reads at boundary modules.
- **Schema precision trade-off:** A precise `oneOf` schema could be attractive but may break external consumers. This plan keeps schema compatibility first.
- **Test fixture churn:** Strict variants can make current `Partial<DiscoveredItem>` fixtures noisy. Shared test builders should absorb defaults rather than scattering casts.

---

## Sources & Research

- `ARCHITECTURE.md` defines `DiscoveredItem` as the central scanner output and durable domain contract.
- `PLAN.md` keeps restore policy mapping as active product architecture, so this refactor should not change restore behavior.
- `src/types.ts` contains the current flat `DiscoveredItem` interface and all evidence kind definitions.
- `src/scanners/base.ts` and `src/scanners/filesystem.ts` are the shared evidence construction choke points.
- `src/audit.ts`, `src/readiness.ts`, `src/graph.ts`, and `src/restore.ts` are the highest-risk consumers because they inspect optional payloads.
- `src/commands/schema.ts` is an external contract surface and currently omits `extension` from its evidence kind enum.
- `tests/scan.test.ts`, `tests/analysis.test.ts`, `tests/bundle.test.ts`, `tests/doctor.test.ts`, `tests/store.test.ts`, `tests/restore.test.ts`, and `tests/tui.test.tsx` are the main regression surfaces.
