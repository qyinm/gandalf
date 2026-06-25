use std::io::{self, Write};
use std::path::{Path, PathBuf};

use clap::{Parser, Subcommand};
use hem_core::{
    apply_with_rollback, build_restore_plan, capture_current_state, create_default_apply_executor,
    create_default_undo_executor, default_store_dir, diff_graphs, ensure_store,
    format_apply_summary, format_snap_error, list_snapshots, parse_dry_run_output, read_snapshot,
    scan_project, snapshot_exists, write_snapshot, AgentId, EvidenceScope, GraphDiff, RestoreAction,
    RestoreOptions, RestorePlan, RollbackSummary, RuntimeOptions, ScanOptions, ScanResult,
    Severity, SnapError, StoreSnapshot, UndoStatus,
};

const VALID_AGENTS: &[&str] = &[
    "claude-code",
    "codex",
    "cursor",
    "opencode",
    "pi-agent",
    "project",
    "unknown",
];

const VALID_SCOPES: &[&str] = &["user", "project", "managed", "unknown"];

#[derive(Debug, Parser)]
#[command(
    name = "hem",
    about = "Save, compare, and restore Codex user-global setup experiments.",
    version
)]
pub struct Cli {
    #[command(subcommand)]
    command: Option<Commands>,
}

#[derive(Debug, Subcommand)]
pub enum Commands {
    /// Scan project for agent configuration and emit evidence inventory
    Scan(ScanArgs),
    /// Create, list, and show snapshots
    Snapshot {
        #[command(subcommand)]
        command: SnapshotCommand,
    },
    /// Show semantic and raw-source changes between two snapshots
    Diff(DiffArgs),
    /// Generate a restore plan (dry-run) or apply a snapshot (experimental)
    Restore(RestoreArgs),
}

#[derive(Debug, Parser)]
pub struct ScanArgs {
    #[command(flatten)]
    common: CommonOptions,
    /// Include paths considered during the scan
    #[arg(long)]
    explain: bool,
}

#[derive(Debug, Subcommand)]
pub enum SnapshotCommand {
    /// Capture and persist a snapshot
    Create(SnapshotCreateArgs),
    /// List snapshots in the store
    List(CommonOptions),
    /// Show snapshot metadata (or full JSON with --json)
    Show(SnapshotShowArgs),
}

#[derive(Debug, Parser)]
pub struct SnapshotCreateArgs {
    #[command(flatten)]
    common: CommonOptions,
    /// Snapshot name
    #[arg(long)]
    name: String,
    /// Store metadata only (no content capture)
    #[arg(long)]
    metadata_only: bool,
}

#[derive(Debug, Parser)]
pub struct SnapshotShowArgs {
    #[command(flatten)]
    common: CommonOptions,
    /// Snapshot name
    name: String,
}

#[derive(Debug, Parser)]
pub struct DiffArgs {
    #[command(flatten)]
    common: CommonOptions,
    /// Baseline snapshot reference (name or "current")
    baseline: String,
    /// Target snapshot reference (name or "current")
    target: String,
}

#[derive(Debug, Parser)]
pub struct RestoreArgs {
    #[command(flatten)]
    common: CommonOptions,
    /// Snapshot to restore from
    #[arg(long)]
    snapshot: String,
    /// Preview restore plan without making changes (default when --apply is omitted)
    #[arg(long, conflicts_with = "apply")]
    dry_run: bool,
    /// Apply restore items (requires --experimental)
    #[arg(long, conflicts_with = "dry_run")]
    apply: bool,
    /// Enable experimental apply mode
    #[arg(long)]
    experimental: bool,
    /// Stop on first failure during apply
    #[arg(long)]
    fail_fast: bool,
    /// Apply then automatically rollback on failure
    #[arg(long)]
    rollback: bool,
}

