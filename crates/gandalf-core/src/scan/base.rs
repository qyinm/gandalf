use serde_json::{json, Map, Value};

use crate::policy::restore_policy_for;
use crate::types::{
    AgentId, CaptureStatus, DiscoveredItem, EvidenceConfidence, EvidenceKind, EvidenceScope,
};

use super::ScanTarget;

#[derive(Debug, Clone)]
pub struct EvidenceBaseTarget {
    pub agent: AgentId,
    pub source_path: String,
    pub scope: EvidenceScope,
    pub precedence: u32,
    pub parser: crate::types::EvidenceParser,
    pub sensitivity: String,
    pub content_policy: String,
}

#[derive(Debug, Clone)]
pub struct ItemIdTarget {
    pub agent: AgentId,
    pub source_path: String,
    pub scope: EvidenceScope,
}

impl From<&ScanTarget> for EvidenceBaseTarget {
    fn from(target: &ScanTarget) -> Self {
        Self {
            agent: target.agent,
            source_path: target.source_path.clone(),
            scope: target.scope,
            precedence: target.precedence,
            parser: target.parser,
            sensitivity: target.sensitivity.clone(),
            content_policy: target.content_policy.clone(),
        }
    }
}

impl From<&ScanTarget> for ItemIdTarget {
    fn from(target: &ScanTarget) -> Self {
        Self {
            agent: target.agent,
            source_path: target.source_path.clone(),
            scope: target.scope,
        }
    }
}

pub struct ScannerBase {
    pub agent_id: AgentId,
}

impl ScannerBase {
    pub fn new(agent_id: AgentId) -> Self {
        Self { agent_id }
    }

    pub fn item_id(&self, target: &ItemIdTarget, suffix: &str) -> String {
        scanner_item_id(target.scope, target.agent, &target.source_path, suffix)
    }

    pub fn captured(
        &self,
        target: &EvidenceBaseTarget,
        kind: EvidenceKind,
        metadata: Option<Value>,
        value: Option<Value>,
    ) -> DiscoveredItem {
        DiscoveredItem {
            id: scanner_item_id(target.scope, target.agent, &target.source_path, kind.as_str()),
            agent: target.agent,
            kind,
            source_path: target.source_path.clone(),
            scope: target.scope,
            precedence: target.precedence,
            parser: target.parser,
            sensitivity: target.sensitivity.clone(),
            content_policy: target.content_policy.clone(),
            restore_policy: restore_policy_for(kind),
            capture_status: CaptureStatus::Captured,
            confidence: EvidenceConfidence::High,
            name: None,
            value,
            checksum: None,
            metadata,
        }
    }

    pub fn parse_failed(
        &self,
        target: &EvidenceBaseTarget,
        kind: EvidenceKind,
        error: &str,
    ) -> DiscoveredItem {
        let mut item = self.captured(
            target,
            kind,
            Some(json!({ "error": error })),
            None,
        );
        item.id = scanner_item_id(
            target.scope,
            target.agent,
            &target.source_path,
            &format!("{}-parse-failed", kind.as_str()),
        );
        item.capture_status = CaptureStatus::ParseFailed;
        item
    }
}

pub fn scanner_item_id(
    scope: EvidenceScope,
    agent: AgentId,
    source_path: &str,
    suffix: &str,
) -> String {
    let mut id = format!(
        "{}.{}.{}.{}",
        scope.as_str(),
        agent.as_str(),
        source_path,
        suffix
    );
    if let Some(stripped) = id.strip_prefix("~/") {
        id = format!("home/{stripped}");
    }
    id = id
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() || matches!(ch, '.' | '_' | '-') {
                ch
            } else {
                '.'
            }
        })
        .collect();
    id.trim_matches('.').to_lowercase()
}

pub fn as_record(value: &Value) -> Option<&Map<String, Value>> {
    value.as_object()
}

pub fn as_object(value: &Value) -> Option<&Map<String, Value>> {
    as_record(value)
}

pub fn array_of_strings(value: Option<&Value>) -> Vec<String> {
    value
        .and_then(|v| v.as_array())
        .map(|items| {
            items
                .iter()
                .filter_map(|item| item.as_str().map(str::to_string))
                .collect()
        })
        .unwrap_or_default()
}

pub fn metadata_string_array(value: Option<&Value>) -> Vec<String> {
    array_of_strings(value)
}

pub fn is_object(value: &Value) -> bool {
    value.is_object()
}

pub fn normalize_source_path(root: &std::path::Path, absolute_path: &std::path::Path) -> String {
    absolute_path
        .strip_prefix(root)
        .map(|relative| relative.to_string_lossy().replace('\\', "/"))
        .unwrap_or_else(|_| absolute_path.to_string_lossy().replace('\\', "/"))
}

pub fn discovered_item_from_scanner_output(item: DiscoveredItem) -> DiscoveredItem {
    item
}

pub fn unquote_yaml_scalar(value: &str) -> String {
    let trimmed = value.trim();
    trimmed
        .trim_start_matches(['\'', '"'])
        .trim_end_matches(['\'', '"'])
        .to_string()
}

pub fn value_to_js_string(value: &Value) -> String {
    match value {
        Value::String(s) => s.clone(),
        Value::Array(items) => items
            .iter()
            .map(value_to_js_string)
            .collect::<Vec<_>>()
            .join(","),
        Value::Object(_) => "[object Object]".to_string(),
        Value::Number(number) => number.to_string(),
        Value::Bool(boolean) => boolean.to_string(),
        Value::Null => "null".to_string(),
    }
}