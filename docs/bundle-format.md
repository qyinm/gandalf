# `.hem` Bundle Format Design

Status: **Current format notes** — updated 2026-06-07

## 1. Motivation

Hem stores snapshots locally in `~/.hem/<name>/` as a directory of JSON files.
A `.hem` bundle is a single portable file that packages a snapshot for:

- **Export**: send a snapshot to a teammate or another machine
- **Import**: apply a received snapshot to a local store (optional: with file contents)
- **Audit**: inspect a snapshot's contents without unpacking
- **CI/CD**: store snapshots as artifacts

## 2. Format Choice: TAR (no compression)

Use **POSIX ustar tar** (no gzip/zstd) as the container format.

Rationale:
- Tar is the standard for Unix archives; `node:tar` is in core, no extra dependency in v0.1.
- No compression makes `inspect` fast (no decode needed for metadata).
- Future: add `.hem.gz` or `.hem.zst` as opt-in variants.
- Tar's sequential format prevents cheap file listing without reading the full archive (acceptable for v0.1).

### Name collision note

`.hem` is an invented extension, not associated with any existing format.
Picking `.tar` would cause collisions with generic tarballs; `.hem` is uniquely
identifiable as a Hem bundle.

## 3. Tar Archive Layout

```
bundle.hem
├── .hem/                           # bundle metadata directory
│   ├── format-version                  # "1" (plain text)
│   ├── manifest.json                   # SnapshotManifest (same schema as store)
│   ├── checksums.json                  # ChecksumRecord (same schema, covers all entries)
│   ├── signature                       # Optional: detached signature (hex or base64)
│   └── provenance.json                 # ProvenanceEntry[] (same schema as store)
│
├── snapshot/                           # snapshot data (same files as store)
│   ├── evidence.json                   # DiscoveredItem[]
│   ├── graph.json                      # GraphNode[]
│   ├── audit-findings.json             # AuditFinding[]
│   ├── checksums.json                  # (redundant with .hem/checksums.json)
│   └── redactions.json                 # redaction log
│
└── content/                            # supported raw file contents unless metadata-only
    ├── CLAUDE.md                       # file from snapshot sourcePath
    ├── .mcp.json
    ├── .claude/
    │   └── settings.json
    ├── .codex/
    │   └── config.toml
    └── ~/
        └── .claude/
            ├── settings.json
            ├── skills/
            │   ├── my-skill/
            │   │   └── SKILL.md
            │   └── ...
            └── ...
```

`snapshot/evidence.json` stores the same JSON object shape that scanners emit. `DiscoveredItem` is typed in source as a `kind`-discriminated union, but bundles do not add a second discriminator, wrapper object, or empty `value`/`metadata` fields. Older bundles may also omit optional payload fields, so import and readiness code must continue to treat evidence JSON as a defensive boundary.

### Entry naming rules

- All paths inside the tar are **relative**, never absolute, never containing `..`.
- The `.hem/` directory is always present and contains only bundle-level metadata.
- The `snapshot/` directory is always present and mirrors the store snapshot file set.
- The `content/` directory is present when supported content is included. Current export includes content by default; use `--metadata-only` to opt out.
- Content paths use the **filesystem source path** from `DiscoveredItem.sourcePath`, with `~` expanded to home-relative. Examples:
  - `sourcePath: "~/.claude/settings.json"` → `content/~/.claude/settings.json`
  - `sourcePath: ".mcp.json"` → `content/.mcp.json`
  - `sourcePath: "~/.claude/skills/my-skill/SKILL.md"` → `content/~/.claude/skills/my-skill/SKILL.md`

### Files not included

- `~/.env` files are **never** included in `content/` (raw env values omitted by policy).
- Secret-like keys' values are omitted/redacted per existing `captureStatusForKey` rules.
- Symlinks are recorded in evidence but their targets are not included (not followed by policy).

## 4. Tar Format Details

### Block size

Standard tar blocks of 512 bytes. No padding extension.

### Entry order

