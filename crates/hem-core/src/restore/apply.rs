use std::collections::HashMap;
use std::fs;
use std::io;
use std::path::Path;

use serde_json::{json, Value};
use time::format_description::well_known::Rfc3339;
use time::OffsetDateTime;
use uuid::Uuid;

use crate::types::{
    ApplyFailure, ApplyOptions, ApplySummary, ApplyWithRollbackResult, RestoreAction,
    RestoreItem, RestoreItemStatus, RollbackSummary, UndoResult, UndoStatus,
};

pub type RestoreExecutor = Box<dyn FnMut(&mut RestoreItem) -> Result<(), String>>;
pub type UndoExecutor = Box<dyn FnMut(&mut RestoreItem) -> Result<(), String>>;
pub type ApplyHandler = Box<dyn FnMut(&mut RestoreItem) -> Result<(), String>>;
pub type UndoHandler = Box<dyn FnMut(&mut RestoreItem) -> Result<(), String>>;

pub struct ApplyHandlerRegistry {
    handlers: HashMap<String, ApplyHandler>,
}

pub struct UndoHandlerRegistry {
    handlers: HashMap<String, UndoHandler>,
}

pub fn apply_restore_items(
    items: &mut [RestoreItem],
    executor: &mut RestoreExecutor,
    options: &ApplyOptions,
) -> ApplySummary {
    let mut summary = ApplySummary {
        total: items.len() as u32,
        successful: 0,
        failed: 0,
        skipped: 0,
        unsupported: 0,
        failures: Vec::new(),
        applied_items: Vec::new(),
        status_registry: HashMap::new(),
    };

    let mut sorted: Vec<usize> = (0..items.len()).collect();
    sorted.sort_by_key(|&index| items[index].execution_order);

    let mut stopped_early = false;

    for index in sorted {
        let item = &mut items[index];
        if item.status == RestoreItemStatus::Unsupported {
            summary.status_registry.insert(item.item_id.clone(), item.status);
            summary.unsupported += 1;
            continue;
        }
        if item.status == RestoreItemStatus::Skipped {
            summary.status_registry.insert(item.item_id.clone(), item.status);
            summary.skipped += 1;
            continue;
        }

        match executor(item) {
            Ok(()) => {
                item.status = RestoreItemStatus::Applied;
                item.apply_at = Some(
                    OffsetDateTime::now_utc()
                        .format(&Rfc3339)
                        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_string()),
                );
                summary.status_registry.insert(item.item_id.clone(), item.status);
                summary.successful += 1;
                summary.applied_items.push(item.clone());
            }
            Err(error_message) => {
                item.status = RestoreItemStatus::Failed;
                item.error_message = Some(error_message.clone());
                summary.status_registry.insert(item.item_id.clone(), item.status);
                summary.failed += 1;
                summary.failures.push(ApplyFailure {
                    item_id: item.item_id.clone(),
                    reason: error_message,
                });
                if options.fail_fast {
                    stopped_early = true;
                    break;
                }
            }
        }
    }

    if stopped_early {
        for item in items.iter_mut() {
            if item.status == RestoreItemStatus::Pending {
                item.status = RestoreItemStatus::Skipped;
                item.skip_reason = Some("Execution stopped before this item".to_string());
                summary.status_registry.insert(item.item_id.clone(), item.status);
                summary.skipped += 1;
            } else if item.status == RestoreItemStatus::Unsupported
                && !summary.status_registry.contains_key(&item.item_id)
            {
                summary.unsupported += 1;
                summary.status_registry.insert(item.item_id.clone(), item.status);
            }
        }
        summary.total = items.len() as u32;
    }

    summary
}

