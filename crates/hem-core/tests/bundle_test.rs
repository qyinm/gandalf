use std::fs;
use std::path::{Path, PathBuf};

use hem_core::{
    bundle_export, bundle_import, bundle_inspect, bundle_verify, list_snapshots, read_snapshot,
    read_tar, write_snapshot, write_tar, AgentId, AuditFinding, BundleExportOptions,
    BundleImportOptions, BundleVerifyOptions, CaptureStatus, DiscoveredItem, EvidenceConfidence,
    EvidenceKind, EvidenceParser, EvidenceScope, GraphNode, ProvenanceEntry, RestorePolicy,
    Severity, Snapshot, SnapshotManifest, SnapshotSecurity, StoreSnapshot, TarEntry, TarEntryType,
};
use serde_json::json;
use tempfile::TempDir;

struct Sandbox {
    _temp: TempDir,
    root: PathBuf,
    store_dir: PathBuf,
    project_path: PathBuf,
    home_dir: PathBuf,
}

fn make_sandbox() -> Sandbox {
    let root = TempDir::new().expect("temp dir");
    let root_path = root.path().to_path_buf();
    let store_dir = root_path.join("store");
    let project_path = root_path.join("project");
    let home_dir = root_path.join("home");
    fs::create_dir_all(&store_dir).expect("store dir");
    fs::create_dir_all(&project_path).expect("project dir");
    fs::create_dir_all(&home_dir).expect("home dir");
    Sandbox {
        _temp: root,
        root: root_path,
        store_dir,
        project_path,
        home_dir,
    }
}

fn sample_snapshot(name: &str) -> Snapshot {
    Snapshot {
        manifest: SnapshotManifest {
            schema_version: "0.1".to_string(),
            name: name.to_string(),
            created_at: "2026-05-12T00:00:00.000Z".to_string(),
            project_path: "/tmp/project".to_string(),
            security: SnapshotSecurity {
                raw_secrets_included: false,
                redaction_policy: "metadata-only".to_string(),
            },
        },
        evidence: vec![DiscoveredItem {
            id: "project.claude-code..mcp.json.mcp-github".to_string(),
            agent: AgentId::ClaudeCode,
            kind: EvidenceKind::McpServer,
            source_path: ".mcp.json".to_string(),
            scope: EvidenceScope::Project,
            precedence: 40,
            parser: EvidenceParser::Json,
            sensitivity: "command_config".to_string(),
            content_policy: "structured_safe_fields_only".to_string(),
            restore_policy: RestorePolicy::NotSupported,
            capture_status: CaptureStatus::Captured,
            confidence: EvidenceConfidence::High,
            name: Some("github".to_string()),
            value: Some(json!({ "command": "gh", "args": ["api"] })),
            checksum: None,
            metadata: None,
        }],
        graph: vec![GraphNode {
            id: "node.project.claude-code.mcp_server.github".to_string(),
            agent: AgentId::ClaudeCode,
            scope: EvidenceScope::Project,
            source_path: ".mcp.json".to_string(),
            entity_kind: EvidenceKind::McpServer,
            entity_name: "github".to_string(),
            effective_value: json!({ "command": "gh", "args": ["api"] }),
            overridden_by: None,
            confidence: EvidenceConfidence::High,
            evidence_id: "project.claude-code..mcp.json.mcp-github".to_string(),
        }],
        audit_findings: vec![AuditFinding {
            code: "EXECUTABLE_CONFIG_ADDED".to_string(),
            severity: Severity::High,
            problem: "MCP server references an executable command.".to_string(),
            cause: ".mcp.json github: command = gh.".to_string(),
            fix: "Confirm the command is trusted.".to_string(),
            path: Some(".mcp.json".to_string()),
            evidence_id: Some("project.claude-code..mcp.json.mcp-github".to_string()),
        }],
        provenance: vec![ProvenanceEntry {
            node_id: "node.project.claude-code.mcp_server.github".to_string(),
            evidence_id: "project.claude-code..mcp.json.mcp-github".to_string(),
            source_path: ".mcp.json".to_string(),
            scope: EvidenceScope::Project,
            precedence: 40,
            confidence: EvidenceConfidence::High,
            capture_status: CaptureStatus::Captured,
        }],
        content: None,
    }
}

