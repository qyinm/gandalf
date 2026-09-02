import * as React from "react";
import { Check, Copy, Eraser, RotateCcw } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

// ANSI Color Helpers
const C = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  dim: "\x1b[2m",
  italic: "\x1b[3m",
  underline: "\x1b[4m",
  // Gandalf Coral Brand Color (#ff5f6a)
  brand: "\x1b[38;2;255;95;106m",
  brandBold: "\x1b[1;38;2;255;95;106m",
  // Standard ANSI
  black: "\x1b[30m",
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  magenta: "\x1b[35m",
  cyan: "\x1b[36m",
  white: "\x1b[37m",
  brightGreen: "\x1b[92m",
  brightYellow: "\x1b[93m",
  brightWhite: "\x1b[97m",
  gray: "\x1b[90m",
};

interface CommandScenario {
  id: string;
  command: string;
  label: string;
  run: (term: Terminal) => Promise<void>;
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function typeWriter(term: Terminal, text: string, speed = 25) {
  for (const char of text) {
    term.write(char);
    await sleep(speed);
  }
}

const SCENARIOS: CommandScenario[] = [
  {
    id: "apply",
    label: "apply",
    command: "gandalf apply --dry-run",
    run: async (term) => {
      term.writeln("");
      await sleep(100);
      term.writeln(`${C.gray}🔍 Reading gandalf.toml manifest...${C.reset}`);
      await sleep(150);
      term.writeln(`${C.brightWhite}📦 Manifest version:${C.reset} 1.0 ${C.gray}(workspace: zero2one/backend)${C.reset}`);
      await sleep(100);
      term.writeln(`${C.brightWhite}🎯 Target agents detected:${C.reset} Claude Code, OpenAI Codex`);
      await sleep(150);
      term.writeln(`${C.gray}────────────────────────────────────────────────────────${C.reset}`);
      term.writeln(`${C.bold}${C.brightWhite}📋 Smart Merge Plan (Non-Destructive):${C.reset}`);
      await sleep(120);
      term.writeln(`  ${C.brand}+ [mcp]${C.reset}    postgres-db   ${C.gray}→ ~/.claude/settings.json${C.reset}`);
      await sleep(100);
      term.writeln(`  ${C.brand}+ [skill]${C.reset}  team-reviewer ${C.gray}→ ~/.claude/skills/reviewer.md${C.reset}`);
      await sleep(100);
      term.writeln(`  ${C.brand}+ [hook]${C.reset}   pre-save-lint ${C.gray}→ ~/.codex/config.toml${C.reset}`);
      await sleep(150);
      term.writeln(`${C.gray}────────────────────────────────────────────────────────${C.reset}`);
      term.writeln(`${C.yellow}🛡️  Safety Snapshot Created:${C.reset} preapply-20260902-160000`);
      await sleep(150);
      term.writeln(`${C.brightGreen}✓ Dry-run complete.${C.reset} 3 items ready to merge, 0 conflicts.`);
      term.writeln(`${C.gray}  Run 'gandalf apply --yes' to apply changes safely.${C.reset}`);
    },
  },
  {
    id: "check",
    label: "check",
    command: "gandalf check --ci",
    run: async (term) => {
      term.writeln("");
      await sleep(100);
      term.writeln(`${C.gray}🔍 Scanning local agent environments against gandalf.toml...${C.reset}`);
      await sleep(150);
      term.writeln(`${C.brightWhite}• Target: Claude Code (~/.claude/settings.json)${C.reset}`);
      await sleep(100);
      term.writeln(`  ${C.red}❌ Missing MCP server:${C.reset} 'postgres-db'`);
      await sleep(100);
      term.writeln(`  ${C.red}❌ Missing standardized skill:${C.reset} 'team-reviewer'`);
      await sleep(120);
      term.writeln(`${C.brightWhite}• Target: OpenAI Codex (~/.codex/config.toml)${C.reset}`);
      await sleep(100);
      term.writeln(`  ${C.brightGreen}✓ Guardrail hook 'pre-save-lint' synchronized${C.reset}`);
      await sleep(150);
      term.writeln(`${C.gray}────────────────────────────────────────────────────────${C.reset}`);
      term.writeln(`${C.yellow}⚡ Drift detected:${C.reset} 2 required items missing.`);
      term.writeln(`${C.red}${C.bold}❌ Check failed with exit code 1${C.reset} (blocking CI drift).`);
      term.writeln(`${C.gray}  Tip: Run 'gandalf apply' to sync missing configurations.${C.reset}`);
    },
  },
  {
    id: "init",
    label: "init",
    command: "gandalf init",
    run: async (term) => {
      term.writeln("");
      await sleep(100);
      term.writeln(`${C.gray}✨ Initializing Gandalf Agent Environment as Code...${C.reset}`);
      await sleep(120);
      term.writeln(`  ${C.brightGreen}✓ Created gandalf.toml${C.reset} (team specification template)`);
      await sleep(100);
      term.writeln(`  ${C.brightGreen}✓ Created .gandalf/skills/${C.reset} (versioned skill repository)`);
      await sleep(100);
      term.writeln(`  ${C.brightGreen}✓ Created .gandalf/hooks/${C.reset} (team guardrails)`);
      await sleep(100);
      term.writeln(`  ${C.brightWhite}✓ Configured Git tracking${C.reset} (.gitignore rules applied)`);
      await sleep(150);
      term.writeln(`${C.gray}────────────────────────────────────────────────────────${C.reset}`);
      term.writeln(`${C.bold}${C.brightWhite}🚀 Ready! Next steps:${C.reset}`);
      term.writeln(`  ${C.brand}1.${C.reset} Edit gandalf.toml to declare shared MCPs & skills`);
      term.writeln(`  ${C.brand}2.${C.reset} Commit to Git to share with your engineering team`);
      term.writeln(`  ${C.brand}3.${C.reset} Teammates run 'gandalf apply' to synchronize in seconds`);
    },
  },
  {
    id: "restore",
    label: "restore",
    command: "gandalf restore --snapshot preapply-20260902-160000 --apply",
    run: async (term) => {
      term.writeln("");
      await sleep(100);
      term.writeln(`${C.gray}🛡️  Loading safety snapshot: preapply-20260902-160000...${C.reset}`);
      await sleep(120);
      term.writeln(`• Verifying SHA-256 backup integrity: ${C.brightGreen}verified ✓${C.reset}`);
      await sleep(150);
      term.writeln(`${C.yellow}⏪ Restoring Claude Code configuration to pre-apply state...${C.reset}`);
      await sleep(120);
      term.writeln(`${C.yellow}⏪ Restoring OpenAI Codex configuration to pre-apply state...${C.reset}`);
      await sleep(150);
      term.writeln(`${C.gray}────────────────────────────────────────────────────────${C.reset}`);
      term.writeln(`${C.brightGreen}✓ Rollback completed successfully in 12ms.${C.reset}`);
      term.writeln(`${C.brightGreen}✓ Configuration verified: 0 errors, 100% restored.${C.reset}`);
    },
  },
];

export default function XTermTerminal() {
  const terminalRef = React.useRef<HTMLDivElement>(null);
  const termInstanceRef = React.useRef<Terminal | null>(null);
  const fitAddonRef = React.useRef<FitAddon | null>(null);

  const [activeScenarioId, setActiveScenarioId] = React.useState<string>("apply");
  const [isRunning, setIsRunning] = React.useState(false);
  const [copied, setCopied] = React.useState(false);

  // Command buffer state for real typing
  const inputBufferRef = React.useRef("");
  const isRunningRef = React.useRef(false);
  isRunningRef.current = isRunning;

  const prompt = React.useCallback((term: Terminal) => {
    term.write(`\r\n${C.brandBold}$ ${C.reset}`);
    inputBufferRef.current = "";
  }, []);

  const executeScenario = React.useCallback(async (scId: string) => {
    const term = termInstanceRef.current;
    if (!term || isRunningRef.current) return;

    const sc = SCENARIOS.find((s) => s.id === scId) ?? SCENARIOS[0];
    setActiveScenarioId(sc.id);
    setIsRunning(true);

    term.clear();
    term.write(`${C.brandBold}$ ${C.reset}`);
    await typeWriter(term, sc.command, 20);
    await sc.run(term);

    setIsRunning(false);
    prompt(term);
  }, [prompt]);

  // Initialize xterm.js
  React.useEffect(() => {
    if (!terminalRef.current) return;

    const term = new Terminal({
      cursorBlink: true,
      cursorStyle: "block",
      fontFamily: "'DM Mono', 'JetBrains Mono', 'Menlo', 'Monaco', monospace",
      fontSize: 13,
      lineHeight: 1.5,
      letterSpacing: 0,
      theme: {
        background: "#09090b",
        foreground: "#f4f4f5",
        cursor: "#ff5f6a",
        selectionBackground: "rgba(255, 95, 106, 0.3)",
        black: "#18181b",
        red: "#ff5f6a",
        green: "#4ade80",
        yellow: "#fbbf24",
        blue: "#60a5fa",
        magenta: "#f472b6",
        cyan: "#38bdf8",
        white: "#f4f4f5",
        brightRed: "#ff7a83",
        brightGreen: "#86efac",
        brightYellow: "#fde047",
        brightBlue: "#93c5fd",
        brightWhite: "#ffffff",
      },
      convertEol: true,
      allowTransparency: false,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);

    try {
      fitAddon.fit();
    } catch {
      /* ignore */
    }

    termInstanceRef.current = term;
    fitAddonRef.current = fitAddon;

    // Handle User Keyboard Inputs
    term.onData(async (data) => {
      if (isRunningRef.current) return;

      // Enter key
      if (data === "\r") {
        const cmd = inputBufferRef.current.trim().toLowerCase();
        term.writeln("");

        if (!cmd) {
          prompt(term);
          return;
        }

        if (cmd === "clear") {
          term.clear();
          prompt(term);
          return;
        }

        if (cmd === "help") {
          term.writeln(`${C.bold}${C.brightWhite}Gandalf CLI Commands:${C.reset}`);
          term.writeln(`  ${C.brand}gandalf init${C.reset}     - Scaffold team manifest (gandalf.toml)`);
          term.writeln(`  ${C.brand}gandalf check${C.reset}    - Detect configuration drift across agents`);
          term.writeln(`  ${C.brand}gandalf apply${C.reset}    - Non-destructively merge team manifest`);
          term.writeln(`  ${C.brand}gandalf restore${C.reset}  - Instant rollback to safety snapshot`);
          term.writeln(`  ${C.gray}clear            - Clear terminal screen${C.reset}`);
          prompt(term);
          return;
        }

        if (cmd.includes("apply")) {
          await executeScenario("apply");
        } else if (cmd.includes("check")) {
          await executeScenario("check");
        } else if (cmd.includes("init")) {
          await executeScenario("init");
        } else if (cmd.includes("restore")) {
          await executeScenario("restore");
        } else {
          term.writeln(`${C.red}gandalf: command not found: '${cmd}'. Try 'apply', 'check', 'init', 'restore', or 'help'${C.reset}`);
          prompt(term);
        }
        return;
      }

      // Backspace
      if (data === "\u007f" || data === "\b") {
        if (inputBufferRef.current.length > 0) {
          inputBufferRef.current = inputBufferRef.current.slice(0, -1);
          term.write("\b \b");
        }
        return;
      }

      // Tab auto-complete
      if (data === "\t") {
        const current = inputBufferRef.current.trim();
        const matches = ["apply", "check", "init", "restore", "help", "clear"].filter((c) =>
          c.startsWith(current)
        );
        if (matches.length === 1) {
          const completion = matches[0].slice(current.length);
          inputBufferRef.current += completion;
          term.write(completion);
        }
        return;
      }

      // Normal character input
      if (data >= " " && data <= "~") {
        inputBufferRef.current += data;
        term.write(data);
      }
    });

    // Run initial apply scenario
    executeScenario("apply");

    // Handle Window Resize
    const handleResize = () => {
      try {
        fitAddon.fit();
      } catch {
        /* ignore */
      }
    };
    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("resize", handleResize);
      term.dispose();
    };
  }, [executeScenario, prompt]);

  const handleCopy = async () => {
    try {
      const activeSc = SCENARIOS.find((s) => s.id === activeScenarioId) ?? SCENARIOS[0];
      await navigator.clipboard.writeText(activeSc.command);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* ignore */
    }
  };

  const handleClear = () => {
    const term = termInstanceRef.current;
    if (term && !isRunning) {
      term.clear();
      prompt(term);
    }
  };

  return (
    <div className="terminal-shell">
      {/* macOS Native Title Bar */}
      <div className="terminal-shell__bar">
        {/* macOS Traffic Lights */}
        <div className="terminal-shell__dots">
          <span className="dot dot--red" />
          <span className="dot dot--yellow" />
          <span className="dot dot--green" />
        </div>

        {/* Quick-Click Command Preset Tabs */}
        <div className="terminal-shell__tabs-wrapper">
          <div className="terminal-tabs-list">
            {SCENARIOS.map((sc) => {
              const isActive = sc.id === activeScenarioId;
              return (
                <button
                  key={sc.id}
                  type="button"
                  onClick={() => executeScenario(sc.id)}
                  disabled={isRunning}
                  className={`terminal-tab-trigger ${isActive ? "is-active" : ""}`}
                >
                  <span className="terminal-tab-prefix">gandalf </span>
                  {sc.label}
                </button>
              );
            })}
          </div>
        </div>

        {/* Action Buttons: Clear, Replay, Copy */}
        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={handleClear}
            className="terminal-copy-btn"
            title="Clear terminal"
            disabled={isRunning}
          >
            <Eraser size={11} />
            <span className="hidden sm:inline">Clear</span>
          </button>

          <button
            type="button"
            onClick={() => executeScenario(activeScenarioId)}
            className="terminal-copy-btn"
            title="Replay execution"
            disabled={isRunning}
          >
            <RotateCcw size={11} className={isRunning ? "animate-spin" : ""} />
            <span className="hidden sm:inline">Replay</span>
          </button>

          <button
            type="button"
            onClick={handleCopy}
            className="terminal-copy-btn"
            title="Copy command"
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

      {/* Real xterm.js Canvas Container */}
      <div className="xterm-container-wrapper">
        <div ref={terminalRef} className="xterm-target-screen" />
      </div>

      {/* Real Terminal Status Footer */}
      <div className="real-terminal-footer">
        <div className="flex items-center gap-2 text-xs font-mono text-zinc-400">
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
          </span>
          <span className="font-semibold text-zinc-200">xterm.js</span>
          <span className="text-zinc-600">•</span>
          <span className="text-zinc-400">Type directly or click tabs above</span>
        </div>
        <div className="hidden sm:block text-xs font-mono text-zinc-500">
          Try <span className="text-[#ff5f6a]">apply</span>, <span className="text-[#ff5f6a]">check</span>, <span className="text-[#ff5f6a]">init</span>, or <span className="text-[#ff5f6a]">restore</span>
        </div>
      </div>
    </div>
  );
}
