import { captureStatusForKey, isSecretLikeKey, redactStructuredValue } from "./policy.js";

export interface ParseSuccess {
  ok: true;
  value: unknown;
}

export interface ParseFailure {
  ok: false;
  error: string;
}

export type ParseResult = ParseSuccess | ParseFailure;

export interface DotenvEntry {
  key: string;
  secretLike: boolean;
  captureStatus: "redacted" | "omitted";
}

export function parseJson(text: string): ParseResult {
  try {
    return { ok: true, value: redactStructuredValue(JSON.parse(text)) };
  } catch (error) {
    return { ok: false, error: error instanceof Error ? error.message : "Invalid JSON" };
  }
}

export function parseTomlKeyValues(text: string): ParseResult {
  const value: Record<string, unknown> = {};

  for (const [index, rawLine] of text.split(/\r?\n/).entries()) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }
    if (line.startsWith("[") && line.endsWith("]")) {
      continue;
    }

    const match = /^([A-Za-z0-9_.-]+)\s*=\s*(.*)$/.exec(line);
    if (!match) {
      return { ok: false, error: `Invalid TOML key/value at line ${index + 1}` };
    }

    const [, key, rawValue] = match;
    value[key] = isSecretLikeKey(key) ? "[redacted]" : parseTomlScalar(rawValue);
  }

  return { ok: true, value };
}

export function parseMarkdown(text: string): ParseResult {
  const frontmatter = /^---\r?\n([\s\S]*?)\r?\n---/.exec(text);
  if (!frontmatter) {
    return { ok: true, value: { hasFrontmatter: false } };
  }

  const metadata: Record<string, unknown> = {};
  for (const rawLine of frontmatter[1].split(/\r?\n/)) {
    const match = /^([A-Za-z0-9_.-]+):\s*(.*)$/.exec(rawLine.trim());
    if (match) {
      const [, key, rawValue] = match;
      metadata[key] = isSecretLikeKey(key) ? "[redacted]" : rawValue;
    }
  }

  return { ok: true, value: { hasFrontmatter: true, metadata } };
}

export function parseDotenvKeys(text: string): DotenvEntry[] {
  const entries: DotenvEntry[] = [];

  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }

    const match = /^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=/.exec(line);
    if (!match) {
      continue;
    }

    const key = match[1];
    entries.push({
      key,
      secretLike: isSecretLikeKey(key),
      captureStatus: captureStatusForKey(key)
    });
  }

  return entries;
}

function parseTomlScalar(rawValue: string): unknown {
  const value = rawValue.trim();
  if ((value.startsWith("\"") && value.endsWith("\"")) || (value.startsWith("'") && value.endsWith("'"))) {
    return value.slice(1, -1);
  }
  if (value === "true") {
    return true;
  }
  if (value === "false") {
    return false;
  }
  if (/^-?\d+(?:\.\d+)?$/.test(value)) {
    return Number(value);
  }
  return value;
}
