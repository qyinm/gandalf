import { restorePolicyFor } from "../policy.js";
import type { AgentId, DiscoveredItem, DiscoveredItemConstruction, EvidenceScope } from "../types.js";
import type { ScanTarget } from "./index.js";

type EvidenceBaseTarget = Pick<
  ScanTarget,
  "agent" | "contentPolicy" | "kind" | "parser" | "precedence" | "scope" | "sensitivity" | "sourcePath"
>;

type ItemIdTarget = Pick<EvidenceBaseTarget, "agent" | "scope" | "sourcePath">;

export interface ScannerBaseAdapter {
  readonly agentId: AgentId;
}

export interface ScannerBase {
  itemId(target: ItemIdTarget, suffix: string): string;
  captured(
    target: EvidenceBaseTarget,
    kind: DiscoveredItem["kind"],
    metadata?: Record<string, unknown>,
    value?: unknown
  ): DiscoveredItem;
  parseFailed(target: EvidenceBaseTarget, kind: DiscoveredItem["kind"], error: string): DiscoveredItem;
}

export function createScannerBase(_adapter: ScannerBaseAdapter): ScannerBase {
  return {
    itemId(target, suffix) {
      return scannerItemId(target.scope, target.agent, target.sourcePath, suffix);
    },
    captured(target, kind, metadata, value) {
      return unsafeDiscoveredItemFromScannerOutput({
        id: scannerItemId(target.scope, target.agent, target.sourcePath, kind),
        agent: target.agent,
        kind,
        sourcePath: target.sourcePath,
        scope: target.scope,
        precedence: target.precedence,
        parser: target.parser,
        sensitivity: target.sensitivity,
        contentPolicy: target.contentPolicy,
        restorePolicy: restorePolicyFor(kind),
        captureStatus: "captured",
        confidence: "high",
        ...(value === undefined ? {} : { value }),
        ...(metadata === undefined ? {} : { metadata }),
      });
    },
    parseFailed(target, kind, error) {
      return {
        ...this.captured(target, kind, { error }),
        id: scannerItemId(target.scope, target.agent, target.sourcePath, `${kind}-parse-failed`),
        captureStatus: "parse_failed",
      };
    },
  };
}

// Scanner targets can be dynamic. Keep the cast named and localized at scanner boundaries.
export function unsafeDiscoveredItemFromScannerOutput(item: DiscoveredItemConstruction): DiscoveredItem {
  return item as DiscoveredItem;
}

export function scannerItemId(scope: EvidenceScope, agent: AgentId | string, sourcePath: string, suffix: string): string {
  return `${scope}.${agent}.${sourcePath}.${suffix}`
    .replace(/^~\//, "home/")
    .replace(/[^A-Za-z0-9_.-]+/g, ".")
    .replace(/^\.+|\.+$/g, "")
    .toLowerCase();
}

export function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

export function arrayOfStrings(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

export function metadataStringArray(value: unknown): string[] {
  return arrayOfStrings(value);
}
