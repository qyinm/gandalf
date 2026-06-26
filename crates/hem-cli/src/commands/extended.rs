use std::fs;
use std::io::{self, Write};
use std::path::Path;

use clap::{Args, Subcommand};
use hem_core::{
    build_readiness_report, build_timeline_undo_plan, bundle_export, bundle_import, bundle_inspect,
    bundle_verify, capture_current_state, diff_graphs,
    format_readiness_summary_lines, list_timeline_entries, render_markdown_report,
    scan_project, BuildTimelineUndoOptions, BundleExportOptions, BundleImportOptions,
    BundleVerifyOptions, ReadinessOptions, ReportInput, ReportTrust,
    TimelineListOptions, TimelineUndoPlan,
};

use super::{
    resolve_runtime, snapshot_by_ref, write_error, write_json, write_stdout, CommonOptions,
    SnapError,
};

#[derive(Debug, Args)]
pub struct DoctorArgs {
    #[command(flatten)]
    common: CommonOptions,
}

#[derive(Debug, Args)]
pub struct ReportArgs {
    #[command(flatten)]
    common: CommonOptions,
    /// Snapshot reference (default: current)
    #[arg(default_value = "current")]
    reference: String,
    #[arg(long)]
    out: Option<String>,
}

#[derive(Debug, Subcommand)]
pub enum TimelineCommand {
    /// List timeline entries
    List(TimelineListArgs),
    /// Show a timeline entry by id or snapshot name
    Show(TimelineRefArgs),
    /// Build a dry-run MCP undo plan for a timeline entry
    Undo(TimelineUndoArgs),
}

#[derive(Debug, Args)]
pub struct TimelineListArgs {
    #[command(flatten)]
    common: CommonOptions,
}

#[derive(Debug, Args)]
pub struct TimelineRefArgs {
    #[command(flatten)]
    common: CommonOptions,
    reference: String,
}

#[derive(Debug, Args)]
pub struct TimelineUndoArgs {
    #[command(flatten)]
    common: CommonOptions,
    reference: String,
    #[arg(long)]
    dry_run: bool,
}

#[derive(Debug, Subcommand)]
pub enum BundleCommand {
    /// Export a snapshot to a .hem bundle
    Export(BundleExportArgs),
    /// Import a .hem bundle
    Import(BundleImportArgs),
    /// Inspect bundle metadata
    Inspect(BundlePathArgs),
    /// Verify bundle format, checksums, and signature
    Verify(BundleVerifyArgs),
}

#[derive(Debug, Args)]
pub struct BundleExportArgs {
    #[command(flatten)]
    common: CommonOptions,
    #[arg(long)]
    name: String,
    #[arg(long)]
    out: String,
    #[arg(long)]
    metadata_only: bool,
}

#[derive(Debug, Args)]
pub struct BundleImportArgs {
    #[command(flatten)]
    common: CommonOptions,
    bundle_path: String,
    #[arg(long)]
    dry_run: bool,
    #[arg(long)]
    apply_content: bool,
    #[arg(long)]
    quarantine: bool,
    #[arg(long)]
    experimental: bool,
    #[arg(long)]
    trust: bool,
}

#[derive(Debug, Args)]
pub struct BundlePathArgs {
    bundle_path: String,
    #[arg(long)]
    json: bool,
}

#[derive(Debug, Args)]
pub struct BundleVerifyArgs {
    bundle_path: String,
    #[arg(long)]
    json: bool,
}

pub fn execute_doctor(args: DoctorArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };
    let scan = scan_project(&hem_core::ScanOptions {
        project_path: runtime.project_path.clone(),
        home_dir: runtime.home_dir.clone(),
        store_dir: runtime.store_dir.clone(),
        explain: None,
        agent: runtime.agent,
        scope: runtime.scope,
    });
    let report = build_readiness_report(
        &scan.evidence,
        &ReadinessOptions {
            source_home_dir: Some(runtime.home_dir.as_str()),
            target_platform: None,
            apply_content: false,
            target_evidence: Some(scan.evidence.as_slice()),
            process_env: None,
            path_env: None,
        },
    );
    if args.common.json {
        return write_json(&report);
    }
    let mut lines = vec![
        "hem doctor".to_string(),
        String::new(),
        format!("Target platform: {}", report.target_platform),
        String::new(),
    ];
    lines.extend(format_readiness_summary_lines(
        &report,
        &hem_core::ReadinessFormatOptions {
            max_items: Some(10),
            include_fixes: true,
            include_actions: true,
        },
    ));
    if report.items.is_empty() {
        lines.push(String::new());
        lines.push("No readiness issues found.".to_string());
    }
    let exit = report
        .items
        .iter()
        .any(|item| item.category == hem_core::ReadinessCategory::Blocked);
    if write_stdout(&format!("{}\n", lines.join("\n"))) != 0 {
        return 1;
    }
    if exit { 1 } else { 0 }
}

