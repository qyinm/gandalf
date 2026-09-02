import * as React from "react";
import { Check, Copy, RotateCcw } from "lucide-react";

interface TerminalLine {
  id: string;
  type: "prompt" | "info" | "success" | "warn" | "error" | "brand" | "dim" | "heading";
  text: string;
  delay?: number;
}

interface Scenario {
  id: string;
  name: string;
  command: string;
  description: string;
  lines: Omit<TerminalLine, "id">[];
}

const SCENARIOS: Scenario[] = [
  {
    id: "apply",
    name: "gandalf apply",
    command: "gandalf apply --dry-run",
    description: "Smart Merge preview & pre-apply snapshot",
    lines: [
      { type: "prompt", text: "$ gandalf apply --dry-run" },
      { type: "dim", text: "🔍 Reading gandalf.toml manifest..." },
      { type: "info", text: "📦 Manifest version: 1.0 (workspace: zero2one/backend)" },
      { type: "info", text: "🎯 Target agents detected: Claude Code, OpenAI Codex" },
      { type: "dim", text: "──────────────────────────────────────────────────────" },
      { type: "heading", text: "📋 Smart Merge Plan (Non-Destructive):" },
      { type: "brand", text: "  + [mcp]    postgres-db   → ~/.claude/settings.json" },
      { type: "brand", text: "  + [skill]  team-reviewer → ~/.claude/skills/reviewer.md" },
      { type: "brand", text: "  + [hook]   pre-save-lint → ~/.codex/config.toml" },
      { type: "dim", text: "──────────────────────────────────────────────────────" },
      { type: "warn", text: "🛡️ Safety Snapshot Created: preapply-20260902-154920" },
      { type: "success", text: "✓ Dry-run complete. 3 items ready to merge, 0 conflicts." },
      { type: "dim", text: "  Run 'gandalf apply --yes' to apply changes safely." },
    ],
  },
  {
    id: "check",
    name: "gandalf check",
    command: "gandalf check --ci",
    description: "Automated drift detection in CI",
    lines: [
      { type: "prompt", text: "$ gandalf check --ci" },
      { type: "dim", text: "🔍 Scanning local agent environments against gandalf.toml..." },
      { type: "info", text: "• Target: Claude Code (~/.claude/settings.json)" },
      { type: "error", text: "  ❌ Missing MCP server: 'postgres-db'" },
      { type: "error", text: "  ❌ Missing standardized skill: 'team-reviewer'" },
      { type: "info", text: "• Target: OpenAI Codex (~/.codex/config.toml)" },
      { type: "success", text: "  ✓ Guardrail hook 'pre-save-lint' synchronized" },
      { type: "dim", text: "──────────────────────────────────────────────────────" },
      { type: "warn", text: "⚡ Drift detected: 2 required items missing." },
      { type: "error", text: "❌ Check failed with exit code 1 (blocking CI drift)." },
      { type: "dim", text: "  Tip: Run 'gandalf apply' to sync missing configurations." },
    ],
  },
  {
    id: "init",
    name: "gandalf init",
    command: "gandalf init",
    description: "Scaffold team manifest & shared skills",
    lines: [
      { type: "prompt", text: "$ gandalf init" },
      { type: "dim", text: "✨ Initializing Gandalf Agent Environment as Code..." },
      { type: "success", text: "  ✓ Created gandalf.toml (team specification template)" },
      { type: "success", text: "  ✓ Created .gandalf/skills/ (versioned skill repository)" },
      { type: "success", text: "  ✓ Created .gandalf/hooks/ (team guardrails)" },
      { type: "info", text: "  ✓ Configured Git tracking (.gitignore rules applied)" },
      { type: "dim", text: "──────────────────────────────────────────────────────" },
      { type: "heading", text: "🚀 Ready! Next steps:" },
      { type: "brand", text: "  1. Edit gandalf.toml to declare shared MCPs & skills" },
      { type: "brand", text: "  2. Commit to Git to share with your entire engineering team" },
      { type: "brand", text: "  3. Teammates run 'gandalf apply' to synchronize in seconds" },
    ],
  },
  {
    id: "restore",
    name: "gandalf restore",
    command: "gandalf restore --snapshot preapply-20260902-154920 --apply",
    description: "Instant 1-click rollback with verified integrity",
    lines: [
      { type: "prompt", text: "$ gandalf restore --snapshot preapply-20260902-154920 --apply" },
      { type: "dim", text: "🛡️ Loading safety snapshot: preapply-20260902-154920..." },
      { type: "info", text: "• Verifying SHA-256 backup integrity: verified ✓" },
      { type: "warn", text: "⏪ Restoring Claude Code configuration to pre-apply state..." },
      { type: "warn", text: "⏪ Restoring OpenAI Codex configuration to pre-apply state..." },
      { type: "dim", text: "──────────────────────────────────────────────────────" },
      { type: "success", text: "✓ Rollback completed successfully in 12ms." },
      { type: "success", text: "✓ Configuration verified: 0 errors, 100% restored." },
    ],
  },
];

