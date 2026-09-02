import * as React from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

// ANSI Color Helpers
const C = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  brand: "\x1b[38;2;255;95;106m",
  brandBold: "\x1b[1;38;2;255;95;106m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  cyan: "\x1b[36m",
  white: "\x1b[37m",
  gray: "\x1b[90m",
};

interface TabView {
  id: string;
  label: string;
  command?: string;
  render: (term: Terminal) => void;
}

const VIEWS: TabView[] = [
  {
    id: "check",
    label: "gandalf check",
    command: "gandalf check",
    render: (term) => {
      term.writeln(`${C.brandBold}$ ${C.reset}gandalf check`);
      term.writeln(`Team Manifest: zero2one-backend (version: 1.0)`);
      term.writeln(`Target Agents: claude-code, codex`);
      term.writeln("");
      term.writeln(`${C.yellow}[DRIFT DETECTED]${C.reset} The following items are missing or out of sync:`);
      term.writeln(`  [1] [claude-code] mcp: postgres-db (~/.claude/settings.json)`);
      term.writeln(`  [2] [claude-code] skill: team-reviewer (~/.claude/skills/reviewer.md)`);
      term.writeln("");
      term.writeln(`${C.gray}Run 'gandalf apply' to synchronize your agent environment.${C.reset}`);
    },
  },
  {
    id: "manifest",
    label: "gandalf.toml",
    render: (term) => {
      term.writeln(`${C.gray}# gandalf.toml (team agent environment specification)${C.reset}`);
      term.writeln(`${C.cyan}version${C.reset} = ${C.green}"1.0"${C.reset}`);
      term.writeln(`${C.cyan}agents${C.reset}  = [${C.green}"claude-code"${C.reset}, ${C.green}"codex"${C.reset}]`);
      term.writeln("");
      term.writeln(`${C.brand}[mcp_servers.postgres-db]${C.reset}`);
      term.writeln(`${C.cyan}command${C.reset}      = ${C.green}"npx"${C.reset}`);
      term.writeln(`${C.cyan}args${C.reset}         = [${C.green}"-y"${C.reset}, ${C.green}"@mcp/postgres"${C.reset}, ${C.green}"\${DB_URL}"${C.reset}]`);
      term.writeln(`${C.cyan}required_env${C.reset} = [${C.green}"DB_URL"${C.reset}]`);
      term.writeln("");
      term.writeln(`${C.brand}[[skills]]${C.reset}`);
      term.writeln(`${C.cyan}name${C.reset}         = ${C.green}"team-reviewer"${C.reset}`);
      term.writeln(`${C.cyan}source${C.reset}       = ${C.green}"./.gandalf/skills/team-reviewer"${C.reset}`);
    },
  },
  {
    id: "apply",
    label: "gandalf apply",
    command: "gandalf apply --yes",
    render: (term) => {
      term.writeln(`${C.brandBold}$ ${C.reset}gandalf apply --yes`);
      term.writeln(`Team Manifest: zero2one-backend (version: 1.0)`);
      term.writeln(`Target Agents: claude-code, codex`);
      term.writeln("");
      term.writeln(`Review Changes before Apply:`);
      term.writeln(`  [1] [claude-code] add MCP server 'postgres-db' (~/.claude/settings.json)`);
      term.writeln(`  [2] [claude-code] add skill 'team-reviewer' (~/.claude/skills/reviewer.md)`);
      term.writeln(`  [3] [codex] add guardrail hook 'pre-save-lint' (~/.codex/config.toml)`);
      term.writeln("");
      term.writeln(`${C.green}Successfully synchronized team agent environment!${C.reset}`);
      term.writeln(`Pre-apply safety snapshot created: preapply-manifest-20260902-160000`);
    },
  },
];

export default function ManifestoTerminal() {
  const terminalRef = React.useRef<HTMLDivElement>(null);
  const termInstanceRef = React.useRef<Terminal | null>(null);
  const fitAddonRef = React.useRef<FitAddon | null>(null);

  const [activeTab, setActiveTab] = React.useState<string>("check");

  const showTab = React.useCallback((tabId: string) => {
    const term = termInstanceRef.current;
    if (!term) return;

    const tab = VIEWS.find((v) => v.id === tabId) ?? VIEWS[0];
    setActiveTab(tab.id);

    term.clear();
    tab.render(term);
    term.scrollToTop();
  }, []);

  React.useEffect(() => {
    if (!terminalRef.current) return;

    const term = new Terminal({
      cursorBlink: false,
      cursorStyle: "bar",
      fontFamily: "'DM Mono', 'JetBrains Mono', 'Menlo', 'Monaco', monospace",
      fontSize: 12.5,
      lineHeight: 1.5,
      letterSpacing: 0,
      rows: 12,
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

    // Initial render
    showTab("check");

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
  }, [showTab]);

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

        {/* Tab Switcher */}
        <div className="terminal-shell__tabs-wrapper">
          <div className="terminal-tabs-list">
            {VIEWS.map((tab) => {
              const isActive = tab.id === activeTab;
              return (
                <button
                  key={tab.id}
                  type="button"
                  onClick={() => showTab(tab.id)}
                  className={`terminal-tab-trigger ${isActive ? "is-active" : ""}`}
                >
                  {tab.label}
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

      {/* xterm.js Screen Container */}
      <div className="xterm-container-wrapper" style={{ minHeight: "260px" }}>
        <div ref={terminalRef} style={{ width: "100%", height: "240px" }} />
      </div>
    </div>
  );
}