#[derive(Debug, Parser)]
pub struct CommonOptions {
    /// Project directory to scan
    #[arg(long, default_value = ".")]
    project: PathBuf,
    /// Home directory (defaults to $HOME)
    #[arg(long, env = "HOME")]
    home: Option<PathBuf>,
    /// Hem store directory (defaults to ~/.hem or $HEM_STORE)
    #[arg(long, env = "HEM_STORE")]
    store: Option<PathBuf>,
    /// Filter by agent
    #[arg(long)]
    agent: Option<String>,
    /// Filter by evidence scope
    #[arg(long)]
    scope: Option<String>,
    /// Emit JSON output
    #[arg(long)]
    json: bool,
}

pub fn run<I, S>(args: I) -> i32
where
    I: IntoIterator<Item = S>,
    S: AsRef<str>,
{
    let args: Vec<String> = args.into_iter().map(|s| s.as_ref().to_string()).collect();
    let cli = match Cli::try_parse_from(std::iter::once("hem".to_string()).chain(args)) {
        Ok(cli) => cli,
        Err(error) => {
            let _ = error.print();
            return error.exit_code();
        }
    };

    match cli.command {
        None => {
            print_help();
            0
        }
        Some(Commands::Scan(args)) => execute_scan(args),
        Some(Commands::Snapshot { command }) => execute_snapshot(command),
        Some(Commands::Diff(args)) => execute_diff(args),
        Some(Commands::Restore(args)) => execute_restore(args),
    }
}

fn execute_scan(args: ScanArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };

    let scan_options = ScanOptions {
        project_path: runtime.project_path,
        home_dir: runtime.home_dir,
        store_dir: runtime.store_dir,
        explain: Some(args.explain),
        agent: runtime.agent,
        scope: runtime.scope,
    };

    let scan = scan_project(&scan_options);

    if args.common.json {
        return write_json(&scan);
    }

    let output = if args.explain {
        render_scan_explain_text(&scan)
    } else {
        render_scan_text(&scan)
    };

    write_stdout(&output)
}

fn execute_snapshot(command: SnapshotCommand) -> i32 {
    match command {
        SnapshotCommand::Create(args) => execute_snapshot_create(args),
        SnapshotCommand::List(common) => execute_snapshot_list(common),
        SnapshotCommand::Show(args) => execute_snapshot_show(args),
    }
}

fn execute_snapshot_create(args: SnapshotCreateArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };

    let content_backed_codex_user =
        runtime.agent == Some(AgentId::Codex) && runtime.scope == Some(EvidenceScope::User);

    if !args.metadata_only && !content_backed_codex_user {
        return write_error(&SnapError {
            code: "HEM_METADATA_ONLY_REQUIRED".to_string(),
            problem: "Snapshots are metadata-only.".to_string(),
            cause: "`snapshot create` was called without `--metadata-only`.".to_string(),
            fix: "Add `--metadata-only`, or use `--agent codex --scope user` for the Codex rollback safety-net path.".to_string(),
            path: None,
        });
    }

    let mut capture_runtime = runtime.clone();
    capture_runtime.capture_content = Some(!args.metadata_only && content_backed_codex_user);

    let state = match capture_current_state(&capture_runtime, &args.name) {
        Ok(state) => state,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_SNAPSHOT_CAPTURE_FAILED".to_string(),
                problem: "Failed to capture current state.".to_string(),
                cause: error.to_string(),
                fix: "Verify project and store paths are accessible.".to_string(),
                path: None,
            });
        }
    };

    if let Err(error) = write_snapshot(
        Path::new(&runtime.store_dir),
        StoreSnapshot::from(state.snapshot),
        runtime.agent,
    ) {
        return write_error(&SnapError {
            code: "HEM_SNAPSHOT_WRITE_FAILED".to_string(),
            problem: "Failed to write snapshot.".to_string(),
            cause: error.to_string(),
            fix: "Verify the store directory is writable.".to_string(),
            path: None,
        });
    }

    let kind = if args.metadata_only {
        "metadata-only"
    } else {
        "content-backed"
    };
    let mut line = format!("Created {kind} snapshot: {}", args.name);
    if let Some(agent) = runtime.agent {
        line.push_str(&format!(" (agent: {})", agent.as_str()));
    }
    if let Some(scope) = runtime.scope {
        line.push_str(&format!(" (scope: {})", scope.as_str()));
    }
    line.push('\n');
    write_stdout(&line)
}

