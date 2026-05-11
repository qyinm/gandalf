import { readdir, readFile, lstat } from "node:fs/promises";
import path from "node:path";

import type { AgentId, DiscoveredItem, EvidenceKind, EvidenceScope } from "./types.js";
import { ignoredDirectory, MAX_DIRECTORY_DEPTH, MAX_DIRECTORY_ENTRIES, MAX_FILE_BYTES } from "./policy.js";
import { parseDotenvKeys, parseJson, parseMarkdown, parseTomlKeyValues } from "./parsers.js";

export interface ScanTrust {
  readOnly: true;
  network: "disabled";
  commandsExecuted: [];
  storeWriteLocation: string;
}

export interface ScanResult {
  trust: ScanTrust;
  evidence: DiscoveredItem[];
  blindSpots: string[];
}

export interface ScanProjectOptions {
  projectPath: string;
  homeDir: string;
  storeDir: string;
  explain?: boolean;
}

interface ScanTarget {
  absolutePath: string;
  sourcePath: string;
  scope: EvidenceScope;
  agent: AgentId;
  kind: EvidenceKind;
  parser: DiscoveredItem["parser"];
  precedence: number;
  sensitivity: string;
  contentPolicy: string;
  directory?: boolean;
  metadataOnly?: boolean;
}

export async function scanProject(options: ScanProjectOptions): Promise<ScanResult> {
  const evidence: DiscoveredItem[] = [];
  const projectPath = path.resolve(options.projectPath);
  const homeDir = path.resolve(options.homeDir);

  for (const target of targets(projectPath, homeDir)) {
    await scanTarget(target, evidence);
  }

  return {
    trust: {
      readOnly: true,
      network: "disabled",
      commandsExecuted: [],
      storeWriteLocation: options.storeDir
    },
    evidence,
    blindSpots: [
      "Remote MCP server behavior cannot be captured",
      "Provider-side model routing cannot be verified",
      "Raw env values are omitted by policy"
    ]
  };
}

function targets(projectPath: string, homeDir: string): ScanTarget[] {
  return [
    projectTarget(projectPath, "CLAUDE.md", "claude-code", "agent_instruction", "markdown"),
    projectTarget(projectPath, "AGENTS.md", "codex", "agent_instruction", "markdown"),
    projectTarget(projectPath, ".mcp.json", "claude-code", "agent_config", "json"),
    projectTarget(projectPath, ".cursor/mcp.json", "cursor", "agent_config", "json"),
    projectTarget(projectPath, ".claude/settings.json", "claude-code", "agent_config", "json"),
    projectTarget(projectPath, ".codex", "codex", "unsupported", "filesystem", { directory: true }),
    projectTarget(projectPath, ".env", "project", "env_key", "dotenv", {
      sensitivity: "env_key_inventory",
      contentPolicy: "key_inventory_only"
    }),
    homeTarget(homeDir, ".claude/settings.json", "claude-code", "agent_config", "json"),
    homeTarget(homeDir, ".claude.json", "claude-code", "agent_config", "json", {
      metadataOnly: true,
      sensitivity: "metadata"
    }),
    homeTarget(homeDir, ".claude/agents", "claude-code", "unsupported", "filesystem", { directory: true }),
    homeTarget(homeDir, ".claude/skills", "claude-code", "skill", "filesystem", { directory: true }),
    homeTarget(homeDir, ".codex/config.toml", "codex", "agent_config", "toml"),
    homeTarget(homeDir, ".codex/skills", "codex", "skill", "filesystem", { directory: true }),
    homeTarget(homeDir, ".cursor/mcp.json", "cursor", "agent_config", "json")
  ];
}

function projectTarget(
  projectPath: string,
  relativePath: string,
  agent: AgentId,
  kind: EvidenceKind,
  parser: DiscoveredItem["parser"],
  overrides: Partial<ScanTarget> = {}
): ScanTarget {
  return makeTarget(projectPath, relativePath, "project", 40, agent, kind, parser, overrides);
}

function homeTarget(
  homeDir: string,
  relativePath: string,
  agent: AgentId,
  kind: EvidenceKind,
  parser: DiscoveredItem["parser"],
  overrides: Partial<ScanTarget> = {}
): ScanTarget {
  return makeTarget(homeDir, relativePath, "user", 10, agent, kind, parser, overrides);
}

function makeTarget(
  root: string,
  relativePath: string,
  scope: EvidenceScope,
  precedence: number,
  agent: AgentId,
  kind: EvidenceKind,
  parser: DiscoveredItem["parser"],
  overrides: Partial<ScanTarget>
): ScanTarget {
  return {
    absolutePath: path.join(root, relativePath),
    sourcePath: scope === "user" ? `~/${relativePath}` : relativePath,
    scope,
    precedence,
    agent,
    kind,
    parser,
    sensitivity: "metadata",
    contentPolicy: "metadata_only",
    ...overrides
  };
}

