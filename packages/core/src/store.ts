import { createHash, randomUUID } from "node:crypto";
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
import type { AgentId, AuditFinding, DiscoveredItem, Snapshot, SnapshotContentEntry, TimelineEntry } from "./types.js";

const SNAPSHOT_FILES = {
  manifest: "manifest.json",
  evidence: "evidence.json",
  graph: "graph.json",
  auditFindings: "audit-findings.json",
  provenance: "provenance.json",
  checksums: "checksums.json",
  redactions: "redactions.json",
  contentIndex: "content-index.json"
} as const;

const CONTENT_DIR = "content";
const TIMELINE_EVENTS_DIR = path.join("timeline", "events");
const AGENT_STORE_DIRS: AgentId[] = ["claude-code", "codex", "cursor", "opencode", "pi-agent", "project", "unknown"];

type ChecksumRecord = Record<string, { sourcePath: string; checksum: string }>;

type StoreSnapshot = Snapshot & {
  checksums?: ChecksumRecord;
  redactions?: unknown[];
};

export function defaultStoreDir(homeDir: string): string {
  return path.join(homeDir, ".hem");
}

/**
 * Return the per-agent subdirectory within the store.
 * When agent is omitted, returns the store root (backward compatible).
 */
export function agentStoreDir(storeDir: string, agent?: AgentId): string {
  return agent ? path.join(storeDir, agent) : storeDir;
}

/**
 * Return the full snapshot directory for a given agent-scoped snapshot.
 */
function snapshotDir(storeDir: string, name: string, agent?: AgentId): string {
  return path.join(agentStoreDir(storeDir, agent), name);
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
      problem: "The local hem snapshot store is writable by group or world.",
      cause: `Store permissions are ${mode.toString(8)}.`,
      fix: "Restrict the store directory to the current user with chmod 700.",
      path: storeDir
    }
  ];
}

/**
 * List the agent subdirectories in the store.
 * Returns agent IDs that have at least one snapshot.
 */
export async function listAgents(storeDir: string): Promise<AgentId[]> {
  if (!(await pathExists(storeDir))) {
    return [];
  }

  const entries = await readdir(storeDir, { withFileTypes: true });
  const agents: AgentId[] = [];
  for (const entry of entries) {
    if (entry.isDirectory() && isSafeAgentName(entry.name) && AGENT_STORE_DIRS.includes(entry.name as AgentId)) {
      // Verify the directory actually contains snapshots (has subdirectories)
      const sub = await readdir(path.join(storeDir, entry.name), { withFileTypes: true });
      const hasSnapshots = sub.some((s) => s.isDirectory());
      if (hasSnapshots) {
        agents.push(entry.name as AgentId);
      }
    }
  }
  return agents.sort();
}

function isSafeAgentName(name: string): boolean {
  return /^[a-z][a-z0-9_-]*$/.test(name) && !name.includes("..") && !name.includes("/");
}

export async function writeSnapshot(
  storeDir: string,
  snapshot: StoreSnapshot,
  agent?: AgentId
): Promise<void> {
  const name = validateSnapshotName(snapshot.manifest.name);
  const dir = snapshotDir(storeDir, name, agent);

  await ensureStore(storeDir);
  if (agent) {
    await ensureStore(agentStoreDir(storeDir, agent));
  }
  await mkdir(dir, { recursive: true, mode: 0o700 });
  await chmod(dir, 0o700);

  await Promise.all([
    writeJsonAtomic(path.join(dir, SNAPSHOT_FILES.manifest), snapshot.manifest),
    writeJsonAtomic(path.join(dir, SNAPSHOT_FILES.evidence), snapshot.evidence),
    writeJsonAtomic(path.join(dir, SNAPSHOT_FILES.graph), snapshot.graph),
    writeJsonAtomic(path.join(dir, SNAPSHOT_FILES.auditFindings), snapshot.auditFindings),
    writeJsonAtomic(path.join(dir, SNAPSHOT_FILES.provenance), snapshot.provenance),
    writeJsonAtomic(path.join(dir, SNAPSHOT_FILES.checksums), snapshot.checksums ?? checksumsFromEvidence(snapshot.evidence)),
    writeJsonAtomic(path.join(dir, SNAPSHOT_FILES.redactions), snapshot.redactions ?? [])
  ]);

  if (snapshot.content && snapshot.content.length > 0) {
    const contentDir = path.join(dir, CONTENT_DIR);
    await rm(contentDir, { recursive: true, force: true });
    await mkdir(contentDir, { recursive: true, mode: 0o700 });
    await chmod(contentDir, 0o700);

    for (const entry of snapshot.content) {
      if (entry.captureStatus !== "captured" || typeof entry.content !== "string") {
        continue;
      }
      if (!isSafeSnapshotRelativePath(entry.storagePath) || !entry.storagePath.startsWith(`${CONTENT_DIR}/`)) {
        throw new Error(`Unsafe snapshot content path: ${JSON.stringify(entry.storagePath)}`);
      }
      const contentPath = path.join(dir, entry.storagePath);
      await mkdir(path.dirname(contentPath), { recursive: true, mode: 0o700 });
      await writeTextAtomic(contentPath, entry.content);
    }

    await writeJsonAtomic(
      path.join(dir, SNAPSHOT_FILES.contentIndex),
      snapshot.content.map(({ content: _content, ...entry }) => entry)
    );
  }
}

