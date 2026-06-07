import { randomUUID } from "node:crypto";

import { captureCurrentState, type CurrentState } from "./current-state.js";
import { diffGraphs, type GraphDiff, type SemanticChange } from "./diff.js";
import {
  appendTimelineEntry,
  latestTimelineEntry,
  readSnapshot,
  writeSnapshot
} from "./store.js";
import type { RuntimeOptions } from "./cli-shared.js";
import type { AgentId, TimelineChangedSurface, TimelineEntry, TimelineRestoreReadiness } from "./types.js";

export interface CaptureTimelineOptions {
  daemonRunId: string;
  snapshotName?: string;
  title?: string;
  skipUnchanged?: boolean;
}

export interface CaptureTimelineResult {
  written: boolean;
  entry?: TimelineEntry;
  state: CurrentState;
  diff?: GraphDiff;
  skippedReason?: "unchanged";
}

export async function captureTimelineSnapshot(
  options: RuntimeOptions,
  captureOptions: CaptureTimelineOptions
): Promise<CaptureTimelineResult> {
  const previous = await latestTimelineEntry(options.storeDir, {
    projectPath: options.projectPath,
    agent: options.agent
  });
  const snapshotName = captureOptions.snapshotName ?? timelineSnapshotName(captureOptions.daemonRunId, options.agent);
  const state = await captureCurrentState(options, snapshotName);

  let diff: GraphDiff | undefined;
  let diffError: string | undefined;
  if (previous) {
    try {
      const previousSnapshot = await readSnapshot(options.storeDir, previous.afterSnapshotName, previous.agent);
      diff = diffGraphs(previousSnapshot.graph, state.snapshot.graph);
    } catch (error) {
      diffError = error instanceof Error ? error.message : String(error);
      diff = undefined;
    }
  }

  if (
    previous &&
    diff &&
    captureOptions.skipUnchanged &&
    diff.semanticChanges.length === 0 &&
    diff.rawSourceChanges.length === 0
  ) {
    return { written: false, state, diff, skippedReason: "unchanged" };
  }

  const changedSurfaces = changedSurfacesForDiff(diff);
  const restoreReadiness = restoreReadinessFor(changedSurfaces);
  const entry: TimelineEntry = {
    schemaVersion: "0.1",
    id: shortId(),
    source: "daemon",
    eventKind: eventKindFor(previous, diff),
    title: captureOptions.title ?? titleForDiff(diff, options.agent),
    projectPath: state.snapshot.manifest.projectPath,
    ...(options.agent ? { agent: options.agent } : {}),
    agents: agentsForState(state),
    ...(previous ? { beforeSnapshotName: previous.afterSnapshotName } : {}),
    afterSnapshotName: snapshotName,
    daemonRunId: captureOptions.daemonRunId,
    createdAt: new Date().toISOString(),
    observedAt: state.snapshot.manifest.createdAt,
    changedSurfaces,
    restoreReadiness,
    confidence: confidenceFor(diff, changedSurfaces, diffError),
    confidenceReason: confidenceReasonFor(diff, changedSurfaces, diffError),
    evidenceCount: state.snapshot.evidence.length,
    graphNodeCount: state.snapshot.graph.length,
    auditFindingCount: state.snapshot.auditFindings.length,
    changes: {
      ...(previous ? { previousEntryId: previous.id, previousSnapshotName: previous.afterSnapshotName } : {}),
      hasChanges: !diff || diff.semanticChanges.length > 0 || diff.rawSourceChanges.length > 0,
      semanticChangeCount: diff?.semanticChanges.length ?? 0,
      rawSourceChangeCount: diff?.rawSourceChanges.length ?? 0,
      highlights: highlightsForDiff(diff)
    }
  };

  await writeSnapshot(options.storeDir, state.snapshot, options.agent);
  await appendTimelineEntry(options.storeDir, entry);

  return { written: true, entry, state, diff };
}

export function timelineSnapshotName(runId: string, agent?: AgentId): string {
  const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
  return ["daemon", runId, agent, timestamp, shortId()].filter(Boolean).join("-");
}

function shortId(): string {
  return randomUUID().replace(/-/g, "").slice(0, 8);
}

function agentsForState(state: CurrentState): AgentId[] {
  return [...new Set(state.snapshot.evidence.map((item) => item.agent))].sort();
}

function titleForDiff(diff: GraphDiff | undefined, agent?: AgentId): string {
  if (!diff) {
    return scopedTitle("baseline setup", agent);
  }

  if (diff.semanticChanges.length === 0 && diff.rawSourceChanges.length === 0) {
    return scopedTitle("unchanged setup", agent);
  }

  const first = [...diff.semanticChanges].sort((left, right) =>
    priorityForChange(left).localeCompare(priorityForChange(right))
  )[0];

  if (!first) {
    return scopedTitle("update setup files", agent);
  }

  switch (first.code) {
    case "MCP_ADDED":
      return scopedTitle(`add ${first.entityName} mcp`, agent);
    case "MCP_REMOVED":
      return scopedTitle(`remove ${first.entityName} mcp`, agent);
    case "MCP_CHANGED":
      return scopedTitle(`update ${first.entityName} mcp`, agent);
    case "ENV_KEY_ADDED":
      return scopedTitle(`add ${first.entityName} env key`, agent);
    case "ENV_KEY_REMOVED":
      return scopedTitle(`remove ${first.entityName} env key`, agent);
    case "PERMISSION_WILDCARD_ADDED":
      return scopedTitle("update permissions", agent);
    case "SKILL_EXECUTABLE_APPEARED":
      return scopedTitle(`update ${first.entityName} skill`, agent);
    case "UNSUPPORTED_STATE_CHANGED":
      return scopedTitle("update unsupported setup", agent);
  }
}

