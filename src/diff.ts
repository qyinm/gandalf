import type { GraphNode } from "./types.js";

export type SemanticChangeCode =
  | "MCP_ADDED"
  | "MCP_REMOVED"
  | "MCP_CHANGED"
  | "PERMISSION_WILDCARD_ADDED"
  | "SKILL_EXECUTABLE_APPEARED"
  | "ENV_KEY_ADDED"
  | "ENV_KEY_REMOVED"
  | "UNSUPPORTED_STATE_CHANGED";

export interface SemanticChange {
  code: SemanticChangeCode;
  entityKind: GraphNode["entityKind"];
  entityName: string;
  severity: "low" | "medium" | "high" | "critical";
  before?: unknown;
  after?: unknown;
  details: {
    changedFields: string[];
    sourcePath?: string;
    [key: string]: unknown;
  };
}

export interface RawSourceChange {
  sourcePath: string;
  beforeEvidenceId?: string;
  afterEvidenceId?: string;
  beforeChecksum?: string;
  afterChecksum?: string;
  status: "added" | "removed" | "changed";
}

export interface GraphDiff {
  semanticChanges: SemanticChange[];
  rawSourceChanges: RawSourceChange[];
}

function graphIdentity(node: GraphNode): string {
  return [node.agent, node.entityKind, node.entityName].join("\0");
}

function sourceIdentity(node: GraphNode): string {
  return [node.sourcePath, node.entityKind, node.entityName].join("\0");
}

function stable(value: unknown): string {
  return JSON.stringify(value, (_key, candidate) => {
    if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) {
      return candidate;
    }

    return Object.fromEntries(Object.entries(candidate).sort(([left], [right]) => left.localeCompare(right)));
  });
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function urlHost(value: unknown): string | undefined {
  const url = asRecord(value).url;
  if (typeof url !== "string") {
    return undefined;
  }

  try {
    return new URL(url).host;
  } catch {
    return undefined;
  }
}

function isWildcardPermission(node: GraphNode): boolean {
  if (node.entityKind !== "permission") {
    return false;
  }

  const value = asRecord(node.effectiveValue);
  const rule = typeof value.rule === "string" ? value.rule : node.entityName;
  return rule.includes("*") || rule.includes("(*)") || rule === "*";
}

function executableAppeared(before: GraphNode | undefined, after: GraphNode): boolean {
  if (after.entityKind !== "skill") {
    return false;
  }

  const beforeExecutable = Boolean(asRecord(before?.effectiveValue).executable);
  const afterExecutable = Boolean(asRecord(after.effectiveValue).executable);
  return !beforeExecutable && afterExecutable;
}

function mcpChangedFields(before: GraphNode, after: GraphNode): string[] {
  const beforeValue = asRecord(before.effectiveValue);
  const afterValue = asRecord(after.effectiveValue);
  const fields = new Set<string>();

  for (const field of ["command", "transport"] as const) {
    if (stable(beforeValue[field]) !== stable(afterValue[field])) {
      fields.add(field);
    }
  }

  if (urlHost(before.effectiveValue) !== urlHost(after.effectiveValue)) {
    fields.add("urlHost");
  }

  return [...fields];
}

