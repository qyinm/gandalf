import * as React from "react";
import { Check, Copy } from "lucide-react";
import { Tabs, TabsList, TabsTrigger } from "./Tabs";

interface CommandScenario {
  id: string;
  tabLabel: string;
  command: string;
  leftLines: { text: string; kind: 'prompt' | 'title' | 'muted' | 'space' | 'warn' | 'change' | 'danger' | 'success' }[];
  rightTitle: string;
  rightLines: string[];
}

const scenarios: CommandScenario[] = [
  {
    id: "apply",
    tabLabel: "gandalf apply",
    command: "gandalf apply --dry-run",
    leftLines: [
      { text: "$ gandalf apply --dry-run", kind: "prompt" },
      { text: "📦 Team Manifest: gandalf.toml (version 1.0)", kind: "title" },
      { text: "🎯 Target Agents: claude-code, codex", kind: "muted" },
      { text: "", kind: "space" },
      { text: "🔍 Review Changes before Apply:", kind: "warn" },
      { text: "  + mcp    postgres-db   (staging db connector)", kind: "change" },
      { text: "  + skill  team-reviewer (PR compliance)", kind: "change" },
      { text: "  + hook   pre-save-lint (before_save guard)", kind: "change" },
    ],
    rightTitle: "Smart Merge Preview",
    rightLines: [
      "Non-Destructive Target:",
      "• ~/.claude/settings.json",
      "• ~/.codex/config.toml",
      "",
      "🛡️ Safety Backup Enabled:",
      "preapply-manifest-20260902-120000",
      "",
      "✓ Personal settings preserved",
      "enter apply · esc cancel",
    ],
  },
  {
    id: "check",
    tabLabel: "gandalf check",
    command: "gandalf check --ci",
    leftLines: [
      { text: "$ gandalf check --ci", kind: "prompt" },
      { text: "🔍 Comparing local state against gandalf.toml...", kind: "title" },
      { text: "", kind: "space" },
      { text: "Claude Code (~/.claude/settings.json):", kind: "muted" },
      { text: "  ❌ missing mcp: postgres-db", kind: "danger" },
      { text: "  ❌ missing skill: team-reviewer", kind: "danger" },
      { text: "", kind: "space" },
      { text: "⚡ Drift detected. Exit code 1 for CI.", kind: "warn" },
    ],
    rightTitle: "CI Drift Detection",
    rightLines: [
      "Zero-Mutation Scan",
      "",
      "• Read-only discovery",
      "• Strict path confinement",
      "• ${ENV_VAR} verified",
      "",
      "Enforce team alignment",
      "in GitHub Actions & PRs",
    ],
  },
  {
    id: "restore",
    tabLabel: "gandalf restore",
    command: "gandalf restore --snapshot preapply-manifest-20260902-120000 --apply",
    leftLines: [
      { text: "$ gandalf restore --snapshot preapply-... --apply", kind: "prompt" },
      { text: "🛡️ Loading safety snapshot...", kind: "title" },
      { text: "Target: ~/.claude/settings.json", kind: "muted" },
      { text: "", kind: "space" },
      { text: "⏪ Restoring previous configuration...", kind: "warn" },
      { text: "  ✓ Rolled back to pre-apply state", kind: "success" },
      { text: "  ✓ Configuration verified (0 errors)", kind: "success" },
    ],
    rightTitle: "Instant 1-Click Rollback",
    rightLines: [
      "Content-Backed Saves",
      "",
      "• Full JSON/TOML snapshot",
      "• Atomic file swap",
      "• Zero configuration loss",
      "",
      "Reversible by default.",
    ],
  },
];

export default function TerminalViewer() {
  const [activeTab, setActiveTab] = React.useState("apply");
  const [copied, setCopied] = React.useState(false);

  const activeScenario = scenarios.find((s) => s.id === activeTab) ?? scenarios[0];

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(activeScenario.command);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* ignore */
    }
  };

  return (
    <div className="terminal-shell">
      {/* Sleek macOS Terminal Header */}
      <div className="terminal-shell__bar">
        {/* macOS Traffic Lights */}
        <div className="terminal-shell__dots">
          <span className="dot dot--red" />
          <span className="dot dot--yellow" />
          <span className="dot dot--green" />
        </div>

        {/* Centered Dark Segmented Tabs */}
        <div className="terminal-shell__tabs-wrapper">
          <Tabs value={activeTab} onValueChange={setActiveTab}>
            <TabsList className="terminal-tabs-list">
              {scenarios.map((s) => (
                <TabsTrigger
                  key={s.id}
                  value={s.id}
                  className="terminal-tab-trigger"
                >
                  <span className="terminal-tab-prefix">gandalf </span>
                  {s.id}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>

        {/* Dark Mode Copy Button */}
        <button
          type="button"
          onClick={handleCopy}
          className="terminal-copy-btn"
          title="Copy command"
          aria-label="Copy terminal command"
        >
          {copied ? (
            <>
              <Check size={12} className="text-emerald-400" />
              <span>Copied</span>
            </>
          ) : (
            <>
              <Copy size={12} />
              <span>Copy</span>
            </>
          )}
        </button>
      </div>

      {/* Terminal Body */}
      <div className="terminal-shell__body">
        <div className="terminal-shell__main">
          {activeScenario.leftLines.map((line, i) => (
            <div
              key={i}
              className={`terminal-shell__line terminal-shell__line--${line.kind}`}
            >
              {line.text || "\u00a0"}
            </div>
          ))}
        </div>

        <div className="terminal-shell__side">
          <div className="terminal-shell__side-title">{activeScenario.rightTitle}</div>
          <div className="terminal-shell__side-body">
            {activeScenario.rightLines.map((line, i) => (
              <div key={i}>{line || "\u00a0"}</div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