fn execute_snapshot_list(common: CommonOptions) -> i32 {
    let runtime = match resolve_runtime(&common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };

    let names = match list_snapshots(Path::new(&runtime.store_dir), runtime.agent) {
        Ok(names) => names,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_SNAPSHOT_LIST_FAILED".to_string(),
                problem: "Failed to list snapshots.".to_string(),
                cause: error.to_string(),
                fix: "Verify the store directory exists and is readable.".to_string(),
                path: None,
            });
        }
    };

    if common.json {
        return write_json(&names);
    }

    let output = if names.is_empty() {
        "No snapshots.\n".to_string()
    } else {
        format!("{}\n", names.join("\n"))
    };
    write_stdout(&output)
}

fn execute_snapshot_show(args: SnapshotShowArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };

    let snapshot = match read_snapshot(
        Path::new(&runtime.store_dir),
        &args.name,
        runtime.agent,
    ) {
        Ok(snapshot) => snapshot,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_SNAPSHOT_NOT_FOUND".to_string(),
                problem: format!("Snapshot \"{}\" not found.", args.name),
                cause: error.to_string(),
                fix: "Run `hem snapshot list` to see available snapshots.".to_string(),
                path: None,
            });
        }
    };

    if args.common.json {
        return write_json(&snapshot);
    }

    write_stdout(&format!("{}\n", snapshot.manifest.name))
}

fn execute_diff(args: DiffArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };

    let before = match snapshot_by_ref(&args.baseline, &runtime) {
        Ok(snapshot) => snapshot,
        Err(error) => return write_error(&error),
    };
    let after = match snapshot_by_ref(&args.target, &runtime) {
        Ok(snapshot) => snapshot,
        Err(error) => return write_error(&error),
    };

    let diff = diff_graphs(&before.graph, &after.graph);

    if args.common.json {
        return write_json(&diff);
    }

    write_stdout(&render_diff_text(&diff))
}

