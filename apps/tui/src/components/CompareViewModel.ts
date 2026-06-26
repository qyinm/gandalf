import type { GraphDiff, SemanticChange } from "@qxinm/gandalf-core/diff.js";
import type { GraphNode, Snapshot } from "@qxinm/gandalf-core/types.js";
import { formatAgentLabel } from "./TuiFormatters.js";

export interface CompareSideBySideRow {
  marker: "+" | "-" | "~" | " ";
  before: string;
  after: string;
}

export interface CompareSection {
  title: string;
  rows: CompareSideBySideRow[];
}

export interface CompareViewModel {
  fromLabel: string;
  toLabel: string;
  scopeLabel: string;
  summary: string[];
  sections: CompareSection[];
  emptyMessage?: string;
}

export function buildCompareViewModel(input: {
  fromSnapshot: Snapshot;
  toSnapshot: Snapshot;
  diff: GraphDiff;
  toLabel?: string;
  scope?: "Full setup" | "This agent";
}): CompareViewModel {
  const summary = input.diff.semanticChanges.map(compareSummaryLabel);

  return {
    fromLabel: `${input.fromSnapshot.manifest.name}  ${formatDate(input.fromSnapshot.manifest.createdAt)}`,
    toLabel: input.toLabel ?? `${input.toSnapshot.manifest.name}  ${formatDate(input.toSnapshot.manifest.createdAt)}`,
    scopeLabel: input.scope ?? "Full setup",
    summary: summary.length > 0 ? summary : ["No structured setup changes."],
    sections: buildSideBySideSections(input.fromSnapshot.graph, input.toSnapshot.graph),
    emptyMessage: input.diff.semanticChanges.length === 0 && input.diff.rawSourceChanges.length === 0
      ? "Current setup matches the selected saved setup."
      : undefined
  };
}

export function latestSnapshotByCreatedAt(snapshots: Snapshot[]): Snapshot | undefined {
  return [...snapshots].sort((left, right) =>
    right.manifest.createdAt.localeCompare(left.manifest.createdAt)
  )[0];
}

function buildSideBySideSections(beforeGraph: GraphNode[], afterGraph: GraphNode[]): CompareSection[] {
  const beforeByIdentity = new Map(beforeGraph.map((node) => [nodeIdentity(node), node]));
  const afterByIdentity = new Map(afterGraph.map((node) => [nodeIdentity(node), node]));
  const identities = [...new Set([...beforeByIdentity.keys(), ...afterByIdentity.keys()])].sort();
  const sections = new Map<string, CompareSideBySideRow[]>();

  for (const identity of identities) {
    const before = beforeByIdentity.get(identity);
    const after = afterByIdentity.get(identity);
    const node = after ?? before;
    if (!node) continue;

    const title = formatAgentLabel(node.agent);
    const rows = sections.get(title) ?? [];
    rows.push({
      marker: markerForNodes(before, after),
      before: before ? nodeLabel(before) : "",
      after: after ? nodeLabel(after) : ""
    });
    sections.set(title, rows);
  }

  return [...sections.entries()].map(([title, rows]) => ({ title, rows }));
}

function markerForNodes(before: GraphNode | undefined, after: GraphNode | undefined): CompareSideBySideRow["marker"] {
  if (!before && after) return "+";
  if (before && !after) return "-";
  if (before && after && stable(before.effectiveValue) !== stable(after.effectiveValue)) return "~";
  return " ";
}

function nodeIdentity(node: GraphNode): string {
  return [node.agent, node.entityKind, node.entityName].join("\0");
}

function nodeLabel(node: GraphNode): string {
  return `${node.entityKind}: ${node.entityName}`;
}

function compareSummaryLabel(change: SemanticChange): string {
  const agent = change.entityKind === "agent_instruction" ? "Project" : undefined;
  const prefix = markerForChange(change);
  const owner = agent ? `${agent} ` : "";
  return `${prefix} ${owner}${entityKindLabel(change.entityKind)}: ${change.entityName}`;
}

function markerForChange(change: SemanticChange): "+" | "-" | "~" {
  if (change.code.endsWith("_ADDED")) return "+";
  if (change.code.endsWith("_REMOVED")) return "-";
  return "~";
}

function entityKindLabel(kind: GraphNode["entityKind"]): string {
  switch (kind) {
    case "mcp_server": return "MCP";
    case "skill": return "Skill";
    case "permission": return "Permission";
    case "hook": return "Hook";
    case "env_key": return "Env key";
    case "agent_instruction": return "Instructions";
    default: return "Setup";
  }
}

function formatDate(value: string): string {
  return value ? value.slice(0, 10) : "";
}

function stable(value: unknown): string {
  return JSON.stringify(value, (_key, candidate) => {
    if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) {
      return candidate;
    }

    return Object.fromEntries(Object.entries(candidate).sort(([left], [right]) => left.localeCompare(right)));
  });
}
