use std::fs;
use std::path::Path;

use regex::Regex;
use sha2::{Digest, Sha256};
use time::format_description::well_known::Rfc3339;
use time::OffsetDateTime;

use crate::audit::audit_evidence;
use crate::graph::build_graph;
use crate::provenance::build_provenance;
use crate::scan::scan_project;
use crate::store::ensure_store;
use crate::types::{
    AgentId, CaptureStatus, CurrentState, DiscoveredItem, EvidenceKind, EvidenceScope,
    RuntimeOptions, ScanOptions, Snapshot, SnapshotContentEntry, SnapshotManifest,
    SnapshotSecurity,
};

pub fn capture_current_state(options: &RuntimeOptions, name: &str) -> Result<CurrentState, std::io::Error> {
    let store_findings = ensure_store(Path::new(&options.store_dir))
        .map_err(|e| std::io::Error::new(std::io::ErrorKind::Other, e.to_string()))?;

    let scan_options = ScanOptions {
        project_path: options.project_path.clone(),
        home_dir: options.home_dir.clone(),
        store_dir: options.store_dir.clone(),
        explain: None,
        agent: options.agent,
        scope: options.scope,
    };
    let base_scan = scan_project(&scan_options);
    let content_capture = if options.capture_content.unwrap_or(false) {
        capture_content_backed_evidence(&base_scan.evidence, options)?
    } else {
        ContentCapture {
            evidence: base_scan.evidence.clone(),
            content: Vec::new(),
        }
    };

    let scan = crate::types::ScanResult {
        trust: base_scan.trust,
        evidence: content_capture.evidence.clone(),
        blind_spots: base_scan.blind_spots,
    };

    let graph = build_graph(&scan.evidence);
    let audit_findings = store_findings
        .clone()
        .into_iter()
        .chain(audit_evidence(&scan.evidence, &graph))
        .collect();
    let provenance = build_provenance(&graph, &scan.evidence);
    let manifest = SnapshotManifest {
        schema_version: "0.1".to_string(),
        name: name.to_string(),
        created_at: OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_string()),
        project_path: options.project_path.clone(),
        security: SnapshotSecurity {
            raw_secrets_included: false,
            redaction_policy: if options.capture_content.unwrap_or(false) {
                "content-backed".to_string()
            } else {
                "metadata-only".to_string()
            },
        },
    };

    let snapshot = Snapshot {
        manifest,
        evidence: scan.evidence.clone(),
        graph,
        audit_findings,
        provenance,
        content: if content_capture.content.is_empty() {
            None
        } else {
            Some(content_capture.content)
        },
    };

    Ok(CurrentState {
        scan,
        store_findings,
        snapshot,
    })
}

struct ContentCapture {
    evidence: Vec<DiscoveredItem>,
    content: Vec<SnapshotContentEntry>,
}

fn capture_content_backed_evidence(
    evidence: &[DiscoveredItem],
    options: &RuntimeOptions,
) -> Result<ContentCapture, std::io::Error> {
    let mut content = Vec::new();
    let mut by_evidence_id = std::collections::HashMap::new();

    for item in evidence {
        let Some(restore_path) = restore_path_for_content(item) else {
            continue;
        };
        if !is_user_global_content_candidate(item) {
            continue;
        }
        let Some(absolute_path) = absolute_path_for_source_path(&restore_path, options) else {
            continue;
        };
        let text = match fs::read_to_string(&absolute_path) {
            Ok(text) => text,
            Err(_) => continue,
        };

        let checksum = format!("sha256:{:x}", Sha256::digest(text.as_bytes()));
        let storage_path = format!(
            "content/{}.txt",
            safe_content_file_name(&item.id, &checksum)
        );

        let entry = if contains_secret_like_assignment(&text) {
            SnapshotContentEntry {
                evidence_id: item.id.clone(),
                source_path: item.source_path.clone(),
                restore_path: restore_path.clone(),
                checksum,
                byte_length: text.len() as u64,
                encoding: "utf8".to_string(),
                storage_path,
                capture_status: "omitted".to_string(),
                reason: Some("secret_like_assignment".to_string()),
                content: None,
            }
        } else {
            SnapshotContentEntry {
                evidence_id: item.id.clone(),
                source_path: item.source_path.clone(),
                restore_path: restore_path.clone(),
                checksum,
                byte_length: text.len() as u64,
                encoding: "utf8".to_string(),
                storage_path,
                capture_status: "captured".to_string(),
                reason: None,
                content: Some(text),
            }
        };

        by_evidence_id.insert(item.id.clone(), entry.clone());
        content.push(entry);
    }

    let updated_evidence = evidence
        .iter()
        .map(|item| {
            let Some(entry) = by_evidence_id.get(&item.id) else {
                return item.clone();
            };
            let mut metadata = item.metadata.clone().unwrap_or_else(|| serde_json::json!({}));
            if let Some(obj) = metadata.as_object_mut() {
                obj.insert(
                    "contentCaptureStatus".to_string(),
                    serde_json::Value::String(entry.capture_status.clone()),
                );
                obj.insert(
                    "contentRestorePath".to_string(),
                    serde_json::Value::String(entry.restore_path.clone()),
                );
                if let Some(reason) = &entry.reason {
                    obj.insert(
                        "contentCaptureReason".to_string(),
                        serde_json::Value::String(reason.clone()),
                    );
                }
            }
            DiscoveredItem {
                checksum: Some(entry.checksum.clone()),
                metadata: Some(metadata),
                ..item.clone()
            }
        })
        .collect();

    Ok(ContentCapture {
        evidence: updated_evidence,
        content,
    })
}

