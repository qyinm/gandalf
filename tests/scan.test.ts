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

  it("discovers OpenCode native and compatible skill roots", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    const skillPaths = [
      join(projectPath, ".opencode", "skills", "project-open", "SKILL.md"),
      join(homeDir, ".config", "opencode", "skills", "global-open", "SKILL.md"),
      join(projectPath, ".claude", "skills", "project-claude", "SKILL.md"),
      join(homeDir, ".claude", "skills", "global-claude", "SKILL.md"),
      join(projectPath, ".agents", "skills", "project-agent", "SKILL.md"),
      join(homeDir, ".agents", "skills", "global-agent", "SKILL.md"),
    ];

    for (const skillPath of skillPaths) {
      const skillName = skillPath.split("/").at(-2)!;
      await mkdir(join(skillPath, ".."), { recursive: true });
      await writeFile(skillPath, `---\nname: ${skillName}\ndescription: ${skillName} skill\n---\n`, "utf8");
    }

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const opencodeSkillSources = scan.evidence
      .filter((item) => item.agent === "opencode" && item.kind === "skill")
      .map((item) => item.sourcePath);

    assert.ok(opencodeSkillSources.includes(".opencode/skills/project-open"));
    assert.ok(opencodeSkillSources.includes("~/.config/opencode/skills/global-open"));
    assert.ok(opencodeSkillSources.includes(".claude/skills/project-claude"));
    assert.ok(opencodeSkillSources.includes("~/.claude/skills/global-claude"));
    assert.ok(opencodeSkillSources.includes(".agents/skills/project-agent"));
    assert.ok(opencodeSkillSources.includes("~/.agents/skills/global-agent"));
    assert.equal(opencodeSkillSources.some((source) => source.endsWith("/SKILL.md")), false);
    assert.ok(
      scan.evidence.some(
        (item) =>
          item.agent === "opencode" &&
          item.kind === "skill" &&
          item.name === "global-agent" &&
          item.metadata?.["entrypointStatus"] === "captured"
      )
    );
  });

  it("summarizes a skill with a symlinked SKILL.md as one skill item", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(homeDir, ".agents", "skills", "office-hours"), { recursive: true });
    await writeFile(
      join(homeDir, "office-hours-source.md"),
      "---\nname: office-hours\ndescription: Office hours skill\n---\n",
      "utf8"
    );
    await symlink(
      join(homeDir, "office-hours-source.md"),
      join(homeDir, ".agents", "skills", "office-hours", "SKILL.md")
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const officeHoursItems = scan.evidence.filter(
      (item) => item.agent === "opencode" && item.sourcePath.includes("office-hours")
    );

    assert.equal(officeHoursItems.length, 1);
    assert.equal(officeHoursItems[0]?.kind, "skill");
    assert.equal(officeHoursItems[0]?.name, "office-hours");
    assert.equal(officeHoursItems[0]?.sourcePath, "~/.agents/skills/office-hours");
    assert.equal(officeHoursItems[0]?.metadata?.["entrypointStatus"], "symlink_followed");
  });

  it("discovers OpenCode plugin package skills from the runtime cache", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    const skillPath = join(
      homeDir,
      ".cache",
      "opencode",
      "packages",
      "superpowers",
      "node_modules",
      "superpowers",
      "skills",
      "brainstorm",
      "SKILL.md"
    );
    await mkdir(join(skillPath, ".."), { recursive: true });
    await writeFile(
      skillPath,
      "---\nname: brainstorm\ndescription: Generate options\n---\n",
      "utf8"
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });

    assert.ok(
      scan.evidence.some(
        (item) =>
          item.agent === "opencode" &&
          item.kind === "skill" &&
          item.name === "brainstorm" &&
          item.sourcePath ===
            "~/.cache/opencode/packages/superpowers/node_modules/superpowers/skills/brainstorm"
      )
    );
  });

  it("matches OpenCode runtime quirks for built-in, uppercase entrypoint, and name mismatch skills", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    const uppercaseSkillPath = join(
      homeDir,
      ".agents",
      "skills",
      "better-auth-security-best-practices",
      "SKILL.MD"
    );
    const mismatchSkillPath = join(homeDir, ".claude", "skills", "gstack-claude", "SKILL.md");
    await mkdir(join(uppercaseSkillPath, ".."), { recursive: true });
    await mkdir(join(mismatchSkillPath, ".."), { recursive: true });
    await writeFile(
      uppercaseSkillPath,
      "---\nname: better-auth-security-best-practices\ndescription: Security skill\n---\n",
      "utf8"
    );
    await writeFile(
      mismatchSkillPath,
      "---\nname: claude\ndescription: Claude compatibility skill\n---\n",
      "utf8"
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const opencodeSkills = scan.evidence.filter((item) => item.agent === "opencode" && item.kind === "skill");

    assert.ok(opencodeSkills.some((item) => item.name === "customize-opencode" && item.scope === "managed"));
    assert.ok(opencodeSkills.some((item) => item.name === "better-auth-security-best-practices"));
    assert.ok(
      opencodeSkills.some(
        (item) =>
          item.name === "claude" &&
          item.sourcePath === "~/.claude/skills/gstack-claude" &&
          item.metadata?.["nameMatchesDirectory"] === false
      )
    );
  });

  it("deduplicates OpenCode skills by effective skill name", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(homeDir, ".claude", "skills", "review"), { recursive: true });
    await mkdir(join(homeDir, ".agents", "skills", "review"), { recursive: true });
    await writeFile(
      join(homeDir, ".claude", "skills", "review", "SKILL.md"),
      "---\nname: review\ndescription: Review code\n---\n",
      "utf8"
    );
    await writeFile(
      join(homeDir, ".agents", "skills", "review", "SKILL.md"),
      "---\nname: review\ndescription: Review code\n---\n",
      "utf8"
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const reviewItems = scan.evidence.filter(
      (item) => item.agent === "opencode" && item.kind === "skill" && item.name === "review"
    );

    assert.equal(reviewItems.length, 1);
    assert.deepEqual(reviewItems[0]?.metadata?.["duplicateSources"], ["~/.agents/skills/review"]);
  });

  it("discovers Pi skills using Pi-specific roots and validation rules", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(homeDir, ".pi", "agent", "skills"), { recursive: true });
    await mkdir(join(projectPath, ".pi", "skills", "project-pi"), { recursive: true });
    await mkdir(join(homeDir, ".agents", "skills", "global-agent"), { recursive: true });

    await writeFile(
      join(homeDir, ".pi", "agent", "skills", "root-file.md"),
      "---\nname: root-file\ndescription: Root file skill\n---\n",
      "utf8"
    );
    await writeFile(
      join(projectPath, ".pi", "skills", "project-pi", "SKILL.md"),
      "---\nname: different-name\ndescription: Pi allows name mismatch\n---\n",
      "utf8"
    );
    await writeFile(
      join(homeDir, ".agents", "skills", "global-agent", "SKILL.md"),
      "---\nname: global-agent\ndescription: Global agent skill\n---\n",
      "utf8"
    );
    await writeFile(
      join(homeDir, ".agents", "skills", "ignored-root.md"),
      "---\nname: ignored-root\ndescription: Pi ignores root md files in .agents skills\n---\n",
      "utf8"
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const piSkills = scan.evidence.filter((item) => item.agent === "pi-agent" && item.kind === "skill");

    assert.ok(piSkills.some((item) => item.name === "root-file" && item.sourcePath === "~/.pi/agent/skills/root-file.md"));
    assert.ok(
      piSkills.some(
        (item) =>
          item.name === "different-name" &&
          item.sourcePath === ".pi/skills/project-pi" &&
          item.metadata?.["nameMatchesDirectory"] === false
      )
    );
    assert.ok(piSkills.some((item) => item.name === "global-agent" && item.sourcePath === "~/.agents/skills/global-agent"));
    assert.equal(piSkills.some((item) => item.name === "ignored-root"), false);
  });

  it("loads Pi skills from settings paths and filters missing descriptions", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(projectPath, ".pi"), { recursive: true });
    await mkdir(join(projectPath, "custom-skills", "loaded"), { recursive: true });
    await mkdir(join(projectPath, "custom-skills", "missing-description"), { recursive: true });
    await writeFile(join(projectPath, ".pi", "settings.json"), JSON.stringify({ skills: ["../custom-skills"] }), "utf8");
    await writeFile(
      join(projectPath, "custom-skills", "loaded", "SKILL.md"),
      "---\nname: loaded\ndescription: Loaded from settings\n---\n",
      "utf8"
    );
    await writeFile(
      join(projectPath, "custom-skills", "missing-description", "SKILL.md"),
      "---\nname: missing-description\n---\n",
      "utf8"
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const piSkills = scan.evidence.filter((item) => item.agent === "pi-agent" && item.kind === "skill");

    assert.ok(piSkills.some((item) => item.name === "loaded" && item.sourcePath === "custom-skills/loaded"));
    assert.equal(piSkills.some((item) => item.name === "missing-description"), false);
  });

  it("discovers Pi extension entrypoints from auto and settings paths", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(projectPath, ".pi", "extensions", "project-extension"), { recursive: true });
    await mkdir(join(homeDir, ".pi", "agent", "extensions"), { recursive: true });
    await mkdir(join(projectPath, ".pi"), { recursive: true });
    await mkdir(join(projectPath, "custom-extension", "src"), { recursive: true });

    await writeFile(join(projectPath, ".pi", "extensions", "project-extension", "index.ts"), "export default function () {}\n", "utf8");
    await writeFile(join(homeDir, ".pi", "agent", "extensions", "global-extension.ts"), "export default function () {}\n", "utf8");
    await writeFile(join(projectPath, "custom-extension", "package.json"), JSON.stringify({ pi: { extensions: ["./src/index.ts"] } }), "utf8");
    await writeFile(join(projectPath, "custom-extension", "src", "index.ts"), "export default function () {}\n", "utf8");
    await writeFile(join(projectPath, ".pi", "settings.json"), JSON.stringify({ extensions: ["../custom-extension"] }), "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const piExtensions = scan.evidence.filter((item) => item.agent === "pi-agent" && item.kind === "extension");

    assert.ok(
      piExtensions.some(
        (item) =>
          item.name === "project-extension" &&
          item.sourcePath === ".pi/extensions/project-extension/index.ts" &&
          item.metadata?.["extensionStyle"] === "directory_index"
      )
    );
    assert.ok(piExtensions.some((item) => item.name === "global-extension" && item.sourcePath === "~/.pi/agent/extensions/global-extension.ts"));
    assert.ok(piExtensions.some((item) => item.name === "custom-extension" && item.sourcePath === "custom-extension/src/index.ts"));
  });

  it("does not count Pi skills whose SKILL.md symlink cannot be followed", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(homeDir, ".agents", "skills", "broken-skill"), { recursive: true });
    await symlink(join(homeDir, "missing", "SKILL.md"), join(homeDir, ".agents", "skills", "broken-skill", "SKILL.md"));

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const piSkills = scan.evidence.filter((item) => item.agent === "pi-agent" && item.kind === "skill");

    assert.equal(piSkills.some((item) => item.name === "broken-skill"), false);
  });
});
