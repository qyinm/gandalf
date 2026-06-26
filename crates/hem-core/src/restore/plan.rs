use std::collections::HashMap;
use std::path::Path;

use serde_json::{json, Map, Value};
use time::format_description::well_known::Rfc3339;
use time::OffsetDateTime;
use uuid::Uuid;

use crate::diff::{diff_graphs, SemanticChange};
use crate::path_confinement::{confinement_roots_from_paths, validate_constrained_write_path};
use crate::graph::build_graph;
use crate::scan::scan_project;
use crate::store::{read_snapshot, read_snapshot_content};
use crate::types::{
    AgentId, DiscoveredItem, EvidenceKind, ItemDiff, RestoreAction, RestoreItem,
    RestoreItemStatus, RestoreOptions, RestorePlan, RestorePlanItem, RestorePlanMetadata,
    RiskSummary, RollbackPlan, RollbackStep, ScanOptions, Severity, Snapshot,
    SnapshotContentEntry, UnsupportedPlanItem,
};

#[derive(Debug, Clone, PartialEq)]
pub struct ParseDryRunError {
    pub message: String,
}

#[derive(Debug, Clone, PartialEq)]
pub struct ParseDryRunResult {
    pub items: Vec<RestoreItem>,
    pub errors: Vec<ParseDryRunError>,
}

