import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { projectTarget, homeTarget } from "./index.js";
import { scanTargets } from "./filesystem.js";
import type { DiscoveredItem } from "../types.js";
import { lstat, readdir, readFile, realpath, stat } from "node:fs/promises";
import path from "node:path";

export const codexScanner: ScannerPlugin = {
  agentId: "codex",
  agentName: "Codex",
  description: "Codex agent configuration (prompts, config, MCP servers, skills)",
  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, "AGENTS.md", "codex", "agent_instruction", "markdown"),
      projectTarget(projectPath, ".codex", "codex", "unsupported", "filesystem", { directory: true }),
      homeTarget(homeDir, ".codex/config.toml", "codex", "agent_config", "toml"),
    ];
  },

  async scan({ projectPath, homeDir }): Promise<DiscoveredItem[]> {
    const configEvidence = await scanTargets(this.targets(projectPath, homeDir));
    const mcpEvidence = await scanCodexMcpServers(homeDir);
    const hookEvidence = await scanCodexHooks(projectPath, homeDir);
    const skillEvidence: DiscoveredItem[] = [];

    for (const target of codexSkillTargets(homeDir)) {
      skillEvidence.push(...await scanCodexSkillDirectory(target));
    }

    return [
      ...configEvidence,
      ...mcpEvidence,
      ...hookEvidence,
      ...dedupeSkillsBySource(skillEvidence),
    ];
  }
};

function codexSkillTargets(homeDir: string): ScanTarget[] {
  return [
    homeTarget(homeDir, ".codex/skills", "codex", "skill", "filesystem", { directory: true }),
    homeTarget(homeDir, ".codex/plugins/cache", "codex", "skill", "filesystem", { directory: true }),
    homeTarget(homeDir, ".codex/vendor_imports/skills", "codex", "skill", "filesystem", { directory: true }),
  ];
}

function codexHookTargets(projectPath: string, homeDir: string): ScanTarget[] {
  return [
    projectTarget(projectPath, ".codex/hooks.json", "codex", "hook", "json", {
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    }),
    homeTarget(homeDir, ".codex/hooks.json", "codex", "hook", "json", {
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    }),
  ];
}

async function scanCodexHooks(projectPath: string, homeDir: string): Promise<DiscoveredItem[]> {
  const evidence: DiscoveredItem[] = [];

  for (const target of codexHookTargets(projectPath, homeDir)) {
    evidence.push(...await scanCodexHooksFile(target));
  }
  for (const target of codexInlineHookTargets(projectPath, homeDir)) {
    evidence.push(...await scanCodexInlineHooksFile(target));
  }

  return evidence;
}

function codexInlineHookTargets(projectPath: string, homeDir: string): ScanTarget[] {
  return [
    projectTarget(projectPath, ".codex/config.toml", "codex", "hook", "toml", {
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    }),
    homeTarget(homeDir, ".codex/config.toml", "codex", "hook", "toml", {
      sensitivity: "command_config",
      contentPolicy: "structured_safe_fields_only",
    }),
  ];
}

async function scanCodexInlineHooksFile(target: ScanTarget): Promise<DiscoveredItem[]> {
  let text;
  try {
    text = await readFile(target.absolutePath, "utf8");
  } catch {
    return [];
  }

  return codexInlineHookItemsFromToml(target, text);
}

function codexInlineHookItemsFromToml(target: ScanTarget, text: string): DiscoveredItem[] {
  const groups: Array<{
    eventName: string;
    matcher: string;
    hooks: Array<Record<string, unknown>>;
  }> = [];
  let currentGroup: typeof groups[number] | null = null;
  let currentHook: Record<string, unknown> | null = null;

  for (const rawLine of text.split(/\r?\n/)) {
    const line = stripTomlComment(rawLine).trim();
    if (!line) continue;

    const tableArray = /^\[\[([^\]]+)]]$/.exec(line);
    if (tableArray) {
      const sectionPath = splitTomlDottedName(tableArray[1]);
      if (sectionPath.length === 2 && sectionPath[0] === "hooks") {
        currentGroup = { eventName: sectionPath[1], matcher: "*", hooks: [] };
        currentHook = null;
        groups.push(currentGroup);
      } else if (sectionPath.length === 3 && sectionPath[0] === "hooks" && sectionPath[2] === "hooks") {
        if (!currentGroup || currentGroup.eventName !== sectionPath[1]) {
          currentGroup = { eventName: sectionPath[1], matcher: "*", hooks: [] };
          groups.push(currentGroup);
        }
        currentHook = {};
        currentGroup.hooks.push(currentHook);
      } else {
        currentGroup = null;
        currentHook = null;
      }
      continue;
    }

    const table = /^\[([^\]]+)]$/.exec(line);
    if (table) {
      currentGroup = null;
      currentHook = null;
      continue;
    }

    const match = /^([A-Za-z0-9_.-]+)\s*=\s*(.*)$/.exec(line);
    if (!match || !currentGroup) {
      continue;
    }

    const [, key, rawValue] = match;
    const parsed = secretLikePath([key]) ? "[redacted]" : parseTomlScalar(rawValue);
    if (currentHook) {
      currentHook[key] = parsed;
    } else if (key === "matcher" && typeof parsed === "string") {
      currentGroup.matcher = parsed;
    }
  }

  const hooks: Record<string, unknown[]> = {};
  for (const group of groups) {
    hooks[group.eventName] ??= [];
    hooks[group.eventName].push({
      matcher: group.matcher,
      hooks: group.hooks,
    });
  }
  const hookShape = { hooks };
  return codexHookItemsFromValue(target, hookShape);
}

