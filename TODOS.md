# hem TUI TODO ✅ COMPLETE

> Ink (React) + Clack interactive prompts for hem CLI

---

## All phases complete!

| Phase | Status | Commit | Files |
|-------|--------|--------|-------|
| **Phase 0**: Foundation | ✅ | `196e4b2` | tsconfig, deps, tui-mode.ts, index.ts (tty/--tui/--json detection) |
| **Phase 1**: Clack Wizards | ✅ | `583c459` | bundle-export, bundle-import, restore-confirm, snapshot-create |
| **Phase 2**: Ink Viewers | ✅ | `585187c` | ScanView, AuditView, DiffView, SnapshotList, ProvenanceView, ReportPreview |
| **Phase 4**: DX + Wiring | ✅ | `02f0b54` | SimpleTable, all commands wired to --tui, smoke test |
| **Phase 3**: Full Dashboard | ✅ | `d44644c` | Dashboard, ErrorPage, tui command, keyboard nav |

## How to use

```bash
# Launch interactive TUI dashboard
hem tui

# Or use --tui flag on any command for rich output
hem scan --project . --tui
hem diff baseline current --project . --tui
hem audit --tui
hem bundle export --tui
hem restore --tui

# Standard CLI still works exactly as before
hem scan --project .
hem scan --project . --json
```