All entries are added in a single pass in **alphabetical order** (`.hem/` first,
then `snapshot/`, then optional `content/`). This ensures deterministic bundle
generation for checksumming.

### Content encoding

All content entries (including JSON) use **UTF-8 without BOM**. Binary files
are stored as-is with no encoding.

## 5. Bundle Export Flow

```
hem bundle export --name baseline --out baseline.hem --project .
```

Steps:
1. Read snapshot `baseline` from store.
2. Validate no secret/redacted content would be leaked (fail if `unsafe_to_export` items exist).
3. Create tar writer.
4. Write `.hem/format-version` — plain text, `"1\n"`.
5. Write `.hem/manifest.json` — snapshot manifest.
6. Write `.hem/checksums.json` — checksums for all tar entries (SHA-256).
7. Write `snapshot/evidence.json`, `graph.json`, `audit-findings.json`, `checksums.json`, `redactions.json`.
8. Unless `--metadata-only`: write `content/` entries for captured evidence items.
9. Finalize tar.

### Flags

| Flag | Description |
|---|---|
| `--name` | Snapshot name in local store (required) |
| `--out` | Output `.hem` path (required) |
| `--metadata-only` | Export snapshot metadata without supported file contents |
| `--project` | Project path for resolving source paths |
| `--json` | Output JSON summary of the export |

### Export validation (pre-flight)

Before writing the tar, export validates:

1. Snapshot exists and is readable.
2. No evidence item has `captureStatus: "unsafe_to_export"`.
3. No content path escapes the content root (`..` segments, absolute paths after resolution).
4. Total content size does not exceed a configurable limit (default: 50MB for `--include-content`).

## 6. Bundle Import Flow

```
hem bundle import baseline.hem --project .
```

Steps:
1. Read bundle file.
2. **Quarantine phase**: extract to a temp directory (`/tmp/.hem-quarantine-<uuid>/`).
3. **Validation phase**:
   - Verify format version is supported.
   - Verify all tar entry paths are safe (no `..`, no absolute paths, no path traversal).
   - Verify `manifest.json` matches `snapshot/` contents.
   - If `--verify-signature`: verify the signature in `.hem/signature`.
   - Check content size caps.
4. **Readiness phase**: build the Mac readiness report for missing local tools, MCP commands, unverified remote MCP URLs, env key gaps, and apply blockers.
5. **Apply phase**: copy validated snapshot to `~/.hem/<name>/`.
6. If `--apply-content`:
   - Requires `--experimental` or `HEM_EXPERIMENTAL=1`.
   - Fails before writes when Mac-only apply blockers are present.
   - Copies content files to their resolved source paths, or stages them under quarantine when `--quarantine` is passed.
7. Clean up temporary state.

### Flags

| Flag | Description |
|---|---|
| `--out` | Output directory (default: `~/.hem/`) |
| `--apply-content` | Apply raw file contents from bundle to their resolved paths; requires experimental opt-in |
| `--quarantine` | Stage content for inspection without writing target files |
| `--trust` | Trust a signed bundle source after manual verification |
| `--dry-run` | Validate bundle without writing anything |
| `--json` | Output JSON summary of what would be imported |

### Import security checks

1. **Path traversal**: each tar entry path is resolved and verified to be within the extraction root.
   - Reject entries with `..`, null bytes, or absolute paths outside the quarantine root.
2. **Bundle size**: reject bundles over 500MB (configurable via env var `HEM_MAX_BUNDLE_BYTES`).
3. **Malformed entries**: reject entries with unexpected formats, truncation, or parsing errors.
4. **Quarantine directory**: created with `0700` permissions, cleaned up on success or failure.

### Content apply and readiness

When `--dry-run` or `--apply-content` is used, import returns a structured readiness report. The report uses stable categories: `ready`, `needs_manual_action`, `warning`, `unverified`, `unsupported`, and `blocked`.

Hem reports manual actions for missing tools and env key values, but it does not install packages, contact registries, execute MCP commands, or write placeholder secret values.