pub fn format_apply_summary(summary: &ApplySummary) -> String {
    let mut lines = vec![
        "Restore apply results".to_string(),
        String::new(),
        format!("  Successful: {}", summary.successful),
        format!("  Failed:     {}", summary.failed),
        format!("  Skipped:    {}", summary.skipped),
        format!("  Unsupported: {}", summary.unsupported),
        format!("  Total:      {}", summary.total),
    ];
    if !summary.failures.is_empty() {
        lines.push(String::new());
        lines.push("Failures:".to_string());
        for failure in &summary.failures {
            lines.push(format!("  [{}] {}", failure.item_id, failure.reason));
        }
    }
    format!("{}\n", lines.join("\n"))
}

pub fn write_file_atomically(file_path: &Path, content: &str) -> io::Result<()> {
    let temp_path_string = format!(
        "{}.{}.{}.tmp",
        file_path.display(),
        std::process::id(),
        Uuid::new_v4()
    );
    let temp_path = Path::new(&temp_path_string);
    if let Err(error) = (|| {
        fs::write(&temp_path, content)?;
        fs::rename(&temp_path, file_path)?;
        Ok::<(), io::Error>(())
    })() {
        let _ = fs::remove_file(&temp_path);
        return Err(error);
    }
    Ok(())
}

fn read_current_content(file_path: &Path) -> Option<String> {
    fs::read_to_string(file_path).ok()
}

fn apply_file_mutation(
    item: &mut RestoreItem,
    file_path: &Path,
    content: Option<&str>,
    mode: Option<u32>,
    force_write: bool,
) -> Result<(), String> {
    let prev = read_current_content(file_path);
    item.rollback_state = Some(json!({
        "filePath": file_path.to_string_lossy(),
        "previousContent": prev
    }));
    if item.action == Some(RestoreAction::Delete) && !force_write {
        fs::remove_file(file_path).or_else(|error| {
            if error.kind() == io::ErrorKind::NotFound {
                Ok(())
            } else {
                Err(error)
            }
        })
        .map_err(|e| e.to_string())?;
        return Ok(());
    }
    let Some(content) = content else {
        return Err(format!("Missing target content for {}", item.item_id));
    };
    if let Some(parent) = file_path.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }
    write_file_atomically(file_path, content).map_err(|e| e.to_string())?;
    #[cfg(unix)]
    if let Some(mode) = mode {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(file_path, fs::Permissions::from_mode(mode)).map_err(|e| e.to_string())?;
    }
    let _ = mode;
    Ok(())
}

pub fn apply_agent_config(item: &mut RestoreItem) -> Result<(), String> {
    let content = match &item.target_content {
        Some(Value::String(text)) => text.clone(),
        Some(_) => {
            return Err(format!(
                "Refusing to apply parsed metadata as file content for {} — snapshot needs content-backed capture",
                item.item_id
            ));
        }
        None => return Err(format!("Missing target content for {}", item.item_id)),
    };
    let dest = item.dest.clone();
    apply_file_mutation(item, Path::new(&dest), Some(&content), None, false)
}

pub fn apply_agent_instruction(item: &mut RestoreItem) -> Result<(), String> {
    let content = item
        .target_content
        .as_ref()
        .and_then(|v| v.as_str())
        .map(str::to_string)
        .unwrap_or_default();
    let dest = item.dest.clone();
    apply_file_mutation(item, Path::new(&dest), Some(&content), None, false)
}

pub fn apply_hook(item: &mut RestoreItem) -> Result<(), String> {
    let content = item
        .target_content
        .as_ref()
        .and_then(|v| v.as_str())
        .map(str::to_string)
        .unwrap_or_default();
    let dest = item.dest.clone();
    apply_file_mutation(item, Path::new(&dest), Some(&content), Some(0o755), false)
}

pub fn apply_skill(item: &mut RestoreItem) -> Result<(), String> {
    let content = match &item.target_content {
        Some(Value::String(text)) => text.clone(),
        Some(value) => serde_json::to_string_pretty(value).map_err(|e| e.to_string())?,
        None => String::new(),
    };
    let dest = item.dest.clone();
    apply_file_mutation(item, Path::new(&dest), Some(&content), None, false)
}

