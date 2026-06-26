use std::collections::HashSet;
use std::path::Path;

use time::format_description::well_known::Rfc3339;
use time::OffsetDateTime;
use uuid::Uuid;

use crate::diff::{diff_graphs, GraphDiff, SemanticChange, SemanticChangeCode};
use crate::snapshot::capture_current_state;
use crate::store::{
    append_timeline_entry, latest_timeline_entry, read_snapshot, write_snapshot, StoreError,
    StoreSnapshot,
};
use crate::types::{
    AgentId, CurrentState, EvidenceKind, RuntimeOptions, TimelineChangeSummary,
    TimelineChangedSurface, TimelineConfidence, TimelineEntry, TimelineEntryEventKind,
    TimelineEntrySource, TimelineRestoreReadiness,
};

#[derive(Debug, thiserror::Error)]
pub enum TimelineError {
    #[error("Timeline entry not found: {0}")]
    NotFound(String),
    #[error(transparent)]
    Store(#[from] StoreError),
    #[error(transparent)]
    Io(#[from] std::io::Error),
}

#[derive(Debug, Clone, Default)]
pub struct CaptureTimelineOptions {
    pub capture_id: Option<String>,
    pub snapshot_name: Option<String>,
    pub title: Option<String>,
    pub skip_unchanged: bool,
}

#[derive(Debug, Clone)]
pub struct CaptureTimelineResult {
    pub written: bool,
    pub entry: Option<TimelineEntry>,
    pub state: CurrentState,
    pub diff: Option<GraphDiff>,
    pub skipped_reason: Option<String>,
}

pub fn capture_timeline_snapshot(
    options: &RuntimeOptions,
    capture_options: &CaptureTimelineOptions,
) -> Result<CaptureTimelineResult, TimelineError> {
    let store_dir = Path::new(&options.store_dir);
    let previous = latest_timeline_entry(
        store_dir,
        crate::store::TimelineListOptions {
            agent: options.agent,
            project_path: Some(&options.project_path),
            limit: None,
            on_corrupt_entry: None,
        },
    )?;

    let capture_id = capture_options
        .capture_id
        .clone()
        .unwrap_or_else(short_id);
    let snapshot_name = capture_options
        .snapshot_name
        .clone()
        .unwrap_or_else(|| timeline_snapshot_name(&capture_id, options.agent));
    let state = capture_current_state(options, &snapshot_name)?;

    let mut diff: Option<GraphDiff> = None;
    let mut diff_error: Option<String> = None;
    if let Some(previous) = &previous {
        match read_snapshot(store_dir, &previous.after_snapshot_name, previous.agent) {
            Ok(previous_snapshot) => {
                diff = Some(diff_graphs(
                    &previous_snapshot.graph,
                    &state.snapshot.graph,
                ));
            }
            Err(error) => {
                diff_error = Some(error.to_string());
            }
        }
    }

    if let (Some(_), Some(ref diff)) = (&previous, &diff) {
        if capture_options.skip_unchanged
            && diff.semantic_changes.is_empty()
            && diff.raw_source_changes.is_empty()
        {
            return Ok(CaptureTimelineResult {
                written: false,
                entry: None,
                state,
                diff: Some(diff.clone()),
                skipped_reason: Some("unchanged".to_string()),
            });
        }
    }

    let changed_surfaces = changed_surfaces_for_diff(diff.as_ref());
    let restore_readiness = restore_readiness_for(&changed_surfaces);
    let entry = TimelineEntry {
        schema_version: "0.1".to_string(),
        id: short_id(),
        source: TimelineEntrySource::Manual,
        event_kind: event_kind_for(previous.as_ref(), diff.as_ref()),
        title: capture_options
            .title
            .clone()
            .unwrap_or_else(|| title_for_diff(diff.as_ref(), options.agent)),
        project_path: state.snapshot.manifest.project_path.clone(),
        agent: options.agent,
        agents: agents_for_state(&state),
        before_snapshot_name: previous.as_ref().map(|p| p.after_snapshot_name.clone()),
        after_snapshot_name: snapshot_name.clone(),
        capture_id: capture_id.clone(),
        created_at: OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_string()),
        observed_at: state.snapshot.manifest.created_at.clone(),
        changed_surfaces: changed_surfaces.clone(),
        restore_readiness,
        confidence: confidence_for(diff.as_ref(), &changed_surfaces, diff_error.as_deref()),
        confidence_reason: confidence_reason_for(diff.as_ref(), &changed_surfaces, diff_error.as_deref()),
        evidence_count: state.snapshot.evidence.len() as u32,
        graph_node_count: state.snapshot.graph.len() as u32,
        audit_finding_count: state.snapshot.audit_findings.len() as u32,
        changes: TimelineChangeSummary {
            previous_entry_id: previous.as_ref().map(|p| p.id.clone()),
            previous_snapshot_name: previous.as_ref().map(|p| p.after_snapshot_name.clone()),
            has_changes: diff.as_ref().map_or(true, |d| {
                !d.semantic_changes.is_empty() || !d.raw_source_changes.is_empty()
            }),
            semantic_change_count: diff.as_ref().map_or(0, |d| d.semantic_changes.len() as u32),
            raw_source_change_count: diff.as_ref().map_or(0, |d| d.raw_source_changes.len() as u32),
            highlights: highlights_for_diff(diff.as_ref()),
        },
    };

    write_snapshot(store_dir, StoreSnapshot::from(state.snapshot.clone()), options.agent)?;
    append_timeline_entry(store_dir, &entry)?;

    Ok(CaptureTimelineResult {
        written: true,
        entry: Some(entry),
        state,
        diff,
        skipped_reason: None,
    })
}

pub fn timeline_snapshot_name(capture_id: &str, agent: Option<AgentId>) -> String {
    let timestamp = OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_string())
        .replace([':', '.'], "-");
    [
        Some("history".to_string()),
        Some(capture_id.to_string()),
        agent.map(|a| a.as_str().to_string()),
        Some(timestamp),
        Some(short_id()),
    ]
    .into_iter()
    .flatten()
    .collect::<Vec<_>>()
    .join("-")
}

