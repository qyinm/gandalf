/**
 * Ink component for snaptailor scan results.
 *
 * Renders a rich terminal view of scan output including:
 * - Trust badges
 * - Detected agents per scope
 * - Evidence table
 * - Audit findings by severity
 * - Blind spots
 */

import React from "react";
import { Text, Box } from "ink";
import Table from "ink-table";

import type { AuditFinding, DiscoveredItem } from "../../types.js";

// ── Props ────────────────────────────────────────────────────

interface ScanViewProps {
  evidence: DiscoveredItem[];
  auditFindings: AuditFinding[];
  blindSpots: string[];
  readOnly: boolean;
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
}: ScanViewProps) {
  // ── Trust Section ─────────────────────────────────────────
  const trustColor = readOnly ? "green" : "red";

  // ── Agent Summary ─────────────────────────────────────────
  const agents = [...new Set(evidence.map((e) => e.agent))].sort();

  // ── Evidence Table ────────────────────────────────────────
  const tableData = evidence.map((item) => ({
    agent: agentLabel(item.agent),
    kind: item.kind,
    scope: item.scope,
    source: item.sourcePath.split("/").pop() ?? item.sourcePath,
    status: item.captureStatus,
  }));

  // ── Findings ──────────────────────────────────────────────
  const sortedFindings = [...auditFindings].sort(
    (a, b) => severityRank(b.severity) - severityRank(a.severity)
  );

  return (
    <Box flexDirection="column" paddingX={0}>
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold>snaptailor scan</Text>
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

      {/* Evidence table */}
      {tableData.length > 0 && (
        <Box marginBottom={1} flexDirection="column">
          <Text bold>Discovered Evidence</Text>
          <Table
            data={tableData.slice(0, 50)}
            columns={["agent", "kind", "scope", "source", "status"]}
            padding={1}
          />
          {tableData.length > 50 && (
            <Text color="yellow">
              ... and {tableData.length - 50} more items
            </Text>
          )}
        </Box>
      )}

      {/* Findings */}
      {sortedFindings.length > 0 && (
        <Box marginBottom={1} flexDirection="column">
          <Text bold>Findings ({sortedFindings.length})</Text>
          {sortedFindings.slice(0, 15).map((f, i) => (
            <Text key={i}>
              <Text color={severityColor(f.severity)}>
                {f.severity.toUpperCase().padEnd(9)}
              </Text>
              {f.problem}
            </Text>
          ))}
          {sortedFindings.length > 15 && (
            <Text color="yellow">
              ... and {sortedFindings.length - 15} more findings
            </Text>
          )}
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

// ── Helpers ──────────────────────────────────────────────────

function severityRank(s: string): number {
  switch (s) {
    case "critical": return 5;
    case "high": return 4;
    case "medium": return 3;
    case "low": return 2;
    default: return 1;
  }
}