async function scanTarget(target: ScanTarget, evidence: DiscoveredItem[]): Promise<void> {
  let stats;
  try {
    stats = await lstat(target.absolutePath);
  } catch (error) {
    if (isNotFound(error)) {
      return;
    }
    evidence.push(baseItem(target, "unsupported", { error: readableError(error) }));
    return;
  }

  if (stats.isSymbolicLink()) {
    evidence.push({
      ...baseItem(target, "omitted", { reason: "symlink_not_followed" }),
      id: itemId(target, "symlink"),
      kind: "symlink",
      parser: "filesystem",
      contentPolicy: "metadata_only"
    });
    return;
  }

  if (target.directory) {
    if (!stats.isDirectory()) {
      return;
    }
    await scanDirectory(target, evidence);
    return;
  }

  if (!stats.isFile()) {
    return;
  }

  if (stats.size > MAX_FILE_BYTES) {
    evidence.push(baseItem(target, "unsupported", { reason: "file_too_large", sizeBytes: stats.size }));
    return;
  }

  if (target.metadataOnly) {
    evidence.push(baseItem(target, "captured", { present: true, sizeBytes: stats.size }));
    return;
  }

  const text = await readFile(target.absolutePath, "utf8");
  if (target.parser === "dotenv") {
    for (const entry of parseDotenvKeys(text)) {
      evidence.push({
        ...baseItem(target, entry.captureStatus, { secretLike: entry.secretLike }),
        id: itemId(target, entry.key),
        name: entry.key
      });
    }
    return;
  }

  const parsed = parseTarget(target, text);
  if (!parsed.ok) {
    evidence.push(baseItem(target, "parse_failed", { error: parsed.error }));
    return;
  }

  if (target.parser === "json" && !target.metadataOnly) {
    emitJsonEvidence(target, parsed.value, evidence);
    return;
  }

  evidence.push(baseItem(target, "captured", undefined, parsed.value));
}

async function scanDirectory(target: ScanTarget, evidence: DiscoveredItem[]): Promise<void> {
  evidence.push(baseItem(target, target.kind === "unsupported" ? "unsupported" : "captured", { present: true }));
  await scanDirectoryEntries(target, target.absolutePath, target.sourcePath, evidence, 0);
}

async function scanDirectoryEntries(
  target: ScanTarget,
  absoluteDir: string,
  sourceDir: string,
  evidence: DiscoveredItem[],
  depth: number
): Promise<void> {
  if (depth >= MAX_DIRECTORY_DEPTH) {
    return;
  }

  let entries;
  try {
    entries = (await readdir(absoluteDir, { withFileTypes: true })).slice(0, MAX_DIRECTORY_ENTRIES);
  } catch {
    return;
  }

  for (const entry of entries) {
    if (entry.isDirectory() && ignoredDirectory(entry.name)) {
      continue;
    }

    const absolutePath = path.join(absoluteDir, entry.name);
    const sourcePath = `${sourceDir}/${entry.name}`;
    const childTarget = { ...target, absolutePath, sourcePath };
    const stats = await lstat(absolutePath);

    if (stats.isSymbolicLink()) {
      evidence.push({
        ...baseItem(childTarget, "omitted", { reason: "symlink_not_followed" }),
        id: itemId(childTarget, "symlink"),
        kind: "symlink",
        parser: "filesystem"
      });
    } else if (stats.isDirectory()) {
      evidence.push(baseItem(childTarget, target.kind === "skill" ? "captured" : "unsupported", { present: true }));
      await scanDirectoryEntries(target, absolutePath, sourcePath, evidence, depth + 1);
    } else if (stats.isFile()) {
      evidence.push(baseItem(childTarget, target.kind === "skill" ? "captured" : "unsupported", { sizeBytes: stats.size }));
    }
  }
}

function parseTarget(target: ScanTarget, text: string) {
  if (target.parser === "json") {
    return parseJson(text);
  }
  if (target.parser === "toml") {
    return parseTomlKeyValues(text);
  }
  if (target.parser === "markdown") {
    return parseMarkdown(text);
  }
  return { ok: true as const, value: { present: true } };
}

function emitJsonEvidence(target: ScanTarget, value: unknown, evidence: DiscoveredItem[]): void {
  if (target.sourcePath.endsWith(".mcp.json") || target.sourcePath.endsWith("/mcp.json")) {
    const servers = mcpServers(value);
    if (servers) {
      for (const [name, serverValue] of Object.entries(servers)) {
        evidence.push({
          ...baseItem(
            {
              ...target,
              kind: "mcp_server",
              sensitivity: "command_config",
              contentPolicy: "structured_safe_fields_only"
            },
            "captured",
            undefined,
            serverValue
          ),
          id: itemId(target, `mcp-${name}`),
          kind: "mcp_server",
          name
        });
      }
      return;
    }
  }

  evidence.push(baseItem(target, "captured", undefined, value));
}

function mcpServers(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }

  const servers = (value as Record<string, unknown>)["mcpServers"];
  if (!servers || typeof servers !== "object" || Array.isArray(servers)) {
    return undefined;
  }

  return servers as Record<string, unknown>;
}

function baseItem(
  target: ScanTarget,
  captureStatus: DiscoveredItem["captureStatus"],
  metadata?: Record<string, unknown>,
  value?: unknown
): DiscoveredItem {
  return {
    id: itemId(target, target.kind),
    agent: target.agent,
    kind: target.kind,
    sourcePath: target.sourcePath,
    scope: target.scope,
    precedence: target.precedence,
    parser: target.parser,
    sensitivity: target.sensitivity,
    contentPolicy: target.contentPolicy,
    restorePolicy: "not_supported_v0_1",
    captureStatus,
    confidence: "high",
    ...(value === undefined ? {} : { value }),
    ...(metadata === undefined ? {} : { metadata })
  };
}

function itemId(target: ScanTarget, suffix: string): string {
  return `${target.scope}.${target.agent}.${target.sourcePath}.${suffix}`
    .replace(/^~\//, "home/")
    .replace(/[^A-Za-z0-9_.-]+/g, ".")
    .replace(/^\.+|\.+$/g, "")
    .toLowerCase();
}

function isNotFound(error: unknown): boolean {
  return typeof error === "object" && error !== null && "code" in error && error.code === "ENOENT";
}

function readableError(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown filesystem error";
}
