import React from "react";
import { Box, Text } from "ink";

import type { GraphDiff } from "@qxinm/hem-core/diff.js";
import { buildSaveSetupViewModel } from "./SaveSetupViewModel.js";

interface SaveSetupViewProps {
  diff?: GraphDiff;
  hasPreviousSnapshot: boolean;
  saving?: boolean;
  savedName?: string;
  error?: string;
}

export default function SaveSetupView({
  diff,
  hasPreviousSnapshot,
  saving = false,
  savedName,
  error
}: SaveSetupViewProps) {
  const model = buildSaveSetupViewModel({ diff, hasPreviousSnapshot });

  return (
    <Box flexDirection="column">
      <Text bold>Save current setup?</Text>

      <Box flexDirection="column" marginTop={1}>
        <Text bold>{model.noChanges ? "No detected changes" : "Detected changes"}</Text>
        {model.detectedChanges.map((change) => (
          <Text key={change}>  {change}</Text>
        ))}
      </Box>

      <Box flexDirection="column" marginTop={1}>
        <Text>This will be saved as</Text>
        <Text color="cyan">  {model.title}</Text>
      </Box>

      <Box flexDirection="column" marginTop={1}>
        <Text bold>Destinations</Text>
        {model.destinations.map((destination) => (
          <Text key={destination.label}>
            {"  "}{destination.selected ? "[x]" : "[ ]"} {destination.label}
            {destination.note ? `  ${destination.note}` : ""}
          </Text>
        ))}
      </Box>

      {saving && (
        <Box marginTop={1}>
          <Text color="yellow">Saving...</Text>
        </Box>
      )}
      {savedName && (
        <Box marginTop={1}>
          <Text color="green">Saved {savedName}</Text>
        </Box>
      )}
      {error && (
        <Box marginTop={1}>
          <Text color="red">{error}</Text>
        </Box>
      )}

      <Box marginTop={1}>
        <Text dimColor>s save  Esc back  q quit</Text>
      </Box>
    </Box>
  );
}
