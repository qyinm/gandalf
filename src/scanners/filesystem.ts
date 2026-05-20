import { lstat, readdir, readFile } from "node:fs/promises";
import path from "node:path";

import { ignoredDirectory, MAX_DIRECTORY_DEPTH, MAX_DIRECTORY_ENTRIES, MAX_FILE_BYTES, restorePolicyFor } from "../policy.js";
import { parseDotenvKeys, parseJson, parseMarkdown, parseTomlKeyValues } from "../parsers.js";
import type { DiscoveredItem } from "../types.js";
import type { ScanTarget } from "./index.js";

export async function scanTargets(targets: ScanTarget[]): Promise<DiscoveredItem[]> {
  const evidence: DiscoveredItem[] = [];
  for (const target of targets) {
    evidence.push(...await scanTarget(target));
  }
  return evidence;
}

export async function scanTarget(target: ScanTarget): Promise<DiscoveredItem[]> {
  const evidence: DiscoveredItem[] = [];
  let stats;
  try {
    stats = await lstat(target.absolutePath);
  } catch (error) {
    if (isNotFound(error)) {
      return evidence;
    }
    return [baseItem(target, "unsupported", { error: readableError(error) })];
  }

  if (stats.isSymbolicLink()) {
    return [{
      ...baseItem(target, "omitted", { reason: "symlink_not_followed" }),
      id: itemId(target, "symlink"),
      kind: "symlink",
      parser: "filesystem",
      contentPolicy: "metadata_only",
      restorePolicy: restorePolicyFor("symlink"),
    }];
  }

  if (target.directory) {
    if (!stats.isDirectory()) {
      return evidence;
    }
    return scanDirectory(target);
  }

  if (!stats.isFile()) {
    return evidence;
  }

  if (stats.size > MAX_FILE_BYTES) {
    return [baseItem(target, "unsupported", { reason: "file_too_large", sizeBytes: stats.size })];
  }

  if (target.metadataOnly) {
    return [baseItem(target, "captured", { present: true, sizeBytes: stats.size })];
  }

  const text = await readFile(target.absolutePath, "utf8");
  if (target.parser === "dotenv") {
    return parseDotenvKeys(text).map((entry) => ({
      ...baseItem(target, entry.captureStatus, { secretLike: entry.secretLike }),
      id: itemId(target, entry.key),
      name: entry.key,
    }));
  }

  const parsed = parseTarget(target, text);
  if (!parsed.ok) {
    return [baseItem(target, "parse_failed", { error: parsed.error })];
  }

  if (target.parser === "json" && !target.metadataOnly) {
    return emitJsonEvidence(target, parsed.value);
  }

  return [baseItem(target, "captured", undefined, parsed.value)];
}

export async function scanSkillDirectory(target: ScanTarget): Promise<DiscoveredItem[]> {
  const evidence: DiscoveredItem[] = [];
  let entries;
  try {
    entries = (await readdir(target.absolutePath, { withFileTypes: true })).slice(0, MAX_DIRECTORY_ENTRIES);
  } catch {
    return evidence;
  }

  for (const entry of entries) {
    if (entry.isDirectory() && ignoredDirectory(entry.name)) {
      continue;
    }

    const absolutePath = path.join(target.absolutePath, entry.name);
    const sourcePath = `${target.sourcePath}/${entry.name}`;
    const childTarget = { ...target, absolutePath, sourcePath };
    const stats = await lstat(absolutePath);

    if (stats.isSymbolicLink()) {
      evidence.push({
        ...baseItem(childTarget, "omitted", { reason: "symlink_not_followed", skillName: entry.name }),
        name: entry.name,
      });
      continue;
    }

    if (!stats.isDirectory()) {
      continue;
    }

    const metadata: Record<string, unknown> = { present: true, skillName: entry.name };
    try {
      const entrypoint = await lstat(path.join(absolutePath, "SKILL.md"));
      metadata["entrypoint"] = "SKILL.md";
      metadata["entrypointStatus"] = entrypoint.isSymbolicLink() ? "symlink_not_followed" : "captured";
      if (entrypoint.isFile()) {
        metadata["entrypointSizeBytes"] = entrypoint.size;
      }
    } catch (error) {
      if (!isNotFound(error)) {
        metadata["entrypointStatus"] = "unreadable";
        metadata["entrypointError"] = readableError(error);
      }
    }

    evidence.push({
      ...baseItem(childTarget, "captured", metadata),
      name: entry.name,
    });
  }

  return evidence;
}

async function scanDirectory(target: ScanTarget): Promise<DiscoveredItem[]> {
  if (target.kind === "skill") {
    return scanSkillDirectory(target);
  }

  const evidence = [baseItem(target, target.kind === "unsupported" ? "unsupported" : "captured", { present: true })];
  await scanDirectoryEntries(target, target.absolutePath, target.sourcePath, evidence, 0);
  return evidence;
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
        parser: "filesystem",
        restorePolicy: restorePolicyFor("symlink"),
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

function emitJsonEvidence(target: ScanTarget, value: unknown): DiscoveredItem[] {
  if (target.sourcePath.endsWith(".mcp.json") || target.sourcePath.endsWith("/mcp.json")) {
    const servers = mcpServers(value);
    if (servers) {
      return Object.entries(servers).map(([name, serverValue]) => ({
        ...baseItem(
          {
            ...target,
            kind: "mcp_server",
            sensitivity: "command_config",
            contentPolicy: "structured_safe_fields_only",
          },
          "captured",
          undefined,
          serverValue
        ),
        id: itemId(target, `mcp-${name}`),
        kind: "mcp_server",
        name,
      }));
    }
  }

  const evidence: DiscoveredItem[] = [];
  if (target.sourcePath.endsWith("/settings.json") || target.sourcePath.endsWith("settings.json")) {
    const value_ = isObject(value) ? (value as Record<string, unknown>) : {};
    const perms = permissionsFrom(value_);
    if (perms) {
      for (const [permName, permRule] of Object.entries(perms)) {
        evidence.push({
          ...baseItem(
            {
              ...target,
              kind: "permission",
              sensitivity: "command_config",
              contentPolicy: "structured_safe_fields_only",
            },
            "captured",
            undefined,
            { rule: permRule }
          ),
          id: itemId(target, `perm-${permName}`),
          kind: "permission",
          name: String(permRule),
        });
      }
    }

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
          for (const hook of nestedHooks) {
            if (!isObject(hook)) continue;
            const command = (hook as Record<string, unknown>).command;
            if (typeof command !== "string") continue;
            evidence.push({
              ...baseItem(
                {
                  ...target,
                  kind: "hook",
                  sensitivity: "command_config",
                  contentPolicy: "structured_safe_fields_only",
                },
                "captured",
                { executable: true, eventName, matcher, command },
                hook
              ),
              id: itemId(target, `hook-${eventName}-${i}`),
              kind: "hook",
              name: `${eventName}.${matcher}`,
            });
          }
        }
      }
    }
  }

  evidence.push(baseItem(target, "captured", undefined, value));
  return evidence;
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
    restorePolicy: restorePolicyFor(target.kind),
    captureStatus,
    confidence: "high",
    ...(value === undefined ? {} : { value }),
    ...(metadata === undefined ? {} : { metadata }),
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
