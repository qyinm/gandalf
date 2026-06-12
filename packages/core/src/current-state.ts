import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";

import { auditEvidence } from "./audit.js";
import { buildGraph } from "./graph.js";
import { buildProvenance } from "./provenance.js";
import { scanProject, type ScanResult } from "./scan.js";
import { ensureStore } from "./store.js";
import type { AuditFinding, DiscoveredItem, Snapshot, SnapshotContentEntry, SnapshotManifest } from "./types.js";
import type { RuntimeOptions } from "./runtime-options.js";

export interface CurrentState {
  scan: ScanResult;
  snapshot: Snapshot;
  storeFindings: AuditFinding[];
}

export async function captureCurrentState(
  options: RuntimeOptions,
  name = "current"
): Promise<CurrentState> {
  const storeFindings = await ensureStore(options.storeDir);
  const baseScan = await scanProject(options);
  const contentCapture = options.captureContent
    ? await captureContentBackedEvidence(baseScan.evidence, options)
    : { evidence: baseScan.evidence, content: [] };
  const scan: ScanResult = {
    ...baseScan,
    evidence: contentCapture.evidence
  };
  const graph = buildGraph(scan.evidence);
  const auditFindings = [...storeFindings, ...auditEvidence(scan.evidence, graph)];
  const provenance = buildProvenance(graph, scan.evidence);
  const manifest: SnapshotManifest = {
    schemaVersion: "0.1",
    name,
    createdAt: new Date().toISOString(),
    projectPath: options.projectPath,
    security: {
      rawSecretsIncluded: false,
      redactionPolicy: options.captureContent ? "content-backed" : "metadata-only"
    }
  };

  return {
    scan,
    storeFindings,
    snapshot: {
      manifest,
      evidence: scan.evidence,
      graph,
      auditFindings,
      provenance,
      ...(contentCapture.content.length > 0 ? { content: contentCapture.content } : {})
    }
  };
}

async function captureContentBackedEvidence(
  evidence: DiscoveredItem[],
  options: RuntimeOptions
): Promise<{ evidence: DiscoveredItem[]; content: SnapshotContentEntry[] }> {
  const content: SnapshotContentEntry[] = [];
  const byEvidenceId = new Map<string, SnapshotContentEntry>();

  for (const item of evidence) {
    const restorePath = restorePathForContent(item);
    if (!restorePath || !isCodexUserGlobalContentCandidate(item)) {
      continue;
    }

    const absolutePath = absolutePathForSourcePath(restorePath, options);
    if (!absolutePath) {
      continue;
    }

    let text: string;
    try {
      text = await readFile(absolutePath, "utf8");
    } catch {
      continue;
    }

    const checksum = `sha256:${createHash("sha256").update(text).digest("hex")}`;
    const storagePath = `content/${safeContentFileName(item.id, checksum)}.txt`;

    const entry: SnapshotContentEntry = containsSecretLikeAssignment(text)
      ? {
          evidenceId: item.id,
          sourcePath: item.sourcePath,
          restorePath,
          checksum,
          byteLength: Buffer.byteLength(text, "utf8"),
          encoding: "utf8",
          storagePath,
          captureStatus: "omitted",
          reason: "secret_like_assignment"
        }
      : {
          evidenceId: item.id,
          sourcePath: item.sourcePath,
          restorePath,
          checksum,
          byteLength: Buffer.byteLength(text, "utf8"),
          encoding: "utf8",
          storagePath,
          captureStatus: "captured",
          content: text
        };

    content.push(entry);
    byEvidenceId.set(item.id, entry);
  }

  const updatedEvidence = evidence.map((item) => {
    const entry = byEvidenceId.get(item.id);
    if (!entry) {
      return item;
    }
    return {
      ...item,
      checksum: entry.checksum,
      metadata: {
        ...(item.metadata ?? {}),
        contentCaptureStatus: entry.captureStatus,
        contentRestorePath: entry.restorePath,
        ...(entry.reason ? { contentCaptureReason: entry.reason } : {})
      }
    } satisfies DiscoveredItem;
  });

  return { evidence: updatedEvidence, content };
}

function isCodexUserGlobalContentCandidate(item: DiscoveredItem): boolean {
  return item.agent === "codex" &&
    item.scope === "user" &&
    item.captureStatus === "captured" &&
    item.sourcePath.startsWith("~/.codex/") &&
    (item.kind === "agent_config" || item.kind === "skill" || item.kind === "hook");
}

function restorePathForContent(item: DiscoveredItem): string | undefined {
  if (item.kind === "skill") {
    const entrypoint = typeof item.metadata?.entrypoint === "string" ? item.metadata.entrypoint : "SKILL.md";
    return `${item.sourcePath}/${entrypoint}`;
  }
  return item.sourcePath;
}

function absolutePathForSourcePath(sourcePath: string, options: RuntimeOptions): string | undefined {
  if (sourcePath === "~") {
    return options.homeDir;
  }
  if (sourcePath.startsWith("~/")) {
    return path.join(options.homeDir, sourcePath.slice(2));
  }
  if (path.isAbsolute(sourcePath)) {
    return sourcePath;
  }
  if (/^[a-z_]+:/i.test(sourcePath)) {
    return undefined;
  }
  return path.resolve(options.projectPath, sourcePath);
}

function safeContentFileName(evidenceId: string, checksum: string): string {
  return `${evidenceId}-${checksum.slice("sha256:".length, "sha256:".length + 12)}`
    .replace(/[^A-Za-z0-9_.-]+/g, ".")
    .replace(/^\.+|\.+$/g, "")
    .toLowerCase();
}

function containsSecretLikeAssignment(text: string): boolean {
  return /(?:api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)\s*=/i.test(text);
}
