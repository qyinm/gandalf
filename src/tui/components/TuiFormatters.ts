import type { AgentId } from "../../types.js";

const AGENT_LABELS: Record<string, string> = {
  "claude-code": "Claude Code",
  codex: "Codex",
  cursor: "Cursor",
  opencode: "OpenCode",
  "pi-agent": "Pi Agent",
  project: "Project",
  unknown: "Unknown"
};

export function formatAgentLabel(id: AgentId): string {
  return AGENT_LABELS[id] ?? id;
}

export function formatAgentScope(agent: AgentId | null | undefined, agents: AgentId[] = []): string {
  if (agent) return formatAgentLabel(agent);
  if (agents.length === 0) return "all";
  if (agents.length > 1) return agents.map(formatAgentLabel).join(", ");
  return formatAgentLabel(agents[0]);
}

export function formatTimelineTimestamp(value: string, now = new Date()): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;

  const dateKey = localDateKey(date);
  const nowKey = localDateKey(now);
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);

  if (dateKey === nowKey) {
    return `Today ${formatClock(date)}`;
  }

  if (dateKey === localDateKey(yesterday)) {
    return `Yesterday ${formatClock(date)}`;
  }

  return `${date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric"
  })} ${formatClock(date)}`;
}

export function truncateText(value: string, width: number): string {
  if (width <= 0) return "";
  if (value.length <= width) return value;
  if (width <= 3) return ".".repeat(width);
  return `${value.slice(0, width - 3)}...`;
}

export function padDisplay(value: string, width: number): string {
  return truncateText(value, width).padEnd(width);
}

function localDateKey(date: Date): string {
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
}

function formatClock(date: Date): string {
  return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}
