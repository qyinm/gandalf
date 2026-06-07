/**
 * Dashboard — sidebar-driven workspace layout for hem TUI.
 *
 *  ┌──────────────┬────────────────────────────────────────────┐
 *  │  Agents       │  Daemon: running/stopped/stale/error      │
 *  │  ──────────   │  ────────────────────────────────────────  │
 *  │  ▸ Claude Cd  │  (content based on sidebar selection)      │
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
import type { TabId } from "./TabBar.js";
import ScanView, { DEFAULT_SCAN_WINDOW_SIZE } from "./ScanView.js";
import AuditView from "./AuditView.js";
import SnapshotList from "./SnapshotList.js";
import SaveSetupView from "./SaveSetupView.js";
import { buildSaveSetupViewModel } from "./SaveSetupViewModel.js";
import CompareView from "./CompareView.js";
import { latestSnapshotByCreatedAt } from "./CompareViewModel.js";
import ErrorPage from "./ErrorPage.js";
import TimelineView from "./TimelineView.js";
import { clampTimelineIndex } from "./TimelineViewModel.js";
import AgentDetailView from "./AgentDetailView.js";
import ProfileView from "./ProfileView.js";
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
  profileScreenOpen: boolean;
  previousView: DashboardView | null;
  notice: string | null;
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
    | { type: "rendered"; baseline: string; target: string; before: Snapshot; after: Snapshot; diff: GraphDiff }
    | { type: "error"; message: string };
  error: AppError | null;
  /** Scroll offset for scan tab evidence/findings */
  scanScrollOffset: number;
  daemonStatus: DaemonStatusReadResult | null;
};

