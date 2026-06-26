use std::fs;

use hem_core::{
    apply_agent_config, apply_mcp_server, apply_permission, apply_restore_items, apply_with_rollback,
    build_restore_plan, capture_current_state, create_default_apply_executor,
    create_default_undo_executor, default_apply_handler_registry, dispatch_default_apply,
    parse_dry_run_output, validate_constrained_write_path, write_snapshot, AgentId, ApplyOptions,
    ConfinementRoots, EvidenceScope, RestoreAction, RestoreItem, RestoreItemStatus, RestoreOptions,
    RuntimeOptions, StoreSnapshot,
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
            home_dir: None,
            project_path: None,
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
            home_dir: None,
            project_path: None,
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
            home_dir: None,
            project_path: None,
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

fn make_item(
    item_id: &str,
    item_type: &str,
    dest: &str,
    action: RestoreAction,
    target_content: Option<serde_json::Value>,
    metadata: Option<serde_json::Value>,
) -> RestoreItem {
    RestoreItem {
        item_id: item_id.to_string(),
        path: dest.to_string(),
        item_type: item_type.to_string(),
        source: dest.to_string(),
        dest: dest.to_string(),
        action: Some(action),
        status: RestoreItemStatus::Pending,
        error_message: None,
        skip_reason: None,
        execution_order: 1,
        rollback_state: None,
        target_content,
        can_rollback: true,
        metadata,
        apply_at: None,
    }
}

#[test]
fn default_registry_dispatches_mcp_permission_and_env_handlers() {
    let (_root, project_path, home_dir, _store_dir) = make_restore_sandbox();
    fs::create_dir_all(format!("{home_dir}/.claude")).expect("mkdir claude");
    let env_path = format!("{project_path}/.env");
    for (item_type, mut item) in [
        (
            "mcp_server",
            make_item(
                "mcp_server:docs:abcd1234",
                "mcp_server",
                &format!("{project_path}/.mcp.json"),
                RestoreAction::Update,
                Some(serde_json::json!({ "command": "x" })),
                Some(serde_json::json!({
                    "serverName": "docs",
                    "mcpPath": format!("{project_path}/.mcp.json")
                })),
            ),
        ),
        (
            "permission",
            make_item(
                "permission.bash",
                "permission",
                &format!("{home_dir}/.claude/settings.json"),
                RestoreAction::Update,
                Some(serde_json::json!({ "allow": [] })),
                None,
            ),
        ),
        (
            "env_key",
            make_item(
                "env_key.TEST",
                "env_key",
                &format!("{project_path}/env:TEST"),
                RestoreAction::Update,
                Some(serde_json::Value::String("v".to_string())),
                None,
            ),
        ),
    ] {
        dispatch_default_apply(&mut item).unwrap_or_else(|error| {
            panic!("handler for {item_type} failed: {error}");
        });
    }
    let _ = default_apply_handler_registry();
    assert!(std::path::Path::new(&env_path).exists());
}

#[test]
fn applies_mcp_server_to_project_mcp_json() {
    let (_root, project_path, home_dir, _store_dir) = make_restore_sandbox();
    let mcp_path = format!("{project_path}/.mcp.json");
    fs::write(&mcp_path, "{\n  \"mcpServers\": {}\n}\n").expect("seed mcp");

    let mut item = make_item(
        "mcp_server:docs:abcd1234",
        "mcp_server",
        &mcp_path,
        RestoreAction::Update,
        Some(serde_json::json!({
            "command": "docs-old",
            "args": []
        })),
        Some(serde_json::json!({
            "serverName": "docs",
            "mcpPath": mcp_path
        })),
    );
    apply_mcp_server(&mut item).expect("apply mcp");
    let written = fs::read_to_string(&mcp_path).expect("read mcp");
    assert!(written.contains("\"docs\""));
    assert!(written.contains("docs-old"));
}

#[test]
fn applies_permission_rule_to_settings_json() {
    let (_root, _project_path, home_dir, _store_dir) = make_restore_sandbox();
    let settings_path = format!("{home_dir}/.claude/settings.json");
    fs::create_dir_all(format!("{home_dir}/.claude")).expect("mkdir");
    fs::write(&settings_path, "{\n  \"permissions\": {}\n}\n").expect("seed settings");

    let mut item = make_item(
        "permission.bash",
        "permission",
        &settings_path,
        RestoreAction::Update,
        Some(serde_json::json!({
            "allow": ["Bash"]
        })),
        None,
    );
    apply_permission(&mut item).expect("apply permission");
    let written = fs::read_to_string(&settings_path).expect("read settings");
    assert!(written.contains("\"bash\""));
    assert!(written.contains("Bash"));
}

#[test]
fn applies_env_key_to_project_dotenv() {
    let (_root, project_path, _home_dir, _store_dir) = make_restore_sandbox();
    let env_path = format!("{project_path}/.env");

    let mut item = make_item(
        "env_key.API_KEY",
        "env_key",
        &format!("{project_path}/env:API_KEY"),
        RestoreAction::Update,
        Some(serde_json::Value::String("secret-value".to_string())),
        None,
    );
    dispatch_default_apply(&mut item).expect("apply env");
    let written = fs::read_to_string(&env_path).expect("read env");
    assert!(written.contains("API_KEY=secret-value"));
}

#[test]
fn rejects_restore_apply_outside_confinement_roots() {
    let (_root, project_path, home_dir, _store_dir) = make_restore_sandbox();
    let outside = "/etc/hem-restore-test-target";
    let mut item = make_item(
        "agent_config:outside",
        "agent_config",
        outside,
        RestoreAction::Update,
        Some(serde_json::Value::String("should-not-write".to_string())),
        None,
    );
    let mut executor = create_default_apply_executor();
    let summary = apply_restore_items(
        &mut [item],
        &mut executor,
        &ApplyOptions {
            fail_fast: true,
            rollback: None,
            home_dir: Some(home_dir),
            project_path: Some(project_path),
        },
    );
    assert_eq!(summary.successful, 0);
    assert_eq!(summary.failed, 1);
    assert!(summary.failures[0].reason.contains("outside home and project"));
}

#[test]
fn parse_dry_run_skips_destinations_with_traversal() {
    let plan_json = serde_json::json!({
        "targetProject": "/tmp/project",
        "targetHome": "/tmp/home",
        "items": [{
            "itemId": "agent_config:test:abcd",
            "agent": "codex",
            "kind": "agent_config",
            "sourcePath": "~/../../etc/passwd",
            "dependsOn": [],
            "action": "update",
            "diff": { "changes": [], "additions": [], "removals": [] },
            "riskLevel": "low",
            "riskReason": "test",
            "needsConfirmation": false,
            "confirmationPrompt": "",
            "rollbackInstruction": "reverse",
            "targetState": {
                "id": "cfg-1",
                "agent": "codex",
                "kind": "agent_config",
                "sourcePath": "~/../../etc/passwd",
                "scope": "user",
                "precedence": 1,
                "parser": "toml",
                "sensitivity": "low",
                "contentPolicy": "content_backed",
                "restorePolicy": "full_content_supported",
                "captureStatus": "captured",
                "confidence": "high",
                "value": "model = \"x\"\n"
            }
        }],
        "executionOrder": ["agent_config:test:abcd"]
    });
    let parsed = parse_dry_run_output(&plan_json.to_string());
    assert!(parsed.items.is_empty());
    assert!(parsed.errors.iter().any(|error| error.message.contains("traversal")));
}

#[test]
fn apply_with_rollback_restores_mcp_json_after_apply() {
    let (_root, project_path, _home_dir, _store_dir) = make_restore_sandbox();
    let mcp_path = format!("{project_path}/.mcp.json");
    let baseline = "{\n  \"mcpServers\": {\n    \"docs\": { \"command\": \"baseline\" }\n  }\n}\n";
    fs::write(&mcp_path, baseline).expect("seed mcp");

    let mut item = make_item(
        "mcp_server:docs:abcd1234",
        "mcp_server",
        &mcp_path,
        RestoreAction::Update,
        Some(serde_json::json!({ "command": "changed" })),
        Some(serde_json::json!({
            "serverName": "docs",
            "mcpPath": mcp_path
        })),
    );
    let mut apply_executor = create_default_apply_executor();
    let mut undo_executor = create_default_undo_executor();
    let result = apply_with_rollback(
        &mut [item],
        &mut apply_executor,
        &mut undo_executor,
        &ApplyOptions {
            fail_fast: true,
            rollback: Some(true),
            home_dir: None,
            project_path: None,
        },
    );
    assert_eq!(result.apply_summary.successful, 1);
    assert!(result.rollback_summary.is_some());
    assert_eq!(fs::read_to_string(&mcp_path).expect("read mcp"), baseline);
}

#[test]
fn validate_constrained_write_path_rejects_blocked_home_prefix() {
    let roots = ConfinementRoots {
        home_dir: std::path::PathBuf::from("/home/user"),
        project_path: std::path::PathBuf::from("/home/user/project"),
    };
    let err = validate_constrained_write_path(
        std::path::Path::new("/home/user/.ssh/id_rsa"),
        &roots,
    )
    .expect_err("blocked");
    assert!(err.contains("Blocked"));
}
