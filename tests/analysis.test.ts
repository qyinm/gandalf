import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { auditEvidence } from "../src/audit.js";
import { diffGraphs } from "../src/diff.js";
import { buildGraph } from "../src/graph.js";
import { buildProvenance } from "../src/provenance.js";
import { renderMarkdownReport } from "../src/report.js";
import type { DiscoveredItem } from "../src/types.js";

function item(overrides: Partial<DiscoveredItem> & Pick<DiscoveredItem, "id" | "kind" | "sourcePath" | "scope" | "precedence">): DiscoveredItem {
  return {
    agent: "claude-code",
    parser: "json",
    sensitivity: "command_config",
    contentPolicy: "structured_safe_fields_only",
    restorePolicy: "not_supported",
    captureStatus: "captured",
    confidence: "high",
    ...overrides
  };
}

describe("analysis modules", () => {
  it("builds a graph where project policy overrides user policy and provenance stays JSON-friendly", () => {
    const evidence = [
      item({
        id: "user-permission-bash",
        kind: "permission",
        sourcePath: "~/.claude/settings.json",
        scope: "user",
        precedence: 10,
        name: "Bash(git status)",
        value: { action: "allow", rule: "Bash(git status)" }
      }),
      item({
        id: "project-permission-bash",
        kind: "permission",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "Bash(git status)",
        value: { action: "deny", rule: "Bash(git status)" }
      })
    ];

    const graph = buildGraph(evidence);
    const userNode = graph.find((node) => node.evidenceId === "user-permission-bash");
    const projectNode = graph.find((node) => node.evidenceId === "project-permission-bash");

    assert.ok(userNode);
    assert.ok(projectNode);
    assert.equal(userNode.overriddenBy, projectNode.id);
    assert.deepEqual(projectNode.effectiveValue, { action: "deny", rule: "Bash(git status)" });

    const provenance = buildProvenance(graph, evidence);
    assert.deepEqual(provenance.find((entry) => entry.evidenceId === "project-permission-bash"), {
      nodeId: projectNode.id,
      evidenceId: "project-permission-bash",
      sourcePath: ".claude/settings.json",
      scope: "project",
      precedence: 40,
      confidence: "high",
      captureStatus: "captured"
    });
  });

  it("audits project override, permission wildcard, parse failure, symlink, and omitted secret-like values", () => {
    const evidence = [
      item({
        id: "user-policy",
        kind: "permission",
        sourcePath: "~/.claude/settings.json",
        scope: "user",
        precedence: 10,
        name: "Bash(git status)",
        value: { action: "allow", rule: "Bash(git status)" }
      }),
      item({
        id: "project-policy",
        kind: "permission",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "Bash(git status)",
        value: { action: "deny", rule: "Bash(git status)" }
      }),
      item({
        id: "project-wildcard",
        kind: "permission",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "Bash(*)",
        value: { action: "allow", rule: "Bash(*)" }
      }),
      item({
        id: "bad-json",
        kind: "agent_config",
        sourcePath: ".mcp.json",
        scope: "project",
        precedence: 40,
        captureStatus: "parse_failed",
        metadata: { error: "Unexpected token" }
      }),
      item({
        id: "skipped-link",
        kind: "symlink",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        captureStatus: "omitted",
        metadata: { skipped: true, target: "../private/settings.json" }
      }),
      item({
        id: "env-token",
        kind: "env_key",
        sourcePath: ".env",
        scope: "project",
        precedence: 40,
        captureStatus: "omitted",
        name: "OPENAI_API_KEY",
        metadata: { secretLike: true }
      })
    ];

    const findings = auditEvidence(evidence, buildGraph(evidence));
    const codes = findings.map((finding) => finding.code).sort();

    assert.deepEqual(codes, [
      "PARSE_FAILED",
      "PERMISSION_WILDCARD_ADDED",
      "PROJECT_OVERRIDES_USER_POLICY",
      "SECRET_LIKE_VALUE_OMITTED",
      "SYMLINK_SKIPPED"
    ]);
    for (const finding of findings) {
      assert.ok(finding.problem);
      assert.ok(finding.cause);
      assert.ok(finding.fix);
      assert.ok(finding.path);
      assert.ok(finding.evidenceId);
    }
  });

  it("diffs semantic MCP changes and raw source changes", () => {
    const baselineGraph = buildGraph([
      item({
        id: "github-old",
        kind: "mcp_server",
        sourcePath: ".mcp.json",
        scope: "project",
        precedence: 40,
        name: "github",
        value: { transport: "stdio", command: "mcp-github", args: ["--read-only"] },
        checksum: "old"
      })
    ]);
    const currentGraph = buildGraph([
      item({
        id: "github-new",
        kind: "mcp_server",
        sourcePath: ".mcp.json",
        scope: "project",
        precedence: 40,
        name: "github",
        value: { transport: "http", url: "https://mcp.example.com/github" },
        checksum: "new"
      })
    ]);

    const diff = diffGraphs(baselineGraph, currentGraph);

    assert.deepEqual(diff.rawSourceChanges, [
      {
        sourcePath: ".mcp.json",
        beforeEvidenceId: "github-old",
        afterEvidenceId: "github-new",
        beforeChecksum: undefined,
        afterChecksum: undefined,
        status: "changed"
      }
    ]);
    assert.equal(diff.semanticChanges.length, 1);
    assert.equal(diff.semanticChanges[0].code, "MCP_CHANGED");
    assert.equal(diff.semanticChanges[0].entityName, "github");
    assert.deepEqual(diff.semanticChanges[0].details.changedFields.sort(), ["command", "transport", "urlHost"]);
  });

  it("diffs setup inventory changes used for deterministic save titles", () => {
    const baselineGraph = buildGraph([
      item({
        id: "instructions-old",
        kind: "agent_instruction",
        sourcePath: "AGENTS.md",
        scope: "project",
        precedence: 40,
        name: "AGENTS.md",
        value: { checksum: "old" },
        checksum: "old"
      }),
      item({
        id: "skill-old",
        kind: "skill",
        sourcePath: ".claude/skills/legacy-review/SKILL.md",
        scope: "project",
        precedence: 40,
        name: "legacy-review",
        value: { installed: true },
        checksum: "legacy"
      }),
      item({
        id: "hook-old",
        kind: "hook",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "pre-tool-use",
        value: { command: "old-hook" },
        checksum: "hook-old"
      }),
      item({
        id: "permission-old",
        kind: "permission",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "Bash(git status)",
        value: { action: "allow", rule: "Bash(git status)" },
        checksum: "permission-old"
      }),
      item({
        id: "permission-removed",
        kind: "permission",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "Bash(bun test)",
        value: { action: "allow", rule: "Bash(bun test)" },
        checksum: "permission-removed"
      })
    ]);
    const currentGraph = buildGraph([
      item({
        id: "instructions-new",
        kind: "agent_instruction",
        sourcePath: "AGENTS.md",
        scope: "project",
        precedence: 40,
        name: "AGENTS.md",
        value: { checksum: "new" },
        checksum: "new"
      }),
      item({
        id: "skill-new",
        kind: "skill",
        sourcePath: ".claude/skills/react-review/SKILL.md",
        scope: "project",
        precedence: 40,
        name: "react-review",
        value: { installed: true },
        checksum: "skill"
      }),
      item({
        id: "hook-new",
        kind: "hook",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "pre-tool-use",
        value: { command: "new-hook" },
        checksum: "hook-new"
      }),
      item({
        id: "hook-added",
        kind: "hook",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "post-tool-use",
        value: { command: "notify" },
        checksum: "hook-added"
      }),
      item({
        id: "permission-new",
        kind: "permission",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "Bash(git status)",
        value: { action: "deny", rule: "Bash(git status)" },
        checksum: "permission-new"
      }),
      item({
        id: "permission-added",
        kind: "permission",
        sourcePath: ".claude/settings.json",
        scope: "project",
        precedence: 40,
        name: "Bash(bun run build)",
        value: { action: "allow", rule: "Bash(bun run build)" },
        checksum: "permission-added"
      })
    ]);

    const diff = diffGraphs(baselineGraph, currentGraph);

    assert.deepEqual(
      diff.semanticChanges.map((change) => change.code).sort(),
      [
        "HOOK_ADDED",
        "HOOK_CHANGED",
        "INSTRUCTION_CHANGED",
        "PERMISSION_CHANGED",
        "PERMISSION_CHANGED",
        "PERMISSION_CHANGED",
        "SKILL_ADDED",
        "SKILL_REMOVED"
      ]
    );
    assert.equal(
      diff.semanticChanges.some((change) => change.code === "PERMISSION_CHANGED" && change.details.removed === true),
      true
    );
  });

  it("renders markdown reports with detected agents, blind spots, findings, provenance, and next command", () => {
    const evidence = [
      item({
        id: "unsupported-codex",
        agent: "codex",
        kind: "unsupported",
        sourcePath: ".codex/config.toml",
        scope: "project",
        precedence: 30,
        captureStatus: "unsupported",
        confidence: "medium",
        metadata: { state: "present" }
      })
    ];
    const graph = buildGraph(evidence);
    const findings = auditEvidence(evidence, graph);
    const provenance = buildProvenance(graph, evidence);

    const markdown = renderMarkdownReport({
      snapshotName: "current",
      trust: { readOnly: true, network: "disabled", commandsExecuted: 0 },
      evidence,
      graph,
      findings,
      provenance,
      blindSpots: ["Remote MCP server behavior cannot be captured", "Raw env values are omitted by policy"]
    });

    assert.match(markdown, /^# hem report: current/m);
    assert.match(markdown, /## Detected agents/);
    assert.match(markdown, /Codex\s+project state found/);
    assert.match(markdown, /## High-signal findings/);
    assert.match(markdown, /UNSUPPORTED_AGENT_STATE/);
    assert.doesNotMatch(markdown, /hem v0\.1/);
    assert.match(markdown, /cannot yet be interpreted by hem/);
    assert.match(markdown, /## Blind spots/);
    assert.match(markdown, /Remote MCP server behavior cannot be captured/);
    assert.match(markdown, /## Reproducibility gaps/);
    assert.match(markdown, /unsupported: 1/);
    assert.match(markdown, /## Provenance/);
    assert.match(markdown, /unsupported-codex/);
    assert.match(markdown, /hem snapshot create --name baseline --metadata-only --project \./);
  });
});
