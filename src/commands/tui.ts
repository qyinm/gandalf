/**
 * Command-pattern implementation of the `tui` CLI command.
 *
 * Launches the full interactive TUI dashboard when run as:
 *   snaptailor tui
 *
 * The dashboard provides keyboard-navigable menu access to all
 * snaptailor commands without needing to remember CLI flags.
 */

import React from "react";
import { renderComponent } from "../tui/index.js";
import { runtimeOptions } from "../cli-shared.js";
import type { Command, CommandContext } from "./index.js";

export const tuiCommand: Command = {
  name: "tui",
  description:
    "Launch interactive TUI dashboard with keyboard-navigable command menu",

  async execute(ctx: CommandContext): Promise<number> {
    const options = runtimeOptions(ctx.args.slice(1));

    // Dynamically import Dashboard — React/Ink only paid on `tui` command
    const { default: Dashboard } = await import(
      "../tui/components/Dashboard.js"
    );

    return renderComponent(() => React.createElement(Dashboard, { options }));
  },
};

export default tuiCommand;