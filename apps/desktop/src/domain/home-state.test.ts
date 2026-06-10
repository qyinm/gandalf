import { describe, expect, it } from "bun:test";

import { homeStateIsActionable, type DesktopHomeState } from "./home-state";

describe("homeStateIsActionable", () => {
  it("accepts a protected captured profile", () => {
    const state: DesktopHomeState = {
      activeProfile: {
        name: "Captured",
        scope: "personal",
        syncState: "local_only",
        ahead: 0,
        behind: 0
      },
      currentSnapshotId: "snapshot-id",
      protection: "on",
      highestRisk: "low",
      workingChanges: 0
    };

    expect(homeStateIsActionable(state)).toBe(true);
  });

  it("rejects an uncaptured state", () => {
    const state: DesktopHomeState = {
      activeProfile: null,
      currentSnapshotId: null,
      protection: "on",
      highestRisk: null,
      workingChanges: 0
    };

    expect(homeStateIsActionable(state)).toBe(false);
  });
});
