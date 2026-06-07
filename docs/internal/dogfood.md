# Dogfood Runs

Date: 2026-05-12

Command shape:

```bash
HEM_STORE=/tmp/hem-dogfood/store node dist/src/cli.js scan --project <project> --json
HEM_STORE=/tmp/hem-dogfood/store node dist/src/cli.js report current --project <project> --out /tmp/hem-dogfood/reports/<name>-report.md
```

The store was redirected to `/tmp/hem-dogfood/store` so dogfood did not write to the user's real `~/.hem` store.

## Results

| Project | Scan JSON | Markdown report | Evidence | Findings | Blind spots |
|---|---|---|---:|---:|---:|
| Hem | `/tmp/hem-dogfood/hem-scan.json` | `/tmp/hem-dogfood/reports/hem-report.md` | 243 | 108 | 3 |
| MirrorNote | `/tmp/hem-dogfood/mirrornote-scan.json` | `/tmp/hem-dogfood/reports/mirrornote-report.md` | 247 | 111 | 3 |
| DuckDocs | `/tmp/hem-dogfood/duckdocs-scan.json` | `/tmp/hem-dogfood/reports/duckdocs-report.md` | 245 | 108 | 3 |

## Observations

- The read-only scan/report path completed on three real project directories.
- The first-run evidence volume is high, so v0.1 needs severity filtering and concise default output.
- Reports are large because home-level skills and project context both produce many evidence items.
- No restore/import/share path was needed for dogfood.

