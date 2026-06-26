import assert from "node:assert/strict";
import { mkdtemp, readFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { describe, it } from "node:test";
import {
  checkForUpdate,
  noticeForLatestVersion,
  shouldCheckForUpdates
} from "../src/update-check.js";

async function makeHome(): Promise<string> {
  return await mkdtemp(join(tmpdir(), "gandalf-update-check-"));
}

describe("update check", () => {
  it("formats an update notice with the sparkle emoji", () => {
    const notice = noticeForLatestVersion("99.0.0");

    assert.ok(notice);
    assert.equal(notice.latestVersion, "99.0.0");
    assert.match(notice.message, /^✨ gandalf 99\.0\.0 is available\./);
    assert.match(notice.message, /bun install -g @qxinm\/gandalf/);
  });

  it("requires explicit opt-in before any network update check", () => {
    assert.equal(shouldCheckForUpdates({
      args: ["scan"],
      homeDir: "/tmp/home",
      stderrIsTty: true
    }), false);
    assert.equal(shouldCheckForUpdates({
      args: ["scan"],
      homeDir: "/tmp/home",
      env: { GANDALF_UPDATE_CHECK: "1" },
      stderrIsTty: true
    }), true);
  });

  it("skips checks that could break automation output even when opted in", () => {
    assert.equal(shouldCheckForUpdates({
      args: ["scan", "--json"],
      homeDir: "/tmp/home",
      env: { GANDALF_UPDATE_CHECK: "1" },
      stderrIsTty: true
    }), false);
    assert.equal(shouldCheckForUpdates({
      args: ["scan"],
      homeDir: "/tmp/home",
      env: { GANDALF_UPDATE_CHECK: "1" },
      stderrIsTty: false
    }), false);
    assert.equal(shouldCheckForUpdates({
      args: ["scan"],
      homeDir: "/tmp/home",
      env: { CI: "true", GANDALF_UPDATE_CHECK: "1" },
      stderrIsTty: true
    }), false);
    assert.equal(shouldCheckForUpdates({
      args: ["scan"],
      homeDir: "/tmp/home",
      env: { GANDALF_UPDATE_CHECK: "0" },
      stderrIsTty: true
    }), false);
  });

  it("caches the latest version for the daily check window", async () => {
    const homeDir = await makeHome();
    let fetches = 0;

    const first = await checkForUpdate({
      args: ["scan"],
      homeDir,
      now: 1_000,
      fetchLatestVersion: async () => {
        fetches += 1;
        return "99.0.0";
      }
    });
    const second = await checkForUpdate({
      args: ["scan"],
      homeDir,
      now: 2_000,
      fetchLatestVersion: async () => {
        fetches += 1;
        return "100.0.0";
      }
    });

    assert.equal(fetches, 1);
    assert.equal(first?.latestVersion, "99.0.0");
    assert.equal(second?.latestVersion, "99.0.0");

    const cache = JSON.parse(await readFile(join(homeDir, ".gandalf", "update-check.json"), "utf8"));
    assert.equal(cache.latestVersion, "99.0.0");
  });
});
