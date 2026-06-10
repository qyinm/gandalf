import { accessSync, constants } from "node:fs";
import path from "node:path";
import { delimiter } from "node:path";
import { platform as currentPlatform } from "node:os";

import type {
  DiscoveredItem,
  McpBinaryInfo,
  McpBinaryReport,
  ReadinessCategory,
  ReadinessItem,
  ReadinessReport,
  Severity
} from "./types.js";

export interface ReadinessOptions {
  sourceHomeDir?: string;
  targetPlatform?: NodeJS.Platform;
  applyContent?: boolean;
  targetEvidence?: DiscoveredItem[];
  processEnv?: NodeJS.ProcessEnv;
  pathEnv?: string;
}

const READINESS_CATEGORIES: ReadinessCategory[] = [
  "ready",
  "needs_manual_action",
  "warning",
  "unverified",
  "unsupported",
  "blocked"
];

export function classifyMcpBinary(command: string, homeDir?: string): McpBinaryInfo["binaryKind"] {
  if (command === "npx" || command === "uvx") return "package_runner";
  if (path.isAbsolute(command)) {
    if (homeDir && isStrictlyUnder(command, homeDir)) return "source_local_path";
    return "path_binary";
  }
  return "command";
}

export function extractMcpBinaries(evidence: DiscoveredItem[], sourceHomeDir?: string): McpBinaryInfo[] {
  const binaries: McpBinaryInfo[] = [];
  for (const item of evidence) {
    if (item.kind !== "mcp_server") continue;
    const value = item.value;

    const command = typeof value?.command === "string" ? value.command : undefined;
    const url = typeof value?.url === "string" ? value.url : undefined;

    if (command || url) {
      const args = Array.isArray(value?.args) ? value.args.filter((a): a is string => typeof a === "string") : undefined;
      const safeUrl = url ? sanitizeRemoteUrl(url) : undefined;
      binaries.push({
        evidenceId: item.id,
        command: command ?? safeUrl ?? "unknown",
        args,
        url: safeUrl,
        binaryKind: url ? "remote_url" : classifyMcpBinary(command ?? "", sourceHomeDir)
      });
    }
  }
  return binaries;
}

export function checkMcpBinaryAvailability(sourceBinaries: McpBinaryInfo[]): McpBinaryReport[] {
  const reports: McpBinaryReport[] = [];

  for (const bin of sourceBinaries) {
    if (bin.url) {
      reports.push({
        evidenceId: bin.evidenceId,
        command: bin.url,
        availableOnTarget: true,
        binaryKind: "remote_url",
        warning: "Remote URL — availability cannot be verified locally"
      });
      continue;
    }

    if (bin.binaryKind === "source_local_path") {
      reports.push({
        evidenceId: bin.evidenceId,
        command: bin.command,
        availableOnTarget: false,
        binaryKind: bin.binaryKind,
        warning: `MCP command points to a source machine local binary path (${bin.command}); install or remap it on this machine.`
      });
      continue;
    }

    const resolved = findExecutableOnPath(bin.command);
    reports.push({
      evidenceId: bin.evidenceId,
      command: bin.command,
      availableOnTarget: resolved.length > 0,
      binaryKind: bin.binaryKind,
      resolvedPath: resolved || undefined,
      warning: bin.binaryKind === "package_runner"
        ? (resolved.length > 0
          ? `Package runner ${bin.command} is available at ${resolved}; package arguments may still differ on this machine.`
          : `Package runner ${bin.command} not found on this machine; MCP package cannot be launched.`)
        : (resolved.length === 0 ? `Binary "${bin.command}" not found on this machine` : undefined)
    });
  }

  return reports;
}

