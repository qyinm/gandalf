import type { TimelineUndoPlan } from "../../timeline-undo.js";
import type { AgentId, TimelineChangedSurface, TimelineEntry, TimelineRestoreReadiness } from "../../types.js";
import type { TimelineCorruptEvent } from "../../store.js";

export interface TimelineRowModel {
  id: string;
  shortId: string;
  observedAt: string;
  eventKind: string;
  readiness: TimelineRestoreReadiness;
  agentScope: string;
  title: string;
  selected: boolean;
}

export interface TimelineDetailModel {
  id: string;
  title: string;
  eventKind: string;
  readiness: TimelineRestoreReadiness;
  confidence: string;
  beforeSnapshotName: string;
  afterSnapshotName: string;
  daemonRunId: string;
  counts: string;
  highlights: string[];
  writableSurfaces: TimelineChangedSurface[];
  observeOnlySurfaces: TimelineChangedSurface[];
}

export interface TimelineUndoPreviewModel {
  title: string;
  writesFiles: "no";
  writableItems: Array<{
    action: string;
    path: string;
    serverName: string;
  }>;
  observeOnlySurfaces: TimelineChangedSurface[];
  emptyWritableMessage?: string;
}

export interface TimelineViewModel {
  filterLabel: string;
  emptyMessage?: string;
  emptyCommand?: string;
  corruptWarning?: string;
  rows: TimelineRowModel[];
  selectedEntry?: TimelineDetailModel;
  undoPreview?: TimelineUndoPreviewModel;
}

export function buildTimelineViewModel(input: {
  entries: TimelineEntry[];
  selectedIndex: number;
  agentFilter: AgentId | null;
  corruptEvents?: TimelineCorruptEvent[];
  undoPlan?: TimelineUndoPlan | null;
}): TimelineViewModel {
  const selectedIndex = clampIndex(input.selectedIndex, input.entries.length);
  const selected = input.entries[selectedIndex];
  const corruptCount = input.corruptEvents?.length ?? 0;

  return {
    filterLabel: input.agentFilter ?? "All agents",
    emptyMessage: input.entries.length === 0 ? "No timeline entries yet." : undefined,
    emptyCommand: input.entries.length === 0 ? "hem daemon start --project ." : undefined,
    corruptWarning: corruptCount > 0
      ? `${corruptCount} corrupt timeline event${corruptCount === 1 ? "" : "s"} skipped`
      : undefined,
    rows: input.entries.map((entry, index) => timelineRowModel(entry, index === selectedIndex)),
    selectedEntry: selected ? timelineDetailModel(selected) : undefined,
    undoPreview: input.undoPlan ? timelineUndoPreviewModel(input.undoPlan) : undefined
  };
}

export function timelineRowModel(entry: TimelineEntry, selected: boolean): TimelineRowModel {
  return {
    id: entry.id,
    shortId: entry.id.slice(0, 8),
    observedAt: entry.observedAt,
    eventKind: entry.eventKind,
    readiness: entry.restoreReadiness,
    agentScope: timelineAgentScope(entry),
    title: entry.title,
    selected
  };
}

export function timelineDetailModel(entry: TimelineEntry): TimelineDetailModel {
  const writableSurfaces = entry.changedSurfaces.filter((surface) => surface.restorable);
  const observeOnlySurfaces = entry.changedSurfaces.filter((surface) => surface.observeOnly || !surface.restorable);

  return {
    id: entry.id,
    title: entry.title,
    eventKind: entry.eventKind,
    readiness: entry.restoreReadiness,
    confidence: `${entry.confidence}: ${entry.confidenceReason}`,
    beforeSnapshotName: entry.beforeSnapshotName ?? "-",
    afterSnapshotName: entry.afterSnapshotName,
    daemonRunId: entry.daemonRunId,
    counts: `${entry.evidenceCount} evidence, ${entry.graphNodeCount} graph nodes, ${entry.auditFindingCount} findings`,
    highlights: entry.changes.highlights,
    writableSurfaces,
    observeOnlySurfaces
  };
}

export function timelineUndoPreviewModel(plan: TimelineUndoPlan): TimelineUndoPreviewModel {
  return {
    title: plan.title,
    writesFiles: "no",
    writableItems: plan.writableItems.map((item) => ({
      action: item.action,
      path: item.path,
      serverName: item.serverName
    })),
    observeOnlySurfaces: plan.observeOnlySurfaces,
    emptyWritableMessage: plan.writableItems.length === 0
      ? "No writable MCP undo items for this event."
      : undefined
  };
}

export function timelineAgentScope(entry: TimelineEntry): string {
  if (entry.agent) return entry.agent;
  if (entry.agents.length === 0) return "all";
  return entry.agents.join(",");
}

export function clampTimelineIndex(index: number, entries: TimelineEntry[]): number {
  return clampIndex(index, entries.length);
}

function clampIndex(index: number, length: number): number {
  if (length <= 0) return 0;
  return Math.min(Math.max(0, index), length - 1);
}
