/**
 * Dashboard — sidebar + tab layout for snaptailor TUI.
 *
 *  ┌──────────────┬────────────────────────────────────────────┐
 *  │  Agents       │  [Snapshots] [Scan] [Audit] [Diff]        │
 *  │  ──────────   │  ────────────────────────────────────────  │
 *  │  ▸ Claude Cd  │  (content based on active tab)             │
 *  │    Codex      │                                            │
 *  │    Cursor     │                                            │
 *  │    Project    │                                            │
 *  │               │                                            │
 *  │  ──────────   │                                            │
 *  │  ↑↓ nav       │                                            │
 *  │  Enter select │                                            │
 *  │  q quit       │                                            │
 *  └──────────────┴────────────────────────────────────────────┘
 */
import React, { useState, useCallback, useEffect } from "react";
import { Text, Box, useInput } from "ink";
import Spinner from "ink-spinner";

import { scanProject } from "../../scan.js";
import { buildGraph } from "../../graph.js";
import { auditEvidence } from "../../audit.js";
import { ensureStore, listSnapshots } from "../../store.js";
import { diffGraphs } from "../../diff.js";
import type { AuditFinding } from "../../types.js";
import type { ScanResult } from "../../scan.js";
import type { RuntimeOptions } from "../../cli-shared.js";
import type { AgentId } from "../../types.js";

import Sidebar, { buildAgentEntries, agentLabelStr } from "./Sidebar.js";
import TabBar from "./TabBar.js";
import type { TabId } from "./TabBar.js";
import ScanView from "./ScanView.js";
import AuditView from "./AuditView.js";
import DiffView from "./DiffView.js";
import SnapshotList from "./SnapshotList.js";
import ErrorPage from "./ErrorPage.js";

// ── State types ──────────────────────────────────────────────

type AppError = {
  code: string;
  problem: string;
  cause: string;
  fix: string;
};

type DashboardState = {
  status: "boot" | "ready" | "error";
  scan: ScanResult | null;
  findings: AuditFinding[];
  selectedAgent: AgentId | null;
  sidebarCursor: number;
  activeTab: TabId;
  /** snapshots for the currently selected agent */
  snapshots: string[];
  /** diff-specific loading */
  diffState:
    | { type: "idle" }
    | { type: "loading" }
    | { type: "rendered"; baseline: string; target: string; diff: any }
    | { type: "error"; message: string };
  error: AppError | null;
  /** Scroll offset for scan tab evidence/findings */
  scanScrollOffset: number;
};

// ── Constants ────────────────────────────────────────────────

type AgentAction =
  | "scan"
  | "save-snapshot"
  | "diff"
  | "audit"
  | "bundle-export"
  | "bundle-import"
  | "restore";

// ── Component ────────────────────────────────────────────────

interface DashboardProps {
  options: RuntimeOptions;
}

