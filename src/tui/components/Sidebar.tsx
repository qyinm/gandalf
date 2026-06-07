/**
 * Fixed sidebar for hem TUI dashboard.
 *
 * Always visible on the left. Shows detected agents with evidence counts.
 * User navigates with ↑↓/jk and selects with Enter.
 */
import React from "react";
import { Text, Box } from "ink";
import type { AgentId } from "../../types.js";
import type { TuiNavSection } from "./TuiNavigationModel.js";

interface AgentEntry {
  id: AgentId | null;
  label: string;
  evidenceCount: number;
}

interface SidebarProps {
  agents?: AgentEntry[];
  selectedAgent?: AgentId | null;
  cursor: number;
  navSections?: TuiNavSection[];
  selectedItemId?: string;
}

const VISIBLE_AGENTS: AgentId[] = [
  "claude-code",
  "codex",
  "cursor",
  "opencode",
  "pi-agent",
  "project",
];

function agentLabel(id: AgentId): string {
  const map: Record<string, string> = {
    "claude-code": "Claude Code",
    codex: "Codex",
    cursor: "Cursor",
    opencode: "OpenCode",
    "pi-agent": "Pi Agent",
    project: "Project",
  };
  return map[id] ?? id;
}

export function buildAgentEntries(
  evidence: { agent: AgentId }[]
): AgentEntry[] {
  const found = new Set(evidence.map((e) => e.agent));
  return VISIBLE_AGENTS.filter((a) => found.has(a)).map((id) => ({
    id,
    label: agentLabel(id),
    evidenceCount: evidence.filter((e) => e.agent === id).length,
  }));
}

export function buildAgentFilterEntries(evidence: { agent: AgentId }[]): AgentEntry[] {
  return [
    {
      id: null,
      label: "All agents",
      evidenceCount: evidence.length,
    },
    ...buildAgentEntries(evidence),
  ];
}

export function agentLabelStr(id: AgentId): string {
  return agentLabel(id);
}

export default function Sidebar({
  agents = [],
  selectedAgent = null,
  cursor,
  navSections,
  selectedItemId,
}: SidebarProps) {
  const SIDEBAR_WIDTH = 26;
  const flatNavItems = navSections?.flatMap((section) => section.items) ?? [];
  const cursorItemId = flatNavItems[cursor]?.id;

  if (navSections) {
    return (
      <Box
        flexDirection="column"
        width={SIDEBAR_WIDTH}
        borderStyle="round"
        borderColor="cyan"
        paddingX={1}
        paddingY={0}
      >
        {navSections.map((section, sectionIndex) => (
          <Box key={section.id} flexDirection="column" marginTop={sectionIndex === 0 ? 0 : 1}>
            <Text bold color="cyan">
              {section.label}
            </Text>
            {section.items.length === 0 ? (
              <Box marginLeft={2}>
                <Text dimColor>none</Text>
              </Box>
            ) : (
              section.items.map((item) => {
                const isCursor = item.id === cursorItemId;
                const isActive = item.id === selectedItemId;
                return (
                  <Box key={item.id}>
                    <Text
                      bold={isCursor}
                      color={isCursor ? "cyan" : isActive ? "white" : "dim"}
                    >
                      {isCursor ? "▸ " : isActive ? "● " : "  "}
                      {item.label.padEnd(14)}
                      {typeof item.evidenceCount === "number" && (
                        <Text dimColor>{item.evidenceCount}</Text>
                      )}
                    </Text>
                  </Box>
                );
              })
            )}
          </Box>
        ))}

        <Box flexDirection="column" marginTop={1}>
          <Text dimColor>{"─".repeat(SIDEBAR_WIDTH - 4)}</Text>
          <Text dimColor>↑↓ move</Text>
          <Text dimColor>Enter open</Text>
          <Text dimColor>q quit</Text>
        </Box>
      </Box>
    );
  }

  return (
    <Box
      flexDirection="column"
      width={SIDEBAR_WIDTH}
      borderStyle="round"
      borderColor="cyan"
      paddingX={1}
      paddingY={0}
    >
      {/* Header */}
      <Box>
        <Text bold color="cyan">
          Agents
        </Text>
      </Box>
      <Text dimColor>{"─".repeat(SIDEBAR_WIDTH - 4)}</Text>

      {/* Agent list */}
      <Box flexDirection="column" marginTop={1}>
        {agents.map((agent, i) => {
          const isActive = agent.id === selectedAgent;
          return (
            <Box key={agent.id ?? "all"}>
              <Text
                bold={cursor === i}
                color={cursor === i ? "cyan" : isActive ? "white" : "dim"}
              >
                {cursor === i ? "▸ " : isActive ? "● " : "  "}
                {agent.label.padEnd(14)}
                <Text dimColor>{agent.evidenceCount}</Text>
              </Text>
            </Box>
          );
        })}
      </Box>

      {/* Footer spacer + hints */}
      <Box flexDirection="column" marginTop={1}>
        <Text dimColor>{"─".repeat(SIDEBAR_WIDTH - 4)}</Text>
        <Text dimColor>↑↓ nav</Text>
        <Text dimColor>Enter select</Text>
        <Text dimColor>q quit</Text>
      </Box>
    </Box>
  );
}
