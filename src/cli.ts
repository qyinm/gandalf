#!/usr/bin/env node

import { writeFile } from "node:fs/promises";
import path from "node:path";

import { auditEvidence } from "./audit.js";
import { diffGraphs, type GraphDiff } from "./diff.js";
import { formatSnapError } from "./errors.js";
import { buildGraph } from "./graph.js";
import { buildProvenance } from "./provenance.js";
import { renderMarkdownReport } from "./report.js";
import { scanProject, type ScanResult } from "./scan.js";
import { defaultStoreDir, ensureStore, listSnapshots, readSnapshot, snapshotExists, writeSnapshot } from "./store.js";
import { buildRestorePlan, applyRestoreItems, applyWithRollback, formatApplySummary, formatRollbackSummary, createDefaultUndoExecutor, defaultUndoHandlerRegistry, parseDryRunOutput, createDefaultApplyExecutor, defaultApplyHandlerRegistry } from "./restore.js";
import { bundleExport, bundleImport, bundleInspect } from "./bundle.js";
import type { AuditFinding, ApplySummary, ApplyWithRollbackResult, RestoreExecutor, RestoreItem, UndoExecutor, Snapshot, SnapshotManifest } from "./types.js";

const HELP = `snaptailor

Read-only drift diagnosis and security audit for AI coding agent setups.

Core v0.1 commands:
  snaptailor scan --project .
  snaptailor scan --project . --explain
  snaptailor snapshot create --name baseline --metadata-only --project .
  snaptailor snapshot list
  snaptailor snapshot show baseline --json
  snaptailor diff baseline current --project .
  snaptailor audit current --project .
  snaptailor provenance current --project . --json
  snaptailor report current --project . --out snaptailor-report.md

v0.2 commands (dry-run only):
  snaptailor restore --snapshot <name> --dry-run --project .   generate a non-mutating restore plan as JSON

v0.2+ commands (apply + rollback):
  snaptailor restore --snapshot <name> --apply --project .              apply restore items sequentially
  snaptailor restore --snapshot <name> --apply --fail-fast --project .  stop on first failure during apply
  snaptailor restore --snapshot <name> --apply --rollback --project .   apply then automatically rollback

v0.2+ bundle commands:
  snaptailor bundle export --name <snapshot> --out <file.stailor> --project .   export snapshot to .stailor bundle
  snaptailor bundle import <file.stailor> --project .                           import .stailor bundle into local store
  snaptailor bundle inspect <file.stailor>                                      inspect bundle metadata
  snaptailor bundle import <file.stailor> --dry-run --project .                 validate bundle without importing
  snaptailor bundle export --name <snapshot> --out <file.stailor> --include-content --project .   export with raw file contents
`;

interface RuntimeOptions {
  projectPath: string;
  homeDir: string;
  storeDir: string;
}

interface CurrentState {
  scan: ScanResult;
  snapshot: Snapshot;
  storeFindings: AuditFinding[];
}

function valueAfter(args: string[], flag: string): string | undefined {
  const index = args.indexOf(flag);
  if (index === -1) {
    return undefined;
  }
  return args[index + 1];
}

function hasFlag(args: string[], flag: string): boolean {
  return args.includes(flag);
}

function runtimeOptions(args: string[]): RuntimeOptions {
  const homeDir = process.env.HOME ?? process.cwd();
  return {
    projectPath: path.resolve(valueAfter(args, "--project") ?? process.cwd()),
    homeDir,
    storeDir: process.env.SNAPTAILOR_STORE ?? defaultStoreDir(homeDir)
  };
}

function json(value: unknown): string {
  return `${JSON.stringify(value, null, 2)}\n`;
}

function trustForReport(scan: ScanResult): { readOnly: boolean; network: "disabled"; commandsExecuted: number } {
  return {
    readOnly: scan.trust.readOnly,
    network: scan.trust.network,
    commandsExecuted: scan.trust.commandsExecuted.length
  };
}

