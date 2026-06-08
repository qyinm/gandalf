import { lstat, readdir, readFile, realpath, stat } from "node:fs/promises";
import path from "node:path";

import { ignoredDirectory, restorePolicyFor } from "../policy.js";
import { parseJson } from "../parsers.js";
import type { DiscoveredItem, EvidenceScope } from "../types.js";
import type { ScanTarget } from "./index.js";
import { homeTarget, projectTarget } from "./index.js";
import type { ScannerPlugin } from "./scanner-plugin.js";

export const cursorScanner: ScannerPlugin = {
  agentId: "cursor",
  agentName: "Cursor",
  description: "Cursor editor configuration (MCP servers, skills, hooks)",

  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return cursorMcpTargets(projectPath, homeDir);
  },

  async scan({ projectPath, homeDir }): Promise<DiscoveredItem[]> {
    const mcpEvidence = await scanCursorMcpServers(projectPath, homeDir);
    const hookEvidence = await scanCursorHooks(projectPath, homeDir);
    const skillEvidence: DiscoveredItem[] = [];

    for (const target of await cursorSkillTargets(projectPath, homeDir)) {
      skillEvidence.push(...await scanCursorSkillDirectory(target));
    }
    const cursorEvidence = [
      ...mcpEvidence,
      ...dedupeSkillsByName(skillEvidence),
      ...hookEvidence,
    ];

    return cursorEvidence.length > 0 ? [...cursorEvidence, cursorTeamHooksBlindSpot()] : [];
  },
};

function cursorMcpTargets(projectPath: string, homeDir: string): ScanTarget[] {
  return [
    projectTarget(projectPath, ".cursor/mcp.json", "cursor", "agent_config", "json", {
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    }),
    homeTarget(homeDir, ".cursor/mcp.json", "cursor", "agent_config", "json", {
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    }),
  ];
}

async function scanCursorMcpServers(projectPath: string, homeDir: string): Promise<DiscoveredItem[]> {
  const evidence: DiscoveredItem[] = [];
  for (const target of cursorMcpTargets(projectPath, homeDir)) {
    evidence.push(...await scanCursorMcpFile(target));
  }
  return evidence;
}

async function scanCursorMcpFile(target: ScanTarget): Promise<DiscoveredItem[]> {
  let text;
  try {
    text = await readFile(target.absolutePath, "utf8");
  } catch {
    return [];
  }

  const parsed = parseJson(text);
  if (!parsed.ok) {
    return [parseFailedItem(target, "agent_config", parsed.error)];
  }

  const servers = asRecord(asRecord(parsed.value)?.["mcpServers"]);
  if (!servers) {
    return [capturedItem(target, "agent_config", undefined, parsed.value)];
  }

  return Object.entries(servers).map(([name, value]) => {
    const serverValue = sanitizeMcpServer(asRecord(value) ?? {});
    const transport = transportForMcpServer(serverValue);
    const remote = transport !== "stdio" && Boolean(serverValue["url"]);
    return {
      ...capturedItem(
        {
          ...target,
          kind: "mcp_server",
          sensitivity: "command_config",
          contentPolicy: "structured_safe_fields_only",
        },
        "mcp_server",
        {
          transport,
          remote,
          source: target.scope,
          ...(serverValue["envFile"] ? { envFile: serverValue["envFile"] } : {}),
          authConfigured: Boolean(serverValue["auth"]),
          interpolationFields: interpolationFieldsForMcpServer(serverValue),
        },
        serverValue
      ),
      id: itemId(target, `mcp-${name}`),
      name,
    };
  });
}

function sanitizeMcpServer(value: Record<string, unknown>): Record<string, unknown> {
  const sanitized: Record<string, unknown> = {};
  for (const [key, nestedValue] of Object.entries(value)) {
    if (key === "url" && typeof nestedValue === "string") {
      sanitized.url = redactUrl(nestedValue);
    } else {
      sanitized[key] = nestedValue;
    }
  }
  return sanitized;
}

function transportForMcpServer(value: Record<string, unknown>): string {
  const type = typeof value["type"] === "string" ? value["type"].toLowerCase() : "";
  if (type === "stdio") return "stdio";
  if (type === "sse") return "sse";
  if (type === "streamable-http" || type === "streamable_http" || type === "http") return "streamable-http";
  if (value["command"]) return "stdio";
  if (value["url"]) return "streamable-http";
  return "unknown";
}

