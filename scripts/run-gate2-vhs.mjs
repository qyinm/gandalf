#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { chmod, copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const envPath = "/tmp/hem-gate2-video-env.sh";
process.env.TMPDIR = "/tmp";
const tape = path.resolve(repo, process.argv[2] ?? "demo/fix.tape");

function parseEnvFile(contents) {
  const env = {};
  for (const line of contents.split("\n")) {
    const match = line.match(/^export ([A-Z_]+)='(.*)'$/);
    if (match) {
      env[match[1]] = match[2].replaceAll("'\\''", "'");
    } else {
      const plain = line.match(/^export ([A-Z_]+)=([^'\s].*)$/);
      if (plain) env[plain[1]] = plain[2];
    }
  }
  return env;
}

async function writeExecutable(file, body) {
  await writeFile(file, body, "utf8");
  await chmod(file, 0o755);
}

const setup = spawnSync(process.execPath, [
  path.join(repo, "scripts/gate2-video-fixture.mjs"),
  "setup",
  envPath
], {
  cwd: repo,
  encoding: "utf8"
});

if (setup.status !== 0) {
  if (setup.stdout) process.stdout.write(setup.stdout);
  if (setup.stderr) process.stderr.write(setup.stderr);
  process.exit(setup.status ?? 1);
}

const demoEnv = parseEnvFile(await readFile(envPath, "utf8"));
const renderedTape = path.join(demoEnv.HEM_DEMO_ROOT, path.basename(tape));
await writeFile(
  renderedTape,
  (await readFile(tape, "utf8")).replaceAll("__HEM_VISIBLE_PROJECT__", demoEnv.HEM_VISIBLE_PROJECT),
  "utf8"
);

const realCodexHome = process.env.CODEX_HOME;
if (realCodexHome) {
  await copyFile(path.join(realCodexHome, "auth.json"), path.join(demoEnv.CODEX_HOME, "auth.json"));
}
const bin = path.join(demoEnv.HEM_DEMO_ROOT, "bin");
await mkdir(bin, { recursive: true });

await writeExecutable(path.join(bin, "hem"), [
  "#!/bin/sh",
  `exec node "${repo}/apps/cli/dist/src/cli.js" "$@"`,
  ""
].join("\n"));

await writeExecutable(path.join(bin, "npx"), [
  "#!/bin/sh",
  "if [ \"$1\" = \"experimental-codex-harness\" ] && [ \"$2\" = \"install\" ]; then",
  `  exec node "${repo}/scripts/gate2-video-fixture.mjs" simulate-harness-install`,
  "fi",
  "echo \"demo npx wrapper only supports: npx experimental-codex-harness install\" >&2",
  "exit 1",
  ""
].join("\n"));

await writeExecutable(path.join(bin, "verify-rollback"), [
  "#!/bin/sh",
  `exec node "${repo}/scripts/gate2-video-fixture.mjs" verify`,
  ""
].join("\n"));

const env = {
  ...process.env,
  ...demoEnv,
  TMPDIR: "/tmp",
  PATH: `${bin}:${process.env.PATH ?? ""}`
};

async function cleanupDemoPaths() {
  if (demoEnv.HEM_VISIBLE_PROJECT?.endsWith("/hem-demo")) {
    await rm(demoEnv.HEM_VISIBLE_PROJECT, { recursive: true, force: true });
  }
  if (demoEnv.HEM_DEMO_ROOT?.startsWith("/tmp/hem-gate2-demo-")) {
    await rm(demoEnv.HEM_DEMO_ROOT, { recursive: true, force: true });
  }
}

let status = 1;
try {
  const result = spawnSync("vhs", [renderedTape], {
    cwd: repo,
    env,
    stdio: "inherit"
  });
  status = result.status ?? 1;
} finally {
  await cleanupDemoPaths();
}

process.exit(status);
