import type { TimelineUndoPlan } from "@qxinm/gandalf-core/timeline-undo.js";
import type { AgentId, DiscoveredItem, TimelineChangedSurface, TimelineEntry, TimelineRestoreReadiness } from "@qxinm/gandalf-core/types.js";
import type { TimelineCorruptEvent } from "@qxinm/gandalf-core/store.js";
import {
  formatAgentLabel,
  formatAgentScope,
  formatInventoryNameWithSource,
  formatInventorySourceRoot,
  formatTimelineTimestamp
} from "./TuiFormatters.js";

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
  captureId: string;
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
  currentSetup: CurrentSetupSummaryModel;
  emptyMessage?: string;
  emptyCommand?: string;
  corruptWarning?: string;
  rows: TimelineRowModel[];
  selectedEntry?: TimelineDetailModel;
  undoPreview?: TimelineUndoPreviewModel;
}

export interface CurrentSetupSummaryModel {
  scopeLabel: string;
  agents: number;
  skills: number;
  mcpServers: number;
  hooks: number;
  permissions: number;
  envKeys: number;
  skillRows: string[];
  mcpServerRows: string[];
  hookRows: string[];
  envKeyRows: string[];
  instructions: string;
}

export type CurrentSetupInventorySection = "skill" | "mcp_server" | "hook" | "env_key";

export function buildTimelineViewModel(input: {
  entries: TimelineEntry[];
  selectedIndex: number;
  agentFilter: AgentId | null;
  evidence?: Pick<DiscoveredItem, "agent" | "id" | "kind" | "metadata" | "name" | "scope" | "sourcePath">[];
  corruptEvents?: TimelineCorruptEvent[];
  undoPlan?: TimelineUndoPlan | null;
  now?: Date;
}): TimelineViewModel {
  const selectedIndex = clampIndex(input.selectedIndex, input.entries.length);
  const selected = input.entries[selectedIndex];
  const corruptCount = input.corruptEvents?.length ?? 0;

  return {
    filterLabel: input.agentFilter ? formatAgentLabel(input.agentFilter) : "All agents",
    currentSetup: buildCurrentSetupSummaryModel({
      evidence: input.evidence ?? [],
      agentFilter: input.agentFilter
    }),
    emptyMessage: input.entries.length === 0 ? "No timeline entries yet." : undefined,
    emptyCommand: input.entries.length === 0 ? "Save a setup to start local history." : undefined,
    corruptWarning: corruptCount > 0
      ? `${corruptCount} corrupt timeline event${corruptCount === 1 ? "" : "s"} skipped`
      : undefined,
    rows: input.entries.map((entry, index) => timelineRowModel(entry, index === selectedIndex, input.now)),
    selectedEntry: selected ? timelineDetailModel(selected) : undefined,
    undoPreview: input.undoPlan ? timelineUndoPreviewModel(input.undoPlan) : undefined
  };
}

export function buildCurrentSetupSummaryModel(input: {
  evidence: Pick<DiscoveredItem, "agent" | "id" | "kind" | "metadata" | "name" | "scope" | "sourcePath">[];
  agentFilter: AgentId | null;
}): CurrentSetupSummaryModel {
  const evidence = input.agentFilter
    ? input.evidence.filter((item) => item.agent === input.agentFilter || item.agent === "project")
    : input.evidence;
  const instructionPaths = [...new Set(
    evidence
      .filter((item) => item.kind === "agent_instruction")
      .map((item) => item.sourcePath)
  )].sort();

  return {
    scopeLabel: input.agentFilter ? formatAgentLabel(input.agentFilter) : "All agents",
    agents: new Set(evidence.filter((item) => item.agent !== "project").map((item) => item.agent)).size,
    skills: countKind(evidence, "skill"),
    mcpServers: countKind(evidence, "mcp_server"),
    hooks: countKind(evidence, "hook"),
    permissions: countKind(evidence, "permission"),
    envKeys: countKind(evidence, "env_key"),
    skillRows: rowsForKind(evidence, "skill", input.agentFilter),
    mcpServerRows: rowsForKind(evidence, "mcp_server", input.agentFilter),
    hookRows: rowsForKind(evidence, "hook", input.agentFilter),
    envKeyRows: rowsForKind(evidence, "env_key", input.agentFilter),
    instructions: instructionPaths.length > 0 ? instructionPaths.slice(0, 3).join(", ") : "none"
  };
}

export function timelineRowModel(entry: TimelineEntry, selected: boolean, now?: Date): TimelineRowModel {
  return {
    id: entry.id,
    shortId: entry.id.slice(0, 8),
    observedAt: formatTimelineTimestamp(entry.observedAt, now),
    eventKind: entry.eventKind,
    readiness: entry.restoreReadiness,
    agentScope: timelineAgentScope(entry),
    title: entry.title,
    selected
  };
}

function countKind(evidence: Pick<DiscoveredItem, "kind">[], kind: DiscoveredItem["kind"]): number {
  return evidence.filter((item) => item.kind === kind).length;
}

function rowsForKind(
  evidence: Pick<DiscoveredItem, "agent" | "id" | "kind" | "metadata" | "name" | "scope" | "sourcePath">[],
  kind: DiscoveredItem["kind"],
  agentFilter: AgentId | null
): string[] {
  const rows = evidence
    .filter((item) => item.kind === kind)
    .map((item) => {
      const name = displayNameForItem(item);
      if (agentFilter) {
        return item.agent === "project" && !formatInventorySourceRoot(item) ? `${name} (project)` : name;
      }
      return `${formatAgentLabel(item.agent)}: ${name}`;
    });
  return [...new Set(rows)].sort();
}

function displayNameForItem(
  item: Pick<DiscoveredItem, "id" | "kind" | "metadata" | "name" | "scope" | "sourcePath">
): string {
  const suffix = item.scope === "managed" || item.metadata?.builtIn === true ? " (built-in)" : "";
  if (item.name) return formatInventoryNameWithSource(`${item.name}${suffix}`, item);
  const parts = item.sourcePath.split("/").filter(Boolean);
  const last = parts.at(-1);
  if (last && last !== "SKILL.md") return formatInventoryNameWithSource(`${last}${suffix}`, item);
  const parent = parts.at(-2);
  if (parent) return formatInventoryNameWithSource(`${parent}${suffix}`, item);
  return formatInventoryNameWithSource(`${item.id}${suffix}`, item);
}

export function currentSetupEmptyText(
  kind: DiscoveredItem["kind"]
): string {
  return `no ${kind.replace("_", " ")}s`;
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
    captureId: entry.captureId,
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
  return formatAgentScope(entry.agent, entry.agents);
}

export function clampTimelineIndex(index: number, entries: TimelineEntry[]): number {
  return clampIndex(index, entries.length);
}

function clampIndex(index: number, length: number): number {
  if (length <= 0) return 0;
  return Math.min(Math.max(0, index), length - 1);
}
