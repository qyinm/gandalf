use std::fs;

use gandalf_core::{
    capture_current_state, types::{AgentId, EvidenceKind, EvidenceScope, RuntimeOptions},
};
use tempfile::TempDir;

#[test]
fn captures_claude_user_global_settings_with_content_backing() {
    let root = TempDir::new().expect("temp dir");
    let project_path = root.path().join("project");
    let home_dir = root.path().join("home");
    let store_dir = home_dir.join(".gandalf");
    fs::create_dir_all(&project_path).expect("project dir");
    fs::create_dir_all(home_dir.join(".claude")).expect("claude dir");
    fs::write(
        home_dir.join(".claude/settings.json"),
        r#"{"permissions":{"allow":["Bash(echo hi)"]}}"#,
    )
    .expect("settings");

    let state = capture_current_state(
        &RuntimeOptions {
            project_path: project_path.display().to_string(),
            home_dir: home_dir.display().to_string(),
            store_dir: store_dir.display().to_string(),
            agent: Some(AgentId::ClaudeCode),
            scope: Some(EvidenceScope::User),
            capture_content: Some(true),
        },
        "claude-baseline",
    )
    .expect("capture");

    let content = state.snapshot.content.expect("content entries");
    assert!(
        content.iter().any(|entry| {
            entry.source_path == "~/.claude/settings.json" && entry.capture_status == "captured"
        }),
        "expected Claude settings.json content capture"
    );

    let settings = state
        .scan
        .evidence
        .iter()
        .find(|item| {
            item.agent == AgentId::ClaudeCode
                && item.kind == EvidenceKind::AgentConfig
                && item.source_path == "~/.claude/settings.json"
        })
        .expect("settings evidence");
    assert_eq!(
        settings
            .metadata
            .as_ref()
            .and_then(|m| m.get("contentCaptureStatus"))
            .and_then(|v| v.as_str()),
        Some("captured")
    );
}

#[test]
fn skips_claude_json_metadata_only_from_content_capture() {
    let root = TempDir::new().expect("temp dir");
    let project_path = root.path().join("project");
    let home_dir = root.path().join("home");
    let store_dir = home_dir.join(".gandalf");
    fs::create_dir_all(&project_path).expect("project dir");
    fs::create_dir_all(&home_dir).expect("home dir");
    fs::write(
        home_dir.join(".claude.json"),
        r#"{"mcpServers":{"docs":{"command":"npx"}}}"#,
    )
    .expect("claude json");

    let state = capture_current_state(
        &RuntimeOptions {
            project_path: project_path.display().to_string(),
            home_dir: home_dir.display().to_string(),
            store_dir: store_dir.display().to_string(),
            agent: Some(AgentId::ClaudeCode),
            scope: Some(EvidenceScope::User),
            capture_content: Some(true),
        },
        "claude-json",
    )
    .expect("capture");

    assert!(
        state.snapshot.content.is_none()
            || state
                .snapshot
                .content
                .as_ref()
                .is_some_and(|entries| entries.is_empty()),
        "~/.claude.json must stay metadata-only"
    );
}