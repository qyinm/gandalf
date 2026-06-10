import { useEffect, useMemo, useState } from "react";
import type { PointerEvent, ReactElement, SVGProps } from "react";
import { invoke } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import {
  AlertTriangle,
  Cable,
  CheckCircle2,
  ChevronsUpDown,
  GitCommit,
  GitCompare,
  Home,
  PanelLeftClose,
  PanelLeftOpen,
  PanelTop,
  RefreshCcw,
  RotateCcw,
  Save,
  Settings,
  Sparkles,
  Upload
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
  activeProfile: ProfileSummary | null;
  currentSnapshotId: string | null;
  protection: "on" | "off";
  highestRisk: RiskLevel | null;
  workingChanges: number;
  changelog: ChangelogEntry[];
  surfaces: SetupSurface[];
}

type CustomIconProps = SVGProps<SVGSVGElement> & { size?: number };
type CustomIcon = (props: CustomIconProps) => ReactElement;

type NavIcon =
  | { type: "lucide"; icon: LucideIcon }
  | { type: "custom"; icon: CustomIcon };

const navItems: Array<{ id: NavItem; label: string; icon: NavIcon }> = [
  { id: "home", label: "Home", icon: { type: "lucide", icon: Home } },
  { id: "setup", label: "Setup", icon: { type: "lucide", icon: PanelTop } },
  { id: "mcp", label: "MCP", icon: { type: "lucide", icon: Cable } },
  { id: "skills", label: "Skills", icon: { type: "lucide", icon: Sparkles } },
  { id: "hooks", label: "Hooks", icon: { type: "custom", icon: PajamasHook } }
];

function PajamasHook({ size = 16, ...props }: CustomIconProps) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width={size} height={size} viewBox="0 0 16 16" {...props}>
      <path
        fill="currentColor"
        fillRule="evenodd"
        d="M1 11.125a3.875 3.875 0 0 0 7 2.292a3.875 3.875 0 0 0 7-2.292V7.002l-1.28 1.28l-1.49 1.488a.75.75 0 0 0 1.061 1.061l.208-.208v.502a2.375 2.375 0 1 1-4.75 0v-5.24a2.501 2.501 0 1 0-1.5 0v5.24a2.375 2.375 0 1 1-4.75 0v-.502l.208.208a.75.75 0 1 0 1.06-1.06L2.28 8.281L1 7.002zM9 3.5a1 1 0 1 0-2 0a1 1 0 0 0 2 0"
        clipRule="evenodd"
      />
    </svg>
  );
}

const emptyState: DesktopHomeState = {
  activeProfile: null,
  currentSnapshotId: null,
  protection: "off",
  highestRisk: null,
  workingChanges: 0,
  surfaces: [],
  changelog: []
};