export function buildReadinessReport(
  sourceEvidence: DiscoveredItem[],
  options: ReadinessOptions = {}
): ReadinessReport {
  const targetPlatform = options.targetPlatform ?? currentPlatform();
  const sourceBinaries = extractMcpBinaries(sourceEvidence, options.sourceHomeDir);
  const mcpReports = checkMcpBinaryAvailability(sourceBinaries);
  const items: ReadinessItem[] = [];

  if (targetPlatform !== "darwin") {
    items.push({
      id: "platform.apply-content-macos-only",
      category: options.applyContent ? "blocked" : "unsupported",
      severity: options.applyContent ? "high" : "medium",
      code: "HEM_MACOS_APPLY_ONLY",
      problem: "Bundle content apply is Mac-only in this release.",
      cause: `Target platform is ${targetPlatform}.`,
      fix: options.applyContent
        ? "Run dry-run or inspect here, or apply the bundle on macOS."
        : "Dry-run and inspect remain available here; apply the bundle on macOS."
    });
  }

  for (const report of mcpReports) {
    items.push(readinessItemForMcpReport(report));
  }

  const targetEnvKeys = envKeySet(options.targetEvidence ?? [], false);
  const sourceEnvKeys = envKeySet(sourceEvidence, true);
  const processEnv = options.processEnv ?? process.env;
  for (const key of [...sourceEnvKeys].sort()) {
    if (targetEnvKeys.has(key) || Object.prototype.hasOwnProperty.call(processEnv, key)) continue;
    items.push({
      id: `env.${key}`,
      category: "needs_manual_action",
      severity: "medium",
      code: "HEM_ENV_VALUE_REQUIRED",
      problem: `Environment key ${key} needs a value on this machine.`,
      cause: "The bundle records the key name only; raw env values are omitted by policy.",
      fix: "Add the value manually or through your preferred secret manager before running tools that need it.",
      path: ".env",
      actions: [{ label: "Set env value manually" }]
    });
  }

  return {
    targetPlatform,
    summary: summarize(items),
    items
  };
}

export function readinessItemForMcpReport(report: McpBinaryReport): ReadinessItem {
  if (report.binaryKind === "remote_url") {
    return {
      id: `mcp.${report.evidenceId}.remote`,
      category: "unverified",
      severity: "low",
      code: "HEM_REMOTE_MCP_UNVERIFIED",
      problem: "Remote MCP URL cannot be verified locally.",
      cause: report.warning ?? "Remote MCP availability depends on network and provider state.",
      fix: "Verify the remote endpoint outside hem if this MCP server is required.",
      evidenceId: report.evidenceId,
      command: report.command
    };
  }

  if (report.availableOnTarget) {
    return {
      id: `mcp.${report.evidenceId}.available`,
      category: "ready",
      severity: "none",
      code: "HEM_MCP_COMMAND_AVAILABLE",
      problem: `MCP command ${report.command} is available.`,
      cause: report.resolvedPath ? `Resolved to ${report.resolvedPath}.` : "The command is available on PATH.",
      fix: "No action needed.",
      evidenceId: report.evidenceId,
      command: report.command
    };
  }

  if (report.binaryKind === "source_local_path") {
    return {
      id: `mcp.${report.evidenceId}.source-local-path`,
      category: "needs_manual_action",
      severity: "medium",
      code: "HEM_SOURCE_LOCAL_MCP_PATH",
      problem: "MCP command points to a source-machine local path.",
      cause: report.warning ?? `The source command path is ${report.command}.`,
      fix: "Install the MCP server on this Mac and update the command path if needed.",
      evidenceId: report.evidenceId,
      command: report.command,
      actions: [{ label: "Install or remap local MCP binary" }]
    };
  }

  return {
    id: `mcp.${report.evidenceId}.missing`,
    category: "needs_manual_action",
    severity: "medium",
    code: "HEM_MCP_COMMAND_MISSING",
    problem: `MCP command ${report.command} is missing on this machine.`,
    cause: report.warning ?? `The command ${report.command} was not found on PATH.`,
    fix: installHintForCommand(report.command, report.binaryKind),
    evidenceId: report.evidenceId,
    command: report.command,
    actions: installActionsForCommand(report.command, report.binaryKind)
  };
}

export interface ReadinessFormatOptions {
  maxItems?: number;
  includeFixes?: boolean;
  includeActions?: boolean;
}