fn execute_restore(args: RestoreArgs) -> i32 {
    let runtime = match resolve_runtime(&args.common) {
        Ok(runtime) => runtime,
        Err(error) => return write_error(&error),
    };

    if let Err(error) = ensure_store(Path::new(&runtime.store_dir)) {
        return write_error(&SnapError {
            code: "HEM_STORE_INIT_FAILED".to_string(),
            problem: "Failed to initialize store.".to_string(),
            cause: error.to_string(),
            fix: "Verify the store directory is writable.".to_string(),
            path: None,
        });
    }

    let exists = match snapshot_exists(
        Path::new(&runtime.store_dir),
        &args.snapshot,
        runtime.agent,
    ) {
        Ok(exists) => exists,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_SNAPSHOT_LOOKUP_FAILED".to_string(),
                problem: format!("Failed to look up snapshot \"{}\".", args.snapshot),
                cause: error.to_string(),
                fix: "Run `hem snapshot list` to see available snapshots.".to_string(),
                path: None,
            });
        }
    };

    if !exists {
        return write_error(&SnapError {
            code: "HEM_SNAPSHOT_NOT_FOUND".to_string(),
            problem: format!("Snapshot \"{}\" not found.", args.snapshot),
            cause: "The named snapshot does not exist in the store.".to_string(),
            fix: "Run `hem snapshot list` to see available snapshots.".to_string(),
            path: None,
        });
    }

    let restore_options = RestoreOptions {
        source_snapshot: args.snapshot.clone(),
        project_path: runtime.project_path.clone(),
        home_dir: runtime.home_dir.clone(),
        store_dir: runtime.store_dir.clone(),
        dry_run: true,
        agent: runtime.agent,
        scope: runtime.scope,
    };

    let plan = match build_restore_plan(&restore_options) {
        Ok(plan) => plan,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_RESTORE_PLAN_FAILED".to_string(),
                problem: "Failed to build restore plan.".to_string(),
                cause: error.to_string(),
                fix: "Verify snapshot, project, and scope options.".to_string(),
                path: None,
            });
        }
    };

    if !args.apply {
        if args.common.json {
            return write_json(&plan);
        }
        return write_stdout(&format_restore_plan_preview(&plan));
    }

    let experimental = args.experimental || std::env::var("HEM_EXPERIMENTAL").is_ok();
    if !experimental {
        return write_error(&SnapError {
            code: "HEM_EXPERIMENTAL_REQUIRED".to_string(),
            problem: "Restore --apply requires --experimental.".to_string(),
            cause: "--apply was used without HEM_EXPERIMENTAL=1 or --experimental.".to_string(),
            fix: "Set HEM_EXPERIMENTAL=1 or pass --experimental to enable experimental features.".to_string(),
            path: None,
        });
    }

    let plan_json = match serde_json::to_string_pretty(&plan) {
        Ok(json) => json,
        Err(error) => {
            return write_error(&SnapError {
                code: "HEM_RESTORE_SERIALIZE_FAILED".to_string(),
                problem: "Failed to serialize restore plan.".to_string(),
                cause: error.to_string(),
                fix: "This is an internal error. Verify the snapshot is valid and try again.".to_string(),
                path: None,
            });
        }
    };

    let parsed = parse_dry_run_output(&plan_json);
    if !parsed.errors.is_empty() {
        return write_error(&SnapError {
            code: "HEM_RESTORE_PARSE_ERROR".to_string(),
            problem: "Failed to parse restore plan for execution.".to_string(),
            cause: parsed.errors[0].message.clone(),
            fix: "This is an internal error. Verify the snapshot is valid and try again.".to_string(),
            path: None,
        });
    }

    let mut items = parsed.items;
    let mut apply_executor = create_default_apply_executor();
    let mut undo_executor = create_default_undo_executor();
    let result = apply_with_rollback(
        &mut items,
        &mut apply_executor,
        &mut undo_executor,
        &hem_core::ApplyOptions {
            fail_fast: args.fail_fast,
            rollback: Some(args.rollback),
        },
    );

    let mut output = format_apply_summary(&result.apply_summary);
    if let Some(rollback) = &result.rollback_summary {
        output.push('\n');
        output.push_str(&format_rollback_summary(rollback));
    }

    if result.apply_summary.failed > 0 || result.rollback_summary.as_ref().map_or(0, |s| s.failed) > 0
    {
        let _ = io::stderr().write_all(output.as_bytes());
        return 1;
    }

    write_stdout(&output)
}

fn resolve_runtime(common: &CommonOptions) -> Result<RuntimeOptions, SnapError> {
    let home_dir = common
        .home
        .clone()
        .or_else(|| std::env::var("HOME").ok().map(PathBuf::from))
        .unwrap_or_else(|| std::env::current_dir().unwrap_or_else(|_| PathBuf::from(".")));

    let store_dir = common
        .store
        .clone()
        .unwrap_or_else(|| default_store_dir(&home_dir));

    let project_path = std::fs::canonicalize(&common.project)
        .unwrap_or_else(|_| common.project.clone());

    let agent = match &common.agent {
        Some(value) => {
            if !VALID_AGENTS.contains(&value.as_str()) {
                return Err(SnapError {
                    code: "HEM_INVALID_AGENT".to_string(),
                    problem: format!("Invalid agent: \"{value}\"."),
                    cause: "An unsupported agent identifier was provided.".to_string(),
                    fix: format!("Valid agents: {}", VALID_AGENTS.join(", ")),
                    path: None,
                });
            }
            Some(AgentId::from_str(value))
        }
        None => None,
    };

    let scope = match &common.scope {
        Some(value) => {
            if !VALID_SCOPES.contains(&value.as_str()) {
                return Err(SnapError {
                    code: "HEM_INVALID_SCOPE".to_string(),
                    problem: format!("Invalid scope: \"{value}\"."),
                    cause: "An unsupported evidence scope was provided.".to_string(),
                    fix: format!("Valid scopes: {}", VALID_SCOPES.join(", ")),
                    path: None,
                });
            }
            Some(parse_scope(value))
        }
        None => None,
    };

    Ok(RuntimeOptions {
        project_path: project_path.display().to_string(),
        home_dir: home_dir.display().to_string(),
        store_dir: store_dir.display().to_string(),
        agent,
        scope,
        capture_content: None,
    })
}

