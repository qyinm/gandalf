use std::fs;
use std::io;
use std::path::Path;

use serde_json::{json, Value};

use crate::policy::{
    ignored_directory, restore_policy_for, MAX_DIRECTORY_DEPTH, MAX_DIRECTORY_ENTRIES,
    MAX_FILE_BYTES,
};
use crate::types::{
    CaptureStatus, DiscoveredItem, EvidenceConfidence, EvidenceKind, EvidenceParser,
};

use crate::parsers::{
    parse_dotenv_keys, parse_json, parse_markdown, parse_toml_key_values, ParseResult,
};

use super::base::{as_record, scanner_item_id, value_to_js_string};
use super::ScanTarget;

pub fn scan_targets(targets: &[ScanTarget]) -> Vec<DiscoveredItem> {
    let mut evidence = Vec::new();
    for target in targets {
        evidence.extend(scan_target(target));
    }
    evidence
}

pub fn scan_target(target: &ScanTarget) -> Vec<DiscoveredItem> {
    let metadata = match fs::symlink_metadata(&target.absolute_path) {
        Ok(stats) => stats,
        Err(error) if is_not_found(&error) => return Vec::new(),
        Err(error) => {
            return vec![base_item(
                target,
                CaptureStatus::Unsupported,
                Some(json!({ "error": readable_error(&error) })),
                None,
            )];
        }
    };

    if metadata.file_type().is_symlink() {
        return vec![DiscoveredItem {
            id: item_id(target, "symlink"),
            agent: target.agent,
            kind: EvidenceKind::Symlink,
            source_path: target.source_path.clone(),
            scope: target.scope,
            precedence: target.precedence,
            parser: EvidenceParser::Filesystem,
            sensitivity: target.sensitivity.clone(),
            content_policy: "metadata_only".to_string(),
            restore_policy: restore_policy_for(EvidenceKind::Symlink),
            capture_status: CaptureStatus::Omitted,
            confidence: EvidenceConfidence::High,
            name: None,
            value: None,
            checksum: None,
            metadata: Some(json!({ "reason": "symlink_not_followed" })),
        }];
    }

    if target.directory {
        if !metadata.is_dir() {
            return Vec::new();
        }
        return scan_directory(target);
    }

    if !metadata.is_file() {
        return Vec::new();
    }

    if metadata.len() > MAX_FILE_BYTES {
        return vec![base_item(
            target,
            CaptureStatus::Unsupported,
            Some(json!({
                "reason": "file_too_large",
                "sizeBytes": metadata.len(),
            })),
            None,
        )];
    }

    if target.metadata_only {
        return vec![base_item(
            target,
            CaptureStatus::Captured,
            Some(json!({ "present": true, "sizeBytes": metadata.len() })),
            None,
        )];
    }

    let text = match fs::read_to_string(&target.absolute_path) {
        Ok(text) => text,
        Err(error) if is_not_found(&error) => return Vec::new(),
        Err(error) => {
            return vec![base_item(
                target,
                CaptureStatus::Unsupported,
                Some(json!({ "error": readable_error(&error) })),
                None,
            )];
        }
    };

    if target.parser == EvidenceParser::Dotenv {
        return parse_dotenv_keys(&text)
            .into_iter()
            .map(|entry| {
                let capture_status = match entry.capture_status {
                    "redacted" => CaptureStatus::Redacted,
                    "omitted" => CaptureStatus::Omitted,
                    _ => CaptureStatus::Omitted,
                };
                let mut item = base_item(
                    target,
                    capture_status,
                    Some(json!({ "secretLike": entry.secret_like })),
                    None,
                );
                item.id = item_id(target, &entry.key);
                item.name = Some(entry.key);
                item
            })
            .collect();
    }

    let parsed = parse_target(target, &text);
    match parsed {
        ParseResult::Err(failure) => {
            vec![base_item(
                target,
                CaptureStatus::ParseFailed,
                Some(json!({ "error": failure.error })),
                None,
            )]
        }
        ParseResult::Ok(success) => {
            if target.parser == EvidenceParser::Json && !target.metadata_only {
                emit_json_evidence(target, &success.value)
            } else {
                vec![base_item(
                    target,
                    CaptureStatus::Captured,
                    None,
                    Some(success.value),
                )]
            }
        }
    }
}



