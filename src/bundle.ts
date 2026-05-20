/**
 * .stailor bundle export, import, and inspect.
 *
 * A .stailor bundle is a POSIX ustar tar archive containing:
 *   .stailor/format-version   — "1"
 *   .stailor/manifest.json    — BundleManifest (includes sourceMachine)
 *   .stailor/checksums.json   — BundleChecksums
 *   snapshot/evidence.json    — DiscoveredItem[]
 *   snapshot/graph.json       — GraphNode[]
 *   snapshot/audit-findings.json — AuditFinding[]
 *   snapshot/checksums.json   — ChecksumRecord
 *   snapshot/redactions.json  — redaction log
 *   content/...               — optional raw file contents (full_content_supported items only)
 *
 * Home paths are stored as {home}/... in the bundle for cross-machine compatibility.
 */

import { createHash, createHmac } from "node:crypto";
import { execSync } from "node:child_process";
import { readFile, stat, writeFile } from "node:fs/promises";
import { homedir, platform, hostname } from "node:os";
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
  GraphNode,
  MachineDiff,
  McpBinaryInfo,
  McpBinaryReport,
  ProvenanceEntry,
  SourceMachine,
  TarEntry
} from "./types.js";

const FORMAT_VERSION = "1";

/** Token used in bundle paths to represent the source home directory. */
const HOME_TOKEN = "{home}";

const MAX_BUNDLE_BYTES = parseInt(process.env.SNAPTAILOR_MAX_BUNDLE_BYTES ?? "", 10) || 500 * 1024 * 1024;
const MAX_CONTENT_BYTES = parseInt(process.env.SNAPTAILOR_MAX_CONTENT_BYTES ?? "", 10) || 50 * 1024 * 1024;
const SIGNATURE_ALGORITHM = "HMAC-SHA256";

function resolveSignatureKey(explicitKey?: string): string | undefined {
  return explicitKey ?? process.env.SNAPTAILOR_BUNDLE_KEY;
}

function cloneManifestWithoutSignature(manifest: BundleManifest): BundleManifest {
  return {
    ...manifest,
    security: {
      ...manifest.security,
      signature: undefined
    }
  };
}

function canonicalSignaturePayload(entries: TarEntry[], manifest: BundleManifest): Buffer {
  const hmacEntries = entries
    .filter((entry) => entry.path !== ".stailor/manifest.json" && entry.path !== ".stailor/checksums.json")
    .filter((entry) => entry.type === "file")
    .sort((a, b) => a.path.localeCompare(b.path));
  const chunks: Buffer[] = [
    Buffer.from(JSON.stringify(cloneManifestWithoutSignature(manifest)) + "\n", "utf-8")
  ];
  for (const entry of hmacEntries) {
    chunks.push(Buffer.from(`${entry.path}\n${entry.content.length}\n`, "utf-8"));
    chunks.push(entry.content);
    chunks.push(Buffer.from("\n", "utf-8"));
  }
  return Buffer.concat(chunks);
}

function signBundleEntries(entries: TarEntry[], manifest: BundleManifest, key: string): string {
  return createHmac("sha256", key).update(canonicalSignaturePayload(entries, manifest)).digest("hex");
}

function verifyBundleSignature(entries: TarEntry[], manifest: BundleManifest, key?: string): { ok: boolean; checked: boolean; warning?: string } {
  if (!manifest.security.signed) return { ok: true, checked: false };
  if (!key) return { ok: true, checked: false, warning: "Bundle is signed, but no signature key was provided; signature was not verified." };
  const expected = manifest.security.signature;
  if (!expected) return { ok: false, checked: true, warning: "Signed bundle manifest is missing security.signature." };
  const actual = signBundleEntries(entries, manifest, key);
  return { ok: actual === expected, checked: true, warning: actual === expected ? undefined : "Bundle signature verification failed." };
}

/** Result of a bundle export, including any warnings. */
export interface BundleExportResult {
  bundlePath: string;
  checksum: string;
  warnings: string[];
}

// ── Path normalisation ───────────────────────────────────────────

/**
 * Normalize a home directory prefix to the conventional root for a platform.
 * This keeps cross-OS reports stable when a manifest records a macOS-style
 * home for a Linux source (or vice versa), e.g. /Users/alice ↔ /home/alice.
 */
