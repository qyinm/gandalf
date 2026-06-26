use hem_core::{
    audit_evidence, build_graph, build_provenance, diff_graphs, AgentId, CaptureStatus,
    DiscoveredItem, EvidenceConfidence, EvidenceKind, EvidenceParser, EvidenceScope,
    RestorePolicy, SemanticChangeCode,
};

fn item(
    id: &str,
    kind: EvidenceKind,
    source_path: &str,
    scope: EvidenceScope,
    precedence: u32,
    overrides: ItemOverrides,
) -> DiscoveredItem {
    DiscoveredItem {
        id: id.to_string(),
        agent: overrides.agent.unwrap_or(AgentId::ClaudeCode),
        kind,
        source_path: source_path.to_string(),
        scope,
        precedence,
        parser: EvidenceParser::Json,
        sensitivity: "command_config".to_string(),
        content_policy: "structured_safe_fields_only".to_string(),
        restore_policy: RestorePolicy::NotSupported,
        capture_status: overrides.capture_status.unwrap_or(CaptureStatus::Captured),
        confidence: EvidenceConfidence::High,
        name: overrides.name,
        value: overrides.value,
        checksum: overrides.checksum,
        metadata: overrides.metadata,
    }
}

struct ItemOverrides {
    agent: Option<AgentId>,
    name: Option<String>,
    value: Option<serde_json::Value>,
    checksum: Option<String>,
    capture_status: Option<CaptureStatus>,
    metadata: Option<serde_json::Value>,
}

impl ItemOverrides {
    fn with_name(mut self, name: &str) -> Self {
        self.name = Some(name.to_string());
        self
    }

    fn with_value(mut self, value: serde_json::Value) -> Self {
        self.value = Some(value);
        self
    }

    fn with_checksum(mut self, checksum: &str) -> Self {
        self.checksum = Some(checksum.to_string());
        self
    }

    fn with_capture_status(mut self, status: CaptureStatus) -> Self {
        self.capture_status = Some(status);
        self
    }

    fn with_metadata(mut self, metadata: serde_json::Value) -> Self {
        self.metadata = Some(metadata);
        self
    }
}

impl Default for ItemOverrides {
    fn default() -> Self {
        Self {
            agent: None,
            name: None,
            value: None,
            checksum: None,
            capture_status: None,
            metadata: None,
        }
    }
}

#[test]
fn builds_graph_with_project_override_and_provenance() {
    let evidence = vec![
        item(
            "user-permission-bash",
            EvidenceKind::Permission,
            "~/.claude/settings.json",
            EvidenceScope::User,
            10,
            ItemOverrides::default()
                .with_name("Bash(git status)")
                .with_value(serde_json::json!({"action": "allow", "rule": "Bash(git status)"})),
        ),
        item(
            "project-permission-bash",
            EvidenceKind::Permission,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("Bash(git status)")
                .with_value(serde_json::json!({"action": "deny", "rule": "Bash(git status)"})),
        ),
    ];

    let graph = build_graph(&evidence);
    let user_node = graph
        .iter()
        .find(|node| node.evidence_id == "user-permission-bash")
        .expect("user node");
    let project_node = graph
        .iter()
        .find(|node| node.evidence_id == "project-permission-bash")
        .expect("project node");

    assert_eq!(user_node.overridden_by.as_deref(), Some(project_node.id.as_str()));
    assert_eq!(
        project_node.effective_value,
        serde_json::json!({"action": "deny", "rule": "Bash(git status)"})
    );

    let provenance = build_provenance(&graph, &evidence);
    assert_eq!(
        provenance
            .iter()
            .find(|entry| entry.evidence_id == "project-permission-bash"),
        Some(&hem_core::ProvenanceEntry {
            node_id: project_node.id.clone(),
            evidence_id: "project-permission-bash".to_string(),
            source_path: ".claude/settings.json".to_string(),
            scope: EvidenceScope::Project,
            precedence: 40,
            confidence: EvidenceConfidence::High,
            capture_status: CaptureStatus::Captured,
        })
    );
}