fn is_user_global_content_candidate(item: &DiscoveredItem) -> bool {
    if item.scope != EvidenceScope::User {
        return false;
    }
    if item.capture_status != CaptureStatus::Captured {
        return false;
    }
    if !item.source_path.starts_with("~/") {
        return false;
    }
    // M1: ~/.claude.json MCP inventory stays metadata-only.
    if item.source_path == "~/.claude.json" {
        return false;
    }
    if !matches!(
        item.kind,
        EvidenceKind::AgentConfig | EvidenceKind::Skill | EvidenceKind::Hook
    ) {
        return false;
    }
    user_global_path_for_agent(item.agent, &item.source_path)
}

fn user_global_path_for_agent(agent: AgentId, source_path: &str) -> bool {
    match agent {
        AgentId::Codex => source_path.starts_with("~/.codex/"),
        AgentId::ClaudeCode => source_path.starts_with("~/.claude/"),
        AgentId::Cursor => {
            source_path.starts_with("~/.cursor/") || source_path.starts_with("~/.agents/")
        }
        AgentId::Opencode => {
            source_path.starts_with("~/.config/opencode/")
                || source_path.starts_with("~/.claude/skills/")
                || source_path.starts_with("~/.codex/skills/")
        }
        AgentId::PiAgent => {
            source_path.starts_with("~/.pi/") || source_path.starts_with("~/.agents/")
        }
        _ => false,
    }
}

fn restore_path_for_content(item: &DiscoveredItem) -> Option<String> {
    if item.kind == crate::types::EvidenceKind::Skill {
        let entrypoint = item
            .metadata
            .as_ref()
            .and_then(|m| m.get("entrypoint"))
            .and_then(|v| v.as_str())
            .unwrap_or("SKILL.md");
        return Some(format!("{}/{}", item.source_path, entrypoint));
    }
    Some(item.source_path.clone())
}

fn absolute_path_for_source_path(source_path: &str, options: &RuntimeOptions) -> Option<std::path::PathBuf> {
    if source_path == "~" {
        return Some(Path::new(&options.home_dir).to_path_buf());
    }
    if let Some(rest) = source_path.strip_prefix("~/") {
        return Some(Path::new(&options.home_dir).join(rest));
    }
    if Path::new(source_path).is_absolute() {
        return Some(Path::new(source_path).to_path_buf());
    }
    if Regex::new(r"^[a-z_]+:").ok()?.is_match(source_path) {
        return None;
    }
    Some(
        Path::new(&options.project_path)
            .join(source_path)
            .canonicalize()
            .unwrap_or_else(|_| Path::new(&options.project_path).join(source_path)),
    )
}

fn safe_content_file_name(evidence_id: &str, checksum: &str) -> String {
    let suffix = checksum
        .strip_prefix("sha256:")
        .unwrap_or(checksum)
        .chars()
        .take(12)
        .collect::<String>();
    let mut name = format!("{evidence_id}-{suffix}");
    name = Regex::new(r"[^A-Za-z0-9_.-]+")
        .expect("regex")
        .replace_all(&name, ".")
        .to_string();
    name.trim_matches('.').to_lowercase()
}

fn contains_secret_like_assignment(text: &str) -> bool {
    Regex::new(r"(?i)(?:api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)\s*=")
        .expect("regex")
        .is_match(text)
}