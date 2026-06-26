use std::collections::HashMap;

use crate::diff::GraphDiff;
use crate::types::{AuditFinding, DiscoveredItem, EvidenceScope, GraphNode, ProvenanceEntry};

#[derive(Debug, Clone)]
pub struct ReportInput<'a> {
    pub snapshot_name: Option<&'a str>,
    pub current: Option<&'a str>,
    pub trust: ReportTrust,
    pub evidence: &'a [DiscoveredItem],
    pub graph: &'a [GraphNode],
    pub findings: &'a [AuditFinding],
    pub provenance: &'a [ProvenanceEntry],
    pub blind_spots: &'a [String],
    pub diffs: Option<&'a GraphDiff>,
}

#[derive(Debug, Clone)]
pub struct ReportTrust {
    pub read_only: bool,
    pub network: String,
    pub commands_executed: u32,
}

impl Default for ReportTrust {
    fn default() -> Self {
        Self {
            read_only: true,
            network: "none".to_string(),
            commands_executed: 0,
        }
    }
}

fn agent_names(agent: &str) -> &str {
    match agent {
        "claude-code" => "Claude Code",
        "codex" => "Codex",
        "cursor" => "Cursor",
        "project" => "Project",
        "unknown" => "Unknown",
        other => other,
    }
}

fn agent_line(agent: &str, items: &[DiscoveredItem]) -> String {
    let mut scopes = HashMap::new();
    for item in items {
        scopes.insert(item.scope, ());
    }
    let mut states = Vec::new();
    if scopes.contains_key(&EvidenceScope::User) {
        states.push("user state found");
    }
    if scopes.contains_key(&EvidenceScope::Project) {
        states.push("project state found");
    }
    if scopes.contains_key(&EvidenceScope::Managed) {
        states.push("managed state found");
    }
    if states.is_empty() {
        states.push("state found");
    }
    format!(
        "- {}  {}",
        agent_names(agent),
        states.join(", ")
    )
}

fn finding_line(finding: &AuditFinding) -> String {
    let path = finding
        .path
        .as_ref()
        .map(|p| format!(" ({p})"))
        .unwrap_or_default();
    format!(
        "- {} {}: {}{}",
        severity_label(finding.severity),
        finding.code,
        finding.problem,
        path
    )
}

fn severity_label(severity: crate::types::Severity) -> String {
    match severity {
        crate::types::Severity::None => "NONE",
        crate::types::Severity::Low => "LOW",
        crate::types::Severity::Medium => "MEDIUM",
        crate::types::Severity::High => "HIGH",
        crate::types::Severity::Critical => "CRITICAL",
    }
    .to_string()
}

fn provenance_line(entry: &ProvenanceEntry) -> String {
    format!(
        "- {} -> {} from {} ({}, precedence {}, {})",
        entry.evidence_id,
        entry.node_id,
        entry.source_path,
        entry.scope.as_str(),
        entry.precedence,
        entry.capture_status.as_str()
    )
}

fn capture_status_counts(evidence: &[DiscoveredItem]) -> HashMap<String, u32> {
    let mut counts = HashMap::new();
    for item in evidence {
        *counts
            .entry(item.capture_status.as_str().to_string())
            .or_insert(0) += 1;
    }
    counts
}

pub fn render_markdown_report(input: &ReportInput<'_>) -> String {
    let snapshot_name = input
        .snapshot_name
        .or(input.current)
        .unwrap_or("current");
    let mut lines = vec![
        format!("# hem report: {snapshot_name}"),
        String::new(),
        "## Trust".to_string(),
        format!(
            "- Read-only: {}",
            if input.trust.read_only { "yes" } else { "no" }
        ),
        format!("- Network: {}", input.trust.network),
        format!(
            "- Commands executed: {}",
            input.trust.commands_executed
        ),
        String::new(),
        "## Detected agents".to_string(),
    ];

    let mut by_agent: HashMap<String, Vec<&DiscoveredItem>> = HashMap::new();
    for item in input.evidence {
        by_agent
            .entry(item.agent.as_str().to_string())
            .or_default()
            .push(item);
    }

    if by_agent.is_empty() {
        lines.push("- None detected".to_string());
    } else {
        let mut agents: Vec<_> = by_agent.keys().cloned().collect();
        agents.sort();
        for agent in agents {
            let items = by_agent.get(&agent).cloned().unwrap_or_default();
            let owned: Vec<DiscoveredItem> = items.into_iter().cloned().collect();
            lines.push(agent_line(&agent, &owned));
        }
    }

    lines.push(String::new());
    lines.push("## High-signal findings".to_string());
    if input.findings.is_empty() {
        lines.push("- None".to_string());
    } else {
        for finding in input.findings {
            lines.push(finding_line(finding));
        }
    }

    lines.push(String::new());
    lines.push("## Blind spots".to_string());
    if input.blind_spots.is_empty() {
        lines.push("- None".to_string());
    } else {
        for blind_spot in input.blind_spots {
            lines.push(format!("- {blind_spot}"));
        }
    }

    lines.push(String::new());
    lines.push("## Reproducibility gaps".to_string());
    let counts = capture_status_counts(input.evidence);
    if counts.is_empty() {
        lines.push("- None".to_string());
    } else {
        let mut statuses: Vec<_> = counts.keys().cloned().collect();
        statuses.sort();
        for status in statuses {
            lines.push(format!("- {status}: {}", counts[&status]));
        }
    }

    if let Some(diffs) = input.diffs {
        lines.push(String::new());
        lines.push("## Semantic diff".to_string());
        if diffs.semantic_changes.is_empty() {
            lines.push("- None".to_string());
        } else {
            for change in &diffs.semantic_changes {
                lines.push(format!(
                    "- {} {}: {}",
                    severity_label(change.severity),
                    change.code.as_str(),
                    change.entity_name
                ));
            }
        }

        lines.push(String::new());
        lines.push("## Raw source changes".to_string());
        if diffs.raw_source_changes.is_empty() {
            lines.push("- None".to_string());
        } else {
            for change in &diffs.raw_source_changes {
                lines.push(format!("- {}: {}", change.status, change.source_path));
            }
        }
    }

    lines.push(String::new());
    lines.push("## Provenance".to_string());
    if input.provenance.is_empty() {
        lines.push("- None".to_string());
    } else {
        for entry in input.provenance {
            lines.push(provenance_line(entry));
        }
    }

    lines.push(String::new());
    lines.push("## Next".to_string());
    lines.push(
        "- `hem snapshot create --name baseline --agent codex --scope user --project .`".to_string(),
    );

    format!("{}\n", lines.join("\n"))
}