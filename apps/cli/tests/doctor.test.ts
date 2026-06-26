import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, it } from "node:test";

import { buildReadinessReport } from "@qxinm/gandalf-core/readiness.js";
import { scanProject } from "@qxinm/gandalf-core/scan.js";
import type { DiscoveredItem, McpServerValue } from "@qxinm/gandalf-core/types.js";

function mcpItem(id: string, value: McpServerValue): DiscoveredItem {
  return {
    id,
    agent: "claude-code",
    kind: "mcp_server",
    sourcePath: ".mcp.json",
    scope: "project",
    precedence: 40,
    parser: "json",
    sensitivity: "command_config",
    contentPolicy: "structured_safe_fields_only",
    restorePolicy: "structured_fields_only",
    captureStatus: "captured",
    confidence: "high",
    name: id,
    value
  };
}

function envItem(key: string): DiscoveredItem {
  return {
    id: `env.${key}`,
    agent: "project",
    kind: "env_key",
    sourcePath: ".env",
    scope: "project",
    precedence: 40,
    parser: "dotenv",
    sensitivity: "env_key_inventory",
    contentPolicy: "key_inventory_only",
    restorePolicy: "key_inventory_only",
    captureStatus: "redacted",
    confidence: "high",
    name: key,
    value: { key }
  };
}

describe("readiness analyzer", () => {
  it("classifies MCP command states without executing shell strings", async () => {
    const root = await mkdtemp(path.join(tmpdir(), "gandalf-doctor-"));
    const markerPath = path.join(root, "shell-marker");
    const maliciousCommand = `missing\" ; touch \"${markerPath}\" ; \"`;
    const report = buildReadinessReport([
      mcpItem("mcp-remote", { url: "https://mcp.example.test" }),
      mcpItem("mcp-local", { command: "/Users/source/.local/bin/private-mcp" }),
      mcpItem("mcp-malicious", { command: maliciousCommand })
    ], {
      sourceHomeDir: "/Users/source",
      processEnv: {}
    });

    assert.equal(report.summary.unverified, 1);
    assert.equal(report.items.find((item) => item.evidenceId === "mcp-remote")?.category, "unverified");
    assert.equal(report.items.find((item) => item.evidenceId === "mcp-local")?.category, "needs_manual_action");
    assert.equal(report.items.find((item) => item.evidenceId === "mcp-malicious")?.category, "needs_manual_action");
    await assert.rejects(import("node:fs/promises").then(({ readFile }) => readFile(markerPath, "utf8")), /ENOENT/);
  });

  it("redacts credentials embedded in remote MCP URLs", () => {
    const report = buildReadinessReport([
      mcpItem("mcp-remote-secret", {
        url: "https://token:secret@mcp.example.test/sse?api_key=sk-real-secret&mode=read#access_token=fragment-secret"
      })
    ]);
    const output = JSON.stringify(report);

    assert.equal(output.includes("token:secret"), false);
    assert.equal(output.includes("sk-real-secret"), false);
    assert.equal(output.includes("fragment-secret"), false);
    assert.equal(output.includes("api_key=%5Bredacted%5D"), true);
    assert.equal(output.includes("mode=read"), true);
  });

  it("ignores malformed legacy MCP payload fields instead of crashing readiness", () => {
    const legacyEvidence = JSON.parse(JSON.stringify([
      {
        id: "legacy-bad-mcp",
        agent: "claude-code",
        kind: "mcp_server",
        sourcePath: ".mcp.json",
        scope: "project",
        precedence: 40,
        parser: "json",
        sensitivity: "command_config",
        contentPolicy: "structured_safe_fields_only",
        restorePolicy: "structured_fields_only",
        captureStatus: "captured",
        confidence: "high",
        value: { command: 123, url: true, args: ["--ok"] }
      }
    ])) as DiscoveredItem[];

    const report = buildReadinessReport(legacyEvidence, { processEnv: {} });

    assert.equal(report.items.some((item) => item.evidenceId === "legacy-bad-mcp"), false);
  });

  it("does not execute a PATH-hijacked which helper during command lookup", async () => {
    const root = await mkdtemp(path.join(tmpdir(), "gandalf-path-hijack-"));
    const markerPath = path.join(root, "which-marker");
    const fakeWhich = path.join(root, "which");
    await writeFile(fakeWhich, `#!/bin/sh\ntouch "${markerPath}"\nexit 0\n`, "utf8");
    await chmod(fakeWhich, 0o755);
    const previousPath = process.env.PATH;
    process.env.PATH = `${root}${path.delimiter}${previousPath ?? ""}`;
    try {
      const report = buildReadinessReport([mcpItem("mcp-missing", { command: "definitely-missing-gandalf-tool" })]);
      assert.equal(report.items.some((item) => item.code === "GANDALF_MCP_COMMAND_MISSING"), true);
      await assert.rejects(import("node:fs/promises").then(({ readFile }) => readFile(markerPath, "utf8")), /ENOENT/);
    } finally {
      process.env.PATH = previousPath;
    }
  });

  it("reports missing env keys by name only", () => {
    const report = buildReadinessReport([
      envItem("OPENAI_API_KEY"),
      mcpItem("mcp-env", { command: "npx", envKeys: ["GITHUB_TOKEN"] })
    ], {
      targetEvidence: [],
      processEnv: {}
    });

    const envItems = report.items.filter((entry) => entry.code === "GANDALF_ENV_VALUE_REQUIRED");
    assert.equal(envItems.length, 2);
    assert.equal(envItems.some((entry) => entry.problem.includes("OPENAI_API_KEY")), true);
    assert.equal(envItems.some((entry) => entry.problem.includes("GITHUB_TOKEN")), true);
    assert.equal(JSON.stringify(report).includes("sk-"), false);
  });

  it("treats target .env inventory and process env presence as satisfying env keys", () => {
    const report = buildReadinessReport([envItem("OPENAI_API_KEY"), envItem("GITHUB_TOKEN")], {
      targetEvidence: [envItem("OPENAI_API_KEY")],
      processEnv: { GITHUB_TOKEN: "present-but-never-rendered" }
    });

    assert.equal(report.items.some((entry) => entry.code === "GANDALF_ENV_VALUE_REQUIRED"), false);
    assert.equal(JSON.stringify(report).includes("present-but-never-rendered"), false);
  });

  it("scans current project env keys for doctor input", async () => {
    const root = await mkdtemp(path.join(tmpdir(), "gandalf-doctor-scan-"));
    const projectPath = path.join(root, "project");
    const homeDir = path.join(root, "home");
    const storeDir = path.join(root, "store");
    await mkdir(projectPath, { recursive: true });
    await mkdir(homeDir, { recursive: true });
    await writeFile(path.join(projectPath, ".env"), "OPENAI_API_KEY=secret\n", "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const report = buildReadinessReport(scan.evidence, {
      targetEvidence: scan.evidence,
      processEnv: {}
    });

    assert.equal(report.items.some((entry) => entry.code === "GANDALF_ENV_VALUE_REQUIRED"), false);
  });
});