async function currentState(args: string[], name = "current"): Promise<CurrentState> {
  const options = runtimeOptions(args);
  const storeFindings = await ensureStore(options.storeDir);
  const scan = await scanProject(options);
  const graph = buildGraph(scan.evidence);
  const auditFindings = [...storeFindings, ...auditEvidence(scan.evidence, graph)];
  const provenance = buildProvenance(graph, scan.evidence);
  const manifest: SnapshotManifest = {
    schemaVersion: "0.1",
    name,
    createdAt: new Date().toISOString(),
    projectPath: options.projectPath,
    security: {
      rawSecretsIncluded: false,
      redactionPolicy: "metadata-only"
    }
  };

  return {
    scan,
    storeFindings,
    snapshot: {
      manifest,
      evidence: scan.evidence,
      graph,
      auditFindings,
      provenance
    }
  };
}

async function snapshotByRef(ref: string, args: string[]): Promise<Snapshot> {
  if (ref === "current") {
    return (await currentState(args)).snapshot;
  }
  return await readSnapshot(runtimeOptions(args).storeDir, ref);
}

function renderScanText(state: CurrentState): string {
  const lines = [
    "snaptailor scan",
    "",
    `Read-only: ${state.scan.trust.readOnly ? "yes" : "no"}`,
    `Network: ${state.scan.trust.network}`,
    `Commands executed: ${state.scan.trust.commandsExecuted.length}`,
    `Writes: ${state.scan.trust.storeWriteLocation}/index only`,
    "",
    "Detected agents"
  ];

  const agents = new Set(state.scan.evidence.map((item) => item.agent));
  if (agents.size === 0) {
    lines.push("  none");
  } else {
    for (const agent of [...agents].sort()) {
      const items = state.scan.evidence.filter((item) => item.agent === agent);
      const scopes = new Set(items.map((item) => item.scope));
      lines.push(`  ${displayAgent(agent)}  ${[...scopes].sort().join(" + ")} state found`);
    }
  }

  lines.push("", "High-signal findings");
  if (state.snapshot.auditFindings.length === 0) {
    lines.push("  none");
  } else {
    for (const finding of state.snapshot.auditFindings.slice(0, 8)) {
      lines.push(`  ${finding.severity.toUpperCase()}  ${finding.problem}`);
    }
  }

  lines.push("", "Blind spots");
  for (const blindSpot of state.scan.blindSpots) {
    lines.push(`  ${blindSpot}`);
  }

  lines.push("", "Next", "  snaptailor snapshot create --name baseline --metadata-only --project .");
  return `${lines.join("\n")}\n`;
}

