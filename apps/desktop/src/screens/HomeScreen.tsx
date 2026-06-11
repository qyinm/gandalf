import { GitCompare, RefreshCcw, RotateCcw, Save, Upload } from "lucide-react";

import { syncLabel, type DesktopHomeState } from "../domain/home-state";
import { ActionButton, RiskBadge } from "../components/ui";

export function HomeScreen({ state }: { state: DesktopHomeState }) {
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
            <p className="surface-description">{surface.description}</p>
          </article>
        ))}
        {state.surfaces.length === 0 ? <div className="empty-panel">No setup surfaces captured yet.</div> : null}
      </section>

      <section className="changelog">
        <div className="section-heading">
          <h2>Changelog</h2>
          <span className="section-description">Latest profile snapshots and setup changes</span>
        </div>
        <div className="timeline-list">
          {state.changelog.map((entry) => (
            <article className="timeline-row" key={entry.id}>
              <code>{entry.id}</code>
              <div>
                <strong>{entry.title}</strong>
                <span className="timeline-meta">{entry.source} - {entry.time}</span>
              </div>
              <RiskBadge risk={entry.risk} />
            </article>
          ))}
          {state.changelog.length === 0 ? <div className="empty-panel">No profile snapshots or setup changes yet.</div> : null}
        </div>
      </section>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="metric">
      <span className="metric-label">{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
