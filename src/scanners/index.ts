/**
 * Scanner Plugin Interface.
 *
 * Each AI coding agent implements this interface to declare which files
 * and directories snaptailor should scan for that agent's configuration.
 *
 * To add support for a new agent:
 *   1. Create a new file in src/scanners/<agent-name>.ts
 *   2. Implement the ScannerPlugin interface
 *   3. Register it in the default registry in this file
 *   4. Add the agentId to the AgentId type in types.ts if new
 */

import type { AgentId, DiscoveredItem, EvidenceKind, EvidenceScope } from "../types.js";

/**
 * A single path/pattern to scan for a specific agent.
 */
export interface ScanTarget {
  /** Absolute filesystem path */
  absolutePath: string;
  /** Display path (relative or ~/-prefixed) */
  sourcePath: string;
  /** Whether this config lives in user space or project space */
  scope: EvidenceScope;
  /** Which agent owns this data */
  agent: AgentId;
  /** What kind of evidence to expect */
  kind: EvidenceKind;
  /** Parser to use */
  parser: DiscoveredItem["parser"];
  /** Precedence: 40 = project, 10 = user */
  precedence: number;
  /** Sensitivity label */
  sensitivity: string;
  /** Content policy label */
  contentPolicy: string;
  /** If true, this is a directory scan rather than a file read */
  directory?: boolean;
  /** If true, only check existence/metadata, don't parse contents */
  metadataOnly?: boolean;
}

/**
 * A scanner plugin for a single AI coding agent.
 */
export interface ScannerPlugin {
  /** Agent identifier (must match AgentId type) */
  readonly agentId: AgentId;
  /** Human-readable name */
  readonly agentName: string;
  /** Description of what this scanner covers */
  readonly description: string;
  /**
   * Return the list of scan targets for this agent.
   * @param projectPath Absolute path to the project directory
   * @param homeDir Absolute path to the user's home directory
   */
  targets(projectPath: string, homeDir: string): ScanTarget[];
}

// ── Helper factory ──────────────────────────────────────────────

/**
 * Build a ScanTarget for a path inside the project directory.
 */
export function projectTarget(
  projectPath: string,
  relativePath: string,
  agent: AgentId,
  kind: EvidenceKind,
  parser: DiscoveredItem["parser"],
  overrides: Partial<ScanTarget> = {}
): ScanTarget {
  return makeTarget(projectPath, relativePath, "project", 40, agent, kind, parser, overrides);
}

/**
 * Build a ScanTarget for a path inside the user's home directory.
 */
export function homeTarget(
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
    absolutePath: root + "/" + relativePath,
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

// ── Default plugin registry ─────────────────────────────────────

/**
 * The default set of scanner plugins shipped with snaptailor.
 * Import this in scan.ts to collect all scan targets.
 */
export function defaultScannerPlugins(): ScannerPlugin[] {
  return [
    new ClaudeCodeScanner(),
    new CodexScanner(),
    new CursorScanner(),
    new ProjectScanner(),
  ];
}

// ── Scanner implementations ─────────────────────────────────────

/**
 * Claude Code scanner.
 * Covers: CLAUDE.md, .mcp.json, .claude/settings.json
 * User: ~/.claude/settings.json, ~/.claude.json, ~/.claude/agents/, ~/.claude/skills/
 */
class ClaudeCodeScanner implements ScannerPlugin {
  readonly agentId: AgentId = "claude-code";
  readonly agentName = "Claude Code";
  readonly description = "Claude Code agent configuration (prompts, MCP servers, settings, skills)";

  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, "CLAUDE.md", "claude-code", "agent_instruction", "markdown"),
      projectTarget(projectPath, ".mcp.json", "claude-code", "agent_config", "json"),
      projectTarget(projectPath, ".claude/settings.json", "claude-code", "agent_config", "json"),
      homeTarget(homeDir, ".claude/settings.json", "claude-code", "agent_config", "json"),
      homeTarget(homeDir, ".claude.json", "claude-code", "agent_config", "json", {
        metadataOnly: true,
        sensitivity: "metadata"
      }),
      homeTarget(homeDir, ".claude/agents", "claude-code", "unsupported", "filesystem", { directory: true }),
      homeTarget(homeDir, ".claude/skills", "claude-code", "skill", "filesystem", { directory: true }),
    ];
  }
}

/**
 * Codex scanner.
 * Covers: AGENTS.md, .codex/
 * User: ~/.codex/config.toml, ~/.codex/skills/
 */
class CodexScanner implements ScannerPlugin {
  readonly agentId: AgentId = "codex";
  readonly agentName = "Codex";
  readonly description = "Codex agent configuration (prompts, config, skills)";

  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, "AGENTS.md", "codex", "agent_instruction", "markdown"),
      projectTarget(projectPath, ".codex", "codex", "unsupported", "filesystem", { directory: true }),
      homeTarget(homeDir, ".codex/config.toml", "codex", "agent_config", "toml"),
      homeTarget(homeDir, ".codex/skills", "codex", "skill", "filesystem", { directory: true }),
    ];
  }
}

/**
 * Cursor scanner.
 * Covers: .cursor/mcp.json (project + user)
 */
class CursorScanner implements ScannerPlugin {
  readonly agentId: AgentId = "cursor";
  readonly agentName = "Cursor";
  readonly description = "Cursor editor MCP server configuration";

  targets(projectPath: string, homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, ".cursor/mcp.json", "cursor", "agent_config", "json"),
      homeTarget(homeDir, ".cursor/mcp.json", "cursor", "agent_config", "json"),
    ];
  }
}

/**
 * Project-level scanner (generic, not agent-specific).
 * Covers: .env (key inventory only)
 */
class ProjectScanner implements ScannerPlugin {
  readonly agentId: AgentId = "project";
  readonly agentName = "Project";
  readonly description = "Project-level environment variable inventory";

  targets(projectPath: string, _homeDir: string): ScanTarget[] {
    return [
      projectTarget(projectPath, ".env", "project", "env_key", "dotenv", {
        sensitivity: "env_key_inventory",
        contentPolicy: "key_inventory_only"
      }),
    ];
  }
}
