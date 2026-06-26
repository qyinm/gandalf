#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const gandalf = path.join(repo, "bin", "gandalf");

function ensureGoBinary() {
  if (existsSync(gandalf)) {
    return;
  }
  const result = spawnSync("go", ["build", "-o", gandalf, "./cmd/gandalf"], {
    cwd: repo,
    encoding: "utf8"
  });
  if (result.status !== 0) {
    throw new Error(
      `bin/gandalf is missing and go build failed.\n${result.stderr || result.stdout || ""}`.trim()
    );
  }
}

function run(args, options = {}) {
  const rendered = `gandalf ${args.join(" ")}`;
  console.log(`\n$ ${rendered}`);
  const result = spawnSync(gandalf, args, {
    cwd: options.cwd ?? repo,
    env: { ...process.env, ...(options.env ?? {}) },
    encoding: "utf8"
  });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.status !== 0) {
    throw new Error(`${rendered} failed with exit ${result.status}`);
  }
  return result;
}

async function main() {
  ensureGoBinary();

  const root = await mkdtemp(path.join(tmpdir(), "gandalf-gate2-acceptance-"));
  try {
    const project = path.join(root, "project");
    const home = path.join(root, "home");
    const store = path.join(root, "store");
    const codexDir = path.join(home, ".codex");
    const configPath = path.join(codexDir, "config.toml");
    await mkdir(project, { recursive: true });
    await mkdir(codexDir, { recursive: true });

    const originalConfig = [
      "model = \"gpt-5\"",
      "approval_policy = \"on-request\"",
      "",
      "[mcp_servers.github]",
      "command = \"gh\"",
      "args = [\"mcp\", \"server\"]",
      ""
    ].join("\n");
    await writeFile(configPath, originalConfig, "utf8");
    await writeFile(path.join(project, "README.md"), "Disposable Gate 2 acceptance project.\n", "utf8");

    const env = { HOME: home, GANDALF_STORE: store, GANDALF_UPDATE_CHECK: "0" };

    console.log("Gate 2 deterministic Codex rollback acceptance");
    console.log(`HOME=${home}`);
    console.log(`GANDALF_STORE=${store}`);
    console.log(`project=${project}`);
    console.log(`binary=${gandalf}`);

    run([
      "snapshot", "create",
      "--name", "clean-codex",
      "--agent", "codex",
      "--scope", "user",
      "--project", project
    ], { cwd: project, env });

    await writeFile(configPath, "", "utf8");
    const addedSkill = path.join(codexDir, "skills", "synthetic-harness", "SKILL.md");
    await mkdir(path.dirname(addedSkill), { recursive: true });
    await writeFile(addedSkill, "---\nname: synthetic-harness\n---\nAdds a disposable acceptance skill.\n", "utf8");
    console.log("\n# Synthetic harness install zero-filled config.toml and added a Codex skill.");

    run([
      "diff", "clean-codex", "current",
      "--agent", "codex",
      "--scope", "user",
      "--project", project
    ], { cwd: project, env });

    run([
      "restore",
      "--snapshot", "clean-codex",
      "--dry-run",
      "--agent", "codex",
      "--scope", "user",
      "--project", project
    ], { cwd: project, env });

    run([
      "restore",
      "--snapshot", "clean-codex",
      "--apply",
      "--experimental",
      "--agent", "codex",
      "--scope", "user",
      "--project", project
    ], { cwd: project, env });

    assert.equal(await readFile(configPath, "utf8"), originalConfig);
    assert.equal(existsSync(addedSkill), false);
    console.log("\nGate 2 acceptance passed: config restored and synthetic skill removed.");
  } finally {
    if (!process.env.GANDALF_KEEP_GATE2_ACCEPTANCE) {
      await rm(root, { recursive: true, force: true });
    }
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
});
