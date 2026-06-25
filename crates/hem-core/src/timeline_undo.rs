use std::path::Path;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::store::{find_timeline_entry, TimelineListOptions};
use crate::timeline::TimelineError;
use crate::types::{TimelineChangedSurface, TimelineRestoreReadiness};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum TimelineUndoAction {
    Add,
    Remove,
    Update,
}

impl TimelineUndoAction {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Add => "add",
            Self::Remove => "remove",
            Self::Update => "update",
        }
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TimelineUndoItem {
    pub action: TimelineUndoAction,
    pub kind: String,
    pub path: String,
    pub server_name: String,
    pub target_value: Option<Value>,
    pub current_value: Option<Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TimelineUndoPlan {
    pub entry_id: String,
    pub title: String,
    pub dry_run: bool,
    pub writes_files: bool,
    pub restore_readiness: TimelineRestoreReadiness,
    pub target_snapshot_name: Option<String>,
    pub current_snapshot_name: String,
    pub writable_items: Vec<TimelineUndoItem>,
    pub observe_only_surfaces: Vec<TimelineChangedSurface>,
}

pub struct BuildTimelineUndoOptions<'a> {
    pub on_corrupt_entry: Option<&'a mut dyn FnMut(crate::store::TimelineCorruptEvent)>,
}

impl<'a> Default for BuildTimelineUndoOptions<'a> {
    fn default() -> Self {
        Self {
            on_corrupt_entry: None,
        }
    }
}

pub fn build_timeline_undo_plan(
    store_dir: &Path,
    reference: &str,
    options: BuildTimelineUndoOptions<'_>,
) -> Result<TimelineUndoPlan, TimelineError> {
    let entry = find_timeline_entry(
        store_dir,
        reference,
        TimelineListOptions {
            agent: None,
            project_path: None,
            limit: None,
            on_corrupt_entry: options.on_corrupt_entry,
        },
    )?
    .ok_or_else(|| TimelineError::NotFound(reference.to_string()))?;

    let writable_items = entry
        .changed_surfaces
        .iter()
        .filter(|surface| surface.restorable && surface.kind == "mcp_server")
        .map(undo_item_for_mcp_surface)
        .collect();

    let observe_only_surfaces = entry
        .changed_surfaces
        .iter()
        .filter(|surface| !surface.restorable)
        .cloned()
        .collect();

    Ok(TimelineUndoPlan {
        entry_id: entry.id.clone(),
        title: format!("dry-run MCP undo: {}", entry.title),
        dry_run: true,
        writes_files: false,
        restore_readiness: entry.restore_readiness,
        target_snapshot_name: entry.before_snapshot_name.clone(),
        current_snapshot_name: entry.after_snapshot_name.clone(),
        writable_items,
        observe_only_surfaces,
    })
}

fn undo_item_for_mcp_surface(surface: &TimelineChangedSurface) -> TimelineUndoItem {
    let server_name = surface
        .entity_name
        .clone()
        .unwrap_or_else(|| "unknown".to_string());

    if surface.change_type == "MCP_ADDED" {
        return TimelineUndoItem {
            action: TimelineUndoAction::Remove,
            kind: "mcp_server".to_string(),
            path: surface.path.clone(),
            server_name,
            target_value: None,
            current_value: surface.after.clone(),
        };
    }

    if surface.change_type == "MCP_REMOVED" {
        return TimelineUndoItem {
            action: TimelineUndoAction::Add,
            kind: "mcp_server".to_string(),
            path: surface.path.clone(),
            server_name,
            target_value: surface.before.clone(),
            current_value: None,
        };
    }

    TimelineUndoItem {
        action: TimelineUndoAction::Update,
        kind: "mcp_server".to_string(),
        path: surface.path.clone(),
        server_name,
        target_value: surface.before.clone(),
        current_value: surface.after.clone(),
    }
}