fn bundle_path(box_: &Sandbox, name: &str) -> PathBuf {
    box_.root.join(format!("{name}.hem"))
}

fn make_minimal_bundle(box_: &Sandbox, snapshot_name: &str, content_entries: Vec<TarEntry>) -> Vec<TarEntry> {
    let manifest = json!({
        "formatVersion": 1,
        "snapshotName": snapshot_name,
        "createdAt": "2026-05-12T00:00:00.000Z",
        "projectPath": box_.project_path.to_string_lossy(),
        "includesContent": true,
        "contentFileCount": content_entries.len(),
        "contentTotalBytes": 0,
        "security": {
            "rawSecretsIncluded": false,
            "redactionPolicy": "metadata-only",
            "signed": false
        }
    });
    let mut entries = vec![
        dir_entry(".hem/"),
        file_entry(".hem/format-version", b"1\n"),
        file_entry(
            ".hem/manifest.json",
            format!("{}\n", serde_json::to_string(&manifest).unwrap()).as_bytes(),
        ),
    ];
    entries.extend(content_entries);
    entries
}

fn dir_entry(path: &str) -> TarEntry {
    TarEntry {
        path: path.to_string(),
        content: Vec::new(),
        mode: 0o755,
        mtime: 1_000_000,
        entry_type: TarEntryType::Directory,
    }
}

fn file_entry(path: &str, content: &[u8]) -> TarEntry {
    TarEntry {
        path: path.to_string(),
        content: content.to_vec(),
        mode: 0o644,
        mtime: 1_000_000,
        entry_type: TarEntryType::File,
    }
}

fn export_options(box_: &Sandbox, name: &str) -> BundleExportOptions {
    BundleExportOptions {
        snapshot_name: name.to_string(),
        output_path: bundle_path(box_, name).to_string_lossy().to_string(),
        store_dir: box_.store_dir.to_string_lossy().to_string(),
        project_path: box_.project_path.to_string_lossy().to_string(),
        home_dir: box_.home_dir.to_string_lossy().to_string(),
        include_content: None,
        signature_key: None,
        agent: None,
    }
}

fn import_options(box_: &Sandbox, bundle: &Path) -> BundleImportOptions {
    BundleImportOptions {
        bundle_path: bundle.to_string_lossy().to_string(),
        store_dir: box_.store_dir.to_string_lossy().to_string(),
        project_path: box_.project_path.to_string_lossy().to_string(),
        home_dir: box_.home_dir.to_string_lossy().to_string(),
        apply_content: None,
        dry_run: None,
        quarantine: None,
        trust: None,
        signature_key: None,
        agent: None,
        target_platform: None,
    }
}

#[test]
fn export_import_roundtrip_preserves_evidence() {
    let box_ = make_sandbox();
    let name = "roundtrip-test";
    write_snapshot(&box_.store_dir, StoreSnapshot::from(sample_snapshot(name)), None)
        .expect("write snapshot");

    let export_result = bundle_export(&export_options(&box_, name)).expect("export");
    assert!(!export_result.checksum.is_empty());
    assert!(export_result.bundle_path.ends_with(".hem"));

    let import_result = bundle_import(&import_options(&box_, Path::new(&export_result.bundle_path)))
        .expect("import");
    assert_eq!(import_result.snapshot_name, name);
    assert_eq!(import_result.evidence_count, 1);
    assert!(!import_result.content_applied);

    let imported = read_snapshot(&box_.store_dir, name, None).expect("read imported");
    assert_eq!(imported.evidence, sample_snapshot(name).evidence);
    assert_eq!(imported.graph, sample_snapshot(name).graph);
    assert_eq!(imported.audit_findings, sample_snapshot(name).audit_findings);

    let (entries, _) = read_tar(Path::new(&export_result.bundle_path)).expect("read tar");
    let bundled_evidence: Vec<serde_json::Value> = entries
        .iter()
        .find(|entry| entry.path == "snapshot/evidence.json")
        .map(|entry| serde_json::from_slice(&entry.content).expect("evidence json"))
        .expect("evidence entry");
    assert!(!bundled_evidence[0].as_object().unwrap().contains_key("metadata"));
}

