import { findTimelineEntry, type TimelineCorruptEvent } from "./store.js";
import type { TimelineChangedSurface, TimelineEntry } from "./types.js";

export interface TimelineUndoItem {
  action: "add" | "remove" | "update";
  kind: "mcp_server";
  path: string;
  serverName: string;
  targetValue?: unknown;
  currentValue?: unknown;
}

export interface TimelineUndoPlan {
  entryId: string;
  title: string;
  dryRun: true;
  writesFiles: false;
  restoreReadiness: TimelineEntry["restoreReadiness"];
  targetSnapshotName?: string;
  currentSnapshotName: string;
  writableItems: TimelineUndoItem[];
  observeOnlySurfaces: TimelineChangedSurface[];
}

export async function buildTimelineUndoPlan(
  storeDir: string,
  ref: string,
  options: { onCorruptEntry?: (event: TimelineCorruptEvent) => void } = {}
): Promise<TimelineUndoPlan> {
  const entry = await findTimelineEntry(storeDir, ref, options);
  if (!entry) {
    throw new Error(`Timeline entry not found: ${ref}`);
  }

  const writableItems = entry.changedSurfaces
    .filter((surface) => surface.restorable && surface.kind === "mcp_server")
    .map(undoItemForMcpSurface);

  return {
    entryId: entry.id,
    title: `dry-run MCP undo: ${entry.title}`,
    dryRun: true,
    writesFiles: false,
    restoreReadiness: entry.restoreReadiness,
    targetSnapshotName: entry.beforeSnapshotName,
    currentSnapshotName: entry.afterSnapshotName,
    writableItems,
    observeOnlySurfaces: entry.changedSurfaces.filter((surface) => !surface.restorable)
  };
}

function undoItemForMcpSurface(surface: TimelineChangedSurface): TimelineUndoItem {
  const serverName = surface.entityName ?? "unknown";
  if (surface.changeType === "MCP_ADDED") {
    return {
      action: "remove",
      kind: "mcp_server",
      path: surface.path,
      serverName,
      currentValue: surface.after
    };
  }

  if (surface.changeType === "MCP_REMOVED") {
    return {
      action: "add",
      kind: "mcp_server",
      path: surface.path,
      serverName,
      targetValue: surface.before
    };
  }

  return {
    action: "update",
    kind: "mcp_server",
    path: surface.path,
    serverName,
    targetValue: surface.before,
    currentValue: surface.after
  };
}
