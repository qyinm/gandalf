import type { AuditFinding, DiscoveredItem, GraphNode, Severity } from "./types.js";

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function finding(
  code: string,
  severity: Severity,
  problem: string,
  cause: string,
  fix: string,
  item: DiscoveredItem
): AuditFinding {
  return {
    code,
    severity,
    problem,
    cause,
    fix,
    path: item.sourcePath,
    evidenceId: item.id
  };
}

function isWildcardPermission(item: DiscoveredItem): boolean {
  if (item.kind !== "permission") {
    return false;
  }

  const value = record(item.value);
  const rule = typeof value.rule === "string" ? value.rule : item.name ?? "";
  return rule === "*" || rule.includes("*") || rule.includes("(*)");
}

function isSecretLike(item: DiscoveredItem): boolean {
  if (item.captureStatus !== "omitted" && item.captureStatus !== "redacted") {
    return false;
  }

  if (item.metadata?.secretLike === true) {
    return true;
  }

  const name = item.name ?? item.id;
  return /(?:secret|token|api[_-]?key|password|credential)/i.test(name);
}

function hasExecutableConfig(item: DiscoveredItem): boolean {
  const value = record(item.value);
  if (item.kind === "mcp_server" && typeof value.command === "string" && value.command.length > 0) {
    return true;
  }

  if ((item.kind === "hook" || item.kind === "skill") && item.metadata?.executable === true) {
    return true;
  }

  return false;
}

function projectOverrideFindings(graph: GraphNode[], evidenceById: Map<string, DiscoveredItem>): AuditFinding[] {
  const nodesById = new Map(graph.map((node) => [node.id, node]));
  const findings: AuditFinding[] = [];
  const emitted = new Set<string>();

  for (const node of graph) {
    if (!node.overriddenBy) {
      continue;
    }

    const overridingNode = nodesById.get(node.overriddenBy);
    const overriddenEvidence = evidenceById.get(node.evidenceId);
    const overridingEvidence = overridingNode ? evidenceById.get(overridingNode.evidenceId) : undefined;
    if (!overriddenEvidence || !overridingEvidence || node.scope !== "user" || overridingNode?.scope !== "project") {
      continue;
    }

    const key = `${overriddenEvidence.id}:${overridingEvidence.id}`;
    if (emitted.has(key)) {
      continue;
    }
    emitted.add(key);

    findings.push({
      code: "PROJECT_OVERRIDES_USER_POLICY",
      severity: "high",
      problem: "Project configuration overrides a user-level agent policy.",
      cause: `${overridingEvidence.sourcePath} has higher precedence than ${overriddenEvidence.sourcePath} for ${node.entityName}.`,
      fix: "Review the project-level rule and remove it if the override is not intentional.",
      path: overridingEvidence.sourcePath,
      evidenceId: overridingEvidence.id
    });
  }

  return findings;
}

export function auditEvidence(evidence: DiscoveredItem[], graph: GraphNode[]): AuditFinding[] {
  const findings: AuditFinding[] = [];
  const evidenceById = new Map(evidence.map((item) => [item.id, item]));

  for (const item of evidence) {
    if (hasExecutableConfig(item)) {
      // Local stdio MCP commands and hooks are normal agent config, not necessarily
      // security incidents. Use medium severity for the general case; high severity
      // is reserved for wildcard permissions, parse failures, and critical findings.
      findings.push(finding(
        "EXECUTABLE_CONFIG_ADDED",
        "medium",
        "Configuration references an executable command or hook.",
        `${item.sourcePath} contains executable configuration for ${item.name ?? item.id}.`,
        "Confirm the command is trusted and keep only explicit, necessary executable entries.",
        item
      ));
    }

    if (item.metadata?.remote === true && item.metadata?.changed === true) {
      findings.push(finding(
        "REMOTE_MCP_CHANGED",
        "medium",
        "Remote MCP configuration changed.",
        `${item.sourcePath} marks ${item.name ?? item.id} as remote and changed.`,
        "Review the remote URL and host before trusting this MCP server.",
        item
      ));
    }

    if (isWildcardPermission(item)) {
      findings.push(finding(
        "PERMISSION_WILDCARD_ADDED",
        "high",
        "Project settings added a broad permission wildcard.",
        `${item.sourcePath} contains ${item.name ?? "a wildcard permission"}.`,
        "Replace the wildcard with explicit allowed commands or resources.",
        item
      ));
    }

    if (isSecretLike(item)) {
      findings.push(finding(
        "SECRET_LIKE_VALUE_OMITTED",
        "medium",
        "A secret-like value was detected and omitted from the evidence inventory.",
        `${item.sourcePath} contains ${item.name ?? item.id}, which matches a sensitive key pattern.`,
        "Keep the value out of snapshots and rotate it if it may have been exposed elsewhere.",
        item
      ));
    }

    if (item.captureStatus === "parse_failed") {
      findings.push(finding(
        "PARSE_FAILED",
        "high",
        "A relevant agent configuration file could not be parsed.",
        `${item.sourcePath} failed to parse${typeof item.metadata?.error === "string" ? `: ${item.metadata.error}` : "."}`,
        "Fix the file syntax or exclude that source from the scan.",
        item
      ));
    }

    if (item.kind === "symlink" && (item.captureStatus === "omitted" || item.metadata?.skipped === true)) {
      // Symlinks inside skill directories are expected (skills are often symlinked to repos).
      // Skip the finding to avoid noise.
      if (!item.sourcePath.includes("/skills/")) {
        findings.push(finding(
          "SYMLINK_SKIPPED",
          "high",
          "A symlink was found and not followed.",
          `${item.sourcePath} points outside the scanned file content boundary.`,
          "Inspect the symlink manually and replace it with a regular config file if it should be captured.",
          item
        ));
      }
    }

    if (item.kind === "unsupported" || item.captureStatus === "unsupported") {
      findings.push(finding(
        "UNSUPPORTED_AGENT_STATE",
        "medium",
        "Agent state was detected but cannot be interpreted by snaptailor v0.1.",
        `${item.sourcePath} is present, but its semantics are unsupported.`,
        "Treat this as a blind spot and inspect the source manually before relying on the snapshot.",
        item
      ));
    }

    if (item.metadata?.worldWritable === true) {
      findings.push(finding(
        "WORLD_WRITABLE_STORE",
        "critical",
        "The snaptailor store is marked world-writable.",
        `${item.sourcePath} metadata reports unsafe store permissions.`,
        "Change the store permissions to 0700 before trusting stored snapshots.",
        item
      ));
    }
  }

  findings.push(...projectOverrideFindings(graph, evidenceById));

  return findings.sort((left, right) => {
    const severityRank: Record<Severity, number> = { none: 4, critical: 0, high: 1, medium: 2, low: 3 };
    return severityRank[left.severity] - severityRank[right.severity] || left.code.localeCompare(right.code);
  });
}
