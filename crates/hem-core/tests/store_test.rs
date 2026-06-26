use std::fs;
use std::path::PathBuf;

use hem_core::{
    agent_store_dir, append_timeline_entry, capture_current_state, ensure_store, find_timeline_entry,
    list_agents, list_snapshots, list_timeline_entries, read_snapshot, read_snapshot_content,
    snapshot_exists, write_snapshot, AgentId, AuditFinding, CaptureStatus, DiscoveredItem,
    EvidenceConfidence, EvidenceKind, EvidenceParser, EvidenceScope, GraphNode, ProvenanceEntry,
    RestorePolicy, RuntimeOptions, Severity, Snapshot, SnapshotContentEntry, SnapshotManifest,
    SnapshotSecurity, StoreSnapshot, TimelineChangeSummary, TimelineChangedSurface,
    TimelineConfidence, TimelineEntry, TimelineEntryEventKind, TimelineEntrySource,
    TimelineListOptions, TimelineRestoreReadiness,
};
use tempfile::TempDir;

fn temp_store() -> TempDir {
    TempDir::new().expect("temp dir")
}

fn snapshot(name: &str) -> Snapshot {
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
            id: "claude.project.settings".to_string(),
            agent: AgentId::ClaudeCode,
            kind: EvidenceKind::AgentConfig,
            source_path: ".claude/settings.json".to_string(),
            scope: EvidenceScope::Project,
            precedence: 40,
            parser: EvidenceParser::Json,
            sensitivity: "command_config".to_string(),
            content_policy: "structured_safe_fields_only".to_string(),
            restore_policy: RestorePolicy::NotSupported,
            capture_status: CaptureStatus::Captured,
            confidence: EvidenceConfidence::High,
            name: None,
            value: None,
            checksum: Some("sha256:observed-config".to_string()),
            metadata: None,
        }],
        graph: vec![GraphNode {
            id: "node.claude.project.settings".to_string(),
            agent: AgentId::ClaudeCode,
            scope: EvidenceScope::Project,
            source_path: ".claude/settings.json".to_string(),
            entity_kind: EvidenceKind::AgentConfig,
            entity_name: "settings".to_string(),
            effective_value: serde_json::json!({ "permissions": ["Bash(bun test)"] }),
            overridden_by: None,
            confidence: EvidenceConfidence::High,
            evidence_id: "claude.project.settings".to_string(),
        }],
        audit_findings: vec![AuditFinding {
            code: "EXECUTABLE_CONFIG_ADDED".to_string(),
            severity: Severity::Medium,
            problem: "Project config allows an executable command.".to_string(),
            cause: ".claude/settings.json allows Bash(bun test).".to_string(),
            fix: "Review the allowed command before sharing the project config.".to_string(),
            path: Some(".claude/settings.json".to_string()),
            evidence_id: Some("claude.project.settings".to_string()),
        }],
        provenance: vec![ProvenanceEntry {
            node_id: "node.claude.project.settings".to_string(),
            evidence_id: "claude.project.settings".to_string(),
            source_path: ".claude/settings.json".to_string(),
            scope: EvidenceScope::Project,
            precedence: 40,
            confidence: EvidenceConfidence::High,
            capture_status: CaptureStatus::Captured,
        }],
        content: None,
    }
}

