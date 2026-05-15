/**
 * Minimal POSIX ustar tar writer/reader for .stailor bundles.
 *
 * No external dependencies. Uses only Node.js core Buffer and crypto.
 *
 * Tar header format (512 bytes per POSIX ustar):
 *   offset  size  field
 *   0       100   name
 *   100     8     mode (octal)
 *   108     8     uid (octal)
 *   116     8     gid (octal)
 *   124     12    size (octal)
 *   136     12    mtime (octal)
 *   148     8     chksum
 *   156     1     typeflag ('0'=file, '5'=dir)
 *   157     100   linkname
 *   257     6     magic ("ustar")
 *   263     32    uname
 *   295     32    gname
 *   297     8     devmajor
 *   305     8     devminor
 *   313     155   prefix
 *   468     45    padding
 */

import { createHash } from "node:crypto";
import { readFile, writeFile, stat, mkdir } from "node:fs/promises";
import path from "node:path";
import type { TarEntry } from "./types.js";

const BLOCK_SIZE = 512;
const HEADER_SIZE = 512;
const MAGIC = "ustar";

interface TarHeader {
  name: string;
  mode: number;
  uid: number;
  gid: number;
  size: number;
  mtime: number;
  checksum: string;
  typeflag: string;
  linkname: string;
  magic: string;
  version: string;
  uname: string;
  gname: string;
  devmajor: number;
  devminor: number;
  prefix: string;
}

/**
 * Write a tar archive to a file.
 * Entries are written in order. End-of-archive is two zero-filled blocks.
 * Returns a checksum hex digest over the full archive content.
 */
export async function writeTar(
  entries: TarEntry[],
  outputPath: string
): Promise<string> {
  const blocks: Buffer[] = [];

  for (const entry of entries) {
    const header = encodeHeader(entry);
    blocks.push(header);

    if (entry.type === "file" && entry.content.length > 0) {
      const data = entry.content;
      blocks.push(data);
      // Pad to 512-byte boundary
      const padding = BLOCK_SIZE - (data.length % BLOCK_SIZE);
      if (padding < BLOCK_SIZE) {
        blocks.push(Buffer.alloc(padding));
      }
    }
  }

  // End-of-archive: two zero blocks
  blocks.push(Buffer.alloc(BLOCK_SIZE));
  blocks.push(Buffer.alloc(BLOCK_SIZE));

  const archive = Buffer.concat(blocks);
  await mkdir(path.dirname(outputPath), { recursive: true });
  await writeFile(outputPath, archive);

  return createHash("sha256").update(archive).digest("hex");
}

/**
 * Read a tar archive from a file and return all entries.
 * Returns both the entries list and the SHA-256 checksum of the raw file.
 */
export async function readTar(
  inputPath: string
): Promise<{ entries: TarEntry[]; checksum: string }> {
  const archive = await readFile(inputPath);
  const checksum = createHash("sha256").update(archive).digest("hex");
  const entries: TarEntry[] = [];
  let offset = 0;

  while (offset + HEADER_SIZE <= archive.length) {
    const headerBlock = archive.subarray(offset, offset + HEADER_SIZE);

    // Check for end-of-archive (all zeros)
    if (isZeroBlock(headerBlock)) {
      offset += BLOCK_SIZE;
      // Check for second zero block
      if (offset + BLOCK_SIZE <= archive.length && isZeroBlock(archive.subarray(offset, offset + BLOCK_SIZE))) {
        break;
      }
      continue;
    }

    const header = decodeHeader(headerBlock);
    offset += HEADER_SIZE;
    const size = header.typeflag === "5" ? 0 : header.size;

    // Validate size
    if (size > 500 * 1024 * 1024) {
      throw new Error(`Tar entry too large: ${header.name} (${size} bytes)`);
    }

    let content = Buffer.alloc(0);
    if (size > 0) {
      if (offset + size > archive.length) {
        throw new Error(`Truncated tar entry: ${header.name}`);
      }
      content = archive.subarray(offset, offset + size);
    }

    const entry: TarEntry = {
      path: header.name,
      content,
      mode: header.mode,
      mtime: header.mtime,
      type: header.typeflag === "5" ? "directory" : "file"
    };
    entries.push(entry);

    // Advance past data + padding
    const dataBlocks = Math.ceil(size / BLOCK_SIZE);
    offset += dataBlocks * BLOCK_SIZE;
  }

  return { entries, checksum };
}

/**
 * Encode a TarEntry into a 512-byte ustar header block.
 */
