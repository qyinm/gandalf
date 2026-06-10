import type { EvidenceKind, RestorePolicy } from "./types.js";

export const MAX_FILE_BYTES = 256 * 1024;
export const MAX_DIRECTORY_DEPTH = 4;
export const MAX_DIRECTORY_ENTRIES = 250;

const SECRET_KEY_PATTERN = /(api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)/i;

/** Map evidence kind to its restore policy. */
export function restorePolicyFor(kind: EvidenceKind): RestorePolicy {
  switch (kind) {
    case "agent_instruction":
    case "agent_config":
    case "skill":
    case "extension":
      return "full_content_supported";
    case "mcp_server":
    case "permission":
    case "hook":
      return "structured_fields_only";
    case "env_key":
      return "key_inventory_only";
    case "symlink":
    case "unsupported":
      return "not_supported";
  }
}

export function isSecretLikeKey(key: string): boolean {
  return SECRET_KEY_PATTERN.test(key);
}

export function captureStatusForKey(key: string): "redacted" | "omitted" {
  return isSecretLikeKey(key) ? "redacted" : "omitted";
}

export function redactStructuredValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((entry) => redactStructuredValue(entry));
  }

  if (!value || typeof value !== "object") {
    return value;
  }

  const redacted: Record<string, unknown> = {};
  for (const [key, nestedValue] of Object.entries(value)) {
    if (isSecretLikeKey(key)) {
      redacted[key] = "[redacted]";
    } else if (key === "env" && nestedValue && typeof nestedValue === "object" && !Array.isArray(nestedValue)) {
      redacted.envKeys = Object.keys(nestedValue);
    } else {
      redacted[key] = redactStructuredValue(nestedValue);
    }
  }

  return redacted;
}

export function ignoredDirectory(name: string): boolean {
  return new Set([
    ".git",
    "node_modules",
    "dist",
    "build",
    ".cache",
    "cache",
    "caches",
    "logs",
    "log",
    ".next",
    "coverage",
    ".turbo"
  ]).has(name);
}
