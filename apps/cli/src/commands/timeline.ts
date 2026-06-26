import { hasFlag, json, valueAfter } from "../cli-shared.js";
import { formatSnapError } from "@qxinm/gandalf-core/errors.js";
import { findTimelineEntry, listTimelineEntries, type TimelineCorruptEvent } from "@qxinm/gandalf-core/store.js";
import { buildTimelineUndoPlan } from "@qxinm/gandalf-core/timeline-undo.js";
import type { TimelineEntry } from "@qxinm/gandalf-core/types.js";
import type { Command, CommandContext } from "./index.js";

export const timelineCommand: Command = {
  name: "timeline",
  description: "List and show local timeline entries",

  async execute(ctx: CommandContext): Promise<number> {
    const { args, options } = ctx;
    const sub = args[1] ?? "list";

    if (sub === "list") {
      const corruptEvents: TimelineCorruptEvent[] = [];
      const entries = await listTimelineEntries(options.storeDir, {
        agent: options.agent,
        projectPath: options.projectPath,
        limit: limitFromArgs(args),
        onCorruptEntry: (event) => corruptEvents.push(event)
      });
      reportCorruptEvents(corruptEvents);

      if (hasFlag(args, "--json")) {
        process.stdout.write(json(entries));
        return 0;
      }

      process.stdout.write(renderTimelineList(entries));
      return 0;
    }

    if (sub === "show") {
      const ref = args[2];
      if (!ref) {
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_TIMELINE_REF_REQUIRED",
            problem: "Timeline entry id or snapshot name is required.",
            cause: "`timeline show` was called without a reference.",
            fix: "Run `gandalf timeline list` and pass an entry id or snapshot name."
          })
        );
        return 1;
      }

      const corruptEvents: TimelineCorruptEvent[] = [];
      const entry = await findTimelineEntry(options.storeDir, ref, {
        onCorruptEntry: (event) => corruptEvents.push(event)
      });
      reportCorruptEvents(corruptEvents);
      if (!entry) {
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_TIMELINE_ENTRY_NOT_FOUND",
            problem: `Timeline entry "${ref}" not found.`,
            cause: "The reference does not match a timeline id or snapshot name.",
            fix: "Run `gandalf timeline list` to see available entries."
          })
        );
        return 1;
      }

      process.stdout.write(hasFlag(args, "--json") ? json(entry) : renderTimelineEntry(entry));
      return 0;
    }

    if (sub === "undo") {
      const ref = args[2];
      if (!ref) {
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_TIMELINE_REF_REQUIRED",
            problem: "Timeline entry id or snapshot name is required.",
            cause: "`timeline undo` was called without a reference.",
            fix: "Run `gandalf timeline list` and pass an entry id."
          })
        );
        return 1;
      }
      if (!hasFlag(args, "--dry-run")) {
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_TIMELINE_UNDO_DRY_RUN_REQUIRED",
            problem: "Timeline undo is dry-run only in P0.",
            cause: "`timeline undo` was called without `--dry-run`.",
            fix: "Run `gandalf timeline undo <id> --dry-run --json`."
          })
        );
        return 1;
      }

      const corruptEvents: TimelineCorruptEvent[] = [];
      let plan: Awaited<ReturnType<typeof buildTimelineUndoPlan>>;
      try {
        plan = await buildTimelineUndoPlan(options.storeDir, ref, {
          onCorruptEntry: (event) => corruptEvents.push(event)
        });
      } catch (error) {
        reportCorruptEvents(corruptEvents);
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_TIMELINE_ENTRY_NOT_FOUND",
            problem: `Timeline entry "${ref}" not found.`,
            cause: error instanceof Error ? error.message : "The reference does not match a readable timeline id or snapshot name.",
            fix: "Run `gandalf timeline list` to see available entries and any corrupt event warnings."
          })
        );
        return 1;
      }
      reportCorruptEvents(corruptEvents);
      process.stdout.write(hasFlag(args, "--json") ? json(plan) : renderTimelineUndoPlan(plan));
      return 0;
    }

    process.stderr.write(
      formatSnapError({
        code: "GANDALF_UNKNOWN_SUBCOMMAND",
        problem: `Unknown timeline subcommand: "${sub}".`,
        cause: "`timeline` was called with an unrecognized subcommand.",
        fix: "Use `list`, `show`, or `undo`. Run `gandalf --help` for details."
      })
    );
    return 1;
  }
};

function limitFromArgs(args: string[]): number | undefined {
  const raw = valueAfter(args, "--limit");
  if (!raw) return undefined;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : undefined;
}

function renderTimelineList(entries: TimelineEntry[]): string {
  if (entries.length === 0) {
    return "No timeline entries.\n";
  }

  const lines = ["gandalf timeline", ""];
  for (const entry of entries) {
    const scope = entry.agent ? ` ${entry.agent}` : "";
    lines.push(`${entry.id}  ${entry.observedAt}  ${entry.restoreReadiness}${scope}  ${entry.title}`);
  }
  return `${lines.join("\n")}\n`;
}

function renderTimelineEntry(entry: TimelineEntry): string {
  const lines = [
    entry.title,
    "",
    `id: ${entry.id}`,
    `event: ${entry.eventKind}`,
    `after snapshot: ${entry.afterSnapshotName}`,
    `before snapshot: ${entry.beforeSnapshotName ?? "-"}`,
    `observed: ${entry.observedAt}`,
    `source: ${entry.source}`,
    `capture: ${entry.captureId}`,
    `project: ${entry.projectPath}`,
    `agent: ${entry.agent ?? "all"}`,
    `restore readiness: ${entry.restoreReadiness}`,
    `confidence: ${entry.confidence} (${entry.confidenceReason})`,
    "",
    "Summary",
    `  evidence: ${entry.evidenceCount}`,
    `  graph nodes: ${entry.graphNodeCount}`,
    `  audit findings: ${entry.auditFindingCount}`,
    `  semantic changes: ${entry.changes.semanticChangeCount}`,
    `  raw source changes: ${entry.changes.rawSourceChangeCount}`
  ];

  if (entry.changes.previousSnapshotName) {
    lines.push(`  previous snapshot: ${entry.changes.previousSnapshotName}`);
  }

  if (entry.changes.highlights.length > 0) {
    lines.push("", "Highlights");
    for (const highlight of entry.changes.highlights) {
      lines.push(`  ${highlight}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

function renderTimelineUndoPlan(plan: Awaited<ReturnType<typeof buildTimelineUndoPlan>>): string {
  const lines = [
    plan.title,
    "",
    `entry: ${plan.entryId}`,
    `dry run: yes`,
    `writes files: no`,
    `restore readiness: ${plan.restoreReadiness}`,
    "",
    "MCP changes"
  ];

  if (plan.writableItems.length === 0) {
    lines.push("  none");
  } else {
    for (const item of plan.writableItems) {
      lines.push(`  ${item.action} ${item.serverName} in ${item.path}`);
    }
  }

  if (plan.observeOnlySurfaces.length > 0) {
    lines.push("", "Observe-only surfaces");
    for (const surface of plan.observeOnlySurfaces) {
      lines.push(`  ${surface.changeType}: ${surface.entityName ?? surface.path}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

function reportCorruptEvents(events: TimelineCorruptEvent[]): void {
  for (const event of events) {
    process.stderr.write(`Skipped corrupt timeline event: ${event.filePath} (${event.error})\n`);
  }
}

export default timelineCommand;
