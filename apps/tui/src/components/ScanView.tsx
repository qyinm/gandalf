/**
 * Ink component for hem scan results.
 *
 * Renders a rich terminal view of scan output including:
 * - Trust badges
 * - Detected agents per scope
 * - Evidence table (using SimpleTable, no CJS dep)
 * - Audit findings by severity
 * - Blind spots
 */

import React from "react";
import { Text, Box } from "ink";
import SimpleTable from "./SimpleTable.js";

import type { AuditFinding, DiscoveredItem } from "@qxinm/hem-core/types.js";

export const DEFAULT_SCAN_WINDOW_SIZE = 10;

// ── Props ────────────────────────────────────────────────────

interface ScanViewProps {
  evidence: DiscoveredItem[];
  auditFindings: AuditFinding[];
  blindSpots: string[];
  readOnly: boolean;
  /** Scroll offset into the evidence + findings list (default 0) */
  scrollOffset?: number;
  /** Number of items visible at once (default 10) */
  windowSize?: number;
}

// ── Helpers ──────────────────────────────────────────────────

function agentLabel(agent: string): string {
  const map: Record<string, string> = {
    "claude-code": "Claude Code",
    codex: "Codex",
    cursor: "Cursor",
    opencode: "OpenCode",
    "pi-agent": "Pi Agent",
    project: "Project",
    unknown: "Unknown",
  };
  return map[agent] ?? agent;
}

function severityColor(severity: string): string {
  switch (severity) {
    case "critical": return "red";
    case "high": return "red";
    case "medium": return "yellow";
    case "low": return "cyan";
    default: return "white";
  }
}

// ── Main Component ───────────────────────────────────────────

export default function ScanView({
  evidence,
  auditFindings,
  blindSpots,
  readOnly,
  scrollOffset = 0,
  windowSize = DEFAULT_SCAN_WINDOW_SIZE,
}: ScanViewProps) {
  const trustColor = readOnly ? "green" : "red";
  const agents = [...new Set(evidence.map((e) => e.agent))].sort();

  const tableData = evidence.map((item) => ({
    agent: agentLabel(item.agent),
    kind: item.kind,
    scope: item.scope,
    source: item.sourcePath.split("/").pop() ?? item.sourcePath,
    status: item.captureStatus,
  }));

  const sortedFindings = [...auditFindings].sort(
    (a, b) => severityRank(b.severity) - severityRank(a.severity)
  );
  const maxScrollOffset = Math.max(0, tableData.length - windowSize);
  const clampedScrollOffset = Math.min(Math.max(0, scrollOffset), maxScrollOffset);

  return (
    <Box flexDirection="column" paddingX={0}>
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold>hem scan</Text>
      </Box>

      {/* Trust badge */}
      <Box marginBottom={1}>
        <Text color={trustColor}>
          {readOnly ? "✓ Read-only" : "✗ May write"}
        </Text>
        <Text> — Discovered {evidence.length} evidence items across </Text>
        <Text bold>{agents.length}</Text>
        <Text> agent{agents.length !== 1 ? "s" : ""}</Text>
      </Box>

      {/* Agent list */}
      {agents.length > 0 && (
        <Box marginBottom={1} flexDirection="column">
          {agents.map((agent) => {
            const items = evidence.filter((e) => e.agent === agent);
            const scopes = [...new Set(items.map((i) => i.scope))];
            return (
              <Text key={agent}>
                <Text color="cyan">●</Text>{" "}
                <Text bold>{agentLabel(agent)}</Text> —{" "}
                {scopes.sort().join(" + ")} state found ({items.length} items)
              </Text>
            );
          })}
        </Box>
      )}

      {/* Evidence table — scrollable */}
      {tableData.length > 0 && (
        <Box marginBottom={1} flexDirection="column">
          <Text bold>Discovered Evidence ({tableData.length})</Text>
          <SimpleTable
            data={tableData.slice(clampedScrollOffset, clampedScrollOffset + windowSize)}
            columns={["agent", "kind", "scope", "source", "status"]}
          />
          {clampedScrollOffset > 0 && (
            <Text dimColor>  ↑ {clampedScrollOffset} above</Text>
          )}
          {clampedScrollOffset + windowSize < tableData.length && (
            <Text dimColor>
              ↓ {tableData.length - clampedScrollOffset - windowSize} more below
            </Text>
          )}
        </Box>
      )}

{/* Findings */}
       {sortedFindings.length > 0 && (
         <Box marginBottom={1} flexDirection="column">
           <Text bold>Findings ({sortedFindings.length})</Text>
           {sortedFindings.map((f, i) => (
            <Text key={i}>
              <Text color={severityColor(f.severity)}>
                {f.severity.toUpperCase().padEnd(9)}
              </Text>
              {f.problem}
            </Text>
          ))}
        </Box>
      )}

      {/* Blind spots */}
      {blindSpots.length > 0 && (
        <Box flexDirection="column">
          <Text bold>Blind spots</Text>
          {blindSpots.map((spot, i) => (
            <Text key={i} color="yellow">  {spot}</Text>
          ))}
        </Box>
      )}
    </Box>
  );
}

function severityRank(s: string): number {
  switch (s) {
    case "critical": return 5;
    case "high": return 4;
    case "medium": return 3;
    case "low": return 2;
    default: return 1;
  }
}
