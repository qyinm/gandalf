use regex::Regex;
use serde_json::{json, Map, Value};

use crate::types::{EvidenceKind, RestorePolicy};

pub const MAX_FILE_BYTES: u64 = 256 * 1024;
pub const MAX_DIRECTORY_DEPTH: u32 = 4;
pub const MAX_DIRECTORY_ENTRIES: u32 = 250;

fn secret_key_pattern() -> &'static Regex {
    static PATTERN: std::sync::OnceLock<Regex> = std::sync::OnceLock::new();
    PATTERN.get_or_init(|| {
        Regex::new(r"(?i)(api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)")
            .expect("valid secret key regex")
    })
}

/// Map evidence kind to its restore policy.
pub fn restore_policy_for(kind: EvidenceKind) -> RestorePolicy {
    match kind {
        EvidenceKind::AgentInstruction
        | EvidenceKind::AgentConfig
        | EvidenceKind::Skill
        | EvidenceKind::Extension => RestorePolicy::FullContentSupported,
        EvidenceKind::McpServer | EvidenceKind::Permission | EvidenceKind::Hook => {
            RestorePolicy::StructuredFieldsOnly
        }
        EvidenceKind::EnvKey => RestorePolicy::KeyInventoryOnly,
        EvidenceKind::Symlink | EvidenceKind::Unsupported => RestorePolicy::NotSupported,
    }
}

pub fn is_secret_like_key(key: &str) -> bool {
    secret_key_pattern().is_match(key)
}

pub fn capture_status_for_key(key: &str) -> &'static str {
    if is_secret_like_key(key) {
        "redacted"
    } else {
        "omitted"
    }
}

pub fn redact_structured_value(value: Value) -> Value {
    match value {
        Value::Array(items) => Value::Array(items.into_iter().map(redact_structured_value).collect()),
        Value::Object(map) => redact_object(map),
        other => other,
    }
}

fn redact_object(map: Map<String, Value>) -> Value {
    let mut redacted = Map::new();

    for (key, nested_value) in map {
        if is_secret_like_key(&key) {
            redacted.insert(key, json!("[redacted]"));
            continue;
        }

        if key == "env" {
            if let Value::Object(env_map) = &nested_value {
                let env_keys: Vec<Value> = env_map.keys().map(|k| json!(k)).collect();
                redacted.insert("envKeys".to_string(), Value::Array(env_keys));
                continue;
            }
        }

        redacted.insert(key, redact_structured_value(nested_value));
    }

    Value::Object(redacted)
}

pub fn ignored_directory(name: &str) -> bool {
    matches!(
        name,
        ".git"
            | "node_modules"
            | "dist"
            | "build"
            | ".cache"
            | "cache"
            | "caches"
            | "logs"
            | "log"
            | ".next"
            | "coverage"
            | ".turbo"
    )
}