pub fn execute_report(args: ReportArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };
    let snapshot = match snapshot_by_ref(&args.reference, &runtime) {
        Ok(snapshot) => snapshot,
        Err(error) => return write_error(&error),
    };
    let diff = if args.reference == "current" {
        None
    } else {
        let current = match capture_current_state(&runtime, "current") {
            Ok(state) => state,
            Err(error) => {
                return write_error(&SnapError {
                    code: "HEM_CURRENT_STATE_FAILED".to_string(),
                    problem: "Failed to capture current state.".to_string(),
                    cause: error.to_string(),
                    fix: "Verify project and store paths are accessible.".to_string(),
                    path: None,
                });
            }
        };
        Some(diff_graphs(&snapshot.graph, &current.snapshot.graph))
    };
    let scan = if args.reference == "current" {
        match capture_current_state(&runtime, "current") {
            Ok(state) => state.scan,
            Err(error) => {
                return write_error(&SnapError {
                    code: "HEM_CURRENT_STATE_FAILED".to_string(),
                    problem: "Failed to capture current state.".to_string(),
                    cause: error.to_string(),
                    fix: "Verify project and store paths are accessible.".to_string(),
                    path: None,
                });
            }
        }
    } else {
        scan_project(&hem_core::ScanOptions {
            project_path: runtime.project_path.clone(),
            home_dir: runtime.home_dir.clone(),
            store_dir: runtime.store_dir.clone(),
            explain: None,
            agent: runtime.agent,
            scope: runtime.scope,
        })
    };
    let markdown = render_markdown_report(&ReportInput {
        snapshot_name: Some(snapshot.manifest.name.as_str()),
        current: None,
        trust: ReportTrust {
            read_only: scan.trust.read_only,
            network: scan.trust.network.clone(),
            commands_executed: scan.trust.commands_executed.len() as u32,
        },
        evidence: &snapshot.evidence,
        graph: &snapshot.graph,
        findings: &snapshot.audit_findings,
        provenance: &snapshot.provenance,
        blind_spots: &scan.blind_spots,
        diffs: diff.as_ref(),
    });
    if args.common.json {
        return write_json(&serde_json::json!({
            "snapshot": snapshot,
            "markdown": markdown,
        }));
    }
    if let Some(out) = &args.out {
        if let Err(error) = fs::write(out, &markdown) {
            return write_error(&SnapError {
                code: "HEM_REPORT_WRITE_FAILED".to_string(),
                problem: "Failed to write report file.".to_string(),
                cause: error.to_string(),
                fix: "Verify the output path is writable.".to_string(),
                path: Some(out.clone()),
            });
        }
        write_stdout(&format!("Wrote report: {out}\n"))
    } else {
        write_stdout(&markdown)
    }
}

pub fn execute_timeline(command: TimelineCommand) -> i32 {
    match command {
        TimelineCommand::List(args) => execute_timeline_list(args),
        TimelineCommand::Show(args) => execute_timeline_show(args),
        TimelineCommand::Undo(args) => execute_timeline_undo(args),
    }
}

fn execute_timeline_list(args: TimelineListArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };
    let mut corrupt_events = Vec::new();
    let entries = match list_timeline_entries(
        Path::new(&runtime.store_dir),
        TimelineListOptions {
            agent: runtime.agent,
            project_path: Some(runtime.project_path.as_str()),
            limit: None,
            on_corrupt_entry: Some(&mut |event| corrupt_events.push(event)),
        },
    ) {
        Ok(entries) => entries,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_TIMELINE_LIST_FAILED".to_string(),
                problem: "Failed to list timeline entries.".to_string(),
                cause: error.to_string(),
                fix: "Verify the store directory is readable.".to_string(),
                path: None,
            });
        }
    };
    for event in &corrupt_events {
        let _ = writeln!(
            io::stderr(),
            "Skipped corrupt timeline event: {} ({})",
            event.file_path.display(),
            event.error
        );
    }
    if args.common.json {
        return write_json(&entries);
    }
    if entries.is_empty() {
        return write_stdout("No timeline entries.\n");
    }
    let mut lines = vec!["hem timeline".to_string(), String::new()];
    for entry in &entries {
        lines.push(format!(
            "- {} {} ({}) -> {}",
            entry.id,
            entry.title,
            format!("{:?}", entry.event_kind).to_lowercase(),
            entry.after_snapshot_name
        ));
    }
    write_stdout(&format!("{}\n", lines.join("\n")))
}

