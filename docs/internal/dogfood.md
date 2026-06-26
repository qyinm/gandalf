# Dogfood Runs

Date: 2026-05-12

Historical note: this run used the old Node CLI path. Current dogfood commands use the Go binary from `bin/gandalf`; see `docs/dogfood.md`.

Command shape:

```bash
GANDALF_STORE=/tmp/gandalf-dogfood/store node dist/src/cli.js scan --project <project> --json
GANDALF_STORE=/tmp/gandalf-dogfood/store node dist/src/cli.js report current --project <project> --out /tmp/gandalf-dogfood/reports/<name>-report.md
```

The store was redirected to `/tmp/gandalf-dogfood/store` so dogfood did not write to the user's real `~/.gandalf` store.

## Results

| Project | Scan JSON | Markdown report | Evidence | Findings | Blind spots |
|---|---|---|---:|---:|---:|
| Gandalf | `/tmp/gandalf-dogfood/gandalf-scan.json` | `/tmp/gandalf-dogfood/reports/gandalf-report.md` | 243 | 108 | 3 |
| MirrorNote | `/tmp/gandalf-dogfood/mirrornote-scan.json` | `/tmp/gandalf-dogfood/reports/mirrornote-report.md` | 247 | 111 | 3 |
| DuckDocs | `/tmp/gandalf-dogfood/duckdocs-scan.json` | `/tmp/gandalf-dogfood/reports/duckdocs-report.md` | 245 | 108 | 3 |

## Observations

- The read-only scan/report path completed on three real project directories.
- The first-run evidence volume is high, so v0.1 needs severity filtering and concise default output.
- Reports are large because home-level skills and project context both produce many evidence items.
- No restore/import/share path was needed for dogfood.
