---
date: 2026-06-26
topic: gandalf-full-rename
---

# Gandalf Full Rename Requirements

## Summary

Rename the service from Hem to Gandalf as a full product identity change. Gandalf is the wizard for AI agent setup: it guides setup changes, captures working agent environments into portable containers, and makes those containers rollbackable across machines.

---

## Problem Frame

Hem currently reads as a safety and rollback tool for Codex setup, but the intended product concept is broader and more memorable: a wizard that manages AI agent setup states. The new name should make the product feel like the guide and operator for agent environments, while the container and rollback concepts remain the durable object model behind the scenes.

The team chose a clean break over a long-lived compatibility brand. That makes the rename clearer, but it raises trust requirements because this product manages saved setup state and rollback artifacts.

---

## Key Decisions

- **Use Gandalf as the service name.** Gandalf is the selected public product identity, despite the stronger IP and searchability risk than a coined name.
- **Make the wizard the lead metaphor.** The product story should lead with Gandalf guiding and managing AI agent setup, while containers describe the saved, portable, rollbackable state.
- **Prefer clean transition over alias compatibility.** Hem should not remain an active brand or long-term command surface after the rename.
- **Treat old artifacts intentionally.** Existing Hem-named stores, bundles, package names, and documentation must be migrated, rejected with clear guidance, or explicitly deprecated.

---

## Requirements

**Brand and positioning**

- R1. Gandalf must replace Hem as the product name across user-facing product surfaces.
- R2. The one-line positioning must describe Gandalf as the wizard for AI agent setup, not as a generic backup, scanner, or container manager.
- R3. Product copy must keep containers as managed setup states that can be moved across machines and rolled back.
- R4. The rename must not dilute the trust contract around preview, rollback, explicit writes, and local setup safety.

**Command and distribution surfaces**

- R5. The primary CLI command must become `gandalf`.
- R6. Hem must not remain a long-term active CLI alias.
- R7. Package, repository, module, app, and website names should move to Gandalf-aligned names unless a distribution constraint blocks the move.
- R8. Any blocked distribution rename must be documented as a temporary exception with user-visible naming that still presents Gandalf as the product.

**State, bundle, and migration policy**

- R9. The persistent store path must move away from Hem naming as part of the clean transition.
- R10. The portable bundle format must move away from `.hem` naming unless planning identifies a hard compatibility reason to keep it temporarily.
- R11. Existing Hem-named local state must not be silently ignored.
- R12. Existing Hem-named local state must either migrate through an explicit transition path or fail with clear recovery guidance.
- R13. Existing Hem-named bundles must either import through an explicit transition path or fail with clear recovery guidance.
- R14. The transition path must avoid presenting old Hem artifacts as current Gandalf artifacts until they have been migrated or accepted by the user.

**Documentation and communication**

- R15. Documentation must explain that Hem was the previous name and Gandalf is the current product name.
- R16. Documentation must state whether old Hem commands, stores, and bundles are supported, unsupported, or migration-only.
- R17. Landing and README copy must use the wizard-first story consistently.
- R18. Release notes or migration docs must call out breaking changes before users run a renamed command.

---

## Scope Boundaries

### Deferred for later

- Trademark, domain, package, and repository availability verification.
- Final legal review of Gandalf as a public commercial product name.
- Visual brand design, mascot treatment, iconography, and landing-page redesign.

### Outside this rename

- Reconsidering non-Gandalf alternatives such as Spellbox, Ward, or Runebox.
- Expanding product scope beyond agent setup containers, portability, and rollback.
- Reworking the underlying restore or bundle engine except where naming and migration require it.

---

## Acceptance Examples

- AE1. **Covers R1, R5, R17.** Given a new user lands on the product README, when they scan the first screen and install command, then they see Gandalf as the product and `gandalf` as the command.
- AE2. **Covers R11, R12.** Given a returning user has existing Hem local state, when they run Gandalf for the first time, then the product does not silently start with an empty state.
- AE3. **Covers R10, R13.** Given a user has an old Hem bundle, when they try to use it with Gandalf, then the product either migrates/imports it through an explicit path or explains why it cannot.
- AE4. **Covers R2, R3, R4.** Given a user reads the positioning copy, when they compare Gandalf to a generic backup tool, then they understand Gandalf as a wizard managing rollbackable AI agent setup containers.

---

## Success Criteria

- A reader can describe Gandalf in one sentence as the wizard for rollbackable AI agent setup containers.
- A planner can enumerate the rename surfaces without inventing whether `hem` remains an alias.
- Returning users with Hem artifacts encounter an intentional migration or rejection path, not accidental data loss.
- User-facing copy consistently uses Gandalf as the current name and Hem only as historical migration context.

---

## Dependencies / Assumptions

- The team accepts the brand and IP risk of using Gandalf as a public product name until legal or availability checks say otherwise.
- The clean break decision is intentional: compatibility can exist only as an explicit migration path, not as an ongoing Hem-branded product surface.
- Planning will determine the precise migration mechanics for stores, bundles, packages, and module paths.

---

## Outstanding Questions

### Deferred to Planning

- Which existing Hem artifacts should be migrated automatically, migrated behind a prompt, or rejected with instructions?
- What is the final Gandalf bundle extension and store path?
- Which package, module, repository, and website names are available and safe to claim?
- How long, if at all, should old `hem` commands remain as deprecation stubs before removal?

---

## Sources / Research

- `PRODUCT.md` defines Hem as a Codex setup rollback product and broader agent setup branch manager.
- `README.md` documents the current Hem command, package, bundle, and trust contract surfaces.
- `CONCEPTS.md` records the existing Hem domain vocabulary around Trust Contract, Evidence, Snapshot, Store, and Restore.
