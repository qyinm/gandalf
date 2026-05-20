/**
 * Full TUI Dashboard for snaptailor.
 *
 * Keyboard-navigable command menu with inline result display.
 * Arrow keys / j/k to navigate, Enter to select, q/Esc to go back.
 *
 * Commands:
 *   scan    → full scan view
 *   diff    → pick two snapshots → diff view
 *   audit   → audit view
 *   snapshot → snapshot list
 *   bundle export/import → Clack wizards
 *   restore → Clack wizard
 */

import React, { useState, useCallback } from "react";
import { Text, Box, useInput } from "ink";
import Spinner from "ink-spinner";

import { scanProject } from "../../scan.js";
import { buildGraph } from "../../graph.js";
import { auditEvidence } from "../../audit.js";
import { buildProvenance } from "../../provenance.js";
import { ensureStore, listSnapshots } from "../../store.js";
import { diffGraphs } from "../../diff.js";
import type { AuditFinding } from "../../types.js";
import type { ScanResult } from "../../scan.js";
import type { RuntimeOptions } from "../../cli-shared.js";

import ScanView from "./ScanView.js";
import AuditView from "./AuditView.js";
import DiffView from "./DiffView.js";
import SnapshotList from "./SnapshotList.js";
import ErrorPage from "./ErrorPage.js";

// ── Menu items ───────────────────────────────────────────────

interface MenuItem {
  id: string;
  label: string;
  description: string;
}

const MENU_ITEMS: MenuItem[] = [
  { id: "scan", label: "Scan", description: "Scan project for agent configuration" },
  { id: "audit", label: "Audit", description: "Run audit rules on current state" },
  { id: "diff", label: "Diff", description: "Diff two snapshots" },
  { id: "snapshots", label: "Snapshots", description: "List saved snapshots" },
  { id: "bundle-export", label: "Bundle Export", description: "Export snapshot to .stailor file" },
  { id: "bundle-import", label: "Bundle Import", description: "Import .stailor bundle" },
  { id: "restore", label: "Restore", description: "Interactive restore wizard" },
  { id: "quit", label: "Quit", description: "Exit snaptailor" },
];

// ── Screens ──────────────────────────────────────────────────

type Screen =
  | { type: "menu" }
  | { type: "loading"; message: string }
  | { type: "scan-result"; scan: ScanResult; findings: AuditFinding[] }
  | { type: "audit-result"; findings: AuditFinding[] }
  | { type: "diff-result"; baseline: string; target: string }
  | { type: "snapshot-list"; names: string[] }
  | { type: "error"; code: string; problem: string; cause: string; fix: string; path?: string };

// ── Dashboard Component ──────────────────────────────────────

interface DashboardProps {
  options: RuntimeOptions;
}