function interpolationFieldsForMcpServer(value: Record<string, unknown>): string[] {
  const fields: string[] = [];
  for (const field of ["command", "args", "env", "url", "headers"] as const) {
    if (containsInterpolation(value[field])) {
      fields.push(field);
    }
  }
  return fields;
}

function containsInterpolation(value: unknown): boolean {
  if (typeof value === "string") {
    return /\$\{(?:env:[^}]+|userHome|workspaceFolder|workspaceFolderBasename|pathSeparator|\/)}/.test(value);
  }
  if (Array.isArray(value)) {
    return value.some(containsInterpolation);
  }
  const record = asRecord(value);
  return record ? Object.values(record).some(containsInterpolation) : false;
}

function redactUrl(value: string): string {
  try {
    const url = new URL(value);
    if (url.username) url.username = "[redacted]";
    if (url.password) url.password = "[redacted]";
    for (const key of [...url.searchParams.keys()]) {
      url.searchParams.set(key, "[redacted]");
    }
    return url.toString();
  } catch {
    return value.replace(/([?&][^=]+)=([^&]+)/g, "$1=[redacted]");
  }
}

async function cursorSkillTargets(projectPath: string, homeDir: string): Promise<ScanTarget[]> {
  const explicitTargets = [
    projectTarget(projectPath, ".cursor/skills", "cursor", "skill", "filesystem", { directory: true }),
    projectTarget(projectPath, ".agents/skills", "cursor", "skill", "filesystem", { directory: true }),
    projectTarget(projectPath, ".claude/skills", "cursor", "skill", "filesystem", { directory: true }),
    projectTarget(projectPath, ".codex/skills", "cursor", "skill", "filesystem", { directory: true }),
    homeTarget(homeDir, ".cursor/skills", "cursor", "skill", "filesystem", { directory: true }),
    homeTarget(homeDir, ".agents/skills", "cursor", "skill", "filesystem", { directory: true }),
    homeTarget(homeDir, ".claude/skills", "cursor", "skill", "filesystem", { directory: true }),
    homeTarget(homeDir, ".codex/skills", "cursor", "skill", "filesystem", { directory: true }),
  ];

  const nestedProjectTargets = await nestedCursorSkillTargets(projectPath);
  const targets = new Map<string, ScanTarget>();
  for (const target of [...explicitTargets, ...nestedProjectTargets]) {
    targets.set(target.sourcePath, target);
  }
  return [...targets.values()];
}

async function nestedCursorSkillTargets(projectPath: string): Promise<ScanTarget[]> {
  const roots: ScanTarget[] = [];
  await walkForNestedSkillRoots(projectPath, projectPath, roots, 0);
  return roots;
}

async function walkForNestedSkillRoots(
  projectPath: string,
  absoluteDir: string,
  targets: ScanTarget[],
  depth: number
): Promise<void> {
  if (depth > 8) {
    return;
  }

  let entries;
  try {
    entries = await readdir(absoluteDir, { withFileTypes: true });
  } catch {
    return;
  }

  for (const entry of entries) {
    if (!entry.isDirectory() || ignoredDirectory(entry.name)) {
      continue;
    }

    const absolutePath = path.join(absoluteDir, entry.name);
    let stats;
    try {
      stats = await lstat(absolutePath);
    } catch {
      continue;
    }
    if (stats.isSymbolicLink()) {
      continue;
    }

    if (entry.name === ".cursor" || entry.name === ".agents") {
      const skillsPath = path.join(absolutePath, "skills");
      try {
        if ((await lstat(skillsPath)).isDirectory()) {
          const sourcePath = normalizeSourcePath(projectPath, skillsPath);
          targets.push({
            absolutePath: skillsPath,
            sourcePath,
            scope: "project",
            precedence: 40,
            agent: "cursor",
            kind: "skill",
            parser: "filesystem",
            sensitivity: "metadata",
            contentPolicy: "metadata_only",
            directory: true,
          });
        }
      } catch {
        // No skill root here.
      }
    }

    await walkForNestedSkillRoots(projectPath, absolutePath, targets, depth + 1);
  }
}

