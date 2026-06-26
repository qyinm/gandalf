//! Minimal POSIX ustar tar writer/reader for `.hem` bundles.

use std::fs;
use std::io;
use std::path::Path;

use sha2::{Digest, Sha256};

use crate::types::{TarEntry, TarEntryType};

const BLOCK_SIZE: usize = 512;
const HEADER_SIZE: usize = 512;
const MAGIC: &str = "ustar";
const MAX_ENTRY_BYTES: u64 = 500 * 1024 * 1024;

#[derive(Debug, thiserror::Error)]
pub enum TarError {
    #[error("{0}")]
    Message(String),
    #[error(transparent)]
    Io(#[from] io::Error),
}

pub type TarResult<T> = Result<T, TarError>;

#[derive(Debug, Clone)]
struct TarHeader {
    name: String,
    mode: u32,
    uid: u32,
    gid: u32,
    size: u64,
    mtime: u64,
    checksum: String,
    typeflag: char,
    linkname: String,
    magic: String,
    version: String,
    uname: String,
    gname: String,
    devmajor: u32,
    devminor: u32,
    prefix: String,
}

/// Write a tar archive to a file and return the SHA-256 hex digest of the archive.
pub fn write_tar(entries: &[TarEntry], output_path: &Path) -> TarResult<String> {
    let mut blocks: Vec<u8> = Vec::new();

    for entry in entries {
        let header = encode_header(entry)?;
        blocks.extend_from_slice(&header);

        if entry.entry_type == TarEntryType::File && !entry.content.is_empty() {
            blocks.extend_from_slice(&entry.content);
            let padding = BLOCK_SIZE - (entry.content.len() % BLOCK_SIZE);
            if padding < BLOCK_SIZE {
                blocks.extend(std::iter::repeat_n(0u8, padding));
            }
        }
    }

    blocks.extend(std::iter::repeat_n(0u8, BLOCK_SIZE));
    blocks.extend(std::iter::repeat_n(0u8, BLOCK_SIZE));

    if let Some(parent) = output_path.parent() {
        fs::create_dir_all(parent)?;
    }
    fs::write(output_path, &blocks)?;

    Ok(hex_digest(&blocks))
}

/// Read a tar archive from a file.
pub fn read_tar(input_path: &Path) -> TarResult<(Vec<TarEntry>, String)> {
    let archive = fs::read(input_path)?;
    let checksum = hex_digest(&archive);
    let mut entries = Vec::new();
    let mut offset = 0usize;

    while offset + HEADER_SIZE <= archive.len() {
        let header_block = &archive[offset..offset + HEADER_SIZE];

        if is_zero_block(header_block) {
            offset += BLOCK_SIZE;
            if offset + BLOCK_SIZE <= archive.len()
                && is_zero_block(&archive[offset..offset + BLOCK_SIZE])
            {
                break;
            }
            continue;
        }

        let header = decode_header(header_block)?;
        offset += HEADER_SIZE;

        if header.typeflag != '0' && header.typeflag != '5' {
            return Err(TarError::Message(format!(
                "Unsupported tar entry type \"{}\" for \"{}\". \
                 Only regular files (typeflag '0') and directories (typeflag '5') are accepted.",
                header.typeflag, header.name
            )));
        }

        if !header.linkname.is_empty() {
            return Err(TarError::Message(format!(
                "Tar entry \"{}\" has a non-empty linkname field (\"{}\"). \
                 Link entries are not supported for security reasons.",
                header.name, header.linkname
            )));
        }

        let size = if header.typeflag == '5' { 0 } else { header.size };

        if size > MAX_ENTRY_BYTES {
            return Err(TarError::Message(format!(
                "Tar entry too large: {} ({} bytes)",
                header.name, size
            )));
        }

        let content = if size > 0 {
            let end = offset + size as usize;
            if end > archive.len() {
                return Err(TarError::Message(format!(
                    "Truncated tar entry: {}",
                    header.name
                )));
            }
            archive[offset..end].to_vec()
        } else {
            Vec::new()
        };

        entries.push(TarEntry {
            path: header.name,
            content,
            mode: header.mode,
            mtime: header.mtime.saturating_mul(1000),
            entry_type: if header.typeflag == '5' {
                TarEntryType::Directory
            } else {
                TarEntryType::File
            },
        });

        let data_blocks = size.div_ceil(BLOCK_SIZE as u64) as usize;
        offset += data_blocks * BLOCK_SIZE;
    }

    Ok((entries, checksum))
}

/// Validate that a tar entry path is safe (no path traversal).
pub fn validate_tar_path(entry_path: &str, root: &Path) -> TarResult<String> {
    if entry_path.contains("..") {
        return Err(TarError::Message(format!(
            "Path traversal detected: \"{entry_path}\" contains \"..\""
        )));
    }
    if entry_path.contains('\0') {
        return Err(TarError::Message(format!(
            "Path traversal detected: \"{entry_path}\" contains null byte"
        )));
    }
    if Path::new(entry_path).is_absolute() {
        return Err(TarError::Message(format!(
            "Path traversal detected: \"{entry_path}\" is absolute"
        )));
    }

    let resolved = root.join(entry_path);
    let normalized_root = root.canonicalize().unwrap_or_else(|_| root.to_path_buf());
    let normalized_resolved = resolved.canonicalize().unwrap_or(resolved);

    let root_str = normalized_root.to_string_lossy();
    let resolved_str = normalized_resolved.to_string_lossy();
    if resolved_str != root_str && !resolved_str.starts_with(&format!("{root_str}{}", MAIN_SEPARATOR)) {
        return Err(TarError::Message(format!(
            "Path traversal detected: \"{entry_path}\" resolves outside root"
        )));
    }

    Ok(entry_path.to_string())
}

fn encode_header(entry: &TarEntry) -> TarResult<Vec<u8>> {
    let mut buf = vec![0u8; HEADER_SIZE];
    let name = &entry.path;
    let prefix = "";

    write_field(&mut buf, 0, 100, name);
    write_field(&mut buf, 100, 8, &format!("{:07o}", entry.mode & 0o7777));
    write_field(&mut buf, 108, 8, "0000000");
    write_field(&mut buf, 116, 8, "0000000");
    let size_field = if entry.entry_type == TarEntryType::File {
        format!("{:011o}", entry.content.len())
    } else {
        "00000000000".to_string()
    };
    write_field(&mut buf, 124, 12, &size_field);
    write_field(
        &mut buf,
        136,
        12,
        &format!("{:011o}", entry.mtime / 1000),
    );
    write_field(&mut buf, 148, 8, &" ".repeat(8));
    let typeflag = if entry.entry_type == TarEntryType::Directory {
        '5'
    } else {
        '0'
    };
    write_field(&mut buf, 156, 1, &typeflag.to_string());
    write_field(&mut buf, 157, 100, "");
    write_field(&mut buf, 257, 6, MAGIC);
    write_field(&mut buf, 263, 2, "00");
    write_field(&mut buf, 265, 32, "");
    write_field(&mut buf, 297, 32, "");
    write_field(&mut buf, 329, 8, "0000000");
    write_field(&mut buf, 337, 8, "0000000");
    write_field(&mut buf, 345, 155, prefix);

    let checksum: u32 = buf.iter().map(|&b| u32::from(b)).sum();
    let checksum_str = format!("{:07o}", checksum);
    write_field(&mut buf, 148, 7, &checksum_str);
    buf[155] = 0x20;

    Ok(buf)
}

fn decode_header(buf: &[u8]) -> TarResult<TarHeader> {
    let name = read_field(buf, 0, 100);
    let mode = parse_octal_field(read_field(buf, 100, 8));
    let uid = parse_octal_field(read_field(buf, 108, 8));
    let gid = parse_octal_field(read_field(buf, 116, 8));
    let size = parse_octal_field(read_field(buf, 124, 12));
    let mtime = parse_octal_field(read_field(buf, 136, 12));
    let checksum = read_field(buf, 148, 8);
    let typeflag = read_field(buf, 156, 1)
        .chars()
        .next()
        .unwrap_or('\0');
    let linkname = read_field(buf, 157, 100);
    let magic = read_field(buf, 257, 6);
    let version = read_field(buf, 263, 2);
    let uname = read_field(buf, 265, 32);
    let gname = read_field(buf, 297, 32);
    let devmajor = parse_octal_field(read_field(buf, 329, 8));
    let devminor = parse_octal_field(read_field(buf, 337, 8));
    let prefix = read_field(buf, 345, 155);

    let saved_checksum = parse_octal_field_u32(checksum.trim());
    let computed: u32 = buf.iter().map(|&b| u32::from(b)).sum();
    let mut recomputed_buf = buf.to_vec();
    write_field(&mut recomputed_buf, 148, 8, &" ".repeat(8));
    let actual: u32 = recomputed_buf.iter().map(|&b| u32::from(b)).sum();

    if saved_checksum != 0 && saved_checksum != actual {
        if saved_checksum != computed && saved_checksum != actual {
            return Err(TarError::Message(format!(
                "Checksum mismatch for tar entry \"{name}\": expected {}, got {}",
                format_octal(actual),
                format_octal(saved_checksum)
            )));
        }
    }

    Ok(TarHeader {
        name,
        mode: mode as u32,
        uid: uid as u32,
        gid: gid as u32,
        size,
        mtime,
        checksum,
        typeflag,
        linkname,
        magic,
        version,
        uname,
        gname,
        devmajor: devmajor as u32,
        devminor: devminor as u32,
        prefix,
    })
}

fn write_field(buf: &mut [u8], offset: usize, length: usize, value: &str) {
    let end = offset + length;
    let slice = &mut buf[offset..end];
    slice.fill(0);
    let bytes = value.as_bytes();
    let copy_len = bytes.len().min(length);
    slice[..copy_len].copy_from_slice(&bytes[..copy_len]);
}

fn read_field(buf: &[u8], offset: usize, length: usize) -> String {
    let end = offset + length;
    let mut null_pos = offset;
    while null_pos < end && buf[null_pos] != 0 {
        null_pos += 1;
    }
    String::from_utf8_lossy(&buf[offset..null_pos]).into_owned()
}

fn parse_octal_field(value: String) -> u64 {
    let trimmed = value.trim_matches('\0').trim();
    if trimmed.is_empty() {
        0
    } else {
        u64::from_str_radix(trimmed, 8).unwrap_or(0)
    }
}

fn parse_octal_field_u32(value: &str) -> u32 {
    let trimmed = value.trim_matches('\0').trim();
    if trimmed.is_empty() {
        0
    } else {
        u32::from_str_radix(trimmed, 8).unwrap_or(0)
    }
}

fn format_octal(value: u32) -> String {
    format!("{value:o}")
}

fn is_zero_block(buf: &[u8]) -> bool {
    buf.iter().all(|&b| b == 0)
}

fn hex_digest(data: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(data);
    hasher
        .finalize()
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

#[cfg(unix)]
const MAIN_SEPARATOR: char = '/';

#[cfg(windows)]
const MAIN_SEPARATOR: char = '\\';