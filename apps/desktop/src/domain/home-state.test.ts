import { describe, expect, it } from "bun:test";

import { homeStateIsActionable, type DesktopHomeState } from "./home-state";

describe("homeStateIsActionable", () => {
  it("accepts protected state with a short snapshot id", () => {
    const state: DesktopHomeState = {
      activeProfile: {
        name: "Default",
        scope: "personal",
        syncState: "local_only",
        ahead: 0,
        behind: 0
      },
      currentSnapshotId: "8f3a2c7",
      protection: "on",
      highestRisk: "low",
      workingChanges: 0
    };

    expect(homeStateIsActionable(state)).toBe(true);
  });

  it("rejects unprotected state", () => {
    const state: DesktopHomeState = {
      activeProfile: {
        name: "Default",
        scope: "personal",
        syncState: "local_only",
        ahead: 0,
        behind: 0
      },
      currentSnapshotId: "8f3a2c7",
      protection: "off",
      highestRisk: "medium",
      workingChanges: 3
    };

    expect(homeStateIsActionable(state)).toBe(false);
  });
});