function renderExplainText(state: CurrentState): string {
  const paths = [...new Set(state.scan.evidence.map((item) => item.sourcePath))].sort();
  const lines = [
    renderScanText(state).trimEnd(),
    "",
    "Paths considered"
  ];

  if (paths.length === 0) {
    lines.push("  none found");
  } else {
    for (const sourcePath of paths) {
      lines.push(`  ${sourcePath}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

function displayAgent(agent: string): string {
  if (agent === "claude-code") return "Claude Code";
  if (agent === "codex") return "Codex";
  if (agent === "cursor") return "Cursor";
  if (agent === "project") return "Project";
  return agent;
}

function renderDiffText(diff: GraphDiff): string {
  const lines = ["snaptailor diff", "", "Semantic changes"];
  if (diff.semanticChanges.length === 0) {
    lines.push("  none");
  } else {
    for (const change of diff.semanticChanges) {
      lines.push(`  ${change.severity.toUpperCase()}  ${change.code}: ${change.entityName}`);
    }
  }

  lines.push("", "Raw source changes");
  if (diff.rawSourceChanges.length === 0) {
    lines.push("  none");
  } else {
    for (const change of diff.rawSourceChanges) {
      lines.push(`  ${change.status}: ${change.sourcePath}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

function renderFindingsText(findings: AuditFinding[]): string {
  if (findings.length === 0) {
    return "No findings.\n";
  }
  return `${findings.map((finding) => `${finding.severity.toUpperCase()} ${finding.code}: ${finding.problem}`).join("\n")}\n`;
}

async function run(args: string[]): Promise<number> {
  if (args.length === 0 || hasFlag(args, "--help") || hasFlag(args, "-h")) {
    process.stdout.write(HELP);
    return 0;
  }

  if (args[0] === "scan") {
    const state = await currentState(args);
    process.stdout.write(hasFlag(args, "--json") ? json(state) : hasFlag(args, "--explain") ? renderExplainText(state) : renderScanText(state));
    return 0;
  }

  if (args[0] === "snapshot" && args[1] === "create") {
    const name = valueAfter(args, "--name");
    if (!name) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_MISSING_NAME",
        problem: "Snapshot name is required.",
        cause: "`snapshot create` was called without `--name`.",
        fix: "Run `snaptailor snapshot create --name baseline --metadata-only --project .`."
      }));
      return 1;
    }
    if (!hasFlag(args, "--metadata-only")) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_METADATA_ONLY_REQUIRED",
        problem: "v0.1 snapshots are metadata-only.",
        cause: "`snapshot create` was called without `--metadata-only`.",
        fix: "Add `--metadata-only`; raw content snapshots are not supported in v0.1."
      }));
      return 1;
    }

    const options = runtimeOptions(args);
    const state = await currentState(args, name);
    await writeSnapshot(options.storeDir, state.snapshot);
    process.stdout.write(`Created metadata-only snapshot: ${name}\n`);
    return 0;
  }

  if (args[0] === "snapshot" && args[1] === "list") {
    const names = await listSnapshots(runtimeOptions(args).storeDir);
    process.stdout.write(names.length === 0 ? "No snapshots.\n" : `${names.join("\n")}\n`);
    return 0;
  }

  if (args[0] === "snapshot" && args[1] === "show") {
    const name = args[2];
    if (!name) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_MISSING_NAME",
        problem: "Snapshot name is required.",
        cause: "`snapshot show` was called without a name.",
        fix: "Run `snaptailor snapshot list` and pass one of the listed names."
      }));
      return 1;
    }
    const snapshot = await readSnapshot(runtimeOptions(args).storeDir, name);
    process.stdout.write(hasFlag(args, "--json") ? json(snapshot) : `${snapshot.manifest.name}\n`);
    return 0;
  }

  if (args[0] === "diff") {
    const baseline = args[1];
    const target = args[2];
    if (!baseline || !target) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_DIFF_REFS_REQUIRED",
        problem: "Two snapshot references are required.",
        cause: "`diff` was called without baseline and target references.",
        fix: "Run `snaptailor diff baseline current --project .`."
      }));
      return 1;
    }
    const before = await snapshotByRef(baseline, args);
    const after = await snapshotByRef(target, args);
    const diff = diffGraphs(before.graph, after.graph);
    process.stdout.write(hasFlag(args, "--json") ? json(diff) : renderDiffText(diff));
    return 0;
  }

  if (args[0] === "audit") {
    const ref = args[1] ?? "current";
    const snapshot = await snapshotByRef(ref, args);
    process.stdout.write(hasFlag(args, "--json") ? json(snapshot.auditFindings) : renderFindingsText(snapshot.auditFindings));
    return 0;
  }

  if (args[0] === "provenance") {
    const ref = args[1] ?? "current";
    const snapshot = await snapshotByRef(ref, args);
    process.stdout.write(hasFlag(args, "--json") ? json(snapshot.provenance) : json(snapshot.provenance));
    return 0;
  }

  if (args[0] === "report") {
    const ref = args[1] ?? "current";
    const options = runtimeOptions(args);
    const snapshot = await snapshotByRef(ref, args);
    const diff = ref === "current" ? undefined : diffGraphs(snapshot.graph, (await currentState(args)).snapshot.graph);
    const scan = ref === "current" ? (await currentState(args)).scan : await scanProject(options);
    const markdown = renderMarkdownReport({
      snapshotName: snapshot.manifest.name,
      trust: trustForReport(scan),
      evidence: snapshot.evidence,
      graph: snapshot.graph,
      findings: snapshot.auditFindings,
      provenance: snapshot.provenance,
      blindSpots: scan.blindSpots,
      diffs: diff
    });
    if (hasFlag(args, "--json")) {
      process.stdout.write(json({ snapshot, markdown }));
      return 0;
    }
    const out = valueAfter(args, "--out");
    if (out) {
      await writeFile(path.resolve(out), markdown);
      process.stdout.write(`Wrote report: ${path.resolve(out)}\n`);
    } else {
      process.stdout.write(markdown);
    }
    return 0;
  }

  if (args[0] === "restore") {
    const snapshotName = valueAfter(args, "--snapshot");
    if (!snapshotName) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_RESTORE_SNAPSHOT_REQUIRED",
        problem: "Snapshot name is required for restore.",
        cause: "`restore` was called without `--snapshot`.",
        fix: "Run `snaptailor restore --snapshot <name> --dry-run --project .`."
      }));
      return 1;
    }

    const isDryRun = hasFlag(args, "--dry-run");
    const isApply = hasFlag(args, "--apply");

    if (!isDryRun && !isApply) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_RESTORE_MODE_REQUIRED",
        problem: "Either --dry-run or --apply is required for restore.",
        cause: "`restore` was called without `--dry-run` or `--apply`.",
        fix: "Add `--dry-run` to generate a non-mutating restore plan, or `--apply` to execute restore items."
      }));
      return 1;
    }

    if (isDryRun && isApply) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_RESTORE_MODE_CONFLICT",
        problem: "--dry-run and --apply are mutually exclusive.",
        cause: "Both `--dry-run` and `--apply` were passed.",
        fix: "Use `--dry-run` to preview changes, or `--apply` to execute them."
      }));
      return 1;
    }

    const options = runtimeOptions(args);
    await ensureStore(options.storeDir);

    const exists = await snapshotExists(options.storeDir, snapshotName);
    if (!exists) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_SNAPSHOT_NOT_FOUND",
        problem: `Snapshot "${snapshotName}" not found.`,
        cause: "The named snapshot does not exist in the store.",
        fix: "Run `snaptailor snapshot list` to see available snapshots."
      }));
      return 1;
    }

    // ── dry-run mode: build plan and output as JSON ───────────
    if (isDryRun) {
      const plan = await buildRestorePlan({
        sourceSnapshot: snapshotName,
        projectPath: options.projectPath,
        homeDir: options.homeDir,
        storeDir: options.storeDir,
        dryRun: true
      });

      process.stdout.write(json(plan));
      return 0;
    }

    // ── apply mode: build plan, parse items, execute ──────────
    const plan = await buildRestorePlan({
      sourceSnapshot: snapshotName,
      projectPath: options.projectPath,
      homeDir: options.homeDir,
      storeDir: options.storeDir,
      dryRun: true
    });

    // Serialize the plan and parse it into executable RestoreItems
    const planJson = json(plan);
    const parsed = parseDryRunOutput(planJson);
    if (parsed.errors.length > 0) {
      process.stderr.write(formatSnapError({
        code: "SNAPTAILOR_RESTORE_PARSE_ERROR",
        problem: "Failed to parse restore plan for execution.",
        cause: parsed.errors[0]?.message ?? "Unknown parse error",
        fix: "This is an internal error. Verify the snapshot is valid and try again."
      }));
      return 1;
    }

    const isFailFast = hasFlag(args, "--fail-fast");
    const isRollback = hasFlag(args, "--rollback");

    // Create the default undo executor from the built-in registry
    const undoExecutor: UndoExecutor = createDefaultUndoExecutor(
      defaultUndoHandlerRegistry()
    );

    // Apply executor — dispatches to per-type handlers (agent_config,
    // agent_instruction, mcp_server, permission, hook, skill, env_key).
    // Each handler saves previous state to item.rollbackState before mutating.
    const applyExecutor: RestoreExecutor = createDefaultApplyExecutor(
      defaultApplyHandlerRegistry()
    );

    const result = await applyWithRollback(parsed.items, applyExecutor, {
      failFast: isFailFast,
      rollback: isRollback,
      undoExecutor
    });

    // Render apply summary
    process.stdout.write(formatApplySummary(result.applySummary));

    // Render rollback summary if rollback was triggered
    if (result.rollbackSummary) {
      process.stdout.write("\n");
      process.stdout.write(formatRollbackSummary(result.rollbackSummary));
    }

    // Exit with non-zero if there were failures
    const hasFailures = result.applySummary.failed > 0 ||
      (result.rollbackSummary?.failed ?? 0) > 0;
    return hasFailures ? 1 : 0;
  }

  if (args[0] === "bundle") {
    const sub = args[1];
    const options = runtimeOptions(args);

    if (sub === "export") {
      const snapshotName = valueAfter(args, "--name");
      const outputPath = valueAfter(args, "--out");
      if (!snapshotName || !outputPath) {
        process.stderr.write(formatSnapError({
          code: "SNAPTAILOR_BUNDLE_MISSING_ARGS",
          problem: "Bundle export requires --name and --out.",
          cause: "`bundle export` was called without required flags.",
          fix: "Run `snaptailor bundle export --name <snapshot> --out <file.stailor> --project .`."
        }));
        return 1;
      }
      await ensureStore(options.storeDir);
      const result = await bundleExport({
        snapshotName,
        outputPath: path.resolve(outputPath),
        storeDir: options.storeDir,
        projectPath: options.projectPath,
        homeDir: options.homeDir,
        includeContent: hasFlag(args, "--include-content")
      });
      if (hasFlag(args, "--json")) {
        process.stdout.write(json(result));
      } else {
        process.stdout.write(`Exported ${snapshotName} to ${result.bundlePath}\n`);
      }
      return 0;
    }

    if (sub === "import") {
      const bundlePath = args[2];
      if (!bundlePath) {
        process.stderr.write(formatSnapError({
          code: "SNAPTAILOR_BUNDLE_MISSING_ARGS",
          problem: "Bundle import requires a .stailor file path.",
          cause: "`bundle import` was called without a bundle path.",
          fix: "Run `snaptailor bundle import <file.stailor> --project .`."
        }));
        return 1;
      }
      await ensureStore(options.storeDir);
      const result = await bundleImport({
        bundlePath: path.resolve(bundlePath),
        storeDir: options.storeDir,
        projectPath: options.projectPath,
        homeDir: options.homeDir,
        applyContent: hasFlag(args, "--apply-content"),
        dryRun: hasFlag(args, "--dry-run"),
        trust: hasFlag(args, "--trust")
      });
      if (hasFlag(args, "--json")) {
        process.stdout.write(json(result));
      } else {
        process.stdout.write(`Imported snapshot: ${result.snapshotName}\n`);
        process.stdout.write(`Evidence items: ${result.evidenceCount}\n`);
        if (result.contentApplied) {
          process.stdout.write("Content files: applied\n");
        }
      }
      return 0;
    }

    if (sub === "inspect") {
      const bundlePath = args[2];
      if (!bundlePath) {
        process.stderr.write(formatSnapError({
          code: "SNAPTAILOR_BUNDLE_MISSING_ARGS",
          problem: "Bundle inspect requires a .stailor file path.",
          cause: "`bundle inspect` was called without a bundle path.",
          fix: "Run `snaptailor bundle inspect <file.stailor>`."
        }));
        return 1;
      }
      const result = await bundleInspect(path.resolve(bundlePath));
      if (hasFlag(args, "--json")) {
        process.stdout.write(json(result));
      } else {
        process.stdout.write(`Bundle: ${result.bundlePath}\n`);
        process.stdout.write(`  Format: ${result.formatVersion}\n`);
        process.stdout.write(`  Snapshot: ${result.snapshotName}\n`);
        process.stdout.write(`  Created: ${result.createdAt}\n`);
        process.stdout.write(`  Project: ${result.projectPath}\n`);
        process.stdout.write(`  Includes content: ${result.includesContent}\n`);
        if (result.includesContent) {
          process.stdout.write(`  Content files: ${result.contentFileCount} (${result.contentTotalBytes} bytes)\n`);
        }
        process.stdout.write(`  Bundle checksum: ${result.bundleChecksum.slice(0, 16)}...\n`);
        process.stdout.write(`  Signed: ${result.isSigned}\n`);
      }
      return 0;
    }

    process.stderr.write(formatSnapError({
      code: "SNAPTAILOR_UNKNOWN_SUBCOMMAND",
      problem: "Unknown bundle subcommand.",
      cause: `snaptailor bundle does not recognize "${sub ?? ""}".`,
      fix: "Run `snaptailor --help` to see supported bundle commands."
    }));
    return 1;
  }

  process.stderr.write(formatSnapError({
    code: "SNAPTAILOR_UNKNOWN_COMMAND",
    problem: "Unknown command.",
    cause: `snaptailor does not recognize "${args.join(" ")}".`,
    fix: "Run `snaptailor --help` to see supported v0.1 commands."
  }));
  return 1;
}

run(process.argv.slice(2))
  .then((code) => {
    process.exitCode = code;
  })
  .catch((error: unknown) => {
    process.stderr.write(formatSnapError({
      code: "SNAPTAILOR_UNHANDLED_ERROR",
      problem: "Command failed.",
      cause: error instanceof Error ? error.message : "Unknown error.",
      fix: "Rerun with `--help` to confirm command syntax, then inspect the reported path if present."
    }));
    process.exitCode = 1;
  });
