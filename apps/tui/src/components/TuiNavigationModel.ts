import type { AgentId, DiscoveredItem } from "@qxinm/gandalf-core/types.js";
import { buildAgentEntries } from "./Sidebar.js";
import { formatAgentLabel } from "./TuiFormatters.js";

export type TuiNavSectionId = "profiles" | "agents" | "history";
export type TuiNavItemKind = "profile" | "agent" | "history";
export type TuiScreenId = "profile" | "agent-detail" | "timeline" | "snapshots";

export interface TuiNavItem {
  id: string;
  kind: TuiNavItemKind;
  label: string;
  screen: TuiScreenId;
  agent?: AgentId;
  profile?: string;
  evidenceCount?: number;
}

export interface TuiNavSection {
  id: TuiNavSectionId;
  label: string;
  items: TuiNavItem[];
}

export interface TuiNavigationModel {
  sections: TuiNavSection[];
  flatItems: TuiNavItem[];
  selectedItemId: string;
  cursor: number;
}

export interface TuiNavigationSelection {
  screen: TuiScreenId;
  selectedAgent: AgentId | null;
  selectedProfile: string;
}

export const DEFAULT_PROFILE = "default";
export const INITIAL_NAV_ITEM_ID = "history:all-changes";

export function buildTuiNavigationModel(input: {
  evidence: Pick<DiscoveredItem, "agent" | "kind">[];
  selectedItemId?: string;
  cursor?: number;
}): TuiNavigationModel {
  const sections = buildTuiNavSections(input.evidence);
  const flatItems = sections.flatMap((section) => section.items);
  const selectedItemId = input.selectedItemId && flatItems.some((item) => item.id === input.selectedItemId)
    ? input.selectedItemId
    : INITIAL_NAV_ITEM_ID;
  const selectedIndex = flatItems.findIndex((item) => item.id === selectedItemId);
  const fallbackCursor = selectedIndex >= 0 ? selectedIndex : 0;
  const cursor = clampCursor(input.cursor ?? fallbackCursor, flatItems.length);

  return {
    sections,
    flatItems,
    selectedItemId,
    cursor
  };
}

export function buildTuiNavSections(evidence: Pick<DiscoveredItem, "agent" | "kind">[]): TuiNavSection[] {
  const agentItems = buildAgentEntries(evidence).flatMap<TuiNavItem>((agent) => {
    if (!agent.id) return [];

    return [{
      id: `agent:${agent.id}`,
      kind: "agent",
      label: formatAgentLabel(agent.id),
      screen: "agent-detail",
      agent: agent.id,
      evidenceCount: agent.evidenceCount
    }];
  });

  return [
    {
      id: "profiles",
      label: "Profiles",
      items: [
        {
          id: `profile:${DEFAULT_PROFILE}`,
          kind: "profile",
          label: DEFAULT_PROFILE,
          screen: "profile",
          profile: DEFAULT_PROFILE
        }
      ]
    },
    {
      id: "agents",
      label: "Agents",
      items: agentItems
    },
    {
      id: "history",
      label: "History",
      items: [
        {
          id: INITIAL_NAV_ITEM_ID,
          kind: "history",
          label: "All changes",
          screen: "timeline"
        },
        {
          id: "history:snapshots",
          kind: "history",
          label: "Snapshots",
          screen: "snapshots"
        }
      ]
    }
  ];
}

export function navItemIdForSelection(selection: TuiNavigationSelection): string {
  if (selection.screen === "timeline") {
    return selection.selectedAgent ? `agent:${selection.selectedAgent}` : INITIAL_NAV_ITEM_ID;
  }
  if (selection.screen === "snapshots") return "history:snapshots";
  if (selection.screen === "profile") return `profile:${selection.selectedProfile || DEFAULT_PROFILE}`;
  if (selection.screen === "agent-detail" && selection.selectedAgent) return `agent:${selection.selectedAgent}`;
  return INITIAL_NAV_ITEM_ID;
}

export function selectTuiNavItem(input: {
  item: TuiNavItem;
  currentScreen: TuiScreenId;
  currentAgent: AgentId | null;
  currentProfile?: string;
}): TuiNavigationSelection {
  if (input.item.kind === "agent") {
    return {
      screen: input.currentScreen === "timeline" ? "timeline" : "agent-detail",
      selectedAgent: input.item.agent ?? input.currentAgent,
      selectedProfile: input.currentProfile ?? DEFAULT_PROFILE
    };
  }

  return {
    screen: input.item.screen,
    selectedAgent: null,
    selectedProfile: input.item.profile ?? input.currentProfile ?? DEFAULT_PROFILE
  };
}

export function clampCursor(cursor: number, length: number): number {
  if (length <= 0) return 0;
  return Math.min(Math.max(0, cursor), length - 1);
}
