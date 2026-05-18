/**
 * Shared helpers extracted from cli.ts for use across command modules.
 */
import path from "node:path";
import { defaultStoreDir } from "./store.js";

export interface RuntimeOptions {
  projectPath: string;
  homeDir: string;
  storeDir: string;
}

export function valueAfter(args: string[], flag: string): string | undefined {
  const index = args.indexOf(flag);
  if (index === -1) return undefined;
  return args[index + 1];
}

export function hasFlag(args: string[], flag: string): boolean {
  return args.includes(flag);
}

export function runtimeOptions(args: string[]): RuntimeOptions {
  const homeDir = process.env.HOME ?? process.cwd();
  return {
    projectPath: path.resolve(valueAfter(args, "--project") ?? process.cwd()),
    homeDir,
    storeDir: process.env.SNAPTAILOR_STORE ?? defaultStoreDir(homeDir)
  };
}

export function json(value: unknown): string {
  return `${JSON.stringify(value, null, 2)}\n`;
}