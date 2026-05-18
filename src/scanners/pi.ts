import type { ScannerPlugin } from "./scanner-plugin.js";
import type { ScanTarget } from "./index.js";
import { projectTarget, homeTarget } from "./index.js";

export const piAgentScanner: ScannerPlugin = {
  agentId: "pi-agent",
  agentName: "Pi Agent",
  description: "Pi coding agent configuration (settings, models, agents, extensions, skills, themes, prompts)",

  targets(projectPath: string, homeDir: string): ScanTarget[] {
    const targets: ScanTarget[] = [];

    // ── Project-level ──
    targets.push(
      projectTarget(projectPath, ".pi/settings.json", "pi-agent", "agent_config", "json"),
      projectTarget(projectPath, ".pi/extensions", "pi-agent", "unsupported", "filesystem", {
        directory: true, sensitivity: "extensions"
      }),
      projectTarget(projectPath, ".pi/skills", "pi-agent", "skill", "filesystem", {
        directory: true
      }),
      projectTarget(projectPath, ".pi/themes", "pi-agent", "unsupported", "filesystem", {
        directory: true, sensitivity: "themes"
      }),
      projectTarget(projectPath, ".pi/prompts", "pi-agent", "agent_instruction", "filesystem", {
        directory: true, sensitivity: "prompt_templates"
      }),
    );

    // ── User-level: runtime config ──
    targets.push(
      homeTarget(homeDir, ".pi/agent/settings.json", "pi-agent", "agent_config", "json"),
      homeTarget(homeDir, ".pi/agent/models.json", "pi-agent", "agent_config", "json", {
        metadataOnly: true, sensitivity: "model_registry"
      }),
    );

    // ── User-level: custom agents ──
    targets.push(
      homeTarget(homeDir, ".pi/agents", "pi-agent", "skill", "filesystem", {
        directory: true, sensitivity: "custom_agents"
      }),
    );

    // ── User-level: extensions (TypeScript modules) ──
    targets.push(
      homeTarget(homeDir, ".pi/agent/extensions", "pi-agent", "unsupported", "filesystem", {
        directory: true, sensitivity: "extensions"
      }),
    );

    // ── User-level: skills ──
    targets.push(
      homeTarget(homeDir, ".pi/agent/skills", "pi-agent", "skill", "filesystem", {
        directory: true
      }),
    );

    // ── User-level: themes ──
    targets.push(
      homeTarget(homeDir, ".pi/agent/themes", "pi-agent", "unsupported", "filesystem", {
        directory: true, sensitivity: "themes"
      }),
    );

    // ── User-level: prompt templates ──
    targets.push(
      homeTarget(homeDir, ".pi/agent/prompts", "pi-agent", "skill", "filesystem", {
        directory: true, sensitivity: "prompt_templates"
      }),
    );

    return targets;
  },
};
