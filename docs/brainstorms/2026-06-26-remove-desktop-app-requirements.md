---
title: "refactor: Remove active desktop app surface"
type: requirements
date: 2026-06-26
topic: remove-desktop-app
---

# refactor: Remove active desktop app surface

## Summary

Gandalf should remove the active desktop app surface and refocus the current repository on the Go CLI and Bubble Tea TUI. Desktop remains a possible future product direction, but it should no longer be built, checked, or described as part of the current MVP path. The implementation plan also removes the deprecated Rust stack entirely because future desktop work may not use Tauri or Rust.

---

## Problem Frame

The repository currently presents a split product posture. Architecture docs say Go CLI and TUI work is canonical, but the Tauri desktop transition path still remains in the workspace, scripts, CI, and current product language. That carrying cost is not aligned with the near-term priority: making CLI/TUI setup capture, restore, bundle, timeline, and trust semantics reliable.

This decision supersedes the prior boundary in `docs/plans/2026-06-26-003-refactor-go-binary-distribution-plan.md` that kept desktop Rust out of the Go distribution cutover.

---

## Key Decisions

- **Remove active desktop code now.** Keeping the desktop app around as a transition path preserves build and validation surface for a product direction that is no longer near-term.
- **Remove Rust as an active stack.** Keeping deprecated Rust crates implies a transition path that the product no longer wants to preserve.
- **Keep future desktop thinking, but demote it.** Product docs may still acknowledge desktop as a possible later surface after core CLI/TUI semantics are stronger.
- **Do not remove all frontend tooling.** The landing site and its Bun-based tooling stay unless a later plan separately removes or replaces them.

---

## Requirements

**Repository surface**

- R1. The repository no longer contains an active `apps/desktop` product app or Tauri desktop runtime.
- R2. Rust workspace membership, deprecated Rust crates, and Rust toolchain metadata are removed from active repository surfaces.
- R3. Root package scripts no longer build, typecheck, test, develop, or package the desktop app.

**Validation and CI**

- R4. CI no longer installs desktop-specific dependencies or runs desktop-specific checks.
- R5. Local verification scripts no longer require `gandalf-desktop` checks to validate the supported product path.
- R6. Supported checks emphasize Go CLI, Go engine, Bubble Tea TUI, installer, release, and landing-site surfaces that remain active.

**Product and architecture docs**

- R7. `ARCHITECTURE.md` describes Gandalf's active runtime as Go CLI, Go engine, and Bubble Tea TUI without a desktop transition box.
- R8. `README.md` repository layout and development instructions no longer present the desktop app as a current contributor workflow.
- R9. `PRODUCT.md` demotes desktop from current MVP ownership to a future/deferred direction where needed.
- R10. `CONCEPTS.md` no longer defines shared concepts in terms of desktop and CLI clients reading the same store APIs.

**Historical material**

- R11. Historical brainstorms, plans, and solution notes may continue to mention desktop when they are clearly historical.
- R12. Active docs should not point planners or contributors toward desktop implementation work as a next step.

---

## Scope Boundaries

### In Scope

- Removing `apps/desktop` and its Tauri/Rust app surface.
- Removing deprecated Rust engine and CLI crates plus Cargo workspace metadata.
- Removing desktop build, test, and dev commands from current workspace metadata.
- Removing desktop-specific CI and local verification requirements.
- Updating current product, architecture, README, and concept language so CLI/TUI are the active focus.

### Outside This Work

- Removing `apps/landing` or all Bun tooling.
- Rewriting historical plans, brainstorms, solution notes, and dogfood reports purely to erase old desktop references.
- Designing the future desktop product shape.
- Changing the Go CLI/TUI behavior except where verification scripts or docs mention desktop.

---

## Acceptance Examples

- AE1. **Covers R1, R3.** Given a contributor runs root workspace commands, when those commands execute, then they do not enter `apps/desktop` or call Tauri desktop scripts.
- AE2. **Covers R4, R5.** Given CI and local review verification run, when the supported product path is checked, then missing desktop code does not fail the run.
- AE3. **Covers R7, R8.** Given a new contributor reads the architecture and README, when they look for current runtime and repository layout, then they see Go CLI, Go engine, Bubble Tea TUI, and landing site as active surfaces.
- AE4. **Covers R9, R11.** Given a future planner reads product docs, when desktop is mentioned, then it is framed as deferred or historical rather than current MVP ownership.

---

## Success Criteria

- Repository search for active desktop commands and Tauri workspace references finds no current build, CI, or contributor workflow dependency.
- The supported verification story remains green without desktop code.
- Current docs consistently direct near-term effort toward CLI/TUI.

---

## Dependencies / Assumptions

- The Go CLI and Bubble Tea TUI are the current supported runtime surfaces.
- Desktop can be revisited later as a new product effort rather than preserved as a transition path.
- Landing-site frontend tooling remains useful independently of the desktop app.

---

## Sources / Research

- `ARCHITECTURE.md` currently names `cmd/gandalf`, `internal/gandalfcore`, and `internal/tui` as canonical, while still retaining a desktop transition path.
- `Cargo.toml` currently includes `apps/desktop/src-tauri` as a Rust workspace member.
- `package.json` currently includes desktop build, typecheck, test, development, and packaging scripts.
- `.github/workflows/ci.yml` currently includes desktop-specific setup and checks.
- `README.md` currently lists `apps/desktop` and desktop development commands.
- `CONCEPTS.md` currently defines `Store` in terms of desktop and CLI clients sharing store APIs.
- `docs/plans/2026-06-26-003-refactor-go-binary-distribution-plan.md` previously excluded Tauri desktop removal from the Go distribution cutover.
