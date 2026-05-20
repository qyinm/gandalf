/**
 * .stailor bundle export, import, and inspect.
 *
 * A .stailor bundle is a POSIX ustar tar archive containing:
 *   .stailor/format-version   — "1"
 *   .stailor/manifest.json    — BundleManifest
 *   .stailor/checksums.json   — BundleChecksums
 *   snapshot/evidence.json    — DiscoveredItem[]
 *   snapshot/graph.json       — GraphNode[]
 *   snapshot/audit-findings.json — AuditFinding[]
 *   snapshot/checksums.json   — ChecksumRecord
 *   snapshot/redactions.json  — redaction log
 *   content/...               — optional raw file contents (full_content_supported items only)
 */

import { createHash } from "node:crypto";
import { readFile, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { restorePolicyFor } from "./policy.js";
import { readSnapshot } from "./store.js";
import { readTar, writeTar } from "./tar.js";
import type {
  BundleChecksums,
  BundleExportOptions,
  BundleImportOptions,
  BundleImportResult,
  BundleInspectResult,
  BundleManifest,
  DiscoveredItem,
  RestorePolicy,
  TarEntry
} from "./types.js";

const FORMAT_VERSION = "1";
const MAX_BUNDLE_BYTES = parseInt(process.env.SNAPTAILOR_MAX_BUNDLE_BYTES ?? "", 10) || 500 * 1024 * 1024;
const MAX_CONTENT_BYTES = parseInt(process.env.SNAPTAILOR_MAX_CONTENT_BYTES ?? "", 10) || 50 * 1024 * 1024;

/** Result of a bundle export, including any warnings. */
export interface BundleExportResult {
  bundlePath: string;
  checksum: string;
  warnings: string[];
}

// ── Export ──────────────────────────────────────────────────────

/**
 * Export a snapshot to a .stailor bundle file.
 *
 * Content inclusion respects per-kind restore policies:
 *   - full_content_supported  → file bytes included in content/ entries
 *   - structured_fields_only  → already captured in evidence.json, no content/ entry
 *   - key_inventory_only      → key names only in evidence.json, no content/ entry
 *   - not_supported           → warning emitted, no content included
 */
export async function bundleExport(options: BundleExportOptions): Promise<BundleExportResult> {
  const { snapshotName, outputPath, storeDir, projectPath, homeDir, includeContent } = options;

  // Read snapshot from store
  const snapshot = await readSnapshot(storeDir, snapshotName);

  // Validate no unsafe-to-export items
  const unsafeItems = snapshot.evidence.filter((item) => item.captureStatus === "unsafe_to_export");
  if (unsafeItems.length > 0) {
    throw new Error(
      `Cannot export: ${unsafeItems.length} evidence item(s) are marked unsafe_to_export. ` +
      `First: ${unsafeItems[0].sourcePath}`
    );
  }

  const warnings: string[] = [];

  // Build tar entries
  const entries: TarEntry[] = [];

  // .stailor/ directory
  entries.push({ path: ".stailor/", content: Buffer.alloc(0), mode: 0o755, mtime: Date.now(), type: "directory" });

  // .stailor/format-version
  entries.push({
    path: ".stailor/format-version",
    content: Buffer.from(`${FORMAT_VERSION}\n`, "utf-8"),
    mode: 0o644,
    mtime: Date.now(),
    type: "file"
  });

  // Build manifest (content stats populated later if includeContent)
  const manifest: BundleManifest = {
    formatVersion: 1,
    snapshotName,
    createdAt: snapshot.manifest.createdAt,
    projectPath,
    includesContent: includeContent ?? false,
    contentFileCount: 0,
    contentTotalBytes: 0,
    security: {
      rawSecretsIncluded: false,
      redactionPolicy: "metadata-only",
      signed: false
    }
  };

  // .stailor/manifest.json
  entries.push({
    path: ".stailor/manifest.json",
    content: Buffer.from(JSON.stringify(manifest, null, 2) + "\n", "utf-8"),
    mode: 0o644,
    mtime: Date.now(),
    type: "file"
  });

  // snapshot/ directory
  entries.push({ path: "snapshot/", content: Buffer.alloc(0), mode: 0o755, mtime: Date.now(), type: "directory" });

  // snapshot/ files
  const snapshotFiles: Array<{ name: string; data: unknown }> = [
    { name: "evidence.json", data: snapshot.evidence },
    { name: "graph.json", data: snapshot.graph },
    { name: "audit-findings.json", data: snapshot.auditFindings },
    { name: "checksums.json", data: {} },
    { name: "redactions.json", data: [] }
  ];

  for (const file of snapshotFiles) {
    entries.push({
      path: `snapshot/${file.name}`,
      content: Buffer.from(JSON.stringify(file.data, null, 2) + "\n", "utf-8"),
      mode: 0o644,
      mtime: Date.now(),
      type: "file"
    });
  }

  // Optional content files — per-kind restore policy filtering
  if (includeContent) {
    let totalContentBytes = 0;
    let contentCount = 0;

    // Filter evidence by restore policy:
    //   full_content_supported  → include file content
    //   structured_fields_only  → skip (already in evidence.json)
    //   key_inventory_only      → skip (key names only, already in evidence.json)
    //   not_supported           → skip, warn

    // Collect not_supported items for warnings
    const notSupported = snapshot.evidence.filter(
      (item) => restorePolicyFor(item.kind) === "not_supported"
    );
    if (notSupported.length > 0) {
      warnings.push(
        `${notSupported.length} evidence item(s) have restorePolicy=not_supported ` +
        `and will not be included as content. ` +
        `First: ${notSupported[0].sourcePath} (kind: ${notSupported[0].kind})`
      );
    }

    // Only include file content for full_content_supported items with captured status
    const contentItems = snapshot.evidence.filter(
      (item) =>
        item.captureStatus === "captured" &&
        restorePolicyFor(item.kind) === "full_content_supported" &&
        item.sourcePath &&
        !item.sourcePath.startsWith("~/.env") // env keys are key_inventory_only anyway, but safety gate
    );

    // Deduplicate by sourcePath
    const seenPaths = new Set<string>();
    const uniqueItems: DiscoveredItem[] = [];
    for (const item of contentItems) {
      if (!seenPaths.has(item.sourcePath)) {
        seenPaths.add(item.sourcePath);
        uniqueItems.push(item);
      }
    }

    // Add content/ directory
    entries.push({ path: "content/", content: Buffer.alloc(0), mode: 0o755, mtime: Date.now(), type: "directory" });

    for (const item of uniqueItems) {
      // Resolve source path to absolute
      const sourceAbs = resolveSourcePath(item.sourcePath, homeDir, projectPath);
      try {
        const fileStat = await stat(sourceAbs);
        if (!fileStat.isFile()) continue;
        if (fileStat.size > MAX_CONTENT_BYTES) {
          warnings.push(`Skipped large file: ${item.sourcePath} (${fileStat.size} bytes > ${MAX_CONTENT_BYTES} limit)`);
          continue;
        }

        const content = await readFile(sourceAbs);
        const tarPath = `content/${item.sourcePath}`;
        entries.push({
          path: tarPath,
          content,
          mode: 0o644,
          mtime: fileStat.mtimeMs,
          type: "file"
        });
        totalContentBytes += content.length;
        contentCount++;
      } catch {
        // File may not exist or be unreadable; skip (best-effort content collection)
        continue;
      }
    }

    // Report structured_fields_only and key_inventory_only counts for transparency
    const structuredItems = snapshot.evidence.filter(
      (item) =>
        item.captureStatus === "captured" &&
        (restorePolicyFor(item.kind) === "structured_fields_only" ||
         restorePolicyFor(item.kind) === "key_inventory_only")
    );
    if (structuredItems.length > 0) {
      const kinds = Array.from(new Set(structuredItems.map((i) => i.kind))).join(", ");
      warnings.push(
        `${structuredItems.length} evidence item(s) (${kinds}) use structured/key-inventory capture. ` +
        `Data is in evidence.json, not included as separate content files.`
      );
    }

    // Update manifest with content stats
    manifest.contentFileCount = contentCount;
    manifest.contentTotalBytes = totalContentBytes;
    // Re-write manifest with updated content stats
    const manifestIndex = entries.findIndex((e) => e.path === ".stailor/manifest.json");
    if (manifestIndex >= 0) {
      entries[manifestIndex] = {
        ...entries[manifestIndex],
        content: Buffer.from(JSON.stringify(manifest, null, 2) + "\n", "utf-8")
      };
    }
  }

  // Compute per-entry checksums BEFORE writing the tar (avoids double write)
  const entryChecksums: Record<string, string> = {};
  for (const entry of entries) {
    const hash = createHash("sha256");
    hash.update(entry.content);
    entryChecksums[entry.path] = hash.digest("hex");
  }

  // Build .stailor/checksums.json entry
  const checksumsEntry: TarEntry = {
    path: ".stailor/checksums.json",
    content: Buffer.from(JSON.stringify({ algorithm: "SHA-256", entries: entryChecksums } as BundleChecksums, null, 2) + "\n", "utf-8"),
    mode: 0o644,
    mtime: Date.now(),
    type: "file"
  };

  // Insert checksums after manifest in the entries array
  const finalEntries: TarEntry[] = [];
  let checksumsInserted = false;
  for (const entry of entries) {
    finalEntries.push(entry);
    if (entry.path === ".stailor/manifest.json" && !checksumsInserted) {
      finalEntries.push(checksumsEntry);
      checksumsInserted = true;
    }
  }
  if (!checksumsInserted) {
    finalEntries.push(checksumsEntry);
  }

  // Single write: tar contains all entries including checksums
  const archiveChecksum = await writeTar(finalEntries, outputPath);

  return { bundlePath: outputPath, checksum: archiveChecksum, warnings };
}

// ── Import ──────────────────────────────────────────────────────

/**
 * Import a .stailor bundle into the local snapshot store.
 */
export async function bundleImport(options: BundleImportOptions): Promise<BundleImportResult> {
  const { bundlePath, storeDir, projectPath, homeDir, applyContent, dryRun, trust } = options;

  // Read bundle
  const { entries, checksum: bundleChecksum } = await readTar(bundlePath);

  // Validate format version
  const formatEntry = entries.find((e) => e.path === ".stailor/format-version");
  if (!formatEntry) {
    throw new Error("Invalid bundle: missing .stailor/format-version");
  }
  const formatVersion = formatEntry.content.toString("utf-8").trim();
  if (formatVersion !== FORMAT_VERSION) {
    throw new Error(`Unsupported bundle format version: "${formatVersion}" (expected "${FORMAT_VERSION}")`);
  }

  // Validate manifest
  const manifestEntry = entries.find((e) => e.path === ".stailor/manifest.json");
  if (!manifestEntry) {
    throw new Error("Invalid bundle: missing .stailor/manifest.json");
  }
  const manifest: BundleManifest = JSON.parse(manifestEntry.content.toString("utf-8"));

  // Validate checksums
  const checksumsEntry = entries.find((e) => e.path === ".stailor/checksums.json");
  if (checksumsEntry) {
    const checksums: BundleChecksums = JSON.parse(checksumsEntry.content.toString("utf-8"));
    for (const entry of entries) {
      if (entry.path === ".stailor/checksums.json") continue; // skip self
      const expected = checksums.entries[entry.path];
      if (expected) {
        const actual = createHash("sha256").update(entry.content).digest("hex");
        if (actual !== expected) {
          throw new Error(`Checksum mismatch for "${entry.path}": expected ${expected}, got ${actual}`);
        }
      }
    }
  }

  // Validate all paths are safe — validate against the REAL destination roots
  // (homeDir and projectPath), not a temporary quarantine root.
  // This ensures path validation matches actual content resolution.
  const allRoots = [homeDir, projectPath];
  for (const entry of entries) {
    // Basic path traversal check (.., null bytes, absolute paths)
    if (entry.path.includes("..")) {
      throw new Error(`Path traversal detected: "${entry.path}" contains ".."`);
    }
    if (entry.path.includes("\0")) {
      throw new Error(`Path traversal detected: "${entry.path}" contains null byte`);
    }
    if (path.isAbsolute(entry.path)) {
      throw new Error(`Path traversal detected: "${entry.path}" is absolute`);
    }

    // Validate against each root — content/ paths must resolve within at least one root
    if (entry.path.startsWith("content/")) {
      const relativePath = entry.path.slice("content/".length);
      const resolved = resolveSourcePath(relativePath, homeDir, projectPath);
      const isUnderRoot = allRoots.some(
        (root) => isStrictlyUnder(resolved, root)
      );
      if (!isUnderRoot) {
        throw new Error(
          `Content path "${relativePath}" resolves outside home and project directories`
        );
      }
    } else {
      // Non-content paths (.stailor/*, snapshot/*) — validate against any root
      const isUnderRoot = allRoots.some(
        (root) => isStrictlyUnder(path.resolve(root, entry.path), root)
      );
      if (!isUnderRoot) {
        throw new Error(`Entry path "${entry.path}" is not valid`);
      }
    }
  }

  // Validate bundle size
  const bundleStat = await stat(bundlePath);
  if (bundleStat.size > MAX_BUNDLE_BYTES) {
    throw new Error(`Bundle too large: ${bundleStat.size} bytes (max ${MAX_BUNDLE_BYTES})`);
  }

  // Validate content paths if applyContent (additional security: block dangerous home paths)
  if (applyContent) {
    const BLOCKED_HOME_PREFIXES = [
      ".ssh", ".aws", ".gnupg", ".config", ".local", ".npm",
      ".docker", ".kube", ".credentials", ".heroku", ".netrc",
      ".env", ".gitconfig", ".git-credentials", ".npmrc",
      ".bash_profile", ".bashrc", ".zshrc", ".profile",
      ".pgpass", ".gem"
    ];
    const contentEntries = entries.filter((e) => e.path.startsWith("content/"));
    for (const entry of contentEntries) {
      const relativePath = entry.path.slice("content/".length);

      // Home-relative paths (starting with ~/) are blocked entirely —
      // bundle content must be project-relative
      if (relativePath.startsWith("~/")) {
        throw new Error(
          `Home-relative content path "${relativePath}" is not allowed. ` +
          `Bundle content must be project-relative.`
        );
      }

      // Block known sensitive home directory paths
      for (const prefix of BLOCKED_HOME_PREFIXES) {
        if (relativePath.includes(`/${prefix}/`) || relativePath.startsWith(`${prefix}/`)) {
          throw new Error(`Blocked content path prefix: "${relativePath}"`);
        }
      }

      const resolved = resolveSourcePath(relativePath, homeDir, projectPath);
      // Verify resolved path is within home or project
      const homeResolved = path.resolve(homeDir);
      const projectResolved = path.resolve(projectPath);
      if (!resolved.startsWith(homeResolved) && !resolved.startsWith(projectResolved)) {
        throw new Error(`Content path "${relativePath}" resolves outside home and project directories`);
      }
    }
  }

  if (dryRun) {
    return {
      snapshotName: manifest.snapshotName,
      evidenceCount: entries.filter((e) => e.path.startsWith("snapshot/")).length,
      includesContent: manifest.includesContent,
      contentApplied: false,
      warnings: []
    };
  }

  // Read snapshot data from entries
  const evidenceEntry = entries.find((e) => e.path === "snapshot/evidence.json");
  const graphEntry = entries.find((e) => e.path === "snapshot/graph.json");
  const auditEntry = entries.find((e) => e.path === "snapshot/audit-findings.json");

  if (!evidenceEntry || !graphEntry || !auditEntry) {
    throw new Error("Invalid bundle: missing snapshot data files");
  }

  // Write snapshot to store using the existing store format
  const { writeSnapshot } = await import("./store.js");
  const snapshot = {
    manifest: {
      schemaVersion: "0.1" as const,
      name: manifest.snapshotName,
      createdAt: manifest.createdAt,
      projectPath: manifest.projectPath,
      security: {
        rawSecretsIncluded: false as const,
        redactionPolicy: "metadata-only" as const
      }
    },
    evidence: JSON.parse(evidenceEntry.content.toString("utf-8")),
    graph: JSON.parse(graphEntry.content.toString("utf-8")),
    auditFindings: JSON.parse(auditEntry.content.toString("utf-8")),
    provenance: []
  };

  await writeSnapshot(storeDir, snapshot);

  // Apply content files if requested
  let contentApplied = false;
  if (applyContent) {
    const contentEntries = entries.filter((e) => e.path.startsWith("content/") && e.type === "file");
    for (const entry of contentEntries) {
      const relativePath = entry.path.slice("content/".length);
      const resolved = resolveSourcePath(relativePath, homeDir, projectPath);
      await writeFile(resolved, entry.content);
    }
    contentApplied = true;
  }

  return {
    snapshotName: manifest.snapshotName,
    evidenceCount: snapshot.evidence.length,
    includesContent: manifest.includesContent,
    contentApplied,
    warnings: []
  };
}

// ── Inspect ─────────────────────────────────────────────────────

/**
 * Inspect a .stailor bundle and return metadata without unpacking.
 */
export async function bundleInspect(bundlePath: string): Promise<BundleInspectResult> {
  const { entries, checksum: bundleChecksum } = await readTar(bundlePath);

  // Read manifest
  const manifestEntry = entries.find((e) => e.path === ".stailor/manifest.json");
  if (!manifestEntry) {
    throw new Error("Invalid bundle: missing .stailor/manifest.json");
  }
  const manifest: BundleManifest = JSON.parse(manifestEntry.content.toString("utf-8"));

  // Read checksums
  const checksumsEntry = entries.find((e) => e.path === ".stailor/checksums.json");
  const checksums: BundleChecksums | null = checksumsEntry
    ? JSON.parse(checksumsEntry.content.toString("utf-8"))
    : null;

  return {
    bundlePath,
    formatVersion: manifest.formatVersion,
    snapshotName: manifest.snapshotName,
    createdAt: manifest.createdAt,
    projectPath: manifest.projectPath,
    includesContent: manifest.includesContent,
    contentFileCount: manifest.contentFileCount,
    contentTotalBytes: manifest.contentTotalBytes,
    checksumAlgorithm: checksums?.algorithm ?? "SHA-256",
    bundleChecksum,
    isSigned: manifest.security.signed,
    signatureAlgorithm: manifest.security.signatureAlgorithm
  };
}

// ── Helpers ─────────────────────────────────────────────────────

/**
 * Resolve a sourcePath (which may start with ~/) to an absolute path.
 */
function resolveSourcePath(sourcePath: string, homeDir: string, projectPath: string): string {
  if (sourcePath.startsWith("~/")) {
    return path.resolve(homeDir, sourcePath.slice(2));
  }
  return path.resolve(projectPath, sourcePath);
}

/**
 * Strict path containment check: verifies that `resolved` is either equal to
 * `root` or a direct descendant (separated by path.sep). Prevents sibling
 * directory prefix collisions like "/Users/alice-other" passing as "/Users/alice".
 */
function isStrictlyUnder(resolved: string, root: string): boolean {
  const normalized = path.resolve(root);
  return resolved === normalized || resolved.startsWith(normalized + path.sep);
}
