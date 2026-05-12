import { randomUUID } from "node:crypto";
import path from "node:path";
import { scanProject } from "./scan.js";
import { readSnapshot } from "./store.js";
import {
  buildGraph
} from "./graph.js";
import type {
  AgentId,
  DiscoveredItem,
  EvidenceKind,
  RestoreAction,
  RestorePlan,
  RestorePlanItem,
  RestoreOptions,
  RiskSummary,
  Severity,
  UnsupportedPlanItem
} from "./types.js";

// ── Kind priority for deterministic same-level ordering ──────────

const KIND_PRIORITY: Record<string, number> = {
  hook: 0,
  permission: 1,
  mcp_server: 2,
  skill: 3,
  config: 4,
  instruction: 5,
  env_key: 6
};

function kindPriority(kind: EvidenceKind): number {
  return KIND_PRIORITY[kind] ?? 99;
}

// ── Severity rank ────────────────────────────────────────────────

const SEVERITY_RANK: Record<string, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
  none: 4
};

function severityRank(level: Severity): number {
  return SEVERITY_RANK[level] ?? 9;
}

// ── Diff computation ─────────────────────────────────────────────

function computeDiff(
  current: DiscoveredItem | null,
  target: DiscoveredItem | null
): { changes: string[]; additions: string[]; removals: string[] } {
  const changes: string[] = [];
  const additions: string[] = [];
  const removals: string[] = [];

  if (!current && target) {
    additions.push("item");
    return { changes, additions, removals };
  }

  if (current && !target) {
    removals.push("item");
    return { changes, additions, removals };
  }

  if (!current && !target) {
    return { changes, additions, removals };
  }

  // Both are non-null after the three early-return guards above
  const cur: DiscoveredItem = current!;
  const tgt: DiscoveredItem = target!;

  // Compare scalar fields
  const scalarFields: (keyof DiscoveredItem)[] = [
    "agent", "kind", "sourcePath", "scope", "precedence",
    "parser", "sensitivity", "contentPolicy", "restorePolicy",
    "captureStatus", "confidence"
  ];

  for (const field of scalarFields) {
    const cv = cur[field];
    const tv = tgt[field];
    if (cv !== undefined && tv === undefined) {
      removals.push(field);
    } else if (cv === undefined && tv !== undefined) {
      additions.push(field);
    } else if (JSON.stringify(cv) !== JSON.stringify(tv)) {
      changes.push(field);
    }
  }

  // Compare value
  if (cur.value !== undefined && tgt.value === undefined) {
    removals.push("value");
  } else if (cur.value === undefined && tgt.value !== undefined) {
    additions.push("value");
  } else if (JSON.stringify(cur.value) !== JSON.stringify(tgt.value)) {
    changes.push("value");
  }

  // Compare name
  if (cur.name !== undefined && tgt.name === undefined) {
    removals.push("name");
  } else if (cur.name === undefined && tgt.name !== undefined) {
    additions.push("name");
  } else if (cur.name !== tgt.name) {
    changes.push("name");
  }

  // Compare checksum
  if (cur.checksum !== undefined && tgt.checksum === undefined) {
    removals.push("checksum");
  } else if (cur.checksum === undefined && tgt.checksum !== undefined) {
    additions.push("checksum");
  } else if (cur.checksum !== tgt.checksum) {
    changes.push("checksum");
  }

  // Compare metadata keys
  const currentMetaKeys = cur.metadata ? Object.keys(cur.metadata) : [];
  const targetMetaKeys = tgt.metadata ? Object.keys(tgt.metadata) : [];
  const metaAllKeys = new Set([...currentMetaKeys, ...targetMetaKeys]);

  for (const key of metaAllKeys) {
    const cv = cur.metadata?.[key];
    const tv = tgt.metadata?.[key];
    if (cv !== undefined && tv === undefined) {
      removals.push(`metadata.${key}`);
    } else if (cv === undefined && tv !== undefined) {
      additions.push(`metadata.${key}`);
    } else if (JSON.stringify(cv) !== JSON.stringify(tv)) {
      changes.push(`metadata.${key}`);
    }
  }

  return { changes, additions, removals };
}

// ── Action determination ─────────────────────────────────────────

function determineAction(
  current: DiscoveredItem | null,
  target: DiscoveredItem | null,
  diff: { changes: string[]; additions: string[]; removals: string[] }
): RestoreAction {
  if (!current && target) {
    return "create";
  }

  if (current && !target) {
    return "conflict";
  }

  if (!current && !target) {
    return "skip";
  }

  // Both exist — unchanged?
  if (diff.changes.length === 0 && diff.additions.length === 0 && diff.removals.length === 0) {
    return "skip";
  }

  // Check for conflict: same id, different content
  if (diff.changes.length > 0 || diff.additions.length > 0 || diff.removals.length > 0) {
    return "update";
  }

  return "conflict";
}

