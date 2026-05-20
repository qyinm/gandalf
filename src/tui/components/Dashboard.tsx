/**
 * Agent-centric TUI Dashboard for snaptailor.
 *
 * ── First screen: agent selection ──
 *   Shows each detected agent with snapshot count and scan status.
 *   User picks an agent to drill into.
 *
 * ── Second screen: agent detail ──
 *   Shows current state summary, saved snapshots, action buttons.
 *   User picks an action (scan, snapshot, diff, audit, etc.).
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

import ScanView from "./ScanView.js";
import AuditView from "./AuditView.js";
import DiffView from "./DiffView.js";
import SimpleTable from "./SimpleTable.js";
import ErrorPage from "./ErrorPage.js";

// ── Agent info ───────────────────────────────────────────────

const ALL_AGENTS: AgentId[] = [
  "claude-code",
  "codex",
  "cursor",
  "opencode",
  "pi-agent",
  "project",
];

function agentLabel(id: AgentId): string {
  const map: Record<string, string> = {
    "claude-code": "Claude Code",
    codex: "Codex",
    cursor: "Cursor",
    opencode: "OpenCode",
    "pi-agent": "Pi Agent",
    project: "Project",
  };
  return map[id] ?? id;
}

// ── Screen types ─────────────────────────────────────────────

type Screen =
  | { type: "init" }
  | { type: "agent-list"; scan: ScanResult; findings: AuditFinding[] }
  | { type: "agent-detail"; agent: AgentId; agentScan: ScanResult; agentFindings: AuditFinding[]; snapshots: string[] }
  | { type: "loading"; message: string }
  | { type: "scan-result"; agent: AgentId; scan: ScanResult; findings: AuditFinding[]; from: "list" | "detail" }
  | { type: "audit-result"; findings: AuditFinding[] }
  | { type: "diff-result"; baseline: string; target: string; agent: AgentId }
  | { type: "error"; code: string; problem: string; cause: string; fix: string; path?: string };

// ── Action IDs for agent detail screen ───────────────────────

type AgentAction =
  | "scan"
  | "save-snapshot"
  | "diff"
  | "audit"
  | "bundle-export"
  | "bundle-import"
  | "restore";

interface ActionEntry {
  id: AgentAction;
  label: string;
  description: string;
}

const AGENT_ACTIONS: ActionEntry[] = [
  { id: "scan", label: "Scan Now", description: "Scan this agent's config" },
  { id: "save-snapshot", label: "Save Snapshot", description: "Scan and save as snapshot" },
  { id: "diff", label: "Diff", description: "Compare two of this agent's snapshots" },
  { id: "audit", label: "Audit", description: "Run audit rules on current state" },
  { id: "bundle-export", label: "Export Bundle", description: "Export snapshot to .stailor" },
  { id: "bundle-import", label: "Import Bundle", description: "Import .stailor bundle" },
  { id: "restore", label: "Restore", description: "Restore from snapshot" },
];

// ── Dashboard ────────────────────────────────────────────────

interface DashboardProps {
  options: RuntimeOptions;
}

export default function Dashboard({ options }: DashboardProps) {
  const [cursor, setCursor] = useState(0);
  const [screen, setScreen] = useState<Screen>({ type: "init" });

  // ── Boot: initial scan to detect agents ───────────────────
  useEffect(() => {
    (async () => {
      try {
        await ensureStore(options.storeDir);
        const scan = await scanProject(options);
        const graph = buildGraph(scan.evidence);
        const findings = auditEvidence(scan.evidence, graph);
        setScreen({ type: "agent-list", scan, findings });
      } catch (err) {
        setScreen({
          type: "error",
          code: "SNAPTAILOR_INIT_FAILED",
          problem: `Initial scan failed: ${err instanceof Error ? err.message : String(err)}`,
          cause: "Could not detect agents in this project.",
          fix: "Verify the project path and try again.",
        });
      }
    })();
  }, []);

  // ── Keyboard handler ──────────────────────────────────────
  const handleInput = useCallback(
    (input: string, key: { upArrow?: boolean; downArrow?: boolean; return?: boolean; escape?: boolean }) => {
      if (screen.type === "init") return;

      // -- Scan result screen: s = save snapshot, q = back --
      if (screen.type === "scan-result") {
        if (input === "q" || key.escape) {
          if (screen.from === "list") { reScan(); } else { openAgentDetail(screen.agent, screen.scan, screen.findings); }
        } else if (input === "s") {
          // Save snapshot from scan result
          executeAgentAction("save-snapshot", screen.agent);
        }
        return;
      }

      // -- Non-agent-list, non-agent-detail screens: q/Esc goes back --
      if (screen.type !== "agent-list" && screen.type !== "agent-detail") {
        if (input === "q" || key.escape) { reScan(); }
        return;
      }

      // -- Agent detail screen: navigate actions --
      if (screen.type === "agent-detail") {
        if (input === "q" || key.escape) { reScan(); return; }
        if (key.upArrow || input === "k") {
          setCursor((c) => (c > 0 ? c - 1 : AGENT_ACTIONS.length - 1));
          return;
        }
        if (key.downArrow || input === "j") {
          setCursor((c) => (c < AGENT_ACTIONS.length - 1 ? c + 1 : 0));
          return;
        }
        if (key.return) {
          executeAgentAction(AGENT_ACTIONS[cursor].id, screen.agent);
        }
        return;
      }

      // -- Agent list screen: navigate + select --
      const agentList = detectedAgents(screen);
      // +1 for "Full Scan" option
      const maxIndex = agentList.length;

      if (key.escape || input === "q") { process.exit(0); return; }
      if (key.upArrow || input === "k") { setCursor((c) => (c > 0 ? c - 1 : maxIndex)); return; }
      if (key.downArrow || input === "j") { setCursor((c) => (c < maxIndex ? c + 1 : 0)); return; }
      if (!key.return) return;

      if (cursor === maxIndex) {
        // "Full scan" → show all agents scan result
        setScreen({ type: "scan-result", agent: "unknown" as AgentId, scan: screen.scan, findings: screen.findings, from: "list" });
        return;
      }
      openAgentDetail(agentList[cursor], screen.scan, screen.findings);
    },
    [cursor, screen]
  );

  useInput(handleInput);

  // ── Re-scan helper ────────────────────────────────────────
  async function reScan() {
    try {
      await ensureStore(options.storeDir);
      const scan = await scanProject(options);
      const graph = buildGraph(scan.evidence);
      const findings = auditEvidence(scan.evidence, graph);
      setScreen({ type: "agent-list", scan, findings });
    } catch { /* silent */ }
  }

  // ── Open agent detail ─────────────────────────────────────
  async function openAgentDetail(agent: AgentId, scan: ScanResult, findings: AuditFinding[]) {
    try {
      const agentEvidence = scan.evidence.filter((e) => e.agent === agent);
    const agentGraph = buildGraph(agentEvidence);
    const agentFindings = auditEvidence(agentEvidence, agentGraph);
    const snapshots = await listSnapshots(options.storeDir, agent);
    setCursor(0);
    setScreen({
      type: "agent-detail",
      agent,
      agentScan: { ...scan, evidence: agentEvidence },
      agentFindings,
      snapshots,
    });
  } catch (err) {
      setScreen({
        type: "error", code: "SNAPTAILOR_DETAIL_FAILED",
        problem: `Could not open agent detail: ${err instanceof Error ? err.message : String(err)}`,
        cause: `Failed to load ${agentLabel(agent)} details.`,
        fix: "Check store directory integrity.",
      });
    }
  }

  // ── Execute agent action ──────────────────────────────────
  async function executeAgentAction(action: AgentAction, agent: AgentId) {
    if (action === "scan") {
      setScreen({ type: "loading", message: `Scanning ${agentLabel(agent)}...` });
      try {
        const agentOpts = { ...options, agent };
        const scan = await scanProject(agentOpts);
        const graph = buildGraph(scan.evidence);
        const findings = auditEvidence(scan.evidence, graph);
        setScreen({ type: "scan-result", agent, scan, findings, from: "detail" });
      } catch (err) {
        setScreen({
          type: "error",
          code: "SNAPTAILOR_SCAN_FAILED",
          problem: `Scan failed: ${err instanceof Error ? err.message : String(err)}`,
          cause: `Could not scan ${agentLabel(agent)} configuration.`,
          fix: "Check agent config paths and permissions.",
        });
      }
    } else if (action === "save-snapshot") {
      setScreen({ type: "loading", message: `Scanning ${agentLabel(agent)}...` });
      try {
        const agentOpts = { ...options, agent };
        const scan = await scanProject(agentOpts);
        const graph = buildGraph(scan.evidence);
        const findings = auditEvidence(scan.evidence, graph);
        const provenance = (await import("../../provenance.js")).buildProvenance(graph, scan.evidence);
        const { writeSnapshot } = await import("../../store.js");
        const snapshotName = `${agent}-${Date.now()}`;
        await writeSnapshot(options.storeDir, {
          manifest: {
            schemaVersion: "0.1",
            name: snapshotName,
            createdAt: new Date().toISOString(),
            projectPath: options.projectPath,
            security: { rawSecretsIncluded: false, redactionPolicy: "metadata-only" },
          },
          evidence: scan.evidence,
          graph,
          auditFindings: findings,
          provenance,
        }, agent);
        setScreen({ type: "scan-result", agent, scan, findings, from: "detail" });
      } catch (err) {
        setScreen({
          type: "error", code: "SNAPTAILOR_SNAPSHOT_FAILED",
          problem: `Snapshot failed: ${err instanceof Error ? err.message : String(err)}`,
          cause: "Could not save snapshot.", fix: "Check store permissions.",
        });
      }
    } else if (action === "audit") {
      setScreen({ type: "loading", message: `Auditing ${agentLabel(agent)}...` });
      try {
        const agentOpts = { ...options, agent };
        const scan = await scanProject(agentOpts);
        const graph = buildGraph(scan.evidence);
        const findings = auditEvidence(scan.evidence, graph);
        setScreen({ type: "audit-result", findings });
      } catch (err) {
        setScreen({
          type: "error", code: "SNAPTAILOR_AUDIT_FAILED",
          problem: `Audit failed: ${err instanceof Error ? err.message : String(err)}`,
          cause: `Could not audit ${agentLabel(agent)}.`, fix: "Check agent config paths.",
        });
      }
    } else if (action === "diff") {
      try {
        const snapshots = await listSnapshots(options.storeDir, agent);
      if (snapshots.length < 2) {
        setScreen({
          type: "error", code: "SNAPTAILOR_DIFF_NO_SNAPSHOTS",
          problem: `Need at least 2 snapshots for ${agentLabel(agent)}.`,
          cause: "Diff requires baseline and target snapshots.",
          fix: `Save snapshots first for ${agentLabel(agent)}.`,
        });
        return;
      }
      setScreen({ type: "diff-result", baseline: snapshots[snapshots.length - 2], target: snapshots[snapshots.length - 1], agent });
      } catch (err) {
        setScreen({ type: "error", code: "SNAPTAILOR_DIFF_FAILED",
          problem: `Diff failed: ${err instanceof Error ? err.message : String(err)}`,
          cause: `Could not list snapshots for ${agentLabel(agent)}.`,
          fix: "Check store directory." });
      }
    } else if (action === "bundle-export" || action === "bundle-import" || action === "restore") {
      // Delegate to Clack wizards
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
          setScreen({ type: "error", code: "SNAPTAILOR_OP_FAILED",
            problem: `${action} did not complete.`, cause: "Cancelled or error.", fix: "Try again." });
          return;
        }
      } catch (err) {
        setScreen({ type: "error", code: "SNAPTAILOR_OP_ERROR",
          problem: `${action} error: ${err instanceof Error ? err.message : String(err)}`,
          cause: "Unexpected error.", fix: "Check logs and retry." });
        return;
      }
      // Back to agent detail after wizard
      await openAgentDetail(agent, screen.type === "agent-list" ? screen.scan : { evidence: [], blindSpots: [], trust: { readOnly: true, network: "disabled", commandsExecuted: [], storeWriteLocation: "" } }, []);
    }
  }

  // ── Render ────────────────────────────────────────────────
  switch (screen.type) {
    case "init":
      return renderBoot();
    case "agent-list":
      return renderAgentList(screen, cursor);
    case "agent-detail":
      return renderAgentDetail(screen, cursor);
    case "loading":
      return renderLoading(screen.message);
    case "scan-result":
      return (
        <Box flexDirection="column">
          <Text dimColor>{agentLabel(screen.agent)} scan</Text>
          <ScanView evidence={screen.scan.evidence} auditFindings={screen.findings}
            blindSpots={screen.scan.blindSpots} readOnly={screen.scan.trust.readOnly} />
          <Box marginTop={1}><Text dimColor>Press s to save as snapshot, q for agent</Text></Box>
        </Box>
      );
    case "audit-result":
      return (
        <Box flexDirection="column">
          <AuditView findings={screen.findings} />
          <Box marginTop={1}><Text dimColor>Press q to go back</Text></Box>
        </Box>
      );
    case "diff-result":
      return <DiffViewInline {...screen} options={options} />;
    case "error":
      return (
        <Box flexDirection="column">
          <ErrorPage code={screen.code} problem={screen.problem}
            cause={screen.cause} fix={screen.fix} path={screen.path} />
          <Box marginTop={1}><Text dimColor>Press q to go back</Text></Box>
        </Box>
      );
  }
}

