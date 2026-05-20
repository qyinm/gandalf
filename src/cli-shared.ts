/**
 * Shared helpers extracted from cli.ts for use across command modules.
 */
import path from "node:path";
import type { AgentId } from "./types.js";
import { defaultStoreDir } from "./store.js";

export interface RuntimeOptions {
  projectPath: string;
  homeDir: string;
  storeDir: string;
  agent?: AgentId;
}

export function valueAfter(args: string[], flag: string): string | undefined {
  const index = args.indexOf(flag);
  if (index === -1) return undefined;
  return args[index + 1];
}

export function hasFlag(args: string[], flag: string): boolean {
  return args.includes(flag);
}

export const VALID_AGENTS: AgentId[] = [
  "claude-code",
  "codex",
  "cursor",
  "opencode",
  "pi-agent",
  "project",
  "unknown"
];

export function runtimeOptions(args: string[]): RuntimeOptions {
  const homeDir = process.env.HOME ?? process.cwd();
  const agent = valueAfter(args, "--agent") as AgentId | undefined;
  if (agent && !VALID_AGENTS.includes(agent)) {
    throw new Error(
      `Invalid agent: "${agent}". Valid agents: ${VALID_AGENTS.join(", ")}`
    );
  }
  return {
    projectPath: path.resolve(valueAfter(args, "--project") ?? process.cwd()),
    homeDir,
    storeDir: process.env.SNAPTAILOR_STORE ?? defaultStoreDir(homeDir),
    agent
  };
}

export function json(value: unknown): string {
  return `${JSON.stringify(value, null, 2)}\n`;
}