fn parse_scope(value: &str) -> EvidenceScope {
    match value {
        "user" => EvidenceScope::User,
        "project" => EvidenceScope::Project,
        "managed" => EvidenceScope::Managed,
        _ => EvidenceScope::Unknown,
    }
}

fn snapshot_by_ref(
    reference: &str,
    runtime: &RuntimeOptions,
) -> Result<hem_core::Snapshot, SnapError> {
    if reference == "current" {
        let state = capture_current_state(runtime, "current").map_err(|error| SnapError {
            code: "HEM_CURRENT_STATE_FAILED".to_string(),
            problem: "Failed to capture current state.".to_string(),
            cause: error.to_string(),
            fix: "Verify project and store paths are accessible.".to_string(),
            path: None,
        })?;
        return Ok(state.snapshot);
    }

    read_snapshot(
        Path::new(&runtime.store_dir),
        reference,
        runtime.agent,
    )
    .map_err(|error| SnapError {
        code: "HEM_SNAPSHOT_NOT_FOUND".to_string(),
        problem: format!("Snapshot \"{reference}\" not found."),
        cause: error.to_string(),
        fix: "Run `hem snapshot list` to see available snapshots.".to_string(),
        path: None,
    })
}

fn display_agent(agent: AgentId) -> &'static str {
    match agent {
        AgentId::ClaudeCode => "Claude Code",
        AgentId::Codex => "Codex",
        AgentId::Cursor => "Cursor",
        AgentId::Project => "Project",
        _ => agent.as_str(),
    }
}

fn render_scan_text(scan: &ScanResult) -> String {
    let mut lines = vec![
        "hem scan".to_string(),
        String::new(),
        format!("Read-only: {}", if scan.trust.read_only { "yes" } else { "no" }),
        format!("Network: {}", scan.trust.network),
        format!(
            "Commands executed: {}",
            scan.trust.commands_executed.len()
        ),
        format!("Writes: {}/index only", scan.trust.store_write_location),
        String::new(),
        "Detected agents".to_string(),
    ];

    let mut agents: Vec<AgentId> = scan
        .evidence
        .iter()
        .map(|item| item.agent)
        .collect::<std::collections::HashSet<_>>()
        .into_iter()
        .collect();
    agents.sort_by_key(|agent| agent.as_str().to_string());

    if agents.is_empty() {
        lines.push("  none".to_string());
    } else {
        for agent in agents {
            let scopes: std::collections::HashSet<_> = scan
                .evidence
                .iter()
                .filter(|item| item.agent == agent)
                .map(|item| item.scope.as_str())
                .collect();
            let mut scope_list: Vec<_> = scopes.into_iter().collect();
            scope_list.sort();
            lines.push(format!(
                "  {}  {} state found",
                display_agent(agent),
                scope_list.join(" + ")
            ));
        }
    }

    lines.push(String::new());
    lines.push("Blind spots".to_string());
    if scan.blind_spots.is_empty() {
        lines.push("  none".to_string());
    } else {
        for blind_spot in &scan.blind_spots {
            lines.push(format!("  {blind_spot}"));
        }
    }

    lines.push(String::new());
    lines.push("Next".to_string());
    lines.push("  hem snapshot create --name baseline --agent codex --scope user --project .".to_string());

    format!("{}\n", lines.join("\n"))
}

