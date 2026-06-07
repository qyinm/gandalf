/**
 * Clack wizard for `snaptailor bundle import`.
 *
 * Walks the user through:
 *   1. Picking a .stailor file
 *   2. Dry-run preview
 *   3. Confirming content apply
 *   4. Executing the import
 */

import path from "node:path";
import * as clack from "@clack/prompts";
import fs from "node:fs";

import { bundleImport, bundleInspect } from "../../bundle.js";
import { ensureStore } from "../../store.js";
import type { RuntimeOptions } from "../../cli-shared.js";
import { formatSnapError } from "../../errors.js";
import { formatReadinessSummaryLines } from "../../readiness.js";

/**
 * Run the bundle import wizard interactively.
 * Returns exit code (0 = success, 1 = cancelled/error).
 */
export async function bundleImportWizard(
  options: RuntimeOptions
): Promise<number> {
  clack.intro("snaptailor bundle import");

  await ensureStore(options.storeDir);

  // ── Step 1: Pick a .stailor file ─────────────────────────
  // Scan cwd for .stailor files
  const stailorFiles = fs
    .readdirSync(options.projectPath)
    .filter((f) => f.endsWith(".stailor"));

  let bundlePath: string | symbol;

  if (stailorFiles.length > 0) {
    bundlePath = await clack.select({
      message: "Select a .stailor bundle to import:",
      options: [
        ...stailorFiles.map((name) => ({
          label: name,
          value: path.join(options.projectPath, name),
        })),
        { label: "Browse other path...", value: "__custom__" },
      ],
    });
  } else {
    const result = await clack.text({
      message: "Enter path to .stailor bundle:",
      placeholder: "./my-setup.stailor",
      validate: (val) => {
        if (!val || val.trim().length === 0) return "Path is required";
        if (!fs.existsSync(val)) return "File not found";
        return;
      },
    });
    bundlePath = result;
  }

  if (clack.isCancel(bundlePath)) {
    clack.cancel("Import cancelled.");
    return 1;
  }

  if (bundlePath === "__custom__") {
    const custom = await clack.text({
      message: "Enter bundle path:",
      placeholder: "./my-setup.stailor",
      validate: (val) => {
        if (!val || val.trim().length === 0) return "Path is required";
        if (!fs.existsSync(val)) return "File not found";
        return;
      },
    });
    if (clack.isCancel(custom)) {
      clack.cancel("Import cancelled.");
      return 1;
    }
    bundlePath = custom;
  }

  // ── Step 2: Inspect bundle ───────────────────────────────
  const spinner = clack.spinner();
  spinner.start("Inspecting bundle...");

  let inspect;
  try {
    inspect = await bundleInspect(path.resolve(bundlePath));
    spinner.stop("Bundle inspected");
  } catch (err) {
    spinner.stop("Inspect failed");
    process.stderr.write(
      formatSnapError({
        code: "SNAPTAILOR_BUNDLE_INSPECT_FAILED",
        problem: `Failed to read bundle: ${err instanceof Error ? err.message : String(err)}`,
        cause: "The bundle file could not be read or is invalid.",
        fix: "Verify the bundle file and try again.",
      })
    );
    return 1;
  }

  // Show bundle info
  const infoLines: string[] = [
    `Snapshot: ${inspect.snapshotName}`,
    `Created: ${inspect.createdAt}`,
    `Project: ${inspect.projectPath}`,
    `Content: ${inspect.includesContent ? `yes (${inspect.contentFileCount} files, ${inspect.contentTotalBytes} bytes)` : "no"}`,
  ];
  if (inspect.sourceMachine) {
    infoLines.push(
      `Source: ${inspect.sourceMachine.hostname} (${inspect.sourceMachine.platform})`
    );
  }
  clack.note(infoLines.join("\n"), "Bundle Info");

  // ── Step 3: Dry-run preview ──────────────────────────────
  clack.log.step("Running dry-run import...");
  const drySpinner = clack.spinner();
  drySpinner.start("Analyzing...");

  try {
    const dryResult = await bundleImport({
      bundlePath: path.resolve(bundlePath),
      storeDir: options.storeDir,
      projectPath: options.projectPath,
      homeDir: options.homeDir,
      applyContent: false,
      dryRun: true,
      quarantine: false,
      trust: false,
      agent: options.agent,
    });
    drySpinner.stop("Dry-run complete");

    if (dryResult.machineDiff) {
      const md = dryResult.machineDiff;
      const diffLines: string[] = [
        `Source: ${md.sourceHostname} (${md.sourcePlatform})`,
        `Target: ${md.targetHostname} (${md.targetPlatform})`,
        `Remapped paths: ${md.remappedPaths.length}`,
      ];
      if (md.mcpBinaryReport.length > 0) {
        const missing = md.mcpBinaryReport.filter(
          (b: { availableOnTarget: boolean }) => !b.availableOnTarget
        );
        diffLines.push(`MCP binaries: ${missing.length} missing`);
      }
      diffLines.push(...formatReadinessSummaryLines(dryResult.readiness, { maxItems: 3 }));
      clack.note(diffLines.join("\n"), "Cross-Machine Check");
    }

    clack.log.info(`Snapshot will be stored as: ${dryResult.snapshotName}`);
  } catch (err) {
    drySpinner.stop("Dry-run failed");
    process.stderr.write(
      formatSnapError({
        code: "SNAPTAILOR_BUNDLE_DRYRUN_FAILED",
        problem: `Dry-run failed: ${err instanceof Error ? err.message : String(err)}`,
        cause: "The bundle could not be validated against this machine.",
        fix: "Check the bundle compatibility with your current environment.",
      })
    );
    return 1;
  }

  // ── Step 4: Apply content? ───────────────────────────────
  let applyContent = false;
  if (inspect.includesContent) {
    const contentResult = await clack.confirm({
      message:
        "Apply content files from bundle? (requires --experimental)",
      active: "Yes, apply content",
      inactive: "No, metadata only",
      initialValue: false,
    });

    if (clack.isCancel(contentResult)) {
      clack.cancel("Import cancelled.");
      return 1;
    }
    applyContent = contentResult as boolean;
  }

  // ── Step 5: Confirm import ───────────────────────────────
  const confirm = await clack.confirm({
    message: `Import "${inspect.snapshotName}" and ${applyContent ? "apply content" : "store metadata only"}?`,
    active: "Yes, import",
    inactive: "Cancel",
    initialValue: true,
  });

  if (clack.isCancel(confirm) || !confirm) {
    clack.cancel("Import cancelled.");
    return 1;
  }

  // ── Step 6: Execute ──────────────────────────────────────
  const execSpinner = clack.spinner();
  execSpinner.start("Importing bundle...");

  try {
    const result = await bundleImport({
      bundlePath: path.resolve(bundlePath),
      storeDir: options.storeDir,
      projectPath: options.projectPath,
      homeDir: options.homeDir,
      applyContent,
      dryRun: false,
      quarantine: false,
      trust: false,
      agent: options.agent,
    });
    execSpinner.stop("Import complete");

    const summaryLines: string[] = [
      `Snapshot: ${result.snapshotName}`,
      `Evidence items: ${result.evidenceCount}`,
    ];
    if (result.contentApplied) summaryLines.push("Content: applied");
    if (result.warnings.length > 0) {
      summaryLines.push("", "Warnings:");
      summaryLines.push(...result.warnings);
    }
    clack.note(summaryLines.join("\n"), "Import Summary");

    clack.outro("Import complete!");
    return 0;
  } catch (err) {
    execSpinner.stop("Import failed");
    process.stderr.write(
      formatSnapError({
        code: "SNAPTAILOR_BUNDLE_IMPORT_FAILED",
        problem: `Bundle import failed: ${err instanceof Error ? err.message : String(err)}`,
        cause: "An error occurred during bundle import.",
        fix: "Check the bundle file and try again.",
      })
    );
    return 1;
  }
}
