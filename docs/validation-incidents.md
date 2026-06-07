# Validation Incidents

Status: 10 real operator incidents collected from prior local project memory.

These incidents are memory-derived from real work on this workstation. They are not invented seed patterns and they are not customer interviews. They are sufficient for the v0.1 implementation validation gate because the initial target operator is a developer running coding agents, gstack skills, local automation, and project-specific agent context across repos.

## Incident Set

| # | Incident | Agent/tooling surface | Suspected changed state | Hem classification | Source |
|---:|---|---|---|---|---|
| 1 | GitHub publishing commands failed even though repo state was correct. | GitHub CLI used by coding agents | `gh` auth or sandbox/network transport state diverged from expected repo state | unsupported | `MEMORY.md:109-114` |
| 2 | Standup automation memory write failed because `$CODEX_HOME` was empty and path construction targeted `/automations`. | Codex automation / environment | Runtime env var missing despite default Codex path existing | unsupported | `MEMORY.md:297-304` |
| 3 | Evidence summary drifted into intent/future-work language when commit evidence was insufficient. | Agent memory / prompt workflow | Agent used narrative fill-in instead of strict evidence sources | unknown | `MEMORY.md:301-305` |
| 4 | Swift build/test failed in sandbox due module-cache permission errors under `~/.cache`, not Swift code. | Local build commands invoked by coding agents | Toolchain cache path/permission state differed from expected environment | unsupported | `MEMORY.md:1320-1322` |
| 5 | Linked helper verification failed under sandbox restrictions during native build. | Linked repo helper / build command | Filesystem access boundary differed from expected linked helper path | unsupported | `MEMORY.md:1327-1328` |
| 6 | `gstack-slug` history lookup failed with `unknown` or zsh nomatch. | gstack project lookup | Project slug/source metadata could not resolve a usable project artifact path | unsupported | `MEMORY.md:1398-1401` |
| 7 | Future agent assumed an `antx` scope decision was accepted because repo docs/reviews were overread as final user choice. | Project context / agent memory | Planning artifact semantics drifted into accepted state without explicit user confirmation | unknown | `MEMORY.md:1400-1402` |
| 8 | Duplicate `office-hours` skills appeared because multiple install roots exposed the same internal skill name. | Skills across home and vendored runtime trees | Skill folders existed under multiple roots with colliding `name:` metadata | captured | `MEMORY.md:1438-1443` |
| 9 | Duplicate-skill cleanup targeted the wrong path by conflating `~/.agents` with vendored `/Users/hippoo/gstack/.agents`. | Skill install paths | Similar-looking skill roots caused wrong source path interpretation | captured | `MEMORY.md:1445-1448` |
| 10 | gstack session setup emitted `Operation not permitted` / `nice(5) failed` side effects. | gstack session preflight / sandbox | Sandbox permission limits changed expected setup behavior without blocking core reasoning | unsupported | `MEMORY.md:1490-1494` |

## Classification Summary

| Classification | Count | Notes |
|---|---:|---|
| captured | 2 | v0.1 scanner can see skill roots / path collisions as metadata evidence. |
| redacted | 0 | No memory-derived incident depended mainly on raw secret value drift. |
| remote | 0 | Network/auth failures are recorded as unsupported in v0.1 because Hem does not perform live auth/network probes. |
| unsupported | 6 | These expose known v0.1 reproducibility gaps: env vars, sandbox permissions, build cache, linked repo boundaries, and gstack slug resolution. |
| unknown | 2 | These require higher-level behavior/provenance analysis beyond current scanner semantics. |

## Product Implications

- v0.1 should keep unsupported-state visibility prominent; most real incidents are not fully capturable by local file scanning.
- Skill root collisions are a strong early captured case and should stay in the audit/risk vocabulary.
- Runtime environment variables, sandbox permissions, and auth status should be considered for a future `doctor` or optional live probe mode, not default read-only scan.
- Agent-memory drift and overreading planning artifacts need report/provenance UX, not restore mechanics.

