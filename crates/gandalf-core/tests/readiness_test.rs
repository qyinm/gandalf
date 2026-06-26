use std::collections::HashMap;
use std::fs;
use std::os::unix::fs::PermissionsExt;

use gandalf_core::{
    build_readiness_report, scan_project, types::{
        AgentId, CaptureStatus, DiscoveredItem, EvidenceConfidence, EvidenceKind,
        EvidenceParser, EvidenceScope, RestorePolicy, ScanOptions,
    },
    ReadinessCategory, ReadinessOptions,
};
use serde_json::json;
use tempfile::TempDir;

fn mcp_item(id: &str, value: serde_json::Value) -> DiscoveredItem {
    DiscoveredItem {
        id: id.to_string(),
        agent: AgentId::ClaudeCode,
        kind: EvidenceKind::McpServer,
        source_path: ".mcp.json".to_string(),
        scope: EvidenceScope::Project,
        precedence: 40,
        parser: EvidenceParser::Json,
        sensitivity: "command_config".to_string(),
        content_policy: "structured_safe_fields_only".to_string(),
        restore_policy: RestorePolicy::StructuredFieldsOnly,
        capture_status: CaptureStatus::Captured,
        confidence: EvidenceConfidence::High,
        name: Some(id.to_string()),
        value: Some(value),
        checksum: None,
        metadata: None,
    }
}

fn env_item(key: &str) -> DiscoveredItem {
    DiscoveredItem {
        id: format!("env.{key}"),
        agent: AgentId::Project,
        kind: EvidenceKind::EnvKey,
        source_path: ".env".to_string(),
        scope: EvidenceScope::Project,
        precedence: 40,
        parser: EvidenceParser::Dotenv,
        sensitivity: "env_key_inventory".to_string(),
        content_policy: "key_inventory_only".to_string(),
        restore_policy: RestorePolicy::KeyInventoryOnly,
        capture_status: CaptureStatus::Redacted,
        confidence: EvidenceConfidence::High,
        name: Some(key.to_string()),
        value: Some(json!({ "key": key })),
        checksum: None,
        metadata: None,
    }
}

#[test]
fn classifies_mcp_command_states_without_executing_shell_strings() {
    let root = TempDir::new().expect("temp dir");
    let marker_path = root.path().join("shell-marker");
    let malicious_command = format!("missing\" ; touch \"{}\" ; \"", marker_path.display());

    let report = build_readiness_report(
        &[
            mcp_item("mcp-remote", json!({ "url": "https://mcp.example.test" })),
            mcp_item(
                "mcp-local",
                json!({ "command": "/Users/source/.local/bin/private-mcp" }),
            ),
            mcp_item("mcp-malicious", json!({ "command": malicious_command })),
        ],
        &ReadinessOptions {
            source_home_dir: Some("/Users/source"),
            process_env: Some(&HashMap::new()),
            ..ReadinessOptions::default()
        },
    );

    assert_eq!(
        *report.summary.get(&ReadinessCategory::Unverified).unwrap_or(&0),
        1
    );
    assert_eq!(
        report
            .items
            .iter()
            .find(|item| item.evidence_id.as_deref() == Some("mcp-remote"))
            .map(|item| item.category),
        Some(ReadinessCategory::Unverified)
    );
    assert_eq!(
        report
            .items
            .iter()
            .find(|item| item.evidence_id.as_deref() == Some("mcp-local"))
            .map(|item| item.category),
        Some(ReadinessCategory::NeedsManualAction)
    );
    assert_eq!(
        report
            .items
            .iter()
            .find(|item| item.evidence_id.as_deref() == Some("mcp-malicious"))
            .map(|item| item.category),
        Some(ReadinessCategory::NeedsManualAction)
    );
    assert!(!marker_path.exists());
}

#[test]
fn redacts_credentials_embedded_in_remote_mcp_urls() {
    let report = build_readiness_report(
        &[mcp_item(
            "mcp-remote-secret",
            json!({
                "url": "https://token:secret@mcp.example.test/sse?api_key=sk-real-secret&mode=read#access_token=fragment-secret"
            }),
        )],
        &ReadinessOptions::default(),
    );
    let output = serde_json::to_string(&report).expect("serialize");

    assert!(!output.contains("token:secret"));
    assert!(!output.contains("sk-real-secret"));
    assert!(!output.contains("fragment-secret"));
    assert!(output.contains("api_key=%5Bredacted%5D"));
    assert!(output.contains("mode=read"));
}