#[test]
fn home_paths_are_stored_as_home_token_and_resolved_on_import() {
    let box_ = make_sandbox();
    let name = "home-abstraction";
    let home_settings = box_.home_dir.join(".claude").join("settings.json");
    fs::create_dir_all(home_settings.parent().unwrap()).expect("mkdir");
    fs::write(&home_settings, "{}").expect("write settings");

    let mut snapshot = sample_snapshot(name);
    snapshot.evidence[0].kind = EvidenceKind::AgentConfig;
    snapshot.evidence[0].source_path = home_settings.to_string_lossy().to_string();
    snapshot.evidence[0].restore_policy = RestorePolicy::FullContentSupported;
    snapshot.evidence[0].value = Some(json!({}));
    snapshot.graph[0].source_path = home_settings.to_string_lossy().to_string();
    snapshot.graph[0].entity_kind = EvidenceKind::AgentConfig;
    snapshot.provenance[0].source_path = home_settings.to_string_lossy().to_string();
    write_snapshot(&box_.store_dir, StoreSnapshot::from(snapshot), None).expect("write");

    let mut options = export_options(&box_, name);
    options.include_content = Some(true);
    let export_result = bundle_export(&options).expect("export");

    let (entries, _) = read_tar(Path::new(&export_result.bundle_path)).expect("read tar");
    let bundled_evidence: Vec<DiscoveredItem> = entries
        .iter()
        .find(|entry| entry.path == "snapshot/evidence.json")
        .map(|entry| serde_json::from_slice(&entry.content).expect("evidence"))
        .expect("evidence entry");
    assert_eq!(bundled_evidence[0].source_path, "{home}/.claude/settings.json");

    let import_box = make_sandbox();
    bundle_import(&import_options(&import_box, Path::new(&export_result.bundle_path)))
        .expect("import");
    let imported = read_snapshot(&import_box.store_dir, name, None).expect("read imported");
    assert_eq!(
        imported.evidence[0].source_path,
        import_box
            .home_dir
            .join(".claude")
            .join("settings.json")
            .to_string_lossy()
    );
}

#[test]
fn rejects_content_bundles_with_not_supported_evidence() {
    let box_ = make_sandbox();
    let name = "unsupported-policy";
    let mut snapshot = sample_snapshot(name);
    snapshot.evidence.push(DiscoveredItem {
        id: "project.symlink.claude-settings".to_string(),
        agent: AgentId::ClaudeCode,
        kind: EvidenceKind::Symlink,
        source_path: ".claude/settings.json".to_string(),
        scope: EvidenceScope::Project,
        precedence: 40,
        parser: EvidenceParser::Filesystem,
        sensitivity: "path_only".to_string(),
        content_policy: "metadata_only".to_string(),
        restore_policy: RestorePolicy::NotSupported,
        capture_status: CaptureStatus::Captured,
        confidence: EvidenceConfidence::High,
        name: Some("settings-symlink".to_string()),
        value: Some(json!({ "target": "/tmp/outside" })),
        checksum: None,
        metadata: None,
    });
    write_snapshot(&box_.store_dir, StoreSnapshot::from(snapshot), None).expect("write");

    let mut options = export_options(&box_, name);
    options.include_content = Some(true);
    let error = bundle_export(&options).unwrap_err().to_string();
    assert!(error.contains("not_supported"));
    assert!(error.contains("would lose restore data"));
}