export function diffGraphs(baselineGraph: GraphNode[], currentGraph: GraphNode[]): GraphDiff {
  const beforeByIdentity = new Map(baselineGraph.map((node) => [graphIdentity(node), node]));
  const afterByIdentity = new Map(currentGraph.map((node) => [graphIdentity(node), node]));
  const semanticChanges: SemanticChange[] = [];

  for (const [identity, after] of afterByIdentity) {
    const before = beforeByIdentity.get(identity);
    if (!before) {
      if (after.entityKind === "mcp_server") {
        semanticChanges.push({
          code: "MCP_ADDED",
          entityKind: after.entityKind,
          entityName: after.entityName,
          severity: "medium",
          after: after.effectiveValue,
          details: { changedFields: [], sourcePath: after.sourcePath }
        });
      }
      if (after.entityKind === "env_key") {
        semanticChanges.push({
          code: "ENV_KEY_ADDED",
          entityKind: after.entityKind,
          entityName: after.entityName,
          severity: "medium",
          after: after.effectiveValue,
          details: { changedFields: [], sourcePath: after.sourcePath }
        });
      }
      if (isWildcardPermission(after)) {
        semanticChanges.push({
          code: "PERMISSION_WILDCARD_ADDED",
          entityKind: after.entityKind,
          entityName: after.entityName,
          severity: "high",
          after: after.effectiveValue,
          details: { changedFields: [], sourcePath: after.sourcePath }
        });
      }
      if (executableAppeared(undefined, after)) {
        semanticChanges.push({
          code: "SKILL_EXECUTABLE_APPEARED",
          entityKind: after.entityKind,
          entityName: after.entityName,
          severity: "high",
          after: after.effectiveValue,
          details: { changedFields: [], sourcePath: after.sourcePath }
        });
      }
      continue;
    }

    if (after.entityKind === "mcp_server" && stable(before.effectiveValue) !== stable(after.effectiveValue)) {
      semanticChanges.push({
        code: "MCP_CHANGED",
        entityKind: after.entityKind,
        entityName: after.entityName,
        severity: "medium",
        before: before.effectiveValue,
        after: after.effectiveValue,
        details: { changedFields: mcpChangedFields(before, after), sourcePath: after.sourcePath }
      });
    }

    if (isWildcardPermission(after) && !isWildcardPermission(before)) {
      semanticChanges.push({
        code: "PERMISSION_WILDCARD_ADDED",
        entityKind: after.entityKind,
        entityName: after.entityName,
        severity: "high",
        before: before.effectiveValue,
        after: after.effectiveValue,
        details: { changedFields: [], sourcePath: after.sourcePath }
      });
    }

    if (executableAppeared(before, after)) {
      semanticChanges.push({
        code: "SKILL_EXECUTABLE_APPEARED",
        entityKind: after.entityKind,
        entityName: after.entityName,
        severity: "high",
        before: before.effectiveValue,
        after: after.effectiveValue,
        details: { changedFields: [], sourcePath: after.sourcePath }
      });
    }

    if (after.entityKind === "unsupported" && stable(before.effectiveValue) !== stable(after.effectiveValue)) {
      semanticChanges.push({
        code: "UNSUPPORTED_STATE_CHANGED",
        entityKind: after.entityKind,
        entityName: after.entityName,
        severity: "medium",
        before: before.effectiveValue,
        after: after.effectiveValue,
        details: { changedFields: [], sourcePath: after.sourcePath }
      });
    }
  }

  for (const [identity, before] of beforeByIdentity) {
    if (afterByIdentity.has(identity)) {
      continue;
    }

    if (before.entityKind === "mcp_server") {
      semanticChanges.push({
        code: "MCP_REMOVED",
        entityKind: before.entityKind,
        entityName: before.entityName,
        severity: "medium",
        before: before.effectiveValue,
        details: { changedFields: [], sourcePath: before.sourcePath }
      });
    }
    if (before.entityKind === "env_key") {
      semanticChanges.push({
        code: "ENV_KEY_REMOVED",
        entityKind: before.entityKind,
        entityName: before.entityName,
        severity: "medium",
        before: before.effectiveValue,
        details: { changedFields: [], sourcePath: before.sourcePath }
      });
    }
  }

  const beforeBySource = new Map(baselineGraph.map((node) => [sourceIdentity(node), node]));
  const afterBySource = new Map(currentGraph.map((node) => [sourceIdentity(node), node]));
  const sourceKeys = new Set([...beforeBySource.keys(), ...afterBySource.keys()]);
  const rawSourceChanges: RawSourceChange[] = [];

  for (const key of [...sourceKeys].sort()) {
    const before = beforeBySource.get(key);
    const after = afterBySource.get(key);
    if (!before && after) {
      rawSourceChanges.push({
        sourcePath: after.sourcePath,
        afterEvidenceId: after.evidenceId,
        beforeChecksum: undefined,
        afterChecksum: undefined,
        status: "added"
      });
    } else if (before && !after) {
      rawSourceChanges.push({
        sourcePath: before.sourcePath,
        beforeEvidenceId: before.evidenceId,
        beforeChecksum: undefined,
        afterChecksum: undefined,
        status: "removed"
      });
    } else if (before && after && (before.evidenceId !== after.evidenceId || stable(before.effectiveValue) !== stable(after.effectiveValue))) {
      rawSourceChanges.push({
        sourcePath: after.sourcePath,
        beforeEvidenceId: before.evidenceId,
        afterEvidenceId: after.evidenceId,
        beforeChecksum: undefined,
        afterChecksum: undefined,
        status: "changed"
      });
    }
  }

  return { semanticChanges, rawSourceChanges };
}
