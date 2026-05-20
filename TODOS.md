# snaptailor Tasks

Direction: **Reproducible AI Coding Agent Environment**

Priority order within each section.

## ✅ v0.3: Restore Policy Matrix (done)

- [x] **P0** Map `restorePolicy` per evidence kind in `src/types.ts`
- [x] **P0** Wire restore policies into the evidence pipeline
- [x] **P1** Implement per-kind content capture in bundle export
- [x] **P1** Add restore policy validation: fail bundle export if `not_supported` items would silently lose data.

## ✅ v0.3: Content Bundles as Default (done)

- [x] **P1** Flip default: `bundle export` includes content by default.
- [x] **P1** Add `--metadata-only` flag as the opt-in for metadata-only bundles.
- [x] **P1** Bundle size reporting and warnings for large bundles (>50MB).
- [x] **P2** Deprecate `--experimental` requirement for content inclusion.

## v0.3: Cross-Machine Restore (next)

- [ ] **P1** Home directory abstraction:
  - Store paths as `{home}/.claude/settings.json` in bundle manifest
  - Resolve `{home}` to `$HOME` on restore
- [ ] **P1** MCP binary path detection and mismatch warning (`npx`, `uvx`, local bins).
- [ ] **P1** Restore dry-run with machine-specific diff report.
- [x] **P2** OS-aware path normalization (macOS `/Users/` ↔ Linux `/home/`).
- [ ] **P2** Cross-machine dogfood: export on macOS, import on Linux.

## v0.3: Bundle Security

- [ ] **P2** Bundle signature: HMAC-SHA256 on manifest + content.
- [ ] **P2** `snaptailor bundle verify <file.stailor>` command.
- [ ] **P3** Trust-on-first-use key management.
- [ ] **P3** Quarantine mode: imports are inspected before content is applied.

## Housekeeping

- [x] **P3** Remove untracked `true` file.
- [ ] **P3** GitHub repo topics 설정.
- [ ] **P3** GitHub release 생성 (v0.1.0 기반).

## Deferred

- Team sharing / cloud sync.
- Desktop UI (TUI/GUI).
- Windsurf / Copilot scanner plugins.
- HMAC-based secret fingerprinting (for env values in bundles).
- Marketplace or skill registry.
