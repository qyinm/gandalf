import type { DiscoveredItem, GraphNode } from "./types.js";

function entityNameFor(item: DiscoveredItem): string {
  return item.name ?? item.id;
}

function nodeIdFor(item: DiscoveredItem): string {
  return [
    item.agent,
    item.scope,
    item.kind,
    entityNameFor(item),
    item.id
  ]
    .join(":")
    .replaceAll(/\s+/g, " ")
    .trim();
}

function overrideIdentity(node: GraphNode): string {
  return [node.agent, node.entityKind, node.entityName].join("\0");
}

function valueFor(item: DiscoveredItem): unknown {
  if (item.value !== undefined) {
    return item.value;
  }

  if (item.captureStatus === "unsupported") {
    return {
      captureStatus: item.captureStatus,
      state: item.metadata?.state ?? "present"
    };
  }

  return {
    captureStatus: item.captureStatus
  };
}

export function buildGraph(evidence: DiscoveredItem[]): GraphNode[] {
  const nodes = evidence.map((item) => ({
    id: nodeIdFor(item),
    agent: item.agent,
    scope: item.scope,
    sourcePath: item.sourcePath,
    entityKind: item.kind,
    entityName: entityNameFor(item),
    effectiveValue: valueFor(item),
    confidence: item.confidence,
    evidenceId: item.id
  } satisfies GraphNode));

  const strongestByIdentity = new Map<string, { node: GraphNode; precedence: number }>();
  for (const node of nodes) {
    const item = evidence.find((candidate) => candidate.id === node.evidenceId);
    const precedence = item?.precedence ?? 0;
    const identity = overrideIdentity(node);
    const current = strongestByIdentity.get(identity);
    if (!current || precedence > current.precedence) {
      strongestByIdentity.set(identity, { node, precedence });
    }
  }

  return nodes.map((node) => {
    const item = evidence.find((candidate) => candidate.id === node.evidenceId);
    const strongest = strongestByIdentity.get(overrideIdentity(node));
    if (!item || !strongest || strongest.node.id === node.id || strongest.precedence <= item.precedence) {
      return node;
    }

    return {
      ...node,
      overriddenBy: strongest.node.id
    };
  });
}