fn render_scan_explain_text(scan: &ScanResult) -> String {
    let mut output = render_scan_text(scan).trim_end().to_string();
    output.push_str("\n\nPaths considered\n");

    let mut paths: Vec<_> = scan
        .evidence
        .iter()
        .map(|item| item.source_path.as_str())
        .collect::<std::collections::HashSet<_>>()
        .into_iter()
        .collect();
    paths.sort();

    if paths.is_empty() {
        output.push_str("  none found\n");
    } else {
        for path in paths {
            output.push_str(&format!("  {path}\n"));
        }
    }

    output
}

fn render_diff_text(diff: &GraphDiff) -> String {
    let mut lines = vec![
        "hem diff".to_string(),
        String::new(),
        "Semantic changes".to_string(),
    ];

    if diff.semantic_changes.is_empty() {
        lines.push("  none".to_string());
    } else {
        for change in &diff.semantic_changes {
            lines.push(format!(
                "  {}  {}: {}",
                severity_label(change.severity),
                change.code.as_str(),
                change.entity_name
            ));
        }
    }

    lines.push(String::new());
    lines.push("Raw source changes".to_string());
    if diff.raw_source_changes.is_empty() {
        lines.push("  none".to_string());
    } else {
        for change in &diff.raw_source_changes {
            lines.push(format!("  {}: {}", change.status, change.source_path));
        }
    }

    format!("{}\n", lines.join("\n"))
}

fn format_restore_plan_preview(plan: &RestorePlan) -> String {
    let mut lines = vec![
        "hem restore dry-run".to_string(),
        String::new(),
        format!("Snapshot: {}", plan.source_snapshot),
        format!("Target project: {}", plan.target_project),
        format!("Target home: {}", plan.target_home),
        format!("Writable changes: {}", plan.items.len()),
        format!("Unsupported items: {}", plan.unsupported_items.len()),
        format!("Risk: {}", format_risk_summary(&plan.risk_summary)),
        String::new(),
    ];

    if plan.items.is_empty() && plan.unsupported_items.is_empty() {
        lines.push("No restore actions needed.".to_string());
    }

    if !plan.items.is_empty() {
        lines.push("Plan:".to_string());
        for (index, item) in plan.items.iter().enumerate() {
            lines.push(format_restore_plan_item(item, index + 1));
        }
    }

    if !plan.unsupported_items.is_empty() {
        lines.push(String::new());
        lines.push("Unsupported:".to_string());
        for (index, item) in plan.unsupported_items.iter().enumerate() {
            lines.push(format!(
                "{}. {} at {}\n   reason: {}",
                index + 1,
                item.kind.as_str(),
                item.source_path,
                item.reason
            ));
        }
    }

    lines.extend([
        String::new(),
        "No files were changed.".to_string(),
        "Use --apply --experimental to apply this plan.".to_string(),
        "Use --json for the machine-readable restore plan.".to_string(),
    ]);

    format!("{}\n", lines.join("\n"))
}

fn format_restore_plan_item(item: &hem_core::RestorePlanItem, index: usize) -> String {
    let mut fields = vec![
        format!(
            "{}. {} {} at {}",
            index,
            restore_action_label(item.action),
            item.kind.as_str(),
            item.source_path
        ),
        format!(
            "   risk: {}{}",
            severity_label(item.risk_level),
            if item.needs_confirmation {
                " (confirmation required)"
            } else {
                ""
            }
        ),
        format!("   rollback: {}", item.rollback_instruction),
    ];

    let mut changed_fields = item.diff.changes.clone();
    changed_fields.extend(item.diff.additions.iter().map(|field| format!("+{field}")));
    changed_fields.extend(item.diff.removals.iter().map(|field| format!("-{field}")));
    if !changed_fields.is_empty() {
        fields.push(format!("   fields: {}", changed_fields.join(", ")));
    }

    fields.join("\n")
}

fn severity_label(severity: Severity) -> &'static str {
    match severity {
        Severity::None => "none",
        Severity::Low => "low",
        Severity::Medium => "medium",
        Severity::High => "high",
        Severity::Critical => "critical",
    }
}