async function scanCursorSkillDirectory(target: ScanTarget): Promise<DiscoveredItem[]> {
  const skillFiles = await findSkillFiles(target.absolutePath);
  const evidence: DiscoveredItem[] = [];
  const scopeRoot = scopeRootForSkillTarget(target);

  for (const skillFile of skillFiles) {
    const frontmatter = await readSkillFrontmatter(skillFile);
    const skillDir = path.dirname(skillFile);
    const directoryName = path.basename(skillDir);
    if (!frontmatter?.name || !frontmatter.description) {
      continue;
    }
    if (!validSkillName(frontmatter.name) || frontmatter.name !== directoryName) {
      continue;
    }

    const relativeSkillDir = path.relative(target.absolutePath, skillDir).split(path.sep).join("/");
    const sourcePath = relativeSkillDir ? `${target.sourcePath}/${relativeSkillDir}` : target.sourcePath;
    evidence.push({
      id: itemId({ ...target, sourcePath }, "skill"),
      agent: "cursor",
      kind: "skill",
      sourcePath,
      scope: target.scope,
      precedence: target.precedence,
      parser: "filesystem",
      sensitivity: target.sensitivity,
      contentPolicy: target.contentPolicy,
      restorePolicy: "full_content_supported",
      captureStatus: "captured",
      confidence: "high",
      name: frontmatter.name,
      metadata: {
        present: true,
        entrypoint: path.basename(skillFile),
        entrypointStatus: "captured",
        entrypointSizeBytes: frontmatter.sizeBytes,
        declaredName: frontmatter.name,
        directoryName,
        nameMatchesDirectory: true,
        description: frontmatter.description,
        sourceRoot: target.sourcePath,
        scopeRoot,
        ...(frontmatter.paths ? { paths: frontmatter.paths } : {}),
        ...(frontmatter.disableModelInvocation !== undefined ? { disableModelInvocation: frontmatter.disableModelInvocation } : {}),
        ...(frontmatter.metadata ? { skillMetadata: frontmatter.metadata } : {}),
      },
    });
  }

  return evidence;
}

async function findSkillFiles(root: string): Promise<string[]> {
  const files: string[] = [];
  await walkSkillFiles(root, files, 0, new Set());
  return files;
}

async function walkSkillFiles(dir: string, files: string[], depth: number, seen: Set<string>): Promise<void> {
  if (depth > 8) {
    return;
  }

  let resolved;
  try {
    resolved = await realpath(dir);
  } catch {
    return;
  }
  if (seen.has(resolved)) {
    return;
  }
  seen.add(resolved);

  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return;
  }

  for (const entry of entries) {
    const absolutePath = path.join(dir, entry.name);
    let entryStats;
    try {
      entryStats = await lstat(absolutePath);
    } catch {
      continue;
    }
    if (entryStats.isSymbolicLink()) continue;
    if (entryStats.isDirectory() && !ignoredDirectory(entry.name)) {
      await walkSkillFiles(absolutePath, files, depth + 1, seen);
      continue;
    }
    if (entryStats.isFile() && entry.name === "SKILL.md") {
      files.push(absolutePath);
    }
  }
}

interface CursorSkillFrontmatter {
  name?: string;
  description?: string;
  paths?: string[];
  disableModelInvocation?: boolean;
  metadata?: Record<string, string>;
  sizeBytes: number;
}

