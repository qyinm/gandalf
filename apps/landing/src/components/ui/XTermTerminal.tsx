import * as React from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

// ANSI Color Helpers
const C = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  dim: "\x1b[2m",
  brand: "\x1b[38;2;255;95;106m",
  brandBold: "\x1b[1;38;2;255;95;106m",
  yellow: "\x1b[33m",
  gray: "\x1b[90m",
};

interface CommandScenario {
  id: string;
  command: string;
  label: string;
  lines: string[];
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// Authentic Gandalf CLI Outputs
const SCENARIOS: CommandScenario[] = [
  {
    id: "apply",
    label: "apply",
    command: "gandalf apply --dry-run",
    lines: [
      "Team Manifest: zero2one-backend (version: 1.0)",
      "Target Agents: claude-code, codex",
      "",
      "Review Changes before Apply:",
      "  [1] [claude-code] add MCP server 'postgres-db' (~/.claude/settings.json)",
      "  [2] [claude-code] add skill 'team-reviewer' (~/.claude/skills/reviewer.md)",
      "  [3] [codex] add guardrail hook 'pre-save-lint' (~/.codex/config.toml)",
      "",
      `${C.gray}[Dry-run mode] No changes were written to disk.${C.reset}`,
    ],
  },
  {
    id: "check",
    label: "check",
    command: "gandalf check --ci",
    lines: [
      "Team Manifest: zero2one-backend (version: 1.0)",
      "Target Agents: claude-code, codex",
      "",
      `${C.yellow}[DRIFT DETECTED]${C.reset} The following items are missing or out of sync:`,
      "  [1] [claude-code] mcp: postgres-db (~/.claude/settings.json)",
      "  [2] [claude-code] skill: team-reviewer (~/.claude/skills/reviewer.md)",
      "",
      `${C.gray}Run 'gandalf apply' to synchronize your agent environment.${C.reset}`,
    ],
  },
  {
    id: "init",
    label: "init",
    command: "gandalf init",
    lines: [
      "Created starter team manifest: /Users/dev/project/gandalf.toml",
      "Created shared directory: /Users/dev/project/.gandalf/skills/",
      "",
      "Next steps:",
      "  1. Edit gandalf.toml to declare shared MCP servers and skills.",
      "  2. Commit gandalf.toml to your Git repository.",
      "  3. Team members run 'gandalf apply' to synchronize.",
    ],
  },
  {
    id: "restore",
    label: "restore",
    command: "gandalf restore --snapshot preapply-manifest-20260902-160000 --apply",
    lines: [
      "Loaded snapshot: preapply-manifest-20260902-160000",
      "Verifying target agent configuration...",
      "",
      "Restoring:",
      "  <- ~/.claude/settings.json",
      "  <- ~/.codex/config.toml",
      "",
      "Restore completed successfully. 2 configuration files restored.",
    ],
  },
];

export default function XTermTerminal() {
  const terminalRef = React.useRef<HTMLDivElement>(null);
  const termInstanceRef = React.useRef<Terminal | null>(null);
  const fitAddonRef = React.useRef<FitAddon | null>(null);

  const [activeScenarioId, setActiveScenarioId] = React.useState<string>("apply");
  const [isRunning, setIsRunning] = React.useState(false);

  const inputBufferRef = React.useRef("");
  const isRunningRef = React.useRef(false);
  isRunningRef.current = isRunning;

  const executeScenario = React.useCallback(async (scId: string) => {
    const term = termInstanceRef.current;
    if (!term || isRunningRef.current) return;

    const sc = SCENARIOS.find((s) => s.id === scId) ?? SCENARIOS[0];
    setActiveScenarioId(sc.id);
    setIsRunning(true);

    term.clear();
    term.write(`${C.brandBold}$ ${C.reset}`);

    // Type the command smoothly
    for (const char of sc.command) {
      term.write(char);
      await sleep(15);
    }
    term.writeln("");
    await sleep(60);

    // Stream lines without extra $ prompts
    for (const line of sc.lines) {
      term.writeln(line);
      await sleep(35);
    }

    term.scrollToTop();
    setIsRunning(false);
    inputBufferRef.current = "";
  }, []);

  // Initialize xterm.js
  React.useEffect(() => {
    if (!terminalRef.current) return;

    const term = new Terminal({
      cursorBlink: true,
      cursorStyle: "block",
      fontFamily: "'DM Mono', 'JetBrains Mono', 'Menlo', 'Monaco', monospace",
      fontSize: 13.5,
      lineHeight: 1.6,
      letterSpacing: 0,
      rows: 14,
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
          term.write(`${C.brandBold}$ ${C.reset}`);
          return;
        }

        if (cmd === "clear") {
          term.clear();
          term.write(`${C.brandBold}$ ${C.reset}`);
          inputBufferRef.current = "";
          return;
        }

        if (cmd === "help") {
          term.writeln(`Gandalf CLI Commands:`);
          term.writeln(`  gandalf init     - Initialize team agent manifest`);
          term.writeln(`  gandalf check    - Check for drift between manifest and local agents`);
          term.writeln(`  gandalf apply    - Apply team manifest to local agents`);
          term.writeln(`  gandalf restore  - Restore agent configs from snapshot`);
          term.writeln(`  clear            - Clear terminal screen`);
          term.write(`\r\n${C.brandBold}$ ${C.reset}`);
          inputBufferRef.current = "";
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
          term.writeln(`gandalf: command not found: '${cmd}'. Try 'apply', 'check', 'init', 'restore', or 'help'`);
          term.write(`\r\n${C.brandBold}$ ${C.reset}`);
          inputBufferRef.current = "";
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
  }, [executeScenario]);

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

        {/* Command Preset Tabs */}
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

        {/* Right spacing balance */}
        <div className="terminal-shell__dots invisible" aria-hidden="true">
          <span className="dot" />
          <span className="dot" />
          <span className="dot" />
        </div>
      </div>

      {/* Real xterm.js Canvas Container */}
      <div className="xterm-container-wrapper">
        <div ref={terminalRef} className="xterm-target-screen" />
      </div>
    </div>
  );
}
