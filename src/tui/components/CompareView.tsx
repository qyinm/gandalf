import React from "react";
import { Box, Text } from "ink";

import type { GraphDiff } from "../../diff.js";
import type { Snapshot } from "../../types.js";
import { buildCompareViewModel } from "./CompareViewModel.js";
import { padDisplay } from "./TuiFormatters.js";

interface CompareViewProps {
  fromSnapshot: Snapshot;
  toSnapshot: Snapshot;
  diff: GraphDiff;
  toLabel?: string;
}

export default function CompareView({
  fromSnapshot,
  toSnapshot,
  diff,
  toLabel
}: CompareViewProps) {
  const model = buildCompareViewModel({
    fromSnapshot,
    toSnapshot,
    diff,
    toLabel,
    scope: "Full setup"
  });

  return (
    <Box flexDirection="column">
      <Text bold>Compare</Text>
      <Text>From  {model.fromLabel}</Text>
      <Text>To    {model.toLabel}</Text>
      <Text>Scope {model.scopeLabel}</Text>

      <Box flexDirection="column" marginTop={1}>
        <Text bold>Summary</Text>
        {model.summary.map((row) => (
          <Text key={row}>  {row}</Text>
        ))}
      </Box>

      <Box flexDirection="column" marginTop={1}>
        <Text bold>Side-by-side</Text>
        {model.sections.map((section) => (
          <Box key={section.title} flexDirection="column" marginTop={1}>
            <Text>{section.title}</Text>
            <Text dimColor>
              {"  "}{padDisplay(fromSnapshot.manifest.name, 30)}  {padDisplay(model.toLabel, 30)}
            </Text>
            {section.rows.slice(0, 12).map((row, index) => (
              <Text key={`${section.title}:${index}`}>
                {row.marker} {padDisplay(row.before, 30)}  {padDisplay(row.after, 30)}
              </Text>
            ))}
          </Box>
        ))}
      </Box>

      <Box marginTop={1}>
        <Text dimColor>r restore from left  s save current  Esc back</Text>
      </Box>
    </Box>
  );
}
