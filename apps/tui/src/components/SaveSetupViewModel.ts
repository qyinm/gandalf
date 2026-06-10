import type { GraphDiff, SemanticChange } from "@qxinm/hem-core/diff.js";

export interface SaveSetupDestination {
  label: string;
  selected: boolean;
  disabled?: boolean;
  note?: string;
}

export interface SaveSetupViewModel {
  title: string;
  detectedChanges: string[];
  destinations: SaveSetupDestination[];
  noChanges: boolean;
}

export function buildSaveSetupViewModel(input: {
  diff?: GraphDiff;
  hasPreviousSnapshot: boolean;
}): SaveSetupViewModel {
  const semanticChanges = input.diff?.semanticChanges ?? [];
  const rawSourceChanges = input.diff?.rawSourceChanges ?? [];
  const noChanges = input.hasPreviousSnapshot && semanticChanges.length === 0 && rawSourceChanges.length === 0;

  return {
    title: snapshotTitleForChanges(input.diff, input.hasPreviousSnapshot),
    detectedChanges: noChanges
      ? ["Current setup matches latest saved setup."]
      : detectedChangeLabels(semanticChanges, rawSourceChanges.length),
    destinations: [
      { label: "Local history", selected: true },
      { label: "Export as .hem", selected: false }
    ],
    noChanges
  };
}

export function snapshotTitleForChanges(diff: GraphDiff | undefined, hasPreviousSnapshot: boolean): string {
  if (!hasPreviousSnapshot) return "capture baseline";
  if (!diff || (diff.semanticChanges.length === 0 && diff.rawSourceChanges.length === 0)) {
    return "current setup unchanged";
  }

  const semanticChanges = diff.semanticChanges;
  const firstMcp = firstChange(semanticChanges, ["MCP_ADDED", "MCP_REMOVED", "MCP_CHANGED"]);
  if (firstMcp) return titleForSemanticChange(firstMcp);

  const firstSkill = firstChange(semanticChanges, ["SKILL_ADDED", "SKILL_REMOVED", "SKILL_EXECUTABLE_APPEARED"]);
  if (firstSkill) return titleForSemanticChange(firstSkill);

  const firstHookOrPermission = firstChange(semanticChanges, [
    "HOOK_ADDED",
    "HOOK_REMOVED",
    "HOOK_CHANGED",
    "PERMISSION_CHANGED",
    "PERMISSION_WILDCARD_ADDED"
  ]);
  if (firstHookOrPermission) return titleForSemanticChange(firstHookOrPermission);

  const firstInstruction = firstChange(semanticChanges, ["INSTRUCTION_CHANGED"]);
  if (firstInstruction) return titleForSemanticChange(firstInstruction);

  const firstEnv = firstChange(semanticChanges, ["ENV_KEY_ADDED", "ENV_KEY_REMOVED"]);
  if (firstEnv) return titleForSemanticChange(firstEnv);

  if (semanticChanges.length > 1 || diff.rawSourceChanges.length > 1) {
    return `change ${semanticChanges.length} setup items and ${diff.rawSourceChanges.length} files`;
  }

  return "update setup";
}

function detectedChangeLabels(semanticChanges: SemanticChange[], rawSourceChangeCount: number): string[] {
  const labels = semanticChanges.slice(0, 8).map(titleForSemanticChange);
  if (rawSourceChangeCount > 0) {
    labels.push(`change ${rawSourceChangeCount} source file${rawSourceChangeCount === 1 ? "" : "s"}`);
  }
  return labels.length > 0 ? labels : ["capture baseline"];
}

function firstChange(changes: SemanticChange[], codes: SemanticChange["code"][]): SemanticChange | undefined {
  return changes.find((change) => codes.includes(change.code));
}

function titleForSemanticChange(change: SemanticChange): string {
  switch (change.code) {
    case "MCP_ADDED":
      return `add ${change.entityName} mcp`;
    case "MCP_REMOVED":
      return `remove ${change.entityName} mcp`;
    case "MCP_CHANGED":
      return `update ${change.entityName} mcp`;
    case "SKILL_ADDED":
    case "SKILL_EXECUTABLE_APPEARED":
      return `install ${change.entityName} skill`;
    case "SKILL_REMOVED":
      return `remove ${change.entityName} skill`;
    case "HOOK_ADDED":
    case "HOOK_REMOVED":
    case "HOOK_CHANGED":
      return "update hooks";
    case "PERMISSION_CHANGED":
    case "PERMISSION_WILDCARD_ADDED":
      return "update permissions";
    case "INSTRUCTION_CHANGED":
      return "update project instructions";
    case "ENV_KEY_ADDED":
    case "ENV_KEY_REMOVED":
      return "update env key inventory";
    default:
      return "update setup";
  }
}
