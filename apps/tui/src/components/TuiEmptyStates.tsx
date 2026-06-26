import React from "react";
import { Box, Text } from "ink";

export function NoSnapshotsEmptyState() {
  return (
    <Box flexDirection="column">
      <Text bold>Saved setups (0 total)</Text>
      <Text dimColor>No saved setups yet.</Text>
      <Text color="cyan">s save setup</Text>
    </Box>
  );
}

export function NoTimelineEventsEmptyState({ command }: { command?: string }) {
  return (
    <Box flexDirection="column">
      <Text dimColor>No timeline entries yet.</Text>
      {command && <Text color="cyan">{command}</Text>}
    </Box>
  );
}

export function NoDetectedAgentsEmptyState() {
  return (
    <Box flexDirection="column">
      <Text dimColor>No supported agent setup found.</Text>
      <Text dimColor>
        Gandalf looks for Claude Code, Codex, Cursor, OpenCode, Pi Agent, and project instruction files.
      </Text>
    </Box>
  );
}
