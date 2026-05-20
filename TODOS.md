# snaptailor Tasks

Direction: **Reproducible AI Coding Agent Environment**

Priority order within each section.

## v0.3: Restore Policy Matrix (active)

- [ ] **P0** Map `restorePolicy` per evidence kind in `src/types.ts`:
  - `agent_instruction` → `full_content_supported`
  - `mcp_server` → `structured_fields_only`
  - `permission` → `structured_fields_only`
  - `skill` → `full_content_supported`
  - `hook` → `structured_fields_only`
  - `env_key` → `key_inventory_only`
  - `symlink` → `not_supported`
  - `unsupported` → `not_supported`
- [ ] **P0** Wire restore policies into the evidence pipeline so snapshots carry accurate restore metadata.
- [ ] **P1** Implement per-kind content capture in bundle export:
  - `full_content_supported`: read and include actual file bytes
  - `structured_fields_only`: include parsed fields, omit raw values
  - `key_inventory_only`: key names only
  - `not_supported`: warn during export
- [ ] **P1** Add restore policy validation: fail bundle export if `not_supported` items would silently lose data.

## v0.3: Content Bundles as Default

- [ ] **P1** Flip default: `bundle export` includes content by default.
- [ ] **P1** Add `--metadata-only` flag as the opt-in for metadata-only bundles.
- [ ] **P1** Bundle size reporting and warnings for large bundles (>50MB).
- [ ] **P2** Deprecate `--experimental` requirement for content inclusion once per-kind policies are in place.

## v0.3: Cross-Machine Restore

- [ ] **P1** Home directory abstraction:
  - Store paths as `{home}/.claude/settings.json` in bundle manifest
  - Resolve `{home}` to `$HOME` on restore
- [ ] **P1** MCP binary path detection and mismatch warning (`npx`, `uvx`, local bins).
- [ ] **P1** Restore dry-run with machine-specific diff report.
- [ ] **P2** OS-aware path normalization (macOS `/Users/` ↔ Linux `/home/`).
- [ ] **P2** Cross-machine dogfood: export on macOS, import on Linux.

## v0.3: Bundle Security

- [ ] **P2** Bundle signature: HMAC-SHA256 on manifest + content.
- [ ] **P2** `snaptailor bundle verify <file.stailor>` command.
- [ ] **P3** Trust-on-first-use key management.
- [ ] **P3** Quarantine mode: imports are inspected before content is applied.

## Housekeeping

- [ ] **P3** Remove untracked `true` file.
- [ ] **P3** GitHub repo topics 설정.
- [ ] **P3** GitHub release 생성 (v0.1.0 기반).

## Deferred

- Team sharing / cloud sync.
- Desktop UI (TUI/GUI).
- Windsurf / Copilot scanner plugins.
- HMAC-based secret fingerprinting (for env values in bundles).
- Marketplace or skill registry.