export async function readSnapshot(
  storeDir: string,
  name: string,
  agent?: AgentId
): Promise<Snapshot> {
  const safeName = validateSnapshotName(name);
  const dir = snapshotDir(storeDir, safeName, agent);

  const [manifest, evidence, graph, auditFindings, provenance, content] = await Promise.all([
    readJson<Snapshot["manifest"]>(path.join(dir, SNAPSHOT_FILES.manifest)),
    readJson<Snapshot["evidence"]>(path.join(dir, SNAPSHOT_FILES.evidence)),
    readJson<Snapshot["graph"]>(path.join(dir, SNAPSHOT_FILES.graph)),
    readJson<Snapshot["auditFindings"]>(path.join(dir, SNAPSHOT_FILES.auditFindings)),
    readJson<Snapshot["provenance"]>(path.join(dir, SNAPSHOT_FILES.provenance)),
    readOptionalJson<SnapshotContentEntry[]>(path.join(dir, SNAPSHOT_FILES.contentIndex))
  ]);

  return {
    manifest,
    evidence,
    graph,
    auditFindings,
    provenance,
    ...(content && content.length > 0 ? { content } : {})
  };
}

export async function readSnapshotContent(
  storeDir: string,
  name: string,
  entry: SnapshotContentEntry,
  agent?: AgentId
): Promise<string> {
  const safeName = validateSnapshotName(name);
  if (!isSafeSnapshotRelativePath(entry.storagePath) || !entry.storagePath.startsWith(`${CONTENT_DIR}/`)) {
    throw new Error(`Unsafe snapshot content path: ${JSON.stringify(entry.storagePath)}`);
  }
  return await readFile(path.join(snapshotDir(storeDir, safeName, agent), entry.storagePath), "utf8");
}

export async function listSnapshots(
  storeDir: string,
  agent?: AgentId
): Promise<string[]> {
  const baseDir = agentStoreDir(storeDir, agent);

  if (!(await pathExists(baseDir))) {
    return [];
  }

  const entries = await readdir(baseDir, { withFileTypes: true });
  const names: string[] = [];

  for (const entry of entries) {
    if (!entry.isDirectory() || !isSafeSnapshotName(entry.name)) continue;

    if (agent) {
      // Agent-scoped: every subdirectory is a snapshot
      names.push(entry.name);
    } else {
      // Flat listing: only include directories that have a manifest.json
      // (not agent directories, which contain subdirectories)
      try {
        await access(
          path.join(baseDir, entry.name, SNAPSHOT_FILES.manifest),
          constants.R_OK
        );
        names.push(entry.name);
      } catch {
        // No manifest.json — this is an agent directory, skip it
      }
    }
  }

  return names.sort();
}

export async function snapshotExists(
  storeDir: string,
  name: string,
  agent?: AgentId
): Promise<boolean> {
  const safeName = validateSnapshotName(name);

  try {
    await access(path.join(snapshotDir(storeDir, safeName, agent), SNAPSHOT_FILES.manifest), constants.R_OK);
    return true;
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code === "ENOENT" || code === "ENOTDIR") {
      return false;
    }
    throw error;
  }
}

export interface TimelineListOptions {
  agent?: AgentId;
  projectPath?: string;
  limit?: number;
  onCorruptEntry?: (event: TimelineCorruptEvent) => void;
}

export interface TimelineCorruptEvent {
  filePath: string;
  error: string;
}

export async function appendTimelineEntry(
  storeDir: string,
  entry: TimelineEntry
): Promise<void> {
  validateSnapshotName(entry.afterSnapshotName);
  if (entry.beforeSnapshotName) {
    validateSnapshotName(entry.beforeSnapshotName);
  }
  await ensureStore(storeDir);
  const dir = timelineEventsDir(storeDir);
  await mkdir(dir, { recursive: true, mode: 0o700 });
  await chmod(dir, 0o700);
  await writeJsonAtomic(timelineEntryPath(storeDir, entry), entry);
}