async function scanCodexHooksFile(target: ScanTarget): Promise<DiscoveredItem[]> {
  let text;
  try {
    text = await readFile(target.absolutePath, "utf8");
  } catch {
    return [];
  }

  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch (error) {
    return [{
      id: itemId(target, "parse-failed"),
      agent: "codex",
      kind: "hook",
      sourcePath: target.sourcePath,
      scope: target.scope,
      precedence: target.precedence,
      parser: "json",
      sensitivity: target.sensitivity,
      contentPolicy: target.contentPolicy,
      restorePolicy: "structured_fields_only",
      captureStatus: "parse_failed",
      confidence: "high",
      metadata: { error: error instanceof Error ? error.message : "Invalid JSON" },
    }];
  }

  return codexHookItemsFromValue(target, value);
}

function codexHookItemsFromValue(target: ScanTarget, value: unknown): DiscoveredItem[] {
  const hooksValue = asObject(value)?.["hooks"];
  if (!isRecord(hooksValue)) {
    return [];
  }

  const evidence: DiscoveredItem[] = [];
  for (const [eventName, eventHooks] of Object.entries(hooksValue)) {
    if (!Array.isArray(eventHooks)) continue;
    for (let groupIndex = 0; groupIndex < eventHooks.length; groupIndex++) {
      const group = asObject(eventHooks[groupIndex]);
      if (!group) continue;
      const matcher = typeof group.matcher === "string" ? group.matcher : "*";
      const nestedHooks = group["hooks"];
      if (!Array.isArray(nestedHooks)) continue;

      for (let hookIndex = 0; hookIndex < nestedHooks.length; hookIndex++) {
        const hook = asObject(nestedHooks[hookIndex]);
        if (!hook) continue;
        const command = typeof hook.command === "string" ? hook.command : undefined;
        const type = typeof hook.type === "string" ? hook.type : "command";
        const timeout = typeof hook.timeout === "number" ? hook.timeout : undefined;
        const name = `${eventName}.${matcher}`;

        evidence.push({
          id: itemId(target, `hook-${eventName}-${groupIndex}-${hookIndex}`),
          agent: "codex",
          kind: "hook",
          sourcePath: target.sourcePath,
          scope: target.scope,
          precedence: target.precedence,
          parser: "json",
          sensitivity: "command_config",
          contentPolicy: "structured_safe_fields_only",
          restorePolicy: "structured_fields_only",
          captureStatus: "captured",
          confidence: "high",
          name,
          value: {
            type,
            ...(command ? { command } : {}),
            ...(timeout !== undefined ? { timeout } : {}),
          },
          metadata: {
            executable: type === "command" && Boolean(command),
            eventName,
            matcher,
            hookIndex,
            groupIndex,
            source: target.scope === "managed" ? "plugin" : target.scope,
          },
        });
      }
    }
  }

  return evidence;
}

async function scanCodexMcpServers(homeDir: string): Promise<DiscoveredItem[]> {
  const target = homeTarget(homeDir, ".codex/config.toml", "codex", "agent_config", "toml");
  let text;
  try {
    text = await readFile(target.absolutePath, "utf8");
  } catch {
    return [];
  }

  return [...codexMcpServersFromToml(text).entries()].map(([name, serverValue]) => ({
    id: itemId(target, `mcp-${name}`),
    agent: "codex",
    kind: "mcp_server",
    sourcePath: target.sourcePath,
    scope: target.scope,
    precedence: target.precedence,
    parser: "toml",
    sensitivity: "command_config",
    contentPolicy: "structured_safe_fields_only",
    restorePolicy: "structured_fields_only",
    captureStatus: "captured",
    confidence: "high",
    name,
    value: serverValue,
  }));
}