## 7. Bundle Inspection

```
hem bundle inspect baseline.hem
```

Reads only `.hem/manifest.json` and `.hem/checksums.json` (first entries in tar)
to show bundle metadata without unpacking the full archive.

```text
Bundle: baseline.hem
  Format: 1
  Snapshot: baseline
  Created: 2026-05-15T10:00:00Z
  Project: /path/to/project
  Includes content: yes (12 files, 48 KB)
  Checksum: SHA-256 a1b2c3d4...
  Signature: none
```

`--json` flag outputs the same info as JSON.

## 8. Bundle Signing (Future)

### Key types

- Ed25519 (preferred) or ECDSA P-256.
- Key stored in `~/.hem/keys/` directory or provided via env var.

### Signing

```bash
hem bundle export --name baseline --out baseline.hem --sign
```

1. Export produces a checksum of the tar (SHA-256 of the entire archive).
2. Sign the checksum with the user's private key.
3. Store signature in `.hem/signature`.

### Verification

```bash
hem bundle import baseline.hem --verify-signature
```

1. Recompute tar checksum.
2. Verify against `.hem/signature` using the public key.
3. If verification fails, reject the import unless `--trust` is set.

### Key management (future)

```bash
hem key generate --type ed25519
hem key list
hem key export --name default --out pubkey.pem
hem key import --name team-key pubkey.pem
```

## 9. Bundle CLI Integration

```text
hem bundle export --name baseline --out baseline.hem --project .
hem bundle import baseline.hem --project .
hem bundle inspect baseline.hem
hem bundle import baseline.hem --dry-run --project .
hem bundle import baseline.hem --apply-content --project .

Future:
hem bundle export --name baseline --out baseline.hem --sign
hem bundle import baseline.hem --verify-signature
hem key generate --type ed25519
hem key list
```

## 10. Data Model (Types)

```typescript
export interface BundleManifest {
  formatVersion: 1;
  snapshotName: string;
  createdAt: string;
  projectPath: string;
  includesContent: boolean;
  contentFileCount: number;
  contentTotalBytes: number;
  security: {
    rawSecretsIncluded: false;
    redactionPolicy: "metadata-only" | "structured_safe_fields_only";
    signed: boolean;
    signatureAlgorithm?: string;
  };
}

export interface BundleChecksums {
  algorithm: "SHA-256";
  entries: Record<string, string>;  // tar entry path → hex digest
}
```

## 11. Implementation Notes

### Dependencies

No new package dependencies. Use Node.js built-in `crypto` for hashing and the
tar/untar logic can be implemented using raw `Buffer` operations on tar blocks
or the `node:tar`-style stream parsing. Since the format is simple and block-based,
a minimal tar reader/writer is ~200 lines.

### Migration path from snapshot store

Existing `~/.hem/<name>/` directories can be converted to bundles:

```bash
hem bundle export --name baseline --out baseline.hem
```

No migration needed; bundles and store snapshots coexist. The store remains the
authoritative local snapshot location; bundles are for transport only.

### Rollback for import

Bundle import is atomic per snapshot directory. If import fails mid-write:

1. Quarantine temp is cleaned up.
2. If the store snapshot was partially written, it's left in an inconsistent state.
   - Mitigation: write to temp dir, then rename atomically.
   - The `writeSnapshot` function in `store.ts` already uses atomic writes.

## 12. Open Questions

| Question | Proposed answer |
|---|---|
| Should bundles default to including `content/`? | No. `--include-content` must be explicit until v0.2+ maturity. |
| Should import overwrite existing snapshots? | No, unless `--overwrite` is set. Default: fail if name exists. |
| Should `.hem` be gzip-compressed? | Not in v0.1. Add `.hem.gz` variant later. |
| How to handle cross-OS path differences? | Store all paths as POSIX inside tar. On import, resolve OS-specific home dir. |
| What about `.hem` files from unknown/older schema versions? | Reject import. Display supported version range. |