async function readSkillFrontmatter(filePath: string): Promise<CursorSkillFrontmatter | null> {
  let text;
  let stats;
  try {
    stats = await stat(filePath);
    text = await readFile(filePath, "utf8");
  } catch {
    return null;
  }

  const match = /^---\r?\n([\s\S]*?)\r?\n---/.exec(text);
  if (!match) {
    return { sizeBytes: stats.size };
  }

  const frontmatter: CursorSkillFrontmatter = { sizeBytes: stats.size };
  const lines = match[1].split(/\r?\n/);
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index].trim();
    const scalar = /^(name|description|disable-model-invocation):\s*(.*)$/.exec(line);
    if (scalar) {
      const [, key, rawValue] = scalar;
      const value = unquoteYamlScalar(rawValue);
      if (key === "name") frontmatter.name = value;
      if (key === "description") frontmatter.description = value;
      if (key === "disable-model-invocation") frontmatter.disableModelInvocation = value === "true";
      continue;
    }
    if (line === "paths:") {
      const values: string[] = [];
      while (lines[index + 1]?.trim().startsWith("- ")) {
        values.push(unquoteYamlScalar(lines[++index].trim().slice(2)));
      }
      frontmatter.paths = values;
      continue;
    }
    if (line === "metadata:") {
      const metadata: Record<string, string> = {};
      while (lines[index + 1]?.startsWith("  ")) {
        const nested = /^\s+([A-Za-z0-9_.-]+):\s*(.*)$/.exec(lines[++index]);
        if (nested) {
          metadata[nested[1]] = unquoteYamlScalar(nested[2]);
        }
      }
      frontmatter.metadata = metadata;
    }
  }

  return frontmatter;
}

