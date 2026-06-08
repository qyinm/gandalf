import type { AgentId, DiscoveredItem, TimelineEntry } from "../../types.js";
import { formatAgentLabel, formatInventoryNameWithSource, formatTimelineTimestamp } from "./TuiFormatters.js";

export interface AgentInventoryRow {
  name: string;
  status?: string;
  path?: string;
}

export interface AgentHistoryRow {
  id: string;
  observedAt: string;
  title: string;
}

export interface AgentDetailViewModel {
  title: string;
  profileLabel: string;
  counts: {
    skills: number;
    mcpServers: number;
    hooks: number;
    permissions: number;
    envKeys: number;
    instructions: number;
  };
  skills: AgentInventoryRow[];
  mcpServers: AgentInventoryRow[];
  hooks: AgentInventoryRow[];
  envKeys: AgentInventoryRow[];
  instructions: AgentInventoryRow[];
  history: AgentHistoryRow[];
  emptyMessage?: string;
}

export function buildAgentDetailViewModel(input: {
  agent: AgentId;
  evidence: DiscoveredItem[];
  timelineEntries?: TimelineEntry[];
  profile?: string;
  now?: Date;
}): AgentDetailViewModel {
  const agentEvidence = input.evidence.filter((item) => item.agent === input.agent);
  const setupEvidence = input.evidence.filter((item) => item.agent === input.agent || item.agent === "project");
  const timelineEntries = input.timelineEntries ?? [];

  return {
    title: formatAgentLabel(input.agent),
    profileLabel: input.profile ?? "default",
    counts: {
      skills: countKind(setupEvidence, "skill"),
      mcpServers: countKind(setupEvidence, "mcp_server"),
      hooks: countKind(setupEvidence, "hook"),
      permissions: countKind(setupEvidence, "permission"),
      envKeys: countKind(setupEvidence, "env_key"),
      instructions: countKind(setupEvidence, "agent_instruction")
    },
    skills: rowsForKind(setupEvidence, "skill"),
    mcpServers: rowsForKind(setupEvidence, "mcp_server").map((row) => ({
      ...row,
      status: row.status ?? "enabled"
    })),
    hooks: rowsForKind(setupEvidence, "hook"),
    envKeys: rowsForKind(setupEvidence, "env_key"),
    instructions: rowsForKind(setupEvidence, "agent_instruction"),
    history: timelineEntries
      .filter((entry) => entry.agent === input.agent || entry.agents.includes(input.agent))
      .slice(0, 6)
      .map((entry) => ({
        id: entry.id.slice(0, 8),
        observedAt: formatTimelineTimestamp(entry.observedAt, input.now),
        title: entry.title
      })),
    emptyMessage: agentEvidence.length === 0
      ? "No supported agent setup found."
      : undefined
  };
}

function countKind(evidence: DiscoveredItem[], kind: DiscoveredItem["kind"]): number {
  return evidence.filter((item) => item.kind === kind).length;
}

function rowsForKind(evidence: DiscoveredItem[], kind: DiscoveredItem["kind"]): AgentInventoryRow[] {
  return evidence
    .filter((item) => item.kind === kind)
    .map((item) => ({
      name: displayNameForItem(item),
      path: item.sourcePath,
      status: statusForItem(item)
    }))
    .sort((left, right) => left.name.localeCompare(right.name));
}

function displayNameForItem(item: DiscoveredItem): string {
  const name = item.name ?? item.id;
  const sourceLabeledName = formatInventoryNameWithSource(name, item);
  if (sourceLabeledName !== name) return sourceLabeledName;
  return item.agent === "project" ? `${name} (project)` : name;
}

function statusForItem(item: DiscoveredItem): string | undefined {
  if (item.kind === "mcp_server") {
    const value = asRecord(item.value);
    if (value.disabled === true || value.enabled === false) return "disabled";
    return "enabled";
  }

  if (item.captureStatus !== "captured") return item.captureStatus;
  return undefined;
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
}
