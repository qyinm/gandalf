import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { daemonTrustHeaderModel } from "../src/tui/components/Dashboard.js";
import type { DaemonStatusReadResult } from "../src/types.js";

function statusResult(overrides: Partial<DaemonStatusReadResult["status"]> = {}): DaemonStatusReadResult {
  return {
    ok: true,
    status: {
      running: true,
      pid: 123,
      identityHash: "sha256:test",
      startedAt: "2026-06-08T00:00:00.000Z",
      lastHeartbeatAt: "2026-06-08T00:00:01.000Z",
      lastEventAt: "2026-06-08T00:00:02.000Z",
      runId: "run-test",
      projectPath: "/project",
      storeDir: "/store",
      watchedPaths: ["/project/.mcp.json", "/home/.claude/settings.json"],
      stale: false,
      errors: [],
      ...overrides,
      identityVerified: overrides.identityVerified ?? true
    }
  };
}

function statusReadError(): DaemonStatusReadResult {
  return {
    ok: false,
    error: "status unreadable",
    status: statusResult({
      running: false,
      pidAlive: false,
      identityVerified: false,
      stale: true,
      errors: ["status unreadable"]
    }).status
  };
}

describe("TUI daemon trust header", () => {
  it("renders checking state before daemon status is loaded", () => {
    const model = daemonTrustHeaderModel(null);

    assert.equal(model.title, "Daemon: checking...");
    assert.equal(model.color, "yellow");
    assert.equal(model.lastEvent, "-");
  });

  it("renders running daemon trust metadata", () => {
    const model = daemonTrustHeaderModel(statusResult());

    assert.equal(model.title, "Daemon: running");
    assert.equal(model.color, "green");
    assert.equal(model.lastEvent, "2026-06-08T00:00:02.000Z");
    assert.equal(model.watchedCount, 2);
    assert.equal(model.storeDir, "/store");
  });

  it("renders stopped daemon state", () => {
    const model = daemonTrustHeaderModel(statusResult({
      running: false,
      pidAlive: false,
      identityVerified: false,
      stale: false
    }));

    assert.equal(model.title, "Daemon: stopped");
    assert.equal(model.color, "yellow");
    assert.equal(model.stale, false);
  });

  it("renders stale daemon warning", () => {
    const model = daemonTrustHeaderModel(statusResult({ running: false, stale: true }));

    assert.equal(model.title, "Daemon: stale");
    assert.equal(model.color, "red");
    assert.equal(model.stale, true);
  });

  it("renders daemon status read errors", () => {
    const model = daemonTrustHeaderModel(statusReadError());

    assert.equal(model.title, "Daemon: error");
    assert.equal(model.color, "red");
    assert.equal(model.error, "status unreadable");
  });
});
