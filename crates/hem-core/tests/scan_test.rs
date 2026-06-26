use hem_core::{
    scan_project,
    types::{AgentId, CaptureStatus, EvidenceKind, EvidenceScope, ScanOptions},
};
use std::fs;
use std::path::PathBuf;
use tempfile::TempDir;

struct Sandbox {
    #[allow(dead_code)]
    root: TempDir,
    project_path: PathBuf,
    home_dir: PathBuf,
    store_dir: PathBuf,
}

fn make_sandbox() -> Sandbox {
    let root = TempDir::new().expect("temp dir");
    let project_path = root.path().join("project");
    let home_dir = root.path().join("home");
    let store_dir = home_dir.join(".hem");
    fs::create_dir_all(&project_path).expect("project dir");
    fs::create_dir_all(&home_dir).expect("home dir");
    Sandbox {
        root,
        project_path,
        home_dir,
        store_dir,
    }
}

fn scan_options(sandbox: &Sandbox) -> ScanOptions {
    ScanOptions {
        project_path: sandbox.project_path.display().to_string(),
        home_dir: sandbox.home_dir.display().to_string(),
        store_dir: sandbox.store_dir.display().to_string(),
        explain: None,
        agent: None,
        scope: None,
    }
}

#[test]
fn discovers_project_mcp_and_reports_read_only_trust() {
    let sandbox = make_sandbox();
    fs::write(
        sandbox.project_path.join(".mcp.json"),
        r#"{"mcpServers":{"github":{"command":"gh","args":["api"],"env":{"GITHUB_TOKEN":"secret"}}}}"#,
    )
    .expect("write mcp");

    let scan = scan_project(&scan_options(&sandbox));

    assert!(scan.trust.read_only);
    assert_eq!(scan.trust.network, "disabled");
    assert!(scan.trust.commands_executed.is_empty());
    assert_eq!(
        scan.trust.store_write_location,
        sandbox.store_dir.display().to_string()
    );

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
        .expect("github mcp");

    assert_eq!(github.restore_policy, hem_core::RestorePolicy::StructuredFieldsOnly);

    let serialized = serde_json::to_string(&scan.evidence).expect("serialize");
    assert!(!serialized.to_lowercase().contains("secret"));
    assert!(!serialized.contains(r#"GITHUB_TOKEN":"#));
    assert!(!serialized.contains(r#"GITHUB_TOKEN": "#));
}

#[test]
fn discovers_codex_mcp_servers_from_config_toml() {
    let sandbox = make_sandbox();
    fs::create_dir_all(sandbox.home_dir.join(".codex")).expect("codex dir");
    fs::write(
        sandbox.home_dir.join(".codex/config.toml"),
        r#"[mcp_servers.context7] # docs server
command = "npx"
args = [
  "-y",
  "@upstash/context7-mcp",
]

[mcp_servers.node_repl]
command = "node"
enabled = false

[mcp_servers.node_repl.env]
OPENAI_API_KEY = "secret"
"#,
    )
    .expect("write config");

    let scan = scan_project(&scan_options(&sandbox));
    let codex_mcp: Vec<_> = scan
        .evidence
        .iter()
        .filter(|item| item.agent == AgentId::Codex && item.kind == EvidenceKind::McpServer)
        .collect();

    let mut names: Vec<String> = codex_mcp
        .iter()
        .filter_map(|item| item.name.clone())
        .collect();
    names.sort();
    assert_eq!(names, vec!["context7", "node_repl"]);

    let context7 = codex_mcp
        .iter()
        .find(|item| item.name.as_deref() == Some("context7"))
        .expect("context7");
    let node_repl = codex_mcp
        .iter()
        .find(|item| item.name.as_deref() == Some("node_repl"))
        .expect("node_repl");

    assert_eq!(
        context7.value.as_ref().and_then(|v| v.get("args")),
        Some(&serde_json::json!(["-y", "@upstash/context7-mcp"]))
    );
    assert_eq!(
        node_repl.value.as_ref().and_then(|v| v.get("enabled")),
        Some(&serde_json::json!(false))
    );
    assert_eq!(
        node_repl.value.as_ref().and_then(|v| v.get("envKeys")),
        Some(&serde_json::json!(["OPENAI_API_KEY"]))
    );

    let serialized = serde_json::to_string(&codex_mcp).expect("serialize");
    assert!(!serialized.contains("secret"));
}

#[test]
fn discovers_codex_skills_from_user_and_plugin_cache() {
    let sandbox = make_sandbox();
    let user_skill = sandbox
        .home_dir
        .join(".codex/skills/review/SKILL.md");
    let plugin_skill = sandbox.home_dir.join(
        ".codex/plugins/cache/openai-curated/build-web-apps/1.0.0/skills/react-best-practices/SKILL.md",
    );
    fs::create_dir_all(user_skill.parent().unwrap()).expect("user skill dir");
    fs::create_dir_all(plugin_skill.parent().unwrap()).expect("plugin skill dir");
    fs::write(user_skill, "---\nname: review\ndescription: Review code\n---\n").expect("user skill");
    fs::write(
        plugin_skill,
        "---\nname: react-best-practices\ndescription: React guidance\n---\n",
    )
    .expect("plugin skill");

    let scan = scan_project(&scan_options(&sandbox));
    let codex_skills: Vec<_> = scan
        .evidence
        .iter()
        .filter(|item| item.agent == AgentId::Codex && item.kind == EvidenceKind::Skill)
        .collect();

    assert!(codex_skills.iter().any(|item| {
        item.name.as_deref() == Some("review")
            && item.source_path == "~/.codex/skills/review"
    }));
    assert!(codex_skills.iter().any(|item| {
        item.name.as_deref() == Some("react-best-practices")
            && item.source_path
                == "~/.codex/plugins/cache/openai-curated/build-web-apps/1.0.0/skills/react-best-practices"
    }));
}

#[test]
fn discovers_codex_hooks_from_hook_files() {
    let sandbox = make_sandbox();
    let project_hooks = sandbox.project_path.join(".codex/hooks.json");
    let user_hooks = sandbox.home_dir.join(".codex/hooks.json");
    fs::create_dir_all(project_hooks.parent().unwrap()).expect("project hooks dir");
    fs::create_dir_all(user_hooks.parent().unwrap()).expect("user hooks dir");
    fs::write(
        project_hooks,
        r#"{"hooks":{"PreToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"project-hook","timeout":5}]}]}}"#,
    )
    .expect("project hooks");
    fs::write(
        user_hooks,
        r#"{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"user-hook"}]}],"Stop":[{"hooks":[{"type":"command","command":"stop-hook"}]}]}}"#,
    )
    .expect("user hooks");

    let scan = scan_project(&scan_options(&sandbox));
    let codex_hooks: Vec<_> = scan
        .evidence
        .iter()
        .filter(|item| item.agent == AgentId::Codex && item.kind == EvidenceKind::Hook)
        .collect();

    let mut names: Vec<_> = codex_hooks
        .iter()
        .filter_map(|item| item.name.clone())
        .collect();
    names.sort();
    assert_eq!(
        names,
        vec!["PreToolUse.Write", "SessionStart.*", "Stop.*"]
    );
    assert!(codex_hooks.iter().any(|item| {
        item.source_path == ".codex/hooks.json" && item.name.as_deref() == Some("PreToolUse.Write")
    }));
    assert!(codex_hooks.iter().any(|item| {
        item.source_path == "~/.codex/hooks.json" && item.name.as_deref() == Some("SessionStart.*")
    }));
}

#[test]
fn filters_codex_user_global_evidence_when_agent_and_scope_specified() {
    let sandbox = make_sandbox();
    fs::create_dir_all(sandbox.project_path.join(".codex")).expect("project codex");
    fs::create_dir_all(sandbox.home_dir.join(".codex/skills/review")).expect("skill dir");
    fs::create_dir_all(sandbox.project_path.join(".claude")).expect("claude dir");
    fs::write(sandbox.project_path.join("AGENTS.md"), "project instructions").expect("agents");
    fs::write(
        sandbox.project_path.join(".codex/hooks.json"),
        r#"{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"project-hook"}]}]}}"#,
    )
    .expect("project hooks");
    fs::write(
        sandbox.project_path.join(".claude/settings.json"),
        r#"{"permissions":{"allow":["Bash(echo hi)"]}}"#,
    )
    .expect("claude settings");
    fs::write(
        sandbox.home_dir.join(".codex/config.toml"),
        "model = \"gpt-5\"\n[mcp_servers.docs]\ncommand = \"docs-mcp\"\n\n[[hooks.Stop]]\n[[hooks.Stop.hooks]]\ntype = \"command\"\ncommand = \"notify\"\n",
    )
    .expect("codex config");
    fs::write(
        sandbox.home_dir.join(".codex/skills/review/SKILL.md"),
        "---\nname: review\n---\n",
    )
    .expect("skill");

    let scan = scan_project(&ScanOptions {
        agent: Some(AgentId::Codex),
        scope: Some(EvidenceScope::User),
        ..scan_options(&sandbox)
    });

    assert!(!scan.evidence.is_empty());
    assert!(scan.evidence.iter().all(|item| item.agent == AgentId::Codex));
    assert!(scan
        .evidence
        .iter()
        .all(|item| item.scope == EvidenceScope::User));
    assert!(scan
        .evidence
        .iter()
        .all(|item| item.source_path.starts_with("~/.codex/")));
    assert!(!scan.evidence.iter().any(|item| item.source_path == "AGENTS.md"));
    assert!(!scan
        .evidence
        .iter()
        .any(|item| item.source_path == ".codex/hooks.json"));
    assert!(!scan
        .evidence
        .iter()
        .any(|item| item.agent == AgentId::ClaudeCode));
    assert!(scan.evidence.iter().any(|item| {
        item.kind == EvidenceKind::AgentConfig && item.source_path == "~/.codex/config.toml"
    }));
    assert!(scan.evidence.iter().any(|item| {
        item.kind == EvidenceKind::McpServer && item.name.as_deref() == Some("docs")
    }));
    assert!(scan
        .evidence
        .iter()
        .any(|item| item.kind == EvidenceKind::Hook && item.name.as_deref() == Some("Stop.*")));
    assert!(scan.evidence.iter().any(|item| {
        item.kind == EvidenceKind::Skill && item.name.as_deref() == Some("review")
    }));
}

#[test]
fn emits_parse_failed_for_malformed_json() {
    let sandbox = make_sandbox();
    fs::create_dir_all(sandbox.project_path.join(".claude")).expect("claude dir");
    fs::write(
        sandbox.project_path.join(".claude/settings.json"),
        "{ not json",
    )
    .expect("bad json");

    let scan = scan_project(&scan_options(&sandbox));
    assert!(scan.evidence.iter().any(|item| {
        item.source_path == ".claude/settings.json"
            && item.parser == hem_core::EvidenceParser::Json
            && item.capture_status == CaptureStatus::ParseFailed
    }));
}

#[test]
fn records_symlink_evidence_without_following() {
    let sandbox = make_sandbox();
    fs::write(sandbox.project_path.join("CLAUDE.md"), "project instructions").expect("claude md");
    #[cfg(unix)]
    {
        std::os::unix::fs::symlink(
            sandbox.project_path.join("CLAUDE.md"),
            sandbox.project_path.join("AGENTS.md"),
        )
        .expect("symlink");
    }
    #[cfg(not(unix))]
    {
        return;
    }

    let scan = scan_project(&scan_options(&sandbox));
    assert!(scan.evidence.iter().any(|item| {
        item.kind == EvidenceKind::Symlink
            && item.source_path == "AGENTS.md"
            && item.capture_status == CaptureStatus::Omitted
            && item
                .metadata
                .as_ref()
                .and_then(|m| m.get("reason"))
                .and_then(|v| v.as_str())
                == Some("symlink_not_followed")
    }));
    assert!(!scan.evidence.iter().any(|item| {
        item.source_path == "AGENTS.md" && item.kind == EvidenceKind::AgentInstruction
    }));
}

#[test]
fn captures_dotenv_key_inventory_while_omitting_secret_values() {
    let sandbox = make_sandbox();
    fs::write(
        sandbox.project_path.join(".env"),
        "OPENAI_API_KEY=sk-real-secret\nHEM_MODE=local\n",
    )
    .expect("env");

    let scan = scan_project(&scan_options(&sandbox));
    let env_evidence: Vec<_> = scan
        .evidence
        .iter()
        .filter(|item| item.kind == EvidenceKind::EnvKey)
        .collect();

    assert!(env_evidence.iter().any(|item| {
        item.name.as_deref() == Some("OPENAI_API_KEY")
            && item.capture_status == CaptureStatus::Redacted
            && item.value.is_none()
    }));
    assert!(env_evidence.iter().any(|item| {
        item.name.as_deref() == Some("HEM_MODE")
            && item.capture_status == CaptureStatus::Omitted
            && item.value.is_none()
    }));

    let serialized = serde_json::to_string(&env_evidence).expect("serialize");
    assert!(!serialized.contains("sk-real-secret"));
    assert!(!serialized.contains("local"));
}

fn seed_verification_fixture(sandbox: &Sandbox) {
    fs::write(
        sandbox.project_path.join(".mcp.json"),
        r#"{"mcpServers":{"claude-mcp":{"command":"claude"}}}"#,
    )
    .expect("claude mcp");
    fs::create_dir_all(sandbox.home_dir.join(".codex")).expect("codex");
    fs::write(
        sandbox.home_dir.join(".codex/config.toml"),
        r#"[mcp_servers.context7]
command = "npx"

[mcp_servers.node_repl]
command = "node"

[mcp_servers.node_repl.env]
OPENAI_API_KEY = "secret"
"#,
    )
    .expect("codex config");
    fs::create_dir_all(sandbox.home_dir.join(".claude")).expect("claude");
    fs::write(
        sandbox.home_dir.join(".claude/settings.json"),
        r#"{"permissions":{"allow":["Bash(echo hi)"]}}"#,
    )
    .expect("claude settings");
    fs::create_dir_all(sandbox.project_path.join(".cursor")).expect("cursor");
    fs::write(
        sandbox.project_path.join(".cursor/mcp.json"),
        r#"{"mcpServers":{"cursor-mcp":{"command":"cursor"}}}"#,
    )
    .expect("cursor mcp");
    fs::create_dir_all(sandbox.home_dir.join(".config/opencode")).expect("opencode");
    fs::write(
        sandbox.home_dir.join(".config/opencode/opencode.json"),
        r#"{"mcp":{"opencode-mcp":{"type":"local","command":["opencode"]}}}"#,
    )
    .expect("opencode config");
    fs::create_dir_all(sandbox.project_path.join(".pi")).expect("pi");
    fs::write(
        sandbox.project_path.join(".pi/settings.json"),
        r#"{"skills":[]}"#,
    )
    .expect("pi settings");
    fs::write(sandbox.project_path.join(".env"), "HEM_MODE=local\n").expect("env");
}

#[test]
fn export_scan_evidence_fixture_for_scratch() {
    let scratch_dir = match std::env::var("HEM_SCRATCH_DIR") {
        Ok(dir) => dir,
        Err(_) => return,
    };

    let sandbox = make_sandbox();
    seed_verification_fixture(&sandbox);
    let scan = scan_project(&scan_options(&sandbox));

    let agents: std::collections::HashSet<_> = scan.evidence.iter().map(|item| item.agent).collect();
    assert!(agents.len() >= 4);
    assert!(scan.evidence.iter().any(|item| {
        item.agent == AgentId::Codex
            && item.kind == EvidenceKind::McpServer
            && item.name.as_deref() == Some("context7")
    }));
    assert!(scan.evidence.iter().any(|item| {
        item.restore_policy == hem_core::RestorePolicy::StructuredFieldsOnly
            && item.kind == EvidenceKind::McpServer
    }));

    let codex_mcp: Vec<_> = scan
        .evidence
        .iter()
        .filter(|item| item.agent == AgentId::Codex && item.kind == EvidenceKind::McpServer)
        .collect();
    let serialized = serde_json::to_string(&codex_mcp).expect("serialize codex mcp");
    assert!(!serialized.contains("secret"));

    let output = serde_json::json!({
        "entrypoint": "hem_core::scan_project",
        "projectPath": sandbox.project_path.display().to_string(),
        "homeDir": sandbox.home_dir.display().to_string(),
        "agentCount": agents.len(),
        "agents": agents.iter().map(|agent| agent.as_str()).collect::<Vec<_>>(),
        "evidence": scan.evidence,
    });
    let path = std::path::Path::new(&scratch_dir).join("scan-evidence-fixture.json");
    fs::write(
        &path,
        serde_json::to_string_pretty(&output).expect("pretty json"),
    )
    .expect("write scratch fixture");
}

#[test]
fn multi_agent_sandbox_discovers_at_least_four_agents() {
    let sandbox = make_sandbox();
    seed_verification_fixture(&sandbox);

    let scan = scan_project(&scan_options(&sandbox));
    let agents: std::collections::HashSet<_> = scan.evidence.iter().map(|item| item.agent).collect();

    assert!(agents.contains(&AgentId::ClaudeCode));
    assert!(agents.contains(&AgentId::Codex));
    assert!(agents.contains(&AgentId::Cursor));
    assert!(agents.contains(&AgentId::Opencode));
    assert!(agents.contains(&AgentId::PiAgent));
    assert!(agents.contains(&AgentId::Project));
    assert!(agents.len() >= 4);
}