fn short_id() -> String {
    Uuid::new_v4().to_string().replace('-', "")[..8].to_string()
}

fn agents_for_state(state: &CurrentState) -> Vec<AgentId> {
    let mut agents: Vec<AgentId> = state
        .snapshot
        .evidence
        .iter()
        .map(|item| item.agent)
        .collect::<HashSet<_>>()
        .into_iter()
        .collect();
    agents.sort_by_key(|a| a.as_str().to_string());
    agents
}

fn title_for_diff(diff: Option<&GraphDiff>, agent: Option<AgentId>) -> String {
    let Some(diff) = diff else {
        return scoped_title("baseline setup", agent);
    };

    if diff.semantic_changes.is_empty() && diff.raw_source_changes.is_empty() {
        return scoped_title("unchanged setup", agent);
    }

    let mut sorted = diff.semantic_changes.clone();
    sorted.sort_by(|left, right| {
        priority_for_change(left)
            .cmp(&priority_for_change(right))
    });

    let Some(first) = sorted.first() else {
        return scoped_title("update setup files", agent);
    };

    let title = match first.code {
        SemanticChangeCode::AgentConfigAdded
        | SemanticChangeCode::AgentConfigRemoved
        | SemanticChangeCode::AgentConfigChanged => "update config",
        SemanticChangeCode::McpAdded => return scoped_title(&format!("add {} mcp", first.entity_name), agent),
        SemanticChangeCode::McpRemoved => {
            return scoped_title(&format!("remove {} mcp", first.entity_name), agent);
        }
        SemanticChangeCode::McpChanged => {
            return scoped_title(&format!("update {} mcp", first.entity_name), agent);
        }
        SemanticChangeCode::EnvKeyAdded => {
            return scoped_title(&format!("add {} env key", first.entity_name), agent);
        }
        SemanticChangeCode::EnvKeyRemoved => {
            return scoped_title(&format!("remove {} env key", first.entity_name), agent);
        }
        SemanticChangeCode::PermissionWildcardAdded | SemanticChangeCode::PermissionChanged => {
            "update permissions"
        }
        SemanticChangeCode::SkillAdded => {
            return scoped_title(&format!("install {} skill", first.entity_name), agent);
        }
        SemanticChangeCode::SkillRemoved => {
            return scoped_title(&format!("remove {} skill", first.entity_name), agent);
        }
        SemanticChangeCode::SkillExecutableAppeared => {
            return scoped_title(&format!("update {} skill", first.entity_name), agent);
        }
        SemanticChangeCode::HookAdded
        | SemanticChangeCode::HookRemoved
        | SemanticChangeCode::HookChanged => "update hooks",
        SemanticChangeCode::InstructionChanged => "update project instructions",
        SemanticChangeCode::UnsupportedStateChanged => "update unsupported setup",
    };

    scoped_title(title, agent)
}

fn changed_surfaces_for_diff(diff: Option<&GraphDiff>) -> Vec<TimelineChangedSurface> {
    let Some(diff) = diff else {
        return Vec::new();
    };

    let mut surfaces = Vec::new();
    let mut semantic_source_paths = HashSet::new();

    for change in &diff.semantic_changes {
        let kind = timeline_surface_kind(change);
        let path = change
            .details
            .source_path
            .clone()
            .unwrap_or_else(|| "unknown".to_string());
        semantic_source_paths.insert(path.clone());
        let restorable = kind == "mcp_server" && path.ends_with(".mcp.json");
        surfaces.push(TimelineChangedSurface {
            kind,
            change_type: change.code.as_str().to_string(),
            path,
            entity_name: Some(change.entity_name.clone()),
            restorable,
            observe_only: !restorable,
            before: change.before.clone(),
            after: change.after.clone(),
        });
    }

    for change in &diff.raw_source_changes {
        if semantic_source_paths.contains(&change.source_path) {
            continue;
        }
        surfaces.push(TimelineChangedSurface {
            kind: "other".to_string(),
            change_type: format!("RAW_{}", change.status.to_uppercase()),
            path: change.source_path.clone(),
            entity_name: None,
            restorable: false,
            observe_only: true,
            before: None,
            after: None,
        });
    }

    surfaces
}