// ── Detected agents from scan ────────────────────────────────

function detectedAgents(screen: { scan: ScanResult }) {
  const agents = new Set(screen.scan.evidence.map((e) => e.agent));
  return ALL_AGENTS.filter((a) => agents.has(a));
}

// ── Boot screen ──────────────────────────────────────────────

function renderBoot() {
  return (
    <Box>
      <Spinner type="dots" />
      <Text> Detecting agents...</Text>
    </Box>
  );
}

// ── Agent list screen ────────────────────────────────────────

function renderAgentList(screen: { scan: ScanResult; findings: AuditFinding[] }, cursor: number) {
  const agents = detectedAgents(screen);
  const items = agents.map((a) => ({
    agent: a,
    evidence: screen.scan.evidence.filter((e) => e.agent === a).length,
  }));

  return (
    <Box flexDirection="column" paddingX={1}>
      <Box marginBottom={1}>
        <Text bold color="cyan">snaptailor — 내 AI 에이전트 설정</Text>
      </Box>
      <Box marginBottom={1}>
        <Text dimColor>{optionsLabel(screen.scan)}</Text>
      </Box>
      <Box flexDirection="column">
        {items.map((item, i) => (
          <Text key={item.agent} bold={i === cursor} color={i === cursor ? "cyan" : undefined}>
            {i === cursor ? "▸ " : "  "}
            {agentLabel(item.agent).padEnd(14)}
            <Text dimColor>{item.evidence} config items</Text>
          </Text>
        ))}
        {/* Full scan option */}
        <Text bold={cursor === items.length} color={cursor === items.length ? "cyan" : undefined}>
          {cursor === items.length ? "▸ " : "  "}
          {"Full Scan".padEnd(14)}
          <Text dimColor>scan all agents</Text>
        </Text>
      </Box>
      <Box marginTop={1}>
        <Text dimColor>↑↓ / jk  Enter  q=quit</Text>
      </Box>
    </Box>
  );
}

