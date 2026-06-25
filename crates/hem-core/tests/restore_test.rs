use std::fs;

use hem_core::{
    apply_agent_config, apply_restore_items, build_restore_plan, capture_current_state,
    create_default_apply_executor, parse_dry_run_output, write_snapshot, AgentId, ApplyOptions,
    EvidenceScope, RestoreAction, RestoreItem, RestoreItemStatus, RestoreOptions, RuntimeOptions,
    StoreSnapshot,
};
use tempfile::TempDir;

fn make_restore_sandbox() -> (TempDir, String, String, String) {
    let root = TempDir::new().expect("temp dir");
    let project_path = root.path().join("project");
    let home_dir = root.path().join("home");
    let store_dir = root.path().join("store");
    fs::create_dir_all(&project_path).expect("mkdir project");
    fs::create_dir_all(&home_dir).expect("mkdir home");
    (
        root,
        project_path.to_string_lossy().to_string(),
        home_dir.to_string_lossy().to_string(),
        store_dir.to_string_lossy().to_string(),
    )
}

#[test]
fn restores_codex_config_byte_for_byte_through_target_home() {
    let (_root, project_path, home_dir, store_dir) = make_restore_sandbox();
    let config_path = format!("{home_dir}/.codex/config.toml");
    let original = "model = \"gpt-5\"\napproval_policy = \"on-request\"\n";
    fs::create_dir_all(format!("{home_dir}/.codex")).expect("mkdir");
    fs::write(&config_path, original).expect("write config");

    let state = capture_current_state(
        &RuntimeOptions {
            project_path: project_path.clone(),
            home_dir: home_dir.clone(),
            store_dir: store_dir.clone(),
            agent: Some(AgentId::Codex),
            scope: Some(EvidenceScope::User),
            capture_content: Some(true),
        },
        "baseline",
    )
    .expect("capture");
    write_snapshot(
        std::path::Path::new(&store_dir),
        StoreSnapshot::from(state.snapshot),
        Some(AgentId::Codex),
    )
    .expect("write snapshot");
    fs::write(&config_path, "").expect("clear config");

    let plan = build_restore_plan(&RestoreOptions {
        source_snapshot: "baseline".to_string(),
        project_path,
        home_dir: home_dir.clone(),
        store_dir,
        dry_run: true,
        agent: Some(AgentId::Codex),
        scope: Some(EvidenceScope::User),
    })
    .expect("plan");

    let config_item = plan
        .items
        .iter()
        .find(|item| item.kind == hem_core::EvidenceKind::AgentConfig)
        .expect("config item");
    assert_eq!(config_item.action, RestoreAction::Update);
    assert_eq!(config_item.agent, AgentId::Codex);
    assert_eq!(config_item.source_path, "~/.codex/config.toml");
    assert_eq!(plan.target_home, home_dir);

    let parsed = parse_dry_run_output(&serde_json::to_string(&plan).expect("serialize plan"));
    assert!(parsed.errors.is_empty());
    let executable = parsed
        .items
        .iter()
        .find(|item| item.item_type == "agent_config")
        .expect("executable item");
    assert_eq!(executable.dest, config_path);
    assert_eq!(
        executable.target_content,
        Some(serde_json::Value::String(original.to_string()))
    );

    let mut items = parsed.items;
    let mut executor = create_default_apply_executor();
    let summary = apply_restore_items(
        &mut items,
        &mut executor,
        &ApplyOptions {
            fail_fast: true,
            rollback: None,
        },
    );
    assert_eq!(summary.failed, 0);
    assert_eq!(fs::read_to_string(&config_path).expect("read"), original);
}

#[test]
fn deletes_codex_user_skill_added_after_baseline() {
    let (_root, project_path, home_dir, store_dir) = make_restore_sandbox();
    fs::create_dir_all(format!("{home_dir}/.codex")).expect("mkdir");
    fs::write(
        format!("{home_dir}/.codex/config.toml"),
        "model = \"gpt-5\"\n",
    )
    .expect("write config");

    let state = capture_current_state(
        &RuntimeOptions {
            project_path: project_path.clone(),
            home_dir: home_dir.clone(),
            store_dir: store_dir.clone(),
            agent: Some(AgentId::Codex),
            scope: Some(EvidenceScope::User),
            capture_content: Some(true),
        },
        "baseline",
    )
    .expect("capture");
    write_snapshot(
        std::path::Path::new(&store_dir),
        StoreSnapshot::from(state.snapshot),
        Some(AgentId::Codex),
    )
    .expect("write snapshot");

    let skill_file = format!("{home_dir}/.codex/skills/unsafe/SKILL.md");
    fs::create_dir_all(format!("{home_dir}/.codex/skills/unsafe")).expect("mkdir skill");
    fs::write(&skill_file, "---\nname: unsafe\n---\n").expect("write skill");

    let plan = build_restore_plan(&RestoreOptions {
        source_snapshot: "baseline".to_string(),
        project_path,
        home_dir,
        store_dir,
        dry_run: true,
        agent: Some(AgentId::Codex),
        scope: Some(EvidenceScope::User),
    })
    .expect("plan");

    let skill_item = plan
        .items
        .iter()
        .find(|item| item.kind == hem_core::EvidenceKind::Skill && item.action == RestoreAction::Delete)
        .expect("skill delete item");
    assert_eq!(skill_item.agent, AgentId::Codex);
    assert_eq!(skill_item.source_path, "~/.codex/skills/unsafe/SKILL.md");

    let mut items = parse_dry_run_output(&serde_json::to_string(&plan).expect("serialize")).items;
    let mut executor = create_default_apply_executor();
    let summary = apply_restore_items(
        &mut items,
        &mut executor,
        &ApplyOptions {
            fail_fast: true,
            rollback: None,
        },
    );
    assert_eq!(summary.failed, 0);
    assert!(!std::path::Path::new(&skill_file).exists());
}

