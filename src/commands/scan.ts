/**
 * Command-pattern implementation of the `scan` CLI command.
 *
 * Reads evidence from the project and outputs a human-readable scan summary,
 * an --explain breakdown of considered paths, or --json structured output.
 */

import React from "react";
import type { AuditFinding } from "../types.js";
import type { ScanResult } from "../scan.js";
import { hasFlag, json } from "../cli-shared.js";
import type { CommandContext, Command } from "./index.js";
import { captureCurrentState, type CurrentState } from "../current-state.js";
import { isInkMode, renderComponent } from "../tui/index.js";

// ── Renderers ─────────────────────────────────────────────────

function displayAgent(agent: string): string {
  if (agent === "claude-code") return "Claude Code";
  if (agent === "codex") return "Codex";
  if (agent === "cursor") return "Cursor";
  if (agent === "project") return "Project";
  return agent;
}

function renderScanText(state: CurrentState): string {
  const lines = [
    "hem scan",
    "",
    `Read-only: ${state.scan.trust.readOnly ? "yes" : "no"}`,
    `Network: ${state.scan.trust.network}`,
    `Commands executed: ${state.scan.trust.commandsExecuted.length}`,
    `Writes: ${state.scan.trust.storeWriteLocation}/index only`,
    "",
    "Detected agents"
  ];

  const agents = new Set(state.scan.evidence.map((item) => item.agent));
  if (agents.size === 0) {
    lines.push("  none");
  } else {
    for (const agent of [...agents].sort()) {
      const items = state.scan.evidence.filter((item) => item.agent === agent);
      const scopes = new Set(items.map((item) => item.scope));
      lines.push(`  ${displayAgent(agent)}  ${[...scopes].sort().join(" + ")} state found`);
    }
  }

  lines.push("", "High-signal findings");
  if (state.snapshot.auditFindings.length === 0) {
    lines.push("  none");
  } else {
    for (const finding of state.snapshot.auditFindings.slice(0, 8)) {
      lines.push(`  ${finding.severity.toUpperCase()}  ${finding.problem}`);
    }
  }

  lines.push("", "Blind spots");
  for (const blindSpot of state.scan.blindSpots) {
    lines.push(`  ${blindSpot}`);
  }

  lines.push("", "Next", "  hem snapshot create --name baseline --metadata-only --project .");
  return `${lines.join("\n")}\n`;
}

function renderExplainText(state: CurrentState): string {
  const paths = [...new Set(state.scan.evidence.map((item) => item.sourcePath))].sort();
  const lines = [
    renderScanText(state).trimEnd(),
    "",
    "Paths considered"
  ];

  if (paths.length === 0) {
    lines.push("  none found");
  } else {
    for (const sourcePath of paths) {
      lines.push(`  ${sourcePath}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

// ── Command export ─────────────────────────────────────────────

export const scanCommand: Command = {
  name: "scan",
  description: "Scan project for agent configuration and emit evidence inventory",
  async execute(ctx: CommandContext): Promise<number> {
    const state = await captureCurrentState(ctx.options);

    // --json: always wins
    if (hasFlag(ctx.args, "--json")) {
      process.stdout.write(json(state));
      return 0;
    }

    // --tui: render with Ink
    if (isInkMode(ctx.args)) {
      const { default: ScanView } = await import("../tui/components/ScanView.js");
      return renderComponent(
        () => React.createElement(ScanView, {
          evidence: state.scan.evidence,
          auditFindings: state.snapshot.auditFindings,
          blindSpots: state.scan.blindSpots,
          readOnly: state.scan.trust.readOnly,
        })
      );
    }

    // Plain text
    process.stdout.write(
      hasFlag(ctx.args, "--explain")
        ? renderExplainText(state)
        : renderScanText(state)
    );
    return 0;
  }
};