export default function Dashboard({ options }: DashboardProps) {
  const [cursor, setCursor] = useState(0);
  const [screen, setScreen] = useState<Screen>({ type: "menu" });
  const [diffBaseline, setDiffBaseline] = useState<string | null>(null);

  // ── Keyboard handler ──────────────────────────────────────
  const handleInput = useCallback(
    (input: string, key: { upArrow?: boolean; downArrow?: boolean; return?: boolean; escape?: boolean }) => {
      if (screen.type !== "menu") {
        // In any non-menu screen, q or Esc goes back to menu
        if (input === "q" || key.escape) {
          setScreen({ type: "menu" });
        }
        return;
      }

      // Menu navigation
      if (key.upArrow || input === "k") {
        setCursor((c) => (c > 0 ? c - 1 : MENU_ITEMS.length - 1));
        return;
      }
      if (key.downArrow || input === "j") {
        setCursor((c) => (c < MENU_ITEMS.length - 1 ? c + 1 : 0));
        return;
      }
      if (key.escape || input === "q") {
        process.exit(0);
        return;
      }
      if (!key.return) return;

      // Execute selected action
      const selected = MENU_ITEMS[cursor];
      executeMenuAction(selected.id);
    },
    [cursor, screen]
  );

  // @ts-ignore - useInput is from ink but may not have official types
  useInput(handleInput);

  // ── Menu action executor ──────────────────────────────────
  async function executeMenuAction(id: string) {
    switch (id) {
      case "quit": {
        process.exit(0);
        break;
      }
      case "scan": {
        setScreen({ type: "loading", message: "Scanning project..." });
        try {
          const storeDir = options.storeDir;
          await ensureStore(storeDir);
          const scan = await scanProject(options);
          const graph = buildGraph(scan.evidence);
          const findings = auditEvidence(scan.evidence, graph);
          setScreen({ type: "scan-result", scan, findings });
        } catch (err) {
          setScreen({
            type: "error",
            code: "SNAPTAILOR_SCAN_FAILED",
            problem: `Scan failed: ${err instanceof Error ? err.message : String(err)}`,
            cause: "An error occurred during project scan.",
            fix: "Check the project path and permissions.",
          });
        }
        break;
      }
      case "audit": {
        setScreen({ type: "loading", message: "Running audit..." });
        try {
          await ensureStore(options.storeDir);
          const scan = await scanProject(options);
          const graph = buildGraph(scan.evidence);
          const findings = auditEvidence(scan.evidence, graph);
          setScreen({ type: "audit-result", findings });
        } catch (err) {
          setScreen({
            type: "error",
            code: "SNAPTAILOR_AUDIT_FAILED",
            problem: `Audit failed: ${err instanceof Error ? err.message : String(err)}`,
            cause: "An error occurred during audit.",
            fix: "Check the project path and try again.",
          });
        }
        break;
      }
      case "diff": {
        setScreen({ type: "loading", message: "Checking snapshots..." });
        try {
          await ensureStore(options.storeDir);
          const names = await listSnapshots(options.storeDir, options.agent);
          if (names.length < 2) {
          setScreen({
            type: "error",
            code: "SNAPTAILOR_DIFF_NO_SNAPSHOTS",
            problem: "Need at least 2 snapshots to diff.",
            cause: "Diff requires both a baseline and a target snapshot.",
            fix: "Create snapshots first with `snaptailor snapshot create`.",
          });
          return;
        }
        setDiffBaseline(names[0]);
        setScreen({ type: "diff-result", baseline: names[0], target: names[names.length - 1] });
        break;
      } catch (err) {
          setScreen({
            type: "error",
            code: "SNAPTAILOR_DIFF_FAILED",
            problem: `Diff failed: ${err instanceof Error ? err.message : String(err)}`,
            cause: "Failed to list or diff snapshots.",
            fix: "Verify snapshots exist and are compatible.",
          });
        }
      }
      case "snapshots": {
        setScreen({ type: "loading", message: "Listing snapshots..." });
        try {
          const names = await listSnapshots(options.storeDir, options.agent);
          setScreen({ type: "snapshot-list", names });
        } catch (err) {
          setScreen({
            type: "error",
            code: "SNAPTAILOR_SNAPSHOT_LIST_FAILED",
            problem: `Failed to list snapshots: ${err instanceof Error ? err.message : String(err)}`,
            cause: "An error occurred reading the snapshot store.",
            fix: "Check ~/.snaptailor directory permissions.",
          });
        }
        break;
      }
      case "bundle-export":
      case "bundle-import":
      case "restore": {
        // These use Clack wizards — delegate to the wizard function
        // (Clack handles its own keyboard input)
        try {
          let exitCode = 1;
          if (id === "bundle-export") {
            const { bundleExportWizard } = await import("../wizards/bundle-export.js");
            exitCode = await bundleExportWizard(options);
          } else if (id === "bundle-import") {
            const { bundleImportWizard } = await import("../wizards/bundle-import.js");
            exitCode = await bundleImportWizard(options);
          } else if (id === "restore") {
            const { restoreWizard } = await import("../wizards/restore-confirm.js");
            exitCode = await restoreWizard(options);
          }
          if (exitCode !== 0) {
            setScreen({
              type: "error",
              code: "SNAPTAILOR_OPERATION_FAILED",
              problem: `${id} operation did not complete successfully.`,
              cause: "The operation was cancelled or encountered an error.",
              fix: "Try again with more specific flags.",
            });
            return;
          }
        } catch (err) {
          setScreen({
            type: "error",
            code: "SNAPTAILOR_OPERATION_ERROR",
            problem: `${id} error: ${err instanceof Error ? err.message : String(err)}`,
            cause: "An unexpected error occurred.",
            fix: "Check the logs and try again.",
          });
          return;
        }
        // Back to menu after wizard completes
        setScreen({ type: "menu" });
        break;
      }
    }
  }

  // ── Render current screen ─────────────────────────────────
  switch (screen.type) {
    case "menu":
      return renderMenu(cursor);
    case "loading":
      return renderLoading(screen.message);
    case "scan-result":
      return (
        <Box flexDirection="column">
          <ScanView
            evidence={screen.scan.evidence}
            auditFindings={screen.findings}
            blindSpots={screen.scan.blindSpots}
            readOnly={screen.scan.trust.readOnly}
          />
          {renderBackHint()}
        </Box>
      );
    case "audit-result":
      return (
        <Box flexDirection="column">
          <AuditView findings={screen.findings} />
          {renderBackHint()}
        </Box>
      );
    case "diff-result":
      return <DiffViewInline baseline={screen.baseline} target={screen.target} options={options} />;
    case "snapshot-list":
      return (
        <Box flexDirection="column">
          <SnapshotList names={screen.names} />
          {renderBackHint()}
        </Box>
      );
    case "error":
      return (
        <Box flexDirection="column">
          <ErrorPage
            code={screen.code}
            problem={screen.problem}
            cause={screen.cause}
            fix={screen.fix}
            path={screen.path}
          />
          {renderBackHint()}
        </Box>
      );
  }
}

