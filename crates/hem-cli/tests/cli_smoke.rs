use std::fs;

use hem_cli::run_scan;
use hem_core::{
    types::{CaptureStatus, EvidenceKind, EvidenceScope, ScanOptions},
};
use tempfile::TempDir;

#[test]
fn scan_discovers_github_mcp_server() {
    let root = TempDir::new().expect("temp dir");
    let project_path = root.path().join("project");
    let home_dir = root.path().join("home");
    let store_dir = home_dir.join(".hem");
    fs::create_dir_all(&project_path).expect("project dir");
    fs::create_dir_all(&home_dir).expect("home dir");
    fs::write(
        project_path.join(".mcp.json"),
        r#"{"mcpServers":{"github":{"command":"gh","args":["api"],"env":{"GITHUB_TOKEN":"secret"}}}}"#,
    )
    .expect("write mcp");

    let scan = run_scan(&ScanOptions {
        project_path: project_path.display().to_string(),
        home_dir: home_dir.display().to_string(),
        store_dir: store_dir.display().to_string(),
        explain: None,
        agent: None,
        scope: None,
    });

    let github = scan
        .evidence
        .iter()
        .find(|item| {
            item.kind == EvidenceKind::McpServer
                && item.name.as_deref() == Some("github")
                && item.source_path == ".mcp.json"
                && item.scope == EvidenceScope::Project
                && item.capture_status == CaptureStatus::Captured
        })
        .expect("github mcp evidence");

    assert_eq!(github.kind, EvidenceKind::McpServer);
    assert_eq!(github.name.as_deref(), Some("github"));
}