fn scan_directory(target: &ScanTarget) -> Vec<DiscoveredItem> {
    if target.kind == EvidenceKind::Skill {
        return scan_skill_directory(target);
    }

    let mut evidence = vec![base_item(
        target,
        if target.kind == EvidenceKind::Unsupported {
            CaptureStatus::Unsupported
        } else {
            CaptureStatus::Captured
        },
        Some(json!({ "present": true })),
        None,
    )];
    scan_directory_entries(target, &target.absolute_path, &target.source_path, &mut evidence, 0);
    evidence
}

pub fn scan_skill_directory(target: &ScanTarget) -> Vec<DiscoveredItem> {
    let mut evidence = Vec::new();
    let entries = match fs::read_dir(&target.absolute_path) {
        Ok(entries) => entries,
        Err(_) => return evidence,
    };

    for entry in entries.take(MAX_DIRECTORY_ENTRIES as usize) {
        let Ok(entry) = entry else { continue };
        let entry_name = entry.file_name().to_string_lossy().to_string();
        if entry
            .file_type()
            .map(|ft| ft.is_dir())
            .unwrap_or(false)
            && ignored_directory(&entry_name)
        {
            continue;
        }

        let absolute_path = entry.path();
        let source_path = format!("{}/{}", target.source_path, entry_name);
        let child_target = ScanTarget {
            absolute_path: absolute_path.clone(),
            source_path: source_path.clone(),
            ..target.clone()
        };

        let metadata = match fs::symlink_metadata(&absolute_path) {
            Ok(metadata) => metadata,
            Err(_) => continue,
        };

        if metadata.file_type().is_symlink() {
            let mut item = base_item(
                &child_target,
                CaptureStatus::Omitted,
                Some(json!({
                    "reason": "symlink_not_followed",
                    "skillName": entry_name,
                })),
                None,
            );
            item.name = Some(entry_name);
            evidence.push(item);
            continue;
        }

        if !metadata.is_dir() {
            continue;
        }

        let mut item_metadata = json!({ "present": true, "skillName": entry_name });
        let skill_md = absolute_path.join("SKILL.md");
        match fs::symlink_metadata(&skill_md) {
            Ok(entrypoint) => {
                if let Some(obj) = item_metadata.as_object_mut() {
                    obj.insert("entrypoint".to_string(), json!("SKILL.md"));
                    obj.insert(
                        "entrypointStatus".to_string(),
                        json!(if entrypoint.file_type().is_symlink() {
                            "symlink_not_followed"
                        } else {
                            "captured"
                        }),
                    );
                    if entrypoint.is_file() {
                        obj.insert(
                            "entrypointSizeBytes".to_string(),
                            json!(entrypoint.len()),
                        );
                    }
                }
            }
            Err(error) if !is_not_found(&error) => {
                if let Some(obj) = item_metadata.as_object_mut() {
                    obj.insert("entrypointStatus".to_string(), json!("unreadable"));
                    obj.insert(
                        "entrypointError".to_string(),
                        json!(readable_error(&error)),
                    );
                }
            }
            Err(_) => {}
        }

        let mut item = base_item(
            &child_target,
            CaptureStatus::Captured,
            Some(item_metadata),
            None,
        );
        item.name = Some(entry_name);
        evidence.push(item);
    }

    evidence
}