export async function listTimelineEntries(
  storeDir: string,
  options: TimelineListOptions = {}
): Promise<TimelineEntry[]> {
  const dir = timelineEventsDir(storeDir);
  if (!(await pathExists(dir))) {
    return [];
  }

  const entries = await readTimelineEntries(dir, options.onCorruptEntry);
  const filtered = entries
    .filter((entry) => options.projectPath === undefined || path.resolve(entry.projectPath) === path.resolve(options.projectPath))
    .filter((entry) => options.agent === undefined || entry.agent === options.agent || entry.agents.includes(options.agent))
    .sort((left, right) => right.observedAt.localeCompare(left.observedAt));

  return typeof options.limit === "number"
    ? filtered.slice(0, Math.max(0, options.limit))
    : filtered;
}

export async function latestTimelineEntry(
  storeDir: string,
  options: Omit<TimelineListOptions, "limit"> = {}
): Promise<TimelineEntry | undefined> {
  return (await listTimelineEntries(storeDir, { ...options, limit: 1 }))[0];
}

export async function findTimelineEntry(
  storeDir: string,
  ref: string,
  options: Pick<TimelineListOptions, "onCorruptEntry"> = {}
): Promise<TimelineEntry | undefined> {
  const entries = await listTimelineEntries(storeDir, options);
  return entries.find((entry) => entry.id === ref || entry.afterSnapshotName === ref);
}

export function stateHash(snapshot: Snapshot): string {
  const hash = createHash("sha256");
  hash.update(JSON.stringify({
    evidence: snapshot.evidence,
    graph: snapshot.graph,
    auditFindings: snapshot.auditFindings,
    provenance: snapshot.provenance
  }));
  return `sha256:${hash.digest("hex")}`;
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

function isSafeSnapshotRelativePath(name: string): boolean {
  return name.trim().length > 0 &&
    !path.isAbsolute(name) &&
    !name.includes("\\") &&
    !name.split("/").includes("..");
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

async function writeTextAtomic(filePath: string, value: string): Promise<void> {
  const tempPath = `${filePath}.${process.pid}.${randomUUID()}.tmp`;
  try {
    await writeFile(tempPath, value, { mode: 0o600 });
    await rename(tempPath, filePath);
  } catch (error) {
    await rm(tempPath, { force: true });
    throw error;
  }
}

async function readJson<T>(filePath: string): Promise<T> {
  return JSON.parse(await readFile(filePath, "utf8")) as T;
}

async function readOptionalJson<T>(filePath: string): Promise<T | undefined> {
  try {
    return await readJson<T>(filePath);
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code === "ENOENT" || code === "ENOTDIR") {
      return undefined;
    }
    throw error;
  }
}

function timelineEventsDir(storeDir: string): string {
  return path.join(storeDir, TIMELINE_EVENTS_DIR);
}

function timelineEntryPath(storeDir: string, entry: TimelineEntry): string {
  const observed = entry.observedAt.replace(/[:.]/g, "-");
  return path.join(timelineEventsDir(storeDir), `${observed}-${entry.id}.json`);
}

async function readTimelineEntries(dir: string, onCorruptEntry?: (event: TimelineCorruptEvent) => void): Promise<TimelineEntry[]> {
  const entries: TimelineEntry[] = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    if (!entry.isFile() || !entry.name.endsWith(".json")) {
      continue;
    }
    try {
      entries.push(normalizeTimelineEntry(await readJson<unknown>(path.join(dir, entry.name))));
    } catch (error) {
      // Corrupt timeline events should not hide the rest of the local history.
      onCorruptEntry?.({
        filePath: path.join(dir, entry.name),
        error: error instanceof Error ? error.message : String(error)
      });
    }
  }
  return entries;
}

function normalizeTimelineEntry(raw: unknown): TimelineEntry {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    throw new Error("timeline event is not an object");
  }

  const record = raw as Record<string, unknown>;
  const legacyDaemonRunId = typeof record.daemonRunId === "string" ? record.daemonRunId : undefined;
  const captureId = typeof record.captureId === "string"
    ? record.captureId
    : legacyDaemonRunId ?? (typeof record.id === "string" ? record.id : "legacy");

  return {
    ...(record as unknown as TimelineEntry),
    source: "manual",
    captureId
  };
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