#[test]
fn marks_codex_toml_mcp_changes_unsupported_while_config_carries_restore() {
    let (_root, project_path, home_dir, store_dir) = make_restore_sandbox();
    let config_path = format!("{home_dir}/.codex/config.toml");
    fs::create_dir_all(format!("{home_dir}/.codex")).expect("mkdir");
    fs::write(
        &config_path,
        "model = \"gpt-5\"\n[mcp_servers.docs]\ncommand = \"docs-old\"\n",
    )
    .expect("write baseline");

    let state = capture_current_state(
        &RuntimeOptions {
            project_path: project_path.clone(),
            home_dir: home_dir.clone(),
            store_dir: store_dir.clone(),
            agent: Some(AgentId::Codex),
            scope: Some(EvidenceScope::User),
            capture_content: Some(true),
        },
        "baseline",
    )
    .expect("capture");
    write_snapshot(
        std::path::Path::new(&store_dir),
        StoreSnapshot::from(state.snapshot),
        Some(AgentId::Codex),
    )
    .expect("write snapshot");

    fs::write(
        &config_path,
        "model = \"gpt-5\"\n[mcp_servers.docs]\ncommand = \"docs-new\"\n",
    )
    .expect("write changed config");

    let plan = build_restore_plan(&RestoreOptions {
        source_snapshot: "baseline".to_string(),
        project_path: project_path.clone(),
        home_dir: home_dir.clone(),
        store_dir: store_dir.clone(),
        dry_run: true,
        agent: Some(AgentId::Codex),
        scope: Some(EvidenceScope::User),
    })
    .expect("plan");
    assert!(plan
        .items
        .iter()
        .any(|item| item.kind == hem_core::EvidenceKind::AgentConfig && item.action == RestoreAction::Update));
    assert!(plan.unsupported_items.iter().any(|item| {
        item.kind == hem_core::EvidenceKind::McpServer
            && item.agent == AgentId::Codex
            && item.reason.contains("No supported restore action")
    }));
}

#[test]
fn metadata_only_snapshot_refuses_agent_config_apply_without_content_backing() {
    let (_root, project_path, home_dir, store_dir) = make_restore_sandbox();
    let config_path = format!("{home_dir}/.codex/config.toml");
    let original = "model = \"gpt-5\"\napproval_policy = \"on-request\"\n";
    fs::create_dir_all(format!("{home_dir}/.codex")).expect("mkdir");
    fs::write(&config_path, original).expect("write config");

    let state = capture_current_state(
        &RuntimeOptions {
            project_path: project_path.clone(),
            home_dir: home_dir.clone(),
            store_dir: store_dir.clone(),
            agent: Some(AgentId::Codex),
            scope: Some(EvidenceScope::User),
            capture_content: Some(false),
        },
        "baseline",
    )
    .expect("capture");
    assert!(state.snapshot.content.is_none());

    write_snapshot(
        std::path::Path::new(&store_dir),
        StoreSnapshot::from(state.snapshot),
        Some(AgentId::Codex),
    )
    .expect("write snapshot");
    fs::write(&config_path, "").expect("clear config");

    let plan = build_restore_plan(&RestoreOptions {
        source_snapshot: "baseline".to_string(),
        project_path,
        home_dir: home_dir.clone(),
        store_dir,
        dry_run: true,
        agent: Some(AgentId::Codex),
        scope: Some(EvidenceScope::User),
    })
    .expect("plan");

    let config_plan_item = plan
        .items
        .iter()
        .find(|item| item.kind == hem_core::EvidenceKind::AgentConfig)
        .expect("config plan item");
    assert!(config_plan_item.target_state.as_ref().unwrap().value.is_some());

    let parsed = parse_dry_run_output(&serde_json::to_string(&plan).expect("serialize plan"));
    let executable = parsed
        .items
        .iter()
        .find(|item| item.item_type == "agent_config")
        .expect("executable item");
    assert!(
        executable.target_content.is_none(),
        "metadata-only snapshots must not populate string file content for apply"
    );

    let mut items = parsed.items;
    let mut executor = create_default_apply_executor();
    let summary = apply_restore_items(
        &mut items,
        &mut executor,
        &ApplyOptions {
            fail_fast: true,
            rollback: None,
        },
    );
    assert_eq!(summary.successful, 0);
    assert_eq!(summary.failed, 1);
    assert!(
        summary.failures[0]
            .reason
            .contains("Missing target content"),
        "unexpected failure: {}",
        summary.failures[0].reason
    );
    assert_ne!(fs::read_to_string(&config_path).expect("read"), original);
}

#[test]
fn writes_agent_config_content_without_appending_newline() {
    let (_root, _project_path, home_dir, _store_dir) = make_restore_sandbox();
    let config_path = format!("{home_dir}/.codex/config.toml");
    let mut item = RestoreItem {
        item_id: "config".to_string(),
        path: "~/.codex/config.toml".to_string(),
        item_type: "agent_config".to_string(),
        source: "~/.codex/config.toml".to_string(),
        dest: config_path.clone(),
        action: Some(RestoreAction::Update),
        status: RestoreItemStatus::Pending,
        error_message: None,
        skip_reason: None,
        execution_order: 1,
        rollback_state: None,
        target_content: Some(serde_json::Value::String("model = \"gpt-5\"".to_string())),
        can_rollback: true,
        metadata: None,
        apply_at: None,
    };
    apply_agent_config(&mut item).expect("apply");
    assert_eq!(
        fs::read_to_string(&config_path).expect("read"),
        "model = \"gpt-5\""
    );
}