fn execute_timeline_show(args: TimelineRefArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };
    let entry = match find_timeline_entry_for_ref(&runtime, &args.reference, &mut Vec::new()) {
        Ok(Some(entry)) => entry,
        Ok(None) => {
            return write_error(&SnapError {
                code: "HEM_TIMELINE_NOT_FOUND".to_string(),
                problem: format!("Timeline entry not found: \"{}\".", args.reference),
                cause: "The reference does not match a timeline id or snapshot name.".to_string(),
                fix: "Run `hem timeline list` to see available entries.".to_string(),
                path: None,
            });
        }
        Err(error) => return write_error(&error),
    };
    if args.common.json {
        write_json(&entry)
    } else {
        write_stdout(&format!("{}\n", serde_json::to_string_pretty(&entry).unwrap_or_default()))
    }
}

fn execute_timeline_undo(args: TimelineUndoArgs) -> i32 {
    if !args.dry_run {
        return write_error(&SnapError {
            code: "HEM_TIMELINE_UNDO_DRY_RUN_REQUIRED".to_string(),
            problem: "Timeline undo requires --dry-run.".to_string(),
            cause: "`timeline undo` was called without `--dry-run`.".to_string(),
            fix: "Run `hem timeline undo <id> --dry-run --json`.".to_string(),
            path: None,
        });
    }
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };
    let mut corrupt_events = Vec::new();
    let plan = match build_timeline_undo_plan(
        Path::new(&runtime.store_dir),
        &args.reference,
        BuildTimelineUndoOptions {
            on_corrupt_entry: Some(&mut |event| corrupt_events.push(event)),
        },
    ) {
        Ok(plan) => plan,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_TIMELINE_UNDO_FAILED".to_string(),
                problem: "Failed to build timeline undo plan.".to_string(),
                cause: error.to_string(),
                fix: "Run `hem timeline list` to see available entries.".to_string(),
                path: None,
            });
        }
    };
    for event in &corrupt_events {
        let _ = writeln!(
            io::stderr(),
            "Skipped corrupt timeline event: {} ({})",
            event.file_path.display(),
            event.error
        );
    }
    if args.common.json {
        write_json(&plan)
    } else {
        write_stdout(&render_timeline_undo_text(&plan))
    }
}

pub fn execute_bundle(command: BundleCommand) -> i32 {
    match command {
        BundleCommand::Export(args) => execute_bundle_export(args),
        BundleCommand::Import(args) => execute_bundle_import(args),
        BundleCommand::Inspect(args) => execute_bundle_inspect(args),
        BundleCommand::Verify(args) => execute_bundle_verify(args),
    }
}

fn execute_bundle_export(args: BundleExportArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };
    let result = match bundle_export(&BundleExportOptions {
        snapshot_name: args.name.clone(),
        output_path: args.out.clone(),
        store_dir: runtime.store_dir.clone(),
        project_path: runtime.project_path.clone(),
        home_dir: runtime.home_dir.clone(),
        include_content: Some(!args.metadata_only),
        signature_key: None,
        agent: runtime.agent,
    }) {
        Ok(result) => result,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_BUNDLE_EXPORT_FAILED".to_string(),
                problem: "Failed to export bundle.".to_string(),
                cause: error.to_string(),
                fix: "Verify snapshot name and output path.".to_string(),
                path: None,
            });
        }
    };
    if args.common.json {
        write_json(&result)
    } else {
        write_stdout(&format!("Exported {} to {}\n", args.name, result.bundle_path))
    }
}

