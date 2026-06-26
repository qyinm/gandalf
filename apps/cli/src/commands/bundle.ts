/**
 * Command-pattern implementation of the `bundle` CLI command.
 *
 * Subcommands:
 *   bundle export --name <snapshot> --out <file.gandalf> [--metadata-only] [--json]
 *   bundle import <file.gandalf> [--apply-content] [--dry-run] [--quarantine] [--experimental] [--trust] [--json]
 *   bundle inspect <file.gandalf> [--json]
 *   bundle verify <file.gandalf> [--json]
 */

import path from "node:path";

import { bundleExport, bundleImport, bundleInspect, bundleVerify } from "@qxinm/gandalf-core/bundle.js";
import { hasFlag, json, runtimeOptions, valueAfter } from "../cli-shared.js";
import { formatSnapError } from "@qxinm/gandalf-core/errors.js";
import { formatReadinessSummaryLines } from "@qxinm/gandalf-core/readiness.js";
import { ensureStore } from "@qxinm/gandalf-core/store.js";
import { detectTuiMode } from "@qxinm/gandalf-tui";
import type { Command, CommandContext } from "./index.js";

// ── Command definition ─────────────────────────────────────────

export const bundleCommand: Command = {
  name: "bundle",
  description:
    "Export, import, inspect, and verify .gandalf bundle archives. " +
    "Usage: gandalf bundle export --name <snapshot> --out <file> [--metadata-only], " +
    "gandalf bundle import <file> [--dry-run] [--apply-content] [--quarantine] [--experimental] [--trust], " +
    "gandalf bundle inspect <file>, " +
    "gandalf bundle verify <file>",

  async execute(ctx: CommandContext): Promise<number> {
    const { args } = ctx;
    const options = runtimeOptions(args);
    const sub = args[1];

    /* ---------- bundle export ---------- */
    if (sub === "export") {
      // --tui: interactive wizard
      if (detectTuiMode(args).mode !== "none") {
        const { bundleExportWizard } = await import("@qxinm/gandalf-tui/wizards/bundle-export.js");
        return bundleExportWizard(options);
      }
      const snapshotName = valueAfter(args, "--name");
      const outputPath = valueAfter(args, "--out");

      if (!snapshotName || !outputPath) {
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_BUNDLE_MISSING_ARGS",
            problem: "Bundle export requires --name and --out.",
            cause: "`bundle export` was called without required flags.",
            fix: "Run `gandalf bundle export --name <snapshot> --out <file.gandalf> --project .`."
          })
        );
        return 1;
      }

      await ensureStore(options.storeDir);

      // Content is included by default. Use --metadata-only to opt out.
      // --include-content is kept for backward compatibility (no-op).
      const metadataOnly = hasFlag(args, "--metadata-only");
      const includeContent = !metadataOnly;

      const result = await bundleExport({
        snapshotName,
        outputPath: path.resolve(outputPath),
        storeDir: options.storeDir,
        projectPath: options.projectPath,
        homeDir: options.homeDir,
        includeContent,
        agent: options.agent
      });

      if (hasFlag(args, "--json")) {
        process.stdout.write(json(result));
      } else {
        process.stdout.write(`Exported ${snapshotName} to ${result.bundlePath}\n`);
        if (result.warnings && result.warnings.length > 0) {
          process.stdout.write(`\nWarnings:\n`);
          for (const warning of result.warnings) {
            process.stdout.write(`  - ${warning}\n`);
          }
        }
      }
      return 0;
    }

    /* ---------- bundle import ---------- */
    if (sub === "import") {
      // --tui: interactive wizard
      if (detectTuiMode(args).mode !== "none") {
        const { bundleImportWizard } = await import("@qxinm/gandalf-tui/wizards/bundle-import.js");
        return bundleImportWizard(options);
      }
      const bundlePath = args[2];
      if (!bundlePath) {
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_BUNDLE_MISSING_ARGS",
            problem: "Bundle import requires a .gandalf file path.",
            cause: "`bundle import` was called without a bundle path.",
            fix: "Run `gandalf bundle import <file.gandalf> --project .`."
          })
        );
        return 1;
      }

      await ensureStore(options.storeDir);

      const applyContent = hasFlag(args, "--apply-content");
      const isDryRun = hasFlag(args, "--dry-run");
      if (applyContent) {
        const experimental = hasFlag(args, "--experimental");
        if (!process.env.GANDALF_EXPERIMENTAL && !experimental) {
          process.stderr.write(
            formatSnapError({
              code: "GANDALF_EXPERIMENTAL_REQUIRED",
              problem: "Bundle import --apply-content requires --experimental.",
              cause: "--apply-content was used without GANDALF_EXPERIMENTAL=1 or --experimental.",
              fix: "Set GANDALF_EXPERIMENTAL=1 or pass --experimental to enable experimental features."
            })
          );
          return 1;
        }
      }

      const result = await bundleImport({
        bundlePath: path.resolve(bundlePath),
        storeDir: options.storeDir,
        projectPath: options.projectPath,
        homeDir: options.homeDir,
        applyContent,
        dryRun: isDryRun,
        quarantine: hasFlag(args, "--quarantine"),
        trust: hasFlag(args, "--trust"),
        agent: options.agent
      });

      if (hasFlag(args, "--json")) {
        process.stdout.write(json(result));
      } else {
        if (result.machineDiff) {
          const md = result.machineDiff;
          process.stdout.write(`Bundle source: ${md.sourceHostname} (${md.sourcePlatform})\n`);
          process.stdout.write(`Target machine: ${md.targetHostname} (${md.targetPlatform})\n`);
          process.stdout.write(`Source home: ${md.sourceHome}\n`);
          process.stdout.write(`Target home: ${md.targetHome}\n`);
          if (md.remappedPaths.length > 0) {
            process.stdout.write(`Remapped paths: ${md.remappedPaths.length}\n`);
            for (const p of md.remappedPaths.slice(0, 5)) {
              process.stdout.write(`  ${p}\n`);
            }
            if (md.remappedPaths.length > 5) {
              process.stdout.write(`  ... and ${md.remappedPaths.length - 5} more\n`);
            }
          }
          if (md.mcpBinaryReport.length > 0) {
            const unavailable = md.mcpBinaryReport.filter((b) => !b.availableOnTarget);
            process.stdout.write(`MCP binaries: ${md.mcpBinaryReport.length} total, ${md.mcpBinaryReport.length - unavailable.length} available, ${unavailable.length} missing\n`);
            for (const b of unavailable) {
              process.stdout.write(`  MISSING: ${b.command}\n`);
            }
          }
          if (md.osDifferences.length > 0) {
            process.stdout.write(`OS differences:\n`);
            for (const difference of md.osDifferences) {
              process.stdout.write(`  - ${difference}\n`);
            }
          }
          if (md.sourcePlatform !== md.targetPlatform) {
            process.stdout.write(`Warning: Cross-OS restore (${md.sourcePlatform} → ${md.targetPlatform})\n`);
          }
        }
        process.stdout.write(`${formatReadinessSummaryLines(result.readiness, { includeFixes: true }).join("\n")}\n`);
        if (!isDryRun) {
          process.stdout.write(`\nImported snapshot: ${result.snapshotName}\n`);
          process.stdout.write(`Evidence items: ${result.evidenceCount}\n`);
        }
        if (result.contentApplied) {
          process.stdout.write("Content files: applied\n");
        }
        if (result.quarantinedContentDir) {
          process.stdout.write(`Content files: quarantined at ${result.quarantinedContentDir}\n`);
        }
        if (result.warnings.length > 0) {
          process.stdout.write(`\nWarnings:\n`);
          for (const warning of result.warnings) {
            process.stdout.write(`  - ${warning}\n`);
          }
        }
      }
      return 0;
    }

    /* ---------- bundle inspect ---------- */
    if (sub === "inspect") {
      const bundlePath = args[2];
      if (!bundlePath) {
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_BUNDLE_MISSING_ARGS",
            problem: "Bundle inspect requires a .gandalf file path.",
            cause: "`bundle inspect` was called without a bundle path.",
            fix: "Run `gandalf bundle inspect <file.gandalf>`."
          })
        );
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
        if (result.sourceMachine) {
          process.stdout.write(`  Source machine: ${result.sourceMachine.hostname} (${result.sourceMachine.platform})\n`);
          process.stdout.write(`  Source home: ${result.sourceMachine.homeDir}\n`);
        }
        process.stdout.write(`  Bundle checksum: ${result.bundleChecksum.slice(0, 16)}...\n`);
        process.stdout.write(`  Signed: ${result.isSigned}\n`);
      }
      return 0;
    }

    /* ---------- bundle verify ---------- */
    if (sub === "verify") {
      const bundlePath = args[2];
      if (!bundlePath) {
        process.stderr.write(
          formatSnapError({
            code: "GANDALF_BUNDLE_MISSING_ARGS",
            problem: "Bundle verify requires a .gandalf file path.",
            cause: "`bundle verify` was called without a bundle path.",
            fix: "Run `gandalf bundle verify <file.gandalf>`."
          })
        );
        return 1;
      }

      const result = await bundleVerify({ bundlePath: path.resolve(bundlePath) });
      if (hasFlag(args, "--json")) {
        process.stdout.write(json(result));
      } else {
        process.stdout.write(`${result.valid ? "Bundle verification passed" : "Bundle verification failed"}: ${result.bundlePath}\n`);
        if (result.snapshotName) process.stdout.write(`  Snapshot: ${result.snapshotName}\n`);
        process.stdout.write(`  Checksums: ${result.checksums.checked ? (result.checksums.ok ? "ok" : "failed") : "not checked"} (${result.checksums.entriesChecked} entries)\n`);
        process.stdout.write(`  Signature: ${result.signature.signed ? (result.signature.checked ? (result.signature.ok ? "ok" : "failed") : "signed, not checked") : "unsigned"}\n`);
        if (result.warnings.length > 0) {
          process.stdout.write(`\nWarnings:\n`);
          for (const warning of result.warnings) process.stdout.write(`  - ${warning}\n`);
        }
        if (result.errors.length > 0) {
          process.stdout.write(`\nErrors:\n`);
          for (const error of result.errors) process.stdout.write(`  - ${error}\n`);
        }
      }
      return result.valid ? 0 : 1;
    }

    /* ---------- unknown subcommand ---------- */
    process.stderr.write(
      formatSnapError({
        code: "GANDALF_UNKNOWN_SUBCOMMAND",
        problem: `Unknown bundle subcommand: "${sub ?? ""}".`,
        cause: "`bundle` was called with an unrecognized subcommand.",
        fix: "Use `export`, `import`, or `inspect`. Run `gandalf --help` for details."
      })
    );
    return 1;
  }
};

export default bundleCommand;
