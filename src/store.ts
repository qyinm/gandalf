import { randomUUID } from "node:crypto";
import { constants } from "node:fs";
import {
  access,
  chmod,
  mkdir,
  readdir,
  readFile,
  rename,
  rm,
  stat,
  writeFile
} from "node:fs/promises";
import path from "node:path";
import type { AuditFinding, DiscoveredItem, Snapshot } from "./types.js";

const SNAPSHOT_FILES = {
  manifest: "manifest.json",
  evidence: "evidence.json",
  graph: "graph.json",
  auditFindings: "audit-findings.json",
  provenance: "provenance.json",
  checksums: "checksums.json",
  redactions: "redactions.json"
} as const;

type ChecksumRecord = Record<string, { sourcePath: string; checksum: string }>;

type StoreSnapshot = Snapshot & {
  checksums?: ChecksumRecord;
  redactions?: unknown[];
};

export function defaultStoreDir(homeDir: string): string {
  return path.join(homeDir, ".snaptailor");
}

export async function ensureStore(storeDir: string): Promise<AuditFinding[]> {
  const existed = await pathExists(storeDir);

  await mkdir(storeDir, { recursive: true, mode: 0o700 });
  if (!existed) {
    await chmod(storeDir, 0o700);
  }

  const mode = (await stat(storeDir)).mode & 0o777;
  if ((mode & 0o022) === 0) {
    return [];
  }

  return [
    {
      code: "WORLD_WRITABLE_STORE",
      severity: "high",
      problem: "The local snaptailor snapshot store is writable by group or world.",
      cause: `Store permissions are ${mode.toString(8)}.`,
      fix: "Restrict the store directory to the current user with chmod 700.",
      path: storeDir
    }
  ];
}

export async function writeSnapshot(storeDir: string, snapshot: StoreSnapshot): Promise<void> {
  const name = validateSnapshotName(snapshot.manifest.name);
  const snapshotDir = path.join(storeDir, name);

  await ensureStore(storeDir);
  await mkdir(snapshotDir, { recursive: true, mode: 0o700 });
  await chmod(snapshotDir, 0o700);

  await Promise.all([
    writeJsonAtomic(path.join(snapshotDir, SNAPSHOT_FILES.manifest), snapshot.manifest),
    writeJsonAtomic(path.join(snapshotDir, SNAPSHOT_FILES.evidence), snapshot.evidence),
    writeJsonAtomic(path.join(snapshotDir, SNAPSHOT_FILES.graph), snapshot.graph),
    writeJsonAtomic(path.join(snapshotDir, SNAPSHOT_FILES.auditFindings), snapshot.auditFindings),
    writeJsonAtomic(path.join(snapshotDir, SNAPSHOT_FILES.provenance), snapshot.provenance),
    writeJsonAtomic(path.join(snapshotDir, SNAPSHOT_FILES.checksums), snapshot.checksums ?? checksumsFromEvidence(snapshot.evidence)),
    writeJsonAtomic(path.join(snapshotDir, SNAPSHOT_FILES.redactions), snapshot.redactions ?? [])
  ]);
}

export async function readSnapshot(storeDir: string, name: string): Promise<Snapshot> {
  const safeName = validateSnapshotName(name);
  const snapshotDir = path.join(storeDir, safeName);

  const [manifest, evidence, graph, auditFindings, provenance] = await Promise.all([
    readJson<Snapshot["manifest"]>(path.join(snapshotDir, SNAPSHOT_FILES.manifest)),
    readJson<Snapshot["evidence"]>(path.join(snapshotDir, SNAPSHOT_FILES.evidence)),
    readJson<Snapshot["graph"]>(path.join(snapshotDir, SNAPSHOT_FILES.graph)),
    readJson<Snapshot["auditFindings"]>(path.join(snapshotDir, SNAPSHOT_FILES.auditFindings)),
    readJson<Snapshot["provenance"]>(path.join(snapshotDir, SNAPSHOT_FILES.provenance))
  ]);

  return {
    manifest,
    evidence,
    graph,
    auditFindings,
    provenance
  };
}

export async function listSnapshots(storeDir: string): Promise<string[]> {
  if (!(await pathExists(storeDir))) {
    return [];
  }

  const entries = await readdir(storeDir, { withFileTypes: true });
  return entries
    .filter((entry) => entry.isDirectory() && isSafeSnapshotName(entry.name))
    .map((entry) => entry.name)
    .sort();
}

export async function snapshotExists(storeDir: string, name: string): Promise<boolean> {
  const safeName = validateSnapshotName(name);

  try {
    await access(path.join(storeDir, safeName, SNAPSHOT_FILES.manifest), constants.R_OK);
    return true;
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code === "ENOENT" || code === "ENOTDIR") {
      return false;
    }
    throw error;
  }
}

function validateSnapshotName(name: string): string {
  if (!isSafeSnapshotName(name)) {
    throw new Error(`Unsafe snapshot name: ${JSON.stringify(name)}`);
  }
  return name;
}

function isSafeSnapshotName(name: string): boolean {
  return name.trim().length > 0 && !name.includes("..") && !name.includes("/") && !name.includes("\\");
}

async function writeJsonAtomic(filePath: string, value: unknown): Promise<void> {
  const tempPath = `${filePath}.${process.pid}.${randomUUID()}.tmp`;
  try {
    await writeFile(tempPath, `${JSON.stringify(value, null, 2)}\n`, { mode: 0o600 });
    await rename(tempPath, filePath);
  } catch (error) {
    await rm(tempPath, { force: true });
    throw error;
  }
}

async function readJson<T>(filePath: string): Promise<T> {
  return JSON.parse(await readFile(filePath, "utf8")) as T;
}

function checksumsFromEvidence(evidence: DiscoveredItem[]): ChecksumRecord {
  const checksums: ChecksumRecord = {};

  for (const item of evidence) {
    if (typeof item.checksum === "string" && item.checksum.length > 0) {
      checksums[item.id] = {
        sourcePath: item.sourcePath,
        checksum: item.checksum
      };
    }
  }

  return checksums;
}

async function pathExists(targetPath: string): Promise<boolean> {
  try {
    await access(targetPath, constants.F_OK);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return false;
    }
    throw error;
  }
}