// ── Agent detail screen ──────────────────────────────────────

function renderAgentDetail(screen: { agent: AgentId; agentScan: ScanResult; agentFindings: AuditFinding[]; snapshots: string[] }, cursor: number) {
  const scan = screen.agentScan;
  const findingHigh = screen.agentFindings.filter((f) => f.severity === "high" || f.severity === "critical").length;

  return (
    <Box flexDirection="column" paddingX={1}>
      <Box marginBottom={1}>
        <Text bold color="cyan">{agentLabel(screen.agent)}</Text>
      </Box>

      {/* Current state summary */}
      <Box marginBottom={1} flexDirection="column">
        <Text bold>현재 상태</Text>
        <Text dimColor>  config 항목: {scan.evidence.length}</Text>
        {findingHigh > 0 && <Text color="red">  high-signal findings: {findingHigh}</Text>}
        <Text dimColor>  read-only: {scan.trust.readOnly ? "yes" : "no"}</Text>
      </Box>

      {/* Snapshots */}
      <Box marginBottom={1} flexDirection="column">
        <Text bold>스냅샷 ({screen.snapshots.length})</Text>
        {screen.snapshots.length === 0 && <Text dimColor>  없음 — 먼저 Save Snapshot 해주세요</Text>}
        {screen.snapshots.slice(-5).map((s) => (
          <Text key={s} dimColor>  ● {s}</Text>
        ))}
      </Box>

      {/* Actions */}
      <Box flexDirection="column">
        <Text bold>액션</Text>
        {AGENT_ACTIONS.map((act, i) => (
          <Text key={act.id} bold={i === cursor} color={i === cursor ? "cyan" : undefined}>
            {i === cursor ? "▸ " : "  "}
            {act.label.padEnd(16)}
            <Text dimColor>{act.description}</Text>
          </Text>
        ))}
      </Box>
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

// ── Diff inline ──────────────────────────────────────────────

function DiffViewInline({ baseline, target, agent, options }: { baseline: string; target: string; agent: AgentId; options: RuntimeOptions }) {
  const [state, setState] = useState<{ type: "loading" | "result" | "error"; diff?: any; error?: string }>({ type: "loading" });

  useEffect(() => {
    (async () => {
      try {
        const { readSnapshot } = await import("../../store.js");
        const before = await readSnapshot(options.storeDir, baseline, agent);
        const after = await readSnapshot(options.storeDir, target, agent);
        setState({ type: "result", diff: diffGraphs(before.graph, after.graph) });
      } catch (err) {
        setState({ type: "error", error: err instanceof Error ? err.message : String(err) });
      }
    })();
  }, []);

  if (state.type === "loading") return <Box><Spinner type="dots" /><Text> Diffing {baseline} → {target}...</Text></Box>;
  if (state.type === "error") return <Box flexDirection="column"><ErrorPage code="DIFF_ERROR" problem={state.error ?? "Unknown"} cause="Failed to load diff." fix="Verify snapshots exist." /><Box marginTop={1}><Text dimColor>Press q to go back</Text></Box></Box>;
  return <Box flexDirection="column"><Text dimColor>{agentLabel(agent)}: {baseline} → {target}</Text><DiffView semanticChanges={state.diff.semanticChanges} rawSourceChanges={state.diff.rawSourceChanges} /><Box marginTop={1}><Text dimColor>Press q to go back</Text></Box></Box>;
}

function optionsLabel(scan: ScanResult) {
  return `${scan.evidence.length} items found, ${new Set(scan.evidence.map(e => e.agent)).size} agents`;
}