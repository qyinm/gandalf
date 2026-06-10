import React from "react";
import { Box, Text } from "ink";

import type { DiscoveredItem, TimelineEntry } from "@qxinm/hem-core/types.js";
import { buildProfileViewModel } from "./ProfileViewModel.js";

interface ProfileViewProps {
  evidence: DiscoveredItem[];
  snapshotNames: string[];
  timelineEntries: TimelineEntry[];
}

export default function ProfileView({
  evidence,
  snapshotNames,
  timelineEntries
}: ProfileViewProps) {
  const model = buildProfileViewModel({
    evidence,
    snapshotNames,
    timelineEntries
  });

  return (
    <Box flexDirection="column">
      <Text bold>{model.title}</Text>
      {model.profiles.map((profile) => (
        <Box key={profile.name} flexDirection="column" marginTop={1}>
          <Text>{profile.selected ? "* " : "  "}{profile.name}</Text>
          <Text dimColor>  {profile.snapshotCount} snapshots</Text>
          <Text dimColor>  {profile.agents}</Text>
          <Text dimColor>  changed {profile.changedAt}</Text>
        </Box>
      ))}

      <Box marginTop={1}>
        <Text dimColor>Enter switch  s save  c compare</Text>
      </Box>
    </Box>
  );
}
