import { readdir, readFile, lstat } from "node:fs/promises";
import path from "node:path";

import type { AgentId, DiscoveredItem, EvidenceKind, EvidenceScope } from "./types.js";
import { ignoredDirectory, MAX_DIRECTORY_DEPTH, MAX_DIRECTORY_ENTRIES, MAX_FILE_BYTES } from "./policy.js";
import { parseDotenvKeys, parseJson, parseMarkdown, parseTomlKeyValues } from "./parsers.js";
import { defaultScannerPlugins, type ScanTarget } from "./scanners/index.js";

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

export async function scanProject(options: ScanProjectOptions): Promise<ScanResult> {
  const evidence: DiscoveredItem[] = [];
  const projectPath = path.resolve(options.projectPath);
  const homeDir = path.resolve(options.homeDir);

  // Collect targets from all registered scanner plugins
  const allTargets = defaultScannerPlugins().flatMap(
    (plugin) => plugin.targets(projectPath, homeDir)
  );

  for (const target of allTargets) {
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
  } catch (error) {
    // Directory may be unreadable (permissions); skip silently but report for debugging
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

  // Extract hook commands and permissions from settings.json
  if (target.sourcePath.endsWith("/settings.json") || target.sourcePath.endsWith("settings.json")) {
    const value_ = isObject(value) ? (value as Record<string, unknown>) : {};
    
    // Extract permissions
    const perms = permissionsFrom(value_);
    if (perms) {
      for (const [permName, permRule] of Object.entries(perms)) {
        evidence.push({
          ...baseItem(
            {
              ...target,
              kind: "permission",
              sensitivity: "command_config",
              contentPolicy: "structured_safe_fields_only"
            },
            "captured",
            undefined,
            { rule: permRule }
          ),
          id: itemId(target, `perm-${permName}`),
          kind: "permission",
          name: String(permRule)
        });
      }
    }

    // Extract hooks
    const hooksValue = isObject(value) ? (value as Record<string, unknown>)["hooks"] : undefined;
    if (isObject(hooksValue)) {
      for (const [eventName, eventHooks] of Object.entries(hooksValue as Record<string, unknown>)) {
        if (!Array.isArray(eventHooks)) continue;
        for (let i = 0; i < eventHooks.length; i++) {
          const entry = eventHooks[i];
          if (!isObject(entry)) continue;
          const matcher = typeof (entry as Record<string, unknown>).matcher === "string"
            ? (entry as Record<string, unknown>).matcher as string
            : "*";
          const nestedHooks = (entry as Record<string, unknown>)["hooks"];
          if (!Array.isArray(nestedHooks)) continue;
          for (let j = 0; j < nestedHooks.length; j++) {
            const hook = nestedHooks[j];
            if (!isObject(hook)) continue;
            const command = (hook as Record<string, unknown>).command;
            if (typeof command !== "string") continue;
            evidence.push({
              ...baseItem(
                {
                  ...target,
                  kind: "hook",
                  sensitivity: "command_config",
                  contentPolicy: "structured_safe_fields_only"
                },
                "captured",
                { executable: true, eventName, matcher, command },
                hook
              ),
              id: itemId(target, `hook-${eventName}-${i}`),
              kind: "hook",
              name: `${eventName}.${matcher}`
            });
          }
        }
      }
    }
  }

  evidence.push(baseItem(target, "captured", undefined, value));
}

function isObject(value: unknown): boolean {
  return value !== null && typeof value === "object" && !Array.isArray(value);
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

/**
 * Extract permission rules from a Claude Code settings.json value.
 * Permissions are typically under the "permissions" key as an object
 * of rule names to rule values (e.g. {"allow": ["Bash(npm test)"]}).
 */
function permissionsFrom(value: Record<string, unknown>): Record<string, unknown> | undefined {
  const perms = value["permissions"];
  if (!perms || typeof perms !== "object" || Array.isArray(perms)) {
    return undefined;
  }
  return perms as Record<string, unknown>;
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
