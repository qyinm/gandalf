/**
 * Dashboard — sidebar + tab layout for hem TUI.
 *
 *  ┌──────────────┬────────────────────────────────────────────┐
 *  │  Agents       │  Daemon: running/stopped/stale/error      │
 *  │               │  [Snapshots] [Scan] [Audit] [Diff]        │
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
import React, { useState, useCallback, useEffect, useRef } from "react";
import { Text, Box, useInput } from "ink";
import Spinner from "ink-spinner";

import { scanProject } from "../../scan.js";
import { buildGraph } from "../../graph.js";
import { auditEvidence } from "../../audit.js";
import { ensureStore, listSnapshots, listTimelineEntries } from "../../store.js";
import { diffGraphs } from "../../diff.js";
import type { AuditFinding } from "../../types.js";
import type { ScanResult } from "../../scan.js";
import type { RuntimeOptions } from "../../cli-shared.js";
import type { AgentId, DaemonStatusReadResult, Snapshot, TimelineEntry } from "../../types.js";
import type { TimelineCorruptEvent } from "../../store.js";
import type { TimelineUndoPlan } from "../../timeline-undo.js";
import type { GraphDiff } from "../../diff.js";

import Sidebar, { buildAgentEntries, buildAgentFilterEntries, agentLabelStr } from "./Sidebar.js";
import TabBar, { TABS } from "./TabBar.js";
import type { TabId } from "./TabBar.js";
import ScanView, { DEFAULT_SCAN_WINDOW_SIZE } from "./ScanView.js";
import AuditView from "./AuditView.js";
import DiffView from "./DiffView.js";
import SnapshotList from "./SnapshotList.js";
import SaveSetupView from "./SaveSetupView.js";
import { buildSaveSetupViewModel } from "./SaveSetupViewModel.js";
import ErrorPage from "./ErrorPage.js";
import TimelineView from "./TimelineView.js";
import { clampTimelineIndex } from "./TimelineViewModel.js";
import AgentDetailView from "./AgentDetailView.js";
import {
  INITIAL_NAV_ITEM_ID,
  buildTuiNavigationModel,
  navItemIdForSelection,
  selectTuiNavItem,
  type TuiScreenId
} from "./TuiNavigationModel.js";

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
  timelineEntries: TimelineEntry[];
  timelineCorruptEvents: TimelineCorruptEvent[];
  timelineCursor: number;
  timelineUndoState:
    | { type: "idle" }
    | { type: "rendered"; plan: TimelineUndoPlan }
    | { type: "error"; message: string };
  /** full setup snapshots */
  snapshots: string[];
  saveSetupState:
    | { type: "idle" }
    | { type: "loading" }
    | {
        type: "preview";
        snapshot: Snapshot;
        diff?: GraphDiff;
        hasPreviousSnapshot: boolean;
        title: string;
      }
    | {
        type: "saving";
        snapshot: Snapshot;
        diff?: GraphDiff;
        hasPreviousSnapshot: boolean;
        title: string;
      }
    | { type: "saved"; name: string; diff?: GraphDiff; hasPreviousSnapshot: boolean }
    | { type: "error"; message: string };
  /** diff-specific loading */
  diffState:
    | { type: "idle" }
    | { type: "loading" }
    | { type: "rendered"; baseline: string; target: string; diff: any }
    | { type: "error"; message: string };
  error: AppError | null;
  /** Scroll offset for scan tab evidence/findings */
  scanScrollOffset: number;
  daemonStatus: DaemonStatusReadResult | null;
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
    activeTab: "timeline",
    timelineEntries: [],
    timelineCorruptEvents: [],
    timelineCursor: 0,
    timelineUndoState: { type: "idle" },
    snapshots: [],
    saveSetupState: { type: "idle" },
    diffState: { type: "idle" },
    error: null,
    scanScrollOffset: 0,
    daemonStatus: null,
  });
  const filterRequestRef = useRef(0);

  const loadTimelineForAgent = useCallback(
    async (agent: AgentId | null): Promise<{
      entries: TimelineEntry[];
      corruptEvents: TimelineCorruptEvent[];
    }> => {
      const corruptEvents: TimelineCorruptEvent[] = [];
      const entries = await listTimelineEntries(options.storeDir, {
        projectPath: options.projectPath,
        ...(agent ? { agent } : {}),
        onCorruptEntry: (event) => corruptEvents.push(event),
      });
      return { entries, corruptEvents };
    },
    [options.storeDir, options.projectPath]
  );

  // ── Boot ───────────────────────────────────────────────────
  useEffect(() => {
    (async () => {
      try {
        await ensureStore(options.storeDir);
        const scan = await scanProject(options);
        const graph = buildGraph(scan.evidence);
        const findings = auditEvidence(scan.evidence, graph);
        const firstAgent = null;
        const { readDaemonStatus } = await import("../../daemon.js");
        const daemonStatus = await readDaemonStatus(options);
        const timeline = await loadTimelineForAgent(firstAgent);
        const snapshots = await listSnapshots(options.storeDir);
        const navModel = buildTuiNavigationModel({
          evidence: scan.evidence,
          selectedItemId: INITIAL_NAV_ITEM_ID
        });

        setState((s) => ({
          ...s,
          status: "ready",
          scan,
          findings,
          selectedAgent: firstAgent,
          sidebarCursor: navModel.cursor,
          snapshots,
          saveSetupState: { type: "idle" },
          timelineEntries: timeline.entries,
          timelineCorruptEvents: timeline.corruptEvents,
          timelineCursor: 0,
          timelineUndoState: { type: "idle" },
          daemonStatus,
        }));
      } catch (err) {
        setState((s) => ({
          ...s,
          status: "error",
          error: {
            code: "HEM_INIT_FAILED",
            problem: `Initial scan failed: ${err instanceof Error ? err.message : String(err)}`,
            cause: "Could not detect agents in this project.",
            fix: "Verify the project path and try again.",
          },
        }));
      }
    })();
  }, [loadTimelineForAgent, options]);

  // ── Helpers ────────────────────────────────────────────────

  const reScan = useCallback(async () => {
    try {
      await ensureStore(options.storeDir);
      const scan = await scanProject(options);
      const graph = buildGraph(scan.evidence);
      const findings = auditEvidence(scan.evidence, graph);
      const agents = buildAgentEntries(scan.evidence);
      const { readDaemonStatus } = await import("../../daemon.js");
      const daemonStatus = await readDaemonStatus(options);

      // Re-select current agent or first
      const currentAgent = state.selectedAgent;
      const stillExists = currentAgent === null || agents.some((a) => a.id === currentAgent);
      const newAgent = stillExists ? currentAgent : null;
      const timeline = await loadTimelineForAgent(newAgent);
      const snapshots = await listSnapshots(options.storeDir);
      const navModel = buildTuiNavigationModel({
        evidence: scan.evidence,
        selectedItemId: navItemIdForSelection({
          screen: screenFromTab(state.activeTab, newAgent),
          selectedAgent: newAgent,
          selectedProfile: "default"
        }),
        cursor: state.sidebarCursor
      });

      setState((s) => ({
        ...s,
        status: "ready",
        scan,
        findings,
        selectedAgent: newAgent,
        snapshots,
        saveSetupState: { type: "idle" },
        timelineEntries: timeline.entries,
        timelineCorruptEvents: timeline.corruptEvents,
        timelineCursor: clampTimelineIndex(s.timelineCursor, timeline.entries),
        timelineUndoState: { type: "idle" },
        diffState: { type: "idle" },
        error: null,
        daemonStatus,
        sidebarCursor: navModel.cursor,
      }));
    } catch {
      // silent
    }
  }, [options, state.selectedAgent, loadTimelineForAgent]);

  const switchAgent = useCallback(
    async (agent: AgentId | null) => {
      const requestId = ++filterRequestRef.current;
      let snapshots: string[] = [];
      try {
        snapshots = await listSnapshots(options.storeDir);
      } catch {
        // silent
      }
      const timeline = await loadTimelineForAgent(agent);
      if (filterRequestRef.current !== requestId) return;
      setState((s) => ({
        ...s,
        selectedAgent: agent,
        snapshots,
        activeTab: s.activeTab,
        timelineEntries: timeline.entries,
        timelineCorruptEvents: timeline.corruptEvents,
        timelineCursor: clampTimelineIndex(s.timelineCursor, timeline.entries),
        timelineUndoState: { type: "idle" },
        diffState: { type: "idle" },
        scanScrollOffset: 0,
        sidebarCursor: Math.max(
          0,
          s.sidebarCursor < (s.scan ? buildAgentFilterEntries(s.scan.evidence).length : 0)
            ? s.sidebarCursor
            : 0
        ),
      }));
    },
    [options.storeDir, loadTimelineForAgent]
  );

  const runAction = useCallback(
    async (action: AgentAction) => {
      const agent = state.selectedAgent;
      if (!state.scan) return;

      if (action === "save-snapshot") {
        setState((s) => ({ ...s, activeTab: "snapshots", saveSetupState: { type: "loading" } }));
        try {
          const { captureCurrentState } = await import("../../current-state.js");
          const { readSnapshot } = await import("../../store.js");
          const current = await captureCurrentState(options);
          const latestSnapshotName = state.snapshots[state.snapshots.length - 1];
          let diff: GraphDiff | undefined;
          if (latestSnapshotName) {
            const latest = await readSnapshot(options.storeDir, latestSnapshotName);
            diff = diffGraphs(latest.graph, current.snapshot.graph);
          }
          const model = buildSaveSetupViewModel({
            diff,
            hasPreviousSnapshot: Boolean(latestSnapshotName)
          });
          const snapshot: Snapshot = {
            ...current.snapshot,
            manifest: {
              ...current.snapshot.manifest,
              name: model.title
            }
          };

          setState((s) => ({
            ...s,
            activeTab: "snapshots",
            saveSetupState: {
              type: "preview",
              snapshot,
              diff,
              hasPreviousSnapshot: Boolean(latestSnapshotName),
              title: model.title
            }
          }));
        } catch (err) {
          setState((s) => ({
            ...s,
            activeTab: "snapshots",
            saveSetupState: {
              type: "error",
              message: err instanceof Error ? err.message : String(err)
            }
          }));
        }
        return;
      }

      if (!agent) return;

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
              code: "HEM_SCAN_FAILED",
              problem: `Scan failed: ${err instanceof Error ? err.message : String(err)}`,
              cause: `Could not scan ${agentLabelStr(agent)} configuration.`,
              fix: "Check agent config paths and permissions.",
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
              code: "HEM_AUDIT_FAILED",
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
                code: "HEM_OP_FAILED",
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
              code: "HEM_OP_ERROR",
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

  const confirmSaveSetup = useCallback(async () => {
    if (state.saveSetupState.type !== "preview") return;
    const preview = state.saveSetupState;
    setState((s) => ({
      ...s,
      saveSetupState: {
        type: "saving",
        snapshot: preview.snapshot,
        diff: preview.diff,
        hasPreviousSnapshot: preview.hasPreviousSnapshot,
        title: preview.title
      }
    }));

    try {
      const { writeSnapshot } = await import("../../store.js");
      await writeSnapshot(options.storeDir, preview.snapshot);
      const snapshots = await listSnapshots(options.storeDir);
      setState((s) => ({
        ...s,
        snapshots,
        saveSetupState: {
          type: "saved",
          name: preview.snapshot.manifest.name,
          diff: preview.diff,
          hasPreviousSnapshot: preview.hasPreviousSnapshot
        }
      }));
    } catch (err) {
      setState((s) => ({
        ...s,
        saveSetupState: {
          type: "error",
          message: err instanceof Error ? err.message : String(err)
        }
      }));
    }
  }, [options.storeDir, state.saveSetupState]);

  // ── Keyboard ───────────────────────────────────────────────

  const handleInput = useCallback(
    (input: string, key: { upArrow?: boolean; downArrow?: boolean; leftArrow?: boolean; rightArrow?: boolean; return?: boolean; escape?: boolean; tab?: boolean; shiftTab?: boolean }) => {
      if (state.status !== "ready") return;

      const navModel = buildTuiNavigationModel({
        evidence: state.scan?.evidence ?? [],
        selectedItemId: navItemIdForSelection({
          screen: screenFromTab(state.activeTab, state.selectedAgent),
          selectedAgent: state.selectedAgent,
          selectedProfile: "default"
        }),
        cursor: state.sidebarCursor
      });
      const navItems = navModel.flatItems;
      const maxCursor = Math.max(0, navItems.length - 1);
      const scanEvidenceCount = state.scan
        ? state.selectedAgent
          ? state.scan.evidence.filter((e) => e.agent === state.selectedAgent).length
          : state.scan.evidence.length
        : 0;
      const maxScanScrollOffset = Math.max(0, scanEvidenceCount - DEFAULT_SCAN_WINDOW_SIZE);

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

      // s = save setup / confirm save setup
      if (input === "s" && state.saveSetupState.type === "preview") {
        confirmSaveSetup();
        return;
      }
      if (input === "s") {
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

      // f = cycle Timeline agent filter without leaving the Timeline tab
      if (input === "f" && state.activeTab === "timeline") {
        const nextCursor = agents.length === 0
          ? 0
          : state.sidebarCursor < agents.length - 1 ? state.sidebarCursor + 1 : 0;
        const nextAgent = agents[nextCursor]?.id ?? null;
        setState((s) => ({ ...s, sidebarCursor: nextCursor }));
        switchAgent(nextAgent);
        return;
      }

      // u = preview timeline undo (dry-run only)
      if (input === "u" && state.activeTab === "timeline") {
        const selected = state.timelineEntries[state.timelineCursor];
        if (!selected) {
          setState((s) => ({
            ...s,
            timelineUndoState: { type: "error", message: "No timeline entry selected." },
          }));
          return;
        }
        (async () => {
          try {
            const { buildTimelineUndoPlan } = await import("../../timeline-undo.js");
            const corruptEvents: TimelineCorruptEvent[] = [];
            const plan = await buildTimelineUndoPlan(options.storeDir, selected.id, {
              onCorruptEntry: (event) => corruptEvents.push(event),
            });
            setState((s) => ({
              ...s,
              ...(s.timelineEntries[s.timelineCursor]?.id === selected.id
                ? {
                    timelineCorruptEvents: corruptEvents.length > 0 ? corruptEvents : s.timelineCorruptEvents,
                    timelineUndoState: { type: "rendered", plan },
                  }
                : {}),
            }));
          } catch (err) {
            setState((s) => ({
              ...s,
              timelineUndoState: { type: "error", message: err instanceof Error ? err.message : String(err) },
            }));
          }
        })();
        return;
      }

      // --- Tab navigation: ← → or 1-5 ---
      const TAB_ORDER: TabId[] = TABS.map((tab) => tab.id);
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
      // 1-5 direct tab switch
      const numericTab = Number(input);
      if (Number.isInteger(numericTab) && numericTab >= 1 && numericTab <= TAB_ORDER.length) {
        const activeTab = TAB_ORDER[numericTab - 1];
        setState((s) => ({
          ...s,
          activeTab,
          scanScrollOffset: activeTab === "scan" ? 0 : s.scanScrollOffset,
        }));
        return;
      }

      // --- ↑↓/jk: scroll scan tab OR navigate sidebar ---
      if (key.upArrow || input === "k") {
        if (state.activeTab === "scan") {
          setState((s) => ({
            ...s,
            scanScrollOffset: Math.max(0, s.scanScrollOffset - 1),
          }));
        } else if (state.activeTab === "timeline") {
          setState((s) => ({
            ...s,
            timelineCursor: s.timelineEntries.length === 0
              ? 0
              : s.timelineCursor > 0 ? s.timelineCursor - 1 : s.timelineEntries.length - 1,
            timelineUndoState: { type: "idle" },
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
            scanScrollOffset: Math.min(maxScanScrollOffset, s.scanScrollOffset + 1),
          }));
        } else if (state.activeTab === "timeline") {
          setState((s) => ({
            ...s,
            timelineCursor: s.timelineEntries.length === 0
              ? 0
              : s.timelineCursor < s.timelineEntries.length - 1 ? s.timelineCursor + 1 : 0,
            timelineUndoState: { type: "idle" },
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
        const item = navItems[state.sidebarCursor];
        if (!item) return;
        const selection = selectTuiNavItem({
          item,
          currentScreen: screenFromTab(state.activeTab, state.selectedAgent),
          currentAgent: state.selectedAgent,
          currentProfile: "default"
        });

        if (selection.screen === "timeline") {
          setState((s) => ({ ...s, activeTab: "timeline" }));
          switchAgent(selection.selectedAgent);
        } else if (selection.screen === "snapshots") {
          setState((s) => ({
            ...s,
            activeTab: "snapshots",
            selectedAgent: null,
            snapshots: [],
            diffState: { type: "idle" }
          }));
        } else if (selection.screen === "agent-detail" && selection.selectedAgent) {
          setState((s) => ({ ...s, activeTab: "scan" }));
          switchAgent(selection.selectedAgent);
        } else {
          setState((s) => ({
            ...s,
            activeTab: "timeline",
            selectedAgent: null,
            timelineUndoState: { type: "idle" },
            diffState: { type: "idle" }
          }));
        }
        return;
      }
    },
    [state, reScan, switchAgent, runAction, confirmSaveSetup]
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
  const agents = state.scan ? buildAgentFilterEntries(state.scan.evidence) : [];
  const selectedNavItemId = navItemIdForSelection({
    screen: screenFromTab(state.activeTab, state.selectedAgent),
    selectedAgent: state.selectedAgent,
    selectedProfile: "default"
  });
  const navModel = buildTuiNavigationModel({
    evidence: state.scan?.evidence ?? [],
    selectedItemId: selectedNavItemId,
    cursor: state.sidebarCursor
  });

  return (
    <Box flexDirection="row" width="100%">
      {/* ── Sidebar ── */}
      <Sidebar
        agents={agents}
        selectedAgent={state.selectedAgent}
        cursor={state.sidebarCursor}
        navSections={navModel.sections}
        selectedItemId={navModel.selectedItemId}
      />

      {/* ── Main panel ── */}
      <Box flexDirection="column" paddingX={1} flexGrow={1}>
        <DaemonTrustHeader status={state.daemonStatus} />

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

          {state.activeTab === "timeline" && renderTimeline()}
          {state.activeTab === "snapshots" && renderSnapshots()}
          {state.activeTab === "scan" && renderScan()}
          {state.activeTab === "audit" && renderAudit()}
          {state.activeTab === "diff" && state.selectedAgent && renderDiff()}
        </Box>

        {/* Footer hint */}
        <Box marginTop={1}>
          <Text dimColor>
            {state.activeTab === "scan"
              ? "↑↓ scroll  ←→ tab  1-5 jump  s=save  d=diff  a=audit  r=rescan  q=quit"
              : state.activeTab === "timeline"
                ? "↑↓ timeline  f=filter  u=preview undo  ←→ tab  1-5 jump  r=rescan  q=quit"
                : "↑↓ agent  ←→ tab  1-5 jump  s=save  d=diff  a=audit  r=rescan  q=quit"}
          </Text>
        </Box>
      </Box>
    </Box>
  );

  // ── Render helpers ────────────────────────────

  function renderTimeline() {
    return (
      <TimelineView
        entries={state.timelineEntries}
        selectedIndex={state.timelineCursor}
        agentFilter={state.selectedAgent}
        corruptEvents={state.timelineCorruptEvents}
        undoPlan={state.timelineUndoState.type === "rendered" ? state.timelineUndoState.plan : null}
        undoError={state.timelineUndoState.type === "error" ? state.timelineUndoState.message : null}
      />
    );
  }

  function renderSnapshots() {
    if (state.saveSetupState.type === "loading") {
      return (
        <Box>
          <Spinner type="dots" />
          <Text> Preparing save setup...</Text>
        </Box>
      );
    }

    if (state.saveSetupState.type === "preview" || state.saveSetupState.type === "saving") {
      return (
        <SaveSetupView
          diff={state.saveSetupState.diff}
          hasPreviousSnapshot={state.saveSetupState.hasPreviousSnapshot}
          saving={state.saveSetupState.type === "saving"}
        />
      );
    }

    if (state.saveSetupState.type === "saved") {
      return (
        <SaveSetupView
          diff={state.saveSetupState.diff}
          hasPreviousSnapshot={state.saveSetupState.hasPreviousSnapshot}
          savedName={state.saveSetupState.name}
        />
      );
    }

    if (state.saveSetupState.type === "error") {
      return (
        <SaveSetupView
          hasPreviousSnapshot={state.snapshots.length > 0}
          error={state.saveSetupState.message}
        />
      );
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

    return (
      <AgentDetailView
        agent={state.selectedAgent}
        evidence={state.scan.evidence}
        timelineEntries={state.timelineEntries}
      />
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

export function DaemonTrustHeader({ status }: { status: DaemonStatusReadResult | null }) {
  const model = daemonTrustHeaderModel(status);
  if (!status) {
    return (
      <Box marginBottom={1}>
        <Text dimColor>{model.title}</Text>
      </Box>
    );
  }

  return (
    <Box flexDirection="column" marginBottom={1}>
      <Text color={model.color}>
        {model.title}
        <Text dimColor>
          {"  "}last event: {model.lastEvent}
          {"  "}watched: {model.watchedCount}
          {"  "}store: {model.storeDir}
        </Text>
      </Text>
      {model.error && (
        <Text color="red">
          {model.error}
        </Text>
      )}
      {model.stale && (
        <Text color="yellow">events may be stale</Text>
      )}
    </Box>
  );
}

function screenFromTab(activeTab: TabId, selectedAgent: AgentId | null): TuiScreenId {
  if (activeTab === "timeline") return "timeline";
  if (activeTab === "snapshots") return "snapshots";
  if (selectedAgent) return "agent-detail";
  return "timeline";
}

export function daemonTrustHeaderModel(status: DaemonStatusReadResult | null): {
  title: string;
  color: "green" | "yellow" | "red";
  lastEvent: string;
  watchedCount: number;
  storeDir: string;
  stale: boolean;
  error?: string;
} {
  if (!status) {
    return {
      title: "Daemon: checking...",
      color: "yellow",
      lastEvent: "-",
      watchedCount: 0,
      storeDir: "-",
      stale: false
    };
  }

  const daemon = status.status;
  const label = !status.ok
    ? "error"
    : daemon.running
      ? daemon.stale ? "stale" : "running"
      : daemon.stale ? "stale" : "stopped";

  return {
    title: `Daemon: ${label}`,
    color: label === "running" ? "green" : label === "stopped" ? "yellow" : "red",
    lastEvent: daemon.lastEventAt ?? "-",
    watchedCount: daemon.watchedPaths.length,
    storeDir: daemon.storeDir,
    stale: daemon.stale,
    ...((!status.ok || daemon.errors.length > 0)
      ? { error: status.ok ? daemon.errors[0] : status.error }
      : {})
  };
}
