/**
 * Tab bar for hem TUI dashboard main panel.
 *
 * Horizontal navigation: ← → or 1-5 keys.
 */
import React from "react";
import { Text, Box } from "ink";

export type TabId = "timeline" | "snapshots" | "scan" | "audit" | "diff";

interface TabItem {
  id: TabId;
  label: string;
}

const TABS: TabItem[] = [
  { id: "timeline", label: "Timeline" },
  { id: "snapshots", label: "Snapshots" },
  { id: "scan", label: "Scan" },
  { id: "audit", label: "Audit" },
  { id: "diff", label: "Diff" },
];

interface TabBarProps {
  activeTab: TabId;
  onTabChange: (tab: TabId) => void;
}

export function tabLabel(id: TabId): string {
  return TABS.find((t) => t.id === id)?.label ?? id;
}

export { TABS };

export default function TabBar({ activeTab, onTabChange }: TabBarProps) {
  return (
    <Box flexDirection="row" gap={1} marginBottom={1}>
      {TABS.map((tab, i) => (
        <Text
          key={tab.id}
          bold={tab.id === activeTab}
          color={tab.id === activeTab ? "cyan" : "dim"}
          inverse={tab.id === activeTab}
        >
          {" "}{i + 1}. {tab.label}{" "}
        </Text>
      ))}
      <Box marginLeft={2}>
        <Text dimColor>s=save  r=rescan</Text>
      </Box>
    </Box>
  );
}