#[test]
fn ignores_malformed_legacy_mcp_payload_fields() {
    let legacy_evidence = vec![DiscoveredItem {
        id: "legacy-bad-mcp".to_string(),
        agent: AgentId::ClaudeCode,
        kind: EvidenceKind::McpServer,
        source_path: ".mcp.json".to_string(),
        scope: EvidenceScope::Project,
        precedence: 40,
        parser: EvidenceParser::Json,
        sensitivity: "command_config".to_string(),
        content_policy: "structured_safe_fields_only".to_string(),
        restore_policy: RestorePolicy::StructuredFieldsOnly,
        capture_status: CaptureStatus::Captured,
        confidence: EvidenceConfidence::High,
        name: None,
        value: Some(json!({ "command": 123, "url": true, "args": ["--ok"] })),
        checksum: None,
        metadata: None,
    }];

    let report = build_readiness_report(
        &legacy_evidence,
        &ReadinessOptions {
            process_env: Some(&HashMap::new()),
            ..ReadinessOptions::default()
        },
    );

    assert!(!report
        .items
        .iter()
        .any(|item| item.evidence_id.as_deref() == Some("legacy-bad-mcp")));
}

#[test]
fn does_not_execute_path_hijacked_which_helper_during_command_lookup() {
    let root = TempDir::new().expect("temp dir");
    let marker_path = root.path().join("which-marker");
    let fake_which = root.path().join("definitely-missing-gandalf-tool");
    fs::write(&fake_which, "#!/bin/sh\ntouch marker\nexit 0\n").expect("write fake bin");
    let mut perms = fs::metadata(&fake_which).expect("metadata").permissions();
    perms.set_mode(0o755);
    fs::set_permissions(&fake_which, perms).expect("chmod");

    let previous_path = std::env::var("PATH").ok();
    let injected = format!(
        "{}{}",
        root.path().display(),
        std::path::MAIN_SEPARATOR
    );
    let new_path = previous_path
        .as_ref()
        .map(|path| format!("{injected}{path}"))
        .unwrap_or(injected);
    unsafe {
        std::env::set_var("PATH", &new_path);
    }

    let report = build_readiness_report(
        &[mcp_item(
            "mcp-missing",
            json!({ "command": "definitely-missing-gandalf-tool" }),
        )],
        &ReadinessOptions::default(),
    );

    if let Some(path) = previous_path {
        unsafe {
            std::env::set_var("PATH", path);
        }
    }

    assert!(report
        .items
        .iter()
        .any(|item| item.code == "GANDALF_MCP_COMMAND_MISSING"));
    assert!(!marker_path.exists());
}

#[test]
fn reports_missing_env_keys_by_name_only() {
    let report = build_readiness_report(
        &[
            env_item("OPENAI_API_KEY"),
            mcp_item(
                "mcp-env",
                json!({ "command": "npx", "envKeys": ["GITHUB_TOKEN"] }),
            ),
        ],
        &ReadinessOptions {
            target_evidence: Some(&[]),
            process_env: Some(&HashMap::new()),
            ..ReadinessOptions::default()
        },
    );

    let env_items: Vec<_> = report
        .items
        .iter()
        .filter(|entry| entry.code == "GANDALF_ENV_VALUE_REQUIRED")
        .collect();
    assert_eq!(env_items.len(), 2);
    assert!(env_items
        .iter()
        .any(|entry| entry.problem.contains("OPENAI_API_KEY")));
    assert!(env_items
        .iter()
        .any(|entry| entry.problem.contains("GITHUB_TOKEN")));
    let serialized = serde_json::to_string(&report).expect("serialize");
    assert!(!serialized.contains("sk-"));
}

#[test]
fn treats_target_env_inventory_and_process_env_as_satisfying_env_keys() {
    let process_env = HashMap::from([(
        "GITHUB_TOKEN".to_string(),
        "present-but-never-rendered".to_string(),
    )]);
    let report = build_readiness_report(
        &[env_item("OPENAI_API_KEY"), env_item("GITHUB_TOKEN")],
        &ReadinessOptions {
            target_evidence: Some(&[env_item("OPENAI_API_KEY")]),
            process_env: Some(&process_env),
            ..ReadinessOptions::default()
        },
    );

    assert!(!report
        .items
        .iter()
        .any(|entry| entry.code == "GANDALF_ENV_VALUE_REQUIRED"));
    let serialized = serde_json::to_string(&report).expect("serialize");
    assert!(!serialized.contains("present-but-never-rendered"));
}

#[test]
fn scans_current_project_env_keys_for_doctor_input() {
    let root = TempDir::new().expect("temp dir");
    let project_path = root.path().join("project");
    let home_dir = root.path().join("home");
    let store_dir = root.path().join("store");
    fs::create_dir_all(&project_path).expect("project dir");
    fs::create_dir_all(&home_dir).expect("home dir");
    fs::write(project_path.join(".env"), "OPENAI_API_KEY=secret\n").expect("env file");

    let scan = scan_project(&ScanOptions {
        project_path: project_path.display().to_string(),
        home_dir: home_dir.display().to_string(),
        store_dir: store_dir.display().to_string(),
        explain: None,
        agent: None,
        scope: None,
    });

    let report = build_readiness_report(
        &scan.evidence,
        &ReadinessOptions {
            target_evidence: Some(&scan.evidence),
            process_env: Some(&HashMap::new()),
            ..ReadinessOptions::default()
        },
    );

    assert!(!report
        .items
        .iter()
        .any(|entry| entry.code == "GANDALF_ENV_VALUE_REQUIRED"));
}