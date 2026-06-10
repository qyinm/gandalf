import { lstat, readdir, readFile, realpath, stat } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";

import type { DiscoveredItem, EvidenceScope } from "../types.js";
import type { ScanTarget } from "./index.js";
import { homeTarget, projectTarget } from "./index.js";
import type { ScannerPlugin } from "./scanner-plugin.js";
import { scanTargets } from "./filesystem.js";
import { arrayOfStrings, metadataStringArray, scannerItemId } from "./base.js";

interface PiSkillTarget {
  absolutePath: string;
  sourcePath: string;
  scope: EvidenceScope;
  precedence: number;
  includeRootFiles: boolean;
  source: string;
}

interface PiSkillFile {
  filePath: string;
  skillDir: string;
  root: string;
}

interface PiExtensionTarget {
  absolutePath: string;
  sourcePath: string;
  scope: EvidenceScope;
  precedence: number;
  source: string;
}

interface PiExtensionFile {
  filePath: string;
  root: string;
}

interface PiFrontmatter {
  name?: string;
  description?: string;
  disableModelInvocation?: boolean;
  sizeBytes: number;
}

export const piAgentScanner: ScannerPlugin = {
  agentId: "pi-agent",
  agentName: "Pi Agent",
  description: "Pi coding agent configuration (settings, models, agents, extensions, skills, themes, prompts)",

  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, ".pi/settings.json", "pi-agent", "agent_config", "json"),
      projectTarget(projectPath, ".pi/themes", "pi-agent", "unsupported", "filesystem", {
        directory: true,
        sensitivity: "themes",
      }),
      projectTarget(projectPath, ".pi/prompts", "pi-agent", "agent_instruction", "filesystem", {
        directory: true,
        sensitivity: "prompt_templates",
      }),
      homeTarget(homeDir, ".pi/agent/settings.json", "pi-agent", "agent_config", "json"),
      homeTarget(homeDir, ".pi/agent/models.json", "pi-agent", "agent_config", "json", {
        metadataOnly: true,
        sensitivity: "model_registry",
      }),
      homeTarget(homeDir, ".pi/agents", "pi-agent", "unsupported", "filesystem", {
        directory: true,
        sensitivity: "custom_agents",
      }),
      homeTarget(homeDir, ".pi/agent/themes", "pi-agent", "unsupported", "filesystem", {
        directory: true,
        sensitivity: "themes",
      }),
      homeTarget(homeDir, ".pi/agent/prompts", "pi-agent", "agent_instruction", "filesystem", {
        directory: true,
        sensitivity: "prompt_templates",
      }),
    ];
  },

  async scan({ projectPath, homeDir }): Promise<DiscoveredItem[]> {
    const configEvidence = await scanTargets(this.targets(projectPath, homeDir));
    const extensionEvidence: DiscoveredItem[] = [];
    const skillEvidence: DiscoveredItem[] = [];

    for (const target of await piExtensionTargets(projectPath, homeDir)) {
      extensionEvidence.push(...await scanPiExtensionTarget(target));
    }

    for (const target of await piSkillTargets(projectPath, homeDir)) {
      skillEvidence.push(...await scanPiSkillTarget(target));
    }

    return [
      ...configEvidence,
      ...dedupePiExtensions(extensionEvidence),
      ...dedupePiSkills(skillEvidence),
    ];
  },
};

async function piExtensionTargets(projectPath: string, homeDir: string): Promise<PiExtensionTarget[]> {
  const targets: PiExtensionTarget[] = [
    makePiExtensionTarget(homeDir, ".pi/agent/extensions", "user", 10, "auto"),
    makePiExtensionTarget(projectPath, ".pi/extensions", "project", 40, "auto"),
  ];

  targets.push(...await configuredExtensionTargets(projectPath, homeDir));
  targets.push(...await packageExtensionTargets(projectPath, homeDir));

  return targets;
}

function makePiExtensionTarget(
  root: string,
  relativePath: string,
  scope: EvidenceScope,
  precedence: number,
  source: string
): PiExtensionTarget {
  return {
    absolutePath: join(root, relativePath),
    sourcePath: scope === "user" ? `~/${relativePath}` : relativePath,
    scope,
    precedence,
    source,
  };
}

