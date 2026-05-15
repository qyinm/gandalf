# Dogfood Runs

Date: 2026-05-12 (initial), 2026-05-15 (re-run after symlink noise fix and TOML parser fix)

Command shape:

```bash
SNAPTAILOR_STORE=/tmp/snaptailor-dogfood/store node dist/src/cli.js scan --project <project> --json
SNAPTAILOR_STORE=/tmp/snaptailor-dogfood/store node dist/src/cli.js report current --project <project> --out /tmp/snaptailor-dogfood/reports/<name>-report.md
```

The store was redirected to `/tmp/snaptailor-dogfood/store` so dogfood did not write to the user's real `~/.snaptailor` store.

## Results (2026-05-15, post-fix)

| Project | Scan JSON | Markdown report | Evidence | Findings | Blind spots |
|---|---|---|---:|---:|---:|
| snaptailor | `/tmp/snaptailor-dogfood/snaptailor-scan.json` | `/tmp/snaptailor-dogfood/reports/snaptailor-report.md` | 283 | 1 | 3 |
| DuckDocs | `/tmp/snaptailor-dogfood/duckdocs-scan.json` | `/tmp/snaptailor-dogfood/reports/DuckDocs-report.md` | 283 | 1 | 3 |
| HyprDuck (replaced MirrorNote — workspace no longer present) | `/tmp/snaptailor-dogfood/hyprduck-scan.json` | `/tmp/snaptailor-dogfood/reports/HyprDuck-report.md` | 283 | 1 | 3 |

All three projects share the same user-level agent configuration and lack project-level agent config files, so they produce identical evidence.

Only finding: `MEDIUM SECRET_LIKE_VALUE_OMITTED` — expected, from a skill directory containing secret-like key names.

### Comparison with 2026-05-12 baseline

| Metric | Before | After | Delta |
|---|---:|---:|---:|
| Evidence items | 243-247 | 283 | +40 (better TOML parsing captures more keys) |
| Audit findings | 108-111 | **1** | -107 (symlink noise suppressed, parse failure fixed) |
| Blind spots | 3 | 3 | unchanged |

## Observations

- The symlink noise fix reduced findings from ~108 to 1, making the default scan output useful.
- The TOML parser fix allowed `~/.codex/config.toml` to be fully parsed (27 key-value pairs extracted), adding 40 more evidence items.
- Reports are still large (79KB) because the provenance section lists every evidence item. This is acceptable for v0.1 but could be summarized in a future iteration.
- No restore/import/share path was needed for dogfood.