// ── Risk classification ──────────────────────────────────────────

function classifyRisk(
  item: DiscoveredItem,
  action: RestoreAction
): { riskLevel: Severity; riskReason: string; needsConfirmation: boolean; confirmationPrompt: string } {
  if (action === "create") {
    if (item.kind === "permission") {
      return {
        riskLevel: "high",
        riskReason: `Creating a new permission rule for ${item.name ?? item.id}`,
        needsConfirmation: true,
        confirmationPrompt: `Create new permission: ${item.name ?? item.sourcePath}?`
      };
    }
    if (item.kind === "mcp_server") {
      return {
        riskLevel: "medium",
        riskReason: `Adding MCP server: ${item.name ?? item.id}`,
        needsConfirmation: true,
        confirmationPrompt: `Add MCP server: ${item.name ?? item.sourcePath}?`
      };
    }
    if (item.kind === "env_key") {
      return {
        riskLevel: "medium",
        riskReason: `Creating env key entry: ${item.name ?? item.id}`,
        needsConfirmation: true,
        confirmationPrompt: `Add env key: ${item.name ?? item.sourcePath}?`
      };
    }
    if (item.kind === "hook") {
      return {
        riskLevel: "high",
        riskReason: `Creating a new hook: ${item.name ?? item.id}`,
        needsConfirmation: true,
        confirmationPrompt: `Create hook: ${item.name ?? item.sourcePath}?`
      };
    }
    return {
      riskLevel: "low",
      riskReason: `Creating new ${item.kind} item`,
      needsConfirmation: false,
      confirmationPrompt: ""
    };
  }

  if (action === "update") {
    if (item.kind === "permission") {
      return {
        riskLevel: "high",
        riskReason: `Updating permission: ${item.name ?? item.id}`,
        needsConfirmation: true,
        confirmationPrompt: `Update permission: ${item.name ?? item.sourcePath}?`
      };
    }
    if (item.kind === "mcp_server") {
      return {
        riskLevel: "medium",
        riskReason: `Updating MCP server configuration: ${item.name ?? item.id}`,
        needsConfirmation: true,
        confirmationPrompt: `Update MCP server: ${item.name ?? item.sourcePath}?`
      };
    }
    if (item.kind === "hook") {
      return {
        riskLevel: "high",
        riskReason: `Updating hook: ${item.name ?? item.id}`,
        needsConfirmation: true,
        confirmationPrompt: `Update hook: ${item.name ?? item.sourcePath}?`
      };
    }
    return {
      riskLevel: "low",
      riskReason: `Updating ${item.kind} item`,
      needsConfirmation: false,
      confirmationPrompt: ""
    };
  }

  if (action === "conflict") {
    return {
      riskLevel: "high",
      riskReason: `Conflict: item exists in current state but missing from target`,
      needsConfirmation: true,
      confirmationPrompt: `Item ${item.id} exists in current state but not in target — resolve conflict?`
    };
  }

  // skip / unsupported
  return {
    riskLevel: "none",
    riskReason: "No action required",
    needsConfirmation: false,
    confirmationPrompt: ""
  };
}

// ── Dependency inference ─────────────────────────────────────────

function inferDependencies(
  currentEvidence: DiscoveredItem[],
  targetEvidence: DiscoveredItem[]
): Map<string, string[]> {
  const deps = new Map<string, string[]>();

  const allItems = [...currentEvidence, ...targetEvidence];
  const byId = new Map(allItems.map((item) => [item.id, item]));

  for (const item of allItems) {
    const depends: string[] = [];

    // Directory/file dependency: if item's sourcePath is under another item's sourcePath
    for (const other of allItems) {
      if (other.id === item.id) continue;
      if (other.kind === "skill" || other.kind === "unsupported") {
        // Skill directories contain file entries underneath
        const otherPath = other.sourcePath.endsWith("/") ? other.sourcePath : `${other.sourcePath}/`;
        if (item.sourcePath.startsWith(otherPath)) {
          depends.push(other.id);
        }
      }
    }

    // Agent config depends on agent instruction
    if (item.kind === "agent_config") {
      const instructionItems = allItems.filter(
        (candidate) =>
          candidate.kind === "agent_instruction" &&
          candidate.agent === item.agent &&
          candidate.id !== item.id
      );
      for (const instr of instructionItems) {
        depends.push(instr.id);
      }
    }

    // MCP server items depend on their parent config file
    if (item.kind === "mcp_server") {
      const parentConfig = allItems.find(
        (candidate) =>
          candidate.kind === "agent_config" &&
          candidate.sourcePath === item.sourcePath &&
          candidate.id !== item.id
      );
      if (parentConfig) {
        depends.push(parentConfig.id);
      }
    }

    deps.set(item.id, [...new Set(depends)].sort());
  }

  return deps;
}

