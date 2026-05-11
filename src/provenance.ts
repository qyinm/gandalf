import type { DiscoveredItem, GraphNode, ProvenanceEntry } from "./types.js";

export function buildProvenance(graph: GraphNode[], evidence: DiscoveredItem[]): ProvenanceEntry[] {
  const evidenceById = new Map(evidence.map((item) => [item.id, item]));

  return graph.map((node) => {
    const item = evidenceById.get(node.evidenceId);
    return {
      nodeId: node.id,
      evidenceId: node.evidenceId,
      sourcePath: node.sourcePath,
      scope: node.scope,
      precedence: item?.precedence ?? 0,
      confidence: node.confidence,
      captureStatus: item?.captureStatus ?? "unsupported"
    };
  });
}