async function piSkillTargets(projectPath: string, homeDir: string): Promise<PiSkillTarget[]> {
  const targets: PiSkillTarget[] = [
    makePiSkillTarget(homeDir, ".pi/agent/skills", "user", 10, true, "pi"),
    makePiSkillTarget(projectPath, ".pi/skills", "project", 40, true, "pi"),
    makePiSkillTarget(homeDir, ".agents/skills", "user", 15, false, "agents"),
    ...ancestorAgentSkillTargets(projectPath),
  ];

  targets.push(...await configuredSkillTargets(projectPath, homeDir));
  targets.push(...await packageSkillTargets(projectPath, homeDir));

  return targets;
}

function makePiSkillTarget(
  root: string,
  relativePath: string,
  scope: EvidenceScope,
  precedence: number,
  includeRootFiles: boolean,
  source: string
): PiSkillTarget {
  return {
    absolutePath: join(root, relativePath),
    sourcePath: scope === "user" ? `~/${relativePath}` : relativePath,
    scope,
    precedence,
    includeRootFiles,
    source,
  };
}

function ancestorAgentSkillTargets(projectPath: string): PiSkillTarget[] {
  const targets: PiSkillTarget[] = [];
  const repoRoot = findGitRepoRoot(projectPath);
  let dir = resolve(projectPath);

  while (true) {
    targets.push({
      absolutePath: join(dir, ".agents", "skills"),
      sourcePath: relative(projectPath, join(dir, ".agents", "skills")).split(sep).join("/") || ".agents/skills",
      scope: "project",
      precedence: 35,
      includeRootFiles: false,
      source: "agents",
    });

    if ((repoRoot && dir === repoRoot) || dirname(dir) === dir) {
      break;
    }
    dir = dirname(dir);
  }

  return targets;
}

function findGitRepoRoot(startDir: string): string | null {
  let dir = resolve(startDir);
  while (true) {
    if (existsSync(join(dir, ".git"))) {
      return dir;
    }
    const parent = dirname(dir);
    if (parent === dir) {
      return null;
    }
    dir = parent;
  }
}

async function configuredExtensionTargets(projectPath: string, homeDir: string): Promise<PiExtensionTarget[]> {
  const targets: PiExtensionTarget[] = [];
  const settings = [
    { path: join(homeDir, ".pi/agent/settings.json"), baseDir: join(homeDir, ".pi/agent"), scope: "user" as const, precedence: 20 },
    { path: join(projectPath, ".pi/settings.json"), baseDir: join(projectPath, ".pi"), scope: "project" as const, precedence: 50 },
  ];

  for (const setting of settings) {
    const value = await readJsonObject(setting.path);
    for (const rawPath of arrayOfStrings(value?.["extensions"])) {
      const absolutePath = resolveConfiguredPath(rawPath, setting.baseDir, homeDir);
      targets.push({
        absolutePath,
        sourcePath: displayPath(absolutePath, homeDir, projectPath),
        scope: setting.scope,
        precedence: setting.precedence,
        source: "settings",
      });
    }
  }

  return targets;
}

async function configuredSkillTargets(projectPath: string, homeDir: string): Promise<PiSkillTarget[]> {
  const targets: PiSkillTarget[] = [];
  const settings = [
    { path: join(homeDir, ".pi/agent/settings.json"), baseDir: join(homeDir, ".pi/agent"), scope: "user" as const, precedence: 20 },
    { path: join(projectPath, ".pi/settings.json"), baseDir: join(projectPath, ".pi"), scope: "project" as const, precedence: 50 },
  ];

  for (const setting of settings) {
    const value = await readJsonObject(setting.path);
    const paths = arrayOfStrings(value?.["skills"]);
    for (const rawPath of paths) {
      const absolutePath = resolveConfiguredPath(rawPath, setting.baseDir, homeDir);
      targets.push({
        absolutePath,
        sourcePath: displayPath(absolutePath, homeDir, projectPath),
        scope: setting.scope,
        precedence: setting.precedence,
        includeRootFiles: true,
        source: "settings",
      });
    }
  }

  return targets;
}

