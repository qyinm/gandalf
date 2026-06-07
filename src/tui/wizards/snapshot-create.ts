/**
 * Clack wizard for `hem snapshot create`.
 *
 * Walks the user through:
 *   1. Entering a snapshot name
 *   2. Choosing metadata-only vs content
 *   3. Running scan + creating snapshot
 */

import * as clack from "@clack/prompts";

import { auditEvidence } from "../../audit.js";
import { buildGraph } from "../../graph.js";
import { buildProvenance } from "../../provenance.js";
import { scanProject } from "../../scan.js";
import { ensureStore, writeSnapshot } from "../../store.js";
import type { AuditFinding, Snapshot, SnapshotManifest } from "../../types.js";
import type { ScanResult } from "../../scan.js";
import type { RuntimeOptions } from "../../cli-shared.js";
import { formatSnapError } from "../../errors.js";

/**
 * Run the snapshot create wizard interactively.
 * Returns exit code (0 = success, 1 = cancelled/error).
 */
export async function snapshotCreateWizard(
  options: RuntimeOptions
): Promise<number> {
  clack.intro("hem snapshot create");

  await ensureStore(options.storeDir);

  // ── Step 1: Enter snapshot name ──────────────────────────
  const name = await clack.text({
    message: "Snapshot name:",
    placeholder: "baseline",
    validate: (val) => {
      if (!val || val.trim().length === 0) return "Name is required";
      if (/[\\/]/.test(val)) return "Name cannot contain / or \\";
      if (val.includes("..")) return "Name cannot contain '..'";
      return;
    },
  });

  if (clack.isCancel(name)) {
    clack.cancel("Snapshot creation cancelled.");
    return 1;
  }

  // ── Step 2: Metadata-only or include content? ────────────
  const metadataOnly = await clack.confirm({
    message: "Metadata-only snapshot? (omit file content values)",
    active: "Yes, metadata-only (safe, default)",
    inactive: "No, include parsed content",
    initialValue: true,
  });

  if (clack.isCancel(metadataOnly)) {
    clack.cancel("Snapshot creation cancelled.");
    return 1;
  }

  // ── Step 3: Confirm ──────────────────────────────────────
  const confirm = await clack.confirm({
    message: `Create snapshot "${name}" (${metadataOnly ? "metadata-only" : "with content"})?`,
    active: "Yes, create snapshot",
    inactive: "Cancel",
    initialValue: true,
  });

  if (clack.isCancel(confirm) || !confirm) {
    clack.cancel("Snapshot creation cancelled.");
    return 1;
  }

  // ── Step 4: Scan and create ──────────────────────────────
  const spinner = clack.spinner();
  spinner.start("Scanning project...");

  try {
    const scan: ScanResult = await scanProject(options);
    const graph = buildGraph(scan.evidence);
    const auditFindings: AuditFinding[] = auditEvidence(scan.evidence, graph);
    const provenance = buildProvenance(graph, scan.evidence);

    const manifest: SnapshotManifest = {
      schemaVersion: "0.1",
      name,
      createdAt: new Date().toISOString(),
      projectPath: options.projectPath,
      security: {
        rawSecretsIncluded: false,
        redactionPolicy: "metadata-only",
      },
    };

    const snapshot: Snapshot = {
      manifest,
      evidence: scan.evidence,
      graph,
      auditFindings,
      provenance,
    };

    await writeSnapshot(options.storeDir, snapshot, options.agent);

    // Show summary
    const agentCount = new Set(scan.evidence.map((e) => e.agent)).size;
    const findingCount = auditFindings.length;
    const evidenceCount = scan.evidence.length;

    spinner.stop(`Snapshot "${name}" created`);

    const summaryLines: string[] = [
      `Agents detected: ${agentCount}`,
      `Evidence items: ${evidenceCount}`,
      `Audit findings: ${findingCount}`,
    ];
    if (findingCount > 0) {
      const highFindings = auditFindings.filter(
        (f) => f.severity === "high" || f.severity === "critical"
      );
      if (highFindings.length > 0) {
        summaryLines.push("", "High-signal findings:");
        for (const f of highFindings.slice(0, 5)) {
          summaryLines.push(`  ${f.severity.toUpperCase()}  ${f.problem}`);
        }
        if (highFindings.length > 5) {
          summaryLines.push(`  ... and ${highFindings.length - 5} more`);
        }
      }
    }

    clack.note(summaryLines.join("\n"), "Snapshot Summary");
    clack.outro("Snapshot created!");
    return 0;
  } catch (err) {
    spinner.stop("Snapshot creation failed");
    process.stderr.write(
      formatSnapError({
        code: "HEM_SNAPSHOT_CREATE_FAILED",
        problem: `Snapshot creation failed: ${err instanceof Error ? err.message : String(err)}`,
        cause: "An error occurred during scanning or snapshot write.",
        fix: "Check the project path and permissions, then try again.",
      })
    );
    return 1;
  }
}