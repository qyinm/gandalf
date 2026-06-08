import React from "react";
import { Box, Text } from "ink";

import type { TimelineUndoPlan } from "../../timeline-undo.js";
import type { AgentId, DiscoveredItem, TimelineChangedSurface, TimelineEntry } from "../../types.js";
import type { TimelineCorruptEvent } from "../../store.js";
import {
  buildTimelineViewModel,
  currentSetupEmptyText,
  type CurrentSetupInventorySection
} from "./TimelineViewModel.js";
import { padDisplay } from "./TuiFormatters.js";
import { NoTimelineEventsEmptyState } from "./TuiEmptyStates.js";

interface TimelineViewProps {
  entries: TimelineEntry[];
  selectedIndex: number;
  agentFilter: AgentId | null;
  evidence?: DiscoveredItem[];
  corruptEvents?: TimelineCorruptEvent[];
  undoPlan?: TimelineUndoPlan | null;
  undoError?: string | null;
  currentSetupFocus?: CurrentSetupInventorySection | "timeline";
  currentSetupOffsets?: Partial<Record<CurrentSetupInventorySection, number>>;
}

export const DEFAULT_CURRENT_SETUP_WINDOW_SIZE = 6;

export default function TimelineView({
  entries,
  selectedIndex,
  agentFilter,
  evidence = [],
  corruptEvents = [],
  undoPlan,
  undoError,
  currentSetupFocus = "timeline",
  currentSetupOffsets = {}
}: TimelineViewProps) {
  const model = buildTimelineViewModel({
    entries,
    selectedIndex,
    agentFilter,
    evidence,
    corruptEvents,
    undoPlan
  });

  return (
    <Box flexDirection="column">
      <Box flexDirection="column" marginBottom={1}>
        <Text bold>Current Setup</Text>
        <Text dimColor>  Scope: {model.currentSetup.scopeLabel}</Text>
        <Text>
          {"  "}Agents {model.currentSetup.agents}  Skills {model.currentSetup.skills}  MCP Servers {model.currentSetup.mcpServers}  Hooks {model.currentSetup.hooks}  Permissions {model.currentSetup.permissions}
        </Text>
        <CurrentSetupRows
          title="Skills"
          rows={model.currentSetup.skillRows}
          kind="skill"
          active={currentSetupFocus === "skill"}
          offset={currentSetupOffsets.skill ?? 0}
        />
        <CurrentSetupRows
          title="MCP Servers"
          rows={model.currentSetup.mcpServerRows}
          kind="mcp_server"
          active={currentSetupFocus === "mcp_server"}
          offset={currentSetupOffsets.mcp_server ?? 0}
        />
        <CurrentSetupRows
          title="Hooks"
          rows={model.currentSetup.hookRows}
          kind="hook"
          active={currentSetupFocus === "hook"}
          offset={currentSetupOffsets.hook ?? 0}
        />
        <Text>  Instructions  {model.currentSetup.instructions}</Text>
      </Box>

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

function CurrentSetupRows({
  title,
  rows,
  kind,
  active,
  offset
}: {
  title: string;
  rows: string[];
  kind: DiscoveredItem["kind"];
  active: boolean;
  offset: number;
}) {
  const safeOffset = clampOffset(offset, rows.length, DEFAULT_CURRENT_SETUP_WINDOW_SIZE);
  const visibleRows = rows.slice(safeOffset, safeOffset + DEFAULT_CURRENT_SETUP_WINDOW_SIZE);
  const hasOverflow = rows.length > DEFAULT_CURRENT_SETUP_WINDOW_SIZE;
  const titlePrefix = active ? "▸" : " ";

  return (
    <Box flexDirection="column">
      <Text color={active ? "cyan" : undefined}> {titlePrefix} {title}</Text>
      {rows.length === 0 ? (
        <Text dimColor>    {currentSetupEmptyText(kind)}</Text>
      ) : (
        visibleRows.map((row) => (
          <Text key={`${title}:${row}`}>    {row}</Text>
        ))
      )}
      {hasOverflow && (
        <Text dimColor>
          {"    "}showing {safeOffset + 1}-{Math.min(rows.length, safeOffset + DEFAULT_CURRENT_SETUP_WINDOW_SIZE)} of {rows.length}
        </Text>
      )}
    </Box>
  );
}

function clampOffset(offset: number, total: number, windowSize: number): number {
  return Math.min(Math.max(0, offset), Math.max(0, total - windowSize));
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
