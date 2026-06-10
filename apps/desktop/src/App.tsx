import { useEffect, useMemo, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import {
  AlertTriangle,
  Cable,
  CheckCircle2,
  ChevronsUpDown,
  GitCommit,
  GitCompare,
  Home,
  PanelTop,
  RefreshCcw,
  RotateCcw,
  Save,
  Settings,
  Sparkles,
  Upload,
  Zap
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import "./App.css";

type SyncState = "local_only" | "up_to_date" | "ahead" | "behind" | "diverged";
type RiskLevel = "low" | "medium" | "high";
type NavItem = "home" | "setup" | "mcp" | "skills" | "hooks";

interface ProfileSummary {
  name: string;
  scope: "personal" | "team";
  team?: string;
  syncState: SyncState;
  ahead: number;
  behind: number;
}

interface ChangelogEntry {
  id: string;
  title: string;
  time: string;
  source: "manual" | "auto" | "restore" | "remote";
  risk: RiskLevel;
}

interface SetupSurface {
  id: string;
  label: string;
  count: number;
  risk: RiskLevel;
  description: string;
}

interface DesktopHomeState {
  activeProfile: ProfileSummary;
  currentSnapshotId: string;
  protection: "on" | "off";
  highestRisk: RiskLevel;
  workingChanges: number;
  changelog: ChangelogEntry[];
  surfaces: SetupSurface[];
}

const navItems: Array<{ id: NavItem; label: string; icon: LucideIcon }> = [
  { id: "home", label: "Home", icon: Home },
  { id: "setup", label: "Setup", icon: PanelTop },
  { id: "mcp", label: "MCP", icon: Cable },
  { id: "skills", label: "Skills", icon: Sparkles },
  { id: "hooks", label: "Hooks", icon: Zap }
];

const fallbackState: DesktopHomeState = {
  activeProfile: {
    name: "Default",
    scope: "personal",
    syncState: "local_only",
    ahead: 0,
    behind: 0
  },
  currentSnapshotId: "8f3a2c7",
  protection: "on",
  highestRisk: "medium",
  workingChanges: 3,
  surfaces: [
    { id: "setup", label: "Setup", count: 7, risk: "medium", description: "Codex config, permissions, env key inventory" },
    { id: "mcp", label: "MCP", count: 2, risk: "high", description: "Configured MCP servers and required env keys" },
    { id: "skills", label: "Skills", count: 4, risk: "low", description: "Installed Codex skills detected in user-global roots" },
    { id: "hooks", label: "Hooks", count: 1, risk: "medium", description: "Executable setup hooks requiring review" }
  ],
  changelog: [
    { id: "8f3a2c7", title: "MCP server changed: figma", time: "12 min ago", source: "auto", risk: "high" },
    { id: "72ab91d", title: "Snapshot created from Default", time: "1h ago", source: "manual", risk: "medium" },
    { id: "19df02a", title: "Initial Codex setup captured", time: "Yesterday", source: "manual", risk: "low" }
  ]
};

function App() {
  const [activeNav, setActiveNav] = useState<NavItem>("home");
  const [settingsMode, setSettingsMode] = useState(false);
  const [state, setState] = useState<DesktopHomeState>(fallbackState);
  const [timelineOpen, setTimelineOpen] = useState(false);

  useEffect(() => {
    invoke<DesktopHomeState>("desktop_home_state")
      .then(setState)
      .catch(() => setState(fallbackState));
  }, []);

  const activeSurface = useMemo(
    () => state.surfaces.find((surface) => surface.id === activeNav),
    [activeNav, state.surfaces]
  );

  return (
    <main className="app-shell">
      <Titlebar
        snapshotId={state.currentSnapshotId}
        timelineOpen={timelineOpen}
        onToggleTimeline={() => setTimelineOpen((open) => !open)}
        changelog={state.changelog}
      />
      <div className="workspace">
        <aside className="sidebar">
          {settingsMode ? (
            <SettingsNav onBack={() => setSettingsMode(false)} />
          ) : (
            <>
              <ProfilePicker profile={state.activeProfile} />
              <nav className="nav-list">
                {navItems.map((item) => {
                  const Icon = item.icon;
                  return (
                    <button
                      key={item.id}
                      className={`nav-item ${activeNav === item.id ? "is-active" : ""}`}
                      type="button"
                      onClick={() => setActiveNav(item.id)}
                    >
                      <Icon size={17} />
                      <span>{item.label}</span>
                    </button>
                  );
                })}
              </nav>
            </>
          )}
          <div className="account-row">
            <button className="account-button" type="button">
              <span className="avatar">h</span>
              <span>hippoo</span>
            </button>
            <button
              className="icon-button"
              type="button"
              aria-label="Open settings"
              onClick={() => setSettingsMode(true)}
            >
              <Settings size={17} />
            </button>
          </div>
        </aside>

        <section className="content">
          {activeNav === "home" ? (
            <HomeScreen state={state} />
          ) : (
            <SurfaceScreen surface={activeSurface} nav={activeNav} />
          )}
        </section>
      </div>
    </main>
  );
}

function Titlebar({
  snapshotId,
  timelineOpen,
  onToggleTimeline,
  changelog
}: {
  snapshotId: string;
  timelineOpen: boolean;
  onToggleTimeline: () => void;
  changelog: ChangelogEntry[];
}) {
  return (
    <header className="titlebar" data-tauri-drag-region>
      <div className="brand" data-tauri-drag-region>Hem</div>
      <button className="snapshot-chip" type="button" onClick={onToggleTimeline}>
        <GitCommit size={14} />
        <span>{snapshotId}</span>
      </button>
      {timelineOpen ? (
        <div className="snapshot-popover">
          <div className="popover-header">
            <span>Current Snapshot</span>
            <strong>{snapshotId}</strong>
          </div>
          <div className="popover-list">
            {changelog.slice(0, 3).map((entry) => (
              <div className="popover-row" key={entry.id}>
                <code>{entry.id}</code>
                <span>{entry.time}</span>
                <RiskBadge risk={entry.risk} />
              </div>
            ))}
          </div>
          <div className="popover-actions">
            <button type="button">Open Timeline</button>
            <button type="button">Restore Snapshot</button>
          </div>
        </div>
      ) : null}
    </header>
  );
}

function ProfilePicker({ profile }: { profile: ProfileSummary }) {
  return (
    <button className="profile-picker" type="button">
      <div>
        <strong>{profile.name}</strong>
        <span>{syncLabel(profile)}</span>
      </div>
      <ChevronsUpDown size={16} />
    </button>
  );
}

function SettingsNav({ onBack }: { onBack: () => void }) {
  const items = ["Account", "Cloud", "Device", "Protection", "Notifications", "Local Paths", "Privacy", "About"];
  return (
    <nav className="nav-list">
      <button className="nav-item" type="button" onClick={onBack}>
        <RotateCcw size={16} />
        <span>Back</span>
      </button>
      <div className="nav-separator" />
      {items.map((item) => (
        <button className="nav-item" type="button" key={item}>
          <span className="nav-spacer" />
          <span>{item}</span>
        </button>
      ))}
    </nav>
  );
}

function HomeScreen({ state }: { state: DesktopHomeState }) {
  return (
    <div className="home-screen">
      <section className="overall">
        <div className="overall-heading">
          <div>
            <p className="eyebrow">Active Profile</p>
            <h1>{state.activeProfile.name}</h1>
          </div>
          <RiskBadge risk={state.highestRisk} />
        </div>
        <div className="metric-grid">
          <Metric label="Snapshot" value={state.currentSnapshotId} />
          <Metric label="Protection" value={state.protection === "on" ? "On" : "Off"} />
          <Metric label="Working Changes" value={String(state.workingChanges)} />
          <Metric label="Cloud" value={syncLabel(state.activeProfile)} />
        </div>
        <div className="action-row">
          <ActionButton icon={Save} label="Create Snapshot" primary />
          <ActionButton icon={GitCompare} label="View Diff" />
          <ActionButton icon={RotateCcw} label="Restore Previous" />
          <ActionButton icon={Upload} label="Push" />
          <ActionButton icon={RefreshCcw} label="Update from Remote" />
        </div>
      </section>

      <section className="surface-strip">
        {state.surfaces.map((surface) => (
          <article className="surface-tile" key={surface.id}>
            <div>
              <span className="surface-label">{surface.label}</span>
              <strong>{surface.count}</strong>
            </div>
            <RiskBadge risk={surface.risk} />
            <p>{surface.description}</p>
          </article>
        ))}
      </section>

      <section className="changelog">
        <div className="section-heading">
          <h2>Changelog</h2>
          <span>Latest profile snapshots and setup changes</span>
        </div>
        <div className="timeline-list">
          {state.changelog.map((entry) => (
            <article className="timeline-row" key={entry.id}>
              <code>{entry.id}</code>
              <div>
                <strong>{entry.title}</strong>
                <span>{entry.source} - {entry.time}</span>
              </div>
              <RiskBadge risk={entry.risk} />
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}

function SurfaceScreen({ surface, nav }: { surface?: SetupSurface; nav: NavItem }) {
  const title = surface?.label ?? nav;
  return (
    <div className="surface-screen">
      <div className="section-heading">
        <h1>{title}</h1>
        <span>{surface?.description ?? "Current Codex setup surface"}</span>
      </div>
      <div className="surface-detail">
        <div>
          <p className="eyebrow">Items</p>
          <strong>{surface?.count ?? 0}</strong>
        </div>
        <div>
          <p className="eyebrow">Risk</p>
          <RiskBadge risk={surface?.risk ?? "low"} />
        </div>
        <div>
          <p className="eyebrow">Mode</p>
          <span>Read-only MVP</span>
        </div>
      </div>
      <div className="action-row">
        <ActionButton icon={GitCompare} label="View Diff" />
        <ActionButton icon={Save} label="Create Snapshot" />
      </div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function ActionButton({ icon: Icon, label, primary = false }: { icon: LucideIcon; label: string; primary?: boolean }) {
  return (
    <button className={`action-button ${primary ? "is-primary" : ""}`} type="button">
      <Icon size={16} />
      <span>{label}</span>
    </button>
  );
}

function RiskBadge({ risk }: { risk: RiskLevel }) {
  const Icon = risk === "high" ? AlertTriangle : risk === "medium" ? AlertTriangle : CheckCircle2;
  return (
    <span className={`risk-badge risk-${risk}`}>
      <Icon size={14} />
      {risk}
    </span>
  );
}

function syncLabel(profile: ProfileSummary): string {
  if (profile.syncState === "local_only") return "Local only";
  if (profile.syncState === "up_to_date") return "Up to date";
  if (profile.syncState === "ahead") return `Ahead ${profile.ahead}`;
  if (profile.syncState === "behind") return `Behind ${profile.behind}`;
  return `Ahead ${profile.ahead}, behind ${profile.behind}`;
}

export default App;
