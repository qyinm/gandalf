use gandalf_core::{
    errors::format_snap_error,
    policy::{
        capture_status_for_key, ignored_directory, is_secret_like_key, redact_structured_value,
        restore_policy_for,
    },
    types::{AgentId, DiscoveredItem, EvidenceKind, EvidenceScope, RestorePolicy},
};
use serde_json::json;
use std::fs;
use std::path::PathBuf;

fn fixtures_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures/evidence")
}

#[test]
fn restore_policy_for_agent_config_is_full_content_supported() {
    assert_eq!(
        restore_policy_for(EvidenceKind::AgentConfig),
        RestorePolicy::FullContentSupported
    );
}

#[test]
fn restore_policy_for_mcp_server_is_structured_fields_only() {
    assert_eq!(
        restore_policy_for(EvidenceKind::McpServer),
        RestorePolicy::StructuredFieldsOnly
    );
}

#[test]
fn restore_policy_for_env_key_is_key_inventory_only() {
    assert_eq!(
        restore_policy_for(EvidenceKind::EnvKey),
        RestorePolicy::KeyInventoryOnly
    );
}

#[test]
fn restore_policy_for_symlink_is_not_supported() {
    assert_eq!(
        restore_policy_for(EvidenceKind::Symlink),
        RestorePolicy::NotSupported
    );
}

#[test]
fn secret_like_keys_trigger_redaction_policy() {
    assert!(is_secret_like_key("OPENAI_API_KEY"));
    assert!(is_secret_like_key("github_token"));
    assert!(is_secret_like_key("client-secret"));
    assert!(!is_secret_like_key("MODEL_NAME"));
    assert_eq!(capture_status_for_key("OPENAI_API_KEY"), "redacted");
    assert_eq!(capture_status_for_key("MODEL_NAME"), "omitted");
}

#[test]
fn redact_structured_value_redacts_secrets_and_expands_env_keys() {
    let input = json!({
        "command": "npx",
        "api_key": "secret-value",
        "env": {
            "GITHUB_TOKEN": "abc",
            "MODEL": "gpt-5"
        }
    });

    let redacted = redact_structured_value(input);
    assert_eq!(redacted["api_key"], "[redacted]");
    assert_eq!(redacted["command"], "npx");
    assert_eq!(redacted["envKeys"], json!(["GITHUB_TOKEN", "MODEL"]));
    assert!(redacted.get("env").is_none());
}

#[test]
fn ignored_directory_matches_ts_policy_set() {
    assert!(ignored_directory("node_modules"));
    assert!(ignored_directory(".git"));
    assert!(!ignored_directory("src"));
}

#[test]
fn evidence_fixture_round_trips_through_serde() {
    for file in ["user-permission-bash.json", "project-permission-bash.json"] {
        let path = fixtures_dir().join(file);
        let raw = fs::read_to_string(&path).expect("fixture should exist");
        let item: DiscoveredItem = serde_json::from_str(&raw).expect("fixture should deserialize");
        let serialized = serde_json::to_string(&item).expect("item should serialize");
        let round_trip: DiscoveredItem =
            serde_json::from_str(&serialized).expect("round trip should deserialize");
        assert_eq!(item, round_trip);
    }
}

#[test]
fn evidence_fixture_scopes_match_user_and_project_boundary() {
    let user: DiscoveredItem = serde_json::from_str(&fs::read_to_string(
        fixtures_dir().join("user-permission-bash.json"),
    )
    .unwrap())
    .unwrap();
    let project: DiscoveredItem = serde_json::from_str(&fs::read_to_string(
        fixtures_dir().join("project-permission-bash.json"),
    )
    .unwrap())
    .unwrap();

    assert_eq!(user.scope, EvidenceScope::User);
    assert_eq!(user.source_path, "~/.claude/settings.json");
    assert_eq!(project.scope, EvidenceScope::Project);
    assert_eq!(project.source_path, ".claude/settings.json");
}

#[test]
fn invalid_agent_id_deserializes_to_unknown() {
    let item: DiscoveredItem = serde_json::from_value(json!({
        "id": "unknown-agent",
        "agent": "not-a-real-agent",
        "kind": "permission",
        "sourcePath": ".claude/settings.json",
        "scope": "project",
        "precedence": 40,
        "parser": "json",
        "sensitivity": "command_config",
        "contentPolicy": "structured_safe_fields_only",
        "restorePolicy": "structured_fields_only",
        "captureStatus": "captured",
        "confidence": "high"
    }))
    .unwrap();

    assert_eq!(item.agent, AgentId::Unknown);
}

#[test]
fn format_snap_error_matches_ts_contract() {
    let output = format_snap_error(&gandalf_core::SnapError {
        code: "GANDALF_PARSE_FAILED".to_string(),
        problem: "Could not parse Codex config.".to_string(),
        cause: "TOML syntax error at line 12.".to_string(),
        fix: "Run `gandalf scan --skip codex` or fix the TOML file.".to_string(),
        path: Some("~/.codex/config.toml".to_string()),
    });

    assert!(output.starts_with("GANDALF_PARSE_FAILED"));
    assert!(output.contains("Problem: Could not parse Codex config."));
    assert!(output.contains("Cause: TOML syntax error at line 12."));
    assert!(output.contains("Fix: Run `gandalf scan --skip codex` or fix the TOML file."));
    assert!(output.contains("Path: ~/.codex/config.toml"));
}