fn restore_action_label(action: RestoreAction) -> &'static str {
    match action {
        RestoreAction::Create => "create",
        RestoreAction::Update => "update",
        RestoreAction::Delete => "delete",
        RestoreAction::Skip => "skip",
        RestoreAction::Conflict => "conflict",
        RestoreAction::Unsupported => "unsupported",
    }
}

fn format_rollback_summary(summary: &RollbackSummary) -> String {
    let mut lines = vec![
        "Rollback complete.".to_string(),
        String::new(),
        format!("  Undone:  {}", summary.undone),
        format!("  Skipped: {}", summary.skipped),
        format!("  Failed:  {}", summary.failed),
        format!("  Total:   {}", summary.total),
    ];

    let failures: Vec<_> = summary
        .results
        .iter()
        .filter(|result| result.status == UndoStatus::Failed)
        .collect();
    if !failures.is_empty() {
        lines.push(String::new());
        lines.push("Failures:".to_string());
        for failure in failures {
            lines.push(format!(
                "  [{}] {}",
                failure.item_id,
                failure.reason.as_deref().unwrap_or("Unknown error")
            ));
        }
    }

    let skipped: Vec<_> = summary
        .results
        .iter()
        .filter(|result| result.status == UndoStatus::Skipped)
        .collect();
    if !skipped.is_empty() {
        lines.push(String::new());
        lines.push("Skipped (non-reversible):".to_string());
        for entry in skipped {
            lines.push(format!(
                "  [{}] {}",
                entry.item_id,
                entry.reason.as_deref().unwrap_or("No reason")
            ));
        }
    }

    format!("{}\n", lines.join("\n"))
}

fn format_risk_summary(risk_summary: &hem_core::RiskSummary) -> String {
    let counts = [
        ("critical", risk_summary.critical),
        ("high", risk_summary.high),
        ("medium", risk_summary.medium),
        ("low", risk_summary.low),
        ("none", risk_summary.none),
    ];
    let non_zero: Vec<_> = counts
        .iter()
        .filter(|(_, count)| *count > 0)
        .map(|(risk, count)| format!("{risk} {count}"))
        .collect();

    if non_zero.is_empty() {
        "none".to_string()
    } else {
        non_zero.join(", ")
    }
}

fn print_help() {
    let help = [
        "hem",
        "",
        "Save, compare, and restore Codex user-global setup experiments.",
        "",
        "Diagnosis commands:",
        "  hem scan --project .",
        "  hem scan --project . --explain",
        "  hem snapshot create --name baseline --agent codex --scope user --project .",
        "  hem snapshot create --name baseline --metadata-only --project .",
        "  hem snapshot list",
        "  hem snapshot list --agent codex",
        "  hem snapshot show baseline --json",
        "  hem diff baseline current --agent codex --scope user --project .",
        "  hem diff baseline current --project .",
        "",
        "Restore commands:",
        "  hem restore --snapshot <name> --dry-run --agent codex --scope user --project .",
        "  hem restore --snapshot <name> --apply --experimental --agent codex --scope user --project .",
    ];
    let _ = write_stdout(&format!("{}\n", help.join("\n")));
}

fn write_json<T: serde::Serialize>(value: &T) -> i32 {
    match serde_json::to_string_pretty(value) {
        Ok(json) => write_stdout(&format!("{json}\n")),
        Err(error) => write_error(&SnapError {
            code: "HEM_JSON_SERIALIZE_FAILED".to_string(),
            problem: "Failed to serialize JSON output.".to_string(),
            cause: error.to_string(),
            fix: "This is an internal error.".to_string(),
            path: None,
        }),
    }
}

fn write_stdout(text: &str) -> i32 {
    match io::stdout().write_all(text.as_bytes()) {
        Ok(()) => 0,
        Err(_) => 1,
    }
}

fn write_error(error: &SnapError) -> i32 {
    let _ = io::stderr().write_all(format_snap_error(error).as_bytes());
    1
}