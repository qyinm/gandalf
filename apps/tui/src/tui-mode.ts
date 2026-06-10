/**
 * TUI mode detection and utilities.
 *
 * Determines whether the CLI should render output via Ink/Clack
 * instead of plain text. Must be safe to call in non-interactive
 * (piped/CI) environments.
 */

/**
 * TUI rendering mode.
 * - "ink": full Ink React render (scan/diff/audit viewers)
 * - "clack": interactive Clack prompts (wizards)
 * - "none": plain text (default, also used when piped)
 */
export type TuiMode = "ink" | "clack" | "none";

export interface TuiOptions {
  /** Resolved rendering mode */
  mode: TuiMode;
  /** Whether stdout is a TTY */
  isInteractive: boolean;
  /** Whether --tui was explicitly passed */
  tuiFlag: boolean;
  /** Whether --no-tui was explicitly passed (overrides --tui) */
  noTuiFlag: boolean;
}

function hasFlag(args: string[], flag: string): boolean {
  return args.includes(flag);
}

/**
 * Detect whether the current terminal supports TUI output.
 *
 * Returns "none" and logs nothing when:
 * - stdout is piped (not a TTY)
 * - --no-tui is set
 * - --json is set (JSON output wins)
 */
export function detectTuiMode(args: string[]): TuiOptions {
  const isInteractive = process.stdout.isTTY === true;
  const jsonFlag = hasFlag(args, "--json");
  const tuiFlag = hasFlag(args, "--tui");
  const noTuiFlag = hasFlag(args, "--no-tui");

  let mode: TuiMode = "none";

  if (noTuiFlag) {
    mode = "none";
  } else if (jsonFlag) {
    // --json always wins over TUI
    mode = "none";
  } else if (tuiFlag && !isInteractive) {
    // User asked for TUI but stdout is piped — fall back
    mode = "none";
  } else if (tuiFlag) {
    // Full TUI mode (Ink for viewers, Clack for wizards)
    mode = "ink";
  }

  return { mode, isInteractive, tuiFlag, noTuiFlag };
}

/**
 * Convenience check: is TUI mode active?
 */
export function isTui(args: string[]): boolean {
  return detectTuiMode(args).mode !== "none";
}

/**
 * Convenience check: is Ink rendering mode active?
 */
export function isInkMode(args: string[]): boolean {
  return detectTuiMode(args).mode === "ink";
}

/**
 * Determine if a command should use Clack (interactive) mode.
 * Clack mode is active when --tui is set AND the command has a wizard.
 * Returns true only for wizard-eligible commands.
 *
 * Accepts pre-resolved TuiOptions to stay in sync with detectTuiMode.
 */
export function isClackMode(options: TuiOptions, commandName: string): boolean {
  if (options.mode !== "clack" && options.mode !== "ink") return false;

  // Only these commands have Clack wizards
  const wizardCommands = ["bundle", "restore", "snapshot"];
  return wizardCommands.includes(commandName);
}

/**
 * Extract a flag value from args with a fallback prompt label.
 * Returns the value if present, or undefined.
 */
export function tuiFlagValue(args: string[], flag: string): string | undefined {
  const idx = args.indexOf(flag);
  if (idx === -1 || idx >= args.length - 1) return undefined;
  return args[idx + 1];
}
