import { AlertTriangle, CheckCircle2 } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { RiskLevel } from "../domain/home-state";

export function ActionButton({ icon: Icon, label, primary = false }: { icon: LucideIcon; label: string; primary?: boolean }) {
  return (
    <button className={`action-button ${primary ? "is-primary" : ""}`} type="button">
      <Icon size={15} />
      <span>{label}</span>
    </button>
  );
}

export function RiskBadge({ risk }: { risk: RiskLevel }) {
  const Icon = risk === "low" ? CheckCircle2 : AlertTriangle;
  return (
    <span className={`risk-badge risk-${risk}`}>
      <Icon size={13} />
      {risk}
    </span>
  );
}
