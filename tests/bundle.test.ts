/**
 * Tests for .hem bundle export, import, and inspect.
 *
 * Covers:
 * - Export -> Import roundtrip (snapshot fidelity)
 * - Export -> Inspect metadata
 * - Security: content paths starting with ~/ are blocked
 * - Security: sensitive directory paths (.ssh, .aws) are blocked
 * - Security: path traversal via .. in content path is blocked
 * - Security: invalid format version rejection
 * - Checksum validation on import
 * - Dry-run import
 */
import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { platform, tmpdir } from "node:os";
import path from "node:path";
import { describe, it } from "node:test";

import { writeSnapshot, readSnapshot, listSnapshots } from "../src/store.js";
import { bundleExport, bundleImport, bundleInspect, bundleVerify } from "../src/bundle.js";
import { readTar, writeTar } from "../src/tar.js";
import type { Snapshot, TarEntry } from "../src/types.js";

// -- Test helpers -------------------------------------------------

interface Sandbox {
  root: string;
  storeDir: string;
  projectPath: string;
  homeDir: string;
}

async function makeSandbox(): Promise<Sandbox> {
  const root = await mkdtemp(path.join(tmpdir(), "hem-bundle-"));
  const storeDir = path.join(root, "store");
  const projectPath = path.join(root, "project");
  const homeDir = path.join(root, "home");

  await mkdir(storeDir, { recursive: true });
  await mkdir(projectPath, { recursive: true });
  await mkdir(homeDir, { recursive: true });

  return { root, storeDir, projectPath, homeDir };
}

function sampleSnapshot(name: string): Snapshot {
  return {
    manifest: {
      schemaVersion: "0.1",
      name,
      createdAt: "2026-05-12T00:00:00.000Z",
      projectPath: "/tmp/project",
      security: {
        rawSecretsIncluded: false,
        redactionPolicy: "metadata-only"
      }
    },
    evidence: [
      {
        id: "project.claude-code..mcp.json.mcp-github",
        agent: "claude-code",
        kind: "mcp_server",
        sourcePath: ".mcp.json",
        scope: "project",
        precedence: 40,
        parser: "json",
        sensitivity: "command_config",
        contentPolicy: "structured_safe_fields_only",
        restorePolicy: "not_supported",
        captureStatus: "captured",
        confidence: "high",
        name: "github",
        value: { command: "gh", args: ["api"] }
      }
    ],
    graph: [
      {
        id: "node.project.claude-code.mcp_server.github",
        agent: "claude-code",
        scope: "project",
        sourcePath: ".mcp.json",
        entityKind: "mcp_server",
        entityName: "github",
        effectiveValue: { command: "gh", args: ["api"] },
        confidence: "high",
        evidenceId: "project.claude-code..mcp.json.mcp-github"
      }
    ],
    auditFindings: [
      {
        code: "EXECUTABLE_CONFIG_ADDED",
        severity: "high",
        problem: "MCP server references an executable command.",
        cause: ".mcp.json github: command = gh.",
        fix: "Confirm the command is trusted.",
        path: ".mcp.json",
        evidenceId: "project.claude-code..mcp.json.mcp-github"
      }
    ],
    provenance: [
      {
        nodeId: "node.project.claude-code.mcp_server.github",
        evidenceId: "project.claude-code..mcp.json.mcp-github",
        sourcePath: ".mcp.json",
        scope: "project",
        precedence: 40,
        confidence: "high",
        captureStatus: "captured"
      }
    ]
  };
}

function bundlePath(box: Sandbox, name: string): string {
  return path.join(box.root, name + ".hem");
}