#[test]
fn signs_bundle_with_hmac_sha256() {
    let box_ = make_sandbox();
    let name = "signed-bundle";
    write_snapshot(&box_.store_dir, StoreSnapshot::from(sample_snapshot(name)), None)
        .expect("write");

    let mut options = export_options(&box_, name);
    options.signature_key = Some("test-secret".to_string());
    let export_result = bundle_export(&options).expect("export");

    let inspect = bundle_inspect(&export_result.bundle_path).expect("inspect");
    assert!(inspect.is_signed);
    assert_eq!(inspect.signature_algorithm.as_deref(), Some("HMAC-SHA256"));

    let (entries, _) = read_tar(Path::new(&export_result.bundle_path)).expect("read tar");
    let manifest: serde_json::Value = entries
        .iter()
        .find(|entry| entry.path == ".hem/manifest.json")
        .map(|entry| serde_json::from_slice(&entry.content).expect("manifest"))
        .expect("manifest");
    let signature = manifest["security"]["signature"].as_str().unwrap();
    assert_eq!(signature.len(), 64);
    assert!(signature.chars().all(|ch| ch.is_ascii_hexdigit()));
}

#[test]
fn trust_on_first_use_for_signed_bundle_keys() {
    let box_ = make_sandbox();
    let trusted_name = "trusted-key";
    let other_name = "other-key";
    write_snapshot(
        &box_.store_dir,
        StoreSnapshot::from(sample_snapshot(trusted_name)),
        None,
    )
    .expect("write trusted");
    let mut other_snapshot = sample_snapshot(other_name);
    other_snapshot.manifest.name = other_name.to_string();
    write_snapshot(&box_.store_dir, StoreSnapshot::from(other_snapshot), None).expect("write other");

    let mut trusted_options = export_options(&box_, trusted_name);
    trusted_options.signature_key = Some("trusted-secret".to_string());
    let trusted_bundle = bundle_export(&trusted_options).expect("export trusted");

    let mut other_options = export_options(&box_, other_name);
    other_options.signature_key = Some("other-secret".to_string());
    let other_bundle = bundle_export(&other_options).expect("export other");

    let mut first_import = import_options(&box_, Path::new(&trusted_bundle.bundle_path));
    first_import.signature_key = Some("trusted-secret".to_string());
    first_import.trust = Some(true);
    first_import.dry_run = Some(true);
    let first_result = bundle_import(&first_import).expect("first import");
    assert!(first_result
        .warnings
        .iter()
        .any(|warning| warning.contains("Trusted bundle signing key")));

    let mut second_import = import_options(&box_, Path::new(&other_bundle.bundle_path));
    second_import.signature_key = Some("other-secret".to_string());
    second_import.dry_run = Some(true);
    let error = bundle_import(&second_import).unwrap_err().to_string();
    assert!(error.contains("does not match trusted key fingerprint"));
}

#[test]
fn quarantines_content_instead_of_applying_target_files() {
    let box_ = make_sandbox();
    let name = "quarantine-content";
    let project_file = box_.project_path.join("config").join("tool.json");
    fs::create_dir_all(project_file.parent().unwrap()).expect("mkdir");
    fs::write(&project_file, "safe content").expect("write project file");

    let mut snapshot = sample_snapshot(name);
    snapshot.evidence[0] = DiscoveredItem {
        id: "project.config.tool".to_string(),
        kind: EvidenceKind::AgentConfig,
        source_path: "config/tool.json".to_string(),
        restore_policy: RestorePolicy::FullContentSupported,
        ..snapshot.evidence[0].clone()
    };
    write_snapshot(&box_.store_dir, StoreSnapshot::from(snapshot), None).expect("write");

    let mut export_opts = export_options(&box_, name);
    export_opts.include_content = Some(true);
    let exported = bundle_export(&export_opts).expect("export");
    fs::write(&project_file, "local content").expect("overwrite local");

    let mut import_opts = import_options(&box_, Path::new(&exported.bundle_path));
    import_opts.apply_content = Some(true);
    import_opts.quarantine = Some(true);
    import_opts.target_platform = Some("darwin".to_string());
    let imported = bundle_import(&import_opts).expect("import");

    assert!(!imported.content_applied);
    assert!(imported.quarantined_content_dir.is_some());
    assert_eq!(fs::read_to_string(&project_file).unwrap(), "local content");
    let quarantine_file = Path::new(imported.quarantined_content_dir.as_ref().unwrap())
        .join("config")
        .join("tool.json");
    assert_eq!(fs::read_to_string(quarantine_file).unwrap(), "safe content");
    assert!(imported
        .warnings
        .iter()
        .any(|warning| warning.contains("quarantined")));
}