// ── Menu renderer ────────────────────────────────────────────

function renderMenu(cursor: number) {
  return (
    <Box flexDirection="column" paddingX={1}>
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold color="cyan">
          snaptailor TUI
        </Text>
      </Box>

      <Box marginBottom={1}>
        <Text dimColor>Use arrow keys / j/k to navigate, Enter to select, q to go back</Text>
      </Box>

      {/* Menu items */}
      {MENU_ITEMS.map((item, i) => (
        <Box key={item.id} marginBottom={0}>
          <Text bold color={i === cursor ? "cyan" : undefined}>
            {i === cursor ? "▸ " : "  "}
            {item.label.padEnd(16)}
          </Text>
          {i === cursor && <Text dimColor>{item.description}</Text>}
        </Box>
      ))}
    </Box>
  );
}

// ── Loading screen ───────────────────────────────────────────

function renderLoading(message: string) {
  return (
    <Box>
      <Spinner type="dots" />
      <Text> {message}</Text>
    </Box>
  );
}

// ── Back hint ────────────────────────────────────────────────

function renderBackHint() {
  return (
    <Box marginTop={1}>
      <Text dimColor>Press q to return to menu</Text>
    </Box>
  );
}

// ── Diff view inline ─────────────────────────────────────────

function DiffViewInline({
  baseline,
  target,
  options,
}: {
  baseline: string;
  target: string;
  options: RuntimeOptions;
}) {
  const [viewState, setViewState] = useState<{
    type: "loading" | "result" | "error";
    diff?: any;
    error?: string;
  }>({ type: "loading" });

  // Load diff on mount
  React.useEffect(() => {
    (async () => {
      try {
        const { readSnapshot } = await import("../../store.js");
        const before = await readSnapshot(options.storeDir, baseline, options.agent);
        const after = await readSnapshot(options.storeDir, target, options.agent);
        const diff = diffGraphs(before.graph, after.graph);
        setViewState({ type: "result", diff });
      } catch (err) {
        setViewState({
          type: "error",
          error: err instanceof Error ? err.message : String(err),
        });
      }
    })();
  }, []);

  if (viewState.type === "loading") {
    return (
      <Box>
        <Spinner type="dots" />
        <Text> Diffing {baseline} → {target}...</Text>
      </Box>
    );
  }

  if (viewState.type === "error") {
    return (
      <Box flexDirection="column">
        <ErrorPage
          code="DIFF_ERROR"
          problem={viewState.error ?? "Unknown error"}
          cause="Failed to load or compute diff."
          fix="Verify both snapshots exist and are compatible."
        />
        {renderBackHint()}
      </Box>
    );
  }

  return (
    <Box flexDirection="column">
      <Text dimColor>Diff: {baseline} → {target}</Text>
      <DiffView
        semanticChanges={viewState.diff.semanticChanges}
        rawSourceChanges={viewState.diff.rawSourceChanges}
      />
      {renderBackHint()}
    </Box>
  );
}