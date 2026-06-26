/**
 * Ink component for gandalf diff output.
 *
 * Renders semantic changes and raw source changes with color coding:
 * - Added = green
 * - Removed = red
 * - Changed = yellow
 */

import React from "react";
import { Text, Box } from "ink";

import type { SemanticChange, RawSourceChange } from "@qxinm/gandalf-core/diff.js";

// ── Props ────────────────────────────────────────────────────

interface DiffViewProps {
  semanticChanges: SemanticChange[];
  rawSourceChanges?: RawSourceChange[];
}

// ── Helpers ──────────────────────────────────────────────────

function changeColor(code: string): string {
  switch (code) {
    case "MCP_ADDED": return "green";
    case "MCP_REMOVED": return "red";
    case "MCP_CHANGED": return "yellow";
    case "PERMISSION_WILDCARD_ADDED": return "red";
    case "SKILL_EXECUTABLE_APPEARED": return "yellow";
    case "ENV_KEY_ADDED": return "green";
    case "ENV_KEY_REMOVED": return "red";
    case "UNSUPPORTED_STATE_CHANGED": return "yellow";
    default: return "white";
  }
}

function changeLabel(code: string): string {
  switch (code) {
    case "MCP_ADDED": return "MCP added";
    case "MCP_REMOVED": return "MCP removed";
    case "MCP_CHANGED": return "MCP changed";
    case "PERMISSION_WILDCARD_ADDED": return "Wildcard permission";
    case "SKILL_EXECUTABLE_APPEARED": return "Executable skill";
    case "ENV_KEY_ADDED": return "Env key added";
    case "ENV_KEY_REMOVED": return "Env key removed";
    case "UNSUPPORTED_STATE_CHANGED": return "State changed";
    default: return code;
  }
}

function rawStatusColor(status: string): string {
  switch (status) {
    case "added": return "green";
    case "removed": return "red";
    case "changed": return "yellow";
    default: return "white";
  }
}

// ── Component ────────────────────────────────────────────────

export default function DiffView({
  semanticChanges,
  rawSourceChanges,
}: DiffViewProps) {
  const hasChanges =
    semanticChanges.length > 0 ||
    (rawSourceChanges && rawSourceChanges.length > 0);

  if (!hasChanges) {
    return (
      <Box flexDirection="column">
        <Text bold color="green">gandalf diff</Text>
        <Text>No changes — baseline and current state match.</Text>
      </Box>
    );
  }

  const addedCount = semanticChanges.filter(
    (c) => c.code === "MCP_ADDED" || c.code === "ENV_KEY_ADDED"
  ).length;
  const removedCount = semanticChanges.filter(
    (c) => c.code === "MCP_REMOVED" || c.code === "ENV_KEY_REMOVED"
  ).length;
  const changedCount = semanticChanges.length - addedCount - removedCount;

  return (
    <Box flexDirection="column">
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold>gandalf diff</Text>
      </Box>

      {/* Summary badges */}
      <Box marginBottom={1} gap={2}>
        {addedCount > 0 && <Text color="green">+{addedCount} added</Text>}
        {removedCount > 0 && <Text color="red">-{removedCount} removed</Text>}
        {changedCount > 0 && (
          <Text color="yellow">~{changedCount} changed</Text>
        )}
      </Box>

      {/* Semantic changes */}
      {semanticChanges.length > 0 && (
        <Box marginBottom={1} flexDirection="column">
          <Text bold>Semantic changes ({semanticChanges.length})</Text>
          {semanticChanges.map((change, i) => {
            const color = changeColor(change.code);
            return (
              <Box key={i} marginLeft={1}>
                <Text color={color}>●</Text>{" "}
                <Text color={color} bold>
                  {changeLabel(change.code)}
                </Text>
                <Text> — {change.entityName}</Text>
                {change.details.sourcePath && (
                  <Text dimColor> ({change.details.sourcePath})</Text>
                )}
              </Box>
            );
          })}
        </Box>
      )}

      {/* Raw source changes */}
      {rawSourceChanges && rawSourceChanges.length > 0 && (
        <Box flexDirection="column">
          <Text bold>
            Raw source changes ({rawSourceChanges.length})
          </Text>
          {rawSourceChanges.map((change, i) => {
            const color = rawStatusColor(change.status);
            return (
              <Box key={i} marginLeft={1}>
                <Text color={color}>
                  {change.status === "added" ? "+" : change.status === "removed" ? "-" : "~"}
                </Text>{" "}
                <Text>{change.sourcePath}</Text>
                {change.afterChecksum && change.beforeChecksum && (
                  <Text dimColor>
                    {" "}
                    ({change.beforeChecksum.slice(0, 8)} →{" "}
                    {change.afterChecksum.slice(0, 8)})
                  </Text>
                )}
              </Box>
            );
          })}
        </Box>
      )}
    </Box>
  );
}