function unquoteYamlScalar(value: string): string {
  return value.trim().replace(/^['"]|['"]$/g, "");
}

function scopeRootForSkillTarget(target: ScanTarget): string | undefined {
  if (target.scope !== "project") {
    return undefined;
  }
  const marker = target.sourcePath.match(/^(.*?)(?:^|\/)(?:\.cursor|\.agents)\/skills$/);
  if (!marker) {
    return ".";
  }
  return marker[1] || ".";
}

function validSkillName(name: string): boolean {
  return /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(name);
}

function cursorHookTargets(projectPath: string, homeDir: string): ScanTarget[] {
  const targets = [
    projectTarget(projectPath, ".cursor/hooks.json", "cursor", "hook", "json", {
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    }),
    homeTarget(homeDir, ".cursor/hooks.json", "cursor", "hook", "json", {
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    }),
  ];

  if (process.platform === "darwin") {
    targets.push({
      absolutePath: "/Library/Application Support/Cursor/hooks.json",
      sourcePath: "/Library/Application Support/Cursor/hooks.json",
      scope: "managed",
      precedence: 80,
      agent: "cursor",
      kind: "hook",
      parser: "json",
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    });
  }

  return targets;
}

async function scanCursorHooks(projectPath: string, homeDir: string): Promise<DiscoveredItem[]> {
  const evidence: DiscoveredItem[] = [];
  for (const target of cursorHookTargets(projectPath, homeDir)) {
    evidence.push(...await scanCursorHookFile(target));
  }
  return evidence;
}

async function scanCursorHookFile(target: ScanTarget): Promise<DiscoveredItem[]> {
  let text;
  try {
    text = await readFile(target.absolutePath, "utf8");
  } catch {
    return [];
  }

  const parsed = parseJson(text);
  if (!parsed.ok) {
    return [parseFailedItem(target, "hook", parsed.error)];
  }

  const hooks = asRecord(asRecord(parsed.value)?.["hooks"]);
  if (!hooks) {
    return [];
  }

  const evidence: DiscoveredItem[] = [];
  for (const [eventName, definitions] of Object.entries(hooks)) {
    if (!Array.isArray(definitions)) {
      continue;
    }
    for (let hookIndex = 0; hookIndex < definitions.length; hookIndex++) {
      const definition = asRecord(definitions[hookIndex]);
      if (!definition) {
        continue;
      }
      const type = typeof definition["type"] === "string" ? definition["type"] : "command";
      const command = typeof definition["command"] === "string" ? definition["command"] : undefined;
      const hookValue = cursorHookValue(definition, type, command);

      evidence.push({
        ...capturedItem(
          {
            ...target,
            kind: "hook",
            sensitivity: "command_config",
            contentPolicy: "structured_safe_fields_only",
          },
          "hook",
          {
            executable: type === "command" && Boolean(command),
            policyEvaluated: type === "prompt",
            eventName,
            hookIndex,
            hookCategory: hookCategory(eventName),
            source: cursorHookSource(target.scope),
            sourcePriority: cursorHookSourcePriority(target.scope),
          },
          hookValue
        ),
        id: itemId(target, `hook-${eventName}-${hookIndex}`),
        name: `${eventName}.${hookIndex}`,
      });
    }
  }

  return evidence;
}

function cursorHookValue(definition: Record<string, unknown>, type: string, command?: string): Record<string, unknown> {
  const value: Record<string, unknown> = { type };
  if (command) value.command = command;
  for (const field of ["timeout", "loop_limit", "failClosed", "matcher"] as const) {
    if (definition[field] !== undefined) {
      value[field] = definition[field];
    }
  }
  return value;
}

function hookCategory(eventName: string): string {
  if (eventName === "beforeTabFileRead" || eventName === "afterTabFileEdit") {
    return "tab";
  }
  if (eventName === "workspaceOpen") {
    return "app_lifecycle";
  }
  return "agent";
}

function cursorHookSource(scope: EvidenceScope): string {
  if (scope === "managed") return "enterprise";
  return scope;
}

function cursorHookSourcePriority(scope: EvidenceScope): number {
  if (scope === "managed") return 40;
  if (scope === "project") return 30;
  if (scope === "user") return 10;
  return 0;
}

function cursorTeamHooksBlindSpot(): DiscoveredItem {
  return {
    id: "managed.cursor.cursor-team-hooks.unsupported",
    agent: "cursor",
    kind: "unsupported",
    sourcePath: "<cursor-team-hooks>",
    scope: "managed",
    precedence: 70,
    parser: "unknown",
    sensitivity: "metadata",
    contentPolicy: "metadata_only",
    restorePolicy: "not_supported",
    captureStatus: "unsupported",
    confidence: "medium",
    name: "Cursor team hooks",
    metadata: {
      reason: "cloud_distributed_hooks_not_locally_readable",
      source: "team",
      sourcePriority: 35,
    },
  };
}

function capturedItem(
  target: ScanTarget,
  kind: DiscoveredItem["kind"],
  metadata?: Record<string, unknown>,
  value?: unknown
): DiscoveredItem {
  return {
    id: itemId(target, kind),
    agent: target.agent,
    kind,
    sourcePath: target.sourcePath,
    scope: target.scope,
    precedence: target.precedence,
    parser: target.parser,
    sensitivity: target.sensitivity,
    contentPolicy: target.contentPolicy,
    restorePolicy: restorePolicyFor(kind),
    captureStatus: "captured",
    confidence: "high",
    ...(value === undefined ? {} : { value }),
    ...(metadata === undefined ? {} : { metadata }),
  };
}

function parseFailedItem(target: ScanTarget, kind: DiscoveredItem["kind"], error: string): DiscoveredItem {
  return {
    ...capturedItem(target, kind, { error }),
    id: itemId(target, `${kind}-parse-failed`),
    captureStatus: "parse_failed",
  };
}

function itemId(target: Pick<ScanTarget, "scope" | "agent" | "sourcePath">, suffix: string): string {
  return `${target.scope}.${target.agent}.${target.sourcePath}.${suffix}`
    .replace(/^~\//, "home/")
    .replace(/[^A-Za-z0-9_.-]+/g, ".")
    .replace(/^\.+|\.+$/g, "")
    .toLowerCase();
}

function normalizeSourcePath(root: string, absolutePath: string): string {
  return path.relative(root, absolutePath).split(path.sep).join("/");
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function dedupeSkillsByName(evidence: DiscoveredItem[]): DiscoveredItem[] {
  const result: DiscoveredItem[] = [];
  const skillIndexes = new Map<string, number>();

  for (const item of evidence) {
    if (item.kind !== "skill" || !item.name) {
      result.push(item);
      continue;
    }

    const existingIndex = skillIndexes.get(item.name);
    if (existingIndex === undefined) {
      skillIndexes.set(item.name, result.length);
      result.push(item);
      continue;
    }

    const existing = result[existingIndex]!;
    if (item.precedence > existing.precedence) {
      result[existingIndex] = {
        ...item,
        metadata: {
          ...item.metadata,
          duplicateSources: [
            existing.sourcePath,
            ...metadataStringArray(existing.metadata?.["duplicateSources"]),
          ],
        },
      };
    } else {
      result[existingIndex] = {
        ...existing,
        metadata: {
          ...existing.metadata,
          duplicateSources: [
            ...metadataStringArray(existing.metadata?.["duplicateSources"]),
            item.sourcePath,
          ],
        },
      };
    }
  }

  return result;
}

function metadataStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}