fn execute_bundle_import(args: BundleImportArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };
    if args.apply_content && !args.experimental && std::env::var("HEM_EXPERIMENTAL").is_err() {
        return write_error(&SnapError {
            code: "HEM_EXPERIMENTAL_REQUIRED".to_string(),
            problem: "Bundle content apply requires --experimental.".to_string(),
            cause: "--apply-content was used without HEM_EXPERIMENTAL or --experimental.".to_string(),
            fix: "Set HEM_EXPERIMENTAL=1 or pass --experimental.".to_string(),
            path: None,
        });
    }
    let result = match bundle_import(&BundleImportOptions {
        bundle_path: args.bundle_path.clone(),
        store_dir: runtime.store_dir.clone(),
        project_path: runtime.project_path.clone(),
        home_dir: runtime.home_dir.clone(),
        apply_content: Some(args.apply_content),
        dry_run: Some(args.dry_run),
        quarantine: Some(args.quarantine),
        trust: Some(args.trust),
        signature_key: None,
        agent: runtime.agent,
        target_platform: None,
    }) {
        Ok(result) => result,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_BUNDLE_IMPORT_FAILED".to_string(),
                problem: "Failed to import bundle.".to_string(),
                cause: error.to_string(),
                fix: "Verify bundle path and store permissions.".to_string(),
                path: None,
            });
        }
    };
    if args.common.json {
        write_json(&result)
    } else {
        write_stdout(&format!(
            "Imported {} (evidence: {}, dry_run: {})\n",
            result.snapshot_name, result.evidence_count, args.dry_run
        ))
    }
}

fn execute_bundle_inspect(args: BundlePathArgs) -> i32 {
    let result = match bundle_inspect(&args.bundle_path) {
        Ok(result) => result,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_BUNDLE_INSPECT_FAILED".to_string(),
                problem: "Failed to inspect bundle.".to_string(),
                cause: error.to_string(),
                fix: "Verify the bundle path exists.".to_string(),
                path: Some(args.bundle_path.clone()),
            });
        }
    };
    if args.json {
        write_json(&result)
    } else {
        write_stdout(&format!(
            "Bundle: {}\nSnapshot: {}\nSigned: {}\n",
            result.bundle_path, result.snapshot_name, result.is_signed
        ))
    }
}

fn execute_bundle_verify(args: BundleVerifyArgs) -> i32 {
    let result = match bundle_verify(&BundleVerifyOptions {
        bundle_path: args.bundle_path.clone(),
        signature_key: None,
    }) {
        Ok(result) => result,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_BUNDLE_VERIFY_FAILED".to_string(),
                problem: "Failed to verify bundle.".to_string(),
                cause: error.to_string(),
                fix: "Verify the bundle path exists.".to_string(),
                path: Some(args.bundle_path.clone()),
            });
        }
    };
    let exit = if result.valid { 0 } else { 1 };
    if args.json {
        if write_json(&result) != 0 {
            return 1;
        }
    } else {
        let status = if result.valid { "passed" } else { "failed" };
        if write_stdout(&format!("Bundle verification {status}: {}\n", result.bundle_path)) != 0 {
            return 1;
        }
    }
    exit
}

fn find_timeline_entry_for_ref(
    runtime: &hem_core::RuntimeOptions,
    reference: &str,
    corrupt_events: &mut Vec<hem_core::TimelineCorruptEvent>,
) -> Result<Option<hem_core::TimelineEntry>, SnapError> {
    hem_core::find_timeline_entry(
        Path::new(&runtime.store_dir),
        reference,
        TimelineListOptions {
            agent: runtime.agent,
            project_path: Some(runtime.project_path.as_str()),
            limit: None,
            on_corrupt_entry: Some(&mut |event| corrupt_events.push(event)),
        },
    )
    .map_err(|error| SnapError {
        code: "HEM_TIMELINE_LOOKUP_FAILED".to_string(),
        problem: "Failed to look up timeline entry.".to_string(),
        cause: error.to_string(),
        fix: "Run `hem timeline list` to see available entries.".to_string(),
        path: None,
    })
}

fn render_timeline_undo_text(plan: &TimelineUndoPlan) -> String {
    let mut lines = vec![
        format!("hem timeline undo (dry-run): {}", plan.title),
        format!("Writable items: {}", plan.writable_items.len()),
        format!("Observe-only surfaces: {}", plan.observe_only_surfaces.len()),
    ];
    for item in &plan.writable_items {
        lines.push(format!(
            "  - {} {} {}",
            item.action.as_str(),
            item.server_name,
            item.path
        ));
    }
    format!("{}\n", lines.join("\n"))
}