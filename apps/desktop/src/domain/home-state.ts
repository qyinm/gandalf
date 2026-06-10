export type SyncState = "local_only" | "up_to_date" | "ahead" | "behind" | "diverged";
export type RiskLevel = "low" | "medium" | "high";

export interface DesktopHomeState {
  activeProfile: {
    name: string;
    scope: "personal" | "team";
    team?: string;
    syncState: SyncState;
    ahead: number;
    behind: number;
  } | null;
  currentSnapshotId: string | null;
  protection: "on" | "off";
  highestRisk: RiskLevel | null;
  workingChanges: number;
}

export function homeStateIsActionable(state: DesktopHomeState): boolean {
  return state.protection === "on" && Boolean(state.activeProfile) && Boolean(state.currentSnapshotId);
}
