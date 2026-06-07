/**
 * Command pattern interface for hem CLI.
 *
 * Each CLI command implements the Command interface and is registered
 * in a Map by name. The top-level dispatcher in cli.ts looks up the
 * command by args[0] and delegates to execute().
 */

import type { RuntimeOptions } from "../cli-shared.js";

export interface CommandContext {
  /** Raw CLI arguments (after process.argv.slice(2)) */
  args: string[];
  /** Parsed runtime paths */
  options: RuntimeOptions;
}

export interface Command {
  /** Command name as it appears in CLI args (e.g. "scan", "snapshot") */
  readonly name: string;
  /** Short description for help text */
  readonly description: string;
  /** Execute the command and return the exit code */
  execute(ctx: CommandContext): Promise<number>;
}