function App() {
  const [activeNav, setActiveNav] = useState<NavItem>("home");
  const [settingsMode, setSettingsMode] = useState(false);
  const [state, setState] = useState<DesktopHomeState>(emptyState);
  const [timelineOpen, setTimelineOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(true);

  useEffect(() => {
    invoke<DesktopHomeState>("desktop_home_state")
      .then(setState)
      .catch(() => setState(emptyState));
  }, []);

  const activeSurface = useMemo(
    () => state.surfaces.find((surface) => surface.id === activeNav),
    [activeNav, state.surfaces]
  );

  return (
    <main className={`app-shell ${sidebarOpen ? "" : "is-sidebar-collapsed"}`}>
      <Titlebar
        snapshotId={state.currentSnapshotId}
        timelineOpen={timelineOpen}
        onToggleTimeline={() => setTimelineOpen((open) => !open)}
        sidebarOpen={sidebarOpen}
        onToggleSidebar={() => setSidebarOpen((open) => !open)}
        changelog={state.changelog}
      />
      <div className="workspace">
        <aside className="sidebar" aria-hidden={!sidebarOpen}>
          {settingsMode ? (
            <SettingsNav onBack={() => setSettingsMode(false)} />
          ) : (
            <>
              <ProfilePicker profile={state.activeProfile} />
              <nav className="nav-list">
                {navItems.map((item) => {
                  const Icon = item.icon.icon;
                  return (
                    <button
                      key={item.id}
                      className={`nav-item ${activeNav === item.id ? "is-active" : ""}`}
                      type="button"
                      onClick={() => setActiveNav(item.id)}
                    >
                      <Icon size={16} />
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
              <Settings size={16} />
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
  sidebarOpen,
  onToggleSidebar,
  changelog
}: {
  snapshotId: string | null;
  timelineOpen: boolean;
  onToggleTimeline: () => void;
  sidebarOpen: boolean;
  onToggleSidebar: () => void;
  changelog: ChangelogEntry[];
}) {
  const appWindow = getCurrentWindow();

  function startWindowDrag(event: PointerEvent<HTMLElement>) {
    if (event.button !== 0) return;
    if ((event.target as HTMLElement).closest("button")) return;
    event.preventDefault();
    void appWindow.startDragging().catch(() => {});
  }

  const SidebarToggleIcon = sidebarOpen ? PanelLeftClose : PanelLeftOpen;

  return (
    <header className="titlebar" data-tauri-drag-region onPointerDown={startWindowDrag}>
      <button
        className="sidebar-toggle icon-button"
        aria-label={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
        title={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
        onClick={onToggleSidebar}
      >
        <SidebarToggleIcon size={16} />
      </button>
      <button className="snapshot-chip" type="button" onClick={onToggleTimeline} disabled={!snapshotId}>
        <GitCommit size={14} />
        <span>{snapshotId ?? "No snapshot"}</span>
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
            {changelog.length === 0 ? (
              <div className="popover-empty">No snapshots yet</div>
            ) : null}
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

function ProfilePicker({ profile }: { profile: ProfileSummary | null }) {
  return (
    <button className="profile-picker" type="button">
      <div>
        <strong>{profile?.name ?? "No profile"}</strong>
        <span>{profile ? syncLabel(profile) : "Not captured"}</span>
      </div>
      <ChevronsUpDown size={15} />
    </button>
  );
}

function SettingsNav({ onBack }: { onBack: () => void }) {
  const items = ["Account", "Cloud", "Device", "Protection", "Notifications", "Local Paths", "Privacy", "About"];
  return (
    <nav className="nav-list">
      <button className="nav-item" type="button" onClick={onBack}>
        <RotateCcw size={15} />
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
            <h1>{state.activeProfile?.name ?? "Not captured"}</h1>
          </div>
          {state.highestRisk ? <RiskBadge risk={state.highestRisk} /> : null}
        </div>
        <div className="metric-grid">
          <Metric label="Snapshot" value={state.currentSnapshotId ?? "None"} />
          <Metric label="Protection" value={state.protection === "on" ? "On" : "Off"} />
          <Metric label="Working Changes" value={String(state.workingChanges)} />
          <Metric label="Cloud" value={state.activeProfile ? syncLabel(state.activeProfile) : "Not connected"} />
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
        {state.surfaces.length === 0 ? (
          <div className="empty-panel">No setup surfaces captured yet.</div>
        ) : null}
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
          {state.changelog.length === 0 ? (
            <div className="empty-panel">No profile snapshots or setup changes yet.</div>
          ) : null}
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
        <span>{surface?.description ?? "No captured data for this setup surface"}</span>
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
      <Icon size={15} />
      <span>{label}</span>
    </button>
  );
}

function RiskBadge({ risk }: { risk: RiskLevel }) {
  const Icon = risk === "high" ? AlertTriangle : risk === "medium" ? AlertTriangle : CheckCircle2;
  return (
    <span className={`risk-badge risk-${risk}`}>
      <Icon size={13} />
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
