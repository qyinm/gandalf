#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const binName = process.platform === "win32" ? "hem.cmd" : "hem";

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repo,
    env: { ...process.env, ...(options.env ?? {}) },
    encoding: "utf8",
    stdio: options.stdio ?? "pipe"
  });
  if (result.status !== 0) {
    const rendered = [
      `$ ${command} ${args.join(" ")}`,
      result.stdout,
      result.stderr
    ].filter(Boolean).join("\n");
    throw new Error(rendered);
  }
  return result;
}

function packPackage(packageDir, destination, npmCache) {
  const result = run("npm", ["pack", "--pack-destination", destination, "--json"], {
    cwd: packageDir,
    env: { npm_config_cache: npmCache }
  });
  const metadata = JSON.parse(result.stdout)[0];
  assert.ok(metadata.filename, `npm pack did not return a filename for ${packageDir}`);
  return {
    tarball: path.join(destination, metadata.filename),
    files: metadata.files.map((file) => file.path)
  };
}

async function main() {
  const cliDist = path.join(repo, "apps/cli/dist/src/cli.js");
  if (!existsSync(cliDist)) {
    throw new Error("apps/cli/dist/src/cli.js is missing. Run `bun run build` before `bun run package:smoke`.");
  }

  const root = await mkdtemp(path.join(tmpdir(), "hem-package-smoke-"));
  try {
    const npmCache = path.join(root, "npm-cache");
    await mkdir(npmCache, { recursive: true });
    const packed = [
      packPackage(path.join(repo, "packages/core"), root, npmCache),
      packPackage(path.join(repo, "apps/tui"), root, npmCache),
      packPackage(path.join(repo, "apps/cli"), root, npmCache)
    ];
    const cliPackage = packed[2];
    assert.ok(cliPackage.files.includes("dist/src/cli.js"), "CLI package is missing dist/src/cli.js");
    assert.ok(cliPackage.files.includes("README.md"), "CLI package is missing README.md");

    const installDir = path.join(root, "install");
    const project = path.join(root, "project");
    const home = path.join(root, "home");
    const store = path.join(root, "store");
    const codexDir = path.join(home, ".codex");
    await mkdir(installDir, { recursive: true });
    await mkdir(project, { recursive: true });
    await mkdir(codexDir, { recursive: true });
    await writeFile(path.join(installDir, "package.json"), "{\"type\":\"module\"}\n", "utf8");

    run("npm", [
      "install",
      "--ignore-scripts",
      "--no-audit",
      "--no-fund",
      "--prefer-offline",
      ...packed.map((item) => item.tarball)
    ], { cwd: installDir, env: { npm_config_cache: npmCache } });

    const hem = path.join(installDir, "node_modules/.bin", binName);
    const env = { HOME: home, HEM_STORE: store, HEM_UPDATE_CHECK: "0" };
    const originalConfig = "model = \"gpt-5\"\napproval_policy = \"on-request\"\n";
    const configPath = path.join(codexDir, "config.toml");
    await writeFile(configPath, originalConfig, "utf8");

    const help = run(hem, ["--help"], { cwd: project, env });
    assert.match(help.stdout, /Save, compare, and restore Codex user-global setup experiments/);

    run(hem, [
      "snapshot", "create",
      "--name", "package-baseline",
      "--agent", "codex",
      "--scope", "user",
      "--project", project
    ], { cwd: project, env });

    await writeFile(configPath, "", "utf8");
    const addedSkill = path.join(codexDir, "skills", "package-smoke", "SKILL.md");
    await mkdir(path.dirname(addedSkill), { recursive: true });
    await writeFile(addedSkill, "---\nname: package-smoke\n---\n", "utf8");

    const diff = run(hem, [
      "diff", "package-baseline", "current",
      "--agent", "codex",
      "--scope", "user",
      "--project", project,
      "--json"
    ], { cwd: project, env });
    const diffJson = JSON.parse(diff.stdout);
    assert.ok(diffJson.semanticChanges.some((change) => change.code === "AGENT_CONFIG_CHANGED"));
    assert.ok(diffJson.semanticChanges.some((change) => change.code === "SKILL_ADDED"));

    const dryRun = run(hem, [
      "restore",
      "--snapshot", "package-baseline",
      "--dry-run",
      "--agent", "codex",
      "--scope", "user",
      "--project", project
    ], { cwd: project, env });
    assert.match(dryRun.stdout, /hem restore dry-run/);

    const dryRunJson = run(hem, [
      "restore",
      "--snapshot", "package-baseline",
      "--dry-run",
      "--agent", "codex",
      "--scope", "user",
      "--project", project,
      "--json"
    ], { cwd: project, env });
    assert.equal(JSON.parse(dryRunJson.stdout).sourceSnapshot, "package-baseline");

    run(hem, [
      "restore",
      "--snapshot", "package-baseline",
      "--apply",
      "--experimental",
      "--agent", "codex",
      "--scope", "user",
      "--project", project
    ], { cwd: project, env });
    assert.equal(await readFile(configPath, "utf8"), originalConfig);
    assert.equal(existsSync(addedSkill), false);

    console.log("Package smoke passed: packed CLI installed and restored a disposable Codex setup.");
  } finally {
    if (!process.env.HEM_KEEP_PACKAGE_SMOKE) {
      await rm(root, { recursive: true, force: true });
    }
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
});
