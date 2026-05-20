/**
 * Simple table renderer using Ink Box + Text primitives.
 *
 * Replaces ink-table which has CJS/ESM compatibility issues
 * with our ESM NodeNext TypeScript setup.
 */

import React from "react";
import { Text, Box } from "ink";

interface SimpleTableProps {
  data: Record<string, string>[];
  columns: string[];
  padding?: number;
}

export default function SimpleTable({
  data,
  columns,
  padding = 1,
}: SimpleTableProps) {
  if (data.length === 0) return null;

  // Calculate column widths
  const widths = columns.map((col) => {
    const headerLen = col.length;
    const maxDataLen = Math.max(
      0,
      ...data.map((row) => String(row[col] ?? "").length)
    );
    return Math.max(headerLen, maxDataLen) + padding * 2;
  });

  // Header
  const headerLine = columns
    .map((col, i) => col.padEnd(widths[i]))
    .join("");

  // Separator
  const sepLine = widths.map((w) => "─".repeat(w)).join("");

  // Data rows
  const rows = data.map((row) =>
    columns.map((col, ci) => String(row[col] ?? "").padEnd(widths[ci])).join("")
  );

  return (
    <Box flexDirection="column">
      <Text bold>{headerLine}</Text>
      <Text dimColor>{sepLine}</Text>
      {rows.map((row, i) => (
        <Text key={i}>{row}</Text>
      ))}
    </Box>
  );
}