function normaliseHomeForPlatform(homeDir: string, machinePlatform: string): string {
  const macMatch = homeDir.match(/^\/Users\/([^/]+)(?:\/)?$/);
  const linuxMatch = homeDir.match(/^\/home\/([^/]+)(?:\/)?$/);

  if (machinePlatform === "linux" && macMatch) {
    return `/home/${macMatch[1]}`;
  }
  if (machinePlatform === "darwin" && linuxMatch) {
    return `/Users/${linuxMatch[1]}`;
  }
  return homeDir;
}

/**
 * Normalise a sourcePath for bundle storage.
 * Home-relative paths (starting with ~/) and absolute paths under the current
 * home directory become {home}/... for portability.
 * Project-relative paths are stored as-is.
 */
function normaliseSourcePath(sourcePath: string, homeDir: string): string {
  if (sourcePath.startsWith("~/")) {
    return `${HOME_TOKEN}/${sourcePath.slice(2)}`;
  }

  const resolvedSource = path.resolve(sourcePath);
  const resolvedHome = path.resolve(homeDir);
  if (path.isAbsolute(sourcePath) && (resolvedSource === resolvedHome || resolvedSource.startsWith(resolvedHome + path.sep))) {
    const homeRelative = path.relative(resolvedHome, resolvedSource);
    return homeRelative.length > 0 ? `${HOME_TOKEN}/${homeRelative}` : HOME_TOKEN;
  }

  return sourcePath;
}

/**
 * Resolve a normalised bundle path to an absolute path on the target machine.
 * {home} is replaced with the current home directory.
 */
function resolveBundlePath(normalisedPath: string, homeDir: string, projectPath: string): string {
  if (normalisedPath.startsWith(`${HOME_TOKEN}/`)) {
    return path.resolve(homeDir, normalisedPath.slice(HOME_TOKEN.length + 1));
  }
  return path.resolve(projectPath, normalisedPath);
}

function normaliseSnapshotPathsForBundle<T extends { sourcePath: string }>(items: T[], homeDir: string): T[] {
  return items.map((item) => ({
    ...item,
    sourcePath: normaliseSourcePath(item.sourcePath, homeDir)
  }));
}

function resolveSnapshotPathForImport(sourcePath: string, homeDir: string): string {
  if (sourcePath.startsWith(`${HOME_TOKEN}/`)) {
    return path.resolve(homeDir, sourcePath.slice(HOME_TOKEN.length + 1));
  }
  return sourcePath;
}

function resolveSnapshotPathsForImport<T extends { sourcePath: string }>(items: T[], homeDir: string): T[] {
  return items.map((item) => ({
    ...item,
    sourcePath: resolveSnapshotPathForImport(item.sourcePath, homeDir)
  }));
}

// ── MCP binary detection ─────────────────────────────────────────

function classifyMcpBinary(command: string, homeDir?: string): McpBinaryInfo["binaryKind"] {
  if (command === "npx" || command === "uvx") return "package_runner";
  if (path.isAbsolute(command)) {
    if (homeDir && isStrictlyUnder(command, homeDir)) return "source_local_path";
    return "path_binary";
  }
  return "command";
}

/**
 * Extract MCP binary information from evidence items.
 * Looks for items with kind "mcp_server" and extracts command/url from their value.
 */
function extractMcpBinaries(evidence: DiscoveredItem[], sourceHomeDir?: string): McpBinaryInfo[] {
  const binaries: McpBinaryInfo[] = [];
  for (const item of evidence) {
    if (item.kind !== "mcp_server") continue;
    const value = item.value as Record<string, unknown> | undefined;
    if (!value || typeof value !== "object") continue;

    const command = typeof value.command === "string" ? value.command : undefined;
    const url = typeof value.url === "string" ? value.url : undefined;

    if (command || url) {
      const args = Array.isArray(value.args) ? value.args.filter((a): a is string => typeof a === "string") : undefined;
      binaries.push({
        evidenceId: item.id,
        command: command ?? url ?? "unknown",
        args,
        url,
        binaryKind: url ? "remote_url" : classifyMcpBinary(command ?? "", sourceHomeDir)
      });
    }
  }
  return binaries;
}

/**
 * Check which MCP binaries are available on the current machine.
 */