#[test]
fn inspect_returns_metadata() {
    let box_ = make_sandbox();
    let name = "inspect-test";
    write_snapshot(&box_.store_dir, StoreSnapshot::from(sample_snapshot(name)), None)
        .expect("write");
    let export_result = bundle_export(&export_options(&box_, name)).expect("export");
    let inspect = bundle_inspect(&export_result.bundle_path).expect("inspect");
    assert_eq!(inspect.format_version, 1);
    assert_eq!(inspect.snapshot_name, name);
    assert!(!inspect.includes_content);
    assert!(!inspect.is_signed);
    assert_eq!(inspect.content_file_count, 0);
    assert_eq!(inspect.content_total_bytes, 0);
    assert!(!inspect.bundle_checksum.is_empty());
}

#[test]
fn blocks_home_relative_content_paths() {
    let box_ = make_sandbox();
    let entries = make_minimal_bundle(
        &box_,
        "malicious",
        vec![file_entry(
            "content/~/.ssh/authorized_keys",
            b"evil",
        )],
    );
    let bundle_file = bundle_path(&box_, "evil-home");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.apply_content = Some(true);
    let error = bundle_import(&options).unwrap_err().to_string();
    assert!(error.contains("Home-relative content path"));
}

#[test]
fn blocks_ssh_content_paths() {
    let box_ = make_sandbox();
    let entries = make_minimal_bundle(
        &box_,
        "evil-ssh",
        vec![file_entry("content/.ssh/config", b"evil")],
    );
    let bundle_file = bundle_path(&box_, "evil-ssh");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.apply_content = Some(true);
    let error = bundle_import(&options).unwrap_err().to_string();
    assert!(error.contains("Blocked content path prefix"));
}

#[test]
fn blocks_dotdot_content_paths() {
    let box_ = make_sandbox();
    let entries = make_minimal_bundle(
        &box_,
        "dotdot",
        vec![file_entry("content/../../etc/passwd", b"root:x:0:0:")],
    );
    let bundle_file = bundle_path(&box_, "evil-dotdot");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.apply_content = Some(true);
    let error = bundle_import(&options).unwrap_err().to_string();
    assert!(error.contains(".."));
}

#[test]
fn blocks_absolute_tar_paths() {
    let box_ = make_sandbox();
    let manifest = json!({
        "formatVersion": 1,
        "snapshotName": "abs",
        "createdAt": "2026-05-12T00:00:00.000Z",
        "projectPath": box_.project_path.to_string_lossy(),
        "includesContent": false,
        "contentFileCount": 0,
        "contentTotalBytes": 0,
        "security": {
            "rawSecretsIncluded": false,
            "redactionPolicy": "metadata-only",
            "signed": false
        }
    });
    let entries = vec![
        dir_entry(".hem/"),
        file_entry(".hem/format-version", b"1\n"),
        file_entry(
            ".hem/manifest.json",
            format!("{}\n", serde_json::to_string(&manifest).unwrap()).as_bytes(),
        ),
        file_entry("/etc/passwd", b"root:x:0:0:"),
    ];
    let bundle_file = bundle_path(&box_, "evil-abs");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.apply_content = Some(true);
    let error = bundle_import(&options).unwrap_err().to_string();
    assert!(error.contains("Path traversal") || error.contains("absolute"));
}