pub fn default_apply_handler_registry() -> ApplyHandlerRegistry {
    let mut handlers: HashMap<String, ApplyHandler> = HashMap::new();
    handlers.insert(
        "agent_config".to_string(),
        Box::new(|item| apply_agent_config(item)),
    );
    handlers.insert(
        "agent_instruction".to_string(),
        Box::new(|item| apply_agent_instruction(item)),
    );
    handlers.insert("hook".to_string(), Box::new(|item| apply_hook(item)));
    handlers.insert("skill".to_string(), Box::new(|item| apply_skill(item)));
    ApplyHandlerRegistry { handlers }
}

pub fn dispatch_default_apply(item: &mut RestoreItem) -> Result<(), String> {
    let mut registry = default_apply_handler_registry();
    if let Some(handler) = registry.handlers.get_mut(&item.item_type) {
        return handler(item);
    }
    let message = format!("No apply handler for type \"{}\"", item.item_type);
    item.skip_reason = Some(message.clone());
    Err(message)
}

pub fn create_default_apply_executor() -> RestoreExecutor {
    Box::new(|item: &mut RestoreItem| dispatch_default_apply(item))
}

pub fn noop_undo_handler(_item: &mut RestoreItem) -> Result<(), String> {
    Ok(())
}

pub fn restore_previous_content_undo_handler(item: &mut RestoreItem) -> Result<(), String> {
    let Some(state) = item.rollback_state.as_ref().and_then(|v| v.as_object()) else {
        return Ok(());
    };
    let prev_content = state.get("previousContent").cloned();
    if let Some(file_path) = state.get("filePath").and_then(|v| v.as_str()) {
        let file_path = Path::new(file_path);
        if prev_content.is_none() || prev_content == Some(Value::Null) {
            let _ = fs::remove_file(file_path);
        } else if let Some(Value::String(content)) = prev_content {
            if let Some(parent) = file_path.parent() {
                fs::create_dir_all(parent).map_err(|e| e.to_string())?;
            }
            write_file_atomically(file_path, &content).map_err(|e| e.to_string())?;
        }
    }
    Ok(())
}

pub fn default_undo_handler_registry() -> UndoHandlerRegistry {
    let mut handlers: HashMap<String, UndoHandler> = HashMap::new();
    for kind in [
        "agent_config",
        "agent_instruction",
        "mcp_server",
        "permission",
        "hook",
        "skill",
        "env_key",
        "env",
        "symlink",
    ] {
        handlers.insert(
            kind.to_string(),
            Box::new(|item| restore_previous_content_undo_handler(item)),
        );
    }
    handlers.insert(
        "unsupported".to_string(),
        Box::new(|item| noop_undo_handler(item)),
    );
    UndoHandlerRegistry { handlers }
}

pub fn dispatch_default_undo(item: &mut RestoreItem) -> Result<(), String> {
    if !item.can_rollback || item.item_type == "unsupported" {
        return Ok(());
    }
    let mut registry = default_undo_handler_registry();
    if let Some(handler) = registry.handlers.get_mut(&item.item_type) {
        return handler(item);
    }
    noop_undo_handler(item)
}

pub fn create_default_undo_executor() -> UndoExecutor {
    Box::new(|item: &mut RestoreItem| dispatch_default_undo(item))
}

