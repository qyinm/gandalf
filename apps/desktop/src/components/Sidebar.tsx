import { useEffect, useRef, useState, type ReactElement, type SVGProps } from "react";
import {
  ArrowLeft,
  Bell,
  Cable,
  ChevronsUpDown,
  Cloud,
  FolderOpen,
  Home,
  Info,
  LockKeyhole,
  Monitor,
  PanelTop,
  Plus,
  Settings,
  ShieldCheck,
  Sparkles,
  UserRound
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { syncLabel, type ProfileSummary, type SetupSurfaceId } from "../domain/home-state";

export type NavItem = "home" | "setup" | SetupSurfaceId;
export type SettingsSection = "account" | "cloud" | "device" | "protection" | "notifications" | "local-paths" | "privacy" | "about";

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

const settingsItems: Array<{ id: SettingsSection; label: string; icon: LucideIcon }> = [
  { id: "account", label: "Account", icon: UserRound },
  { id: "cloud", label: "Cloud", icon: Cloud },
  { id: "device", label: "Device", icon: Monitor },
  { id: "protection", label: "Protection", icon: ShieldCheck },
  { id: "notifications", label: "Notifications", icon: Bell },
  { id: "local-paths", label: "Local Paths", icon: FolderOpen },
  { id: "privacy", label: "Privacy", icon: LockKeyhole },
  { id: "about", label: "About", icon: Info }
];

export function Sidebar({
  profile,
  profiles,
  activeNav,
  settingsMode,
  activeSettingsSection,
  onSelectNav,
  onSelectSettingsSection,
  onOpenSettings,
  onCloseSettings,
  sidebarOpen
}: {
  profile: ProfileSummary | null;
  profiles: ProfileSummary[];
  activeNav: NavItem;
  settingsMode: boolean;
  activeSettingsSection: SettingsSection;
  onSelectNav: (nav: NavItem) => void;
  onSelectSettingsSection: (section: SettingsSection) => void;
  onOpenSettings: () => void;
  onCloseSettings: () => void;
  sidebarOpen: boolean;
}) {
  return (
    <aside className="sidebar" aria-hidden={!sidebarOpen}>
      {settingsMode ? (
        <SettingsNav activeSection={activeSettingsSection} onSelectSection={onSelectSettingsSection} onBack={onCloseSettings} />
      ) : (
        <>
          <ProfilePicker profile={profile} profiles={profiles} />
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
        <button className="icon-button account-settings-button" type="button" aria-label="Open settings" onClick={onOpenSettings}>
          <Settings size={16} />
        </button>
      </div>
    </aside>
  );
}

function ProfilePicker({ profile, profiles }: { profile: ProfileSummary | null; profiles: ProfileSummary[] }) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const visibleProfiles = profiles.length > 0 ? profiles : profile ? [profile] : [];

  useEffect(() => {
    if (!open) return;

    function handlePointerDown(event: PointerEvent) {
      if (menuRef.current?.contains(event.target as Node)) return;
      setOpen(false);
    }

    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, [open]);

  return (
    <div className="profile-menu" ref={menuRef}>
      <button className="profile-picker" type="button" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
        <div>
          <strong>{profile?.name ?? "No profile"}</strong>
          <span className="profile-sync">{profile ? syncLabel(profile) : "Not captured"}</span>
        </div>
        <ChevronsUpDown size={15} />
      </button>
      {open ? (
        <div className="profile-options">
          {visibleProfiles.map((item) => (
            <button className="profile-option" type="button" key={item.name} aria-current={item.name === profile?.name ? "true" : undefined}>
              <span>{item.name}</span>
              <span className="profile-sync">{syncLabel(item)}</span>
            </button>
          ))}
          <button className="profile-create" type="button">
            <Plus size={14} />
            <span>Create New Profile</span>
          </button>
        </div>
      ) : null}
    </div>
  );
}

function SettingsNav({
  activeSection,
  onSelectSection,
  onBack
}: {
  activeSection: SettingsSection;
  onSelectSection: (section: SettingsSection) => void;
  onBack: () => void;
}) {
  return (
    <nav className="nav-list">
      <button className="nav-item" type="button" onClick={onBack}>
        <ArrowLeft size={15} />
        <span>Back</span>
      </button>
      <div className="nav-separator" />
      {settingsItems.map((item) => {
        const Icon = item.icon;
        return (
          <button
            className={`nav-item ${activeSection === item.id ? "is-active" : ""}`}
            type="button"
            key={item.id}
            onClick={() => onSelectSection(item.id)}
          >
            <Icon size={15} />
            <span>{item.label}</span>
          </button>
        );
      })}
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