#[test]
fn rejects_unsupported_format_version() {
    let box_ = make_sandbox();
    let manifest = json!({
        "formatVersion": 999,
        "snapshotName": "bad",
        "createdAt": "2026-05-12T00:00:00.000Z",
        "projectPath": box_.project_path.to_string_lossy(),
        "includesContent": false,
        "contentFileCount": 0,
        "contentTotalBytes": 0,
        "security": {
            "rawSecretsIncluded": false,
            "redactionPolicy": "metadata-only",
            "signed": false
        }
    });
    let entries = vec![
        dir_entry(".hem/"),
        file_entry(".hem/format-version", b"999\n"),
        file_entry(
            ".hem/manifest.json",
            format!("{}\n", serde_json::to_string(&manifest).unwrap()).as_bytes(),
        ),
    ];
    let bundle_file = bundle_path(&box_, "bad-format");
    write_tar(&entries, &bundle_file).expect("write tar");

    let error = bundle_import(&import_options(&box_, &bundle_file))
        .unwrap_err()
        .to_string();
    assert!(error.contains("Unsupported bundle format version"));
}

#[test]
fn dry_run_returns_metadata_without_writing_store() {
    let box_ = make_sandbox();
    let name = "dry-run-test";
    write_snapshot(&box_.store_dir, StoreSnapshot::from(sample_snapshot(name)), None)
        .expect("write");
    let export_result = bundle_export(&export_options(&box_, name)).expect("export");
    let before = list_snapshots(&box_.store_dir, None).expect("list before");

    let mut options = import_options(&box_, Path::new(&export_result.bundle_path));
    options.dry_run = Some(true);
    let result = bundle_import(&options).expect("dry-run import");
    let after = list_snapshots(&box_.store_dir, None).expect("list after");

    assert_eq!(result.snapshot_name, name);
    assert!(!result.content_applied);
    assert_eq!(before, after);
}

#[test]
fn dry_run_reports_cross_os_machine_diff() {
    let box_ = make_sandbox();
    let source_platform = if cfg!(target_os = "macos") {
        "linux"
    } else {
        "darwin"
    };
    let mut entries = make_minimal_bundle(
        &box_,
        "cross-os",
        vec![file_entry(
            "content/{home}/.claude/settings.json",
            b"{}",
        )],
    );
    let manifest_index = entries
        .iter()
        .position(|entry| entry.path == ".hem/manifest.json")
        .unwrap();
    let mut manifest: serde_json::Value =
        serde_json::from_slice(&entries[manifest_index].content).unwrap();
    manifest["sourceMachine"] = json!({
        "hostname": "source-host",
        "platform": source_platform,
        "homeDir": "/Users/source"
    });
    entries[manifest_index].content =
        format!("{}\n", serde_json::to_string(&manifest).unwrap()).into_bytes();

    let bundle_file = bundle_path(&box_, "cross-os");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.dry_run = Some(true);
    let result = bundle_import(&options).expect("import");
    let machine_diff = result.machine_diff.expect("machine diff");
    assert!(machine_diff.cross_os);
    assert!(!machine_diff.target_hostname.is_empty());
    assert!(machine_diff
        .os_differences
        .iter()
        .any(|difference| difference.contains("cross-OS restore")));
    let expected_prefix = if source_platform == "linux" {
        "/home/source"
    } else {
        "/Users/source"
    };
    assert!(machine_diff.remapped_paths.iter().any(|mapping| {
        mapping.contains(&format!("{expected_prefix}/.claude/settings.json"))
    }));
}

