---
title: Keep TUI boot status cheap and vendor-cache-free
date: 2026-09-04
category: docs/solutions/architecture-patterns
module: Gandalf TUI boot
problem_type: architecture_pattern
component: tooling
severity: high
applies_when:
  - Loading Changes-First Home on gandalf tui startup
  - Building baseline.Status for header chips or Home drift
  - Discovering Codex skills from the user home directory
  - Listing snapshots for Saves without needing full snapshot bodies
tags: [tui, boot, baseline, scan, snapshot, codex, plugin-cache, latency]
---

# Keep TUI boot status cheap and vendor-cache-free

## Context

`gandalf tui` blocked first paint on `ScanProject` plus `baseline.BuildStatus`.
On a real machine that path was dominated by capturing current skill file
contents, walking Codex `~/.codex/plugins/cache`, and decoding full snapshot
JSON for every save.

Changes-First Home needs graph diffs against the latest user baseline. It does
not need current file bodies, vendor plugin-cache SKILL.md trees, Timeline, or
Saves listing.

## Guidance

- Status reads set `CaptureContent=false`. Snapshot write paths still capture.
- Reuse one `ScanProject` evidence set via `BuildStatusFromEvidence`.
- List snapshots from manifests / diff-state, not full snapshot documents.
- Paint Home after scan + baseline status, then load Timeline and Saves.
- Treat `~/.codex/plugins/cache` as a vendor download tree. Installed plugins
  come from `config.toml`; user skills come from `~/.codex/skills`.

## Measured result

On this machine (gandalf repo + real `~/.gandalf`):

- full TUI boot replica 1.640s -> 50ms
- Home-ready replica 50ms after skipping plugin-cache skill walks