fn timeline_entry(
    id: &str,
    observed_at: &str,
    after_snapshot_name: &str,
) -> TimelineEntry {
    TimelineEntry {
        schema_version: "0.1".to_string(),
        id: id.to_string(),
        source: TimelineEntrySource::Manual,
        event_kind: TimelineEntryEventKind::SetupChanged,
        title: "update github mcp".to_string(),
        project_path: "/tmp/project".to_string(),
        agent: None,
        agents: vec![AgentId::ClaudeCode],
        before_snapshot_name: None,
        after_snapshot_name: after_snapshot_name.to_string(),
        capture_id: "capture-test".to_string(),
        created_at: observed_at.to_string(),
        observed_at: observed_at.to_string(),
        changed_surfaces: vec![TimelineChangedSurface {
            kind: "mcp_server".to_string(),
            change_type: "MCP_CHANGED".to_string(),
            path: ".mcp.json".to_string(),
            entity_name: Some("github".to_string()),
            restorable: true,
            observe_only: false,
            before: None,
            after: None,
        }],
        restore_readiness: TimelineRestoreReadiness::Full,
        confidence: TimelineConfidence::High,
        confidence_reason: "test".to_string(),
        evidence_count: 1,
        graph_node_count: 1,
        audit_finding_count: 0,
        changes: TimelineChangeSummary {
            previous_entry_id: None,
            previous_snapshot_name: None,
            has_changes: true,
            semantic_change_count: 1,
            raw_source_change_count: 0,
            highlights: vec!["MCP_CHANGED: github".to_string()],
        },
    }
}

#[cfg(unix)]
use std::os::unix::fs::PermissionsExt;

#[test]
fn creates_store_directory_with_0700_permissions() {
    let root = temp_store();
    let store_dir = root.path().join("store");
    let findings = ensure_store(&store_dir).expect("ensure store");
    #[cfg(unix)]
    {
        let mode = fs::metadata(&store_dir).expect("stat").permissions().mode() & 0o777;
        assert_eq!(mode, 0o700);
    }
    assert!(findings.is_empty());
}

#[cfg(unix)]
#[test]
fn returns_audit_finding_when_store_is_world_writable() {
    let root = temp_store();
    let store_dir = root.path().join("store");
    ensure_store(&store_dir).expect("ensure store");
    let mut perms = fs::metadata(&store_dir).expect("stat").permissions();
    perms.set_mode(0o777);
    fs::set_permissions(&store_dir, perms).expect("chmod");

    let findings = ensure_store(&store_dir).expect("ensure store");
    assert_eq!(findings.len(), 1);
    assert_eq!(findings[0].code, "WORLD_WRITABLE_STORE");
    assert_eq!(findings[0].severity, Severity::High);
    assert_eq!(findings[0].path.as_deref(), Some(store_dir.to_str().unwrap()));
}

#[test]
fn rejects_unsafe_snapshot_names() {
    let root = temp_store();
    let store_dir = root.path();
    for name in ["", "../baseline", "base/line", "base\\line", "safe/../unsafe", ".."] {
        let err = write_snapshot(store_dir, StoreSnapshot::from(snapshot(name)), None).unwrap_err();
        assert!(err.to_string().to_lowercase().contains("unsafe snapshot name"));
        let err = read_snapshot(store_dir, name, None).unwrap_err();
        assert!(err.to_string().to_lowercase().contains("unsafe snapshot name"));
    }
}

#[test]
fn lists_snapshots_and_round_trips() {
    let root = temp_store();
    let store_dir = root.path();
    write_snapshot(store_dir, StoreSnapshot::from(snapshot("current")), None).expect("write");
    write_snapshot(store_dir, StoreSnapshot::from(snapshot("baseline")), None).expect("write");

    assert_eq!(
        list_snapshots(store_dir, None).expect("list"),
        vec!["baseline".to_string(), "current".to_string()]
    );
    assert!(snapshot_exists(store_dir, "baseline", None).expect("exists"));
    assert!(!snapshot_exists(store_dir, "missing", None).expect("exists"));
    assert_eq!(
        read_snapshot(store_dir, "baseline", None).expect("read"),
        snapshot("baseline")
    );
}

