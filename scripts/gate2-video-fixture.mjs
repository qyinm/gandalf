#!/usr/bin/env node
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { homedir, tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const gandalf = path.join(repo, "bin", "gandalf");

const cleanConfig = [
  "model = \"gpt-5\"",
  "approval_policy = \"on-request\"",
  "sandbox_mode = \"workspace-write\"",
  ""
].join("\n");

const badConfig = [
  "model = \"experimental-harness-model\"",
  "approval_policy = \"never\"",
  "sandbox_mode = \"danger-full-access\"",
  ""
].join("\n");

function sh(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function requireEnv(name) {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required`);
  return value;
}

async function setup(envPath) {
  if (!existsSync(gandalf)) {
    throw new Error("bin/gandalf is missing. Run `make build` before recording the demo.");
  }

  const root = await mkdtemp(path.join(tmpdir(), "gandalf-gate2-demo-"));
  const visibleProject = path.join(homedir(), "gandalf-demo");
  const home = path.join(visibleProject, "home");
  const codexHome = path.join(root, "codex-home");
  const store = path.join(root, "store");
  const project = visibleProject;
  const codexDir = path.join(home, ".codex");

  if (visibleProject === homedir() || visibleProject === "/" || !visibleProject.endsWith("gandalf-demo")) {
    throw new Error(`Refusing unsafe visible project path: ${visibleProject}`);
  }
  await rm(visibleProject, { recursive: true, force: true });
  await mkdir(codexDir, { recursive: true });
  await mkdir(codexHome, { recursive: true });
  await mkdir(project, { recursive: true });
  await writeFile(path.join(codexDir, "config.toml"), cleanConfig, "utf8");
  await writeFile(path.join(codexHome, "config.toml"), [
    "model = \"gpt-5.5\"",
    "approval_policy = \"never\"",
    "sandbox_mode = \"read-only\"",
    "",
    "[shell_environment_policy]",
    "inherit = \"all\"",
    ""
  ].join("\n"), "utf8");
  await writeFile(path.join(project, "README.md"), "Disposable Gate 2 video demo project.\n", "utf8");
  await writeFile(path.join(project, "restore-request.txt"), [
    "User request: restore before prev agent setup",
    "",
    "Context:",
    "- An experimental harness changed ~/.codex/config.toml.",
    "- It added ~/.codex/skills/synthetic-harness/SKILL.md.",
    "- There is no known-good baseline snapshot or backup in this environment.",
    "",
    "Do not run shell commands and do not modify files.",
    "Reply in four lines or fewer.",
    "Use ~/.codex paths only, not absolute temp paths.",
    "Explain whether this can be safely restored."
  ].join("\n"), "utf8");

  await writeFile(envPath, [
    `export REAL_HOME=${sh(homedir())}`,
    `export GANDALF_DEMO_ROOT=${sh(root)}`,
    `export CODEX_HOME=${sh(codexHome)}`,
    `export HOME=${sh(home)}`,
    `export GANDALF_STORE=${sh(store)}`,
    `export GANDALF_PROJECT=${sh(project)}`,
    `export GANDALF_VISIBLE_PROJECT=${sh(visibleProject)}`,
    `export GANDALF_REPO=${sh(repo)}`,
    "export GANDALF_UPDATE_CHECK=0",
    ""
  ].join("\n"), "utf8");
}

async function simulateHarnessInstall() {
  const codexDir = path.join(requireEnv("HOME"), ".codex");
  await writeFile(path.join(codexDir, "config.toml"), badConfig, "utf8");
  const skill = path.join(codexDir, "skills", "synthetic-harness", "SKILL.md");
  await mkdir(path.dirname(skill), { recursive: true });
  await writeFile(skill, "---\nname: synthetic-harness\n---\nDemo skill installed by a fake harness.\n", "utf8");

  console.log("experimental-codex-harness installed");
  console.log("  changed ~/.codex/config.toml");
  console.log("  added   ~/.codex/skills/synthetic-harness/SKILL.md");
  console.log("  uninstall command: not available");
}

function explainCodexRestoreAttempt() {
  console.log("Codex agent:");
  console.log("  I can inspect the current Codex files, but I do not know the previous state.");
  console.log("  Current changes I can see:");
  console.log("    ~/.codex/config.toml");
  console.log("    ~/.codex/skills/synthetic-harness/SKILL.md");
  console.log("");
  console.log("  Without a snapshot, restore would be a guess.");
  console.log("  I need a known-good baseline or backup to safely roll this back.");
}

async function verify() {
  const codexDir = path.join(requireEnv("HOME"), ".codex");
  const configPath = path.join(codexDir, "config.toml");
  const skill = path.join(codexDir, "skills", "synthetic-harness", "SKILL.md");

  assert.equal(await readFile(configPath, "utf8"), cleanConfig);
  assert.equal(existsSync(skill), false);

  console.log("Result:");
  console.log("  config.toml restored");
  console.log("  synthetic harness skill removed");
  console.log("  real ~/.codex was never targeted");
  console.log("");
  console.log("Install: curl -fsSL https://raw.githubusercontent.com/qyinm/gandalf/main/install.sh | sh");
}

async function cleanup() {
  const root = requireEnv("GANDALF_DEMO_ROOT");
  await rm(root, { recursive: true, force: true });
}

const command = process.argv[2];
if (command === "setup") {
  const envPath = process.argv[3];
  if (!envPath) throw new Error("usage: gate2-video-fixture.mjs setup <env-path>");
  await setup(envPath);
} else if (command === "simulate-harness-install") {
  await simulateHarnessInstall();
} else if (command === "codex-restore-attempt") {
  explainCodexRestoreAttempt();
} else if (command === "verify") {
  await verify();
} else if (command === "cleanup") {
  await cleanup();
} else {
  throw new Error("usage: gate2-video-fixture.mjs <setup|simulate-harness-install|verify|cleanup>");
}
