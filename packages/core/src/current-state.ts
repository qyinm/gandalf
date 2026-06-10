import { auditEvidence } from "./audit.js";
import { buildGraph } from "./graph.js";
import { buildProvenance } from "./provenance.js";
import { scanProject, type ScanResult } from "./scan.js";
import { ensureStore } from "./store.js";
import type { AuditFinding, Snapshot, SnapshotManifest } from "./types.js";
import type { RuntimeOptions } from "./runtime-options.js";

export interface CurrentState {
  scan: ScanResult;
  snapshot: Snapshot;
  storeFindings: AuditFinding[];
}

export async function captureCurrentState(
  options: RuntimeOptions,
  name = "current"
): Promise<CurrentState> {
  const storeFindings = await ensureStore(options.storeDir);
  const scan = await scanProject(options);
  const graph = buildGraph(scan.evidence);
  const auditFindings = [...storeFindings, ...auditEvidence(scan.evidence, graph)];
  const provenance = buildProvenance(graph, scan.evidence);
  const manifest: SnapshotManifest = {
    schemaVersion: "0.1",
    name,
    createdAt: new Date().toISOString(),
    projectPath: options.projectPath,
    security: {
      rawSecretsIncluded: false,
      redactionPolicy: "metadata-only"
    }
  };

  return {
    scan,
    storeFindings,
    snapshot: {
      manifest,
      evidence: scan.evidence,
      graph,
      auditFindings,
      provenance
    }
  };
}