type DashboardView = {
  activeTab: TabId;
  selectedAgent: AgentId | null;
  profileScreenOpen: boolean;
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
    profileScreenOpen: false,
    previousView: null,
    notice: null,
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
          profileScreenOpen: false,
          previousView: null,
          notice: null,
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
          screen: state.profileScreenOpen ? "profile" : screenFromTab(state.activeTab, newAgent),
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
        profileScreenOpen: false,
        previousView: null,
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
  }, [options, state.activeTab, state.profileScreenOpen, state.selectedAgent, state.sidebarCursor, loadTimelineForAgent]);

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
        profileScreenOpen: false,
        previousView: null,
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

  const openProfile = useCallback(async () => {
    const requestId = ++filterRequestRef.current;
    let snapshots: string[] = [];
    try {
      snapshots = await listSnapshots(options.storeDir);
    } catch {
      // silent
    }
    const timeline = await loadTimelineForAgent(null);
    if (filterRequestRef.current !== requestId) return;
    setState((s) => ({
      ...s,
      activeTab: "timeline",
      selectedAgent: null,
      profileScreenOpen: true,
      previousView: captureDashboardView(s),
      notice: null,
      snapshots,
      timelineEntries: timeline.entries,
      timelineCorruptEvents: timeline.corruptEvents,
      timelineCursor: clampTimelineIndex(s.timelineCursor, timeline.entries),
      timelineUndoState: { type: "idle" },
      diffState: { type: "idle" }
    }));
  }, [options.storeDir, loadTimelineForAgent]);

  const openSnapshots = useCallback(async () => {
    const requestId = ++filterRequestRef.current;
    let snapshots: string[] = [];
    try {
      snapshots = await listSnapshots(options.storeDir);
    } catch {
      // silent
    }
    const timeline = await loadTimelineForAgent(null);
    if (filterRequestRef.current !== requestId) return;
    setState((s) => ({
      ...s,
      activeTab: "snapshots",
      profileScreenOpen: false,
      selectedAgent: null,
      previousView: null,
      notice: null,
      snapshots,
      timelineEntries: timeline.entries,
      timelineCorruptEvents: timeline.corruptEvents,
      timelineCursor: clampTimelineIndex(s.timelineCursor, timeline.entries),
      timelineUndoState: { type: "idle" },
      diffState: { type: "idle" }
    }));
  }, [options.storeDir, loadTimelineForAgent]);

  const runAction = useCallback(
    async (action: AgentAction) => {
      const agent = state.selectedAgent;
      if (!state.scan) return;

      if (action === "save-snapshot") {
        setState((s) => ({
          ...s,
          activeTab: "snapshots",
          profileScreenOpen: false,
          previousView: captureDashboardView(s),
          notice: null,
          saveSetupState: { type: "loading" }
        }));
        try {
          const { captureCurrentState } = await import("../../current-state.js");
          const { readSnapshot } = await import("../../store.js");
          const current = await captureCurrentState(options);
          const snapshotNames = await listSnapshots(options.storeDir);
          const savedSnapshots = await Promise.all(
            snapshotNames.map((name) => readSnapshot(options.storeDir, name))
          );
          const latest = latestSnapshotByCreatedAt(savedSnapshots);
          let diff: GraphDiff | undefined;
          if (latest) {
            diff = diffGraphs(latest.graph, current.snapshot.graph);
          }
          const model = buildSaveSetupViewModel({
            diff,
            hasPreviousSnapshot: Boolean(latest)
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
            profileScreenOpen: false,
            notice: null,
            saveSetupState: {
              type: "preview",
              snapshot,
              diff,
              hasPreviousSnapshot: Boolean(latest),
              title: model.title
            }
          }));
        } catch (err) {
          setState((s) => ({
            ...s,
            activeTab: "snapshots",
            profileScreenOpen: false,
            notice: null,
            saveSetupState: {
              type: "error",
              message: err instanceof Error ? err.message : String(err)
            }
          }));
        }
        return;
      }

      if (action === "diff") {
        setState((s) => ({
          ...s,
          diffState: { type: "loading" },
          activeTab: "diff",
          profileScreenOpen: false,
          previousView: captureDashboardView(s),
          notice: null
        }));
        try {
          const { captureCurrentState } = await import("../../current-state.js");
          const { readSnapshot } = await import("../../store.js");
          const snapshotNames = await listSnapshots(options.storeDir);
          const savedSnapshots = await Promise.all(
            snapshotNames.map((name) => readSnapshot(options.storeDir, name))
          );
          const before = latestSnapshotByCreatedAt(savedSnapshots);
          if (!before) {
            setState((s) => ({
              ...s,
              diffState: {
                type: "error",
                message: "No saved setups yet."
              }
            }));
            return;
          }

          const current = await captureCurrentState(options);
          const diff = diffGraphs(before.graph, current.snapshot.graph);
          setState((s) => ({
            ...s,
            diffState: {
              type: "rendered",
              baseline: before.manifest.name,
              target: "Current",
              before,
              after: current.snapshot,
              diff
            }
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
          screen: state.profileScreenOpen ? "profile" : screenFromTab(state.activeTab, state.selectedAgent),
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

      if (key.escape) {
        if (state.previousView) {
          setState((s) => ({
            ...s,
            activeTab: s.previousView!.activeTab,
            selectedAgent: s.previousView!.selectedAgent,
            profileScreenOpen: s.previousView!.profileScreenOpen,
            previousView: null,
            saveSetupState: { type: "idle" },
            diffState: { type: "idle" },
            notice: null
          }));
          return;
        }
        setState((s) => ({
          ...s,
          saveSetupState: { type: "idle" },
          diffState: s.activeTab === "diff" ? { type: "idle" } : s.diffState,
          activeTab: s.activeTab === "diff" ? "timeline" : s.activeTab,
          profileScreenOpen: false,
          notice: null
        }));
        return;
      }

      // q = quit
      if (input === "q") {
        process.exit(0);
        return;
      }

      // r = re-scan
      if (input === "r") {
        setState((s) => ({ ...s, notice: null }));
        reScan();
        return;
      }

      if (input === "p") {
        openProfile();
        return;
      }

      if (input === "/") {
        setState((s) => ({ ...s, notice: "Search is reserved for a future TUI search model." }));
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

      // c = compare latest saved full setup with current setup
      if (input === "c") {
        runAction("diff");
        return;
      }

      // a = audit (only if agent selected)
      if (input === "a" && state.selectedAgent) {
        runAction("audit");
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

      // --- ↑↓/jk: move the persistent left navigation selection ---
      if (key.upArrow || input === "k") {
        setState((s) => ({
          ...s,
          sidebarCursor: s.sidebarCursor > 0 ? s.sidebarCursor - 1 : maxCursor,
        }));
        return;
      }
      if (key.downArrow || input === "j") {
        setState((s) => ({
          ...s,
          sidebarCursor: s.sidebarCursor < maxCursor ? s.sidebarCursor + 1 : 0,
        }));
        return;
      }

      // --- ←→/hl: move within the current content panel when it has local selection ---
      if (key.leftArrow || input === "h") {
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
        }
        return;
      }
      if (key.rightArrow || input === "l") {
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
          setState((s) => ({ ...s, activeTab: "timeline", profileScreenOpen: false, previousView: null, notice: null }));
          switchAgent(selection.selectedAgent);
        } else if (selection.screen === "snapshots") {
          openSnapshots();
        } else if (selection.screen === "agent-detail" && selection.selectedAgent) {
          setState((s) => ({ ...s, activeTab: "scan", profileScreenOpen: false, previousView: null, notice: null }));
          switchAgent(selection.selectedAgent);
        } else {
          if (selection.screen === "profile") {
            openProfile();
            return;
          }
          setState((s) => ({
            ...s,
            activeTab: "timeline",
            selectedAgent: null,
            profileScreenOpen: selection.screen === "profile",
            previousView: selection.screen === "profile" ? captureDashboardView(s) : null,
            timelineUndoState: { type: "idle" },
            diffState: { type: "idle" },
            notice: null
          }));
        }
        return;
      }
    },
    [state, reScan, switchAgent, openProfile, openSnapshots, runAction, confirmSaveSetup]
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
    screen: state.profileScreenOpen ? "profile" : screenFromTab(state.activeTab, state.selectedAgent),
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

          {state.notice && (
            <Box marginTop={1}>
              <Text color="yellow">{state.notice}</Text>
            </Box>
          )}

          {state.profileScreenOpen && renderProfile()}
          {!state.profileScreenOpen && state.activeTab === "timeline" && renderTimeline()}
          {!state.profileScreenOpen && state.activeTab === "snapshots" && renderSnapshots()}
          {!state.profileScreenOpen && state.activeTab === "scan" && renderScan()}
          {!state.profileScreenOpen && state.activeTab === "audit" && renderAudit()}
          {!state.profileScreenOpen && state.activeTab === "diff" && renderDiff()}
        </Box>

        {/* Footer hint */}
        <Box marginTop={1}>
          <Text dimColor>
            {state.activeTab === "scan"
              ? "↑↓ move  Enter open  ←→ scroll  s=save  c=compare  a=audit  r=rescan  q=quit"
              : state.activeTab === "timeline"
                ? "↑↓ move  Enter open  ←→ timeline  u=preview undo  c=compare  r=rescan  q=quit"
                : "↑↓ move  Enter open  s=save  c=compare  r=rescan  q=quit"}
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

  function renderProfile() {
    if (!state.scan) return <Text dimColor>No scan data.</Text>;

    return (
      <ProfileView
        evidence={state.scan.evidence}
        snapshotNames={state.snapshots}
        timelineEntries={state.timelineEntries}
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
          <Text color="cyan">[s] Save Setup</Text>
          <Text color="cyan">[c] Compare</Text>
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
      return (
        <Box flexDirection="column">
          <Text dimColor>Press [c] to compare the latest saved setup with current setup.</Text>
          {state.snapshots.length >= 1 && (
            <Box marginTop={1} flexDirection="column">
              <Text dimColor>From: {state.snapshots[state.snapshots.length - 1]}</Text>
              <Text dimColor>To:   Current</Text>
            </Box>
          )}
          <Box marginTop={1}>
            <Text color="cyan">[c] Compare</Text>
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
          fix="Save your current setup first."
        />
      );
    }

    return (
      <CompareView
        fromSnapshot={ds.before}
        toSnapshot={ds.after}
        diff={ds.diff}
        toLabel="Current  unsaved changes"
      />
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

function captureDashboardView(state: DashboardState): DashboardView {
  return {
    activeTab: state.activeTab,
    selectedAgent: state.selectedAgent,
    profileScreenOpen: state.profileScreenOpen
  };
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
