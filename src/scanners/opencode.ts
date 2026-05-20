import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { homeTarget, projectTarget } from "./index.js";
import { scanTargets } from "./filesystem.js";
import type { DiscoveredItem } from "../types.js";
import { lstat, readdir, readFile, realpath, stat } from "node:fs/promises";
import path from "node:path";

export const opencodeScanner: ScannerPlugin = {
  agentId: "opencode",
  agentName: "OpenCode",
  description: "OpenCode CLI configuration (MCP servers, plugins, providers, skills)",

  targets(_projectPath: string, homeDir: string): ScanTarget[] {
    return [
      // Main config: MCP servers, plugins, providers
      homeTarget(homeDir, ".config/opencode/opencode.json", "opencode", "agent_config", "json"),
    ];
  },

  async scan({ projectPath, homeDir }): Promise<DiscoveredItem[]> {
    const configEvidence = await scanTargets(this.targets(projectPath, homeDir));
    const skillEvidence: DiscoveredItem[] = [builtinCustomizeOpenCodeSkill()];

    for (const target of opencodeSkillTargets(projectPath, homeDir)) {
      skillEvidence.push(...await scanOpenCodeSkillDirectory(target));
    }

    return [
      ...configEvidence,
      ...dedupeSkillsByName(skillEvidence),
    ];
  },
};

function opencodeSkillTargets(projectPath: string, homeDir: string): ScanTarget[] {
  return [
    // Native OpenCode skills
    projectTarget(projectPath, ".opencode/skills", "opencode", "skill", "filesystem", {
      directory: true,
    }),
    homeTarget(homeDir, ".config/opencode/skills", "opencode", "skill", "filesystem", {
      directory: true,
    }),
    // OpenCode runtime also accepts the singular form.
    projectTarget(projectPath, ".opencode/skill", "opencode", "skill", "filesystem", {
      directory: true,
    }),
    homeTarget(homeDir, ".config/opencode/skill", "opencode", "skill", "filesystem", {
      directory: true,
    }),

    // OpenCode also discovers Claude-compatible and agent-compatible skills.
    projectTarget(projectPath, ".claude/skills", "opencode", "skill", "filesystem", {
      directory: true,
    }),
    homeTarget(homeDir, ".claude/skills", "opencode", "skill", "filesystem", {
      directory: true,
    }),
    projectTarget(projectPath, ".agents/skills", "opencode", "skill", "filesystem", {
      directory: true,
    }),
    homeTarget(homeDir, ".agents/skills", "opencode", "skill", "filesystem", {
      directory: true,
    }),

    // Plugin-provided skills are installed under OpenCode's package cache.
    homeTarget(homeDir, ".cache/opencode/packages", "opencode", "skill", "filesystem", {
      directory: true,
    }),
  ];
}

async function scanOpenCodeSkillDirectory(target: ScanTarget): Promise<DiscoveredItem[]> {
  const skillFiles = await findSkillFiles(target.absolutePath);
  const evidence: DiscoveredItem[] = [];

  for (const skillFile of skillFiles) {
    const frontmatter = await readSkillFrontmatter(skillFile);
    if (!frontmatter?.name || !frontmatter.description) {
      continue;
    }

    const skillDir = path.dirname(skillFile);
    const directoryName = path.basename(skillDir);
    if (!validSkillName(frontmatter.name)) {
      continue;
    }

    const relativeSkillDir = path.relative(target.absolutePath, skillDir).split(path.sep).join("/");
    const sourcePath = `${target.sourcePath}/${relativeSkillDir}`;
    const entrypointStatus = await skillEntrypointStatus(target.absolutePath, skillFile);

    evidence.push({
      id: itemId({ ...target, sourcePath }, "skill"),
      agent: target.agent,
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
        entrypoint: "SKILL.md",
        entrypointStatus,
        entrypointSizeBytes: frontmatter.sizeBytes,
        declaredName: frontmatter.name,
        directoryName,
        nameMatchesDirectory: frontmatter.name === directoryName,
        description: frontmatter.description,
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
    resolved = await realpathSafe(dir);
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

async function realpathSafe(filePath: string): Promise<string> {
  return await realpath(filePath);
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

  const match = /^---\n([\s\S]*?)\n---/.exec(text);
  if (!match) {
    return null;
  }

  const frontmatter: { name?: string; description?: string; sizeBytes: number } = { sizeBytes: stats.size };
  for (const line of match[1].split("\n")) {
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
        return part === "SKILL.md" ? "symlink_followed" : "symlink_directory_followed";
      }
    } catch {
      return "captured";
    }
  }

  return "captured";
}

function validSkillName(name: string): boolean {
  return /^[a-z0-9]+(-[a-z0-9]+)*$/.test(name) && name.length <= 64;
}

function builtinCustomizeOpenCodeSkill(): DiscoveredItem {
  return {
    id: "managed.opencode.built-in.customize-opencode.skill",
    agent: "opencode",
    kind: "skill",
    sourcePath: "<built-in>",
    scope: "managed",
    precedence: 100,
    parser: "filesystem",
    sensitivity: "metadata",
    contentPolicy: "metadata_only",
    restorePolicy: "not_supported",
    captureStatus: "captured",
    confidence: "high",
    name: "customize-opencode",
    metadata: {
      present: true,
      builtIn: true,
      declaredName: "customize-opencode",
      description: "Use when editing or creating opencode configuration, agents, skills, plugins, MCP servers, or permission rules.",
    },
  };
}

function itemId(target: ScanTarget, suffix: string): string {
  return `${target.scope}.${target.agent}.${target.sourcePath}.${suffix}`
    .replace(/^~\//, "home/")
    .replace(/[^A-Za-z0-9_.-]+/g, ".")
    .replace(/^\.+|\.+$/g, "")
    .toLowerCase();
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

  return result;
}

function metadataStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}
