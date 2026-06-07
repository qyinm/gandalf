import type { GraphDiff } from "./diff.js";
import type { AuditFinding, DiscoveredItem, GraphNode, ProvenanceEntry } from "./types.js";

export interface ReportInput {
  snapshotName?: string;
  current?: string;
  trust: {
    readOnly: boolean;
    network: "disabled" | "enabled" | string;
    commandsExecuted: number;
  };
  evidence: DiscoveredItem[];
  graph: GraphNode[];
  findings: AuditFinding[];
  provenance: ProvenanceEntry[];
  blindSpots: string[];
  diffs?: GraphDiff;
}

const agentNames: Record<string, string> = {
  "claude-code": "Claude Code",
  codex: "Codex",
  cursor: "Cursor",
  project: "Project",
  unknown: "Unknown"
};

function agentLine(agent: string, items: DiscoveredItem[]): string {
  const scopes = new Set(items.map((item) => item.scope));
  const states: string[] = [];
  if (scopes.has("user")) {
    states.push("user state found");
  }
  if (scopes.has("project")) {
    states.push("project state found");
  }
  if (scopes.has("managed")) {
    states.push("managed state found");
  }
  if (states.length === 0) {
    states.push("state found");
  }

  return `- ${agentNames[agent] ?? agent}  ${states.join(", ")}`;
}

function findingLine(finding: AuditFinding): string {
  const path = finding.path ? ` (${finding.path})` : "";
  return `- ${finding.severity.toUpperCase()} ${finding.code}: ${finding.problem}${path}`;
}

function provenanceLine(entry: ProvenanceEntry): string {
  return `- ${entry.evidenceId} -> ${entry.nodeId} from ${entry.sourcePath} (${entry.scope}, precedence ${entry.precedence}, ${entry.captureStatus})`;
}

function captureStatusCounts(evidence: DiscoveredItem[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const item of evidence) {
    counts.set(item.captureStatus, (counts.get(item.captureStatus) ?? 0) + 1);
  }
  return counts;
}

export function renderMarkdownReport(input: ReportInput): string {
  const snapshotName = input.snapshotName ?? input.current ?? "current";
  const lines: string[] = [
    `# hem report: ${snapshotName}`,
    "",
    "## Trust",
    `- Read-only: ${input.trust.readOnly ? "yes" : "no"}`,
    `- Network: ${input.trust.network}`,
    `- Commands executed: ${input.trust.commandsExecuted}`,
    "",
    "## Detected agents"
  ];

  const byAgent = new Map<string, DiscoveredItem[]>();
  for (const item of input.evidence) {
    byAgent.set(item.agent, [...byAgent.get(item.agent) ?? [], item]);
  }

  if (byAgent.size === 0) {
    lines.push("- None detected");
  } else {
    for (const [agent, items] of [...byAgent.entries()].sort(([left], [right]) => left.localeCompare(right))) {
      lines.push(agentLine(agent, items));
    }
  }

  lines.push("", "## High-signal findings");
  if (input.findings.length === 0) {
    lines.push("- None");
  } else {
    for (const finding of input.findings) {
      lines.push(findingLine(finding));
    }
  }

  lines.push("", "## Blind spots");
  if (input.blindSpots.length === 0) {
    lines.push("- None");
  } else {
    for (const blindSpot of input.blindSpots) {
      lines.push(`- ${blindSpot}`);
    }
  }

  lines.push("", "## Reproducibility gaps");
  const counts = captureStatusCounts(input.evidence);
  if (counts.size === 0) {
    lines.push("- None");
  } else {
    for (const [status, count] of [...counts.entries()].sort(([left], [right]) => left.localeCompare(right))) {
      lines.push(`- ${status}: ${count}`);
    }
  }

  if (input.diffs) {
    lines.push("", "## Semantic diff");
    if (input.diffs.semanticChanges.length === 0) {
      lines.push("- None");
    } else {
      for (const change of input.diffs.semanticChanges) {
        lines.push(`- ${change.severity.toUpperCase()} ${change.code}: ${change.entityName}`);
      }
    }

    lines.push("", "## Raw source changes");
    if (input.diffs.rawSourceChanges.length === 0) {
      lines.push("- None");
    } else {
      for (const change of input.diffs.rawSourceChanges) {
        lines.push(`- ${change.status}: ${change.sourcePath}`);
      }
    }
  }

  lines.push("", "## Provenance");
  if (input.provenance.length === 0) {
    lines.push("- None");
  } else {
    for (const entry of input.provenance) {
      lines.push(provenanceLine(entry));
    }
  }

  lines.push(
    "",
    "## Next",
    "- `hem snapshot create --name baseline --metadata-only --project .`"
  );

  return `${lines.join("\n")}\n`;
}
