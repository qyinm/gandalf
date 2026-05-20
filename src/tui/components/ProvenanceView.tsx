/**
 * Ink component for snaptailor provenance output.
 *
 * Renders graph node → source path trace in a tree-like view.
 */

import React from "react";
import { Text, Box } from "ink";

import type { ProvenanceEntry } from "../../types.js";

// ── Props ────────────────────────────────────────────────────

interface ProvenanceViewProps {
  entries: ProvenanceEntry[];
}

// ── Component ────────────────────────────────────────────────

export default function ProvenanceView({ entries }: ProvenanceViewProps) {
  if (entries.length === 0) {
    return (
      <Box flexDirection="column">
        <Text bold>snaptailor provenance</Text>
        <Text dimColor>No provenance data available.</Text>
      </Box>
    );
  }

  return (
    <Box flexDirection="column">
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold>snaptailor provenance ({entries.length} entries)</Text>
      </Box>

      {/* Entries */}
      {entries.map((entry, i) => (
        <Box key={i} flexDirection="column" marginBottom={1}>
          {/* Graph node */}
          <Box>
            <Text color="cyan">●</Text>{" "}
            <Text bold>{entry.nodeId}</Text>
          </Box>

          {/* Evidence source */}
          <Box marginLeft={3} flexDirection="column">
            <Text>
              <Text dimColor>source: </Text>
              {entry.evidenceId}
            </Text>
            <Text>
              <Text dimColor>path: </Text>
              {entry.sourcePath}
            </Text>
            <Text>
              <Text dimColor>scope: </Text>
              {entry.scope}
              <Text dimColor> | confidence: </Text>
              {entry.confidence}
            </Text>
          </Box>
        </Box>
      ))}
    </Box>
  );
}