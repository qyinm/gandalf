import type { LucideIcon } from "lucide-react";

export function ActionButton({ icon: Icon, label, primary = false }: { icon: LucideIcon; label: string; primary?: boolean }) {
  return (
    <button className={`action-button ${primary ? "is-primary" : ""}`} type="button">
      <Icon size={15} />
      <span>{label}</span>
    </button>
  );
}