export default function InteractiveTerminal() {
  const [activeScenarioId, setActiveScenarioId] = React.useState<string>("apply");
  const [displayedLines, setDisplayedLines] = React.useState<TerminalLine[]>([]);
  const [isTyping, setIsTyping] = React.useState(false);
  const [customInput, setCustomInput] = React.useState("");
  const [copied, setCopied] = React.useState(false);
  const terminalEndRef = React.useRef<HTMLDivElement>(null);

  const scenario = SCENARIOS.find((s) => s.id === activeScenarioId) ?? SCENARIOS[0];

  // Run simulation whenever active scenario changes
  const runScenario = React.useCallback((sc: Scenario) => {
    setIsTyping(true);
    setDisplayedLines([]);

    let currentIdx = 0;
    const interval = setInterval(() => {
      if (currentIdx < sc.lines.length) {
        const line = sc.lines[currentIdx];
        setDisplayedLines((prev) => [
          ...prev,
          { ...line, id: `${sc.id}-${currentIdx}-${Date.now()}` },
        ]);
        currentIdx++;
      } else {
        clearInterval(interval);
        setIsTyping(false);
      }
    }, 60);

    return () => clearInterval(interval);
  }, []);

  React.useEffect(() => {
    const cleanup = runScenario(scenario);
    return cleanup;
  }, [scenario, runScenario]);

  // Handle custom command execution
  const handleCustomSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const cmd = customInput.trim().toLowerCase();
    if (!cmd) return;

    if (cmd === "clear") {
      setDisplayedLines([]);
      setCustomInput("");
      return;
    }

    if (cmd.includes("apply")) {
      setActiveScenarioId("apply");
    } else if (cmd.includes("check")) {
      setActiveScenarioId("check");
    } else if (cmd.includes("init")) {
      setActiveScenarioId("init");
    } else if (cmd.includes("restore")) {
      setActiveScenarioId("restore");
    } else if (cmd === "help") {
      setDisplayedLines((prev) => [
        ...prev,
        { id: `help-prompt-${Date.now()}`, type: "prompt", text: `$ ${customInput}` },
        { id: `help-1-${Date.now()}`, type: "info", text: "Gandalf CLI Commands:" },
        { id: `help-2-${Date.now()}`, type: "brand", text: "  gandalf init     - Scaffold team manifest (gandalf.toml)" },
        { id: `help-3-${Date.now()}`, type: "brand", text: "  gandalf check    - Detect configuration drift across agents" },
        { id: `help-4-${Date.now()}`, type: "brand", text: "  gandalf apply    - Non-destructively merge team manifest" },
        { id: `help-5-${Date.now()}`, type: "brand", text: "  gandalf restore  - Instant rollback to safety snapshot" },
        { id: `help-6-${Date.now()}`, type: "dim", text: "  clear            - Clear terminal screen" },
      ]);
    } else {
      setDisplayedLines((prev) => [
        ...prev,
        { id: `err-prompt-${Date.now()}`, type: "prompt", text: `$ ${customInput}` },
        { id: `err-out-${Date.now()}`, type: "error", text: `gandalf: command not found: '${cmd}'. Try: apply, check, init, restore, help` },
      ]);
    }
    setCustomInput("");
  };

  const handleCopy = async () => {
    try {
      const fullText = displayedLines.map((l) => l.text).join("\n");
      await navigator.clipboard.writeText(fullText || scenario.command);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* ignore */
    }
  };

  return (
    <div className="terminal-shell">
      {/* macOS Top Bar */}
      <div className="terminal-shell__bar">
        <div className="terminal-shell__dots">
          <span className="dot dot--red" />
          <span className="dot dot--yellow" />
          <span className="dot dot--green" />
        </div>

        {/* Preset Command Switcher Tabs */}
        <div className="terminal-shell__tabs-wrapper">
          <div className="terminal-tabs-list">
            {SCENARIOS.map((sc) => {
              const isActive = sc.id === activeScenarioId;
              return (
                <button
                  key={sc.id}
                  type="button"
                  onClick={() => setActiveScenarioId(sc.id)}
                  className={`terminal-tab-trigger ${isActive ? "is-active" : ""}`}
                >
                  <span className="terminal-tab-prefix">gandalf </span>
                  {sc.id}
                </button>
              );
            })}
          </div>
        </div>

        {/* Action Buttons: Replay & Copy */}
        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => runScenario(scenario)}
            className="terminal-copy-btn"
            title="Replay execution"
            disabled={isTyping}
          >
            <RotateCcw size={11} className={isTyping ? "animate-spin" : ""} />
            <span className="hidden sm:inline">Replay</span>
          </button>

          <button
            type="button"
            onClick={handleCopy}
            className="terminal-copy-btn"
            title="Copy terminal output"
          >
            {copied ? (
              <>
                <Check size={11} className="text-emerald-400" />
                <span>Copied</span>
              </>
            ) : (
              <>
                <Copy size={11} />
                <span>Copy</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* Terminal Screen Body */}
      <div className="real-terminal-body">
        <div className="real-terminal-screen">
          {displayedLines.map((line) => (
            <div
              key={line.id}
              className={`real-terminal-line real-terminal-line--${line.type}`}
            >
              {line.text}
            </div>
          ))}

          {/* Typing Cursor */}
          {isTyping && (
            <div className="real-terminal-line text-zinc-500 flex items-center gap-1">
              <span className="inline-block w-2 h-4 bg-[#ff5f6a] animate-pulse" />
            </div>
          )}

          {/* Interactive Shell Input Line */}
          {!isTyping && (
            <form onSubmit={handleCustomSubmit} className="real-terminal-input-row">
              <span className="real-terminal-prompt-sigil">$</span>
              <input
                type="text"
                value={customInput}
                onChange={(e) => setCustomInput(e.target.value)}
                placeholder="type 'apply', 'check', 'init', 'restore', or 'help'..."
                className="real-terminal-input"
                aria-label="Interactive terminal input"
              />
              <button type="submit" className="sr-only">Run</button>
            </form>
          )}

          <div ref={terminalEndRef} />
        </div>

        {/* Live Execution Status Footer */}
        <div className="real-terminal-footer">
          <div className="flex items-center gap-2 text-xs font-mono text-zinc-400">
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
            </span>
            <span>CLI v0.7.0</span>
            <span className="text-zinc-600">•</span>
            <span className="text-zinc-400">{scenario.description}</span>
          </div>
          <div className="hidden sm:block text-xs font-mono text-zinc-500">
            Press <kbd className="bg-zinc-800 border border-zinc-700 px-1 py-0.5 rounded text-[10px] text-zinc-300">Enter</kbd> to execute
          </div>
        </div>
      </div>
    </div>
  );
}
