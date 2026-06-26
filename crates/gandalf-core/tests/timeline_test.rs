use std::fs;

use gandalf_core::{
    build_timeline_undo_plan, capture_timeline_snapshot, list_timeline_entries, read_snapshot,
    types::RuntimeOptions, BuildTimelineUndoOptions, CaptureTimelineOptions,
    TimelineEntryEventKind, TimelineRestoreReadiness,
};
use tempfile::TempDir;

fn make_runtime() -> (TempDir, RuntimeOptions) {
    let root = TempDir::new().expect("temp dir");
    let project_path = root.path().join("project");
    let home_dir = root.path().join("home");
    let store_dir = root.path().join("store");
    fs::create_dir_all(&project_path).expect("project dir");
    fs::create_dir_all(&home_dir).expect("home dir");
    (
        root,
        RuntimeOptions {
            project_path: project_path.display().to_string(),
            home_dir: home_dir.display().to_string(),
            store_dir: store_dir.display().to_string(),
            agent: None,
            scope: None,
            capture_content: None,
        },
    )
}

fn write_mcp(project_path: &std::path::Path, command: &str) {
    let payload = serde_json::json!({
        "mcpServers": {
            "github": {
                "transport": "stdio",
                "command": command
            }
        }
    });
    fs::write(
        project_path.join(".mcp.json"),
        serde_json::to_string_pretty(&payload).expect("serialize mcp"),
    )
    .expect("write mcp");
}

#[test]
fn creates_baseline_captures_partial_changes_and_builds_mcp_only_dry_run_undo() {
    let (root, options) = make_runtime();
    write_mcp(&root.path().join("project"), "gh-mcp");

    let baseline = capture_timeline_snapshot(
        &options,
        &CaptureTimelineOptions {
            capture_id: Some("capture-test".to_string()),
            snapshot_name: Some("manual-baseline-capture-test".to_string()),
            title: None,
            skip_unchanged: false,
        },
    )
    .expect("baseline capture");

    assert!(baseline.written);
    let entry = baseline.entry.expect("baseline entry");
    assert_eq!(entry.event_kind, TimelineEntryEventKind::Baseline);
    assert_eq!(entry.restore_readiness, TimelineRestoreReadiness::ObserveOnly);
    assert_eq!(
        list_timeline_entries(
            root.path().join("store").as_path(),
            gandalf_core::TimelineListOptions {
                agent: None,
                project_path: None,
                limit: None,
                on_corrupt_entry: None,
            }
        )
        .expect("list timeline")
        .len(),
        1
    );
    assert_eq!(
        read_snapshot(
            root.path().join("store").as_path(),
            "manual-baseline-capture-test",
            None
        )
        .expect("read snapshot")
        .manifest
        .name,
        "manual-baseline-capture-test"
    );

    write_mcp(&root.path().join("project"), "gh-mcp-v2");
    let skill_dir = root
        .path()
        .join("home")
        .join(".claude")
        .join("skills")
        .join("react-review");
    fs::create_dir_all(&skill_dir).expect("skill dir");
    fs::write(skill_dir.join("SKILL.md"), "# React Review\n").expect("skill file");

    let changed = capture_timeline_snapshot(
        &options,
        &CaptureTimelineOptions {
            capture_id: Some("capture-test".to_string()),
            snapshot_name: None,
            title: None,
            skip_unchanged: true,
        },
    )
    .expect("changed capture");

    assert!(changed.written);
    let changed_entry = changed.entry.expect("changed entry");
    assert_eq!(changed_entry.event_kind, TimelineEntryEventKind::SetupChanged);
    assert_eq!(
        changed_entry.restore_readiness,
        TimelineRestoreReadiness::Partial
    );
    assert!(changed_entry.changed_surfaces.iter().any(|surface| {
        surface.kind == "mcp_server" && surface.restorable
    }));
    assert!(changed_entry.changed_surfaces.iter().any(|surface| {
        surface.kind != "mcp_server" && surface.observe_only
    }));

    let undo = build_timeline_undo_plan(
        root.path().join("store").as_path(),
        &changed_entry.id,
        BuildTimelineUndoOptions::default(),
    )
    .expect("undo plan");
    assert!(undo.dry_run);
    assert!(!undo.writes_files);
    assert_eq!(undo.writable_items.len(), 1);
    assert_eq!(undo.writable_items[0].action.as_str(), "update");
    assert_eq!(undo.writable_items[0].server_name, "github");
    assert!(undo.observe_only_surfaces.len() >= 1);

    let unchanged = capture_timeline_snapshot(
        &options,
        &CaptureTimelineOptions {
            capture_id: Some("capture-test".to_string()),
            snapshot_name: None,
            title: None,
            skip_unchanged: true,
        },
    )
    .expect("unchanged capture");
    assert!(!unchanged.written);
    assert_eq!(unchanged.skipped_reason.as_deref(), Some("unchanged"));
}