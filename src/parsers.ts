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
  const lines = text.split(/\r?\n/);

  for (let index = 0; index < lines.length; index++) {
    let line = lines[index].trim();
    if (!line || line.startsWith("#")) {
      continue;
    }
    // Skip section headers (e.g. [features], [mcp_servers.pencil], [plugins."foo@bar"])
    if (line.startsWith("[") && line.endsWith("]")) {
      continue;
    }

    const match = /^([A-Za-z0-9_.-]+)\s*=\s*(.*)$/.exec(line);
    if (!match) {
      // Lines that don't match key=value are silently skipped.
      // This handles multi-line array/table values (e.g. args = [\n  "--app",\n  "desktop",\n])
      // and any other TOML constructs the simple parser doesn't understand.
      continue;
    }

    const [, key, rawValue] = match;
    let processedValue = rawValue.trim();

    // Accumulate continuation lines for multi-line inline arrays
    // e.g. args = [\n  "--app",\n  "desktop",\n]
    if (processedValue.startsWith("[") && !processedValue.endsWith("]")) {
      const arrayLines = [processedValue];
      while (++index < lines.length) {
        const continuationLine = lines[index].trim();
        arrayLines.push(continuationLine);
        if (continuationLine.endsWith("]") || continuationLine.endsWith("],")) {
          break;
        }
      }
      processedValue = arrayLines.join(" ");
    }

    value[key] = isSecretLikeKey(key) ? "[redacted]" : parseTomlScalar(processedValue);
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