function makeMinimalBundle(
  box: Sandbox,
  snapshotName: string,
  contentEntries: TarEntry[]
): TarEntry[] {
  return [
    { path: ".hem/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
    {
      path: ".hem/format-version",
      content: Buffer.from("1\n", "utf-8"),
      mode: 0o644, mtime: 1000000, type: "file"
    },
    {
      path: ".hem/manifest.json",
      content: Buffer.from(JSON.stringify({
        formatVersion: 1,
        snapshotName,
        createdAt: "2026-05-12T00:00:00.000Z",
        projectPath: box.projectPath,
        includesContent: true,
        contentFileCount: contentEntries.length,
        contentTotalBytes: 0,
        security: { rawSecretsIncluded: false, redactionPolicy: "metadata-only", signed: false }
      }) + "\n", "utf-8"),
      mode: 0o644, mtime: 1000000, type: "file"
    },
    ...contentEntries
  ];
}

// -- Export -> Import roundtrip -----------------------------------

describe("bundle export/import roundtrip", () => {
  it("exports a snapshot and imports it back with identical evidence", async () => {
    const box = await makeSandbox();
    const snapshotName = "roundtrip-test";
    await writeSnapshot(box.storeDir, sampleSnapshot(snapshotName));

    const exportResult = await bundleExport({
      snapshotName,
      outputPath: bundlePath(box, snapshotName),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir
    });

    assert.ok(exportResult.checksum.length > 0);
    assert.ok(exportResult.bundlePath.endsWith(".hem"));

    const importResult = await bundleImport({
      bundlePath: exportResult.bundlePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir
    });

    assert.equal(importResult.snapshotName, snapshotName);
    assert.equal(importResult.evidenceCount, 1);
    assert.equal(importResult.contentApplied, false);

    const imported = await readSnapshot(box.storeDir, snapshotName);
    assert.deepEqual(imported.evidence, sampleSnapshot(snapshotName).evidence);
    assert.deepEqual(imported.graph, sampleSnapshot(snapshotName).graph);
    assert.deepEqual(imported.auditFindings, sampleSnapshot(snapshotName).auditFindings);
  });

  it("stores home-scoped evidence paths as {home} and resolves them on import", async () => {
    const box = await makeSandbox();
    const name = "home-abstraction";
    const homeSettingsPath = path.join(box.homeDir, ".claude", "settings.json");
    await mkdir(path.dirname(homeSettingsPath), { recursive: true });
    await writeFile(homeSettingsPath, "{}", "utf-8");

    const snapshot = sampleSnapshot(name);
    snapshot.evidence[0] = {
      ...snapshot.evidence[0],
      kind: "agent_config",
      sourcePath: homeSettingsPath,
      restorePolicy: "full_content_supported",
      value: {}
    };
    snapshot.graph[0] = {
      ...snapshot.graph[0],
      sourcePath: homeSettingsPath,
      entityKind: "agent_config"
    };
    snapshot.provenance[0] = {
      ...snapshot.provenance[0],
      sourcePath: homeSettingsPath
    };
    await writeSnapshot(box.storeDir, snapshot);

    const exportResult = await bundleExport({
      snapshotName: name,
      outputPath: bundlePath(box, name),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      includeContent: true
    });
    const { entries } = await readTar(exportResult.bundlePath);
    const bundledEvidence = JSON.parse(entries.find((entry) => entry.path === "snapshot/evidence.json")!.content.toString("utf-8"));
    assert.equal(bundledEvidence[0].sourcePath, "{home}/.claude/settings.json");

    const importBox = await makeSandbox();
    await bundleImport({
      bundlePath: exportResult.bundlePath,
      storeDir: importBox.storeDir,
      projectPath: importBox.projectPath,
      homeDir: importBox.homeDir
    });
    const imported = await readSnapshot(importBox.storeDir, name);
    assert.equal(imported.evidence[0].sourcePath, path.join(importBox.homeDir, ".claude", "settings.json"));
    assert.equal(imported.graph[0].sourcePath, path.join(importBox.homeDir, ".claude", "settings.json"));
    assert.equal(imported.provenance[0].sourcePath, path.join(importBox.homeDir, ".claude", "settings.json"));
  });
});

// -- Export policy validation -------------------------------------

describe("bundle export policy validation", () => {
  it("rejects content bundles when not_supported evidence would lose restore data", async () => {
    const box = await makeSandbox();
    const name = "unsupported-policy";
    const snapshot = sampleSnapshot(name);
    snapshot.evidence.push({
      id: "project.symlink.claude-settings",
      agent: "claude-code",
      kind: "symlink",
      sourcePath: ".claude/settings.json",
      scope: "project",
      precedence: 40,
      parser: "filesystem",
      sensitivity: "path_only",
      contentPolicy: "metadata_only",
      restorePolicy: "not_supported",
      captureStatus: "captured",
      confidence: "high",
      name: "settings-symlink",
      value: { target: "/tmp/outside" }
    });
    await writeSnapshot(box.storeDir, snapshot);

    await assert.rejects(
      () => bundleExport({
        snapshotName: name,
        outputPath: bundlePath(box, name),
        storeDir: box.storeDir,
        projectPath: box.projectPath,
        homeDir: box.homeDir,
        includeContent: true
      }),
      /not_supported.*would lose restore data/
    );
  });
});

// -- Bundle signatures -------------------------------------------

describe("bundle signatures", () => {
  it("signs manifest and content with HMAC-SHA256 when a signature key is provided", async () => {
    const box = await makeSandbox();
    const name = "signed-bundle";
    await writeSnapshot(box.storeDir, sampleSnapshot(name));

    const exportResult = await bundleExport({
      snapshotName: name,
      outputPath: bundlePath(box, name),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      signatureKey: "test-secret"
    });

    const inspectResult = await bundleInspect(exportResult.bundlePath);
    assert.equal(inspectResult.isSigned, true);
    assert.equal(inspectResult.signatureAlgorithm, "HMAC-SHA256");

    const { entries } = await readTar(exportResult.bundlePath);
    const manifest = JSON.parse(entries.find((entry) => entry.path === ".hem/manifest.json")!.content.toString("utf-8"));
    assert.match(manifest.security.signature, /^[a-f0-9]{64}$/);
  });

  it("supports trust-on-first-use for signed bundle keys", async () => {
    const box = await makeSandbox();
    const trustedName = "trusted-key";
    const otherName = "other-key";
    await writeSnapshot(box.storeDir, sampleSnapshot(trustedName));
    await writeSnapshot(box.storeDir, { ...sampleSnapshot(otherName), manifest: { ...sampleSnapshot(otherName).manifest, name: otherName } });

    const trustedBundle = await bundleExport({
      snapshotName: trustedName,
      outputPath: bundlePath(box, trustedName),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      signatureKey: "trusted-secret"
    });
    const otherBundle = await bundleExport({
      snapshotName: otherName,
      outputPath: bundlePath(box, otherName),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      signatureKey: "other-secret"
    });

    const firstImport = await bundleImport({
      bundlePath: trustedBundle.bundlePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      signatureKey: "trusted-secret",
      trust: true,
      dryRun: true
    });
    assert.ok(firstImport.warnings.some((warning) => warning.includes("Trusted bundle signing key")));

    await assert.rejects(
      () => bundleImport({
        bundlePath: otherBundle.bundlePath,
        storeDir: box.storeDir,
        projectPath: box.projectPath,
        homeDir: box.homeDir,
        signatureKey: "other-secret",
        dryRun: true
      }),
      /does not match trusted key fingerprint/
    );
  });

  it("quarantines content for inspection instead of applying target files", async () => {
    const box = await makeSandbox();
    const name = "quarantine-content";
    const projectFile = path.join(box.projectPath, "config", "tool.json");
    await mkdir(path.dirname(projectFile), { recursive: true });
    await writeFile(projectFile, "safe content", "utf-8");
    await writeSnapshot(box.storeDir, {
      ...sampleSnapshot(name),
      evidence: [
        {
          ...sampleSnapshot(name).evidence[0],
          id: "project.config.tool",
          kind: "agent_config",
          sourcePath: "config/tool.json",
          restorePolicy: "full_content_supported"
        }
      ]
    });
    const exported = await bundleExport({
      snapshotName: name,
      outputPath: bundlePath(box, name),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      includeContent: true
    });
    await writeFile(projectFile, "local content", "utf-8");

    const imported = await bundleImport({
      bundlePath: exported.bundlePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      applyContent: true,
      quarantine: true,
      targetPlatform: "darwin"
    });

    assert.equal(imported.contentApplied, false);
    assert.ok(imported.quarantinedContentDir);
    assert.equal(await readFile(projectFile, "utf-8"), "local content");
    assert.equal(await readFile(path.join(imported.quarantinedContentDir, "config", "tool.json"), "utf-8"), "safe content");
    assert.ok(imported.warnings.some((warning) => warning.includes("quarantined")));
  });
});

// -- Export -> Inspect --

describe("bundle inspect", () => {
  it("inspects an exported bundle and returns correct metadata", async () => {
    const box = await makeSandbox();
    const name = "inspect-test";
    await writeSnapshot(box.storeDir, sampleSnapshot(name));

    const exportResult = await bundleExport({
      snapshotName: name,
      outputPath: bundlePath(box, name),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir
    });

    const inspectResult = await bundleInspect(exportResult.bundlePath);

    assert.equal(inspectResult.formatVersion, 1);
    assert.equal(inspectResult.snapshotName, name);
    assert.equal(inspectResult.includesContent, false);
    assert.equal(inspectResult.isSigned, false);
    assert.equal(inspectResult.contentFileCount, 0);
    assert.equal(inspectResult.contentTotalBytes, 0);
    assert.ok(inspectResult.bundleChecksum.length > 0);
  });
});

// -- Security: content path --

describe("bundle import security -- content path validation", () => {
  it("blocks content paths starting with ~/", async () => {
    const box = await makeSandbox();
    const entries = makeMinimalBundle(box, "malicious", [
      {
        path: "content/~/.ssh/authorized_keys",
        content: Buffer.from("evil", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      }
    ]);
    const bundleFilePath = bundlePath(box, "evil-home");
    await writeTar(entries, bundleFilePath);

    await assert.rejects(
      () => bundleImport({
        bundlePath: bundleFilePath,
        storeDir: box.storeDir,
        projectPath: box.projectPath,
        homeDir: box.homeDir,
        applyContent: true
      }),
      /Home-relative content path/
    );
  });

  it("blocks .ssh content paths", async () => {
    const box = await makeSandbox();
    const entries = makeMinimalBundle(box, "evil-ssh", [
      {
        path: "content/.ssh/config",
        content: Buffer.from("evil", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      }
    ]);
    const bundleFilePath = bundlePath(box, "evil-ssh");
    await writeTar(entries, bundleFilePath);

    await assert.rejects(
      () => bundleImport({
        bundlePath: bundleFilePath,
        storeDir: box.storeDir,
        projectPath: box.projectPath,
        homeDir: box.homeDir,
        applyContent: true
      }),
      /Blocked content path prefix/
    );
  });

  it("blocks content paths with .. traversal", async () => {
    const box = await makeSandbox();
    const entries = makeMinimalBundle(box, "dotdot", [
      {
        path: "content/../../etc/passwd",
        content: Buffer.from("root:x:0:0:", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      }
    ]);
    const bundleFilePath = bundlePath(box, "evil-dotdot");
    await writeTar(entries, bundleFilePath);

    await assert.rejects(
      () => bundleImport({
        bundlePath: bundleFilePath,
        storeDir: box.storeDir,
        projectPath: box.projectPath,
        homeDir: box.homeDir,
        applyContent: true
      }),
      /\.\./
    );
  });

  it("blocks absolute content paths", async () => {
    const box = await makeSandbox();
    const entries = [
      { path: ".hem/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
      {
        path: ".hem/format-version",
        content: Buffer.from("1\n", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      },
      {
        path: ".hem/manifest.json",
        content: Buffer.from(JSON.stringify({
          formatVersion: 1, snapshotName: "abs",
          createdAt: "2026-05-12T00:00:00.000Z",
          projectPath: box.projectPath,
          includesContent: false, contentFileCount: 0, contentTotalBytes: 0,
          security: { rawSecretsIncluded: false, redactionPolicy: "metadata-only", signed: false }
        }) + "\n", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      },
      {
        path: "/etc/passwd",
        content: Buffer.from("root:x:0:0:", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      }
    ] as TarEntry[];
    const bundleFilePath = bundlePath(box, "evil-abs");
    await writeTar(entries, bundleFilePath);

    await assert.rejects(
      () => bundleImport({
        bundlePath: bundleFilePath, storeDir: box.storeDir,
        projectPath: box.projectPath, homeDir: box.homeDir,
        applyContent: true
      }),
      /Path traversal|absolute/
    );
  });
});

// -- Format version rejection --

describe("bundle import -- format version", () => {
  it("rejects unsupported format version", async () => {
    const box = await makeSandbox();
    const entries: TarEntry[] = [
      { path: ".hem/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
      {
        path: ".hem/format-version",
        content: Buffer.from("999\n", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      },
      {
        path: ".hem/manifest.json",
        content: Buffer.from(JSON.stringify({
          formatVersion: 999, snapshotName: "bad",
          createdAt: "2026-05-12T00:00:00.000Z",
          projectPath: box.projectPath,
          includesContent: false, contentFileCount: 0, contentTotalBytes: 0,
          security: { rawSecretsIncluded: false, redactionPolicy: "metadata-only", signed: false }
        }) + "\n", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      }
    ];
    const bundleFilePath = bundlePath(box, "bad-format");
    await writeTar(entries, bundleFilePath);

    await assert.rejects(
      () => bundleImport({
        bundlePath: bundleFilePath, storeDir: box.storeDir,
        projectPath: box.projectPath, homeDir: box.homeDir
      }),
      /Unsupported bundle format version/
    );
  });
});

// -- Dry-run import --

describe("bundle import -- dry-run", () => {
  it("dry-run returns metadata without writing to store", async () => {
    const box = await makeSandbox();
    const name = "dry-run-test";
    await writeSnapshot(box.storeDir, sampleSnapshot(name));

    const exportResult = await bundleExport({
      snapshotName: name,
      outputPath: bundlePath(box, name),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir
    });

    const allBefore = await listSnapshots(box.storeDir);

    const result = await bundleImport({
      bundlePath: exportResult.bundlePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      dryRun: true
    });

    const allAfter = await listSnapshots(box.storeDir);

    assert.equal(result.snapshotName, name);
    assert.equal(result.contentApplied, false);
    assert.deepEqual(allBefore, allAfter); // store unchanged
  });

  it("reports cross-OS compatibility differences in machine diff", async () => {
    const box = await makeSandbox();
    const sourcePlatform = platform() === "darwin" ? "linux" : "darwin";
    const entries = makeMinimalBundle(box, "cross-os", [
      {
        path: "content/{home}/.claude/settings.json",
        content: Buffer.from("{}", "utf-8"),
        mode: 0o644,
        mtime: 1000000,
        type: "file"
      }
    ]);
    const manifestEntry = entries.find((entry) => entry.path === ".hem/manifest.json");
    assert.ok(manifestEntry);
    const manifest = JSON.parse(manifestEntry.content.toString("utf-8"));
    manifest.sourceMachine = {
      hostname: "source-host",
      platform: sourcePlatform,
      arch: "arm64",
      homeDir: "/Users/source",
      projectPath: "/Users/source/project"
    };
    manifestEntry.content = Buffer.from(JSON.stringify(manifest) + "\n", "utf-8");

    const bundleFilePath = bundlePath(box, "cross-os");
    await writeTar(entries, bundleFilePath);

    const result = await bundleImport({
      bundlePath: bundleFilePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      dryRun: true
    });

    assert.equal(result.machineDiff?.crossOS, true);
    assert.ok(result.machineDiff?.targetHostname);
    assert.ok(result.machineDiff?.osDifferences.some((difference) => difference.includes("cross-OS restore")));
    const expectedSourcePrefix = sourcePlatform === "linux" ? "/home/source" : "/Users/source";
    assert.ok(result.machineDiff?.remappedPaths.some((mapping) => mapping.includes(`${expectedSourcePrefix}/.claude/settings.json`)));
  });

  it("classifies MCP package runners and source-local binary mismatches", async () => {
    const box = await makeSandbox();
    const entries = makeMinimalBundle(box, "mcp-binaries", []);
    const manifestEntry = entries.find((entry) => entry.path === ".hem/manifest.json");
    assert.ok(manifestEntry);
    const manifest = JSON.parse(manifestEntry.content.toString("utf-8"));
    manifest.sourceMachine = {
      hostname: "source-host",
      platform: "darwin",
      homeDir: "/Users/alice"
    };
    manifestEntry.content = Buffer.from(JSON.stringify(manifest) + "\n", "utf-8");
    entries.push({
      path: "snapshot/evidence.json",
      content: Buffer.from(JSON.stringify([
        {
          ...sampleSnapshot("mcp-binaries").evidence[0],
          id: "mcp-npx",
          value: { command: "npx", args: ["-y", "@modelcontextprotocol/server-github"] }
        },
        {
          ...sampleSnapshot("mcp-binaries").evidence[0],
          id: "mcp-local",
          value: { command: "/Users/alice/.local/bin/private-mcp" }
        },
        {
          ...sampleSnapshot("mcp-binaries").evidence[0],
          id: "mcp-remote",
          value: { url: "https://mcp.example.test" }
        },
        {
          ...sampleSnapshot("mcp-binaries").evidence[0],
          id: "env-openai",
          agent: "project",
          kind: "env_key",
          sourcePath: ".env",
          parser: "dotenv",
          sensitivity: "env_key_inventory",
          contentPolicy: "key_inventory_only",
          restorePolicy: "key_inventory_only",
          captureStatus: "redacted",
          name: "OPENAI_API_KEY",
          value: { key: "OPENAI_API_KEY" }
        }
      ]) + "\n", "utf-8"),
      mode: 0o644,
      mtime: 1000000,
      type: "file"
    });
    const bundleFilePath = bundlePath(box, "mcp-binaries");
    await writeTar(entries, bundleFilePath);

    const result = await bundleImport({
      bundlePath: bundleFilePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      dryRun: true
    });

    const npxReport = result.machineDiff?.mcpBinaryReport.find((report) => report.evidenceId === "mcp-npx");
    const localReport = result.machineDiff?.mcpBinaryReport.find((report) => report.evidenceId === "mcp-local");
    assert.equal(npxReport?.binaryKind, "package_runner");
    assert.match(npxReport?.warning ?? "", /Package runner npx/);
    assert.equal(localReport?.binaryKind, "source_local_path");
    assert.equal(localReport?.availableOnTarget, false);
    assert.match(localReport?.warning ?? "", /source machine local binary path/);
    assert.equal(result.readiness.summary.needs_manual_action >= 2, true);
    assert.equal(result.readiness.summary.unverified, 1);
    assert.equal(
      result.readiness.items.some((item) => item.code === "HEM_ENV_VALUE_REQUIRED" && item.problem.includes("OPENAI_API_KEY")),
      true
    );
    assert.equal(JSON.stringify(result.readiness).includes("sk-real-secret"), false);
  });

  it("checks MCP command availability without invoking a shell", async () => {
    const box = await makeSandbox();
    const markerPath = path.join(box.root, "shell-injection-marker");
    const maliciousCommand = `missing\" ; touch \"${markerPath}\" ; \"`;
    const entries = makeMinimalBundle(box, "mcp-shell-safe", []);
    entries.push({
      path: "snapshot/evidence.json",
      content: Buffer.from(JSON.stringify([
        {
          ...sampleSnapshot("mcp-shell-safe").evidence[0],
          id: "mcp-malicious",
          value: { command: maliciousCommand }
        }
      ]) + "\n", "utf-8"),
      mode: 0o644,
      mtime: 1000000,
      type: "file"
    });
    const bundleFilePath = bundlePath(box, "mcp-shell-safe");
    await writeTar(entries, bundleFilePath);

    const result = await bundleImport({
      bundlePath: bundleFilePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      dryRun: true
    });

    const report = result.machineDiff?.mcpBinaryReport.find((item) => item.evidenceId === "mcp-malicious");
    assert.equal(report?.availableOnTarget, false);
    await assert.rejects(readFile(markerPath, "utf-8"), /ENOENT/);
  });

  it("blocks content apply outside macOS before writing content", async () => {
    const box = await makeSandbox();
    const entries = makeMinimalBundle(box, "mac-apply-only", [
      {
        path: "content/config/tool.json",
        content: Buffer.from("unsafe write", "utf-8"),
        mode: 0o644,
        mtime: 1000000,
        type: "file"
      }
    ]);
    const bundleFilePath = bundlePath(box, "mac-apply-only");
    await writeTar(entries, bundleFilePath);

    await assert.rejects(
      bundleImport({
        bundlePath: bundleFilePath,
        storeDir: box.storeDir,
        projectPath: box.projectPath,
        homeDir: box.homeDir,
        applyContent: true,
        targetPlatform: "linux"
      }),
      /HEM_MACOS_APPLY_ONLY/
    );
    await assert.rejects(readFile(path.join(box.projectPath, "config", "tool.json"), "utf-8"), /ENOENT/);
  });

  it("returns non-macOS apply blockers during dry-run instead of throwing", async () => {
    const box = await makeSandbox();
    const entries = makeMinimalBundle(box, "mac-dry-run-blocker", []);
    const bundleFilePath = bundlePath(box, "mac-dry-run-blocker");
    await writeTar(entries, bundleFilePath);

    const result = await bundleImport({
      bundlePath: bundleFilePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      applyContent: true,
      dryRun: true,
      targetPlatform: "linux"
    });

    assert.equal(result.readiness.summary.blocked, 1);
    assert.equal(result.readiness.items[0]?.code, "HEM_MACOS_APPLY_ONLY");
    await assert.rejects(readSnapshot(box.storeDir, "mac-dry-run-blocker"), /ENOENT/);
  });

  it("reports non-macOS apply limitations during ordinary dry-run", async () => {
    const box = await makeSandbox();
    const entries = makeMinimalBundle(box, "mac-dry-run-limitation", []);
    const bundleFilePath = bundlePath(box, "mac-dry-run-limitation");
    await writeTar(entries, bundleFilePath);

    const result = await bundleImport({
      bundlePath: bundleFilePath,
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir,
      dryRun: true,
      targetPlatform: "linux"
    });

    assert.equal(result.readiness.summary.unsupported, 1);
    assert.equal(result.readiness.items[0]?.code, "HEM_MACOS_APPLY_ONLY");
  });
});