fn scan_directory_entries(
    target: &ScanTarget,
    absolute_dir: &Path,
    source_dir: &str,
    evidence: &mut Vec<DiscoveredItem>,
    depth: u32,
) {
    if depth >= MAX_DIRECTORY_DEPTH {
        return;
    }

    let entries = match fs::read_dir(absolute_dir) {
        Ok(entries) => entries,
        Err(_) => return,
    };

    for entry in entries.take(MAX_DIRECTORY_ENTRIES as usize) {
        let Ok(entry) = entry else { continue };
        let entry_name = entry.file_name().to_string_lossy().to_string();
        let absolute_path = entry.path();
        let source_path = format!("{source_dir}/{entry_name}");
        let child_target = ScanTarget {
            absolute_path: absolute_path.clone(),
            source_path: source_path.clone(),
            ..target.clone()
        };

        let metadata = match fs::symlink_metadata(&absolute_path) {
            Ok(metadata) => metadata,
            Err(_) => continue,
        };

        if metadata.file_type().is_symlink() {
            evidence.push(DiscoveredItem {
                id: item_id(&child_target, "symlink"),
                agent: child_target.agent,
                kind: EvidenceKind::Symlink,
                source_path: child_target.source_path.clone(),
                scope: child_target.scope,
                precedence: child_target.precedence,
                parser: EvidenceParser::Filesystem,
                sensitivity: child_target.sensitivity.clone(),
                content_policy: child_target.content_policy.clone(),
                restore_policy: restore_policy_for(EvidenceKind::Symlink),
                capture_status: CaptureStatus::Omitted,
                confidence: EvidenceConfidence::High,
                name: None,
                value: None,
                checksum: None,
                metadata: Some(json!({ "reason": "symlink_not_followed" })),
            });
        } else if metadata.is_dir() {
            evidence.push(base_item(
                &child_target,
                if target.kind == EvidenceKind::Skill {
                    CaptureStatus::Captured
                } else {
                    CaptureStatus::Unsupported
                },
                Some(json!({ "present": true })),
                None,
            ));
            scan_directory_entries(
                target,
                &absolute_path,
                &source_path,
                evidence,
                depth + 1,
            );
        } else if metadata.is_file() {
            evidence.push(base_item(
                &child_target,
                if target.kind == EvidenceKind::Skill {
                    CaptureStatus::Captured
                } else {
                    CaptureStatus::Unsupported
                },
                Some(json!({ "sizeBytes": metadata.len() })),
                None,
            ));
        }
    }
}

fn parse_target(target: &ScanTarget, text: &str) -> ParseResult {
    match target.parser {
        EvidenceParser::Json => parse_json(text),
        EvidenceParser::Toml => parse_toml_key_values(text),
        EvidenceParser::Markdown => parse_markdown(text),
        _ => ParseResult::Ok(crate::parsers::ParseSuccess {
            value: json!({ "present": true }),
        }),
    }
}