#[test]
fn dry_run_classifies_mcp_binaries_and_env_keys() {
    let box_ = make_sandbox();
    let mut entries = make_minimal_bundle(&box_, "mcp-binaries", vec![]);
    let manifest_index = entries
        .iter()
        .position(|entry| entry.path == ".hem/manifest.json")
        .unwrap();
    let mut manifest: serde_json::Value =
        serde_json::from_slice(&entries[manifest_index].content).unwrap();
    manifest["sourceMachine"] = json!({
        "hostname": "source-host",
        "platform": "darwin",
        "homeDir": "/Users/alice"
    });
    entries[manifest_index].content =
        format!("{}\n", serde_json::to_string(&manifest).unwrap()).into_bytes();
    entries.push(file_entry(
        "snapshot/evidence.json",
        format!(
            "{}\n",
            serde_json::to_string(&vec![
                json!({
                    "id": "mcp-npx",
                    "agent": "claude-code",
                    "kind": "mcp_server",
                    "sourcePath": ".mcp.json",
                    "scope": "project",
                    "precedence": 40,
                    "parser": "json",
                    "sensitivity": "command_config",
                    "contentPolicy": "structured_safe_fields_only",
                    "restorePolicy": "not_supported",
                    "captureStatus": "captured",
                    "confidence": "high",
                    "name": "github",
                    "value": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"] }
                }),
                json!({
                    "id": "mcp-local",
                    "agent": "claude-code",
                    "kind": "mcp_server",
                    "sourcePath": ".mcp.json",
                    "scope": "project",
                    "precedence": 40,
                    "parser": "json",
                    "sensitivity": "command_config",
                    "contentPolicy": "structured_safe_fields_only",
                    "restorePolicy": "not_supported",
                    "captureStatus": "captured",
                    "confidence": "high",
                    "name": "github",
                    "value": { "command": "/Users/alice/.local/bin/private-mcp" }
                }),
                json!({
                    "id": "mcp-remote",
                    "agent": "claude-code",
                    "kind": "mcp_server",
                    "sourcePath": ".mcp.json",
                    "scope": "project",
                    "precedence": 40,
                    "parser": "json",
                    "sensitivity": "command_config",
                    "contentPolicy": "structured_safe_fields_only",
                    "restorePolicy": "not_supported",
                    "captureStatus": "captured",
                    "confidence": "high",
                    "name": "github",
                    "value": { "url": "https://mcp.example.test" }
                }),
                json!({
                    "id": "env-openai",
                    "agent": "project",
                    "kind": "env_key",
                    "sourcePath": ".env",
                    "scope": "project",
                    "precedence": 40,
                    "parser": "dotenv",
                    "sensitivity": "env_key_inventory",
                    "contentPolicy": "key_inventory_only",
                    "restorePolicy": "key_inventory_only",
                    "captureStatus": "redacted",
                    "confidence": "high",
                    "name": "OPENAI_API_KEY",
                    "value": { "key": "OPENAI_API_KEY" }
                })
            ])
            .unwrap()
        )
        .as_bytes(),
    ));

    let bundle_file = bundle_path(&box_, "mcp-binaries");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.dry_run = Some(true);
    let result = bundle_import(&options).expect("import");
    let machine_diff = result.machine_diff.expect("machine diff");

    let npx = machine_diff
        .mcp_binary_report
        .iter()
        .find(|report| report.evidence_id == "mcp-npx")
        .expect("npx report");
    assert_eq!(npx.binary_kind, Some(hem_core::McpBinaryKind::PackageRunner));
    assert!(npx.warning.as_deref().unwrap_or("").contains("Package runner npx"));

    let local = machine_diff
        .mcp_binary_report
        .iter()
        .find(|report| report.evidence_id == "mcp-local")
        .expect("local report");
    assert_eq!(
        local.binary_kind,
        Some(hem_core::McpBinaryKind::SourceLocalPath)
    );
    assert!(!local.available_on_target);
    assert!(local
        .warning
        .as_deref()
        .unwrap_or("")
        .contains("source machine local binary path"));

    assert!(result.readiness.summary[&hem_core::ReadinessCategory::NeedsManualAction] >= 2);
    assert_eq!(result.readiness.summary[&hem_core::ReadinessCategory::Unverified], 1);
    assert!(result.readiness.items.iter().any(|item| {
        item.code == "HEM_ENV_VALUE_REQUIRED" && item.problem.contains("OPENAI_API_KEY")
    }));
    assert!(!serde_json::to_string(&result.readiness)
        .unwrap()
        .contains("sk-real-secret"));
}