function checkMcpBinaryAvailability(sourceBinaries: McpBinaryInfo[]): McpBinaryReport[] {
  const reports: McpBinaryReport[] = [];

  for (const bin of sourceBinaries) {
    // URLs are not "binaries" — they're remote endpoints
    if (bin.url) {
      reports.push({
        evidenceId: bin.evidenceId,
        command: bin.url,
        availableOnTarget: true, // URLs can't be checked — assume reachable
        binaryKind: "remote_url",
        warning: "Remote URL — availability cannot be verified locally"
      });
      continue;
    }

    if (bin.binaryKind === "source_local_path") {
      reports.push({
        evidenceId: bin.evidenceId,
        command: bin.command,
        availableOnTarget: false,
        binaryKind: bin.binaryKind,
        warning: `MCP command points to a source machine local binary path (${bin.command}); install or remap it on this machine.`
      });
      continue;
    }

    // "npx" and "uvx" are package runners — check if they exist
    try {
      const resolved = execSync(`which "${bin.command}" 2>/dev/null`, { encoding: "utf-8", timeout: 2000 }).trim();
      reports.push({
        evidenceId: bin.evidenceId,
        command: bin.command,
        availableOnTarget: resolved.length > 0,
        binaryKind: bin.binaryKind,
        resolvedPath: resolved || undefined,
        warning: bin.binaryKind === "package_runner"
          ? `Package runner ${bin.command} is available at ${resolved}; package arguments may still differ on this machine.`
          : (resolved.length === 0 ? `Binary "${bin.command}" not found on this machine` : undefined)
      });
    } catch {
      reports.push({
        evidenceId: bin.evidenceId,
        command: bin.command,
        availableOnTarget: false,
        binaryKind: bin.binaryKind,
        warning: bin.binaryKind === "package_runner"
          ? `Package runner ${bin.command} not found on this machine; MCP package cannot be launched.`
          : `Binary "${bin.command}" not found on this machine`
      });
    }
  }

  return reports;
}

// ── Machine info ─────────────────────────────────────────────────

function captureSourceMachine(): SourceMachine {
  return {
    homeDir: homedir(),
    platform: platform(),
    hostname: hostname()
  };
}

// ── Export ──────────────────────────────────────────────────────

/**
 * Export a snapshot to a .stailor bundle file.
 *
 * Content inclusion respects per-kind restore policies.
 * Home paths are normalised to {home}/... for cross-machine portability.
 */
