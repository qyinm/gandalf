# `.stailor` Bundle Format Design

Status: **Design Draft** — 2026-05-15

## 1. Motivation

snaptailor stores snapshots locally in `~/.snaptailor/<name>/` as a directory of JSON files.
A `.stailor` bundle is a single portable file that packages a snapshot for:

- **Export**: send a snapshot to a teammate or another machine
- **Import**: apply a received snapshot to a local store (optional: with file contents)
- **Audit**: inspect a snapshot's contents without unpacking
- **CI/CD**: store snapshots as artifacts

## 2. Format Choice: TAR (no compression)

Use **POSIX ustar tar** (no gzip/zstd) as the container format.

Rationale:
- Tar is the standard for Unix archives; `node:tar` is in core, no extra dependency in v0.1.
- No compression makes `inspect` fast (no decode needed for metadata).
- Future: add `.stailor.gz` or `.stailor.zst` as opt-in variants.
- Tar's sequential format prevents cheap file listing without reading the full archive (acceptable for v0.1).

### Name collision note

`.stailor` is an invented extension, not associated with any existing format.
Picking `.tar` would cause collisions with generic tarballs; `.stailor` is uniquely
identifiable as a snaptailor bundle.

## 3. Tar Archive Layout

```
bundle.stailor
├── .stailor/                           # bundle metadata directory
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
│   ├── checksums.json                  # (redundant with .stailor/checksums.json)
│   └── redactions.json                 # redaction log
│
└── content/                            # optional raw file contents (user opt-in)
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

### Entry naming rules

- All paths inside the tar are **relative**, never absolute, never containing `..`.
- The `.stailor/` directory is always present and contains only bundle-level metadata.
- The `snapshot/` directory is always present and mirrors the store snapshot file set.
- The `content/` directory is present **only** when the export was created with `--include-content`.
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

All entries are added in a single pass in **alphabetical order** (`.stailor/` first,
then `snapshot/`, then optional `content/`). This ensures deterministic bundle
generation for checksumming.

### Content encoding

All content entries (including JSON) use **UTF-8 without BOM**. Binary files
are stored as-is with no encoding.

## 5. Bundle Export Flow

```
snaptailor bundle export --name baseline --out baseline.stailor --project .
```

Steps:
1. Read snapshot `baseline` from store.
2. Validate no secret/redacted content would be leaked (fail if `unsafe_to_export` items exist).
3. Create tar writer.
4. Write `.stailor/format-version` — plain text, `"1\n"`.
5. Write `.stailor/manifest.json` — snapshot manifest.
6. Write `.stailor/checksums.json` — checksums for all tar entries (SHA-256).
7. Write `snapshot/evidence.json`, `graph.json`, `audit-findings.json`, `checksums.json`, `redactions.json`.
8. If `--include-content`: write `content/` entries for captured evidence items.
9. Finalize tar.

### Flags

| Flag | Description |
|---|---|
| `--name` | Snapshot name in local store (required) |
| `--out` | Output `.stailor` path (required) |
| `--include-content` | Include raw file contents in `content/` (v0.2+, opt-in) |
| `--project` | Project path for resolving source paths |
| `--sign` | Sign the bundle with a provided key (future) |
| `--json` | Output JSON summary of the export |

### Export validation (pre-flight)

Before writing the tar, export validates:

1. Snapshot exists and is readable.
2. No evidence item has `captureStatus: "unsafe_to_export"`.
3. No content path escapes the content root (`..` segments, absolute paths after resolution).
4. Total content size does not exceed a configurable limit (default: 50MB for `--include-content`).

## 6. Bundle Import Flow

```
snaptailor bundle import baseline.stailor --project .
```

Steps:
1. Read bundle file.
2. **Quarantine phase**: extract to a temp directory (`/tmp/.stailor-quarantine-<uuid>/`).
3. **Validation phase**:
   - Verify format version is supported.
   - Verify all tar entry paths are safe (no `..`, no absolute paths, no path traversal).
   - Verify `manifest.json` matches `snapshot/` contents.
   - If `--verify-signature`: verify the signature in `.stailor/signature`.
   - Check content size caps.
4. **Apply phase**: copy validated snapshot to `~/.snaptailor/<name>/`.
5. If `--apply-content`:
   - Copy content files to their original source paths (prompting for confirmation per path).
   - Scrub content directory after successful apply.
6. Clean up quarantine temp directory.

### Flags

| Flag | Description |
|---|---|
| `--out` | Output directory (default: `~/.snaptailor/`) |
| `--apply-content` | Apply raw file contents from bundle to their original paths (v0.2+, requires confirmation) |
| `--verify-signature` | Verify detached signature before importing (future) |
| `--trust` | Bypass confirmation prompts (HEADLESS mode) |
| `--dry-run` | Validate bundle without writing anything |
| `--json` | Output JSON summary of what would be imported |

### Import security checks

1. **Path traversal**: each tar entry path is resolved and verified to be within the extraction root.
   - Reject entries with `..`, null bytes, or absolute paths outside the quarantine root.
2. **Bundle size**: reject bundles over 500MB (configurable via env var `SNAPTAILOR_MAX_BUNDLE_BYTES`).
3. **Malformed entries**: reject entries with unexpected formats, truncation, or parsing errors.
4. **Quarantine directory**: created with `0700` permissions, cleaned up on success or failure.

### Content apply confirmation

When `--apply-content` is used, each content file is applied to its original source path:

```text
Content file to apply: ~/.claude/settings.json
  From bundle: baseline.stailor (created 2026-05-15T10:00:00Z)
  Current size: 2048 bytes
  Bundle size: 2120 bytes
  Apply? [y/N]