export function formatReadinessSummaryLines(
  report: ReadinessReport,
  options: ReadinessFormatOptions = {}
): string[] {
  const maxItems = options.maxItems ?? 5;
  const lines = [
    "Readiness:",
    `  ready: ${report.summary.ready}`,
    `  needs manual action: ${report.summary.needs_manual_action}`,
    `  warnings: ${report.summary.warning}`,
    `  unverified: ${report.summary.unverified}`,
    `  unsupported: ${report.summary.unsupported}`,
    `  blocked: ${report.summary.blocked}`
  ];

  const actionable = report.items.filter((item) => item.category === "blocked" || item.category === "needs_manual_action");
  for (const item of actionable.slice(0, maxItems)) {
    lines.push(`  - ${item.problem}`);
    if (options.includeFixes) lines.push(`    fix: ${item.fix}`);
    if (options.includeActions) {
      for (const action of item.actions ?? []) {
        lines.push(`    action: ${action.command ?? action.label}`);
      }
    }
  }
  if (actionable.length > maxItems) {
    lines.push(`  ... and ${actionable.length - maxItems} more action item(s)`);
  }

  return lines;
}

function envKeySet(evidence: DiscoveredItem[], includeMcpEnvKeys: boolean): Set<string> {
  const keys = new Set<string>();
  for (const item of evidence) {
    if (item.kind === "env_key") {
      const key = typeof item.name === "string"
        ? item.name
        : typeof item.value?.key === "string"
          ? item.value.key
          : undefined;
      if (key) keys.add(key);
    }
    if (includeMcpEnvKeys && item.kind === "mcp_server" && Array.isArray(item.value?.envKeys)) {
      for (const key of item.value.envKeys) {
        if (typeof key === "string") keys.add(key);
      }
    }
  }
  return keys;
}

function summarize(items: ReadinessItem[]): Record<ReadinessCategory, number> {
  const summary = Object.fromEntries(READINESS_CATEGORIES.map((category) => [category, 0])) as Record<ReadinessCategory, number>;
  for (const item of items) summary[item.category] += 1;
  return summary;
}

function installHintForCommand(command: string, kind?: McpBinaryReport["binaryKind"]): string {
  if (command === "npx") return "Install Node.js on this Mac, then rerun the dry-run.";
  if (command === "uvx") return "Install uv on this Mac, then rerun the dry-run.";
  if (command === "gh") return "Install GitHub CLI on this Mac and authenticate it if the MCP server needs GitHub access.";
  if (kind === "package_runner") return `Install package runner ${command} on this Mac, then rerun the dry-run.`;
  return `Install ${command} on this Mac or update the MCP command to a local path that exists.`;
}

function installActionsForCommand(command: string, kind?: McpBinaryReport["binaryKind"]) {
  if (command === "npx") return [{ label: "Install Node.js", command: "brew install node" }];
  if (command === "uvx") return [{ label: "Install uv", command: "brew install uv" }];
  if (command === "gh") return [{ label: "Install GitHub CLI", command: "brew install gh" }];
  if (kind === "package_runner") return [{ label: `Install ${command}` }];
  return [{ label: `Install ${command}` }];
}

function findExecutableOnPath(command: string): string {
  const pathEnv = process.env.PATH ?? "";
  if (path.isAbsolute(command)) {
    return executablePath(command) ? command : "";
  }

  for (const dir of pathEnv.split(delimiter)) {
    if (!dir) continue;
    const candidate = path.join(dir, command);
    if (executablePath(candidate)) return candidate;
  }
  return "";
}

function executablePath(candidate: string): boolean {
  try {
    accessSync(candidate, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

function sanitizeRemoteUrl(rawUrl: string): string {
  try {
    const url = new URL(rawUrl);
    url.username = "";
    url.password = "";
    url.hash = "";
    for (const key of [...url.searchParams.keys()]) {
      if (/(api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)/i.test(key)) {
        url.searchParams.set(key, "[redacted]");
      }
    }
    return url.toString();
  } catch {
    return "[remote-url]";
  }
}

function isStrictlyUnder(resolved: string, root: string): boolean {
  const normalized = path.resolve(root);
  return resolved === normalized || resolved.startsWith(normalized + path.sep);
}