#[test]
fn writes_metadata_only_snapshot_file_set() {
    let root = temp_store();
    let store_dir = root.path();
    write_snapshot(store_dir, StoreSnapshot::from(snapshot("baseline")), None).expect("write");

    let snapshot_dir = store_dir.join("baseline");
    let mut files: Vec<String> = fs::read_dir(&snapshot_dir)
        .expect("readdir")
        .filter_map(Result::ok)
        .map(|entry| entry.file_name().to_string_lossy().to_string())
        .collect();
    files.sort();
    assert_eq!(
        files,
        vec![
            "audit-findings.json",
            "checksums.json",
            "evidence.json",
            "graph.json",
            "manifest.json",
            "provenance.json",
            "redactions.json"
        ]
    );

    let checksums: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(snapshot_dir.join("checksums.json")).unwrap())
            .unwrap();
    let redactions: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(snapshot_dir.join("redactions.json")).unwrap())
            .unwrap();
    assert_eq!(
        checksums,
        serde_json::json!({
            "claude.project.settings": {
                "sourcePath": ".claude/settings.json",
                "checksum": "sha256:observed-config"
            }
        })
    );
    assert_eq!(redactions, serde_json::json!([]));
}

#[test]
fn writes_and_reads_content_backed_snapshot_entries() {
    let root = temp_store();
    let store_dir = root.path();
    let mut content_snapshot = StoreSnapshot::from(snapshot("codex-baseline"));
    content_snapshot.manifest.security = SnapshotSecurity {
        raw_secrets_included: false,
        redaction_policy: "content-backed".to_string(),
    };
    content_snapshot.content = Some(vec![
        SnapshotContentEntry {
            evidence_id: "claude.project.settings".to_string(),
            source_path: "~/.codex/config.toml".to_string(),
            restore_path: "~/.codex/config.toml".to_string(),
            checksum: "sha256:codex-config".to_string(),
            byte_length: 14,
            encoding: "utf8".to_string(),
            storage_path: "content/claude.project.settings.txt".to_string(),
            capture_status: "captured".to_string(),
            reason: None,
            content: Some("model = \"gpt-5\"".to_string()),
        },
        SnapshotContentEntry {
            evidence_id: "secret".to_string(),
            source_path: "~/.codex/config.toml".to_string(),
            restore_path: "~/.codex/config.toml".to_string(),
            checksum: "sha256:secret".to_string(),
            byte_length: 18,
            encoding: "utf8".to_string(),
            storage_path: "content/secret.txt".to_string(),
            capture_status: "omitted".to_string(),
            reason: Some("secret_like_assignment".to_string()),
            content: None,
        },
    ]);
    write_snapshot(store_dir, content_snapshot, Some(AgentId::Codex)).expect("write");

    let read = read_snapshot(store_dir, "codex-baseline", Some(AgentId::Codex)).expect("read");
    let content_index = read.content.clone().expect("content");
    assert_eq!(content_index.len(), 2);
    assert!(content_index[0].content.is_none());
    assert_eq!(content_index[1].capture_status, "omitted");
    assert_eq!(
        content_index[1].reason.as_deref(),
        Some("secret_like_assignment")
    );
    assert_eq!(
        read_snapshot_content(
            store_dir,
            "codex-baseline",
            &content_index[0],
            Some(AgentId::Codex)
        )
        .expect("content"),
        "model = \"gpt-5\""
    );

    let mut bad_entry = content_index[0].clone();
    bad_entry.storage_path = "../escape".to_string();
    let err = read_snapshot_content(
        store_dir,
        "codex-baseline",
        &bad_entry,
        Some(AgentId::Codex),
    )
    .unwrap_err();
    assert!(err.to_string().to_lowercase().contains("unsafe snapshot content path"));
}