function encodeHeader(entry: TarEntry): Buffer {
  const buf = Buffer.alloc(HEADER_SIZE, 0);

  const name = entry.path;
  const prefix = "";
  const nameField = prefix ? "" : name;
  const prefixField = prefix || "";

  writeField(buf, 0, 100, nameField);
  writeField(buf, 100, 8, entry.mode.toString(8).padStart(7, "0"));
  writeField(buf, 108, 8, "0000000"); // uid
  writeField(buf, 116, 8, "0000000"); // gid
  writeField(buf, 124, 12, entry.type === "file" ? entry.content.length.toString(8).padStart(11, "0") : "00000000000");
  writeField(buf, 136, 12, Math.floor(entry.mtime / 1000).toString(8).padStart(11, "0"));
  // checksum field: spaces, will be computed
  writeField(buf, 148, 8, " ".repeat(8));
  writeField(buf, 156, 1, entry.type === "directory" ? "5" : "0");
  writeField(buf, 157, 100, ""); // linkname
  writeField(buf, 257, 6, MAGIC);
  writeField(buf, 263, 2, "00"); // version
  writeField(buf, 265, 32, ""); // uname
  writeField(buf, 297, 32, ""); // gname
  writeField(buf, 329, 8, "0000000"); // devmajor
  writeField(buf, 337, 8, "0000000"); // devminor
  writeField(buf, 345, 155, prefixField); // prefix

  // Compute checksum
  let checksum = 0;
  for (let i = 0; i < HEADER_SIZE; i++) {
    checksum += buf[i];
  }
  const checksumStr = checksum.toString(8).padStart(7, "0");
  writeField(buf, 148, 7, checksumStr);
  buf[155] = 0x20; // trailing space after checksum

  return buf;
}

/**
 * Decode a 512-byte ustar header block.
 */
function decodeHeader(buf: Buffer): TarHeader {
  const name = readField(buf, 0, 100);
  const mode = parseInt(readField(buf, 100, 8), 8);
  const uid = parseInt(readField(buf, 108, 8), 8);
  const gid = parseInt(readField(buf, 116, 8), 8);
  const size = parseInt(readField(buf, 124, 12), 8);
  const mtime = parseInt(readField(buf, 136, 12), 8);
  const checksum = readField(buf, 148, 8);
  const typeflag = readField(buf, 156, 1);
  const linkname = readField(buf, 157, 100);
  const magic = readField(buf, 257, 6);
  const version = readField(buf, 263, 2);
  const uname = readField(buf, 265, 32);
  const gname = readField(buf, 297, 32);
  const devmajor = parseInt(readField(buf, 329, 8), 8);
  const devminor = parseInt(readField(buf, 337, 8), 8);
  const prefix = readField(buf, 345, 155);

  // Validate checksum
  const savedChecksum = parseInt(checksum.trim(), 8);
  let computed = 0;
  for (let i = 0; i < HEADER_SIZE; i++) {
    computed += buf[i];
  }
  // The checksum field is stored as spaces during computation, then overwritten.
  // We need to recompute with spaces in the checksum field.
  const recomputedBuf = Buffer.from(buf);
  writeField(recomputedBuf, 148, 8, " ".repeat(8));
  let actual = 0;
  for (let i = 0; i < HEADER_SIZE; i++) {
    actual += recomputedBuf[i];
  }
  if (savedChecksum !== 0 && savedChecksum !== actual) {
    // Non-fatal: some tar implementations compute differently.
    // Accept if either computed value matches.
    if (savedChecksum !== computed && savedChecksum !== actual) {
      throw new Error(`Checksum mismatch for tar entry "${name}": expected ${actual.toString(8)}, got ${savedChecksum.toString(8)}`);
    }
  }

  return { name, mode, uid, gid, size, mtime, checksum, typeflag, linkname, magic, version, uname, gname, devmajor, devminor, prefix };
}

function writeField(buf: Buffer, offset: number, length: number, value: string): void {
  const str = value.substring(0, length);
  buf.write(str, offset, str.length, "ascii");
}

function readField(buf: Buffer, offset: number, length: number): string {
  const end = offset + length;
  // Find the null terminator
  let nullPos = offset;
  while (nullPos < end && buf[nullPos] !== 0) {
    nullPos++;
  }
  return buf.toString("ascii", offset, nullPos);
}

function isZeroBlock(buf: Buffer): boolean {
  for (let i = 0; i < buf.length; i++) {
    if (buf[i] !== 0) return false;
  }
  return true;
}

/**
 * Validate that a tar entry path is safe (no path traversal).
 * Returns the normalized path if safe, or throws.
 */
export function validateTarPath(entryPath: string, root: string): string {
  if (entryPath.includes("..")) {
    throw new Error(`Path traversal detected: "${entryPath}" contains ".."`);
  }
  if (entryPath.includes("\0")) {
    throw new Error(`Path traversal detected: "${entryPath}" contains null byte`);
  }
  if (path.isAbsolute(entryPath)) {
    throw new Error(`Path traversal detected: "${entryPath}" is absolute`);
  }

  const resolved = path.resolve(root, entryPath);
  const normalizedRoot = path.resolve(root);

  if (!resolved.startsWith(normalizedRoot)) {
    throw new Error(`Path traversal detected: "${entryPath}" resolves outside root`);
  }

  return entryPath;
}