async function packageExtensionTargets(projectPath: string, homeDir: string): Promise<PiExtensionTarget[]> {
  const targets: PiExtensionTarget[] = [];
  const settings = [
    { path: join(homeDir, ".pi/agent/settings.json"), scope: "user" as const, precedence: 25 },
    { path: join(projectPath, ".pi/settings.json"), scope: "project" as const, precedence: 55 },
  ];

  for (const setting of settings) {
    const value = await readJsonObject(setting.path);
    for (const spec of arrayOfStrings(value?.["packages"])) {
      const packageRoot = resolvePackageRoot(spec);
      if (!packageRoot) {
        continue;
      }

      const packageJson = await readJsonObject(join(packageRoot, "package.json"));
      const piValue = packageJson?.["pi"];
      const piConfig = isObject(piValue) ? piValue as Record<string, unknown> : {};
      const extensionPaths = arrayOfStrings(piConfig["extensions"]);
      const rawPaths = extensionPaths.length > 0 ? extensionPaths : ["extensions"];

      for (const rawPath of rawPaths) {
        const absolutePath = resolveConfiguredPath(rawPath, packageRoot, homeDir);
        targets.push({
          absolutePath,
          sourcePath: displayPath(absolutePath, homeDir, projectPath),
          scope: setting.scope,
          precedence: setting.precedence,
          source: "package",
        });
      }
    }
  }

  return targets;
}

async function packageSkillTargets(projectPath: string, homeDir: string): Promise<PiSkillTarget[]> {
  const targets: PiSkillTarget[] = [];
  const settings = [
    { path: join(homeDir, ".pi/agent/settings.json"), scope: "user" as const, precedence: 25 },
    { path: join(projectPath, ".pi/settings.json"), scope: "project" as const, precedence: 55 },
  ];

  for (const setting of settings) {
    const value = await readJsonObject(setting.path);
    for (const spec of arrayOfStrings(value?.["packages"])) {
      const packageRoot = resolvePackageRoot(spec);
      if (!packageRoot) {
        continue;
      }

      const packageJson = await readJsonObject(join(packageRoot, "package.json"));
      const piValue = packageJson?.["pi"];
      const piConfig = isObject(piValue) ? piValue as Record<string, unknown> : {};
      const skillPaths = arrayOfStrings(piConfig["skills"]);
      const rawPaths = skillPaths.length > 0 ? skillPaths : ["skills"];

      for (const rawPath of rawPaths) {
        const absolutePath = resolveConfiguredPath(rawPath, packageRoot, homeDir);
        targets.push({
          absolutePath,
          sourcePath: displayPath(absolutePath, homeDir, projectPath),
          scope: setting.scope,
          precedence: setting.precedence,
          includeRootFiles: true,
          source: "package",
        });
      }
    }
  }

  return targets;
}

function resolvePackageRoot(spec: string): string | null {
  const packageName = packageNameFromSpec(spec);
  if (!packageName) {
    return null;
  }

  for (const root of nodeModuleRoots()) {
    const packageRoot = join(root, packageName);
    if (existsSync(join(packageRoot, "package.json"))) {
      return packageRoot;
    }
  }

  return null;
}

function packageNameFromSpec(spec: string): string | null {
  let value = spec;
  if (value.startsWith("npm:")) {
    value = value.slice(4);
  }
  if (value.startsWith("@")) {
    const [scope, name] = value.split("/");
    return scope && name ? `${scope}/${name.split("@")[0]}` : null;
  }
  return value.split("@")[0] || null;
}

function nodeModuleRoots(): string[] {
  const roots = [
    join(dirname(process.execPath), "..", "lib", "node_modules"),
    "/opt/homebrew/lib/node_modules",
    "/usr/local/lib/node_modules",
  ];
  return [...new Set(roots.map((root) => resolve(root)))];
}

