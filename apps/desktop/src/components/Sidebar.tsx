import type { ReactElement, SVGProps } from "react";
import { Cable, ChevronsUpDown, Home, PanelTop, RotateCcw, Settings, Sparkles } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { syncLabel, type ProfileSummary, type SetupSurfaceId } from "../domain/home-state";

export type NavItem = "home" | "setup" | SetupSurfaceId;

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

export function Sidebar({
  profile,
  activeNav,
  settingsMode,
  onSelectNav,
  onOpenSettings,
  onCloseSettings,
  sidebarOpen
}: {
  profile: ProfileSummary | null;
  activeNav: NavItem;
  settingsMode: boolean;
  onSelectNav: (nav: NavItem) => void;
  onOpenSettings: () => void;
  onCloseSettings: () => void;
  sidebarOpen: boolean;
}) {
  return (
    <aside className="sidebar" aria-hidden={!sidebarOpen}>
      {settingsMode ? (
        <SettingsNav onBack={onCloseSettings} />
      ) : (
        <>
          <ProfilePicker profile={profile} />
          <nav className="nav-list">
            {navItems.map((item) => {
              const Icon = item.icon.icon;
              return (
                <button
                  key={item.id}
                  className={`nav-item ${activeNav === item.id ? "is-active" : ""}`}
                  type="button"
                  onClick={() => onSelectNav(item.id)}
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
        <button className="icon-button" type="button" aria-label="Open settings" onClick={onOpenSettings}>
          <Settings size={16} />
        </button>
      </div>
    </aside>
  );
}

function ProfilePicker({ profile }: { profile: ProfileSummary | null }) {
  return (
    <button className="profile-picker" type="button">
      <div>
        <strong>{profile?.name ?? "No profile"}</strong>
        <span className="profile-sync">{profile ? syncLabel(profile) : "Not captured"}</span>
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