#[test]
fn omits_secret_like_codex_content_during_capture() {
    let root = temp_store();
    let project_path = root.path().join("project");
    let home_dir = root.path().join("home");
    let store_dir = root.path().join("store");
    fs::create_dir_all(&project_path).expect("mkdir");
    fs::create_dir_all(home_dir.join(".codex")).expect("mkdir");
    fs::write(
        home_dir.join(".codex/config.toml"),
        "[mcp_servers.docs]\ncommand = \"docs\"\n[mcp_servers.docs.env]\nOPENAI_API_KEY = \"sk-secret\"\n",
    )
    .expect("write config");

    let state = capture_current_state(
        &RuntimeOptions {
            project_path: project_path.to_string_lossy().to_string(),
            home_dir: home_dir.to_string_lossy().to_string(),
            store_dir: store_dir.to_string_lossy().to_string(),
            agent: Some(AgentId::Codex),
            scope: Some(EvidenceScope::User),
            capture_content: Some(true),
        },
        "secret-baseline",
    )
    .expect("capture");

    let config_content = state
        .snapshot
        .content
        .as_ref()
        .and_then(|entries| {
            entries
                .iter()
                .find(|entry| entry.source_path == "~/.codex/config.toml")
        });
    assert_eq!(config_content.map(|e| e.capture_status.as_str()), Some("omitted"));
    assert_eq!(
        config_content.and_then(|e| e.reason.as_deref()),
        Some("secret_like_assignment")
    );
    assert!(config_content.and_then(|e| e.content.as_ref()).is_none());
    let serialized = serde_json::to_string(&state.snapshot).expect("serialize");
    assert!(!serialized.contains("sk-secret"));
}

#[test]
fn agent_store_dir_returns_scoped_paths() {
    assert_eq!(
        agent_store_dir(PathBuf::from("/store").as_path(), Some(AgentId::ClaudeCode)),
        PathBuf::from("/store/claude-code")
    );
    assert_eq!(
        agent_store_dir(PathBuf::from("/store").as_path(), Some(AgentId::Codex)),
        PathBuf::from("/store/codex")
    );
    assert_eq!(
        agent_store_dir(PathBuf::from("/store").as_path(), None),
        PathBuf::from("/store")
    );
}

#[test]
fn writes_and_reads_snapshots_per_agent() {
    let root = temp_store();
    let store_dir = root.path();
    write_snapshot(
        store_dir,
        StoreSnapshot::from(snapshot("baseline")),
        Some(AgentId::ClaudeCode),
    )
    .expect("write cc");
    write_snapshot(
        store_dir,
        StoreSnapshot::from(snapshot("codex-baseline")),
        Some(AgentId::Codex),
    )
    .expect("write codex");

    assert!(store_dir.join("claude-code/baseline").is_dir());
    assert!(store_dir.join("codex/codex-baseline").is_dir());
    assert!(list_snapshots(store_dir, None).expect("list").is_empty());
    assert_eq!(
        list_snapshots(store_dir, Some(AgentId::ClaudeCode)).expect("list"),
        vec!["baseline".to_string()]
    );
    assert_eq!(
        list_snapshots(store_dir, Some(AgentId::Codex)).expect("list"),
        vec!["codex-baseline".to_string()]
    );
    assert_eq!(
        read_snapshot(store_dir, "baseline", Some(AgentId::ClaudeCode)).expect("read"),
        snapshot("baseline")
    );
    assert_eq!(
        read_snapshot(store_dir, "codex-baseline", Some(AgentId::Codex)).expect("read"),
        snapshot("codex-baseline")
    );
    assert!(snapshot_exists(store_dir, "baseline", Some(AgentId::ClaudeCode)).expect("exists"));
    assert!(!snapshot_exists(store_dir, "baseline", Some(AgentId::Codex)).expect("exists"));
}

#[test]
fn list_agents_returns_agents_with_snapshots() {
    let root = temp_store();
    let store_dir = root.path();
    write_snapshot(
        store_dir,
        StoreSnapshot::from(snapshot("v1")),
        Some(AgentId::ClaudeCode),
    )
    .expect("write");
    write_snapshot(
        store_dir,
        StoreSnapshot::from(snapshot("v1")),
        Some(AgentId::Codex),
    )
    .expect("write");
    write_snapshot(
        store_dir,
        StoreSnapshot::from(snapshot("v1")),
        Some(AgentId::Cursor),
    )
    .expect("write");

    assert_eq!(
        list_agents(store_dir).expect("agents"),
        vec![AgentId::ClaudeCode, AgentId::Codex, AgentId::Cursor]
    );
}

#[test]
fn list_agents_returns_empty_for_empty_store() {
    let root = temp_store();
    let store_dir = root.path();
    ensure_store(store_dir).expect("ensure");
    assert!(list_agents(store_dir).expect("agents").is_empty());
}