fn event_kind_for(
    previous: Option<&TimelineEntry>,
    diff: Option<&GraphDiff>,
) -> TimelineEntryEventKind {
    if previous.is_none() {
        return TimelineEntryEventKind::Baseline;
    }
    if let Some(diff) = diff {
        if diff.semantic_changes.is_empty() && diff.raw_source_changes.is_empty() {
            return TimelineEntryEventKind::Unchanged;
        }
    }
    TimelineEntryEventKind::SetupChanged
}

fn timeline_surface_kind(change: &SemanticChange) -> String {
    match change.entity_kind {
        EvidenceKind::McpServer => "mcp_server".to_string(),
        EvidenceKind::Skill => "skill".to_string(),
        EvidenceKind::Permission => "permission".to_string(),
        EvidenceKind::Hook => "hook".to_string(),
        EvidenceKind::EnvKey => "env_key".to_string(),
        EvidenceKind::Unsupported => "unsupported".to_string(),
        _ => "other".to_string(),
    }
}

fn restore_readiness_for(surfaces: &[TimelineChangedSurface]) -> TimelineRestoreReadiness {
    if surfaces.is_empty() {
        return TimelineRestoreReadiness::ObserveOnly;
    }
    let restorable = surfaces.iter().filter(|surface| surface.restorable).count();
    if restorable == surfaces.len() {
        TimelineRestoreReadiness::Full
    } else if restorable > 0 {
        TimelineRestoreReadiness::Partial
    } else {
        TimelineRestoreReadiness::ObserveOnly
    }
}

fn confidence_for(
    diff: Option<&GraphDiff>,
    surfaces: &[TimelineChangedSurface],
    diff_error: Option<&str>,
) -> TimelineConfidence {
    if diff_error.is_some() {
        return TimelineConfidence::Low;
    }
    if diff.is_none() {
        return TimelineConfidence::High;
    }
    if surfaces.iter().any(|surface| surface.path == "unknown") {
        return TimelineConfidence::Medium;
    }
    TimelineConfidence::High
}

fn confidence_reason_for(
    diff: Option<&GraphDiff>,
    surfaces: &[TimelineChangedSurface],
    diff_error: Option<&str>,
) -> String {
    if let Some(error) = diff_error {
        return format!("previous snapshot could not be diffed: {error}");
    }
    if diff.is_none() {
        return "first manual history baseline".to_string();
    }
    if surfaces.is_empty() {
        return "no semantic or raw source changes".to_string();
    }
    if surfaces.iter().any(|surface| surface.path == "unknown") {
        return "some changes lacked source path metadata".to_string();
    }
    "derived from snapshot graph diff".to_string()
}

fn priority_for_change(change: &SemanticChange) -> String {
    let code = change.code.as_str();
    if code.starts_with("MCP_") {
        return format!("0-{}", change.entity_name);
    }
    if code == "SKILL_EXECUTABLE_APPEARED" {
        return format!("1-{}", change.entity_name);
    }
    if code == "PERMISSION_WILDCARD_ADDED" {
        return format!("2-{}", change.entity_name);
    }
    if code.starts_with("ENV_KEY_") {
        return format!("4-{}", change.entity_name);
    }
    format!("5-{}", change.entity_name)
}

fn scoped_title(title: &str, agent: Option<AgentId>) -> String {
    match agent {
        Some(agent) => format!("{title} for {}", agent_label(agent)),
        None => title.to_string(),
    }
}

fn agent_label(agent: AgentId) -> &'static str {
    match agent {
        AgentId::ClaudeCode => "Claude Code",
        AgentId::Codex => "Codex",
        AgentId::Cursor => "Cursor",
        AgentId::Opencode => "OpenCode",
        AgentId::PiAgent => "Pi Agent",
        AgentId::Project => "Project",
        AgentId::Unknown => "Unknown",
    }
}

fn highlights_for_diff(diff: Option<&GraphDiff>) -> Vec<String> {
    let Some(diff) = diff else {
        return Vec::new();
    };

    let mut highlights: Vec<String> = diff
        .semantic_changes
        .iter()
        .take(5)
        .map(|change| format!("{}: {}", change.code.as_str(), change.entity_name))
        .collect();

    let remaining = 8usize.saturating_sub(highlights.len());
    highlights.extend(
        diff.raw_source_changes
            .iter()
            .take(remaining)
            .map(|change| format!("{}: {}", change.status, change.source_path)),
    );
    highlights
}