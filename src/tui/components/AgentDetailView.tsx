import React from "react";
import { Box, Text } from "ink";

import type { AgentId, DiscoveredItem, TimelineEntry } from "../../types.js";
import { buildAgentDetailViewModel, type AgentInventoryRow } from "./AgentDetailViewModel.js";
import { NoDetectedAgentsEmptyState } from "./TuiEmptyStates.js";

interface AgentDetailViewProps {
  agent: AgentId;
  evidence: DiscoveredItem[];
  timelineEntries: TimelineEntry[];
}

export default function AgentDetailView({
  agent,
  evidence,
  timelineEntries
}: AgentDetailViewProps) {
  const model = buildAgentDetailViewModel({
    agent,
    evidence,
    timelineEntries
  });

  if (model.emptyMessage) {
    return (
      <Box flexDirection="column">
        <Text bold>{model.title}</Text>
        <NoDetectedAgentsEmptyState />
      </Box>
    );
  }

  return (
    <Box flexDirection="column">
      <Text bold>{model.title}</Text>
      <Text dimColor>Profile: {model.profileLabel}</Text>

      <Box flexDirection="column" marginTop={1}>
        <Text bold>Current Setup</Text>
        <Text>  Skills        {model.counts.skills}</Text>
        <Text>  MCP Servers   {model.counts.mcpServers}</Text>
        <Text>  Hooks         {model.counts.hooks}</Text>
        <Text>  Permissions   {model.counts.permissions}</Text>
        <Text>  Env Keys      {model.counts.envKeys}</Text>
        <Text>  Instructions  {model.counts.instructions}</Text>
      </Box>

      <InventorySection title="Skills" rows={model.skills} />
      <InventorySection title="MCP Servers" rows={model.mcpServers} />
      <InventorySection title="Env Keys" rows={model.envKeys} />
      <InventorySection title="Instructions" rows={model.instructions} showPath />

      <Box flexDirection="column" marginTop={1}>
        <Text bold>History</Text>
        {model.history.length === 0 ? (
          <Text dimColor>  none</Text>
        ) : (
          model.history.map((entry) => (
            <Text key={entry.id}>* {entry.id}  {entry.observedAt}  {entry.title}</Text>
          ))
        )}
      </Box>
    </Box>
  );
}

function InventorySection({
  title,
  rows,
  showPath = false
}: {
  title: string;
  rows: AgentInventoryRow[];
  showPath?: boolean;
}) {
  return (
    <Box flexDirection="column" marginTop={1}>
      <Text bold>{title}</Text>
      {rows.length === 0 ? (
        <Text dimColor>  none</Text>
      ) : (
        rows.slice(0, 8).map((row) => (
          <Text key={`${title}:${row.name}:${row.path ?? ""}`}>
            {"  "}{row.name}
            {row.status && !showPath ? `  ${row.status}` : ""}
            {showPath && row.path ? `  ${row.path}` : ""}
          </Text>
        ))
      )}
    </Box>
  );
}