pub fn rollback_applied_items(
    items: &mut [RestoreItem],
    undo_executor: &mut UndoExecutor,
    fail_fast: bool,
) -> RollbackSummary {
    let mut summary = RollbackSummary {
        total: 0,
        undone: 0,
        skipped: 0,
        failed: 0,
        results: Vec::new(),
    };

    let mut reversed: Vec<usize> = (0..items.len())
        .filter(|&index| items[index].status == RestoreItemStatus::Applied)
        .collect();
    reversed.sort_by_key(|&index| std::cmp::Reverse(items[index].execution_order));
    summary.total = reversed.len() as u32;

    let mut stopped = false;
    for index in reversed {
        let item = &mut items[index];
        if !item.can_rollback {
            summary.skipped += 1;
            summary.results.push(UndoResult {
                item_id: item.item_id.clone(),
                status: UndoStatus::Skipped,
                reason: Some("Item does not support rollback".to_string()),
            });
            continue;
        }
        match undo_executor(item) {
            Ok(()) => {
                item.status = RestoreItemStatus::Pending;
                item.rollback_state = None;
                summary.undone += 1;
                summary.results.push(UndoResult {
                    item_id: item.item_id.clone(),
                    status: UndoStatus::Undone,
                    reason: None,
                });
            }
            Err(reason) => {
                item.error_message = Some(format!("Rollback failed: {reason}"));
                summary.failed += 1;
                summary.results.push(UndoResult {
                    item_id: item.item_id.clone(),
                    status: UndoStatus::Failed,
                    reason: Some(reason),
                });
                if fail_fast {
                    stopped = true;
                    break;
                }
            }
        }
    }

    if stopped {
        for item in items.iter_mut() {
            if item.status == RestoreItemStatus::Applied {
                summary.skipped += 1;
                summary.results.push(UndoResult {
                    item_id: item.item_id.clone(),
                    status: UndoStatus::Skipped,
                    reason: Some("Rollback stopped before this item".to_string()),
                });
            }
        }
    }

    summary
}

pub fn sort_by_descending_order(items: &[RestoreItem]) -> Vec<RestoreItem> {
    let mut sorted = items.to_vec();
    sorted.sort_by_key(|b| std::cmp::Reverse(b.execution_order));
    sorted
}

pub fn get_applied_items(items: &[RestoreItem]) -> Vec<RestoreItem> {
    let mut applied: Vec<RestoreItem> = items
        .iter()
        .filter(|item| item.status == RestoreItemStatus::Applied)
        .cloned()
        .collect();
    applied.sort_by_key(|item| item.execution_order);
    applied
}

pub fn get_successful_items(
    items: &[RestoreItem],
    status_registry: &HashMap<String, RestoreItemStatus>,
) -> Vec<RestoreItem> {
    let mut successful: Vec<RestoreItem> = items
        .iter()
        .filter(|item| {
            status_registry.get(&item.item_id) == Some(&RestoreItemStatus::Applied)
        })
        .cloned()
        .collect();
    successful.sort_by_key(|item| item.execution_order);
    successful
}

pub fn clear_applied_items(summary: &mut ApplySummary) {
    summary.applied_items.clear();
}

pub fn apply_with_rollback(
    items: &mut [RestoreItem],
    executor: &mut RestoreExecutor,
    undo_executor: &mut UndoExecutor,
    options: &ApplyOptions,
) -> ApplyWithRollbackResult {
    let apply_summary = apply_restore_items(items, executor, options);
    if options.rollback.unwrap_or(false) && !apply_summary.applied_items.is_empty() {
        let mut applied_indices: Vec<usize> = items
            .iter()
            .enumerate()
            .filter(|(_, item)| {
                apply_summary.status_registry.get(&item.item_id)
                    == Some(&RestoreItemStatus::Applied)
            })
            .map(|(index, _)| index)
            .collect();
        let mut subset: Vec<RestoreItem> = applied_indices
            .iter()
            .map(|&index| items[index].clone())
            .collect();
        let rollback_summary = rollback_applied_items(
            &mut subset,
            undo_executor,
            options.fail_fast,
        );
        for (index, updated) in applied_indices.iter().zip(subset.iter()) {
            items[*index] = updated.clone();
        }
        let mut summary = apply_summary;
        clear_applied_items(&mut summary);
        return ApplyWithRollbackResult {
            apply_summary: summary,
            rollback_summary: Some(rollback_summary),
        };
    }
    let mut summary = apply_summary;
    if options.rollback.unwrap_or(false) {
        clear_applied_items(&mut summary);
    }
    ApplyWithRollbackResult {
        apply_summary: summary,
        rollback_summary: None,
    }
}