#[test]
fn mcp_command_check_does_not_invoke_shell() {
    let box_ = make_sandbox();
    let marker = box_.root.join("shell-injection-marker");
    let malicious_command = format!("missing\" ; touch \"{}\" ; \"", marker.display());
    let mut entries = make_minimal_bundle(&box_, "mcp-shell-safe", vec![]);
    entries.push(file_entry(
        "snapshot/evidence.json",
        format!(
            "{}\n",
            serde_json::to_string(&vec![json!({
                "id": "mcp-malicious",
                "agent": "claude-code",
                "kind": "mcp_server",
                "sourcePath": ".mcp.json",
                "scope": "project",
                "precedence": 40,
                "parser": "json",
                "sensitivity": "command_config",
                "contentPolicy": "structured_safe_fields_only",
                "restorePolicy": "not_supported",
                "captureStatus": "captured",
                "confidence": "high",
                "name": "github",
                "value": { "command": malicious_command }
            })])
            .unwrap()
        )
        .as_bytes(),
    ));
    let bundle_file = bundle_path(&box_, "mcp-shell-safe");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.dry_run = Some(true);
    let result = bundle_import(&options).expect("import");
    let report = result
        .machine_diff
        .expect("machine diff")
        .mcp_binary_report
        .into_iter()
        .find(|item| item.evidence_id == "mcp-malicious")
        .expect("report");
    assert!(!report.available_on_target);
    assert!(!marker.exists());
}

#[test]
fn blocks_non_macos_content_apply_before_writing_files() {
    let box_ = make_sandbox();
    let entries = make_minimal_bundle(
        &box_,
        "mac-apply-only",
        vec![file_entry("content/config/tool.json", b"unsafe write")],
    );
    let bundle_file = bundle_path(&box_, "mac-apply-only");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.apply_content = Some(true);
    options.target_platform = Some("linux".to_string());
    let error = bundle_import(&options).unwrap_err().to_string();
    assert!(error.contains("HEM_MACOS_APPLY_ONLY"));
    assert!(!box_.project_path.join("config/tool.json").exists());
}

#[test]
fn dry_run_reports_non_macos_apply_blockers_without_throwing() {
    let box_ = make_sandbox();
    let entries = make_minimal_bundle(&box_, "mac-dry-run-blocker", vec![]);
    let bundle_file = bundle_path(&box_, "mac-dry-run-blocker");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.apply_content = Some(true);
    options.dry_run = Some(true);
    options.target_platform = Some("linux".to_string());
    let result = bundle_import(&options).expect("import");
    assert_eq!(result.readiness.summary[&hem_core::ReadinessCategory::Blocked], 1);
    assert_eq!(result.readiness.items[0].code, "HEM_MACOS_APPLY_ONLY");
    assert!(read_snapshot(&box_.store_dir, "mac-dry-run-blocker", None).is_err());
}

#[test]
fn dry_run_reports_non_macos_apply_limitations() {
    let box_ = make_sandbox();
    let entries = make_minimal_bundle(&box_, "mac-dry-run-limitation", vec![]);
    let bundle_file = bundle_path(&box_, "mac-dry-run-limitation");
    write_tar(&entries, &bundle_file).expect("write tar");

    let mut options = import_options(&box_, &bundle_file);
    options.dry_run = Some(true);
    options.target_platform = Some("linux".to_string());
    let result = bundle_import(&options).expect("import");
    assert_eq!(
        result.readiness.summary[&hem_core::ReadinessCategory::Unsupported],
        1
    );
    assert_eq!(result.readiness.items[0].code, "HEM_MACOS_APPLY_ONLY");
}

#[test]
fn bundle_verify_accepts_valid_exported_bundle() {
    let box_ = make_sandbox();
    let name = "verify-test";
    write_snapshot(&box_.store_dir, StoreSnapshot::from(sample_snapshot(name)), None)
        .expect("write");
    let export_result = bundle_export(&export_options(&box_, name)).expect("export");
    let verify = bundle_verify(&BundleVerifyOptions {
        bundle_path: export_result.bundle_path,
        signature_key: None,
    })
    .expect("verify");
    assert!(verify.valid);
    assert!(verify.checksums.checked);
    assert!(verify.checksums.ok);
}