async function scanPiExtensionTarget(target: PiExtensionTarget): Promise<DiscoveredItem[]> {
  const extensionFiles = await findPiExtensionFiles(target.absolutePath);
  const evidence: DiscoveredItem[] = [];

  for (const extensionFile of extensionFiles) {
    let stats;
    let realFilePath: string | undefined;
    try {
      stats = await stat(extensionFile.filePath);
      realFilePath = await realpath(extensionFile.filePath);
    } catch {
      continue;
    }

    const sourcePath = displayExtensionSourcePath(target, extensionFile);
    const entrypoint = dirnameBasename(extensionFile.filePath);
    const isIndexEntrypoint = entrypoint === "index.ts" || entrypoint === "index.js";

    evidence.push({
      id: scannerItemId(target.scope, "pi-agent", sourcePath, "extension"),
      agent: "pi-agent",
      kind: "extension",
      sourcePath,
      scope: target.scope,
      precedence: target.precedence,
      parser: "filesystem",
      sensitivity: "command_config",
      contentPolicy: "metadata_only",
      restorePolicy: "full_content_supported",
      captureStatus: "captured",
      confidence: "high",
      name: extensionNameFromPath(extensionFile.filePath, extensionFile.root),
      metadata: {
        present: true,
        source: target.source,
        executable: true,
        entrypoint,
        extensionStyle: isIndexEntrypoint ? "directory_index" : "single_file",
        sizeBytes: stats.size,
        realPath: realFilePath,
      },
    });
  }

  return evidence;
}

async function findPiExtensionFiles(root: string): Promise<PiExtensionFile[]> {
  let stats;
  try {
    stats = await stat(root);
  } catch {
    return [];
  }

  if (stats.isFile()) {
    return isExtensionFile(root) ? [{ filePath: root, root }] : [];
  }
  if (!stats.isDirectory()) {
    return [];
  }
  return collectPiExtensionEntries(root, root);
}

async function collectPiExtensionEntries(dir: string, root: string): Promise<PiExtensionFile[]> {
  const manifestEntries = await resolvePiExtensionEntries(dir, root);
  if (manifestEntries.length > 0) {
    return manifestEntries;
  }

  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return [];
  }

  const discovered: PiExtensionFile[] = [];
  for (const entry of entries) {
    if (entry.name.startsWith(".") || entry.name === "node_modules") {
      continue;
    }

    const entryPath = join(dir, entry.name);
    let stats;
    try {
      stats = await stat(entryPath);
    } catch {
      continue;
    }

    if (stats.isFile() && isExtensionFile(entryPath)) {
      discovered.push({ filePath: entryPath, root });
    } else if (stats.isDirectory()) {
      discovered.push(...await resolvePiExtensionEntries(entryPath, root));
    }
  }

  return discovered;
}

async function resolvePiExtensionEntries(dir: string, root: string): Promise<PiExtensionFile[]> {
  const packageJson = await readJsonObject(join(dir, "package.json"));
  const piValue = packageJson?.["pi"];
  const piConfig = isObject(piValue) ? piValue as Record<string, unknown> : {};
  const manifestExtensions = arrayOfStrings(piConfig["extensions"]);
  const files: PiExtensionFile[] = [];

  for (const rawPath of manifestExtensions) {
    const resolvedPath = resolve(dir, rawPath);
    files.push(...await findPiExtensionFilesFromManifestPath(resolvedPath, root));
  }
  if (files.length > 0) {
    return files;
  }

  for (const indexFile of [join(dir, "index.ts"), join(dir, "index.js")]) {
    try {
      if ((await stat(indexFile)).isFile()) {
        return [{ filePath: indexFile, root }];
      }
    } catch {
      // Keep checking candidates.
    }
  }

  return [];
}

async function findPiExtensionFilesFromManifestPath(absolutePath: string, root: string): Promise<PiExtensionFile[]> {
  let stats;
  try {
    stats = await stat(absolutePath);
  } catch {
    return [];
  }

  if (stats.isFile()) {
    return isExtensionFile(absolutePath) ? [{ filePath: absolutePath, root }] : [];
  }
  if (stats.isDirectory()) {
    return collectPiExtensionEntries(absolutePath, root);
  }
  return [];
}

