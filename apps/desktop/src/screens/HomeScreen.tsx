import { Save } from "lucide-react";

import type { DesktopHomeState } from "../domain/home-state";
import { ActionButton } from "../components/ui";

export function HomeScreen({ state }: { state: DesktopHomeState }) {
  return (
    <div className="home-screen">
      <section className="overall">
        <div className="overall-heading">
          <div>
            <p className="eyebrow">Active Profile</p>
            <h1>{state.activeProfile?.name ?? "Not captured"}</h1>
          </div>
          <ActionButton icon={Save} label="Create Snapshot" primary />
        </div>
      </section>

      <section className="surface-strip">
        {state.surfaces.map((surface) => (
          <article className="surface-row" key={surface.id}>
            <span className="surface-label">{surface.label}</span>
            <strong>{surface.count}</strong>
          </article>
        ))}
        {state.surfaces.length === 0 ? <div className="inline-empty">No setup surfaces captured yet.</div> : null}
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
            </article>
          ))}
          {state.changelog.length === 0 ? <div className="inline-empty">No profile snapshots or setup changes yet.</div> : null}
        </div>
      </section>
    </div>
  );
}
