import type { AgentId, DiscoveredItem, EvidenceKind, EvidenceScope } from "../types.js";
import type { ScannerPlugin } from "./scanner-plugin.js";

export type { ScannerPlugin } from "./scanner-plugin.js";

export interface ScanTarget {
  absolutePath: string;
  sourcePath: string;
  scope: EvidenceScope;
  agent: AgentId;
  kind: EvidenceKind;
  parser: DiscoveredItem["parser"];
  precedence: number;
  sensitivity: string;
  contentPolicy: string;
  directory?: boolean;
  metadataOnly?: boolean;
}

function makeTarget(
  root: string,
  relativePath: string,
  scope: EvidenceScope,
  precedence: number,
  agent: AgentId,
  kind: EvidenceKind,
  parser: DiscoveredItem["parser"],
  overrides: Partial<ScanTarget>
): ScanTarget {
  return Object.assign(
    {
      absolutePath: root + "/" + relativePath,
      sourcePath: scope === "user" ? "~/" + relativePath : relativePath,
      scope,
      precedence,
      agent,
      kind,
      parser,
      sensitivity: "metadata",
      contentPolicy: "metadata_only",
    },
    overrides
  );
}

export function projectTarget(
  projectPath: string,
  relativePath: string,
  agent: AgentId,
  kind: EvidenceKind,
  parser: DiscoveredItem["parser"],
  overrides: Partial<ScanTarget> = {}
): ScanTarget {
  return makeTarget(projectPath, relativePath, "project", 40, agent, kind, parser, overrides);
}

export function homeTarget(
  homeDir: string,
  relativePath: string,
  agent: AgentId,
  kind: EvidenceKind,
  parser: DiscoveredItem["parser"],
  overrides: Partial<ScanTarget> = {}
): ScanTarget {
  return makeTarget(homeDir, relativePath, "user", 10, agent, kind, parser, overrides);
}

import { claudeCodeScanner } from "./claude-code.js";
import { codexScanner } from "./codex.js";
import { cursorScanner } from "./cursor.js";
import { projectScanner } from "./project.js";

export function defaultScannerPlugins(): ScannerPlugin[] {
  return [claudeCodeScanner, codexScanner, cursorScanner, projectScanner];
}