fn emit_json_evidence(target: &ScanTarget, value: &Value) -> Vec<DiscoveredItem> {
    if target.source_path.ends_with(".mcp.json") || target.source_path.ends_with("/mcp.json") {
        if let Some(servers) = mcp_servers(value) {
            return servers
                .iter()
                .map(|(name, server_value)| {
                    let mut item = base_item(
                        &ScanTarget {
                            kind: EvidenceKind::McpServer,
                            sensitivity: "command_config".to_string(),
                            content_policy: "structured_safe_fields_only".to_string(),
                            ..target.clone()
                        },
                        CaptureStatus::Captured,
                        None,
                        Some(server_value.clone()),
                    );
                    item.id = item_id(target, &format!("mcp-{name}"));
                    item.kind = EvidenceKind::McpServer;
                    item.name = Some(name.clone());
                    item
                })
                .collect();
        }
    }

    let mut evidence = Vec::new();
    if target.source_path.ends_with("/settings.json")
        || target.source_path.ends_with("settings.json")
    {
        if let Some(record) = as_record(value) {
            if let Some(perms) = as_record(record.get("permissions").unwrap_or(&Value::Null)) {
                for (perm_name, perm_rule) in perms {
                    let mut item = base_item(
                        &ScanTarget {
                            kind: EvidenceKind::Permission,
                            sensitivity: "command_config".to_string(),
                            content_policy: "structured_safe_fields_only".to_string(),
                            ..target.clone()
                        },
                        CaptureStatus::Captured,
                        Some(json!({ "permissionKey": perm_name })),
                        Some(json!({ "rule": perm_rule })),
                    );
                    item.id = item_id(target, &format!("perm-{perm_name}"));
                    item.kind = EvidenceKind::Permission;
                    item.name = Some(value_to_js_string(perm_rule));
                    evidence.push(item);
                }
            }

            if let Some(hooks) = as_record(record.get("hooks").unwrap_or(&Value::Null)) {
                for (event_name, event_hooks) in hooks {
                    let Some(event_hooks) = event_hooks.as_array() else {
                        continue;
                    };
                    for (i, event_hook) in event_hooks.iter().enumerate() {
                        let Some(entry) = as_record(event_hook) else {
                            continue;
                        };
                        let matcher = entry
                            .get("matcher")
                            .and_then(|v| v.as_str())
                            .unwrap_or("*");
                        let Some(nested_hooks) = entry.get("hooks").and_then(|v| v.as_array())
                        else {
                            continue;
                        };
                        for hook in nested_hooks {
                            let Some(hook_record) = as_record(hook) else {
                                continue;
                            };
                            let Some(command) = hook_record.get("command").and_then(|v| v.as_str())
                            else {
                                continue;
                            };
                            let mut item = base_item(
                                &ScanTarget {
                                    kind: EvidenceKind::Hook,
                                    sensitivity: "command_config".to_string(),
                                    content_policy: "structured_safe_fields_only".to_string(),
                                    ..target.clone()
                                },
                                CaptureStatus::Captured,
                                Some(json!({
                                    "executable": true,
                                    "eventName": event_name,
                                    "matcher": matcher,
                                    "command": command,
                                })),
                                Some(Value::Object(hook_record.clone())),
                            );
                            item.id = item_id(target, &format!("hook-{event_name}-{i}"));
                            item.kind = EvidenceKind::Hook;
                            item.name = Some(format!("{event_name}.{matcher}"));
                            evidence.push(item);
                        }
                    }
                }
            }
        }
    }

    evidence.push(base_item(
        target,
        CaptureStatus::Captured,
        None,
        Some(value.clone()),
    ));

    evidence
}

fn mcp_servers(value: &Value) -> Option<Vec<(String, Value)>> {
    let servers = as_record(
        as_record(value)?
            .get("mcpServers")
            .unwrap_or(&Value::Null),
    )?;
    Some(
        servers
            .iter()
            .map(|(name, server_value)| (name.clone(), server_value.clone()))
            .collect(),
    )
}

fn base_item(
    target: &ScanTarget,
    capture_status: CaptureStatus,
    metadata: Option<Value>,
    value: Option<Value>,
) -> DiscoveredItem {
    DiscoveredItem {
        id: item_id(target, target.kind.as_str()),
        agent: target.agent,
        kind: target.kind,
        source_path: target.source_path.clone(),
        scope: target.scope,
        precedence: target.precedence,
        parser: target.parser,
        sensitivity: target.sensitivity.clone(),
        content_policy: target.content_policy.clone(),
        restore_policy: restore_policy_for(target.kind),
        capture_status,
        confidence: EvidenceConfidence::High,
        name: None,
        value,
        checksum: None,
        metadata,
    }
}

fn item_id(target: &ScanTarget, suffix: &str) -> String {
    scanner_item_id(
        target.scope,
        target.agent,
        &target.source_path,
        suffix,
    )
}

fn is_not_found(error: &io::Error) -> bool {
    error.kind() == io::ErrorKind::NotFound
}

fn readable_error(error: &io::Error) -> String {
    error.to_string()
}