// ── Topological sort for executionOrder ──────────────────────────

function topologicalSort(
  items: RestorePlanItem[],
  deps: Map<string, string[]>
): { order: string[]; unsupported: UnsupportedPlanItem[] } {
  const itemMap = new Map(items.map((item) => [item.itemId, item]));
  const visited = new Map<string, "white" | "gray" | "black">();
  const result: string[] = [];
  const unsupported: UnsupportedPlanItem[] = [];
  const inCycle = new Set<string>();

  for (const item of items) {
    visited.set(item.itemId, "white");
  }

  function dfs(id: string): boolean {
    const state = visited.get(id);
    if (state === "black") return true;
    if (state === "gray") {
      // Cycle detected
      inCycle.add(id);
      return false;
    }

    visited.set(id, "gray");
    const itemDeps = deps.get(id) ?? [];

    for (const depId of itemDeps) {
      if (!itemMap.has(depId)) {
        // Missing dependency — mark as unsupported
        const planItem = itemMap.get(id);
        if (planItem) {
          unsupported.push({
            itemId: id,
            agent: planItem.agent,
            kind: planItem.kind,
            sourcePath: planItem.sourcePath,
            reason: `Missing dependency: ${depId}`
          });
        }
        visited.set(id, "black");
        return false;
      }

      if (!dfs(depId)) {
        return false;
      }
    }

    visited.set(id, "black");
    if (!inCycle.has(id)) {
      result.push(id);
    }
    return true;
  }

  for (const item of items) {
    if (visited.get(item.itemId) === "white") {
      dfs(item.itemId);
    }
  }

  // Add cycle-detected items as unsupported
  for (const cycleId of inCycle) {
    const planItem = itemMap.get(cycleId);
    if (planItem) {
      unsupported.push({
        itemId: cycleId,
        agent: planItem.agent,
        kind: planItem.kind,
        sourcePath: planItem.sourcePath,
        reason: "Dependency cycle detected"
      });
    }
  }

  return { order: result, unsupported };
}

// ── Rollback generation ──────────────────────────────────────────

function generateRollback(
  items: RestorePlanItem[],
  order: string[]
): { steps: { itemId: string; action: string; instruction: string }[] } {
  const steps: { itemId: string; action: string; instruction: string }[] = [];

  for (const itemId of [...order].reverse()) {
    const planItem = items.find((item) => item.itemId === itemId);
    if (!planItem) continue;

    if (planItem.action === "create") {
      steps.push({
        itemId,
        action: "delete",
        instruction: `Delete created item ${itemId} at ${planItem.sourcePath}`
      });
    } else if (planItem.action === "update") {
      steps.push({
        itemId,
        action: "restore",
        instruction: `Restore previous state of ${itemId} at ${planItem.sourcePath}`
      });
    }
  }

  return { steps };
}

// ── Restore planner ──────────────────────────────────────────────