function changedSurfacesForDiff(diff: GraphDiff | undefined): TimelineChangedSurface[] {
  if (!diff) return [];

  const surfaces: TimelineChangedSurface[] = [];
  const semanticSourcePaths = new Set<string>();
  for (const change of diff.semanticChanges) {
    const kind = timelineSurfaceKind(change);
    const path = typeof change.details.sourcePath === "string" ? change.details.sourcePath : "unknown";
    semanticSourcePaths.add(path);
    const restorable = kind === "mcp_server" && path.endsWith(".mcp.json");
    surfaces.push({
      kind,
      changeType: change.code,
      path,
      entityName: change.entityName,
      restorable,
      observeOnly: !restorable,
      ...(change.before === undefined ? {} : { before: change.before }),
      ...(change.after === undefined ? {} : { after: change.after })
    });
  }

  for (const change of diff.rawSourceChanges) {
    if (semanticSourcePaths.has(change.sourcePath)) continue;
    surfaces.push({
      kind: "other",
      changeType: `RAW_${change.status.toUpperCase()}`,
      path: change.sourcePath,
      restorable: false,
      observeOnly: true
    });
  }
  return surfaces;
}

function eventKindFor(previous: TimelineEntry | undefined, diff: GraphDiff | undefined): TimelineEntry["eventKind"] {
  if (!previous) return "baseline";
  if (diff && diff.semanticChanges.length === 0 && diff.rawSourceChanges.length === 0) return "unchanged";
  return "setup_changed";
}

function timelineSurfaceKind(change: SemanticChange): TimelineChangedSurface["kind"] {
  if (change.entityKind === "mcp_server") return "mcp_server";
  if (change.entityKind === "skill") return "skill";
  if (change.entityKind === "permission") return "permission";
  if (change.entityKind === "hook") return "hook";
  if (change.entityKind === "env_key") return "env_key";
  if (change.entityKind === "unsupported") return "unsupported";
  return "other";
}

function restoreReadinessFor(surfaces: TimelineChangedSurface[]): TimelineRestoreReadiness {
  if (surfaces.length === 0) return "observe-only";
  const restorable = surfaces.filter((surface) => surface.restorable).length;
  if (restorable === surfaces.length) return "full";
  if (restorable > 0) return "partial";
  return "observe-only";
}

function confidenceFor(diff: GraphDiff | undefined, surfaces: TimelineChangedSurface[], diffError?: string): TimelineEntry["confidence"] {
  if (diffError) return "low";
  if (!diff) return "high";
  if (surfaces.some((surface) => surface.path === "unknown")) return "medium";
  return "high";
}

function confidenceReasonFor(diff: GraphDiff | undefined, surfaces: TimelineChangedSurface[], diffError?: string): string {
  if (diffError) return `previous snapshot could not be diffed: ${diffError}`;
  if (!diff) return "first daemon baseline";
  if (surfaces.length === 0) return "no semantic or raw source changes";
  if (surfaces.some((surface) => surface.path === "unknown")) return "some changes lacked source path metadata";
  return "derived from snapshot graph diff";
}

function priorityForChange(change: SemanticChange): string {
  if (change.code.startsWith("MCP_")) return `0-${change.entityName}`;
  if (change.code === "SKILL_EXECUTABLE_APPEARED") return `1-${change.entityName}`;
  if (change.code === "PERMISSION_WILDCARD_ADDED") return `2-${change.entityName}`;
  if (change.code.startsWith("ENV_KEY_")) return `4-${change.entityName}`;
  return `5-${change.entityName}`;
}

function scopedTitle(title: string, agent?: AgentId): string {
  return agent ? `${title} for ${agentLabel(agent)}` : title;
}

function agentLabel(agent: AgentId): string {
  if (agent === "claude-code") return "Claude Code";
  if (agent === "codex") return "Codex";
  if (agent === "cursor") return "Cursor";
  if (agent === "opencode") return "OpenCode";
  if (agent === "pi-agent") return "Pi Agent";
  if (agent === "project") return "Project";
  return "Unknown";
}

function highlightsForDiff(diff: GraphDiff | undefined): string[] {
  if (!diff) return [];

  const highlights: string[] = [];
  for (const change of diff.semanticChanges.slice(0, 5)) {
    highlights.push(`${change.code}: ${change.entityName}`);
  }
  for (const change of diff.rawSourceChanges.slice(0, Math.max(0, 8 - highlights.length))) {
    highlights.push(`${change.status}: ${change.sourcePath}`);
  }
  return highlights;
}
