pub mod base;
pub mod filesystem;
pub mod plugins;

use std::path::{Path, PathBuf};

use crate::types::{
    AgentId, EvidenceKind, EvidenceParser, EvidenceScope, ScanOptions, ScanResult, ScanTrust,
};

pub use base::{
    array_of_strings, as_object, as_record, metadata_string_array, normalize_source_path,
    scanner_item_id, unquote_yaml_scalar, value_to_js_string, EvidenceBaseTarget, ItemIdTarget,
    ScannerBase,
};
pub use filesystem::{scan_skill_directory, scan_target, scan_targets};
pub use plugins::{
    claude_code_scanner, cursor_scanner, opencode_scanner, pi_agent_scanner, project_scanner,
    CodexScanner, ScannerContext, ScannerPlugin,
};

#[derive(Debug, Clone)]
pub struct ScanTarget {
    pub absolute_path: PathBuf,
    pub source_path: String,
    pub scope: EvidenceScope,
    pub agent: AgentId,
    pub kind: EvidenceKind,
    pub parser: EvidenceParser,
    pub precedence: u32,
    pub sensitivity: String,
    pub content_policy: String,
    pub directory: bool,
    pub metadata_only: bool,
}

#[derive(Debug, Clone, Default)]
pub struct ScanTargetOverrides {
    pub sensitivity: Option<String>,
    pub content_policy: Option<String>,
    pub directory: Option<bool>,
    pub metadata_only: Option<bool>,
    pub precedence: Option<u32>,
}

pub fn project_target(
    project_path: &Path,
    relative_path: &str,
    agent: AgentId,
    kind: EvidenceKind,
    parser: EvidenceParser,
    overrides: ScanTargetOverrides,
) -> ScanTarget {
    make_target(
        project_path,
        relative_path,
        EvidenceScope::Project,
        40,
        agent,
        kind,
        parser,
        overrides,
    )
}

pub fn home_target(
    home_dir: &Path,
    relative_path: &str,
    agent: AgentId,
    kind: EvidenceKind,
    parser: EvidenceParser,
    overrides: ScanTargetOverrides,
) -> ScanTarget {
    make_target(
        home_dir,
        relative_path,
        EvidenceScope::User,
        10,
        agent,
        kind,
        parser,
        overrides,
    )
}

fn make_target(
    root: &Path,
    relative_path: &str,
    scope: EvidenceScope,
    precedence: u32,
    agent: AgentId,
    kind: EvidenceKind,
    parser: EvidenceParser,
    overrides: ScanTargetOverrides,
) -> ScanTarget {
    let source_path = if scope == EvidenceScope::User {
        format!("~/{relative_path}")
    } else {
        relative_path.to_string()
    };

    ScanTarget {
        absolute_path: root.join(relative_path),
        source_path,
        scope,
        precedence: overrides.precedence.unwrap_or(precedence),
        agent,
        kind,
        parser,
        sensitivity: overrides
            .sensitivity
            .unwrap_or_else(|| "metadata".to_string()),
        content_policy: overrides
            .content_policy
            .unwrap_or_else(|| "metadata_only".to_string()),
        directory: overrides.directory.unwrap_or(false),
        metadata_only: overrides.metadata_only.unwrap_or(false),
    }
}

pub fn default_scanner_plugins() -> Vec<Box<dyn ScannerPlugin>> {
    vec![
        Box::new(claude_code_scanner()),
        Box::new(CodexScanner),
        Box::new(cursor_scanner()),
        Box::new(opencode_scanner()),
        Box::new(pi_agent_scanner()),
        Box::new(project_scanner()),
    ]
}

pub fn scan_project(options: &ScanOptions) -> ScanResult {
    let project_path = resolve_path(&options.project_path);
    let home_dir = resolve_path(&options.home_dir);
    let context = ScannerContext {
        project_path: project_path.clone(),
        home_dir: home_dir.clone(),
        store_dir: options.store_dir.clone(),
        explain: options.explain.unwrap_or(false),
        scope: options.scope,
    };

    let mut evidence = Vec::new();

    for plugin in default_scanner_plugins() {
        if options.agent.is_some_and(|agent| plugin.agent_id() != agent) {
            continue;
        }

        if let Some(items) = plugin.scan(&context) {
            evidence.extend(items);
            continue;
        }

        let targets = plugin
            .targets(&project_path, &home_dir)
            .into_iter()
            .filter(|target| options.scope.is_none_or(|scope| target.scope == scope))
            .collect::<Vec<_>>();
        evidence.extend(scan_targets(&targets));
    }

    let filtered_evidence = evidence
        .into_iter()
        .filter(|item| {
            options.agent.is_none_or(|agent| item.agent == agent)
                && options.scope.is_none_or(|scope| item.scope == scope)
        })
        .collect();

    ScanResult {
        trust: ScanTrust {
            read_only: true,
            network: "disabled".to_string(),
            commands_executed: Vec::new(),
            store_write_location: options.store_dir.clone(),
        },
        evidence: filtered_evidence,
        blind_spots: vec![
            "Remote MCP server behavior cannot be captured".to_string(),
            "Provider-side model routing cannot be verified".to_string(),
            "Raw env values are omitted by policy".to_string(),
        ],
    }
}

fn resolve_path(path: &str) -> PathBuf {
    let path = PathBuf::from(path);
    std::fs::canonicalize(&path).unwrap_or(path)
}