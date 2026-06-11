import { describe, expect, it } from "bun:test";

import {
  homeStateIsActionable,
  normalizeDesktopHomeState,
  surfaceById,
  type DesktopHomeState
} from "./home-state";

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
      workingChanges: 0,
      changelog: [],
      surfaces: [],
      inventory: { skills: [], mcp: [], hooks: [] }
    };

    expect(homeStateIsActionable(state)).toBe(true);
  });

  it("rejects an uncaptured state", () => {
    const state: DesktopHomeState = {
      activeProfile: null,
      currentSnapshotId: null,
      protection: "on",
      highestRisk: null,
      workingChanges: 0,
      changelog: [],
      surfaces: [],
      inventory: { skills: [], mcp: [], hooks: [] }
    };

    expect(homeStateIsActionable(state)).toBe(false);
  });
});

describe("normalizeDesktopHomeState", () => {
  it("defaults malformed payload fields before they reach the UI", () => {
    const state = normalizeDesktopHomeState({
        activeProfile: { name: "Bad profile", scope: "team", syncState: "invalid" },
        currentSnapshotId: "",
        protection: "maybe",
        highestRisk: "critical",
        workingChanges: -3,
      changelog: [{ id: "1", title: "Skipped", time: "now", source: "unknown", risk: "low" }],
      surfaces: [{ id: "setup", label: "Setup", count: 1, risk: "medium", description: "Wrong boundary" }],
      inventory: {
          skills: [{ id: "", name: "Skipped", agent: "codex", sourcePath: "~/.codex/skills/skipped", scope: "user" }]
        }
      });

    expect(state.activeProfile).toBeNull();
    expect(state.inventory).toEqual({ skills: [], mcp: [], hooks: [] });
    expect(state.surfaces.map((surface) => [surface.id, surface.count])).toEqual([
      ["mcp", 0],
      ["skills", 0],
      ["hooks", 0]
    ]);
  });

  it("keeps valid known setup surfaces addressable by route id", () => {
    const state = normalizeDesktopHomeState({
      activeProfile: {
        name: "Captured",
        scope: "personal",
        syncState: "local_only",
        ahead: 0,
        behind: 0
      },
      currentSnapshotId: "snapshot-id",
      protection: "on",
      highestRisk: "medium",
      workingChanges: 2,
      changelog: [{ id: "c1", title: "Captured MCP", time: "1m ago", source: "manual", risk: "medium" }],
      surfaces: [{ id: "mcp", label: "MCP", count: 99, risk: "medium", description: "Two servers" }],
      inventory: {
        mcp: [
          {
            id: "mcp:github",
            name: "github",
            agent: "Claude Code",
            sourcePath: ".mcp.json",
            scope: "project",
            status: "enabled"
          },
          {
            id: "mcp:linear",
            name: "linear",
            agent: "Codex",
            sourcePath: "~/.codex/config.toml",
            scope: "user"
          }
        ]
      }
    });

    expect(surfaceById(state.surfaces, "mcp")?.count).toBe(2);
    expect(state.inventory.mcp.map((item) => item.name)).toEqual(["github", "linear"]);
    expect(state.changelog).toHaveLength(1);
  });
});
