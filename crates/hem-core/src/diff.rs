use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

use crate::types::{EvidenceKind, GraphNode, Severity};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum SemanticChangeCode {
    AgentConfigAdded,
    AgentConfigRemoved,
    AgentConfigChanged,
    McpAdded,
    McpRemoved,
    McpChanged,
    SkillAdded,
    SkillRemoved,
    HookAdded,
    HookRemoved,
    HookChanged,
    PermissionChanged,
    InstructionChanged,
    PermissionWildcardAdded,
    SkillExecutableAppeared,
    EnvKeyAdded,
    EnvKeyRemoved,
    UnsupportedStateChanged,
}

impl SemanticChangeCode {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::AgentConfigAdded => "AGENT_CONFIG_ADDED",
            Self::AgentConfigRemoved => "AGENT_CONFIG_REMOVED",
            Self::AgentConfigChanged => "AGENT_CONFIG_CHANGED",
            Self::McpAdded => "MCP_ADDED",
            Self::McpRemoved => "MCP_REMOVED",
            Self::McpChanged => "MCP_CHANGED",
            Self::SkillAdded => "SKILL_ADDED",
            Self::SkillRemoved => "SKILL_REMOVED",
            Self::HookAdded => "HOOK_ADDED",
            Self::HookRemoved => "HOOK_REMOVED",
            Self::HookChanged => "HOOK_CHANGED",
            Self::PermissionChanged => "PERMISSION_CHANGED",
            Self::InstructionChanged => "INSTRUCTION_CHANGED",
            Self::PermissionWildcardAdded => "PERMISSION_WILDCARD_ADDED",
            Self::SkillExecutableAppeared => "SKILL_EXECUTABLE_APPEARED",
            Self::EnvKeyAdded => "ENV_KEY_ADDED",
            Self::EnvKeyRemoved => "ENV_KEY_REMOVED",
            Self::UnsupportedStateChanged => "UNSUPPORTED_STATE_CHANGED",
        }
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct SemanticChangeDetails {
    pub changed_fields: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub source_path: Option<String>,
    #[serde(flatten)]
    pub extra: Map<String, Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct SemanticChange {
    pub code: SemanticChangeCode,
    pub entity_kind: EvidenceKind,
    pub entity_name: String,
    pub severity: Severity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub before: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub after: Option<Value>,
    pub details: SemanticChangeDetails,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RawSourceChange {
    pub source_path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub before_evidence_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub after_evidence_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub before_checksum: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub after_checksum: Option<String>,
    pub status: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct GraphDiff {
    pub semantic_changes: Vec<SemanticChange>,
    pub raw_source_changes: Vec<RawSourceChange>,
}

fn graph_identity(node: &GraphNode) -> String {
    [
        node.agent.as_str(),
        node.entity_kind.as_str(),
        &node.entity_name,
    ]
    .join("\0")
}

fn source_identity(node: &GraphNode) -> String {
    [&node.source_path, node.entity_kind.as_str(), &node.entity_name].join("\0")
}

fn stable(value: &Value) -> String {
    fn sort_value(value: &Value) -> Value {
        match value {
            Value::Object(map) => {
                let mut sorted = Map::new();
                let mut keys: Vec<_> = map.keys().cloned().collect();
                keys.sort();
                for key in keys {
                    if let Some(v) = map.get(&key) {
                        sorted.insert(key, sort_value(v));
                    }
                }
                Value::Object(sorted)
            }
            Value::Array(items) => Value::Array(items.iter().map(sort_value).collect()),
            other => other.clone(),
        }
    }
    serde_json::to_string(&sort_value(value)).unwrap_or_default()
}

fn as_record(value: &Value) -> Option<&Map<String, Value>> {
    value.as_object()
}

fn url_host(value: &Value) -> Option<String> {
    let url = as_record(value)?.get("url")?.as_str()?;
    let without_scheme = url.split("://").nth(1).unwrap_or(url);
    without_scheme.split('/').next().map(str::to_string)
}

fn is_wildcard_permission(node: &GraphNode) -> bool {
    if node.entity_kind != EvidenceKind::Permission {
        return false;
    }
    let rule = as_record(&node.effective_value)
        .and_then(|m| m.get("rule"))
        .and_then(|v| v.as_str())
        .unwrap_or(&node.entity_name);
    rule.contains('*') || rule.contains("(*)") || rule == "*"
}

fn executable_appeared(before: Option<&GraphNode>, after: &GraphNode) -> bool {
    if after.entity_kind != EvidenceKind::Skill {
        return false;
    }
    let before_exec = before
        .and_then(|n| as_record(&n.effective_value))
        .and_then(|m| m.get("executable"))
        .and_then(|v| v.as_bool())
        .unwrap_or(false);
    let after_exec = as_record(&after.effective_value)
        .and_then(|m| m.get("executable"))
        .and_then(|v| v.as_bool())
        .unwrap_or(false);
    !before_exec && after_exec
}

fn mcp_changed_fields(before: &GraphNode, after: &GraphNode) -> Vec<String> {
    let empty = Map::new();
    let before_value = as_record(&before.effective_value).unwrap_or(&empty);
    let after_value = as_record(&after.effective_value).unwrap_or(&empty);
    let mut fields = Vec::new();
    for field in ["command", "transport"] {
        if stable(before_value.get(field).unwrap_or(&Value::Null))
            != stable(after_value.get(field).unwrap_or(&Value::Null))
        {
            fields.push(field.to_string());
        }
    }
    if url_host(&before.effective_value) != url_host(&after.effective_value) {
        fields.push("urlHost".to_string());
    }
    fields
}

fn push_added(
    semantic_changes: &mut Vec<SemanticChange>,
    after: &GraphNode,
    code: SemanticChangeCode,
    severity: Severity,
) {
    semantic_changes.push(SemanticChange {
        code,
        entity_kind: after.entity_kind,
        entity_name: after.entity_name.clone(),
        severity,
        before: None,
        after: Some(after.effective_value.clone()),
        details: SemanticChangeDetails {
            changed_fields: Vec::new(),
            source_path: Some(after.source_path.clone()),
            extra: Map::new(),
        },
    });
}

pub fn diff_graphs(baseline_graph: &[GraphNode], current_graph: &[GraphNode]) -> GraphDiff {
    let before_by_identity: std::collections::HashMap<String, &GraphNode> = baseline_graph
        .iter()
        .map(|node| (graph_identity(node), node))
        .collect();
    let after_by_identity: std::collections::HashMap<String, &GraphNode> = current_graph
        .iter()
        .map(|node| (graph_identity(node), node))
        .collect();
    let mut semantic_changes = Vec::new();

    for (identity, after) in &after_by_identity {
        let before = before_by_identity.get(identity.as_str());
        if before.is_none() {
            match after.entity_kind {
                EvidenceKind::McpServer => {
                    push_added(&mut semantic_changes, after, SemanticChangeCode::McpAdded, Severity::Medium);
                }
                EvidenceKind::AgentConfig => {
                    push_added(&mut semantic_changes, after, SemanticChangeCode::AgentConfigAdded, Severity::Medium);
                }
                EvidenceKind::EnvKey => {
                    push_added(&mut semantic_changes, after, SemanticChangeCode::EnvKeyAdded, Severity::Medium);
                }
                EvidenceKind::Skill => {
                    push_added(&mut semantic_changes, after, SemanticChangeCode::SkillAdded, Severity::Low);
                }
                EvidenceKind::Hook => {
                    push_added(&mut semantic_changes, after, SemanticChangeCode::HookAdded, Severity::Medium);
                }
                EvidenceKind::Permission => {
                    push_added(&mut semantic_changes, after, SemanticChangeCode::PermissionChanged, Severity::Medium);
                }
                EvidenceKind::AgentInstruction => {
                    push_added(&mut semantic_changes, after, SemanticChangeCode::InstructionChanged, Severity::Medium);
                }
                _ => {}
            }
            if is_wildcard_permission(after) {
                push_added(
                    &mut semantic_changes,
                    after,
                    SemanticChangeCode::PermissionWildcardAdded,
                    Severity::High,
                );
            }
            if executable_appeared(None, after) {
                push_added(
                    &mut semantic_changes,
                    after,
                    SemanticChangeCode::SkillExecutableAppeared,
                    Severity::High,
                );
            }
            continue;
        }

        let before = before.unwrap();
        if after.entity_kind == EvidenceKind::McpServer
            && stable(&before.effective_value) != stable(&after.effective_value)
        {
            semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::McpChanged,
                entity_kind: after.entity_kind,
                entity_name: after.entity_name.clone(),
                severity: Severity::Medium,
                before: Some(before.effective_value.clone()),
                after: Some(after.effective_value.clone()),
                details: SemanticChangeDetails {
                    changed_fields: mcp_changed_fields(before, after),
                    source_path: Some(after.source_path.clone()),
                    extra: Map::new(),
                },
            });
        }
        macro_rules! changed {
            ($kind:expr, $code:ident) => {
                if after.entity_kind == $kind
                    && stable(&before.effective_value) != stable(&after.effective_value)
                {
                    semantic_changes.push(SemanticChange {
                        code: SemanticChangeCode::$code,
                        entity_kind: after.entity_kind,
                        entity_name: after.entity_name.clone(),
                        severity: Severity::Medium,
                        before: Some(before.effective_value.clone()),
                        after: Some(after.effective_value.clone()),
                        details: SemanticChangeDetails {
                            changed_fields: Vec::new(),
                            source_path: Some(after.source_path.clone()),
                            extra: Map::new(),
                        },
                    });
                }
            };
        }
        changed!(EvidenceKind::AgentConfig, AgentConfigChanged);
        changed!(EvidenceKind::Hook, HookChanged);
        changed!(EvidenceKind::Permission, PermissionChanged);
        changed!(EvidenceKind::AgentInstruction, InstructionChanged);

        if is_wildcard_permission(after) && !is_wildcard_permission(before) {
            semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::PermissionWildcardAdded,
                entity_kind: after.entity_kind,
                entity_name: after.entity_name.clone(),
                severity: Severity::High,
                before: Some(before.effective_value.clone()),
                after: Some(after.effective_value.clone()),
                details: SemanticChangeDetails {
                    changed_fields: Vec::new(),
                    source_path: Some(after.source_path.clone()),
                    extra: Map::new(),
                },
            });
        }
        if executable_appeared(Some(before), after) {
            semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::SkillExecutableAppeared,
                entity_kind: after.entity_kind,
                entity_name: after.entity_name.clone(),
                severity: Severity::High,
                before: Some(before.effective_value.clone()),
                after: Some(after.effective_value.clone()),
                details: SemanticChangeDetails {
                    changed_fields: Vec::new(),
                    source_path: Some(after.source_path.clone()),
                    extra: Map::new(),
                },
            });
        }
        if after.entity_kind == EvidenceKind::Unsupported
            && stable(&before.effective_value) != stable(&after.effective_value)
        {
            semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::UnsupportedStateChanged,
                entity_kind: after.entity_kind,
                entity_name: after.entity_name.clone(),
                severity: Severity::Medium,
                before: Some(before.effective_value.clone()),
                after: Some(after.effective_value.clone()),
                details: SemanticChangeDetails {
                    changed_fields: Vec::new(),
                    source_path: Some(after.source_path.clone()),
                    extra: Map::new(),
                },
            });
        }
    }

    for (identity, before) in &before_by_identity {
        if after_by_identity.contains_key(identity.as_str()) {
            continue;
        }
        match before.entity_kind {
            EvidenceKind::McpServer => semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::McpRemoved,
                entity_kind: before.entity_kind,
                entity_name: before.entity_name.clone(),
                severity: Severity::Medium,
                before: Some(before.effective_value.clone()),
                after: None,
                details: SemanticChangeDetails {
                    changed_fields: Vec::new(),
                    source_path: Some(before.source_path.clone()),
                    extra: Map::new(),
                },
            }),
            EvidenceKind::AgentConfig => semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::AgentConfigRemoved,
                entity_kind: before.entity_kind,
                entity_name: before.entity_name.clone(),
                severity: Severity::Medium,
                before: Some(before.effective_value.clone()),
                after: None,
                details: SemanticChangeDetails {
                    changed_fields: Vec::new(),
                    source_path: Some(before.source_path.clone()),
                    extra: Map::new(),
                },
            }),
            EvidenceKind::EnvKey => semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::EnvKeyRemoved,
                entity_kind: before.entity_kind,
                entity_name: before.entity_name.clone(),
                severity: Severity::Medium,
                before: Some(before.effective_value.clone()),
                after: None,
                details: SemanticChangeDetails {
                    changed_fields: Vec::new(),
                    source_path: Some(before.source_path.clone()),
                    extra: Map::new(),
                },
            }),
            EvidenceKind::Skill => semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::SkillRemoved,
                entity_kind: before.entity_kind,
                entity_name: before.entity_name.clone(),
                severity: Severity::Low,
                before: Some(before.effective_value.clone()),
                after: None,
                details: SemanticChangeDetails {
                    changed_fields: Vec::new(),
                    source_path: Some(before.source_path.clone()),
                    extra: Map::new(),
                },
            }),
            EvidenceKind::Hook => semantic_changes.push(SemanticChange {
                code: SemanticChangeCode::HookRemoved,
                entity_kind: before.entity_kind,
                entity_name: before.entity_name.clone(),
                severity: Severity::Medium,
                before: Some(before.effective_value.clone()),
                after: None,
                details: SemanticChangeDetails {
                    changed_fields: Vec::new(),
                    source_path: Some(before.source_path.clone()),
                    extra: Map::new(),
                },
            }),
            EvidenceKind::Permission => {
                let mut extra = Map::new();
                extra.insert("removed".to_string(), Value::Bool(true));
                semantic_changes.push(SemanticChange {
                    code: SemanticChangeCode::PermissionChanged,
                    entity_kind: before.entity_kind,
                    entity_name: before.entity_name.clone(),
                    severity: Severity::Medium,
                    before: Some(before.effective_value.clone()),
                    after: None,
                    details: SemanticChangeDetails {
                        changed_fields: Vec::new(),
                        source_path: Some(before.source_path.clone()),
                        extra,
                    },
                });
            }
            _ => {}
        }
    }

    let before_by_source: std::collections::HashMap<String, &GraphNode> = baseline_graph
        .iter()
        .map(|node| (source_identity(node), node))
        .collect();
    let after_by_source: std::collections::HashMap<String, &GraphNode> = current_graph
        .iter()
        .map(|node| (source_identity(node), node))
        .collect();
    let mut source_keys: Vec<_> = before_by_source
        .keys()
        .chain(after_by_source.keys())
        .cloned()
        .collect::<std::collections::HashSet<_>>()
        .into_iter()
        .collect();
    source_keys.sort();

    let mut raw_source_changes = Vec::new();
    for key in source_keys {
        let before = before_by_source.get(&key);
        let after = after_by_source.get(&key);
        match (before, after) {
            (None, Some(after)) => raw_source_changes.push(RawSourceChange {
                source_path: after.source_path.clone(),
                before_evidence_id: None,
                after_evidence_id: Some(after.evidence_id.clone()),
                before_checksum: None,
                after_checksum: None,
                status: "added".to_string(),
            }),
            (Some(before), None) => raw_source_changes.push(RawSourceChange {
                source_path: before.source_path.clone(),
                before_evidence_id: Some(before.evidence_id.clone()),
                after_evidence_id: None,
                before_checksum: None,
                after_checksum: None,
                status: "removed".to_string(),
            }),
            (Some(before), Some(after))
                if before.evidence_id != after.evidence_id
                    || stable(&before.effective_value) != stable(&after.effective_value) =>
            {
                raw_source_changes.push(RawSourceChange {
                    source_path: after.source_path.clone(),
                    before_evidence_id: Some(before.evidence_id.clone()),
                    after_evidence_id: Some(after.evidence_id.clone()),
                    before_checksum: None,
                    after_checksum: None,
                    status: "changed".to_string(),
                });
            }
            _ => {}
        }
    }

    GraphDiff {
        semantic_changes,
        raw_source_changes,
    }
}