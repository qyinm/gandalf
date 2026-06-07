import type { DiscoveredItem, TimelineEntry } from "../../types.js";
import { formatAgentLabel, formatTimelineTimestamp } from "./TuiFormatters.js";

export interface ProfileViewModel {
  title: string;
  profiles: Array<{
    name: string;
    selected: boolean;
    snapshotCount: number;
    agents: string;
    changedAt: string;
  }>;
}

export function buildProfileViewModel(input: {
  evidence: Pick<DiscoveredItem, "agent">[];
  snapshotNames: string[];
  timelineEntries: TimelineEntry[];
  now?: Date;
}): ProfileViewModel {
  const agents = [...new Set(input.evidence.map((item) => item.agent))]
    .sort()
    .map(formatAgentLabel);
  const latest = [...input.timelineEntries].sort((left, right) =>
    right.observedAt.localeCompare(left.observedAt)
  )[0];

  return {
    title: "Profiles",
    profiles: [
      {
        name: "default",
        selected: true,
        snapshotCount: input.snapshotNames.length,
        agents: agents.length > 0 ? agents.join(", ") : "none",
        changedAt: latest ? formatTimelineTimestamp(latest.observedAt, input.now) : "-"
      }
    ]
  };
}
