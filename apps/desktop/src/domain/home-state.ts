export type SyncState = "local_only" | "up_to_date" | "ahead" | "behind" | "diverged";
export type RiskLevel = "low" | "medium" | "high";
export type ProfileScope = "personal" | "team";
export type ProtectionState = "on" | "off";
export type ChangelogSource = "manual" | "auto" | "restore" | "remote";
export type SetupSurfaceId = "mcp" | "skills" | "hooks";

export interface ProfileSummary {
  name: string;
  scope: ProfileScope;
  team?: string;
  syncState: SyncState;
  ahead: number;
  behind: number;
}

export interface ChangelogEntry {
  id: string;
  title: string;
  time: string;
  source: ChangelogSource;
  risk: RiskLevel;
}

export interface SetupSurface {
  id: SetupSurfaceId;
  label: string;
  count: number;
  risk: RiskLevel;
  description: string;
}

export interface InventoryItem {
  id: string;
  name: string;
  agent: string;
  sourcePath: string;
  scope: "user" | "project" | "managed" | "unknown";
  status?: string;
  detail?: string;
}

export interface DesktopInventory {
  skills: InventoryItem[];
  mcp: InventoryItem[];
  hooks: InventoryItem[];
}

export interface DesktopHomeState {
  activeProfile: ProfileSummary | null;
  currentSnapshotId: string | null;
  protection: ProtectionState;
  highestRisk: RiskLevel | null;
  workingChanges: number;
  changelog: ChangelogEntry[];
  surfaces: SetupSurface[];
  inventory: DesktopInventory;
}

export const emptyDesktopInventory: DesktopInventory = {
  skills: [],
  mcp: [],
  hooks: []
};

export const emptyDesktopHomeState: DesktopHomeState = {
  activeProfile: null,
  currentSnapshotId: null,
  protection: "off",
  highestRisk: null,
  workingChanges: 0,
  surfaces: [],
  changelog: [],
  inventory: emptyDesktopInventory
};

export function homeStateIsActionable(state: DesktopHomeState): boolean {
  return state.protection === "on" && Boolean(state.activeProfile) && Boolean(state.currentSnapshotId);
}

export function normalizeDesktopHomeState(value: unknown): DesktopHomeState {
  const payload = objectFrom(value);
  const inventory = normalizeDesktopInventory(payload.inventory);

  return {
    activeProfile: normalizeProfile(payload.activeProfile),
    currentSnapshotId: optionalString(payload.currentSnapshotId),
    protection: isProtectionState(payload.protection) ? payload.protection : emptyDesktopHomeState.protection,
    highestRisk: isRiskLevel(payload.highestRisk) ? payload.highestRisk : emptyDesktopHomeState.highestRisk,
    workingChanges: nonNegativeNumber(payload.workingChanges),
    changelog: arrayFrom(payload.changelog).map(normalizeChangelogEntry).filter(isPresent),
    surfaces: normalizeSetupSurfaces(payload.surfaces, inventory),
    inventory
  };
}

export function syncLabel(profile: ProfileSummary): string {
  if (profile.syncState === "local_only") return "Local only";
  if (profile.syncState === "up_to_date") return "Up to date";
  if (profile.syncState === "ahead") return `Ahead ${profile.ahead}`;
  if (profile.syncState === "behind") return `Behind ${profile.behind}`;
  return `Ahead ${profile.ahead}, behind ${profile.behind}`;
}

export function surfaceById(surfaces: SetupSurface[], id: SetupSurfaceId): SetupSurface | undefined {
  return surfaces.find((surface) => surface.id === id);
}

export function inventoryForSurface(inventory: DesktopInventory, id: SetupSurfaceId): InventoryItem[] {
  return inventory[id];
}

function normalizeDesktopInventory(value: unknown): DesktopInventory {
  const payload = objectFrom(value);
  return {
    skills: arrayFrom(payload.skills).map(normalizeInventoryItem).filter(isPresent),
    mcp: arrayFrom(payload.mcp).map(normalizeInventoryItem).filter(isPresent),
    hooks: arrayFrom(payload.hooks).map(normalizeInventoryItem).filter(isPresent)
  };
}

