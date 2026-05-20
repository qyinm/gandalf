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
// Ink requires React. We dynamically import both so non-TUI code
// paths never pay the React/Ink import cost.

import type { ReactElement } from "react";

/**
 * Render data through an Ink React component and return the exit code.
 *
 * Callers pass a pre-built React element. Ink handles terminal rendering.
 *
 * Usage:
 * ```ts
 * import { renderInk } from "../tui/index.js";
 * const exitCode = await renderInk(<ScanView data={state} />);
 * ```
 */
export async function renderInk(element: ReactElement): Promise<number> {
  try {
    const { render } = await import("ink");
    const { waitUntilExit } = render(element);
    await waitUntilExit();
    return 0;
  } catch (err) {
    process.stderr.write(
      `TUI render error: ${err instanceof Error ? err.message : String(err)}\n`
    );
    return 1;
  }
}
