#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

const repo = path.resolve(new URL("..", import.meta.url).pathname);
const node = process.execPath;
const cli = path.join(repo, "dist/src/cli.js");

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

function requireDocker() {
  const docker = spawnSync("docker", ["version", "--format", "{{.Server.Version}}"], { encoding: "utf8" });
  if (docker.status !== 0) {
    throw new Error("Docker is required for cross-machine dogfood (Linux import container), but Docker is not available/running.");
  }
}

async function main() {
  if (!existsSync(cli)) {
    run("npm", ["run", "build"], { stdio: "inherit" });
  }
  requireDocker();

  const root = await mkdtemp(path.join(tmpdir(), "hem-cross-machine-"));
  try {
    const macProject = path.join(root, "mac-project");
    const macHome = path.join(root, "mac-home");
    const macStore = path.join(root, "mac-store");
    const out = path.join(root, "mac-export.hem");
    await mkdir(macProject, { recursive: true });
    await mkdir(path.join(macHome, ".claude"), { recursive: true });
    await mkdir(path.join(macHome, ".local/bin"), { recursive: true });
    await writeFile(path.join(macHome, ".claude/settings.json"), JSON.stringify({ permissions: { allow: ["Bash(bun test:*)"] } }));
    await writeFile(path.join(macHome, ".local/bin/private-mcp"), "#!/bin/sh\nexit 0\n");
    await writeFile(path.join(macProject, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "stdio", command: "npx", args: ["-y", "@modelcontextprotocol/server-github"] },
        local: { transport: "stdio", command: path.join(macHome, ".local/bin/private-mcp") }
      }
    }, null, 2));

    const env = { HOME: macHome, HEM_STORE: macStore };
    run(node, [cli, "snapshot", "create", "--name", "mac-baseline", "--metadata-only", "--project", macProject], { cwd: macProject, env });
    run(node, [cli, "bundle", "export", "--name", "mac-baseline", "--out", out, "--project", macProject], { cwd: macProject, env });

    const linuxScript = [
      "set -euo pipefail",
      "mkdir -p /home/hem /linux/project /linux/store",
      "HOME=/home/hem HEM_STORE=/linux/store node /repo/dist/src/cli.js bundle import /work/mac-export.hem --dry-run --project /linux/project --json > /work/import.json"
    ].join(" && ");
    run("docker", [
      "run", "--rm",
      "-v", `${repo}:/repo:ro`,
      "-v", `${root}:/work`,
      "node:22-bookworm",
      "bash", "-lc", linuxScript
    ], { cwd: repo });

    const importJson = JSON.parse(await import("node:fs/promises").then(({ readFile }) => readFile(path.join(root, "import.json"), "utf8")));
    if (importJson.contentApplied) {
      throw new Error("Linux import was not a safe dry-run.");
    }
    if (importJson.machineDiff?.sourcePlatform !== "darwin" || importJson.machineDiff?.targetPlatform !== "linux") {
      throw new Error(`Expected darwin → linux machine diff, got ${JSON.stringify(importJson.machineDiff)}`);
    }
    if (!importJson.machineDiff?.crossOS) {
      throw new Error("Expected crossOS=true in Linux dry-run machine diff.");
    }
    if (!importJson.machineDiff?.mcpBinaryReport?.some((report) => report.binaryKind === "source_local_path" && report.availableOnTarget === false)) {
      throw new Error("Expected source-local MCP binary mismatch warning in Linux dry-run.");
    }

    console.log("Cross-machine dogfood passed: macOS export dry-ran successfully inside Linux container.");
    console.log(`Bundle: ${out}`);
  } finally {
    if (!process.env.HEM_KEEP_DOGFOOD) {
      await rm(root, { recursive: true, force: true });
    }
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
});