export default function Dashboard({ options }: DashboardProps) {
  const [state, setState] = useState<DashboardState>({
    status: "boot",
    scan: null,
    findings: [],
    selectedAgent: null,
    sidebarCursor: 0,
    activeTab: "snapshots",
    snapshots: [],
    diffState: { type: "idle" },
    error: null,
    scanScrollOffset: 0,
  });

  // ── Boot ───────────────────────────────────────────────────
  useEffect(() => {
    (async () => {
      try {
        await ensureStore(options.storeDir);
        const scan = await scanProject(options);
        const graph = buildGraph(scan.evidence);
        const findings = auditEvidence(scan.evidence, graph);
        const agents = buildAgentEntries(scan.evidence);
        const firstAgent = agents.length > 0 ? agents[0].id : null;

        let snapshots: string[] = [];
        if (firstAgent) {
          snapshots = await listSnapshots(options.storeDir, firstAgent);
        }

        setState((s) => ({
          ...s,
          status: "ready",
          scan,
          findings,
          selectedAgent: firstAgent,
          snapshots,
        }));
      } catch (err) {
        setState((s) => ({
          ...s,
          status: "error",
          error: {
            code: "SNAPTAILOR_INIT_FAILED",
            problem: `Initial scan failed: ${err instanceof Error ? err.message : String(err)}`,
            cause: "Could not detect agents in this project.",
            fix: "Verify the project path and try again.",
          },
        }));
      }
    })();
  }, []);

  // ── Helpers ────────────────────────────────────────────────

  const reScan = useCallback(async () => {
    try {
      await ensureStore(options.storeDir);
      const scan = await scanProject(options);
      const graph = buildGraph(scan.evidence);
      const findings = auditEvidence(scan.evidence, graph);
      const agents = buildAgentEntries(scan.evidence);

      // Re-select current agent or first
      const currentAgent = state.selectedAgent;
      const stillExists = agents.some((a) => a.id === currentAgent);
      const newAgent = stillExists ? currentAgent : (agents[0]?.id ?? null);

      let snapshots: string[] = [];
      if (newAgent) {
        snapshots = await listSnapshots(options.storeDir, newAgent);
      }

      setState((s) => ({
        ...s,
        status: "ready",
        scan,
        findings,
        selectedAgent: newAgent,
        snapshots,
        diffState: { type: "idle" },
        error: null,
      }));
    } catch {
      // silent
    }
  }, [options, state.selectedAgent]);

  const switchAgent = useCallback(
    async (agent: AgentId | null) => {
      let snapshots: string[] = [];
      if (agent) {
        try {
          snapshots = await listSnapshots(options.storeDir, agent);
        } catch {
          // silent
        }
      }
      setState((s) => ({
        ...s,
        selectedAgent: agent,
        snapshots,
        activeTab: "snapshots",
        diffState: { type: "idle" },
        scanScrollOffset: 0,
        sidebarCursor: Math.max(
          0,
          s.sidebarCursor < (agent ? buildAgentEntries(s.scan!.evidence).length : 0)
            ? s.sidebarCursor
            : 0
        ),
      }));
    },
    [options]
  );

  const runAction = useCallback(
    async (action: AgentAction) => {
      const agent = state.selectedAgent;
      if (!agent || !state.scan) return;

      if (action === "scan") {
        try {
          const agentOpts = { ...options, agent };
          const scan = await scanProject(agentOpts);
          const graph = buildGraph(scan.evidence);
          const findings = auditEvidence(scan.evidence, graph);
          const snapshots = await listSnapshots(options.storeDir, agent);
          setState((s) => ({
            ...s,
            scan: { ...s.scan!, evidence: scan.evidence, blindSpots: scan.blindSpots, trust: scan.trust },
            findings,
            snapshots,
            scanScrollOffset: 0,
            activeTab: "scan",
          }));
        } catch (err) {
          setState((s) => ({
            ...s,
            error: {
              code: "SNAPTAILOR_SCAN_FAILED",
              problem: `Scan failed: ${err instanceof Error ? err.message : String(err)}`,
              cause: `Could not scan ${agentLabelStr(agent)} configuration.`,
              fix: "Check agent config paths and permissions.",
            },
          }));
        }
      } else if (action === "save-snapshot") {
        try {
          const agentOpts = { ...options, agent };
          const scan = await scanProject(agentOpts);
          const graph = buildGraph(scan.evidence);
          const findings = auditEvidence(scan.evidence, graph);
          const { buildProvenance } = await import("../../provenance.js");
          const provenance = buildProvenance(graph, scan.evidence);
          const { writeSnapshot } = await import("../../store.js");
          const snapshotName = `${agent}-${Date.now()}`;
          await writeSnapshot(
            options.storeDir,
            {
              manifest: {
                schemaVersion: "0.1",
                name: snapshotName,
                createdAt: new Date().toISOString(),
                projectPath: options.projectPath,
                security: {
                  rawSecretsIncluded: false,
                  redactionPolicy: "metadata-only",
                },
              },
              evidence: scan.evidence,
              graph,
              auditFindings: findings,
              provenance,
            },
            agent
          );
          const snapshots = await listSnapshots(options.storeDir, agent);
          setState((s) => ({
            ...s,
            snapshots,
            activeTab: "snapshots",
          }));
        } catch (err) {
          setState((s) => ({
            ...s,
            error: {
              code: "SNAPTAILOR_SNAPSHOT_FAILED",
              problem: `Snapshot failed: ${err instanceof Error ? err.message : String(err)}`,
              cause: "Could not save snapshot.",
              fix: "Check store permissions.",
            },
          }));
        }
      } else if (action === "audit") {
        try {
          const agentOpts = { ...options, agent };
          const scan = await scanProject(agentOpts);
          const graph = buildGraph(scan.evidence);
          const findings = auditEvidence(scan.evidence, graph);
          setState((s) => ({
            ...s,
            findings,
            activeTab: "audit",
          }));
        } catch (err) {
          setState((s) => ({
            ...s,
            error: {
              code: "SNAPTAILOR_AUDIT_FAILED",
              problem: `Audit failed: ${err instanceof Error ? err.message : String(err)}`,
              cause: `Could not audit ${agentLabelStr(agent)}.`,
              fix: "Check agent config paths.",
            },
          }));
        }
      } else if (action === "diff") {
        setState((s) => ({ ...s, diffState: { type: "loading" }, activeTab: "diff" }));
        try {
          const snapshots = await listSnapshots(options.storeDir, agent);
          if (snapshots.length < 2) {
            setState((s) => ({
              ...s,
              diffState: {
                type: "error",
                message: `Need at least 2 snapshots for ${agentLabelStr(agent)}.`,
              },
            }));
            return;
          }
          const baseline = snapshots[snapshots.length - 2];
          const target = snapshots[snapshots.length - 1];
          const { readSnapshot } = await import("../../store.js");
          const before = await readSnapshot(options.storeDir, baseline, agent);
          const after = await readSnapshot(options.storeDir, target, agent);
          const diff = diffGraphs(before.graph, after.graph);
          setState((s) => ({
            ...s,
            diffState: { type: "rendered", baseline, target, diff },
          }));
        } catch (err) {
          setState((s) => ({
            ...s,
            diffState: {
              type: "error",
              message: err instanceof Error ? err.message : String(err),
            },
          }));
        }
      } else if (
        action === "bundle-export" ||
        action === "bundle-import" ||
        action === "restore"
      ) {
        try {
          const wizardOpts = { ...options, agent };
          let exitCode = 1;
          if (action === "bundle-export") {
            const { bundleExportWizard } = await import("../wizards/bundle-export.js");
            exitCode = await bundleExportWizard(wizardOpts);
          } else if (action === "bundle-import") {
            const { bundleImportWizard } = await import("../wizards/bundle-import.js");
            exitCode = await bundleImportWizard(wizardOpts);
          } else if (action === "restore") {
            const { restoreWizard } = await import("../wizards/restore-confirm.js");
            exitCode = await restoreWizard(wizardOpts);
          }
          if (exitCode !== 0) {
            setState((s) => ({
              ...s,
              error: {
                code: "SNAPTAILOR_OP_FAILED",
                problem: `${action} did not complete.`,
                cause: "Cancelled or error.",
                fix: "Try again.",
              },
            }));
            return;
          }
          // Refresh after wizard
          await reScan();
        } catch (err) {
          setState((s) => ({
            ...s,
            error: {
              code: "SNAPTAILOR_OP_ERROR",
              problem: `${action} error: ${err instanceof Error ? err.message : String(err)}`,
              cause: "Unexpected error.",
              fix: "Check logs and retry.",
            },
          }));
        }
      }
    },
    [options, state.selectedAgent, state.scan, reScan]
  );

  // ── Keyboard ───────────────────────────────────────────────

  const handleInput = useCallback(
    (input: string, key: { upArrow?: boolean; downArrow?: boolean; leftArrow?: boolean; rightArrow?: boolean; return?: boolean; escape?: boolean; tab?: boolean; shiftTab?: boolean }) => {
      if (state.status !== "ready") return;

      const agents = state.scan
        ? buildAgentEntries(state.scan.evidence)
        : [];
      const maxCursor = Math.max(0, agents.length - 1);

      // q = quit
      if (input === "q" || key.escape) {
        process.exit(0);
        return;
      }

      // r = re-scan
      if (input === "r") {
        reScan();
        return;
      }

      // s = save snapshot (only if agent selected)
      if (input === "s" && state.selectedAgent) {
        runAction("save-snapshot");
        return;
      }

      // d = diff (only if agent selected)
      if (input === "d" && state.selectedAgent) {
        runAction("diff");
        return;
      }

      // a = audit (only if agent selected)
      if (input === "a" && state.selectedAgent) {
        runAction("audit");
        return;
      }

      // --- Tab navigation: ← → or 1-4 ---
      const TAB_ORDER: TabId[] = ["snapshots", "scan", "audit", "diff"];
      if (key.leftArrow || input === "h") {
        const idx = TAB_ORDER.indexOf(state.activeTab);
        if (idx > 0) {
          setState((s) => ({ ...s, activeTab: TAB_ORDER[idx - 1] }));
        }
        return;
      }
      if (key.rightArrow || input === "l") {
        const idx = TAB_ORDER.indexOf(state.activeTab);
        if (idx < TAB_ORDER.length - 1) {
          setState((s) => ({ ...s, activeTab: TAB_ORDER[idx + 1] }));
        }
        return;
      }
      // 1-4 direct tab switch
      if (input === "1") { setState((s) => ({ ...s, activeTab: "snapshots" })); return; }
      if (input === "2") { setState((s) => ({ ...s, activeTab: "scan", scanScrollOffset: 0 })); return; }
      if (input === "3") { setState((s) => ({ ...s, activeTab: "audit" })); return; }
      if (input === "4") { setState((s) => ({ ...s, activeTab: "diff" })); return; }

      // --- ↑↓/jk: scroll scan tab OR navigate sidebar ---
      if (key.upArrow || input === "k") {
        if (state.activeTab === "scan") {
          setState((s) => ({
            ...s,
            scanScrollOffset: Math.max(0, s.scanScrollOffset - 1),
          }));
        } else {
          setState((s) => ({
            ...s,
            sidebarCursor: s.sidebarCursor > 0 ? s.sidebarCursor - 1 : maxCursor,
          }));
        }
        return;
      }
      if (key.downArrow || input === "j") {
        if (state.activeTab === "scan") {
          setState((s) => ({
            ...s,
            scanScrollOffset: s.scanScrollOffset + 1,
          }));
        } else {
          setState((s) => ({
            ...s,
            sidebarCursor: s.sidebarCursor < maxCursor ? s.sidebarCursor + 1 : 0,
          }));
        }
        return;
      }

      // --- Enter = select agent ---
      if (key.return) {
        const agent = agents[state.sidebarCursor]?.id ?? null;
        if (agent) switchAgent(agent);
        return;
      }
    },
    [state, reScan, switchAgent, runAction]
  );

  useInput(handleInput);

  // ── Render: boot state ────────────────────────
  if (state.status === "boot") {
    return (
      <Box>
        <Spinner type="dots" />
        <Text> Detecting agents...</Text>
      </Box>
    );
  }

  // ── Render: error state (full screen) ─────────
  if (state.status === "error" && state.error) {
    return (
      <Box flexDirection="column">
        <ErrorPage
          code={state.error.code}
          problem={state.error.problem}
          cause={state.error.cause}
          fix={state.error.fix}
        />
        <Box marginTop={1}>
          <Text dimColor>Press q to quit</Text>
        </Box>
      </Box>
    );
  }

  // ── Render: dashboard ─────────────────────────
  const agents = state.scan ? buildAgentEntries(state.scan.evidence) : [];

  return (
    <Box flexDirection="row" width="100%">
      {/* ── Sidebar ── */}
      <Sidebar
        agents={agents}
        selectedAgent={state.selectedAgent}
        cursor={state.sidebarCursor}
      />

      {/* ── Main panel ── */}
      <Box flexDirection="column" paddingX={1} flexGrow={1}>
        {/* Tab bar */}
        <TabBar
          activeTab={state.activeTab}
          onTabChange={(tab) => setState((s) => ({ ...s, activeTab: tab }))}
        />

        {/* Content area */}
        <Box flexDirection="column" flexGrow={1}>
          {state.error && (
            <Box marginBottom={1}>
              <ErrorPage
                code={state.error.code}
                problem={state.error.problem}
                cause={state.error.cause}
                fix={state.error.fix}
              />
            </Box>
          )}

          {(state.activeTab === "snapshots" || !state.selectedAgent) && renderSnapshots()}
          {state.activeTab === "scan" && state.selectedAgent && renderScan()}
          {state.activeTab === "audit" && state.selectedAgent && renderAudit()}
          {state.activeTab === "diff" && state.selectedAgent && renderDiff()}
        </Box>

        {/* Footer hint */}
        <Box marginTop={1}>
          <Text dimColor>
            {state.activeTab === "scan"
              ? "↑↓ scroll  ←→ tab  1-4 jump  s=save  d=diff  a=audit  r=rescan  q=quit"
              : "↑↓ agent  ←→ tab  1-4 jump  s=save  d=diff  a=audit  r=rescan  q=quit"}
          </Text>
        </Box>
      </Box>
    </Box>
  );

  // ── Render helpers ────────────────────────────

  function renderSnapshots() {
    if (!state.selectedAgent) {
      return <Text dimColor>Select an agent from the sidebar to view snapshots.</Text>;
    }

    return (
      <Box flexDirection="column">
        <SnapshotList names={state.snapshots} pageSize={15} />
        <Box marginTop={1} flexDirection="row" gap={2}>
          <Text color="cyan">[s] Save Snapshot</Text>
          <Text color="cyan">[d] Diff (last 2)</Text>
          <Text color="cyan">[a] Audit</Text>
        </Box>
      </Box>
    );
  }

  function renderScan() {
    if (!state.scan) return <Text dimColor>No scan data.</Text>;

    // If no agent selected, show full scan
    if (!state.selectedAgent) {
      return (
        <ScanView
          evidence={state.scan.evidence}
          auditFindings={state.findings}
          blindSpots={state.scan.blindSpots}
          readOnly={state.scan.trust.readOnly}
          scrollOffset={state.scanScrollOffset}
        />
      );
    }

    // Filter for selected agent
    const agentEvidence = state.scan.evidence.filter(
      (e) => e.agent === state.selectedAgent
    );
    const agentGraph = buildGraph(agentEvidence);
    const agentFindings = auditEvidence(agentEvidence, agentGraph);

    return (
      <Box flexDirection="column">
        <ScanView
          evidence={agentEvidence}
          auditFindings={agentFindings}
          blindSpots={state.scan.blindSpots}
          readOnly={state.scan.trust.readOnly}
          scrollOffset={state.scanScrollOffset}
        />
      </Box>
    );
  }

  function renderAudit() {
    if (!state.findings || state.findings.length === 0) {
      return <Text dimColor>No audit findings. Run Scan or Save Snapshot first.</Text>;
    }
    return <AuditView findings={state.findings} />;
  }

  function renderDiff() {
    const ds = state.diffState;

    if (ds.type === "idle") {
      // Auto-trigger diff when tab is entered (on next render)
      // But we need to handle this via effect or button
      return (
        <Box flexDirection="column">
          <Text dimColor>Press [d] to diff the two most recent snapshots.</Text>
          {state.snapshots.length >= 2 && (
            <Box marginTop={1} flexDirection="column">
              <Text dimColor>Baseline: {state.snapshots[state.snapshots.length - 2]}</Text>
              <Text dimColor>Target:   {state.snapshots[state.snapshots.length - 1]}</Text>
            </Box>
          )}
          <Box marginTop={1}>
            <Text color="cyan">[d] Run Diff</Text>
          </Box>
        </Box>
      );
    }

    if (ds.type === "loading") {
      return (
        <Box>
          <Spinner type="dots" />
          <Text> Diffing...</Text>
        </Box>
      );
    }

    if (ds.type === "error") {
      return (
        <ErrorPage
          code="DIFF_ERROR"
          problem={ds.message}
          cause="Could not load snapshots."
          fix="Save at least 2 snapshots first."
        />
      );
    }

    return (
      <Box flexDirection="column">
        <Text dimColor>
          {agentLabelStr(state.selectedAgent!)}: {ds.baseline} → {ds.target}
        </Text>
        <DiffView
          semanticChanges={ds.diff.semanticChanges}
          rawSourceChanges={ds.diff.rawSourceChanges}
        />
      </Box>
    );
  }
}