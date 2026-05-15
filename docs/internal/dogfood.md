# Dogfood Runs

Date: 2026-05-12

Command shape:

```bash
SNAPTAILOR_STORE=/tmp/snaptailor-dogfood/store node dist/src/cli.js scan --project <project> --json
SNAPTAILOR_STORE=/tmp/snaptailor-dogfood/store node dist/src/cli.js report current --project <project> --out /tmp/snaptailor-dogfood/reports/<name>-report.md
```

The store was redirected to `/tmp/snaptailor-dogfood/store` so dogfood did not write to the user's real `~/.snaptailor` store.

## Results

| Project | Scan JSON | Markdown report | Evidence | Findings | Blind spots |
|---|---|---|---:|---:|---:|
| snaptailor | `/tmp/snaptailor-dogfood/snaptailor-scan.json` | `/tmp/snaptailor-dogfood/reports/snaptailor-report.md` | 243 | 108 | 3 |
| MirrorNote | `/tmp/snaptailor-dogfood/mirrornote-scan.json` | `/tmp/snaptailor-dogfood/reports/mirrornote-report.md` | 247 | 111 | 3 |
| DuckDocs | `/tmp/snaptailor-dogfood/duckdocs-scan.json` | `/tmp/snaptailor-dogfood/reports/duckdocs-report.md` | 245 | 108 | 3 |

## Observations

- The read-only scan/report path completed on three real project directories.
- The first-run evidence volume is high, so v0.1 needs severity filtering and concise default output.
- Reports are large because home-level skills and project context both produce many evidence items.
- No restore/import/share path was needed for dogfood.

