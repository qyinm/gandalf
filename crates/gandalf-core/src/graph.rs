use serde_json::{json, Value};

use crate::types::{DiscoveredItem, GraphNode};

fn entity_name_for(item: &DiscoveredItem) -> String {
    item.name.clone().unwrap_or_else(|| item.id.clone())
}

fn node_id_for(item: &DiscoveredItem) -> String {
    [
        item.agent.as_str(),
        item.scope.as_str(),
        item.kind.as_str(),
        &entity_name_for(item),
        &item.id,
    ]
    .join(":")
    .split_whitespace()
    .collect::<Vec<_>>()
    .join(" ")
    .trim()
    .to_string()
}

fn override_identity(node: &GraphNode) -> String {
    [
        node.agent.as_str(),
        node.entity_kind.as_str(),
        &node.entity_name,
    ]
    .join("\0")
}

fn value_for(item: &DiscoveredItem) -> Value {
    if let Some(value) = &item.value {
        return value.clone();
    }

    if item.capture_status == crate::types::CaptureStatus::Unsupported {
        return json!({
            "captureStatus": "unsupported",
            "state": item.metadata.as_ref().and_then(|m| m.get("state")).cloned().unwrap_or(json!("present"))
        });
    }

    json!({ "captureStatus": item.capture_status.as_str() })
}

pub fn build_graph(evidence: &[DiscoveredItem]) -> Vec<GraphNode> {
    let nodes: Vec<GraphNode> = evidence
        .iter()
        .map(|item| GraphNode {
            id: node_id_for(item),
            agent: item.agent,
            scope: item.scope,
            source_path: item.source_path.clone(),
            entity_kind: item.kind,
            entity_name: entity_name_for(item),
            effective_value: value_for(item),
            overridden_by: None,
            confidence: item.confidence,
            evidence_id: item.id.clone(),
        })
        .collect();

    let mut strongest_by_identity: std::collections::HashMap<String, (String, u32)> =
        std::collections::HashMap::new();

    for node in &nodes {
        let item = evidence.iter().find(|c| c.id == node.evidence_id);
        let precedence = item.map(|i| i.precedence).unwrap_or(0);
        let identity = override_identity(node);
        let replace = strongest_by_identity
            .get(&identity)
            .is_none_or(|(_, current)| precedence > *current);
        if replace {
            strongest_by_identity.insert(identity, (node.id.clone(), precedence));
        }
    }

    nodes
        .into_iter()
        .map(|mut node| {
            let item = evidence.iter().find(|c| c.id == node.evidence_id);
            let strongest = strongest_by_identity.get(&override_identity(&node));
            if let (Some(item), Some((strongest_id, strongest_precedence))) = (item, strongest) {
                if strongest_id != &node.id && *strongest_precedence > item.precedence {
                    node.overridden_by = Some(strongest_id.clone());
                }
            }
            node
        })
        .collect()
}