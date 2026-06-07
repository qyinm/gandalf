import React from "react";
import { Box, Text } from "ink";

import type { TimelineUndoPlan } from "../../timeline-undo.js";
import type { AgentId, TimelineChangedSurface, TimelineEntry } from "../../types.js";
import type { TimelineCorruptEvent } from "../../store.js";
import { buildTimelineViewModel } from "./TimelineViewModel.js";
import { padDisplay } from "./TuiFormatters.js";
import { NoTimelineEventsEmptyState } from "./TuiEmptyStates.js";

interface TimelineViewProps {
  entries: TimelineEntry[];
  selectedIndex: number;
  agentFilter: AgentId | null;
  corruptEvents?: TimelineCorruptEvent[];
  undoPlan?: TimelineUndoPlan | null;
  undoError?: string | null;
}

export default function TimelineView({
  entries,
  selectedIndex,
  agentFilter,
  corruptEvents = [],
  undoPlan,
  undoError
}: TimelineViewProps) {
  const model = buildTimelineViewModel({
    entries,
    selectedIndex,
    agentFilter,
    corruptEvents,
    undoPlan
  });

  return (
    <Box flexDirection="column">
      <Box marginBottom={1}>
        <Text bold>Timeline</Text>
        <Text dimColor>  Filter: {model.filterLabel}</Text>
      </Box>

      {model.corruptWarning && (
        <Text color="yellow">{model.corruptWarning}</Text>
      )}

      {model.emptyMessage && (
        <NoTimelineEventsEmptyState command={model.emptyCommand} />
      )}

      {model.rows.length > 0 && (
        <Box flexDirection="row" gap={2}>
          <Box flexDirection="column" width={72}>
            <Text bold>event    observed                 kind           readiness     agent        title</Text>
            <Text dimColor>{"─".repeat(72)}</Text>
            {model.rows.map((row) => (
              <Text key={row.id} color={row.selected ? "cyan" : undefined} bold={row.selected}>
                {row.selected ? "▸ " : "  "}
                {pad(row.shortId, 8)} {pad(row.observedAt, 24)} {pad(row.eventKind, 14)} {pad(row.readiness, 13)} {pad(row.agentScope, 12)} {row.title}
              </Text>
            ))}
          </Box>

          {model.selectedEntry && (
            <Box flexDirection="column" flexGrow={1}>
              <Text bold>Selected: {model.selectedEntry.id.slice(0, 8)}</Text>
              <Text>{model.selectedEntry.title}</Text>
              <Text dimColor>
                kind: {model.selectedEntry.eventKind}  readiness: {model.selectedEntry.readiness}
              </Text>
              <Text dimColor>
                before: {model.selectedEntry.beforeSnapshotName}  after: {model.selectedEntry.afterSnapshotName}
              </Text>

              {model.selectedEntry.highlights.length > 0 && (
                <Box flexDirection="column" marginTop={1}>
                  <Text bold>Changed</Text>
                  {model.selectedEntry.highlights.slice(0, 4).map((highlight) => (
                    <Text key={highlight}>- {highlight}</Text>
                  ))}
                </Box>
              )}

              <SurfaceList title="Writable preview" surfaces={model.selectedEntry.writableSurfaces} />
              <SurfaceList title="Observe-only" surfaces={model.selectedEntry.observeOnlySurfaces} />
              <Box flexDirection="column" marginTop={1}>
                <Text bold>Actions</Text>
                <Text color="cyan">u preview undo</Text>
              </Box>
            </Box>
          )}
        </Box>
      )}

      {undoError && (
        <Box marginTop={1}>
          <Text color="red">Preview error: {undoError}</Text>
        </Box>
      )}

      {model.undoPreview && (
        <Box flexDirection="column" marginTop={1}>
          <Text bold>Undo preview</Text>
          <Text>{model.undoPreview.title}</Text>
          <Text>writes files: {model.undoPreview.writesFiles}</Text>
          {model.undoPreview.emptyWritableMessage && (
            <Text dimColor>{model.undoPreview.emptyWritableMessage}</Text>
          )}
          {model.undoPreview.writableItems.map((item) => (
            <Text key={`${item.action}:${item.path}:${item.serverName}`} color="cyan">
              {item.action} mcp_server {item.serverName} at {item.path}
            </Text>
          ))}
          <SurfaceList title="Observe-only in preview" surfaces={model.undoPreview.observeOnlySurfaces} />
        </Box>
      )}
    </Box>
  );
}

function SurfaceList({ title, surfaces }: { title: string; surfaces: TimelineChangedSurface[] }) {
  if (surfaces.length === 0) return null;

  return (
    <Box flexDirection="column" marginTop={1}>
      <Text bold>{title}</Text>
      {surfaces.slice(0, 6).map((surface, index) => (
        <Text key={`${surface.kind}:${surface.changeType}:${surface.path}:${index}`}>
          - {surface.kind} {surface.changeType} {surface.entityName ?? "-"} {surface.path}
        </Text>
      ))}
    </Box>
  );
}

function pad(value: string, width: number): string {
  return padDisplay(value, width);
}
