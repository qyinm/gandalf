import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { spawnSync } from "node:child_process";

describe("snaptailor CLI scaffold", () => {
  it("prints help with the v0.1 read-only commands", () => {
    const result = spawnSync(process.execPath, ["dist/src/cli.js", "--help"], {
      cwd: process.cwd(),
      encoding: "utf8"
    });

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /snaptailor scan --project/);
    assert.match(result.stdout, /snapshot create --name baseline --metadata-only/);
    assert.match(result.stdout, /diff baseline current --project/);
    assert.match(result.stdout, /audit current --project/);
    assert.match(result.stdout, /provenance current --project/);
    assert.match(result.stdout, /report current --project/);
  });
});
