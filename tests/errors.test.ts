import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { formatSnapError } from "../src/errors.js";

describe("error contract", () => {
  it("formats problem, cause, fix, and path when present", () => {
    const output = formatSnapError({
      code: "HEM_PARSE_FAILED",
      problem: "Could not parse Codex config.",
      cause: "TOML syntax error at line 12.",
      fix: "Run `hem scan --skip codex` or fix the TOML file.",
      path: "~/.codex/config.toml"
    });

    assert.match(output, /^HEM_PARSE_FAILED/);
    assert.match(output, /Problem: Could not parse Codex config\./);
    assert.match(output, /Cause: TOML syntax error at line 12\./);
    assert.match(output, /Fix: Run `hem scan --skip codex` or fix the TOML file\./);
    assert.match(output, /Path: ~\/\.codex\/config\.toml/);
  });
});