export async function bundleExport(options: BundleExportOptions): Promise<BundleExportResult> {
  const { snapshotName, outputPath, storeDir, projectPath, homeDir, includeContent } = options;
  const signatureKey = resolveSignatureKey(options.signatureKey);

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

  // Validate no unsupported restore data is silently dropped from content bundles.
  if (includeContent) {
    const unsupportedRestoreItems = snapshot.evidence.filter(
      (item) => item.captureStatus === "captured" && restorePolicyFor(item.kind) === "not_supported"
    );
    if (unsupportedRestoreItems.length > 0) {
      throw new Error(
        `Cannot export content bundle: ${unsupportedRestoreItems.length} not_supported evidence item(s) would lose restore data. ` +
        `First: ${unsupportedRestoreItems[0].sourcePath} (kind: ${unsupportedRestoreItems[0].kind}). ` +
        `Use --metadata-only or remove unsupported items before exporting a restorable content bundle.`
      );
    }
  }

  const warnings: string[] = [];

  // Capture source machine info for cross-machine compatibility
  const sourceMachine = captureSourceMachine();

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

  // Build manifest
  const manifest: BundleManifest = {
    formatVersion: 1,
    snapshotName,
    createdAt: snapshot.manifest.createdAt,
    projectPath,
    includesContent: includeContent ?? false,
    contentFileCount: 0,
    contentTotalBytes: 0,
    sourceMachine,
    security: {
      rawSecretsIncluded: false,
      redactionPolicy: "metadata-only",
      signed: Boolean(signatureKey),
      signatureAlgorithm: signatureKey ? SIGNATURE_ALGORITHM : undefined
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
  const bundledEvidence = normaliseSnapshotPathsForBundle(snapshot.evidence, homeDir);
  const bundledGraph = normaliseSnapshotPathsForBundle(snapshot.graph, homeDir);
  const bundledProvenance = normaliseSnapshotPathsForBundle(snapshot.provenance, homeDir);
  const snapshotFiles: Array<{ name: string; data: unknown }> = [
    { name: "evidence.json", data: bundledEvidence },
    { name: "graph.json", data: bundledGraph },
    { name: "audit-findings.json", data: snapshot.auditFindings },
    { name: "provenance.json", data: bundledProvenance },
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

  // Optional content files — per-kind restore policy filtering with path normalisation
  if (includeContent) {
    let totalContentBytes = 0;
    let contentCount = 0;

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
        !item.sourcePath.startsWith("~/.env")
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
      const sourceAbs = resolveSourcePath(item.sourcePath, homeDir, projectPath);
      try {
        const fileStat = await stat(sourceAbs);
        if (!fileStat.isFile()) continue;
        if (fileStat.size > MAX_CONTENT_BYTES) {
          warnings.push(`Skipped large file: ${item.sourcePath} (${fileStat.size} bytes > ${MAX_CONTENT_BYTES} limit)`);
          continue;
        }

        const content = await readFile(sourceAbs);
        // Normalise home paths to {home}/... for cross-machine portability
        const normalisedPath = normaliseSourcePath(item.sourcePath, homeDir);
        const tarPath = `content/${normalisedPath}`;
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
        continue;
      }
    }

    // Report structured_fields_only and key_inventory_only counts
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
    const manifestIndex = entries.findIndex((e) => e.path === ".stailor/manifest.json");
    if (manifestIndex >= 0) {
      entries[manifestIndex] = {
        ...entries[manifestIndex],
        content: Buffer.from(JSON.stringify(manifest, null, 2) + "\n", "utf-8")
      };
    }
  }

  // Compute and persist signature after content stats are final, but before checksums.
  if (signatureKey) {
    manifest.security.signature = signBundleEntries(entries, manifest, signatureKey);
    const manifestIndex = entries.findIndex((e) => e.path === ".stailor/manifest.json");
    if (manifestIndex >= 0) {
      entries[manifestIndex] = {
        ...entries[manifestIndex],
        content: Buffer.from(JSON.stringify(manifest, null, 2) + "\n", "utf-8")
      };
    }
  }

  // Compute per-entry checksums
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

  // Insert checksums after manifest
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

  const archiveChecksum = await writeTar(finalEntries, outputPath);

  return { bundlePath: outputPath, checksum: archiveChecksum, warnings };
}

// ── Import ──────────────────────────────────────────────────────

/**
 * Import a .stailor bundle into the local snapshot store.
 *
 * On import, cross-machine compatibility is assessed:
 *   - Home paths ({home}/...) are remapped to the current machine's $HOME
 *   - MCP binary availability is checked and reported
 *   - OS differences are noted
 */
export async function bundleImport(options: BundleImportOptions): Promise<BundleImportResult> {
  const { bundlePath, storeDir, projectPath, homeDir, applyContent, dryRun } = options;
  const signatureKey = resolveSignatureKey(options.signatureKey);

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

  const signatureVerification = verifyBundleSignature(entries, manifest, signatureKey);
  if (!signatureVerification.ok) {
    throw new Error(signatureVerification.warning ?? "Bundle signature verification failed.");
  }

  // Validate checksums
  const checksumsEntry = entries.find((e) => e.path === ".stailor/checksums.json");
  if (checksumsEntry) {
    const checksums: BundleChecksums = JSON.parse(checksumsEntry.content.toString("utf-8"));
    for (const entry of entries) {
      if (entry.path === ".stailor/checksums.json") continue;
      const expected = checksums.entries[entry.path];
      if (expected) {
        const actual = createHash("sha256").update(entry.content).digest("hex");
        if (actual !== expected) {
          throw new Error(`Checksum mismatch for "${entry.path}": expected ${expected}, got ${actual}`);
        }
      }
    }
  }

  // Validate all paths are safe
  const allRoots = [homeDir, projectPath];
  for (const entry of entries) {
    if (entry.path.includes("..")) {
      throw new Error(`Path traversal detected: "${entry.path}" contains ".."`);
    }
    if (entry.path.includes("\0")) {
      throw new Error(`Path traversal detected: "${entry.path}" contains null byte`);
    }
    if (path.isAbsolute(entry.path)) {
      throw new Error(`Path traversal detected: "${entry.path}" is absolute`);
    }

    if (entry.path.startsWith("content/")) {
      const relativePath = entry.path.slice("content/".length);
      // Resolve with {home} handling
      const resolved = resolveBundlePath(relativePath, homeDir, projectPath);
      const isUnderRoot = allRoots.some((root) => isStrictlyUnder(resolved, root));
      if (!isUnderRoot) {
        throw new Error(
          `Content path "${relativePath}" resolves outside home and project directories`
        );
      }
    } else {
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

  // Validate content paths if applyContent
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

      // Home-relative paths (either legacy ~/ or {home}/ format)
      const isHomeRelative =
        relativePath.startsWith("~/") ||
        relativePath.startsWith(`${HOME_TOKEN}/`);
      if (isHomeRelative) {
        throw new Error(
          `Home-relative content path "${relativePath}" is not allowed. ` +
          `Bundle content must be project-relative.`
        );
      }

      // Block known sensitive directory paths (regardless of home-relative)
      for (const prefix of BLOCKED_HOME_PREFIXES) {
        if (relativePath.includes(`/${prefix}/`) || relativePath.startsWith(`${prefix}/`)) {
          throw new Error(`Blocked content path prefix: "${relativePath}"`);
        }
      }

      const resolved = resolveBundlePath(relativePath, homeDir, projectPath);
      const homeResolved = path.resolve(homeDir);
      const projectResolved = path.resolve(projectPath);
      if (!resolved.startsWith(homeResolved) && !resolved.startsWith(projectResolved)) {
        throw new Error(`Content path "${relativePath}" resolves outside home and project directories`);
      }
    }
  }

  // ── Cross-machine compatibility assessment ──────────────────

  const sourceMachine = manifest.sourceMachine;
  const targetHome = homedir();
  const targetPlatform = platform();
  const targetHostname = hostname();

  // Build remapped paths list
  const remappedPaths: string[] = [];
  const contentEntries = entries.filter((e) => e.path.startsWith("content/") && e.type === "file");
  const sourceHome = sourceMachine ? normaliseHomeForPlatform(sourceMachine.homeDir, sourceMachine.platform) : undefined;
  const normalisedTargetHome = normaliseHomeForPlatform(targetHome, targetPlatform);
  for (const entry of contentEntries) {
    const relativePath = entry.path.slice("content/".length);
    if (relativePath.startsWith(`${HOME_TOKEN}/`)) {
      const homeRelative = relativePath.slice(HOME_TOKEN.length + 1);
      const sourceAbs = sourceHome ? path.join(sourceHome, homeRelative) : `source:${homeRelative}`;
      const targetAbs = path.join(normalisedTargetHome, homeRelative);
      remappedPaths.push(`${sourceAbs} → ${targetAbs}`);
    }
  }

  // Detect OS-level differences
  const crossOS = sourceMachine ? sourceMachine.platform !== targetPlatform : false;
  const osDifferences: string[] = [];
  if (crossOS && sourceMachine) {
    osDifferences.push(`${sourceMachine.platform} → ${targetPlatform} (cross-OS restore)`);
    if (sourceMachine.platform === "darwin" && targetPlatform !== "darwin") {
      osDifferences.push("macOS extended attributes and ACLs not preserved on non-macOS");
    }
    if (sourceMachine.platform === "darwin") {
      osDifferences.push("macOS uses case-insensitive FS by default; Linux is case-sensitive — filename conflicts possible");
    }
    if (targetPlatform === "win32") {
      osDifferences.push("Windows uses \\ path separator — paths will be normalized");
    }
    osDifferences.push("Binary/script files copied as-is; manual line-ending normalization may be needed");
  }

  // Extract MCP binaries from evidence and check availability
  let sourceMcpBinaries: McpBinaryInfo[] = [];
  let mcpBinaryReport: McpBinaryReport[] = [];
  const evidenceEntry = entries.find((e) => e.path === "snapshot/evidence.json");
  if (evidenceEntry) {
    const evidence: DiscoveredItem[] = JSON.parse(evidenceEntry.content.toString("utf-8"));
    sourceMcpBinaries = extractMcpBinaries(evidence, sourceHome);
    mcpBinaryReport = checkMcpBinaryAvailability(sourceMcpBinaries);
  }

  const machineDiff: MachineDiff = {
    sourceHome: sourceMachine?.homeDir ?? "unknown",
    targetHome,
    sourcePlatform: sourceMachine?.platform ?? "unknown",
    targetPlatform,
    sourceHostname: sourceMachine?.hostname ?? "unknown",
    targetHostname,
    crossOS,
    osDifferences,
    remappedPaths,
    sourceMcpBinaries,
    mcpBinaryReport
  };

  // Collect machine-related warnings
  const warnings: string[] = [];
  if (signatureVerification.warning) {
    warnings.push(signatureVerification.warning);
  }

  if (sourceMachine && sourceMachine.homeDir !== targetHome) {
    warnings.push(
      `Home directory differs: source=${sourceMachine.homeDir}, target=${targetHome}. ` +
      `${remappedPaths.length} path(s) will be remapped.`
    );
  }

  if (sourceMachine && sourceMachine.platform !== targetPlatform) {
    warnings.push(
      `Platform differs: source=${sourceMachine.platform}, target=${targetPlatform}. ` +
      `Cross-OS restore may have issues with binary paths and file permissions.`
    );
  }

  const unavailableBinaries = mcpBinaryReport.filter((b) => !b.availableOnTarget);
  if (unavailableBinaries.length > 0) {
    warnings.push(
      `${unavailableBinaries.length} MCP binary/bundles not found on this machine: ` +
      unavailableBinaries.map((b) => b.command).join(", ")
    );
  }

  if (dryRun) {
    return {
      snapshotName: manifest.snapshotName,
      evidenceCount: entries.filter((e) => e.path.startsWith("snapshot/")).length,
      includesContent: manifest.includesContent,
      contentApplied: false,
      warnings,
      machineDiff
    };
  }

  // Read snapshot data from entries
  const graphEntry = entries.find((e) => e.path === "snapshot/graph.json");
  const auditEntry = entries.find((e) => e.path === "snapshot/audit-findings.json");
  const provenanceEntry = entries.find((e) => e.path === "snapshot/provenance.json");

  if (!evidenceEntry || !graphEntry || !auditEntry) {
    throw new Error("Invalid bundle: missing snapshot data files");
  }

  const importedEvidence = resolveSnapshotPathsForImport(
    JSON.parse(evidenceEntry.content.toString("utf-8")) as DiscoveredItem[],
    homeDir
  );
  const importedGraph = resolveSnapshotPathsForImport(
    JSON.parse(graphEntry.content.toString("utf-8")) as GraphNode[],
    homeDir
  );
  const importedProvenance = provenanceEntry
    ? resolveSnapshotPathsForImport(JSON.parse(provenanceEntry.content.toString("utf-8")) as ProvenanceEntry[], homeDir)
    : [];

  // Write snapshot to store
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
    evidence: importedEvidence,
    graph: importedGraph,
    auditFindings: JSON.parse(auditEntry.content.toString("utf-8")),
    provenance: importedProvenance
  };

  await writeSnapshot(storeDir, snapshot);

  // Apply content files with {home} resolution
  let contentApplied = false;
  if (applyContent) {
    const applyEntries = entries.filter((e) => e.path.startsWith("content/") && e.type === "file");
    for (const entry of applyEntries) {
      const relativePath = entry.path.slice("content/".length);
      const resolved = resolveBundlePath(relativePath, homeDir, projectPath);
      await writeFile(resolved, entry.content);
    }
    contentApplied = true;
  }

  return {
    snapshotName: manifest.snapshotName,
    evidenceCount: snapshot.evidence.length,
    includesContent: manifest.includesContent,
    contentApplied,
    warnings,
    machineDiff
  };
}

// ── Inspect ─────────────────────────────────────────────────────

/**
 * Inspect a .stailor bundle and return metadata without unpacking.
 */
export async function bundleInspect(bundlePath: string): Promise<BundleInspectResult> {
  const { entries, checksum: bundleChecksum } = await readTar(bundlePath);

  const manifestEntry = entries.find((e) => e.path === ".stailor/manifest.json");
  if (!manifestEntry) {
    throw new Error("Invalid bundle: missing .stailor/manifest.json");
  }
  const manifest: BundleManifest = JSON.parse(manifestEntry.content.toString("utf-8"));

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
    signatureAlgorithm: manifest.security.signatureAlgorithm,
    sourceMachine: manifest.sourceMachine
  };
}

// ── Helpers ─────────────────────────────────────────────────────

/**
 * Resolve a raw sourcePath (which may start with ~/) to an absolute path.
 * Used during EXPORT only — imports use resolveBundlePath().
 */
function resolveSourcePath(sourcePath: string, homeDir: string, projectPath: string): string {
  if (sourcePath.startsWith("~/")) {
    return path.resolve(homeDir, sourcePath.slice(2));
  }
  if (path.isAbsolute(sourcePath)) {
    return path.resolve(sourcePath);
  }
  return path.resolve(projectPath, sourcePath);
}

/**
 * Strict path containment check.
 */
function isStrictlyUnder(resolved: string, root: string): boolean {
  const normalized = path.resolve(root);
  return resolved === normalized || resolved.startsWith(normalized + path.sep);
}
