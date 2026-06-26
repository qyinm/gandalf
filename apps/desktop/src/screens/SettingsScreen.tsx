import { useState } from "react";
import type { ReactNode } from "react";
import { invoke } from "@tauri-apps/api/core";

import type { SettingsSection } from "../components/Sidebar";
import type { DesktopHomeState } from "../domain/home-state";

interface SettingsModel {
  title: string;
  rows: Array<{ label: string; value: ReactNode }>;
}

interface NotificationPermission {
  granted: boolean;
  status: string;
}

const settingsCopy: Record<SettingsSection, SettingsModel> = {
  account: {
    title: "Account",
    rows: [
      { label: "User", value: "hippoo" },
      { label: "Plan", value: "Local only" }
    ]
  },
  cloud: {
    title: "Cloud",
    rows: [
      { label: "Status", value: "Not connected" },
      { label: "Sync", value: "Manual" }
    ]
  },
  device: {
    title: "Device",
    rows: [
      { label: "Name", value: "Work Mac" },
      { label: "Scope", value: "This device" }
    ]
  },
  protection: {
    title: "Protection",
    rows: [
      { label: "Status", value: "Off" },
      { label: "Apply mode", value: "Local review" }
    ]
  },
  notifications: {
    title: "Notifications",
    rows: [
      { label: "Risk alerts", value: "Medium and high risk" },
      { label: "Snapshot alerts", value: "Enabled" }
    ]
  },
  "local-paths": {
    title: "Local Paths",
    rows: [
      { label: "Profile store", value: "~/.gandalf" },
      { label: "Agent setup", value: "User and project paths" }
    ]
  },
  privacy: {
    title: "Privacy",
    rows: [
      { label: "Secrets", value: "Raw secrets are not uploaded" },
      { label: "Execution", value: "Commands, hooks, and skills are not executed" }
    ]
  },
  about: {
    title: "About",
    rows: [
      { label: "App", value: "Gandalf Desktop" },
      { label: "Mode", value: "Desktop MVP" }
    ]
  }
};

export function SettingsScreen({
  section,
  state,
  onSelectSection
}: {
  section: SettingsSection;
  state: DesktopHomeState;
  onSelectSection: (section: SettingsSection) => void;
}) {
  const model = settingsModel(section, state);
  const [notificationsEnabled, setNotificationsEnabled] = useState(false);
  const [notificationStatus, setNotificationStatus] = useState("Not requested");

  async function toggleNotifications(checked: boolean) {
    if (!checked) {
      setNotificationsEnabled(false);
      setNotificationStatus("Off");
      return;
    }

    setNotificationStatus("Requesting permission");
    try {
      const permission = await invoke<NotificationPermission>("request_notification_permission");
      setNotificationsEnabled(permission.granted);
      setNotificationStatus(permission.status);
    } catch {
      setNotificationsEnabled(false);
      setNotificationStatus("Unavailable");
    }
  }

  return (
    <div className="settings-screen">
      <div className="section-heading">
        <h1>{model.title}</h1>
        <span className="section-description">Settings</span>
      </div>
      <section className="settings-panel">
        {model.rows.map((row) => (
          <div className="settings-row" key={row.label}>
            <span className="settings-label">{row.label}</span>
            <strong>{row.value}</strong>
          </div>
        ))}
        {section === "notifications" ? (
          <div className="settings-row">
            <span className="settings-label">Desktop alerts</span>
            <label className="toggle-control">
              <input
                type="checkbox"
                checked={notificationsEnabled}
                onChange={(event) => void toggleNotifications(event.currentTarget.checked)}
              />
              <span className="toggle-track" />
              <strong>{notificationStatus}</strong>
            </label>
          </div>
        ) : null}
      </section>
      {section === "account" ? (
        <div className="action-row">
          <button className="action-button" type="button">Rename Device</button>
          <button className="action-button" type="button" onClick={() => onSelectSection("notifications")}>
            Notification Settings
          </button>
          <button className="action-button" type="button">Sign Out</button>
        </div>
      ) : null}
    </div>
  );
}

function settingsModel(section: SettingsSection, state: DesktopHomeState) {
  const base = settingsCopy[section];
  if (section === "account") {
    return {
      ...base,
      rows: [
        { label: "User", value: state.activeProfile?.name ?? "hippoo" },
        { label: "Plan", value: "Local only" },
        { label: "Device", value: "Work Mac" },
        { label: "Protection", value: state.protection === "on" ? "On" : "Off" },
        { label: "Notifications", value: "Medium and high risk" },
        { label: "Privacy", value: "Raw secrets are not uploaded" }
      ]
    };
  }
  if (section === "protection") {
    return {
      ...base,
      rows: [
        { label: "Status", value: state.protection === "on" ? "On" : "Off" },
        { label: "Highest risk", value: state.highestRisk ?? "None" }
      ]
    };
  }
  return base;
}