function isExtensionFile(filePath: string): boolean {
  return filePath.endsWith(".ts") || filePath.endsWith(".js");
}

function extensionNameFromPath(filePath: string, root: string): string {
  const entrypoint = dirnameBasename(filePath);
  if (entrypoint === "index.ts" || entrypoint === "index.js") {
    if (existsSync(join(root, "package.json"))) {
      return dirnameBasename(root);
    }
    return dirnameBasename(dirname(filePath));
  }
  return entrypoint.replace(/\.(ts|js)$/, "");
}

async function scanPiSkillTarget(target: PiSkillTarget): Promise<DiscoveredItem[]> {
  const skillFiles = await findPiSkillFiles(target.absolutePath, target.includeRootFiles);
  const evidence: DiscoveredItem[] = [];

  for (const skillFile of skillFiles) {
    const frontmatter = await readSkillFrontmatter(skillFile.filePath);
    if (!frontmatter?.description?.trim()) {
      continue;
    }

    const name = frontmatter.name || dirnameBasename(skillFile.skillDir);
    const sourcePath = displaySkillSourcePath(target, skillFile);
    const entrypointStatus = await skillEntrypointStatus(target.absolutePath, skillFile.filePath);

    evidence.push({
      id: scannerItemId(target.scope, "pi-agent", sourcePath, "skill"),
      agent: "pi-agent",
      kind: "skill",
      sourcePath,
      scope: target.scope,
      precedence: target.precedence,
      parser: "filesystem",
      sensitivity: "metadata",
      contentPolicy: "metadata_only",
      restorePolicy: "full_content_supported",
      captureStatus: "captured",
      confidence: "high",
      name,
      metadata: {
        present: true,
        source: target.source,
        entrypoint: skillFile.filePath.endsWith("/SKILL.md") ? "SKILL.md" : dirnameBasename(skillFile.filePath),
        entrypointStatus,
        entrypointSizeBytes: frontmatter.sizeBytes,
        declaredName: frontmatter.name,
        directoryName: dirnameBasename(skillFile.skillDir),
        nameMatchesDirectory: name === dirnameBasename(skillFile.skillDir),
        description: frontmatter.description,
        disableModelInvocation: frontmatter.disableModelInvocation === true,
      },
    });
  }

  return evidence;
}

async function findPiSkillFiles(root: string, includeRootFiles: boolean): Promise<PiSkillFile[]> {
  const files: PiSkillFile[] = [];
  await walkPiSkillFiles(root, root, includeRootFiles, files, new Set());
  return files;
}

async function walkPiSkillFiles(
  dir: string,
  root: string,
  includeRootFiles: boolean,
  files: PiSkillFile[],
  seen: Set<string>
): Promise<void> {
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

  const entrypoint = entries.find((entry) => entry.name === "SKILL.md");
  if (entrypoint) {
    const filePath = join(dir, entrypoint.name);
    try {
      if ((await stat(filePath)).isFile()) {
        files.push({ filePath, skillDir: dir, root });
        return;
      }
    } catch {
      return;
    }
  }

  for (const entry of entries) {
    if (entry.name.startsWith(".") || entry.name === "node_modules") {
      continue;
    }

    const filePath = join(dir, entry.name);
    let stats;
    try {
      stats = await stat(filePath);
    } catch {
      continue;
    }

    if (stats.isDirectory()) {
      await walkPiSkillFiles(filePath, root, false, files, seen);
      continue;
    }

    if (includeRootFiles && stats.isFile() && entry.name.endsWith(".md")) {
      files.push({ filePath, skillDir: dirname(filePath), root });
    }
  }
}

