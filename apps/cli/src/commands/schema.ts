/**
 * Command-pattern implementation of the `schema` CLI command.
 *
 * Exports JSON Schema definitions for gandalf's core data types.
 * Useful for CI pipelines, editor autocompletion, and external tooling.
 */

import { json, hasFlag } from "../cli-shared.js";
import { EVIDENCE_KINDS } from "@qxinm/gandalf-core/types.js";
import type { CommandContext, Command } from "./index.js";

const SCHEMA = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  title: "gandalf",
  description: "JSON Schema for gandalf data types",
  definitions: {
    agentId: {
      type: "string",
      enum: ["claude-code", "codex", "cursor", "opencode", "pi-agent", "project", "unknown"],
      description: "AI coding agent identifier"
    },
    evidenceKind: {
      type: "string",
      enum: EVIDENCE_KINDS,
      description: "Type of evidence discovered during scan"
    },
    captureStatus: {
      type: "string",
      enum: ["captured", "redacted", "omitted", "parse_failed", "unsafe_to_export", "unsupported"],
      description: "What was done with the discovered data"
    },
    severity: {
      type: "string",
      enum: ["none", "low", "medium", "high", "critical"],
      description: "Risk severity level"
    },
    evidenceScope: {
      type: "string",
      enum: ["user", "project", "managed", "unknown"],
      description: "Where the config lives"
    },
    discoveredItem: {
      type: "object",
      description: "A single piece of evidence discovered during a scan",
      properties: {
        id: { type: "string" },
        agent: { "$ref": "#/definitions/agentId" },
        kind: { "$ref": "#/definitions/evidenceKind" },
        sourcePath: { type: "string" },
        scope: { "$ref": "#/definitions/evidenceScope" },
        precedence: { type: "integer", enum: [10, 40] },
        parser: { type: "string", enum: ["json", "toml", "markdown", "dotenv", "filesystem", "unknown"] },
        captureStatus: { "$ref": "#/definitions/captureStatus" },
        confidence: { type: "string", enum: ["low", "medium", "high"] },
        name: { type: "string" },
        value: { type: "object" },
        checksum: { type: "string" },
        metadata: { type: "object" }
      },
      required: ["id", "agent", "kind", "sourcePath", "scope", "precedence", "parser", "captureStatus", "confidence"]
    },
    auditFinding: {
      type: "object",
      description: "A single security or risk finding from the audit engine",
      properties: {
        code: { type: "string" },
        severity: { "$ref": "#/definitions/severity" },
        problem: { type: "string" },
        cause: { type: "string" },
        fix: { type: "string" },
        path: { type: "string" },
        evidenceId: { type: "string" }
      },
      required: ["code", "severity", "problem", "cause", "fix"]
    },
    graphNode: {
      type: "object",
      description: "A normalized node in the agent-state graph",
      properties: {
        id: { type: "string" },
        agent: { "$ref": "#/definitions/agentId" },
        scope: { "$ref": "#/definitions/evidenceScope" },
        sourcePath: { type: "string" },
        entityKind: { "$ref": "#/definitions/evidenceKind" },
        entityName: { type: "string" },
        effectiveValue: { type: "object" },
        overriddenBy: { type: "string" },
        confidence: { type: "string", enum: ["low", "medium", "high"] },
        evidenceId: { type: "string" }
      },
      required: ["id", "agent", "scope", "sourcePath", "entityKind", "entityName", "confidence", "evidenceId"]
    },
    semanticChange: {
      type: "object",
      description: "A semantic change detected between two snapshots",
      properties: {
        code: {
          type: "string",
          enum: [
            "AGENT_CONFIG_ADDED", "AGENT_CONFIG_REMOVED", "AGENT_CONFIG_CHANGED",
            "MCP_ADDED", "MCP_REMOVED", "MCP_CHANGED",
            "SKILL_ADDED", "SKILL_REMOVED",
            "HOOK_ADDED", "HOOK_REMOVED", "HOOK_CHANGED",
            "PERMISSION_CHANGED",
            "INSTRUCTION_CHANGED",
            "PERMISSION_WILDCARD_ADDED",
            "SKILL_EXECUTABLE_APPEARED",
            "ENV_KEY_ADDED", "ENV_KEY_REMOVED",
            "UNSUPPORTED_STATE_CHANGED"
          ]
        },
        entityName: { type: "string" },
        entityKind: { "$ref": "#/definitions/evidenceKind" },
        severity: { "$ref": "#/definitions/severity" },
        details: { type: "object" }
      },
      required: ["code", "entityName", "entityKind", "severity"]
    },
    scanResult: {
      type: "object",
      description: "Result of a scan operation",
      properties: {
        trust: {
          type: "object",
          properties: {
            readOnly: { type: "boolean", const: true },
            network: { type: "string", const: "disabled" },
            commandsExecuted: { type: "array", items: { type: "string" } },
            storeWriteLocation: { type: "string" }
          }
        },
        evidence: {
          type: "array",
          items: { "$ref": "#/definitions/discoveredItem" }
        },
        blindSpots: {
          type: "array",
          items: { type: "string" }
        }
      }
    }
  }
};

export const schemaCommand: Command = {
  name: "schema",
  description: "Export JSON Schema for gandalf data types",
  async execute(ctx: CommandContext): Promise<number> {
    if (hasFlag(ctx.args, "--json") || !hasFlag(ctx.args, "--markdown")) {
      process.stdout.write(json(SCHEMA));
    }
    return 0;
  }
};
