import path from "node:path";

import type { DiscoveredItem } from "./types.js";
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
  };

  for (const plugin of defaultScannerPlugins()) {
    if (plugin.scan) {
      evidence.push(...(await plugin.scan(context)).map(unsafeDiscoveredItemFromScannerOutput));
      continue;
    }

    evidence.push(...await scanTargets(plugin.targets(projectPath, homeDir)));
  }

  return {
    trust: {
      readOnly: true,
      network: "disabled",
      commandsExecuted: [],
      storeWriteLocation: options.storeDir,
    },
    evidence,
    blindSpots: [
      "Remote MCP server behavior cannot be captured",
      "Provider-side model routing cannot be verified",
      "Raw env values are omitted by policy",
    ],
  };
}
