---
title: "Land Gandalf rename brief"
type: docs
date: 2026-06-26
origin: docs/brainstorms/2026-06-26-gandalf-full-rename-requirements.md
---

# Land Gandalf Rename Brief

## Summary

Document the Gandalf rename decision and make the new product vocabulary discoverable for planning and follow-on implementation.

---

## Problem Frame

The service is moving from Gandalf toward Gandalf as a wizard-first product identity. The decision needs a durable requirements brief and glossary entries before implementation planning or code rename work begins.

---

## Requirements

- R1. The requirements brief must capture the full product rename from Gandalf to Gandalf.
- R2. The brief must preserve the clean-break decision for the old Gandalf command and brand.
- R3. The brief must require intentional migration or clear rejection for old Gandalf state and bundles.
- R4. The glossary must define Gandalf and Setup Container in product terms.
- R5. The PR body for this change must use only `Summary`, `Why`, and `Changes` sections and must not mention the engineering pipeline by name.

---

## Key Technical Decisions

- **Docs-only landing:** This change should land only the brainstorm brief and glossary vocabulary, leaving command, package, store, and bundle renames to a follow-up implementation plan.
- **PR body constraint as release hygiene:** The PR description should be human-facing and avoid implementation-process branding.

---

## Implementation Units

### U1. Requirements brief

- **Goal:** Add a requirements document for the Gandalf full rename.
- **Files:** `docs/brainstorms/2026-06-26-gandalf-full-rename-requirements.md`
- **Test Scenarios:** Verify the document has frontmatter, stable requirement IDs, scope boundaries, acceptance examples, and no absolute file paths.
- **Verification:** Manual document review.

### U2. Glossary vocabulary

- **Goal:** Add the resolved product identity terms to the shared glossary.
- **Files:** `CONCEPTS.md`
- **Test Scenarios:** Verify the new terms are product concepts rather than implementation details.
- **Verification:** Manual document review.

### U3. PR body formatting

- **Goal:** Ensure the final pull request body has only `Summary`, `Why`, and `Changes` sections and excludes process-branding text.
- **Files:** Pull request body only.
- **Test Scenarios:** Inspect the PR body before or after creation.
- **Verification:** `gh pr view --json body` when a PR exists.

---

## Scope Boundaries

- Do not rename CLI commands, modules, packages, desktop identifiers, store paths, or bundle extensions in this change.
- Do not keep Gandalf as a long-term compatibility brand.
- Do not perform trademark or domain availability research in this change.

---

## Documentation / Operational Notes

The eventual PR body should use this shape:

```markdown
## Summary

## Why

## Changes
```

The body should not include pipeline tool names.
