# Validation Incidents

Status: seed backlog created; real operator incidents still need collection before claiming product validation complete.

The v0.1 implementation can proceed with fixture-backed behavior, but this file must be replaced or extended with real target-operator incidents before the product claim is treated as validated.

## Incident Capture Template

For each incident:

- What changed in agent behavior?
- Which agent and project were involved?
- What files/settings changed?
- Could snaptailor detect it?
- Classification: captured, redacted, remote, unsupported, unknown.
- Time saved if semantic diff existed.

## Seed Incident Patterns To Validate

| # | Incident pattern | Expected snaptailor classification |
|---:|---|---|
| 1 | Project MCP server command changed and agent gained new tool behavior | captured |
| 2 | Project `CLAUDE.md` added instructions that override user memory | captured |
| 3 | Claude Code permission wildcard added in project settings | captured |
| 4 | Skill folder gained executable helper script | captured |
| 5 | Remote MCP server URL changed host | captured |
| 6 | Provider-side model routing changed without local config change | remote |
| 7 | Local `.env` value changed but raw value is omitted by policy | redacted |
| 8 | Cursor project MCP config changed but scanner only has metadata support | unsupported |
| 9 | Symlinked skill folder points outside allowed scan root | unsupported |
| 10 | Malformed Codex config caused agent startup fallback behavior | captured |

