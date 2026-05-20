/**
 * Ink component for snaptailor audit report.
 *
 * Renders audit findings grouped by severity with color coding.
 * Uses ink-markdown for section rendering.
 */

import React from "react";
import { Text, Box } from "ink";

import type { AuditFinding } from "../../types.js";

// ── Props ────────────────────────────────────────────────────

interface AuditViewProps {
  findings: AuditFinding[];
}

// ── Color map ────────────────────────────────────────────────

const SEVERITY_COLORS: Record<string, string> = {
  critical: "red",
  high: "redBright",
  medium: "yellow",
  low: "cyan",
  none: "white",
};

const SEVERITY_RANK: Record<string, number> = {
  critical: 5,
  high: 4,
  medium: 3,
  low: 2,
  none: 1,
};

// ── Component ────────────────────────────────────────────────

export default function AuditView({ findings }: AuditViewProps) {
  if (findings.length === 0) {
    return (
      <Box flexDirection="column">
        <Text bold color="green">snaptailor audit</Text>
        <Text>No findings — everything looks clean.</Text>
      </Box>
    );
  }

  const sorted = [...findings].sort(
    (a, b) => (SEVERITY_RANK[b.severity] ?? 0) - (SEVERITY_RANK[a.severity] ?? 0)
  );

  const criticalCount = sorted.filter((f) => f.severity === "critical").length;
  const highCount = sorted.filter((f) => f.severity === "high").length;
  const mediumCount = sorted.filter((f) => f.severity === "medium").length;
  const lowCount = sorted.filter((f) => f.severity === "low").length;

  return (
    <Box flexDirection="column">
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold>snaptailor audit — {sorted.length} findings</Text>
      </Box>

      {/* Summary badges */}
      <Box marginBottom={1} gap={2}>
        {criticalCount > 0 && (
          <Text color="red" bold>
            {criticalCount} critical
          </Text>
        )}
        {highCount > 0 && (
          <Text color="redBright" bold>
            {highCount} high
          </Text>
        )}
        {mediumCount > 0 && (
          <Text color="yellow" bold>
            {mediumCount} medium
          </Text>
        )}
        {lowCount > 0 && (
          <Text color="cyan">
            {lowCount} low
          </Text>
        )}
      </Box>

      {/* Findings grouped by severity */}
      {renderGroup(sorted, "critical")}
      {renderGroup(sorted, "high")}
      {renderGroup(sorted, "medium")}
      {renderGroup(sorted, "low")}
    </Box>
  );
}

// ── Group renderer ───────────────────────────────────────────

function renderGroup(findings: AuditFinding[], severity: string) {
  const group = findings.filter((f) => f.severity === severity);
  if (group.length === 0) return null;

  const color = SEVERITY_COLORS[severity] ?? "white";

  return (
    <Box marginBottom={1} flexDirection="column">
      <Text bold color={color}>
        {severity.toUpperCase()} ({group.length})
      </Text>
      {group.map((f, i) => (
        <Box key={i} flexDirection="column" marginLeft={2}>
          <Text>
            <Text color={color}>●</Text>{" "}
            <Text bold>{f.code}</Text>
            {f.path && <Text dimColor> — {f.path}</Text>}
          </Text>
          <Box marginLeft={2}><Text>{f.problem}</Text></Box>
          {f.fix && <Box marginLeft={2}><Text dimColor>fix: {f.fix}</Text></Box>}
        </Box>
      ))}
    </Box>
  );
}