#[test]
fn persists_timeline_events_sorted_by_observed_time() {
    let root = temp_store();
    let store_dir = root.path();
    append_timeline_entry(
        store_dir,
        &timeline_entry("older", "2026-06-07T00:00:00.000Z", "after-older"),
    )
    .expect("append");
    append_timeline_entry(
        store_dir,
        &timeline_entry("newer", "2026-06-07T00:01:00.000Z", "after-newer"),
    )
    .expect("append");

    let ids: Vec<_> = list_timeline_entries(store_dir, TimelineListOptions {
        agent: None,
        project_path: None,
        limit: None,
        on_corrupt_entry: None,
    })
    .expect("list")
    .into_iter()
    .map(|entry| entry.id)
    .collect();
    assert_eq!(ids, vec!["newer".to_string(), "older".to_string()]);
    assert_eq!(
        find_timeline_entry(store_dir, "after-older", TimelineListOptions {
            agent: None,
            project_path: None,
            limit: None,
            on_corrupt_entry: None,
        })
        .expect("find")
        .expect("entry")
        .id,
        "older"
    );
}

#[test]
fn skips_corrupt_timeline_events() {
    let root = temp_store();
    let store_dir = root.path();
    append_timeline_entry(
        store_dir,
        &timeline_entry("valid", "2026-06-07T00:00:00.000Z", "after-valid"),
    )
    .expect("append");
    fs::create_dir_all(store_dir.join("timeline/events")).expect("mkdir");
    fs::write(
        store_dir.join("timeline/events/bad.json"),
        "{bad json",
    )
    .expect("write bad");

    let mut corrupt_events = Vec::new();
    let entries = list_timeline_entries(store_dir, TimelineListOptions {
        agent: None,
        project_path: None,
        limit: None,
        on_corrupt_entry: Some(&mut |event| corrupt_events.push(event)),
    })
    .expect("list");
    assert_eq!(entries.len(), 1);
    assert_eq!(entries[0].id, "valid");
    assert_eq!(corrupt_events.len(), 1);
    assert!(corrupt_events[0].file_path.ends_with("bad.json"));
    assert!(corrupt_events[0].error.to_lowercase().contains("json"));
}

#[test]
fn normalizes_legacy_daemon_timeline_events() {
    let root = temp_store();
    let store_dir = root.path();
    ensure_store(store_dir).expect("ensure");
    fs::create_dir_all(store_dir.join("timeline/events")).expect("mkdir");
    fs::write(
        store_dir.join("timeline/events/legacy.json"),
        serde_json::json!({
            "schemaVersion": "0.1",
            "id": "legacy-event",
            "source": "daemon",
            "eventKind": "baseline",
            "title": "legacy daemon baseline",
            "projectPath": "/tmp/project",
            "agents": ["claude-code"],
            "afterSnapshotName": "legacy-baseline",
            "daemonRunId": "run-legacy",
            "createdAt": "2026-06-07T00:00:00.000Z",
            "observedAt": "2026-06-07T00:00:00.000Z",
            "changedSurfaces": [],
            "restoreReadiness": "observe-only",
            "confidence": "high",
            "confidenceReason": "legacy",
            "evidenceCount": 0,
            "graphNodeCount": 0,
            "auditFindingCount": 0,
            "changes": {
                "hasChanges": false,
                "semanticChangeCount": 0,
                "rawSourceChangeCount": 0,
                "highlights": []
            },
            "captureId": ""
        })
        .to_string(),
    )
    .expect("write legacy");

    let entry = list_timeline_entries(store_dir, TimelineListOptions {
        agent: None,
        project_path: None,
        limit: None,
        on_corrupt_entry: None,
    })
    .expect("list")
    .into_iter()
    .next()
    .expect("entry");
    assert_eq!(entry.source, TimelineEntrySource::Manual);
    assert_eq!(entry.capture_id, "run-legacy");
    assert_eq!(entry.after_snapshot_name, "legacy-baseline");
}