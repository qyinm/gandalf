use serde_json::{Map, Value};

use crate::graph::build_graph;
use crate::types::{AuditFinding, DiscoveredItem, GraphNode, Severity};

fn record(value: &Value) -> Option<&Map<String, Value>> {
    value.as_object()
}

fn finding(
    code: &str,
    severity: Severity,
    problem: &str,
    cause: &str,
    fix: &str,
    item: &DiscoveredItem,
) -> AuditFinding {
    AuditFinding {
        code: code.to_string(),
        severity,
        problem: problem.to_string(),
        cause: cause.to_string(),
        fix: fix.to_string(),
        path: Some(item.source_path.clone()),
        evidence_id: Some(item.id.clone()),
    }
}

fn is_wildcard_permission(item: &DiscoveredItem) -> bool {
    if item.kind != crate::types::EvidenceKind::Permission {
        return false;
    }
    let rule = item
        .value
        .as_ref()
        .and_then(|v| record(v))
        .and_then(|m| m.get("rule"))
        .and_then(|v| v.as_str())
        .unwrap_or(item.name.as_deref().unwrap_or(""));
    rule == "*" || rule.contains('*') || rule.contains("(*)")
}

fn is_secret_like(item: &DiscoveredItem) -> bool {
    if item.capture_status != crate::types::CaptureStatus::Omitted
        && item.capture_status != crate::types::CaptureStatus::Redacted
    {
        return false;
    }
    if item
        .metadata
        .as_ref()
        .and_then(|m| m.get("secretLike"))
        .and_then(|v| v.as_bool())
        .unwrap_or(false)
    {
        return true;
    }
    let name = item.name.as_deref().unwrap_or(&item.id);
    regex::Regex::new(r"(?i)(?:secret|token|api[_-]?key|password|credential)")
        .expect("regex")
        .is_match(name)
}

fn has_executable_config(item: &DiscoveredItem) -> bool {
    let value = item.value.as_ref().and_then(|v| record(v));
    if item.kind == crate::types::EvidenceKind::McpServer {
        if let Some(command) = value.and_then(|m| m.get("command")).and_then(|v| v.as_str()) {
            return !command.is_empty();
        }
    }
    if matches!(
        item.kind,
        crate::types::EvidenceKind::Hook
            | crate::types::EvidenceKind::Skill
            | crate::types::EvidenceKind::Extension
    ) {
        return item
            .metadata
            .as_ref()
            .and_then(|m| m.get("executable"))
            .and_then(|v| v.as_bool())
            .unwrap_or(false);
    }
    false
}

fn project_override_findings(
    graph: &[GraphNode],
    evidence_by_id: &std::collections::HashMap<&str, &DiscoveredItem>,
) -> Vec<AuditFinding> {
    let nodes_by_id: std::collections::HashMap<&str, &GraphNode> =
        graph.iter().map(|n| (n.id.as_str(), n)).collect();
    let mut findings = Vec::new();
    let mut emitted = std::collections::HashSet::new();

    for node in graph {
        let Some(overridden_by) = &node.overridden_by else {
            continue;
        };
        let overriding_node = nodes_by_id.get(overridden_by.as_str());
        let overridden_evidence = evidence_by_id.get(node.evidence_id.as_str());
        let overriding_evidence = overriding_node
            .and_then(|n| evidence_by_id.get(n.evidence_id.as_str()));
        if overridden_evidence.is_none()
            || overriding_evidence.is_none()
            || node.scope != crate::types::EvidenceScope::User
            || overriding_node.map(|n| n.scope) != Some(crate::types::EvidenceScope::Project)
        {
            continue;
        }
        let key = format!(
            "{}:{}",
            overridden_evidence.unwrap().id,
            overriding_evidence.unwrap().id
        );
        if !emitted.insert(key) {
            continue;
        }
        findings.push(AuditFinding {
            code: "PROJECT_OVERRIDES_USER_POLICY".to_string(),
            severity: Severity::High,
            problem: "Project configuration overrides a user-level agent policy.".to_string(),
            cause: format!(
                "{} has higher precedence than {} for {}.",
                overriding_evidence.unwrap().source_path,
                overridden_evidence.unwrap().source_path,
                node.entity_name
            ),
            fix: "Review the project-level rule and remove it if the override is not intentional."
                .to_string(),
            path: Some(overriding_evidence.unwrap().source_path.clone()),
            evidence_id: Some(overriding_evidence.unwrap().id.clone()),
        });
    }
    findings
}

