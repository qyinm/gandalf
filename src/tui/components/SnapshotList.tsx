/**
 * Ink component for hem snapshot list.
 *
 * Paginated table of snapshots with name, date, and metadata.
 */

import React, { useState } from "react";
import { Text, Box } from "ink";
import SimpleTable from "./SimpleTable.js";
import { buildSnapshotListViewModel } from "./SnapshotListViewModel.js";

import type { Snapshot } from "../../types.js";

// ── Props ────────────────────────────────────────────────────

interface SnapshotListProps {
  names: string[];
  snapshots?: Record<string, Snapshot>;  // name → snapshot for detail view
  pageSize?: number;
}

// ── Component ────────────────────────────────────────────────

export default function SnapshotList({
  names,
  snapshots,
  pageSize = 20,
}: SnapshotListProps) {
  const [page, setPage] = useState(0);
  const model = buildSnapshotListViewModel({ names });
  const totalPages = Math.ceil(names.length / pageSize);
  const pageNames = names.slice(page * pageSize, (page + 1) * pageSize);

  if (names.length === 0) {
    return (
      <Box flexDirection="column">
        <Text bold>{model.title}</Text>
        <Text dimColor>{model.emptyMessage}</Text>
        <Text color="cyan">{model.emptyAction}</Text>
      </Box>
    );
  }

  // ── Table data ────────────────────────────────────────────
  // Load abbreviated details from each snapshot manifest
  const tableData = pageNames.map((name) => {
    const snapshot = snapshots?.[name];
    const createdAt = snapshot?.manifest.createdAt ?? "";
    const dateStr = createdAt
      ? new Date(createdAt).toLocaleDateString("en-CA")
      : "";
    const evidenceCount = snapshot?.evidence.length ?? "?";
    const agentCount = snapshot
      ? new Set(snapshot.evidence.map((e: { agent: string }) => e.agent)).size
      : "?";

    return {
      name,
      created: dateStr,
      agents: String(agentCount),
      evidence: String(evidenceCount),
    };
  });

  return (
    <Box flexDirection="column">
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold>{model.title}</Text>
      </Box>

      {/* Table */}
      <SimpleTable
        data={tableData}
        columns={["name", "created", "agents", "evidence"]}
      />

      {/* Pagination */}
      {totalPages > 1 && (
        <Box marginTop={1}>
          <Text dimColor>
            Page {page + 1} / {totalPages}
          </Text>
          {page > 0 && (
            <Box marginLeft={2}>
              <Text color="cyan">← prev (p)</Text>
            </Box>
          )}
          {page < totalPages - 1 && (
            <Box marginLeft={2}>
              <Text color="cyan">next (n) →</Text>
            </Box>
          )}
        </Box>
      )}
    </Box>
  );
}