function normalizeSetupSurfaces(value: unknown, inventory: DesktopInventory): SetupSurface[] {
  const surfaces = arrayFrom(value).map(normalizeSetupSurface).filter(isPresent);
  const byId = new Map<SetupSurfaceId, SetupSurface>(surfaces.map((surface) => [surface.id, surface]));

  return (["mcp", "skills", "hooks"] as const).map((id) => ({
    ...defaultSurface(id, inventory),
    ...byId.get(id),
    count: inventoryForSurface(inventory, id).length
  }));
}

function defaultSurface(id: SetupSurfaceId, inventory: DesktopInventory): SetupSurface {
  const count = inventoryForSurface(inventory, id).length;
  if (id === "mcp") {
    return { id, label: "MCP", count, risk: "low", description: `${count} MCP server${count === 1 ? "" : "s"} installed` };
  }
  if (id === "skills") {
    return { id, label: "Skills", count, risk: "low", description: `${count} skill${count === 1 ? "" : "s"} installed` };
  }
  return { id, label: "Hooks", count, risk: "low", description: `${count} hook${count === 1 ? "" : "s"} installed` };
}

function normalizeProfile(value: unknown): ProfileSummary | null {
  const payload = objectFrom(value);
  const name = optionalString(payload.name);
  const scope = isProfileScope(payload.scope) ? payload.scope : null;
  const syncState = isSyncState(payload.syncState) ? payload.syncState : null;

  if (!name || !scope || !syncState) return null;

  return {
    name,
    scope,
    team: optionalString(payload.team) ?? undefined,
    syncState,
    ahead: nonNegativeNumber(payload.ahead),
    behind: nonNegativeNumber(payload.behind)
  };
}

function normalizeChangelogEntry(value: unknown): ChangelogEntry | null {
  const payload = objectFrom(value);
  const id = optionalString(payload.id);
  const title = optionalString(payload.title);
  const time = optionalString(payload.time);
  const source = isChangelogSource(payload.source) ? payload.source : null;
  const risk = isRiskLevel(payload.risk) ? payload.risk : null;

  if (!id || !title || !time || !source || !risk) return null;
  return { id, title, time, source, risk };
}

function normalizeSetupSurface(value: unknown): SetupSurface | null {
  const payload = objectFrom(value);
  const id = isSetupSurfaceId(payload.id) ? payload.id : null;
  const label = optionalString(payload.label);
  const description = optionalString(payload.description);
  const risk = isRiskLevel(payload.risk) ? payload.risk : null;

  if (!id || !label || !description || !risk) return null;
  return {
    id,
    label,
    count: nonNegativeNumber(payload.count),
    risk,
    description
  };
}

function normalizeInventoryItem(value: unknown): InventoryItem | null {
  const payload = objectFrom(value);
  const id = optionalString(payload.id);
  const name = optionalString(payload.name);
  const agent = optionalString(payload.agent);
  const sourcePath = optionalString(payload.sourcePath);

  if (!id || !name || !agent || !sourcePath) return null;
  return {
    id,
    name,
    agent,
    sourcePath,
    scope: isInventoryScope(payload.scope) ? payload.scope : "unknown",
    status: optionalString(payload.status) ?? undefined,
    detail: optionalString(payload.detail) ?? undefined
  };
}

function objectFrom(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function arrayFrom(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function optionalString(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null;
}

function nonNegativeNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : 0;
}

function isPresent<T>(value: T | null): value is T {
  return value !== null;
}

function isSyncState(value: unknown): value is SyncState {
  return value === "local_only" || value === "up_to_date" || value === "ahead" || value === "behind" || value === "diverged";
}

function isRiskLevel(value: unknown): value is RiskLevel {
  return value === "low" || value === "medium" || value === "high";
}

function isProfileScope(value: unknown): value is ProfileScope {
  return value === "personal" || value === "team";
}

function isProtectionState(value: unknown): value is ProtectionState {
  return value === "on" || value === "off";
}

function isChangelogSource(value: unknown): value is ChangelogSource {
  return value === "manual" || value === "auto" || value === "restore" || value === "remote";
}

function isSetupSurfaceId(value: unknown): value is SetupSurfaceId {
  return value === "mcp" || value === "skills" || value === "hooks";
}

function isInventoryScope(value: unknown): value is InventoryItem["scope"] {
  return value === "user" || value === "project" || value === "managed" || value === "unknown";
}
