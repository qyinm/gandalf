import type { PointerEvent } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { GitCommit, PanelLeftClose, PanelLeftOpen } from "lucide-react";

import type { ChangelogEntry } from "../domain/home-state";
import { RiskBadge } from "./ui";

export function Titlebar({
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
            <span className="popover-label">Current Snapshot</span>
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
            {changelog.length === 0 ? <div className="popover-empty">No snapshots yet</div> : null}
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