#[test]
fn audits_project_override_wildcard_parse_failure_symlink_and_secret_like_values() {
    let evidence = vec![
        item(
            "user-policy",
            EvidenceKind::Permission,
            "~/.claude/settings.json",
            EvidenceScope::User,
            10,
            ItemOverrides::default()
                .with_name("Bash(git status)")
                .with_value(serde_json::json!({"action": "allow", "rule": "Bash(git status)"})),
        ),
        item(
            "project-policy",
            EvidenceKind::Permission,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("Bash(git status)")
                .with_value(serde_json::json!({"action": "deny", "rule": "Bash(git status)"})),
        ),
        item(
            "project-wildcard",
            EvidenceKind::Permission,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("Bash(*)")
                .with_value(serde_json::json!({"action": "allow", "rule": "Bash(*)"})),
        ),
        item(
            "bad-json",
            EvidenceKind::AgentConfig,
            ".mcp.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_capture_status(CaptureStatus::ParseFailed)
                .with_metadata(serde_json::json!({"error": "Unexpected token"})),
        ),
        item(
            "skipped-link",
            EvidenceKind::Symlink,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_capture_status(CaptureStatus::Omitted)
                .with_metadata(serde_json::json!({"skipped": true, "target": "../private/settings.json"})),
        ),
        item(
            "env-token",
            EvidenceKind::EnvKey,
            ".env",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_capture_status(CaptureStatus::Omitted)
                .with_name("OPENAI_API_KEY")
                .with_metadata(serde_json::json!({"secretLike": true})),
        ),
    ];

    let findings = audit_evidence(&evidence, &build_graph(&evidence));
    let codes: Vec<_> = findings.iter().map(|f| f.code.as_str()).collect();
    assert_eq!(
        codes,
        vec![
            "PARSE_FAILED",
            "PERMISSION_WILDCARD_ADDED",
            "PROJECT_OVERRIDES_USER_POLICY",
            "SYMLINK_SKIPPED",
            "SECRET_LIKE_VALUE_OMITTED"
        ]
    );
    for finding in &findings {
        assert!(!finding.problem.is_empty());
        assert!(!finding.cause.is_empty());
        assert!(!finding.fix.is_empty());
        assert!(finding.path.is_some());
        assert!(finding.evidence_id.is_some());
    }
}

#[test]
fn diffs_semantic_mcp_changes_and_raw_source_changes() {
    let baseline_graph = build_graph(&[item(
        "github-old",
        EvidenceKind::McpServer,
        ".mcp.json",
        EvidenceScope::Project,
        40,
        ItemOverrides::default()
            .with_name("github")
            .with_value(serde_json::json!({
                "transport": "stdio",
                "command": "mcp-github",
                "args": ["--read-only"]
            }))
            .with_checksum("old"),
    )]);
    let current_graph = build_graph(&[item(
        "github-new",
        EvidenceKind::McpServer,
        ".mcp.json",
        EvidenceScope::Project,
        40,
        ItemOverrides::default()
            .with_name("github")
            .with_value(serde_json::json!({
                "transport": "http",
                "url": "https://mcp.example.com/github"
            }))
            .with_checksum("new"),
    )]);

    let diff = diff_graphs(&baseline_graph, &current_graph);
    assert_eq!(diff.raw_source_changes.len(), 1);
    assert_eq!(diff.raw_source_changes[0].source_path, ".mcp.json");
    assert_eq!(diff.raw_source_changes[0].before_evidence_id.as_deref(), Some("github-old"));
    assert_eq!(diff.raw_source_changes[0].after_evidence_id.as_deref(), Some("github-new"));
    assert_eq!(diff.raw_source_changes[0].status, "changed");
    assert_eq!(diff.semantic_changes.len(), 1);
    assert_eq!(diff.semantic_changes[0].code, SemanticChangeCode::McpChanged);
    assert_eq!(diff.semantic_changes[0].entity_name, "github");
    let mut fields = diff.semantic_changes[0].details.changed_fields.clone();
    fields.sort();
    assert_eq!(fields, vec!["command", "transport", "urlHost"]);
}

