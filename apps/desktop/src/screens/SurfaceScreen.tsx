import type { InventoryItem, SetupSurface } from "../domain/home-state";
import type { NavItem } from "../components/Sidebar";

export function SurfaceScreen({ surface, nav, inventory }: { surface?: SetupSurface; nav: NavItem; inventory: InventoryItem[] }) {
  const title = surface?.label ?? nav;
  return (
    <div className="surface-screen">
      <div className="section-heading">
        <h1>{title}</h1>
        <span className="section-description">{surface?.description ?? "No captured data for this setup surface"}</span>
      </div>
      <InventoryList items={inventory} title={title} />
    </div>
  );
}

function InventoryList({ items, title }: { items: InventoryItem[]; title: string }) {
  return (
    <div className="inventory-rows">
      {items.map((item) => (
        <article className="inventory-row" key={item.id}>
          <div>
            <strong>{item.name}</strong>
            <span className="inventory-meta">{item.agent} - {item.scope} - {item.sourcePath}</span>
          </div>
          {item.status ? <span className="inventory-status">{item.status}</span> : null}
          {item.detail ? <p className="inventory-detail">{item.detail}</p> : null}
        </article>
      ))}
      {items.length === 0 ? <div className="inventory-empty">No {title.toLowerCase()} found.</div> : null}
    </div>
  );
}
