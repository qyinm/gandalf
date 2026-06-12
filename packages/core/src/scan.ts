import path from "node:path";

import type { AgentId, DiscoveredItem, EvidenceScope } from "./types.js";
import { unsafeDiscoveredItemFromScannerOutput } from "./scanners/base.js";
import { defaultScannerPlugins } from "./scanners/index.js";
import { scanTargets } from "./scanners/filesystem.js";

export interface ScanTrust {
  readOnly: true;
  network: "disabled";
  commandsExecuted: [];
  storeWriteLocation: string;
}

export interface ScanResult {
  trust: ScanTrust;
  evidence: DiscoveredItem[];
  blindSpots: string[];
}

export interface ScanProjectOptions {
  projectPath: string;
  homeDir: string;
  storeDir: string;
  explain?: boolean;
  agent?: AgentId;
  scope?: EvidenceScope;
}

export async function scanProject(options: ScanProjectOptions): Promise<ScanResult> {
  const evidence: DiscoveredItem[] = [];
  const projectPath = path.resolve(options.projectPath);
  const homeDir = path.resolve(options.homeDir);
  const context = {
    projectPath,
    homeDir,
    storeDir: options.storeDir,
    explain: options.explain,
    scope: options.scope,
  };

  for (const plugin of defaultScannerPlugins()) {
    if (options.agent && plugin.agentId !== options.agent) {
      continue;
    }

    if (plugin.scan) {
      evidence.push(...(await plugin.scan(context)).map(unsafeDiscoveredItemFromScannerOutput));
      continue;
    }

    const targets = plugin.targets(projectPath, homeDir)
      .filter((target) => options.scope === undefined || target.scope === options.scope);
    evidence.push(...await scanTargets(targets));
  }

  const filteredEvidence = evidence.filter((item) =>
    (options.agent === undefined || item.agent === options.agent) &&
    (options.scope === undefined || item.scope === options.scope)
  );

  return {
    trust: {
      readOnly: true,
      network: "disabled",
      commandsExecuted: [],
      storeWriteLocation: options.storeDir,
    },
    evidence: filteredEvidence,
    blindSpots: [
      "Remote MCP server behavior cannot be captured",
      "Provider-side model routing cannot be verified",
      "Raw env values are omitted by policy",
    ],
  };
}
