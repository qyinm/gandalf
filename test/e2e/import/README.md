# `gandalf import` E2E Test Suite

Opaque-box, requirement-driven E2E suite for the `gandalf import` subsystem
(multi-agent detection, secret templatization, precedence reconciliation,
safe CLI modes, interactive TUI wizard).

## Layout

| File | Scope |
|---|---|
| `e2e_helpers_test.go` | Sandbox creation (`makeSandbox`), CLI runner (`runCLI`), `manifest.Validate` assertion, `gandalf check --project-only --ci` InSync assertion |
| `tier1_features_test.go` | Tier 1: feature coverage (per-agent parsing, CLI flags, templatization, precedence, skills) |
| `tier2_boundaries_test.go` | Tier 2: boundary & corner cases (malformed JSON/TOML, symlinks, path traversal, env vars) |
| `tier3_interactions_test.go` | Tier 3: cross-feature interactions (multi-agent + secrets + flag combinations) |
| `tier4_realworld_test.go` | Tier 4: real-world scenarios asserting `manifest.Validate` 0 errors and `check --project-only` InSync=true after config alignment |

## Run

```bash
# Whole suite
go test -v -count=1 ./test/e2e/import/...

# By tier
go test -v -count=1 -run "TestTier1" ./test/e2e/import/...
go test -v -count=1 -run "TestTier2" ./test/e2e/import/...
go test -v -count=1 -run "TestTier3" ./test/e2e/import/...
go test -v -count=1 -run "TestTier4" ./test/e2e/import/...
```

## Conventions

- Tests are hermetic: every test builds its own project/home sandbox via
  `t.TempDir()` and never touches the real `$HOME`.
- Tests exercise the CLI (`runCLI`) and public importer API only; no coupling
  to private implementation details.
- The InSync assertion intentionally runs after `syncProjectAgentConfigs`:
  import produces the canonical manifest, and aligning project agent files
  (what `gandalf apply` does for local setups) is what brings the repository
  to `InSync = true`.