pub fn parse_dry_run_output(input: &str) -> ParseDryRunResult {
    let mut errors = Vec::new();
    let mut seen_ids = std::collections::HashSet::new();
    let mut result = Vec::new();

    let cleaned = input
        .lines()
        .filter(|line| {
            let trimmed = line.trim();
            !(trimmed.starts_with('#')
                || trimmed.starts_with("//")
                || trimmed.starts_with("---"))
        })
        .collect::<Vec<_>>()
        .join("\n")
        .trim()
        .to_string();

    if cleaned.is_empty() {
        return ParseDryRunResult {
            items: Vec::new(),
            errors,
        };
    }

    let parsed: Value = match serde_json::from_str(&cleaned) {
        Ok(value) => value,
        Err(error) => {
            errors.push(ParseDryRunError {
                message: format!("Failed to parse dry-run output as JSON: {error}"),
            });
            return ParseDryRunResult {
                items: Vec::new(),
                errors,
            };
        }
    };

    let Some(plan) = parsed.as_object() else {
        errors.push(ParseDryRunError {
            message: "Dry-run output is not a valid JSON object".to_string(),
        });
        return ParseDryRunResult {
            items: Vec::new(),
            errors,
        };
    };

    let target_project = plan
        .get("targetProject")
        .and_then(|v| v.as_str())
        .map(str::to_string);
    let target_home = plan
        .get("targetHome")
        .and_then(|v| v.as_str())
        .map(str::to_string);

    let Some(plan_items) = plan.get("items").and_then(|v| v.as_array()) else {
        errors.push(ParseDryRunError {
            message: "Dry-run plan is missing required \"items\" array".to_string(),
        });
        return ParseDryRunResult {
            items: Vec::new(),
            errors,
        };
    };

    let execution_order: Vec<String> = plan
        .get("executionOrder")
        .and_then(|v| v.as_array())
        .map(|items| {
            items
                .iter()
                .filter_map(|v| v.as_str().map(str::to_string))
                .collect()
        })
        .unwrap_or_default();
    let unsupported_items: Vec<UnsupportedPlanItem> = plan
        .get("unsupportedItems")
        .and_then(|v| serde_json::from_value(v.clone()).ok())
        .unwrap_or_default();

    let mut order_lookup = HashMap::new();
    for (index, item_id) in execution_order.iter().enumerate() {
        order_lookup.insert(item_id.clone(), (index + 1) as u32);
    }

    let mut items_item_ids = std::collections::HashSet::new();
    let mut next_append_order = execution_order.len() as u32 + 1;

    for raw_item in plan_items {
        let Ok(plan_item) = serde_json::from_value::<RestorePlanItem>(raw_item.clone()) else {
            errors.push(ParseDryRunError {
                message: format!(
                    "Skipping item: invalid restore plan item structure"
                ),
            });
            continue;
        };

        if seen_ids.contains(&plan_item.item_id) {
            errors.push(ParseDryRunError {
                message: format!("Duplicate itemId \"{}\" skipped", plan_item.item_id),
            });
            continue;
        }
        seen_ids.insert(plan_item.item_id.clone());
        items_item_ids.insert(plan_item.item_id.clone());

        let order = order_lookup
            .get(&plan_item.item_id)
            .copied()
            .unwrap_or_else(|| {
                let current = next_append_order;
                next_append_order += 1;
                current
            });
        let can_rollback = can_rollback_action(plan_item.action);
        let dest = resolve_plan_destination(
            &plan_item,
            target_project.as_deref(),
            target_home.as_deref(),
        );
        if let Some(roots) =
            confinement_roots_from_paths(target_home.as_deref(), target_project.as_deref())
        {
            if let Err(reason) = validate_constrained_write_path(Path::new(&dest), &roots) {
                errors.push(ParseDryRunError {
                    message: format!(
                        "Skipping item \"{}\": {reason}",
                        plan_item.item_id
                    ),
                });
                continue;
            }
        }

        result.push(RestoreItem {
            item_id: plan_item.item_id.clone(),
            path: plan_item.source_path.clone(),
            item_type: plan_item.kind.as_str().to_string(),
            source: plan_item.source_path.clone(),
            dest,
            action: Some(plan_item.action),
            status: if plan_item.action == RestoreAction::Unsupported {
                RestoreItemStatus::Unsupported
            } else {
                RestoreItemStatus::Pending
            },
            error_message: None,
            skip_reason: None,
            execution_order: order,
            rollback_state: None,
            target_content: target_content_for_plan_item(&plan_item),
            can_rollback,
            metadata: restore_item_metadata(&plan_item),
            apply_at: None,
        });
    }

    for unsupported in unsupported_items {
        if items_item_ids.contains(&unsupported.item_id) {
            continue;
        }
        if seen_ids.contains(&unsupported.item_id) {
            errors.push(ParseDryRunError {
                message: format!("Duplicate itemId \"{}\" skipped", unsupported.item_id),
            });
            continue;
        }
        seen_ids.insert(unsupported.item_id.clone());
        result.push(RestoreItem {
            item_id: unsupported.item_id.clone(),
            path: unsupported.source_path.clone(),
            item_type: unsupported.kind.as_str().to_string(),
            source: unsupported.source_path.clone(),
            dest: unsupported.source_path.clone(),
            action: Some(RestoreAction::Unsupported),
            status: RestoreItemStatus::Unsupported,
            error_message: None,
            skip_reason: Some(unsupported.reason.clone()),
            execution_order: next_append_order,
            rollback_state: None,
            target_content: None,
            can_rollback: false,
            metadata: None,
            apply_at: None,
        });
        next_append_order += 1;
    }

    result.sort_by_key(|item| item.execution_order);
    ParseDryRunResult { items: result, errors }
}