function codexMcpServersFromToml(text: string): Map<string, Record<string, unknown>> {
  const servers = new Map<string, Record<string, unknown>>();
  let currentServer: string | null = null;
  let currentNestedPath: string[] = [];

  const lines = text.split(/\r?\n/);
  for (let index = 0; index < lines.length; index++) {
    const line = stripTomlComment(lines[index]).trim();
    if (!line || line.startsWith("#")) {
      continue;
    }

    if (line.startsWith("[") && line.endsWith("]")) {
      const sectionPath = splitTomlDottedName(line.slice(1, -1));
      if (sectionPath[0] === "mcp_servers" && sectionPath[1]) {
        currentServer = sectionPath[1];
        currentNestedPath = sectionPath.slice(2);
        if (!servers.has(currentServer)) {
          servers.set(currentServer, {});
        }
      } else {
        currentServer = null;
        currentNestedPath = [];
      }
      continue;
    }

    if (!currentServer) {
      continue;
    }

    const match = /^([A-Za-z0-9_.-]+)\s*=\s*(.*)$/.exec(line);
    if (!match) {
      continue;
    }

    const [, key, initialRawValue] = match;
    let rawValue = initialRawValue.trim();
    if (rawValue.startsWith("[") && !completeTomlArray(rawValue)) {
      const arrayLines = [rawValue];
      while (++index < lines.length) {
        const continuationLine = stripTomlComment(lines[index]).trim();
        arrayLines.push(continuationLine);
        if (completeTomlArray(arrayLines.join(" "))) {
          break;
        }
      }
      rawValue = arrayLines.join(" ");
    }

    const server = servers.get(currentServer)!;
    const pathParts = [...currentNestedPath, ...splitTomlDottedName(key)];
    assignTomlValue(server, pathParts, rawValue);
  }

  return servers;
}

function assignTomlValue(target: Record<string, unknown>, pathParts: string[], rawValue: string): void {
  if (pathParts.length === 0) {
    return;
  }

  if (pathParts[0] === "env" && pathParts[1]) {
    const existing = Array.isArray(target.envKeys) ? target.envKeys.filter((item): item is string => typeof item === "string") : [];
    target.envKeys = [...new Set([...existing, pathParts[1]])];
    return;
  }

  let cursor = target;
  for (const part of pathParts.slice(0, -1)) {
    const next = cursor[part];
    if (!next || typeof next !== "object" || Array.isArray(next)) {
      cursor[part] = {};
    }
    cursor = cursor[part] as Record<string, unknown>;
  }

  const key = pathParts.at(-1)!;
  cursor[key] = secretLikePath(pathParts) ? "[redacted]" : parseTomlScalar(rawValue);
}

function stripTomlComment(rawLine: string): string {
  let quote: string | null = null;
  for (let index = 0; index < rawLine.length; index++) {
    const char = rawLine[index];
    const previous = rawLine[index - 1];
    if ((char === "\"" || char === "'") && quote === null) {
      quote = char;
      continue;
    }
    if (char === quote && previous !== "\\") {
      quote = null;
      continue;
    }
    if (char === "#" && quote === null) {
      return rawLine.slice(0, index);
    }
  }
  return rawLine;
}

function completeTomlArray(value: string): boolean {
  let quote: string | null = null;
  let depth = 0;

  for (let index = 0; index < value.length; index++) {
    const char = value[index];
    const previous = value[index - 1];
    if ((char === "\"" || char === "'") && quote === null) {
      quote = char;
      continue;
    }
    if (char === quote && previous !== "\\") {
      quote = null;
      continue;
    }
    if (quote !== null) {
      continue;
    }
    if (char === "[") depth++;
    if (char === "]") depth--;
  }

  return depth === 0;
}

function splitTomlDottedName(name: string): string[] {
  const parts: string[] = [];
  let current = "";
  let quote: string | null = null;

  for (const char of name) {
    if ((char === "\"" || char === "'") && quote === null) {
      quote = char;
      continue;
    }
    if (char === quote) {
      quote = null;
      continue;
    }
    if (char === "." && quote === null) {
      parts.push(current);
      current = "";
      continue;
    }
    current += char;
  }

  parts.push(current);
  return parts.map((part) => part.trim()).filter(Boolean);
}