pub fn audit_evidence(evidence: &[DiscoveredItem], graph: &[GraphNode]) -> Vec<AuditFinding> {
    let mut findings = Vec::new();
    let evidence_by_id: std::collections::HashMap<&str, &DiscoveredItem> =
        evidence.iter().map(|item| (item.id.as_str(), item)).collect();

    for item in evidence {
        if has_executable_config(item) {
            findings.push(finding(
                "EXECUTABLE_CONFIG_ADDED",
                Severity::Medium,
                "Configuration references an executable command or hook.",
                &format!(
                    "{} contains executable configuration for {}.",
                    item.source_path,
                    item.name.as_deref().unwrap_or(&item.id)
                ),
                "Confirm the command is trusted and keep only explicit, necessary executable entries.",
                item,
            ));
        }

        if item
            .metadata
            .as_ref()
            .and_then(|m| m.get("remote"))
            .and_then(|v| v.as_bool())
            .unwrap_or(false)
            && item
                .metadata
                .as_ref()
                .and_then(|m| m.get("changed"))
                .and_then(|v| v.as_bool())
                .unwrap_or(false)
        {
            findings.push(finding(
                "REMOTE_MCP_CHANGED",
                Severity::Medium,
                "Remote MCP configuration changed.",
                &format!(
                    "{} marks {} as remote and changed.",
                    item.source_path,
                    item.name.as_deref().unwrap_or(&item.id)
                ),
                "Review the remote URL and host before trusting this MCP server.",
                item,
            ));
        }

        if is_wildcard_permission(item) {
            findings.push(finding(
                "PERMISSION_WILDCARD_ADDED",
                Severity::High,
                "Project settings added a broad permission wildcard.",
                &format!(
                    "{} contains {}.",
                    item.source_path,
                    item.name.as_deref().unwrap_or("a wildcard permission")
                ),
                "Replace the wildcard with explicit allowed commands or resources.",
                item,
            ));
        }

        if is_secret_like(item) {
            findings.push(finding(
                "SECRET_LIKE_VALUE_OMITTED",
                Severity::Medium,
                "A secret-like value was detected and omitted from the evidence inventory.",
                &format!(
                    "{} contains {}, which matches a sensitive key pattern.",
                    item.source_path,
                    item.name.as_deref().unwrap_or(&item.id)
                ),
                "Keep the value out of snapshots and rotate it if it may have been exposed elsewhere.",
                item,
            ));
        }

        if item.capture_status == crate::types::CaptureStatus::ParseFailed {
            let error_suffix = item
                .metadata
                .as_ref()
                .and_then(|m| m.get("error"))
                .and_then(|v| v.as_str())
                .map(|e| format!(": {e}"))
                .unwrap_or_default();
            findings.push(finding(
                "PARSE_FAILED",
                Severity::High,
                "A relevant agent configuration file could not be parsed.",
                &format!("{} failed to parse{error_suffix}", item.source_path),
                "Fix the file syntax or exclude that source from the scan.",
                item,
            ));
        }

        if item.kind == crate::types::EvidenceKind::Symlink
            && (item.capture_status == crate::types::CaptureStatus::Omitted
                || item
                    .metadata
                    .as_ref()
                    .and_then(|m| m.get("skipped"))
                    .and_then(|v| v.as_bool())
                    .unwrap_or(false))
            && !item.source_path.contains("/skills/")
        {
            findings.push(finding(
                "SYMLINK_SKIPPED",
                Severity::High,
                "A symlink was found and not followed.",
                &format!(
                    "{} points outside the scanned file content boundary.",
                    item.source_path
                ),
                "Inspect the symlink manually and replace it with a regular config file if it should be captured.",
                item,
            ));
        }

        if item.kind == crate::types::EvidenceKind::Unsupported
            || item.capture_status == crate::types::CaptureStatus::Unsupported
        {
            findings.push(finding(
                "UNSUPPORTED_AGENT_STATE",
                Severity::Medium,
                "Agent state was detected but cannot yet be interpreted by hem.",
                &format!(
                    "{} is present, but its semantics are unsupported.",
                    item.source_path
                ),
                "Treat this as a blind spot and inspect the source manually before relying on the snapshot.",
                item,
            ));
        }

        if item
            .metadata
            .as_ref()
            .and_then(|m| m.get("worldWritable"))
            .and_then(|v| v.as_bool())
            .unwrap_or(false)
        {
            findings.push(finding(
                "WORLD_WRITABLE_STORE",
                Severity::Critical,
                "The hem store is marked world-writable.",
                &format!(
                    "{} metadata reports unsafe store permissions.",
                    item.source_path
                ),
                "Change the store permissions to 0700 before trusting stored snapshots.",
                item,
            ));
        }
    }

    findings.extend(project_override_findings(graph, &evidence_by_id));

    fn severity_rank(severity: Severity) -> u8 {
        match severity {
            Severity::Critical => 0,
            Severity::High => 1,
            Severity::Medium => 2,
            Severity::Low => 3,
            Severity::None => 4,
        }
    }

    findings.sort_by(|left, right| {
        severity_rank(left.severity)
            .cmp(&severity_rank(right.severity))
            .then_with(|| left.code.cmp(&right.code))
    });
    findings
}

pub fn audit_evidence_with_graph(evidence: &[DiscoveredItem]) -> Vec<AuditFinding> {
    audit_evidence(evidence, &build_graph(evidence))
}