pub fn build_restore_plan(options: &RestoreOptions) -> Result<RestorePlan, Box<dyn std::error::Error>> {
    let snapshot = read_snapshot(
        Path::new(&options.store_dir),
        &options.source_snapshot,
        options.agent,
    )?;
    let scan = scan_project(&ScanOptions {
        project_path: options.project_path.clone(),
        home_dir: options.home_dir.clone(),
        store_dir: options.store_dir.clone(),
        explain: None,
        agent: options.agent,
        scope: options.scope,
    });
    let current_graph = build_graph(&scan.evidence);
    let snapshot_content = snapshot_content_by_evidence_id(&snapshot, options)?;

    let diff = diff_graphs(&snapshot.graph, &current_graph);
    let mut items = Vec::new();
    let mut unsupported_items = Vec::new();
    let mut execution_order = Vec::new();
    let mut risk_counts = RiskSummary::default();

    for change in &diff.semantic_changes {
        let source_path = change.details.source_path.clone();
        let current_state = find_matching_evidence(change, &scan.evidence, source_path.as_deref());
        let target_state = with_snapshot_content(
            find_matching_evidence(change, &snapshot.evidence, source_path.as_deref()),
            &snapshot_content,
        );
        let item_id = format!(
            "{}:{}:{}",
            change.entity_kind.as_str(),
            change.entity_name,
            &Uuid::new_v4().simple().to_string()[..8]
        );
        let restore_path = restore_path_from_content(target_state.as_ref())
            .or_else(|| restore_path_from_content(current_state.as_ref()))
            .or_else(|| restore_path_for_evidence_file(target_state.as_ref()))
            .or_else(|| restore_path_for_evidence_file(current_state.as_ref()))
            .unwrap_or_else(|| {
                source_path_for_restore_item(
                    change.entity_kind,
                    &change.entity_name,
                    current_state.as_ref(),
                    target_state.as_ref(),
                    source_path.as_deref(),
                )
            });
        let action = restore_action_for_change(change.code.as_str(), current_state.as_ref(), target_state.as_ref());

        let risk_level = change.severity;
        if matches!(action, RestoreAction::Unsupported | RestoreAction::Skip) {
            unsupported_items.push(UnsupportedPlanItem {
                item_id: item_id.clone(),
                agent: agent_for_restore_item(current_state.as_ref(), target_state.as_ref()),
                kind: change.entity_kind,
                source_path: restore_path.clone(),
                reason: unsupported_reason_for(
                    change.code.as_str(),
                    change.entity_kind,
                    &change.entity_name,
                    current_state.as_ref(),
                    target_state.as_ref(),
                ),
            });
            continue;
        }

        increment_risk(&mut risk_counts, risk_level);
        items.push(RestorePlanItem {
            item_id: item_id.clone(),
            agent: agent_for_restore_item(current_state.as_ref(), target_state.as_ref()),
            kind: change.entity_kind,
            source_path: restore_path,
            depends_on: Vec::new(),
            action,
            current_state,
            target_state,
            diff: ItemDiff {
                changes: change.details.changed_fields.clone(),
                additions: Vec::new(),
                removals: Vec::new(),
            },
            risk_level,
            risk_reason: format!(
                "Restore {:?} for {}: {}",
                action,
                change.entity_kind.as_str(),
                change.entity_name
            ),
            needs_confirmation: matches!(risk_level, Severity::High | Severity::Critical),
            confirmation_prompt: if matches!(risk_level, Severity::High | Severity::Critical) {
                format!(
                    "Restore {} \"{}\" with risk {:?}. Continue?",
                    change.entity_kind.as_str(),
                    change.entity_name,
                    risk_level
                )
            } else {
                String::new()
            },
            rollback_instruction: rollback_instruction_for(action, change.entity_kind, &change.entity_name),
        });
        execution_order.push(item_id);
    }

    let rollback_steps = build_rollback_steps(&items);
    Ok(RestorePlan {
        plan_id: format!("plan-{}", &Uuid::new_v4().simple().to_string()[..12]),
        source_snapshot: options.source_snapshot.clone(),
        target_project: options.project_path.clone(),
        target_home: options.home_dir.clone(),
        created_at: OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_string()),
        item_count: items.len() as u32,
        risk_summary: risk_counts,
        items,
        rollback_plan: RollbackPlan {
            steps: rollback_steps,
        },
        execution_order,
        unsupported_items,
        plan_metadata: RestorePlanMetadata {
            planner_version: "0.2.0".to_string(),
            generated_by: "hem restore".to_string(),
        },
    })
}

fn snapshot_content_by_evidence_id(
    snapshot: &Snapshot,
    options: &RestoreOptions,
) -> Result<HashMap<String, SnapshotContentEntry>, Box<dyn std::error::Error>> {
    let mut content = HashMap::new();
    for entry in snapshot.content.clone().unwrap_or_default() {
        if entry.capture_status != "captured" {
            continue;
        }
        let text = read_snapshot_content(
            Path::new(&options.store_dir),
            &options.source_snapshot,
            &entry,
            options.agent,
        )?;
        let mut with_content = entry;
        with_content.content = Some(text);
        content.insert(with_content.evidence_id.clone(), with_content);
    }
    Ok(content)
}

