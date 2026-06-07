import assert from "node:assert/strict";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { writeTar } from "../src/tar.js";
import type { TarEntry } from "../src/types.js";

async function makeTempRoot(): Promise<string> {
  return await import("node:fs/promises").then(({ mkdtemp }) => mkdtemp(join(tmpdir(), "hem-cli-")));
}

function runCli(args: string[], cwd: string, env: NodeJS.ProcessEnv = {}) {
  return spawnSync(process.execPath, [join(process.cwd(), "dist/src/cli.js"), ...args], {
    cwd,
    encoding: "utf8",
    env: { ...process.env, ...env }
  });
}

async function writeCliBundle(root: string, snapshotName: string): Promise<string> {
  const bundlePath = join(root, `${snapshotName}.hem`);
  const entries: TarEntry[] = [
    { path: ".hem/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
    { path: ".hem/format-version", content: Buffer.from("1\n", "utf8"), mode: 0o644, mtime: 1000000, type: "file" },
    {
      path: ".hem/manifest.json",
      content: Buffer.from(JSON.stringify({
        formatVersion: 1,
        snapshotName,
        createdAt: "2026-06-07T00:00:00.000Z",
        projectPath: ".",
        includesContent: false,
        contentFileCount: 0,
        contentTotalBytes: 0,
        sourceMachine: { hostname: "source", platform: "darwin", homeDir: "/Users/source" },
        security: { rawSecretsIncluded: false, redactionPolicy: "metadata-only", signed: false }
      }) + "\n", "utf8"),
      mode: 0o644,
      mtime: 1000000,
      type: "file"
    },
    {
      path: "snapshot/evidence.json",
      content: Buffer.from(JSON.stringify([
        {
          id: "mcp-missing",
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
          name: "missing",
          value: { command: "hem-missing-mcp-binary" }
        },
        {
          id: "env-openai",
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
          name: "OPENAI_API_KEY",
          value: { key: "OPENAI_API_KEY" }
        }
      ]) + "\n", "utf8"),
      mode: 0o644,
      mtime: 1000000,
      type: "file"
    }
  ];
  await writeTar(entries, bundlePath);
  return bundlePath;
}

describe("hem CLI scaffold", () => {
  it("prints help with current diagnosis, restore, and bundle safety commands", () => {
    const result = runCli(["--help"], process.cwd());

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /Diagnosis commands:/);
    assert.match(result.stdout, /hem scan --project/);
    assert.match(result.stdout, /snapshot create --name baseline --metadata-only/);
    assert.match(result.stdout, /diff baseline current --project/);
    assert.match(result.stdout, /audit current --project/);
    assert.match(result.stdout, /provenance current --project/);
    assert.match(result.stdout, /report current --project/);
    assert.match(result.stdout, /hem doctor --project/);
    assert.match(result.stdout, /hem bundle verify <file\.hem>/);
    assert.match(result.stdout, /--apply-content --quarantine --experimental/);
    assert.doesNotMatch(result.stdout, /v0\.1|dry-run only/);
  });

  it("prints current snapshot metadata-only guidance without stale version labels", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });

    const result = runCli(["snapshot", "create", "--name", "baseline", "--project", project], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(result.status, 1);
    assert.match(result.stderr, /Snapshots are metadata-only/);
    assert.match(result.stderr, /Add `--metadata-only`/);
    assert.doesNotMatch(result.stderr, /v0\.1/);
  });

  it("runs the read-only workflow from scan to report", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "stdio", command: "gh-mcp" }
      }
    }));

    const env = { HOME: home, HEM_STORE: store };

    const scan = runCli(["scan", "--project", project], project, env);
    assert.equal(scan.status, 0, scan.stderr);
    assert.match(scan.stdout, /Read-only: yes/);
    assert.match(scan.stdout, /Claude Code/);

    const explain = runCli(["scan", "--project", project, "--explain"], project, env);
    assert.equal(explain.status, 0, explain.stderr);
    assert.match(explain.stdout, /Paths considered/);
    assert.match(explain.stdout, /\.mcp\.json/);

    const create = runCli(["snapshot", "create", "--name", "baseline", "--metadata-only", "--project", project], project, env);
    assert.equal(create.status, 0, create.stderr);
    assert.match(create.stdout, /Created metadata-only snapshot: baseline/);

    const list = runCli(["snapshot", "list"], project, env);
    assert.equal(list.status, 0, list.stderr);
    assert.match(list.stdout, /baseline/);

    const show = runCli(["snapshot", "show", "baseline", "--json"], project, env);
    assert.equal(show.status, 0, show.stderr);
    assert.equal(JSON.parse(show.stdout).manifest.name, "baseline");

    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        github: { transport: "http", url: "https://mcp.example.com/github" }
      }
    }));

    const diff = runCli(["diff", "baseline", "current", "--project", project, "--json"], project, env);
    assert.equal(diff.status, 0, diff.stderr);
    assert.equal(JSON.parse(diff.stdout).semanticChanges[0].code, "MCP_CHANGED");

    const audit = runCli(["audit", "current", "--project", project, "--json"], project, env);
    assert.equal(audit.status, 0, audit.stderr);
    assert.ok(Array.isArray(JSON.parse(audit.stdout)));

    const provenance = runCli(["provenance", "current", "--project", project, "--json"], project, env);
    assert.equal(provenance.status, 0, provenance.stderr);
    assert.ok(Array.isArray(JSON.parse(provenance.stdout)));

    const reportPath = join(root, "hem-report.md");
    const report = runCli(["report", "current", "--project", project, "--out", reportPath], project, env);
    assert.equal(report.status, 0, report.stderr);
    assert.match(await readFile(reportPath, "utf8"), /# hem report: current/);

    const reportJson = runCli(["report", "current", "--project", project, "--json"], project, env);
    assert.equal(reportJson.status, 0, reportJson.stderr);
    assert.equal(JSON.parse(reportJson.stdout).snapshot.manifest.name, "current");
  });

  it("runs doctor with JSON readiness output", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        missingTool: { command: "hem-missing-mcp-binary" }
      }
    }));

    const result = runCli(["doctor", "--project", project, "--json"], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(result.status, 0, result.stderr);
    const report = JSON.parse(result.stdout);
    assert.equal(report.summary.needs_manual_action >= 1, true);
    assert.equal(report.items.some((item: { code: string }) => item.code === "HEM_MCP_COMMAND_MISSING"), true);
  });

  it("runs doctor with human-readable readiness output", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    await writeFile(join(project, ".mcp.json"), JSON.stringify({
      mcpServers: {
        missingTool: { command: "hem-missing-mcp-binary" }
      }
    }));

    const result = runCli(["doctor", "--project", project], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /hem doctor/);
    assert.match(result.stdout, /Readiness:/);
    assert.match(result.stdout, /MCP command hem-missing-mcp-binary is missing/);
    assert.match(result.stdout, /fix:/);
  });

  it("prints bundle import dry-run readiness summary", async () => {
    const root = await makeTempRoot();
    const project = join(root, "project");
    const home = join(root, "home");
    const store = join(root, "store");
    await mkdir(project, { recursive: true });
    await mkdir(home, { recursive: true });
    const bundlePath = await writeCliBundle(root, "cli-readiness");

    const result = runCli(["bundle", "import", bundlePath, "--dry-run", "--project", project], project, {
      HOME: home,
      HEM_STORE: store
    });

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /Readiness:/);
    assert.match(result.stdout, /needs manual action:/);
    assert.match(result.stdout, /MCP command hem-missing-mcp-binary is missing/);
    assert.match(result.stdout, /Environment key OPENAI_API_KEY needs a value/);
  });
});
