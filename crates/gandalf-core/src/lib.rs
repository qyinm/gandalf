//! Gandalf core engine — Rust port of `packages/core`.

pub mod audit;
pub mod bundle;
pub mod diff;
pub mod errors;
pub mod graph;
pub mod parsers;
pub mod path_confinement;
pub mod policy;
pub mod provenance;
pub mod readiness;
pub mod report;
pub mod restore;
pub mod scan;
pub mod snapshot;
pub mod store;
pub mod tar;
pub mod timeline;
pub mod timeline_undo;
pub mod types;

pub const ENGINE_ID: &str = "gandalf-core";

pub use audit::audit_evidence;
pub use bundle::{
    bundle_export, bundle_import, bundle_inspect, bundle_verify, BundleError, BundleResult,
};
pub use tar::{read_tar, validate_tar_path, write_tar, TarError};
pub use diff::{diff_graphs, GraphDiff, RawSourceChange, SemanticChange, SemanticChangeCode};
pub use errors::{format_snap_error, SnapError};
pub use graph::build_graph;
pub use policy::{
    capture_status_for_key, ignored_directory, is_secret_like_key, redact_structured_value,
    restore_policy_for, MAX_DIRECTORY_DEPTH, MAX_DIRECTORY_ENTRIES, MAX_FILE_BYTES,
};
pub use parsers::{
    parse_dotenv_keys, parse_json, parse_markdown, parse_toml_key_values, DotenvEntry, ParseResult,
};
pub use path_confinement::{
    confinement_roots_from_paths, validate_constrained_write_path,
    validate_home_relative_import_segment, ConfinementRoots,
};
pub use provenance::build_provenance;
pub use restore::{
    apply_agent_config, apply_env, apply_env_key, apply_mcp_server, apply_permission,
    apply_restore_items, apply_with_rollback, build_restore_plan,
    clear_applied_items, create_default_apply_executor, create_default_undo_executor,
    default_apply_handler_registry, default_undo_handler_registry, dispatch_default_apply,
    dispatch_default_undo, format_apply_summary, get_applied_items, get_successful_items,
    noop_undo_handler, parse_dry_run_output, rollback_applied_items, sort_by_descending_order,
    write_file_atomically, ApplyHandlerRegistry, ParseDryRunError, ParseDryRunResult,
    RestoreExecutor, UndoExecutor, UndoHandlerRegistry,
};
pub use scan::{
    default_scanner_plugins, home_target, project_target, scan_project, scan_skill_directory,
    scan_target, scan_targets, ScanTarget, ScanTargetOverrides, ScannerContext, ScannerPlugin,
};
pub use readiness::{
    build_readiness_report, check_mcp_binary_availability, classify_mcp_binary,
    current_platform, extract_mcp_binaries, format_readiness_summary_lines,
    readiness_item_for_mcp_report, ReadinessFormatOptions, ReadinessOptions,
};
pub use report::{render_markdown_report, ReportInput, ReportTrust};
pub use snapshot::capture_current_state;
pub use timeline::{
    capture_timeline_snapshot, timeline_snapshot_name, CaptureTimelineOptions, CaptureTimelineResult,
    TimelineError,
};
pub use timeline_undo::{
    build_timeline_undo_plan, BuildTimelineUndoOptions, TimelineUndoAction, TimelineUndoItem,
    TimelineUndoPlan,
};
pub use store::{
    agent_store_dir, append_timeline_entry, default_store_dir, ensure_store, find_timeline_entry,
    list_agents, list_snapshots, list_timeline_entries, read_snapshot, read_snapshot_content,
    snapshot_exists, state_hash, write_snapshot, StoreError, StoreSnapshot, TimelineCorruptEvent,
    TimelineListOptions,
};
pub use types::*;

#[cfg(test)]
mod tests {
    use super::ENGINE_ID;

    #[test]
    fn workspace_smoke_test() {
        assert_eq!(ENGINE_ID, "gandalf-core");
    }
}