#[test]
fn diffs_setup_inventory_changes_for_save_titles() {
    let baseline_graph = build_graph(&[
        item(
            "instructions-old",
            EvidenceKind::AgentInstruction,
            "AGENTS.md",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("AGENTS.md")
                .with_value(serde_json::json!({"checksum": "old"}))
                .with_checksum("old"),
        ),
        item(
            "skill-old",
            EvidenceKind::Skill,
            ".claude/skills/legacy-review/SKILL.md",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("legacy-review")
                .with_value(serde_json::json!({"installed": true}))
                .with_checksum("legacy"),
        ),
        item(
            "hook-old",
            EvidenceKind::Hook,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("pre-tool-use")
                .with_value(serde_json::json!({"command": "old-hook"}))
                .with_checksum("hook-old"),
        ),
        item(
            "permission-old",
            EvidenceKind::Permission,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("Bash(git status)")
                .with_value(serde_json::json!({"action": "allow", "rule": "Bash(git status)"}))
                .with_checksum("permission-old"),
        ),
        item(
            "permission-removed",
            EvidenceKind::Permission,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("Bash(bun test)")
                .with_value(serde_json::json!({"action": "allow", "rule": "Bash(bun test)"}))
                .with_checksum("permission-removed"),
        ),
    ]);
    let current_graph = build_graph(&[
        item(
            "instructions-new",
            EvidenceKind::AgentInstruction,
            "AGENTS.md",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("AGENTS.md")
                .with_value(serde_json::json!({"checksum": "new"}))
                .with_checksum("new"),
        ),
        item(
            "skill-new",
            EvidenceKind::Skill,
            ".claude/skills/react-review/SKILL.md",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("react-review")
                .with_value(serde_json::json!({"installed": true}))
                .with_checksum("skill"),
        ),
        item(
            "hook-new",
            EvidenceKind::Hook,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("pre-tool-use")
                .with_value(serde_json::json!({"command": "new-hook"}))
                .with_checksum("hook-new"),
        ),
        item(
            "hook-added",
            EvidenceKind::Hook,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("post-tool-use")
                .with_value(serde_json::json!({"command": "notify"}))
                .with_checksum("hook-added"),
        ),
        item(
            "permission-new",
            EvidenceKind::Permission,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("Bash(git status)")
                .with_value(serde_json::json!({"action": "deny", "rule": "Bash(git status)"}))
                .with_checksum("permission-new"),
        ),
        item(
            "permission-added",
            EvidenceKind::Permission,
            ".claude/settings.json",
            EvidenceScope::Project,
            40,
            ItemOverrides::default()
                .with_name("Bash(bun run build)")
                .with_value(serde_json::json!({"action": "allow", "rule": "Bash(bun run build)"}))
                .with_checksum("permission-added"),
        ),
    ]);

    let diff = diff_graphs(&baseline_graph, &current_graph);
    let mut codes: Vec<_> = diff
        .semantic_changes
        .iter()
        .map(|change| change.code.as_str())
        .collect();
    codes.sort();
    assert_eq!(
        codes,
        vec![
            "HOOK_ADDED",
            "HOOK_CHANGED",
            "INSTRUCTION_CHANGED",
            "PERMISSION_CHANGED",
            "PERMISSION_CHANGED",
            "PERMISSION_CHANGED",
            "SKILL_ADDED",
            "SKILL_REMOVED"
        ]
    );
    assert!(diff.semantic_changes.iter().any(|change| {
        change.code == SemanticChangeCode::PermissionChanged
            && change
                .details
                .extra
                .get("removed")
                .and_then(|v| v.as_bool())
                .unwrap_or(false)
    }));
}