async function readSkillFrontmatter(filePath: string): Promise<PiFrontmatter | null> {
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

  const frontmatter: PiFrontmatter = { sizeBytes: stats.size };
  for (const line of match[1].split("\n")) {
    const field = /^(name|description|disable-model-invocation):\s*(.*)$/.exec(line.trim());
    if (!field) {
      continue;
    }
    const key = field[1];
    const value = field[2].replace(/^['"]|['"]$/g, "");
    if (key === "disable-model-invocation") {
      frontmatter.disableModelInvocation = value === "true";
    } else {
      frontmatter[key as "name" | "description"] = value;
    }
  }
  return frontmatter;
}

async function skillEntrypointStatus(root: string, skillFile: string): Promise<string> {
  const relativeParts = relative(root, skillFile).split(sep);
  let cursor = root;

  for (const part of relativeParts) {
    cursor = join(cursor, part);
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

function dedupePiExtensions(evidence: DiscoveredItem[]): DiscoveredItem[] {
  const result: DiscoveredItem[] = [];
  const realPaths = new Set<string>();

  for (const item of evidence) {
    const realPath = typeof item.metadata?.["realPath"] === "string" ? item.metadata["realPath"] : undefined;
    if (realPath && realPaths.has(realPath)) {
      continue;
    }

    result.push(item);
    if (realPath) {
      realPaths.add(realPath);
    }
  }

  return result;
}

function dedupePiSkills(evidence: DiscoveredItem[]): DiscoveredItem[] {
  const result: DiscoveredItem[] = [];
  const skillIndexes = new Map<string, number>();
  const realPaths = new Set<string>();

  for (const item of evidence) {
    const realPath = typeof item.metadata?.["realPath"] === "string" ? item.metadata["realPath"] : undefined;
    if (realPath && realPaths.has(realPath)) {
      continue;
    }

    const name = item.name;
    if (!name) {
      result.push(item);
      continue;
    }

    const existingIndex = skillIndexes.get(name);
    if (existingIndex === undefined) {
      skillIndexes.set(name, result.length);
      result.push(item);
      if (realPath) {
        realPaths.add(realPath);
      }
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

function displayExtensionSourcePath(target: PiExtensionTarget, extensionFile: PiExtensionFile): string {
  const relativePath = relative(target.absolutePath, extensionFile.filePath).split(sep).join("/");
  if (!relativePath) {
    return target.sourcePath;
  }
  return `${target.sourcePath}/${relativePath}`;
}

function displaySkillSourcePath(target: PiSkillTarget, skillFile: PiSkillFile): string {
  const relativePath = relative(target.absolutePath, skillFile.filePath).split(sep).join("/");
  if (!relativePath || relativePath === "SKILL.md") {
    return target.sourcePath;
  }
  if (relativePath.endsWith("/SKILL.md")) {
    return `${target.sourcePath}/${relativePath.slice(0, -"/SKILL.md".length)}`;
  }
  return `${target.sourcePath}/${relativePath}`;
}

async function readJsonObject(filePath: string): Promise<Record<string, unknown> | null> {
  try {
    const value = JSON.parse(await readFile(filePath, "utf8"));
    return isObject(value) ? value as Record<string, unknown> : null;
  } catch {
    return null;
  }
}

function resolveConfiguredPath(rawPath: string, baseDir: string, homeDir: string): string {
  if (rawPath === "~") {
    return homeDir;
  }
  if (rawPath.startsWith("~/")) {
    return join(homeDir, rawPath.slice(2));
  }
  return isAbsolute(rawPath) ? rawPath : resolve(baseDir, rawPath);
}

function displayPath(absolutePath: string, homeDir: string, projectPath: string): string {
  const resolved = resolve(absolutePath);
  const resolvedHome = resolve(homeDir);
  const resolvedProject = resolve(projectPath);

  if (resolved === resolvedHome || resolved.startsWith(`${resolvedHome}${sep}`)) {
    return `~/${relative(resolvedHome, resolved).split(sep).join("/")}`;
  }
  if (resolved === resolvedProject) {
    return ".";
  }
  if (resolved.startsWith(`${resolvedProject}${sep}`)) {
    return relative(resolvedProject, resolved).split(sep).join("/");
  }
  return resolved;
}

function dirnameBasename(filePath: string): string {
  const parts = filePath.split(sep).filter(Boolean);
  return parts.at(-1) ?? filePath;
}

function isObject(value: unknown): boolean {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
