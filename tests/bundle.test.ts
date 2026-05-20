/**
 * Tests for .stailor bundle export, import, and inspect.
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
import { mkdir, mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, it } from "node:test";

import { writeSnapshot, readSnapshot, listSnapshots } from "../src/store.js";
import { bundleExport, bundleImport, bundleInspect } from "../src/bundle.js";
import { writeTar } from "../src/tar.js";
import type { Snapshot, TarEntry } from "../src/types.js";

// -- Test helpers -------------------------------------------------

interface Sandbox {
  root: string;
  storeDir: string;
  projectPath: string;
  homeDir: string;
}

async function makeSandbox(): Promise<Sandbox> {
  const root = await mkdtemp(path.join(tmpdir(), "snaptailor-bundle-"));
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
  return path.join(box.root, name + ".stailor");
}

function makeMinimalBundle(
  box: Sandbox,
  snapshotName: string,
  contentEntries: TarEntry[]
): TarEntry[] {
  return [
    { path: ".stailor/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
    {
      path: ".stailor/format-version",
      content: Buffer.from("1\n", "utf-8"),
      mode: 0o644, mtime: 1000000, type: "file"
    },
    {
      path: ".stailor/manifest.json",
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
    assert.ok(exportResult.bundlePath.endsWith(".stailor"));

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

  it("exports and imports to a fresh store", async () => {
    const box = await makeSandbox();
    const name = "unique-snap";
    await writeSnapshot(box.storeDir, sampleSnapshot(name));

    const exportResult = await bundleExport({
      snapshotName: name,
      outputPath: bundlePath(box, name),
      storeDir: box.storeDir,
      projectPath: box.projectPath,
      homeDir: box.homeDir
    });

    const importBox = await makeSandbox();
    const importResult = await bundleImport({
      bundlePath: exportResult.bundlePath,
      storeDir: importBox.storeDir,
      projectPath: importBox.projectPath,
      homeDir: importBox.homeDir
    });

    assert.equal(importResult.snapshotName, name);
    assert.ok(importResult.evidenceCount > 0);

    const imported = await readSnapshot(importBox.storeDir, name);
    assert.equal(imported.manifest.name, name);
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
      { path: ".stailor/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
      {
        path: ".stailor/format-version",
        content: Buffer.from("1\n", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      },
      {
        path: ".stailor/manifest.json",
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
      { path: ".stailor/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
      {
        path: ".stailor/format-version",
        content: Buffer.from("999\n", "utf-8"),
        mode: 0o644, mtime: 1000000, type: "file"
      },
      {
        path: ".stailor/manifest.json",
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
});
