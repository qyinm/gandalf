import { buildReadinessReport, formatReadinessSummaryLines } from "@qxinm/gandalf-core/readiness.js";
import { scanProject } from "@qxinm/gandalf-core/scan.js";
import { json } from "../cli-shared.js";
import type { Command, CommandContext } from "./index.js";
import type { ReadinessReport } from "@qxinm/gandalf-core/types.js";

function renderReadinessText(report: ReadinessReport): string {
  const lines = [
    "gandalf doctor",
    "",
    `Target platform: ${report.targetPlatform}`,
    ""
  ];
  lines.push(...formatReadinessSummaryLines(report, { maxItems: 10, includeFixes: true, includeActions: true }));

  if (report.items.length === 0) {
    lines.push("", "No readiness issues found.");
  }

  return `${lines.join("\n")}\n`;
}

export const doctorCommand: Command = {
  name: "doctor",
  description: "Check local Mac readiness for agent setup portability",
  async execute(ctx: CommandContext): Promise<number> {
    const scan = await scanProject(ctx.options);
    const report = buildReadinessReport(scan.evidence, {
      sourceHomeDir: ctx.options.homeDir,
      targetEvidence: scan.evidence
    });

    if (ctx.args.includes("--json")) {
      process.stdout.write(json(report));
    } else {
      process.stdout.write(renderReadinessText(report));
    }

    return report.items.some((item) => item.category === "blocked") ? 1 : 0;
  }
};
