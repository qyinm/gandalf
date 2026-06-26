/**
 * Clack wizard for `gandalf bundle export`.
 *
 * Walks the user through:
 *   1. Picking a snapshot (or typing a new name)
 *   2. Choosing an output path
 *   3. Confirming metadata-only vs content
 *   4. Executing the export
 */

import path from "node:path";
import * as clack from "@clack/prompts";

import { bundleExport } from "@qxinm/gandalf-core/bundle.js";
import { ensureStore, listSnapshots } from "@qxinm/gandalf-core/store.js";
import type { RuntimeOptions } from "@qxinm/gandalf-core";
import { formatSnapError } from "@qxinm/gandalf-core/errors.js";

/**
 * Run the bundle export wizard interactively.
 * Returns exit code (0 = success, 1 = cancelled/error).
 */
export async function bundleExportWizard(
  options: RuntimeOptions
): Promise<number> {
  clack.intro("gandalf bundle export");

  await ensureStore(options.storeDir);
  const snapshots = await listSnapshots(options.storeDir, options.agent);

  // ── Step 1: Pick or type snapshot name ───────────────────
  let snapshotName: string | symbol;

  if (snapshots.length > 0) {
    snapshotName = await clack.select({
      message: "Select a snapshot to export:",
      options: [
        ...snapshots.map((name) => ({ label: name, value: name })),
        { label: "Type another name...", value: "__custom__" },
      ],
    });
  } else {
    const result = await clack.text({
      message: "No snapshots found. Enter a snapshot name to export:",
      placeholder: "my-setup",
      validate: (val) => {
        if (!val || val.trim().length === 0) return "Name is required";
        return;
      },
    });
    snapshotName = result;
  }

  if (clack.isCancel(snapshotName)) {
    clack.cancel("Export cancelled.");
    return 1;
  }

  if (snapshotName === "__custom__") {
    const custom = await clack.text({
      message: "Enter snapshot name:",
      placeholder: "baseline",
      validate: (val) => {
        if (!val || val.trim().length === 0) return "Name is required";
        return;
      },
    });
    if (clack.isCancel(custom)) {
      clack.cancel("Export cancelled.");
      return 1;
    }
    snapshotName = custom;
  }

  // ── Step 2: Output path ──────────────────────────────────
  const defaultOut = `${snapshotName}.gandalf`;

  const outputPath = await clack.text({
    message: "Output .gandalf path:",
    placeholder: defaultOut,
    initialValue: defaultOut,
    validate: (val) => {
      if (!val || val.trim().length === 0) return "Path is required";
      if (!val.endsWith(".gandalf")) return "Path should end with .gandalf";
      return;
    },
  });

  if (clack.isCancel(outputPath)) {
    clack.cancel("Export cancelled.");
    return 1;
  }

  // ── Step 3: Content or metadata-only ─────────────────────
  const includeContent = await clack.confirm({
    message: "Include file content in bundle?",
    active: "Yes, include content (default)",
    inactive: "No, metadata-only",
    initialValue: true,
  });

  if (clack.isCancel(includeContent)) {
    clack.cancel("Export cancelled.");
    return 1;
  }

  // ── Step 4: Confirm and execute ──────────────────────────
  const confirm = await clack.confirm({
    message: `Export "${snapshotName}" to ${outputPath}?`,
    active: "Yes, export",
    inactive: "Cancel",
    initialValue: true,
  });

  if (clack.isCancel(confirm) || !confirm) {
    clack.cancel("Export cancelled.");
    return 1;
  }

  const spinner = clack.spinner();
  spinner.start("Exporting bundle...");

  try {
    const result = await bundleExport({
      snapshotName,
      outputPath: path.resolve(outputPath),
      storeDir: options.storeDir,
      projectPath: options.projectPath,
      homeDir: options.homeDir,
      includeContent,
      agent: options.agent,
    });
    spinner.stop(`Exported to ${result.bundlePath}`);

    if (result.warnings && result.warnings.length > 0) {
      clack.note(result.warnings.join("\n"), "Warnings");
    }

    clack.outro("Export complete!");
    return 0;
  } catch (err) {
    spinner.stop("Export failed");
    process.stderr.write(
      formatSnapError({
        code: "GANDALF_BUNDLE_EXPORT_FAILED",
        problem: `Bundle export failed: ${err instanceof Error ? err.message : String(err)}`,
        cause: "An error occurred during bundle export.",
        fix: "Check the snapshot name and output path, then try again.",
      })
    );
    return 1;
  }
}