fn target_content_for_plan_item(plan_item: &RestorePlanItem) -> Option<Value> {
    let value = plan_item
        .target_state
        .as_ref()
        .and_then(|state| state.value.as_ref())?;
    match plan_item.kind {
        EvidenceKind::McpServer | EvidenceKind::Permission | EvidenceKind::EnvKey => {
            Some(value.clone())
        }
        _ => match value {
            Value::String(_) => plan_item.target_state.as_ref()?.value.clone(),
            _ => None,
        },
    }
}

fn with_snapshot_content(
    item: Option<DiscoveredItem>,
    content: &HashMap<String, SnapshotContentEntry>,
) -> Option<DiscoveredItem> {
    let mut item = item?;
    let Some(entry) = content.get(&item.id) else {
        return Some(item);
    };
    let mut metadata = item.metadata.clone().unwrap_or_else(|| json!({}));
    if let Some(obj) = metadata.as_object_mut() {
        obj.insert("contentCaptureStatus".to_string(), json!("captured"));
        obj.insert(
            "contentRestorePath".to_string(),
            json!(entry.restore_path),
        );
        obj.insert("contentChecksum".to_string(), json!(entry.checksum));
    }
    item.value = entry.content.clone().map(Value::String);
    item.metadata = Some(metadata);
    Some(item)
}

fn restore_path_from_content(item: Option<&DiscoveredItem>) -> Option<String> {
    let restore_path = item?
        .metadata
        .as_ref()?
        .get("contentRestorePath")?
        .as_str()?;
    if restore_path.is_empty() {
        None
    } else {
        Some(restore_path.to_string())
    }
}

fn restore_path_for_evidence_file(item: Option<&DiscoveredItem>) -> Option<String> {
    let item = item?;
    if item.kind == EvidenceKind::Skill {
        let entrypoint = item
            .metadata
            .as_ref()
            .and_then(|m| m.get("entrypoint"))
            .and_then(|v| v.as_str())
            .unwrap_or("SKILL.md");
        return Some(format!("{}/{}", item.source_path, entrypoint));
    }
    if item.source_path.starts_with("~/")
        || item.source_path.starts_with('.')
        || Path::new(&item.source_path).is_absolute()
    {
        return Some(item.source_path.clone());
    }
    None
}

fn agent_for_restore_item(
    current_state: Option<&DiscoveredItem>,
    target_state: Option<&DiscoveredItem>,
) -> AgentId {
    target_state
        .map(|s| s.agent)
        .or_else(|| current_state.map(|s| s.agent))
        .unwrap_or(AgentId::Unknown)
}

fn unsupported_reason_for(
    code: &str,
    kind: EvidenceKind,
    name: &str,
    current_state: Option<&DiscoveredItem>,
    target_state: Option<&DiscoveredItem>,
) -> String {
    if current_state.is_none() && target_state.is_none() {
        return format!("Cannot map {code} for {} {name} to captured evidence", kind.as_str());
    }
    if let Some(target) = target_state {
        if target
            .metadata
            .as_ref()
            .and_then(|m| m.get("contentCaptureStatus"))
            .and_then(|v| v.as_str())
            == Some("omitted")
        {
            let reason = target
                .metadata
                .as_ref()
                .and_then(|m| m.get("contentCaptureReason"))
                .and_then(|v| v.as_str())
                .unwrap_or("policy");
            return format!(
                "Snapshot content for {} {name} was omitted: {reason}",
                kind.as_str()
            );
        }
    }
    if kind == EvidenceKind::EnvKey {
        return format!(
            "Environment key values are key-inventory-only; {code} cannot be restored without a user-supplied value"
        );
    }
    if code == "UNSUPPORTED_STATE_CHANGED" {
        return format!("Unsupported state change: {} {name}", kind.as_str());
    }
    format!("No supported restore action for {code} on {} {name}", kind.as_str())
}

fn can_rollback_action(action: RestoreAction) -> bool {
    matches!(
        action,
        RestoreAction::Create | RestoreAction::Update | RestoreAction::Delete
    )
}

fn build_rollback_steps(items: &[RestorePlanItem]) -> Vec<RollbackStep> {
    items
        .iter()
        .filter(|item| can_rollback_action(item.action))
        .map(|item| RollbackStep {
            item_id: item.item_id.clone(),
            action: rollback_action_for(item.action).to_string(),
            instruction: item.rollback_instruction.clone(),
        })
        .collect()
}