```

For headless environments, `--trust` bypasses prompts (but still logs applied paths).

## 7. Bundle Inspection

```
snaptailor bundle inspect baseline.stailor
```

Reads only `.stailor/manifest.json` and `.stailor/checksums.json` (first entries in tar)
to show bundle metadata without unpacking the full archive.

```text
Bundle: baseline.stailor
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
- Key stored in `~/.snaptailor/keys/` directory or provided via env var.

### Signing

```bash
snaptailor bundle export --name baseline --out baseline.stailor --sign
```

1. Export produces a checksum of the tar (SHA-256 of the entire archive).
2. Sign the checksum with the user's private key.
3. Store signature in `.stailor/signature`.

### Verification

```bash
snaptailor bundle import baseline.stailor --verify-signature
```

1. Recompute tar checksum.
2. Verify against `.stailor/signature` using the public key.
3. If verification fails, reject the import unless `--trust` is set.

### Key management (future)

```bash
snaptailor key generate --type ed25519
snaptailor key list
snaptailor key export --name default --out pubkey.pem
snaptailor key import --name team-key pubkey.pem
```

## 9. Bundle CLI Integration

```text
snaptailor bundle export --name baseline --out baseline.stailor --project .
snaptailor bundle import baseline.stailor --project .
snaptailor bundle inspect baseline.stailor
snaptailor bundle import baseline.stailor --dry-run --project .
snaptailor bundle import baseline.stailor --apply-content --project .

Future:
snaptailor bundle export --name baseline --out baseline.stailor --sign
snaptailor bundle import baseline.stailor --verify-signature
snaptailor key generate --type ed25519
snaptailor key list
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

No new npm dependencies. Use Node.js built-in `crypto` for hashing and the
tar/untar logic can be implemented using raw `Buffer` operations on tar blocks
or the `node:tar`-style stream parsing. Since the format is simple and block-based,
a minimal tar reader/writer is ~200 lines.

### Migration path from snapshot store

Existing `~/.snaptailor/<name>/` directories can be converted to bundles:

```bash
snaptailor bundle export --name baseline --out baseline.stailor
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
| Should `.stailor` be gzip-compressed? | Not in v0.1. Add `.stailor.gz` variant later. |
| How to handle cross-OS path differences? | Store all paths as POSIX inside tar. On import, resolve OS-specific home dir. |
| What about `.stailor` files from unknown/older schema versions? | Reject import. Display supported version range. |
