import { useEffect, useMemo, useState } from "react";
import { invoke } from "@tauri-apps/api/core";

import { Sidebar, type NavItem, type SettingsSection } from "./components/Sidebar";
import { Titlebar } from "./components/Titlebar";
import {
  emptyDesktopHomeState,
  inventoryForSurface,
  normalizeDesktopHomeState,
  surfaceById,
  type DesktopHomeState,
  type SetupSurface
} from "./domain/home-state";
import { HomeScreen } from "./screens/HomeScreen";
import { SettingsScreen } from "./screens/SettingsScreen";
import { SurfaceScreen } from "./screens/SurfaceScreen";
import "./App.css";

function App() {
  const [activeNav, setActiveNav] = useState<NavItem>("home");
  const [settingsMode, setSettingsMode] = useState(false);
  const [activeSettingsSection, setActiveSettingsSection] = useState<SettingsSection>("account");
  const [state, setState] = useState<DesktopHomeState>(emptyDesktopHomeState);
  const [timelineOpen, setTimelineOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(true);

  useEffect(() => {
    invoke<unknown>("desktop_home_state")
      .then((payload) => setState(normalizeDesktopHomeState(payload)))
      .catch(() => setState(emptyDesktopHomeState));
  }, []);

  const activeSurface = useMemo(() => surfaceForNav(state.surfaces, activeNav), [activeNav, state.surfaces]);

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
        <Sidebar
          profile={state.activeProfile}
          profiles={state.profiles}
          activeNav={activeNav}
          settingsMode={settingsMode}
          activeSettingsSection={activeSettingsSection}
          onSelectNav={setActiveNav}
          onSelectSettingsSection={setActiveSettingsSection}
          onOpenSettings={() => {
            setActiveSettingsSection("account");
            setSettingsMode(true);
          }}
          onCloseSettings={() => setSettingsMode(false)}
          sidebarOpen={sidebarOpen}
        />

        <section className="content">
          {settingsMode ? (
            <SettingsScreen section={activeSettingsSection} state={state} onSelectSection={setActiveSettingsSection} />
          ) : activeNav === "home" ? (
            <HomeScreen state={state} />
          ) : (
            <SurfaceScreen inventory={inventoryForNav(state, activeNav)} surface={activeSurface} nav={activeNav} />
          )}
        </section>
      </div>
    </main>
  );
}

function surfaceForNav(surfaces: DesktopHomeState["surfaces"], nav: NavItem): SetupSurface | undefined {
  if (nav === "home" || nav === "setup") return undefined;
  return surfaceById(surfaces, nav);
}

function inventoryForNav(state: DesktopHomeState, nav: NavItem) {
  if (nav === "home" || nav === "setup") return [];
  return inventoryForSurface(state.inventory, nav);
}

export default App;