fn rollback_action_for(action: RestoreAction) -> &'static str {
    match action {
        RestoreAction::Create => "delete",
        RestoreAction::Delete => "create",
        _ => "revert",
    }
}

fn restore_action_for_change(
    code: &str,
    current_state: Option<&DiscoveredItem>,
    target_state: Option<&DiscoveredItem>,
) -> RestoreAction {
    match code {
        "AGENT_CONFIG_ADDED" | "SKILL_ADDED" | "HOOK_ADDED" => {
            if current_state.is_some() {
                RestoreAction::Delete
            } else {
                RestoreAction::Unsupported
            }
        }
        "MCP_ADDED" => {
            if current_state.is_some_and(is_json_mcp_state) {
                RestoreAction::Delete
            } else {
                RestoreAction::Unsupported
            }
        }
        "AGENT_CONFIG_REMOVED" | "SKILL_REMOVED" | "HOOK_REMOVED" => {
            if target_state.is_some() {
                RestoreAction::Create
            } else {
                RestoreAction::Unsupported
            }
        }
        "MCP_REMOVED" => {
            if target_state.is_some_and(is_json_mcp_state) {
                RestoreAction::Create
            } else {
                RestoreAction::Unsupported
            }
        }
        "AGENT_CONFIG_CHANGED"
        | "HOOK_CHANGED"
        | "PERMISSION_CHANGED"
        | "INSTRUCTION_CHANGED"
        | "SKILL_EXECUTABLE_APPEARED" => {
            if target_state.is_some() {
                RestoreAction::Update
            } else {
                RestoreAction::Unsupported
            }
        }
        "MCP_CHANGED" => {
            if target_state.is_some_and(is_json_mcp_state) {
                RestoreAction::Update
            } else {
                RestoreAction::Unsupported
            }
        }
        "ENV_KEY_ADDED" => {
            if current_state.is_some() {
                RestoreAction::Delete
            } else {
                RestoreAction::Unsupported
            }
        }
        "ENV_KEY_REMOVED" | "UNSUPPORTED_STATE_CHANGED" => RestoreAction::Unsupported,
        _ => RestoreAction::Unsupported,
    }
}

fn is_json_mcp_state(item: &DiscoveredItem) -> bool {
    item.source_path.ends_with(".mcp.json") || item.source_path.ends_with("/mcp.json")
}

fn rollback_instruction_for(action: RestoreAction, kind: EvidenceKind, name: &str) -> String {
    match action {
        RestoreAction::Delete => format!("Recreate deleted {}: {name}", kind.as_str()),
        RestoreAction::Create => format!("Remove created {}: {name}", kind.as_str()),
        _ => format!("Reverse {action:?} for {}: {name}", kind.as_str()),
    }
}

fn source_path_for_restore_item(
    kind: EvidenceKind,
    name: &str,
    current_state: Option<&DiscoveredItem>,
    target_state: Option<&DiscoveredItem>,
    diff_source_path: Option<&str>,
) -> String {
    target_state
        .map(|s| s.source_path.clone())
        .or_else(|| current_state.map(|s| s.source_path.clone()))
        .or_else(|| diff_source_path.map(str::to_string))
        .unwrap_or_else(|| resolve_source_path_by_kind(kind, name))
}

fn is_virtual_source_path(source_path: &str) -> bool {
    regex::Regex::new(r"^[a-z_]+:")
        .expect("regex")
        .is_match(source_path)
}

fn resolve_plan_destination(
    plan_item: &RestorePlanItem,
    target_project: Option<&str>,
    target_home: Option<&str>,
) -> String {
    if let Some(target_home) = target_home {
        if plan_item.source_path == "~" {
            return target_home.to_string();
        }
        if let Some(rest) = plan_item.source_path.strip_prefix("~/") {
            return Path::new(target_home).join(rest).to_string_lossy().to_string();
        }
    }
    if target_project.is_none()
        || Path::new(&plan_item.source_path).is_absolute()
        || is_virtual_source_path(&plan_item.source_path)
    {
        return plan_item.source_path.clone();
    }
    Path::new(target_project.unwrap())
        .join(&plan_item.source_path)
        .to_string_lossy()
        .to_string()
}

