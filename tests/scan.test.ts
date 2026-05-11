import assert from "node:assert/strict";
import { mkdir, mkdtemp, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";

import { scanProject } from "../src/scan.js";

async function makeSandbox(): Promise<{ root: string; projectPath: string; homeDir: string; storeDir: string }> {
  const root = await mkdtemp(join(tmpdir(), "snaptailor-scan-"));
  const projectPath = join(root, "project");
  const homeDir = join(root, "home");
  const storeDir = join(homeDir, ".snaptailor");

  await mkdir(projectPath, { recursive: true });
  await mkdir(homeDir, { recursive: true });

  return { root, projectPath, homeDir, storeDir };
}

describe("scanProject", () => {
  it("discovers project MCP config and reports read-only trust preflight", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await writeFile(
      join(projectPath, ".mcp.json"),
      JSON.stringify({ mcpServers: { github: { command: "gh", args: ["api"], env: { GITHUB_TOKEN: "secret" } } } }),
      "utf8"
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });

    assert.deepEqual(scan.trust, {
      readOnly: true,
      network: "disabled",
      commandsExecuted: [],
      storeWriteLocation: storeDir
    });
    assert.ok(
      scan.evidence.some(
        (item) =>
          item.kind === "mcp_server" &&
          item.name === "github" &&
          item.sourcePath === ".mcp.json" &&
          item.scope === "project" &&
          item.captureStatus === "captured"
      )
    );
    assert.doesNotMatch(JSON.stringify(scan.evidence), /secret|GITHUB_TOKEN:[^"}]+/i);
  });

  it("emits parse_failed evidence for malformed JSON instead of throwing", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(projectPath, ".claude"), { recursive: true });
    await writeFile(join(projectPath, ".claude", "settings.json"), "{ not json", "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });

    assert.ok(
      scan.evidence.some(
        (item) =>
          item.sourcePath === ".claude/settings.json" &&
          item.parser === "json" &&
          item.captureStatus === "parse_failed"
      )
    );
  });

  it("records symlink evidence without following it", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await writeFile(join(projectPath, "CLAUDE.md"), "project instructions", "utf8");
    await symlink(join(projectPath, "CLAUDE.md"), join(projectPath, "AGENTS.md"));

    const scan = await scanProject({ projectPath, homeDir, storeDir });

    assert.ok(
      scan.evidence.some(
        (item) =>
          item.kind === "symlink" &&
          item.sourcePath === "AGENTS.md" &&
          item.captureStatus === "omitted" &&
          item.metadata?.["reason"] === "symlink_not_followed"
      )
    );
    assert.equal(scan.evidence.some((item) => item.sourcePath === "AGENTS.md" && item.kind === "agent_instruction"), false);
  });

  it("captures dotenv key inventory while omitting secret-like values", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await writeFile(join(projectPath, ".env"), "OPENAI_API_KEY=sk-real-secret\nSNAPTAILOR_MODE=local\n", "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });

    assert.ok(
      scan.evidence.some(
        (item) =>
          item.kind === "env_key" &&
          item.name === "OPENAI_API_KEY" &&
          item.captureStatus === "redacted" &&
          item.value === undefined
      )
    );
    assert.ok(
      scan.evidence.some(
        (item) =>
          item.kind === "env_key" &&
          item.name === "SNAPTAILOR_MODE" &&
          item.captureStatus === "omitted" &&
          item.value === undefined
      )
    );
    assert.doesNotMatch(JSON.stringify(scan.evidence), /sk-real-secret|local/);
  });

  it("ignores node_modules while discovering project files", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(projectPath, "node_modules", "pkg"), { recursive: true });
    await writeFile(join(projectPath, "node_modules", "pkg", ".mcp.json"), JSON.stringify({ mcpServers: { ignored: {} } }), "utf8");
    await writeFile(join(projectPath, "AGENTS.md"), "agent instructions", "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });

    assert.ok(scan.evidence.some((item) => item.sourcePath === "AGENTS.md"));
    assert.equal(scan.evidence.some((item) => item.sourcePath.includes("node_modules")), false);
    assert.equal(scan.evidence.some((item) => item.name === "ignored"), false);
  });
});
