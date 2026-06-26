use crate::types::{DiscoveredItem, GraphNode, ProvenanceEntry};

pub fn build_provenance(graph: &[GraphNode], evidence: &[DiscoveredItem]) -> Vec<ProvenanceEntry> {
    let evidence_by_id: std::collections::HashMap<&str, &DiscoveredItem> =
        evidence.iter().map(|item| (item.id.as_str(), item)).collect();

    graph
        .iter()
        .map(|node| {
            let item = evidence_by_id.get(node.evidence_id.as_str());
            ProvenanceEntry {
                node_id: node.id.clone(),
                evidence_id: node.evidence_id.clone(),
                source_path: node.source_path.clone(),
                scope: node.scope,
                precedence: item.map(|i| i.precedence).unwrap_or(0),
                confidence: node.confidence,
                capture_status: item
                    .map(|i| i.capture_status)
                    .unwrap_or(crate::types::CaptureStatus::Unsupported),
            }
        })
        .collect()
}