fn restore_item_metadata(plan_item: &RestorePlanItem) -> Option<Value> {
    let mut metadata = Map::new();
    if let Some(restore_path) = restore_path_from_content(plan_item.target_state.as_ref())
        .or_else(|| restore_path_from_content(plan_item.current_state.as_ref()))
    {
        metadata.insert("restorePath".to_string(), json!(restore_path));
    }
    if plan_item.kind == EvidenceKind::McpServer {
        let server_name = plan_item
            .target_state
            .as_ref()
            .and_then(|s| s.name.clone())
            .or_else(|| plan_item.current_state.as_ref().and_then(|s| s.name.clone()));
        if let Some(server_name) = server_name {
            metadata.insert("serverName".to_string(), json!(server_name));
        }
        metadata.insert("sourcePath".to_string(), json!(plan_item.source_path));
        if plan_item.source_path.ends_with(".mcp.json")
            || plan_item.source_path.ends_with("/mcp.json")
        {
            metadata.insert("mcpPath".to_string(), json!(plan_item.source_path));
        }
    }
    if plan_item.kind == EvidenceKind::Permission {
        if let Some(permission_name) = permission_name_from_state(plan_item.target_state.as_ref())
            .or_else(|| permission_name_from_state(plan_item.current_state.as_ref()))
        {
            metadata.insert("permissionName".to_string(), json!(permission_name));
        }
    }
    if metadata.is_empty() {
        None
    } else {
        Some(Value::Object(metadata))
    }
}

fn permission_name_from_state(state: Option<&DiscoveredItem>) -> Option<String> {
    let state = state?;
    if let Some(key) = state
        .metadata
        .as_ref()
        .and_then(|metadata| metadata.get("permissionKey"))
        .and_then(|value| value.as_str())
    {
        return Some(key.to_string());
    }
    permission_name_from_evidence_id(&state.id)
}

fn permission_name_from_evidence_id(id: &str) -> Option<String> {
    let marker = ".perm-";
    let (_, suffix) = id.rsplit_once(marker)?;
    let name = suffix.split(':').next().unwrap_or(suffix);
    if name.is_empty() {
        None
    } else {
        Some(name.to_string())
    }
}

fn resolve_source_path_by_kind(kind: EvidenceKind, name: &str) -> String {
    match kind {
        EvidenceKind::McpServer => format!(".mcp.json ({name})"),
        EvidenceKind::EnvKey => format!("env:{name}"),
        EvidenceKind::AgentConfig => format!("config:{name}"),
        EvidenceKind::AgentInstruction => format!("instruction:{name}"),
        EvidenceKind::Skill => format!("skill:{name}"),
        EvidenceKind::Permission => format!("permission:{name}"),
        EvidenceKind::Hook => format!("hook:{name}"),
        _ => format!("unknown:{name}"),
    }
}

fn find_matching_evidence(
    change: &SemanticChange,
    evidence: &[DiscoveredItem],
    source_path: Option<&str>,
) -> Option<DiscoveredItem> {
    if let Some(source_path) = source_path {
        for item in evidence {
            if item.kind == change.entity_kind
                && item.name.as_deref() == Some(&change.entity_name)
                && item.source_path == source_path
            {
                return Some(item.clone());
            }
        }
        for item in evidence {
            if item.kind == change.entity_kind && item.source_path == source_path {
                return Some(item.clone());
            }
        }
    }
    for item in evidence {
        if item.kind == change.entity_kind && item.name.as_deref() == Some(&change.entity_name) {
            return Some(item.clone());
        }
    }
    for item in evidence {
        if item.kind == change.entity_kind {
            return Some(item.clone());
        }
    }
    None
}

fn increment_risk(summary: &mut RiskSummary, severity: Severity) {
    match severity {
        Severity::None => summary.none += 1,
        Severity::Low => summary.low += 1,
        Severity::Medium => summary.medium += 1,
        Severity::High => summary.high += 1,
        Severity::Critical => summary.critical += 1,
    }
}