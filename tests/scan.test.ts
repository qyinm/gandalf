import assert from "node:assert/strict";
import { mkdir, mkdtemp, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";

import { scanProject } from "../src/scan.js";

async function makeSandbox(): Promise<{ root: string; projectPath: string; homeDir: string; storeDir: string }> {
  const root = await mkdtemp(join(tmpdir(), "hem-scan-"));
  const projectPath = join(root, "project");
  const homeDir = join(root, "home");
  const storeDir = join(homeDir, ".hem");

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

  it("discovers Codex MCP servers from config.toml sections", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(homeDir, ".codex"), { recursive: true });
    await writeFile(
      join(homeDir, ".codex", "config.toml"),
      [
        "[mcp_servers.context7] # docs server",
        "command = \"npx\"",
        "args = [",
        "  \"-y\",",
        "  \"@upstash/context7-mcp\",",
        "]",
        "",
        "[mcp_servers.node_repl]",
        "command = \"node\"",
        "enabled = false",
        "",
        "[mcp_servers.node_repl.env]",
        "OPENAI_API_KEY = \"secret\"",
      ].join("\n"),
      "utf8"
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const codexMcpServers = scan.evidence.filter((item) => item.agent === "codex" && item.kind === "mcp_server");
    const context7Value = codexMcpServers.find((item) => item.name === "context7")?.value as Record<string, unknown> | undefined;
    const nodeReplValue = codexMcpServers.find((item) => item.name === "node_repl")?.value as Record<string, unknown> | undefined;

    assert.deepEqual(codexMcpServers.map((item) => item.name).sort(), ["context7", "node_repl"]);
    assert.deepEqual(context7Value?.["args"], ["-y", "@upstash/context7-mcp"]);
    assert.equal(nodeReplValue?.["enabled"], false);
    assert.deepEqual(nodeReplValue?.["envKeys"], ["OPENAI_API_KEY"]);
    assert.doesNotMatch(JSON.stringify(codexMcpServers), /secret/);
  });

  it("discovers Codex skills from user and plugin cache roots", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    const userSkill = join(homeDir, ".codex", "skills", "review", "SKILL.md");
    const pluginSkill = join(
      homeDir,
      ".codex",
      "plugins",
      "cache",
      "openai-curated",
      "build-web-apps",
      "1.0.0",
      "skills",
      "react-best-practices",
      "SKILL.md"
    );

    await mkdir(join(userSkill, ".."), { recursive: true });
    await mkdir(join(pluginSkill, ".."), { recursive: true });
    await writeFile(userSkill, "---\nname: review\ndescription: Review code\n---\n", "utf8");
    await writeFile(pluginSkill, "---\nname: react-best-practices\ndescription: React guidance\n---\n", "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const codexSkills = scan.evidence.filter((item) => item.agent === "codex" && item.kind === "skill");

    assert.ok(codexSkills.some((item) => item.name === "review" && item.sourcePath === "~/.codex/skills/review"));
    assert.ok(
      codexSkills.some(
        (item) =>
          item.name === "react-best-practices" &&
          item.sourcePath === "~/.codex/plugins/cache/openai-curated/build-web-apps/1.0.0/skills/react-best-practices"
      )
    );
  });

  it("discovers Codex hooks from user and project hook files", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    const projectHooks = join(projectPath, ".codex", "hooks.json");
    const userHooks = join(homeDir, ".codex", "hooks.json");
    const pluginHooks = join(homeDir, ".codex", "plugins", "cache", "openai-codex", "codex", "1.0.0", "hooks", "hooks.json");

    await mkdir(join(projectHooks, ".."), { recursive: true });
    await mkdir(join(userHooks, ".."), { recursive: true });
    await mkdir(join(pluginHooks, ".."), { recursive: true });
    await writeFile(projectHooks, JSON.stringify({
      hooks: {
        PreToolUse: [
          { matcher: "Write", hooks: [{ type: "command", command: "project-hook", timeout: 5 }] }
        ]
      }
    }), "utf8");
    await writeFile(userHooks, JSON.stringify({
      hooks: {
        SessionStart: [
          { hooks: [{ type: "command", command: "user-hook" }] }
        ],
        Stop: [
          { hooks: [{ type: "command", command: "stop-hook" }] }
        ]
      }
    }), "utf8");
    await writeFile(pluginHooks, JSON.stringify({
      hooks: {
        UserPromptSubmit: [
          { hooks: [{ type: "command", command: "plugin-hook" }] }
        ]
      }
    }), "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const codexHooks = scan.evidence.filter((item) => item.agent === "codex" && item.kind === "hook");

    assert.deepEqual(codexHooks.map((item) => item.name).sort(), [
      "PreToolUse.Write",
      "SessionStart.*",
      "Stop.*"
    ]);
    assert.ok(codexHooks.some((item) => item.sourcePath === ".codex/hooks.json" && item.name === "PreToolUse.Write"));
    assert.ok(codexHooks.some((item) => item.sourcePath === "~/.codex/hooks.json" && item.name === "SessionStart.*"));
    assert.equal(codexHooks.some((item) => item.sourcePath.includes(".codex/plugins/cache")), false);
  });

  it("discovers Codex inline hooks from config.toml without counting hook state", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(homeDir, ".codex"), { recursive: true });
    await writeFile(join(homeDir, ".codex", "config.toml"), [
      "[features]",
      "hooks = true",
      "",
      "[hooks.state.\"/tmp/hooks.json:pre_tool_use:0:0\"]",
      "trusted = true",
      "",
      "[[hooks.PreToolUse]]",
      "matcher = \"^Bash$\"",
      "",
      "[[hooks.PreToolUse.hooks]]",
      "type = \"command\"",
      "command = \"python3 ~/.codex/hooks/pre_tool_use.py\"",
      "timeout = 30",
      "",
      "[[hooks.Stop]]",
      "",
      "[[hooks.Stop.hooks]]",
      "type = \"command\"",
      "command = \"python3 ~/.codex/hooks/stop.py\"",
    ].join("\n"), "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const codexHooks = scan.evidence.filter((item) => item.agent === "codex" && item.kind === "hook");

    assert.deepEqual(codexHooks.map((item) => item.name).sort(), ["PreToolUse.^Bash$", "Stop.*"]);
    assert.ok(codexHooks.every((item) => item.sourcePath === "~/.codex/config.toml"));
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
    await writeFile(join(projectPath, ".env"), "OPENAI_API_KEY=sk-real-secret\nHEM_MODE=local\n", "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const envEvidence = scan.evidence.filter((item) => item.kind === "env_key");

    assert.ok(
      envEvidence.some(
        (item) =>
          item.kind === "env_key" &&
          item.name === "OPENAI_API_KEY" &&
          item.captureStatus === "redacted" &&
          item.value === undefined
      )
    );
    assert.ok(
      envEvidence.some(
        (item) =>
          item.kind === "env_key" &&
          item.name === "HEM_MODE" &&
          item.captureStatus === "omitted" &&
          item.value === undefined
      )
    );
    assert.doesNotMatch(JSON.stringify(envEvidence), /sk-real-secret|local/);
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

  it("does not report Cursor when no local Cursor setup is visible", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();

    const scan = await scanProject({ projectPath, homeDir, storeDir });

    assert.equal(scan.evidence.some((item) => item.agent === "cursor"), false);
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

  it("discovers Cursor MCP transports and redacts auth material", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(projectPath, ".cursor"), { recursive: true });
    await mkdir(join(homeDir, ".cursor"), { recursive: true });

    await writeFile(join(projectPath, ".cursor", "mcp.json"), JSON.stringify({
      mcpServers: {
        filesystem: {
          type: "stdio",
          command: "node",
          args: ["server.js", "--token", "${env:FILESYSTEM_TOKEN}"],
          env: { API_KEY: "project-secret" },
          envFile: ".env",
        },
        linear: {
          type: "sse",
          url: "https://mcp.example.test/sse?token=remote-secret",
          headers: { Authorization: "Bearer header-secret" },
          auth: {
            CLIENT_ID: "${env:LINEAR_CLIENT_ID}",
            CLIENT_SECRET: "oauth-secret",
            scopes: ["read"],
          },
        },
      },
    }), "utf8");
    await writeFile(join(homeDir, ".cursor", "mcp.json"), JSON.stringify({
      mcpServers: {
        docs: {
          type: "streamable-http",
          url: "https://docs.example.test/mcp",
        },
      },
    }), "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const cursorMcp = scan.evidence.filter((item) => item.agent === "cursor" && item.kind === "mcp_server");
    const filesystem = cursorMcp.find((item) => item.name === "filesystem");
    const linear = cursorMcp.find((item) => item.name === "linear");
    const docs = cursorMcp.find((item) => item.name === "docs");

    assert.equal(filesystem?.sourcePath, ".cursor/mcp.json");
    assert.equal(filesystem?.metadata?.["transport"], "stdio");
    assert.equal(filesystem?.metadata?.["remote"], false);
    assert.equal(linear?.metadata?.["transport"], "sse");
    assert.equal(linear?.metadata?.["remote"], true);
    assert.equal(docs?.sourcePath, "~/.cursor/mcp.json");
    assert.equal(docs?.metadata?.["transport"], "streamable-http");
    assert.equal(docs?.metadata?.["remote"], true);
    assert.doesNotMatch(JSON.stringify(cursorMcp), /project-secret|remote-secret|header-secret|oauth-secret/);
    assert.match(JSON.stringify(cursorMcp), /\$\{env:FILESYSTEM_TOKEN\}/);
  });

  it("discovers Cursor skills from documented roots and nested project roots", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    const skillPaths = [
      join(projectPath, ".cursor", "skills", "project-cursor", "SKILL.md"),
      join(homeDir, ".cursor", "skills", "global-cursor", "SKILL.md"),
      join(projectPath, ".agents", "skills", "project-agent", "SKILL.md"),
      join(homeDir, ".agents", "skills", "global-agent", "SKILL.md"),
      join(projectPath, ".claude", "skills", "project-claude", "SKILL.md"),
      join(projectPath, ".codex", "skills", "project-codex", "SKILL.md"),
      join(projectPath, "apps", "web", ".cursor", "skills", "deploy-web", "SKILL.md"),
      join(projectPath, "packages", "api", ".agents", "skills", "deploy-api", "SKILL.md"),
    ];

    for (const skillPath of skillPaths) {
      const skillName = skillPath.split("/").at(-2)!;
      await mkdir(join(skillPath, ".."), { recursive: true });
      await writeFile(skillPath, [
        "---",
        `name: ${skillName}`,
        `description: ${skillName} skill`,
        "paths:",
        "  - src/**",
        "disable-model-invocation: true",
        "metadata:",
        "  team: platform",
        "---",
        "",
      ].join("\n"), "utf8");
    }

    await mkdir(join(projectPath, ".cursor", "skills", "missing-description"), { recursive: true });
    await writeFile(
      join(projectPath, ".cursor", "skills", "missing-description", "SKILL.md"),
      "---\nname: missing-description\n---\n",
      "utf8"
    );
    await mkdir(join(projectPath, ".cursor", "skills", "wrong-folder"), { recursive: true });
    await writeFile(
      join(projectPath, ".cursor", "skills", "wrong-folder", "SKILL.md"),
      "---\nname: different-name\ndescription: Should be rejected\n---\n",
      "utf8"
    );

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const cursorSkills = scan.evidence.filter((item) => item.agent === "cursor" && item.kind === "skill");
    const sources = cursorSkills.map((item) => item.sourcePath);

    assert.ok(sources.includes(".cursor/skills/project-cursor"));
    assert.ok(sources.includes("~/.cursor/skills/global-cursor"));
    assert.ok(sources.includes(".agents/skills/project-agent"));
    assert.ok(sources.includes("~/.agents/skills/global-agent"));
    assert.ok(sources.includes(".claude/skills/project-claude"));
    assert.ok(sources.includes(".codex/skills/project-codex"));
    assert.ok(sources.includes("apps/web/.cursor/skills/deploy-web"));
    assert.ok(sources.includes("packages/api/.agents/skills/deploy-api"));
    assert.equal(cursorSkills.some((item) => item.name === "missing-description"), false);
    assert.equal(cursorSkills.some((item) => item.name === "different-name"), false);
    assert.ok(
      cursorSkills.some(
        (item) =>
          item.name === "deploy-web" &&
          item.metadata?.["scopeRoot"] === "apps/web" &&
          item.metadata?.["disableModelInvocation"] === true
      )
    );
  });

  it("keeps higher-precedence Cursor skills when duplicates exist", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    const userSkill = join(homeDir, ".cursor", "skills", "deploy-web", "SKILL.md");
    const nestedProjectSkill = join(projectPath, "apps", "web", ".cursor", "skills", "deploy-web", "SKILL.md");
    await mkdir(join(userSkill, ".."), { recursive: true });
    await mkdir(join(nestedProjectSkill, ".."), { recursive: true });
    await writeFile(userSkill, "---\nname: deploy-web\ndescription: User deploy skill\n---\n", "utf8");
    await writeFile(nestedProjectSkill, "---\nname: deploy-web\ndescription: Project deploy skill\n---\n", "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const deploySkills = scan.evidence.filter(
      (item) => item.agent === "cursor" && item.kind === "skill" && item.name === "deploy-web"
    );

    assert.equal(deploySkills.length, 1);
    assert.equal(deploySkills[0]?.sourcePath, "apps/web/.cursor/skills/deploy-web");
    assert.equal(deploySkills[0]?.precedence, 40);
    assert.deepEqual(deploySkills[0]?.metadata?.["duplicateSources"], ["~/.cursor/skills/deploy-web"]);
  });

  it("discovers Cursor hooks with documented fields and blind spots", async () => {
    const { projectPath, homeDir, storeDir } = await makeSandbox();
    await mkdir(join(projectPath, ".cursor"), { recursive: true });
    await mkdir(join(homeDir, ".cursor"), { recursive: true });
    await writeFile(join(projectPath, ".cursor", "hooks.json"), JSON.stringify({
      version: 1,
      hooks: {
        beforeShellExecution: [
          {
            command: ".cursor/hooks/approve-network.sh",
            matcher: { pattern: "curl|wget" },
            timeout: 5,
            failClosed: true,
          },
        ],
        stop: [
          {
            type: "prompt",
            command: "Check whether the run finished safely: $ARGUMENTS",
            loop_limit: null,
          },
        ],
      },
    }), "utf8");
    await writeFile(join(homeDir, ".cursor", "hooks.json"), JSON.stringify({
      version: 1,
      hooks: {
        workspaceOpen: [
          { command: "./hooks/load-plugins.sh", type: "command" },
        ],
      },
    }), "utf8");

    const scan = await scanProject({ projectPath, homeDir, storeDir });
    const cursorHooks = scan.evidence.filter((item) => item.agent === "cursor" && item.kind === "hook");
    const beforeShell = cursorHooks.find((item) => item.name === "beforeShellExecution.0");
    const stop = cursorHooks.find((item) => item.name === "stop.0");
    const workspaceOpen = cursorHooks.find((item) => item.name === "workspaceOpen.0");

    assert.equal(beforeShell?.sourcePath, ".cursor/hooks.json");
    assert.equal(beforeShell?.metadata?.["eventName"], "beforeShellExecution");
    assert.equal(beforeShell?.metadata?.["hookCategory"], "agent");
    assert.equal(beforeShell?.metadata?.["sourcePriority"], 30);
    assert.equal(beforeShell?.metadata?.["executable"], true);
    assert.equal(beforeShell?.value && (beforeShell.value as Record<string, unknown>)["failClosed"], true);
    assert.equal(stop?.value && (stop.value as Record<string, unknown>)["type"], "prompt");
    assert.equal(stop?.metadata?.["policyEvaluated"], true);
    assert.equal(workspaceOpen?.sourcePath, "~/.cursor/hooks.json");
    assert.equal(workspaceOpen?.metadata?.["hookCategory"], "app_lifecycle");
    assert.ok(
      scan.evidence.some(
        (item) =>
          item.agent === "cursor" &&
          item.kind === "unsupported" &&
          item.sourcePath === "<cursor-team-hooks>" &&
          item.metadata?.["reason"] === "cloud_distributed_hooks_not_locally_readable"
      )
    );
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