function parseTomlScalar(rawValue: string): unknown {
  const value = rawValue.trim().replace(/,$/, "");
  if ((value.startsWith("\"") && value.endsWith("\"")) || (value.startsWith("'") && value.endsWith("'"))) {
    return value.slice(1, -1);
  }
  if (value === "true") return true;
  if (value === "false") return false;
  if (/^-?\d+(?:\.\d+)?$/.test(value)) return Number(value);
  if (value.startsWith("[") && value.endsWith("]")) {
    return value
      .slice(1, -1)
      .split(",")
      .map((entry) => parseTomlScalar(entry.trim()))
      .filter((entry) => entry !== "");
  }
  return value;
}

function secretLikePath(pathParts: string[]): boolean {
  return /(api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)/i.test(pathParts.join("."));
}

async function scanCodexSkillDirectory(target: ScanTarget): Promise<DiscoveredItem[]> {
  const skillFiles = await findSkillFiles(target.absolutePath);
  const evidence: DiscoveredItem[] = [];

  for (const skillFile of skillFiles) {
    const frontmatter = await readSkillFrontmatter(skillFile);
    const skillDir = path.dirname(skillFile);
    const relativeSkillDir = path.relative(target.absolutePath, skillDir).split(path.sep).join("/");
    const sourcePath = relativeSkillDir ? `${target.sourcePath}/${relativeSkillDir}` : target.sourcePath;
    const directoryName = path.basename(skillDir);
    const name = frontmatter?.name || directoryName;
    const entrypointStatus = await skillEntrypointStatus(target.absolutePath, skillFile);

    evidence.push({
      id: itemId({ ...target, sourcePath }, "skill"),
      agent: "codex",
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
      name,
      metadata: {
        present: true,
        entrypoint: path.basename(skillFile),
        entrypointStatus,
        ...(frontmatter?.sizeBytes ? { entrypointSizeBytes: frontmatter.sizeBytes } : {}),
        ...(frontmatter?.name ? { declaredName: frontmatter.name } : {}),
        directoryName,
        nameMatchesDirectory: name === directoryName,
        ...(frontmatter?.description ? { description: frontmatter.description } : {}),
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

async function walkSkillFiles(
  dir: string,
  files: string[],
  depth: number,
  seen: Set<string>
): Promise<void> {
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
    let stats;
    try {
      stats = await stat(absolutePath);
    } catch {
      continue;
    }

    if (stats.isDirectory()) {
      await walkSkillFiles(absolutePath, files, depth + 1, seen);
      continue;
    }

    if (stats.isFile() && entry.name.toLowerCase() === "skill.md") {
      files.push(absolutePath);
    }
  }
}

async function readSkillFrontmatter(filePath: string): Promise<{ name?: string; description?: string; sizeBytes: number } | null> {
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

  const frontmatter: { name?: string; description?: string; sizeBytes: number } = { sizeBytes: stats.size };
  for (const line of match[1].split(/\r?\n/)) {
    const field = /^(name|description):\s*(.*)$/.exec(line.trim());
    if (!field) {
      continue;
    }
    frontmatter[field[1] as "name" | "description"] = field[2].replace(/^['"]|['"]$/g, "");
  }
  return frontmatter;
}

async function skillEntrypointStatus(root: string, skillFile: string): Promise<string> {
  const relativeParts = path.relative(root, skillFile).split(path.sep);
  let cursor = root;

  for (const part of relativeParts) {
    cursor = path.join(cursor, part);
    try {
      if ((await lstat(cursor)).isSymbolicLink()) {
        return part.toLowerCase() === "skill.md" ? "symlink_followed" : "symlink_directory_followed";
      }
    } catch {
      return "captured";
    }
  }

  return "captured";
}

function dedupeSkillsBySource(evidence: DiscoveredItem[]): DiscoveredItem[] {
  const seen = new Set<string>();
  const result: DiscoveredItem[] = [];

  for (const item of evidence) {
    if (seen.has(item.sourcePath)) {
      continue;
    }
    seen.add(item.sourcePath);
    result.push(item);
  }

  return result;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function asObject(value: unknown): Record<string, unknown> | null {
  return isRecord(value) ? value : null;
}

function itemId(target: ScanTarget, suffix: string): string {
  return `${target.scope}.${target.agent}.${target.sourcePath}.${suffix}`
    .replace(/^~\//, "home/")
    .replace(/[^A-Za-z0-9_.-]+/g, ".")
    .replace(/^\.+|\.+$/g, "")
    .toLowerCase();
}
