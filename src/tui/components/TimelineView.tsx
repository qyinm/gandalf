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
  height?: number;
  header?: React.ReactNode;
  footer?: string;
}

export const DEFAULT_CURRENT_SETUP_WINDOW_SIZE = 5;

export default function TimelineView({
  entries,
  selectedIndex,
  agentFilter,
  evidence = [],
  corruptEvents = [],
  undoPlan,
  undoError,
  currentSetupFocus = "timeline",
  currentSetupOffsets = {},
  height,
  header,
  footer
}: TimelineViewProps) {
  const model = buildTimelineViewModel({
    entries,
    selectedIndex,
    agentFilter,
    evidence,
    corruptEvents,
    undoPlan
  });
  const setupSection = currentSetupFocus === "timeline" ? "skill" : currentSetupFocus;
  const setupRows = rowsForSetupSection(model.currentSetup, setupSection);
  const setupOffset = currentSetupOffsets[setupSection] ?? 0;
  const setupPanelHeight = height ? Math.min(16, Math.max(12, height - 8)) : undefined;
  const timelinePanelHeight = height && setupPanelHeight
    ? Math.max(6, height - setupPanelHeight - 1)
    : undefined;

  return (
    <Box flexDirection="column" height={height}>
      <Box
        flexDirection="column"
        borderStyle="round"
        borderColor="cyan"
        paddingX={1}
        paddingY={0}
        marginBottom={1}
        height={setupPanelHeight}
      >
        {header && (
          <Box marginBottom={1}>
            {header}
          </Box>
        )}
        <Text bold>Current Setup</Text>
        <Text dimColor>  Scope: {model.currentSetup.scopeLabel}</Text>
        <Text>
          {"  "}Agents {model.currentSetup.agents}  Skills {model.currentSetup.skills}  MCP Servers {model.currentSetup.mcpServers}  Hooks {model.currentSetup.hooks}  Permissions {model.currentSetup.permissions}  Env Keys {model.currentSetup.envKeys}
        </Text>
        <CurrentSetupTabs
          activeSection={setupSection}
          focused={currentSetupFocus !== "timeline"}
          model={model.currentSetup}
        />
        <CurrentSetupRows
          rows={setupRows}
          kind={setupSection}
          offset={setupOffset}
        />
      </Box>

      <Box
        flexDirection="column"
        borderStyle="round"
        borderColor="cyan"
        paddingX={1}
        paddingY={0}
        height={timelinePanelHeight}
        flexGrow={height ? 1 : undefined}
      >
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
        {footer && (
          <Box marginTop={1}>
            <Text dimColor>{footer}</Text>
          </Box>
        )}
      </Box>
    </Box>
  );
}

function CurrentSetupRows({
  rows,
  kind,
  offset
}: {
  rows: string[];
  kind: DiscoveredItem["kind"];
  offset: number;
}) {
  const safeOffset = clampOffset(offset, rows.length, DEFAULT_CURRENT_SETUP_WINDOW_SIZE);
  const visibleRows = rows.slice(safeOffset, safeOffset + DEFAULT_CURRENT_SETUP_WINDOW_SIZE);
  const hasOverflow = rows.length > DEFAULT_CURRENT_SETUP_WINDOW_SIZE;

  return (
    <Box flexDirection="column" marginTop={1}>
      {rows.length === 0 ? (
        <Text dimColor>  {currentSetupEmptyText(kind)}</Text>
      ) : (
        visibleRows.map((row) => (
          <Text key={`${kind}:${row}`}>  {row}</Text>
        ))
      )}
      {hasOverflow && (
        <Text dimColor>
          {"  "}showing {safeOffset + 1}-{Math.min(rows.length, safeOffset + DEFAULT_CURRENT_SETUP_WINDOW_SIZE)} of {rows.length}
        </Text>
      )}
    </Box>
  );
}

function CurrentSetupTabs({
  activeSection,
  focused,
  model
}: {
  activeSection: CurrentSetupInventorySection;
  focused: boolean;
  model: ReturnType<typeof buildTimelineViewModel>["currentSetup"];
}) {
  const tabs: Array<{ section: CurrentSetupInventorySection; label: string; count: number }> = [
    { section: "skill", label: "Skills", count: model.skills },
    { section: "mcp_server", label: "MCP Servers", count: model.mcpServers },
    { section: "hook", label: "Hooks", count: model.hooks },
    { section: "env_key", label: "Project", count: model.envKeys },
  ];

  return (
    <Box flexDirection="row" gap={1} marginTop={1}>
      {tabs.map((tab) => {
        const active = tab.section === activeSection;
        return (
          <Text
            key={tab.section}
            bold={active}
            color={active ? "cyan" : "dim"}
            inverse={active && focused}
          >
            {" "}{tab.label} {tab.count}{" "}
          </Text>
        );
      })}
    </Box>
  );
}

function rowsForSetupSection(
  model: ReturnType<typeof buildTimelineViewModel>["currentSetup"],
  section: CurrentSetupInventorySection
): string[] {
  if (section === "skill") return model.skillRows;
  if (section === "mcp_server") return model.mcpServerRows;
  if (section === "hook") return model.hookRows;
  return model.envKeyRows;
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
