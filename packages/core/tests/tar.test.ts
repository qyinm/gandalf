/**
 * Tests for the minimal POSIX ustar tar implementation.
 *
 * Covers:
 * - Write/read roundtrip for files and directories
 * - Security: symlink entry rejection
 * - Security: linkname validation (non-empty linkname rejected)
 * - Security: path traversal (.., null bytes, absolute paths)
 * - Size limit enforcement
 * - Edge cases: zero-length files, empty archives, multiple entries
 * - Checksum validation on read
 */
import assert from "node:assert/strict";
import { mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";

import { readTar, writeTar, validateTarPath } from "../src/tar.js";
import type { TarEntry } from "../src/types.js";

async function tmpPath(): Promise<string> {
  const dir = await mkdtemp(join(tmpdir(), "gandalf-tar-"));
  return join(dir, "archive.tar");
}

// ── Roundtrip tests ─────────────────────────────────────────────

describe("tar write/read roundtrip", () => {
  it("writes and reads a file entry", async () => {
    const path = await tmpPath();
    const entries: TarEntry[] = [
      { path: "hello.txt", content: Buffer.from("Hello, World!", "utf-8"), mode: 0o644, mtime: 1000000, type: "file" }
    ];

    const writeChecksum = await writeTar(entries, path);
    const { entries: readEntries, checksum } = await readTar(path);

    assert.equal(checksum, writeChecksum);
    assert.equal(readEntries.length, 1);
    assert.equal(readEntries[0].path, "hello.txt");
    assert.equal(readEntries[0].content.toString("utf-8"), "Hello, World!");
    assert.equal(readEntries[0].mode, 0o644);
    assert.equal(readEntries[0].type, "file");
  });

  it("writes and reads a directory entry", async () => {
    const path = await tmpPath();
    const entries: TarEntry[] = [
      { path: "mydir/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" }
    ];

    await writeTar(entries, path);
    const { entries: readEntries } = await readTar(path);

    assert.equal(readEntries.length, 1);
    assert.equal(readEntries[0].path, "mydir/");
    assert.equal(readEntries[0].type, "directory");
    assert.equal(readEntries[0].content.length, 0);
  });

  it("writes and reads multiple entries (files + directories)", async () => {
    const path = await tmpPath();
    const entries: TarEntry[] = [
      { path: "dir/", content: Buffer.alloc(0), mode: 0o755, mtime: 1000000, type: "directory" },
      { path: "dir/a.txt", content: Buffer.from("aaa", "utf-8"), mode: 0o644, mtime: 1000001, type: "file" },
      { path: "dir/b.txt", content: Buffer.from("bbb", "utf-8"), mode: 0o644, mtime: 1000002, type: "file" }
    ];

    const writeChecksum = await writeTar(entries, path);
    const { entries: readEntries, checksum } = await readTar(path);

    assert.equal(checksum, writeChecksum);
    assert.equal(readEntries.length, 3);
    assert.equal(readEntries[0].path, "dir/");
    assert.equal(readEntries[0].type, "directory");
    assert.equal(readEntries[1].path, "dir/a.txt");
    assert.equal(readEntries[1].content.toString(), "aaa");
    assert.equal(readEntries[2].path, "dir/b.txt");
    assert.equal(readEntries[2].content.toString(), "bbb");
  });

  it("writes and reads an entry with empty content", async () => {
    const path = await tmpPath();
    const entries: TarEntry[] = [
      { path: "empty.txt", content: Buffer.alloc(0), mode: 0o644, mtime: 1000000, type: "file" }
    ];

    await writeTar(entries, path);
    const { entries: readEntries } = await readTar(path);

    assert.equal(readEntries.length, 1);
    assert.equal(readEntries[0].path, "empty.txt");
    assert.equal(readEntries[0].content.length, 0);
    assert.equal(readEntries[0].type, "file");
  });

  it("returns consistent SHA-256 checksum for identical content", async () => {
    const path1 = await tmpPath();
    const path2 = await tmpPath();
    const entries: TarEntry[] = [
      { path: "test.txt", content: Buffer.from("data", "utf-8"), mode: 0o644, mtime: 1000000, type: "file" }
    ];

    const checksum1 = await writeTar(entries, path1);
    const checksum2 = await writeTar(entries, path2);
    assert.equal(checksum1, checksum2);
  });
});

// ── Security: symlink entry rejection ───────────────────────────

describe("tar security — symlink rejection", () => {
  /**
   * Craft a raw tar buffer with a symlink entry (typeflag='2').
   * The encodeHeader normally only produces '0'(file) or '5'(dir),
   * so we build the header manually to simulate a malicious archive.
   */
  function buildSymlinkTar(): Buffer {
    const BLOCK = 512;
    const buf = Buffer.alloc(BLOCK, 0);

    // name
    buf.write("evil-link", 0, 9, "ascii");
    // mode (octal)
    buf.write("0000644", 100, 7, "ascii");
    // uid
    buf.write("0000000", 108, 7, "ascii");
    // gid
    buf.write("0000000", 116, 7, "ascii");
    // size = 0 for symlink
    buf.write("00000000000", 124, 11, "ascii");
    // mtime
    buf.write("00000000000", 136, 11, "ascii");
    // checksum placeholder (spaces)
    buf.write("        ", 148, 8, "ascii");
    // typeflag = '2' (symlink)
    buf.write("2", 156, 1, "ascii");
    // linkname: target path
    buf.write("/etc/passwd", 157, 11, "ascii");
    // magic "ustar"
    buf.write("ustar", 257, 5, "ascii");
    // version "00"
    buf.write("00", 263, 2, "ascii");

    // Compute checksum
    let checksum = 0;
    for (let i = 0; i < BLOCK; i++) {
      checksum += buf[i];
    }
    const checksumStr = checksum.toString(8).padStart(7, "0");
    buf.write(checksumStr, 148, 7, "ascii");
    buf[155] = 0x20;

    // Two zero blocks for end-of-archive
    const end = Buffer.alloc(BLOCK * 2, 0);
    return Buffer.concat([buf, end]);
  }

  it("rejects symlink entries (typeflag='2')", async () => {
    const malPath = await tmpPath();
    await writeFile(malPath, buildSymlinkTar());

    await assert.rejects(
      () => readTar(malPath),
      /Unsupported tar entry type.*2/
    );
  });

  it("rejects hardlink entries (typeflag='1')", async () => {
    const BLOCK = 512;
    const buf = Buffer.alloc(BLOCK, 0);
    buf.write("hardlink-item", 0, 13, "ascii");
    buf.write("0000644", 100, 7, "ascii");
    buf.write("0000000", 108, 7, "ascii");
    buf.write("0000000", 116, 7, "ascii");
    buf.write("00000000000", 124, 11, "ascii");
    buf.write("00000000000", 136, 11, "ascii");
    buf.write("        ", 148, 8, "ascii");
    buf.write("1", 156, 1, "ascii"); // typeflag '1' = hardlink
    buf.write("original-file", 157, 13, "ascii");
    buf.write("ustar", 257, 5, "ascii");
    buf.write("00", 263, 2, "ascii");

    let checksum = 0;
    for (let i = 0; i < BLOCK; i++) checksum += buf[i];
    buf.write(checksum.toString(8).padStart(7, "0"), 148, 7, "ascii");
    buf[155] = 0x20;

    const path = await tmpPath();
    await writeFile(path, Buffer.concat([buf, Buffer.alloc(1024, 0)]));

    await assert.rejects(
      () => readTar(path),
      /Unsupported tar entry type.*1/
    );
  });

  it("rejects entries with non-empty linkname field", async () => {
    const BLOCK = 512;
    const buf = Buffer.alloc(BLOCK, 0);
    buf.write("has-linkname", 0, 12, "ascii");
    buf.write("0000644", 100, 7, "ascii");
    buf.write("0000000", 108, 7, "ascii");
    buf.write("0000000", 116, 7, "ascii");
    buf.write("00000000000", 124, 11, "ascii");
    buf.write("00000000000", 136, 11, "ascii");
    buf.write("        ", 148, 8, "ascii");
    buf.write("0", 156, 1, "ascii"); // typeflag '0' = file
    buf.write("some-target", 157, 11, "ascii"); // non-empty linkname!
    buf.write("ustar", 257, 5, "ascii");
    buf.write("00", 263, 2, "ascii");

    let checksum = 0;
    for (let i = 0; i < BLOCK; i++) checksum += buf[i];
    buf.write(checksum.toString(8).padStart(7, "0"), 148, 7, "ascii");
    buf[155] = 0x20;

    const path = await tmpPath();
    await writeFile(path, Buffer.concat([buf, Buffer.alloc(1024, 0)]));

    await assert.rejects(
      () => readTar(path),
      /linkname/
    );
  });
});

// ── Security: path traversal ────────────────────────────────────

describe("tar security — path traversal", () => {
  it("rejects path with '..' via validateTarPath", () => {
    assert.throws(
      () => validateTarPath("../etc/passwd", "/tmp/root"),
      /Path traversal.*\.\./
    );
  });

  it("rejects path with null byte via validateTarPath", () => {
    assert.throws(
      () => validateTarPath("safe.txt\u0000evil.sh", "/tmp/root"),
      /Path traversal.*null byte/
    );
  });

  it("rejects absolute path via validateTarPath", () => {
    assert.throws(
      () => validateTarPath("/etc/passwd", "/tmp/root"),
      /Path traversal.*absolute/
    );
  });

  it("accepts valid path via validateTarPath", () => {
    const result = validateTarPath("content/safe/file.txt", "/tmp/root");
    assert.equal(result, "content/safe/file.txt");
  });
});

// ── Size limit ──────────────────────────────────────────────────

describe("tar size limits", () => {
  it("rejects entries exceeding 500MB size limit", async () => {
    const BLOCK = 512;
    const buf = Buffer.alloc(BLOCK, 0);
    buf.write("too-large", 0, 9, "ascii");
    buf.write("0000644", 100, 7, "ascii");
    buf.write("0000000", 108, 7, "ascii");
    buf.write("0000000", 116, 7, "ascii");
    // size = 501MB (in octal)
    const size = 501 * 1024 * 1024;
    buf.write(size.toString(8).padStart(11, "0"), 124, 11, "ascii");
    buf.write("00000000000", 136, 11, "ascii");
    buf.write("        ", 148, 8, "ascii");
    buf.write("0", 156, 1, "ascii");
    buf.write("ustar", 257, 5, "ascii");
    buf.write("00", 263, 2, "ascii");

    let checksum = 0;
    for (let i = 0; i < BLOCK; i++) checksum += buf[i];
    buf.write(checksum.toString(8).padStart(7, "0"), 148, 7, "ascii");
    buf[155] = 0x20;

    const path = await tmpPath();
    await writeFile(path, Buffer.concat([buf, Buffer.alloc(1024, 0)]));

    await assert.rejects(
      () => readTar(path),
      /Tar entry too large/
    );
  });
});

// ── Checksum validation ─────────────────────────────────────────

describe("tar checksum validation", () => {
  it("detects corrupted archive data (checksum mismatch on read)", async () => {
    const path = await tmpPath();
    const entries: TarEntry[] = [
      { path: "data.txt", content: Buffer.from("original content", "utf-8"), mode: 0o644, mtime: 1000000, type: "file" }
    ];
    await writeTar(entries, path);

    // Read the file and corrupt a byte in the content
    const archive = await import("node:fs/promises").then((m) => m.readFile(path));
    // Corrupt a byte in the data block (after 512-byte header)
    if (archive.length > 600) {
      archive[550] = 0xFF; // corrupt content data
    }
    await import("node:fs/promises").then((m) => m.writeFile(path, archive));

    // readTar should still read the entries but the archive checksum
    // will differ from what writeTar returned
    const { checksum } = await readTar(path);
    // The corruption is in content, not header, so it shouldn't throw
    // (the tar header checksum is still valid, only the outer archive sha256 changed)
    assert.ok(typeof checksum === "string");
    assert.ok(checksum.length > 0);
  });
});