export async function buildRestorePlan(options: RestoreOptions): Promise<RestorePlan> {
  const sourceSnapshot = await readSnapshot(options.storeDir, options.sourceSnapshot);
  const sourceEvidence = sourceSnapshot.evidence;

  const currentScan = await scanProject({
    projectPath: options.projectPath,
    homeDir: options.homeDir,
    storeDir: options.storeDir
  });
  const currentEvidence = currentScan.evidence;

  // Build lookup maps by ID
  const currentById = new Map(currentEvidence.map((item) => [item.id, item]));
  const sourceById = new Map(sourceEvidence.map((item) => [item.id, item]));

  // All item IDs from both sides
  const allIds = new Set([...sourceById.keys(), ...currentById.keys()]);
  const allItems: RestorePlanItem[] = [];
  const unsupportedItems: UnsupportedPlanItem[] = [];

  for (const id of allIds) {
    const source = sourceById.get(id) ?? null;
    const current = currentById.get(id) ?? null;

    // Items not in source snapshot are current-state-only — mark as conflict
    if (!source && current) {
      const diff = computeDiff(current, null);
      const action = "conflict";
      const risk = classifyRisk(current, action);
      allItems.push({
        itemId: id,
        agent: current.agent,
        kind: current.kind,
        sourcePath: current.sourcePath,
        dependsOn: [],
        action,
        currentState: current,
        targetState: null,
        diff,
        riskLevel: risk.riskLevel,
        riskReason: risk.riskReason,
        needsConfirmation: risk.needsConfirmation,
        confirmationPrompt: risk.confirmationPrompt,
        rollbackInstruction: ""
      });
      continue;
    }

    // Items in source but not present — create
    if (source && !current) {
      const diff = computeDiff(null, source);
      const action = "create";
      const risk = classifyRisk(source, action);
      allItems.push({
        itemId: id,
        agent: source.agent,
        kind: source.kind,
        sourcePath: source.sourcePath,
        dependsOn: [],
        action,
        currentState: null,
        targetState: source,
        diff,
        riskLevel: risk.riskLevel,
        riskReason: risk.riskReason,
        needsConfirmation: risk.needsConfirmation,
        confirmationPrompt: risk.confirmationPrompt,
        rollbackInstruction: `Delete created item ${id} at ${source.sourcePath}`
      });
      continue;
    }

    // Both present — compare
    if (source && current) {
      const diff = computeDiff(current, source);
      const action = determineAction(current, source, diff);

      if (action === "unsupported") {
        unsupportedItems.push({
          itemId: id,
          agent: source.agent,
          kind: source.kind,
          sourcePath: source.sourcePath,
          reason: "Item cannot be restored in v0.2 planner"
        });
        continue;
      }

      const risk = classifyRisk(source, action);
      allItems.push({
        itemId: id,
        agent: source.agent,
        kind: source.kind,
        sourcePath: source.sourcePath,
        dependsOn: [],
        action,
        currentState: current,
        targetState: source,
        diff,
        riskLevel: risk.riskLevel,
        riskReason: risk.riskReason,
        needsConfirmation: risk.needsConfirmation,
        confirmationPrompt: risk.confirmationPrompt,
        rollbackInstruction:
          action === "update"
            ? `Restore previous state of ${id} at ${source.sourcePath}`
            : ""
      });
    }
  }

  // Infer dependencies
  const dependencyMap = inferDependencies(currentEvidence, sourceEvidence);

  // Apply inferred dependencies
  for (const item of allItems) {
    const inferred = dependencyMap.get(item.itemId) ?? [];
    item.dependsOn = inferred;
  }

  // Sort items by riskLevel desc, kind priority, then itemId asc (deterministic)
  allItems.sort((a, b) => {
    const byRisk = severityRank(a.riskLevel) - severityRank(b.riskLevel);
    if (byRisk !== 0) return byRisk;
    const byKind = kindPriority(a.kind) - kindPriority(b.kind);
    if (byKind !== 0) return byKind;
    return a.itemId.localeCompare(b.itemId);
  });

  // Topological sort for execution order
  const { order, unsupported: cycleUnsupported } = topologicalSort(allItems, dependencyMap);
  unsupportedItems.push(...cycleUnsupported);

  // Filter cycle items from main items
  const cycleIds = new Set(cycleUnsupported.map((u) => u.itemId));
  const filteredItems = allItems.filter((item) => !cycleIds.has(item.itemId));

  // Mark items with missing deps as unsupported
  const missingDepItems: UnsupportedPlanItem[] = [];
  for (const item of filteredItems) {
    for (const depId of item.dependsOn) {
      if (!allItems.find((i) => i.itemId === depId)) {
        missingDepItems.push({
          itemId: item.itemId,
          agent: item.agent,
          kind: item.kind,
          sourcePath: item.sourcePath,
          reason: `Missing dependency: ${depId}`
        });
      }
    }
  }
  unsupportedItems.push(...missingDepItems);
  const missingDepIds = new Set(missingDepItems.map((u) => u.itemId));
  const finalItems = filteredItems.filter((item) => !missingDepIds.has(item.itemId));

  // Compute risk summary
  const riskSummary: RiskSummary = { none: 0, low: 0, medium: 0, high: 0, critical: 0 };
  for (const item of finalItems) {
    if (item.riskLevel in riskSummary) {
      riskSummary[item.riskLevel as keyof RiskSummary]++;
    }
  }

  // Generate rollback plan
  const rollbackPlan = generateRollback(finalItems, order);

  // Build the final plan
  const plan: RestorePlan = {
    planId: randomUUID(),
    sourceSnapshot: options.sourceSnapshot,
    targetProject: path.resolve(options.projectPath),
    createdAt: new Date().toISOString(),
    itemCount: finalItems.length,
    riskSummary,
    items: finalItems,
    rollbackPlan,
    executionOrder: order.filter((id) => !cycleIds.has(id) && !missingDepIds.has(id)),
    unsupportedItems,
    planMetadata: {
      plannerVersion: "0.2.0",
      generatedBy: "snaptailor restore"
    }
  };

  return plan;
}