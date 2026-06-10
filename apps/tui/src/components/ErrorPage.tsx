/**
 * Ink component for structured SnapError display.
 *
 * Renders a clean, visually distinct error card when commands fail.
 */

import React from "react";
import { Text, Box } from "ink";

interface ErrorPageProps {
  code: string;
  problem: string;
  cause: string;
  fix: string;
  path?: string;
}

export default function ErrorPage({
  code,
  problem,
  cause,
  fix,
  path,
}: ErrorPageProps) {
  return (
    <Box flexDirection="column" paddingX={1}>
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold color="red">
          ✗ {code}
        </Text>
      </Box>

      {/* Problem */}
      <Box marginBottom={1}>
        <Text bold>Problem: </Text>
        <Text>{problem}</Text>
      </Box>

      {/* Cause */}
      <Box marginBottom={1}>
        <Text bold>Cause: </Text>
        <Text>{cause}</Text>
      </Box>

      {/* Fix */}
      <Box marginBottom={1}>
        <Text bold color="yellow">
          Fix:{" "}
        </Text>
        <Text>{fix}</Text>
      </Box>

      {/* Optional path */}
      {path && (
        <Box marginBottom={1}>
          <Text bold>Path: </Text>
          <Text dimColor>{path}</Text>
        </Box>
      )}
    </Box>
  );
}