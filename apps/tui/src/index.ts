/**
 * TUI entry point — renders command output using Ink or Clack.
 *
 * This module provides render functions used by each command's execute()
 * when `--tui` is active. It delegates to the appropriate component
 * based on command type and data shape.
 */

export { detectTuiMode, isTui, isInkMode, isClackMode } from "./tui-mode.js";
export type { TuiMode, TuiOptions } from "./tui-mode.js";

// ── Ink renderer entry ─────────────────────────────────────────
// Ink requires React. We dynamically import it so non-TUI code
// paths never pay the React/Ink import cost.

/**
 * Render a component factory through Ink and return the exit code.
 *
 * For use in .ts files (no JSX transform): pass a factory function that
 * creates the React element.
 *
 * Usage:
 * ```ts
 * import { renderComponent } from "../tui/index.js";
 * import React from "react";
 * const exitCode = await renderComponent(
 *   () => React.createElement(ScanView, { evidence })
 * );
 * ```
 */
export async function renderComponent(
  factory: () => React.ReactElement
): Promise<number> {
  try {
    const { render } = await import("ink");
    const { waitUntilExit } = render(factory());
    await waitUntilExit();
    return 0;
  } catch (err) {
    process.stderr.write(
      `TUI render error: ${err instanceof Error ? err.message : String(err)}\n`
    );
    return 1;
  }
}