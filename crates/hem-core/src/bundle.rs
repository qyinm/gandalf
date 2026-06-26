//! `.hem` bundle export, import, inspect, and verify.

use std::collections::HashMap;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use hmac::{Hmac, Mac};
use sha2::{Digest, Sha256};

use crate::path_confinement::{
    validate_constrained_write_path, validate_home_relative_import_segment, ConfinementRoots,
};
use crate::policy::restore_policy_for;
use crate::restore::write_file_atomically;
use crate::readiness::{build_readiness_report, check_mcp_binary_availability, current_platform, extract_mcp_binaries, ReadinessOptions};
use crate::scan::scan_project;
use crate::store::{read_snapshot, write_snapshot, StoreError, StoreSnapshot};
use crate::tar::{read_tar, write_tar};
use crate::types::{
    BundleChecksums, BundleExportOptions, BundleExportResult, BundleImportOptions,
    BundleImportResult, BundleInspectResult, BundleManifest, BundleSecurity, BundleVerifyOptions,
    BundleVerifyResult, BundleVerifyChecksumResult, BundleVerifySignatureResult, DiscoveredItem,
    GraphNode, MachineDiff, ProvenanceEntry, ReadinessCategory, ScanOptions, Snapshot,
    SnapshotManifest, SnapshotSecurity, SourceMachine, TarEntry, TarEntryType,
};

type HmacSha256 = Hmac<Sha256>;

const FORMAT_VERSION: &str = "1";
const HOME_TOKEN: &str = "{home}";
const SIGNATURE_ALGORITHM: &str = "HMAC-SHA256";

fn max_bundle_bytes() -> u64 {
    std::env::var("HEM_MAX_BUNDLE_BYTES")
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or(500 * 1024 * 1024)
}

fn max_content_bytes() -> u64 {
    std::env::var("HEM_MAX_CONTENT_BYTES")
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or(50 * 1024 * 1024)
}

#[derive(Debug, thiserror::Error)]
pub enum BundleError {
    #[error("{0}")]
    Message(String),
    #[error(transparent)]
    Store(#[from] StoreError),
    #[error(transparent)]
    Io(#[from] io::Error),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
    #[error(transparent)]
    Tar(#[from] crate::tar::TarError),
    #[error(transparent)]
    Hmac(#[from] hmac::digest::InvalidLength),
}

pub type BundleResult<T> = Result<T, BundleError>;

pub fn bundle_export(options: &BundleExportOptions) -> BundleResult<BundleExportResult> {
    let signature_key = resolve_signature_key(options.signature_key.as_deref());
    let include_content = options.include_content.unwrap_or(false);

    let snapshot = read_snapshot(
        Path::new(&options.store_dir),
        &options.snapshot_name,
        options.agent,
    )?;

    let unsafe_items: Vec<_> = snapshot
        .evidence
        .iter()
        .filter(|item| item.capture_status == crate::types::CaptureStatus::UnsafeToExport)
        .collect();
    if !unsafe_items.is_empty() {
        return Err(BundleError::Message(format!(
            "Cannot export: {} evidence item(s) are marked unsafe_to_export. First: {}",
            unsafe_items.len(),
            unsafe_items[0].source_path
        )));
    }

    if include_content {
        let unsupported: Vec<_> = snapshot
            .evidence
            .iter()
            .filter(|item| {
                item.capture_status == crate::types::CaptureStatus::Captured
                    && restore_policy_for(item.kind) == crate::types::RestorePolicy::NotSupported
            })
            .collect();
        if !unsupported.is_empty() {
            return Err(BundleError::Message(format!(
                "Cannot export content bundle: {} not_supported evidence item(s) would lose restore data. First: {} (kind: {}). Use --metadata-only or remove unsupported items before exporting a restorable content bundle.",
                unsupported.len(),
                unsupported[0].source_path,
                unsupported[0].kind.as_str()
            )));
        }
    }

    let mut warnings = Vec::new();
    let source_machine = capture_source_machine();
    let now = now_millis();
    let mut entries = Vec::new();

    entries.push(dir_entry(".hem/", now));
    entries.push(file_entry(
        ".hem/format-version",
        format!("{FORMAT_VERSION}\n").into_bytes(),
        now,
    ));

    let mut manifest = BundleManifest {
        format_version: 1,
        snapshot_name: options.snapshot_name.clone(),
        created_at: snapshot.manifest.created_at.clone(),
        project_path: options.project_path.clone(),
        includes_content: include_content,
        content_file_count: 0,
        content_total_bytes: 0,
        source_machine: Some(source_machine),
        security: BundleSecurity {
            raw_secrets_included: false,
            redaction_policy: "metadata-only".to_string(),
            signed: signature_key.is_some(),
            signature_algorithm: signature_key
                .as_ref()
                .map(|_| SIGNATURE_ALGORITHM.to_string()),
            signature: None,
        },
    };

    entries.push(file_entry(
        ".hem/manifest.json",
        json_pretty_line(&manifest)?,
        now,
    ));
    entries.push(dir_entry("snapshot/", now));

    let bundled_evidence =
        normalise_snapshot_paths_for_bundle(&snapshot.evidence, &options.home_dir);
    let bundled_graph = normalise_snapshot_paths_for_bundle(&snapshot.graph, &options.home_dir);
    let bundled_provenance =
        normalise_snapshot_paths_for_bundle(&snapshot.provenance, &options.home_dir);

    let snapshot_files: [(&str, serde_json::Value); 6] = [
        ("evidence.json", serde_json::to_value(&bundled_evidence)?),
        ("graph.json", serde_json::to_value(&bundled_graph)?),
        (
            "audit-findings.json",
            serde_json::to_value(&snapshot.audit_findings)?,
        ),
        ("provenance.json", serde_json::to_value(&bundled_provenance)?),
        ("checksums.json", serde_json::json!({})),
        ("redactions.json", serde_json::json!([])),
    ];

    for (name, data) in snapshot_files {
        entries.push(file_entry(
            &format!("snapshot/{name}"),
            json_pretty_line(&data)?,
            now,
        ));
    }

    if include_content {
        let mut total_content_bytes = 0u64;
        let mut content_count = 0u32;

        let not_supported: Vec<_> = snapshot
            .evidence
            .iter()
            .filter(|item| restore_policy_for(item.kind) == crate::types::RestorePolicy::NotSupported)
            .collect();
        if !not_supported.is_empty() {
            warnings.push(format!(
                "{} evidence item(s) have restorePolicy=not_supported and will not be included as content. First: {} (kind: {})",
                not_supported.len(),
                not_supported[0].source_path,
                not_supported[0].kind.as_str()
            ));
        }

        let content_items: Vec<_> = snapshot
            .evidence
            .iter()
            .filter(|item| {
                item.capture_status == crate::types::CaptureStatus::Captured
                    && restore_policy_for(item.kind)
                        == crate::types::RestorePolicy::FullContentSupported
                    && !item.source_path.is_empty()
                    && !item.source_path.starts_with("~/.env")
            })
            .collect();

        let mut seen_paths = std::collections::HashSet::new();
        let mut unique_items = Vec::new();
        for item in content_items {
            if seen_paths.insert(item.source_path.clone()) {
                unique_items.push(item);
            }
        }

        entries.push(dir_entry("content/", now));

        for item in unique_items {
            let source_abs = resolve_source_path(
                &item.source_path,
                &options.home_dir,
                &options.project_path,
            );
            let metadata = match fs::metadata(&source_abs) {
                Ok(metadata) => metadata,
                Err(_) => continue,
            };
            if !metadata.is_file() {
                continue;
            }
            if metadata.len() > max_content_bytes() {
                warnings.push(format!(
                    "Skipped large file: {} ({} bytes > {} limit)",
                    item.source_path,
                    metadata.len(),
                    max_content_bytes()
                ));
                continue;
            }

            let content = match fs::read(&source_abs) {
                Ok(content) => content,
                Err(_) => continue,
            };
            let normalised_path = normalise_source_path(&item.source_path, &options.home_dir);
            let tar_path = format!("content/{normalised_path}");
            let mtime = metadata
                .modified()
                .ok()
                .and_then(|time| time.duration_since(UNIX_EPOCH).ok())
                .map(|duration| duration.as_millis() as u64)
                .unwrap_or(now);
            entries.push(TarEntry {
                path: tar_path,
                content,
                mode: 0o644,
                mtime,
                entry_type: TarEntryType::File,
            });
            total_content_bytes += metadata.len();
            content_count += 1;
        }

        let structured_items: Vec<_> = snapshot
            .evidence
            .iter()
            .filter(|item| {
                item.capture_status == crate::types::CaptureStatus::Captured
                    && matches!(
                        restore_policy_for(item.kind),
                        crate::types::RestorePolicy::StructuredFieldsOnly
                            | crate::types::RestorePolicy::KeyInventoryOnly
                    )
            })
            .collect();
        if !structured_items.is_empty() {
            let kinds: Vec<_> = structured_items
                .iter()
                .map(|item| item.kind.as_str())
                .collect::<std::collections::HashSet<_>>()
                .into_iter()
                .collect();
            warnings.push(format!(
                "{} evidence item(s) ({}) use structured/key-inventory capture. Data is in evidence.json, not included as separate content files.",
                structured_items.len(),
                kinds.join(", ")
            ));
        }

        manifest.content_file_count = content_count;
        manifest.content_total_bytes = total_content_bytes;
        update_manifest_entry(&mut entries, &manifest)?;
    }

    if let Some(key) = signature_key.as_ref() {
        manifest.security.signature = Some(sign_bundle_entries(&entries, &manifest, key)?);
        update_manifest_entry(&mut entries, &manifest)?;
    }

    let entry_checksums = compute_entry_checksums(&entries);
    let checksums_entry = TarEntry {
        path: ".hem/checksums.json".to_string(),
        content: json_pretty_line(&BundleChecksums {
            algorithm: "SHA-256".to_string(),
            entries: entry_checksums,
        })?,
        mode: 0o644,
        mtime: now,
        entry_type: TarEntryType::File,
    };

    let mut final_entries = Vec::new();
    let mut checksums_inserted = false;
    for entry in entries {
        final_entries.push(entry);
        if final_entries
            .last()
            .is_some_and(|entry| entry.path == ".hem/manifest.json")
            && !checksums_inserted
        {
            final_entries.push(checksums_entry.clone());
            checksums_inserted = true;
        }
    }
    if !checksums_inserted {
        final_entries.push(checksums_entry);
    }

    let output_path = Path::new(&options.output_path);
    let archive_checksum = write_tar(&final_entries, output_path)?;

    Ok(BundleExportResult {
        bundle_path: options.output_path.clone(),
        checksum: archive_checksum,
        warnings,
    })
}

pub fn bundle_import(options: &BundleImportOptions) -> BundleResult<BundleImportResult> {
    let bundle_path = Path::new(&options.bundle_path);
    let signature_key = resolve_signature_key(options.signature_key.as_deref());
    let apply_content = options.apply_content.unwrap_or(false);
    let dry_run = options.dry_run.unwrap_or(false);
    let quarantine = options.quarantine.unwrap_or(false);

    let (entries, _bundle_checksum) = read_tar(bundle_path)?;

    let format_entry = entries
        .iter()
        .find(|entry| entry.path == ".hem/format-version")
        .ok_or_else(|| BundleError::Message("Invalid bundle: missing .hem/format-version".into()))?;
    let format_version = String::from_utf8_lossy(&format_entry.content)
        .trim()
        .to_string();
    if format_version != FORMAT_VERSION {
        return Err(BundleError::Message(format!(
            "Unsupported bundle format version: \"{format_version}\" (expected \"{FORMAT_VERSION}\")"
        )));
    }

    let manifest_entry = entries
        .iter()
        .find(|entry| entry.path == ".hem/manifest.json")
        .ok_or_else(|| BundleError::Message("Invalid bundle: missing .hem/manifest.json".into()))?;
    let manifest: BundleManifest = serde_json::from_slice(&manifest_entry.content)?;

    let signature_verification =
        verify_bundle_signature(&entries, &manifest, signature_key.as_deref());
    if !signature_verification.ok {
        return Err(BundleError::Message(
            signature_verification
                .warning
                .unwrap_or_else(|| "Bundle signature verification failed.".to_string()),
        ));
    }
    let trust_warning = if signature_verification.checked && signature_key.is_some() {
        enforce_bundle_key_trust(
            Path::new(&options.store_dir),
            signature_key.as_deref().unwrap_or_default(),
            options.trust.unwrap_or(false),
        )?
    } else {
        None
    };

    if let Some(checksums_entry) = entries.iter().find(|entry| entry.path == ".hem/checksums.json")
    {
        let checksums: BundleChecksums = serde_json::from_slice(&checksums_entry.content)?;
        for entry in &entries {
            if entry.path == ".hem/checksums.json" {
                continue;
            }
            if let Some(expected) = checksums.entries.get(&entry.path) {
                let actual = sha256_hex(&entry.content);
                if &actual != expected {
                    return Err(BundleError::Message(format!(
                        "Checksum mismatch for \"{}\": expected {expected}, got {actual}",
                        entry.path
                    )));
                }
            }
        }
    }

    let home_dir = Path::new(&options.home_dir);
    let project_path = Path::new(&options.project_path);
    let all_roots = [home_dir, project_path];

    for entry in &entries {
        if entry.path.contains("..") {
            return Err(BundleError::Message(format!(
                "Path traversal detected: \"{}\" contains \"..\"",
                entry.path
            )));
        }
        if entry.path.contains('\0') {
            return Err(BundleError::Message(format!(
                "Path traversal detected: \"{}\" contains null byte",
                entry.path
            )));
        }
        if Path::new(&entry.path).is_absolute() {
            return Err(BundleError::Message(format!(
                "Path traversal detected: \"{}\" is absolute",
                entry.path
            )));
        }

        if entry.path.starts_with("content/") {
            let relative_path = entry.path.strip_prefix("content/").unwrap_or(&entry.path);
            let resolved = resolve_bundle_path(relative_path, home_dir, project_path);
            if !all_roots
                .iter()
                .any(|root| is_strictly_under(&resolved, root))
            {
                return Err(BundleError::Message(format!(
                    "Content path \"{relative_path}\" resolves outside home and project directories"
                )));
            }
        } else if !all_roots.iter().any(|root| {
            is_strictly_under(&root.join(&entry.path), root)
        }) {
            return Err(BundleError::Message(format!(
                "Entry path \"{}\" is not valid",
                entry.path
            )));
        }
    }

    let bundle_size = fs::metadata(bundle_path)?.len();
    if bundle_size > max_bundle_bytes() {
        return Err(BundleError::Message(format!(
            "Bundle too large: {bundle_size} bytes (max {})",
            max_bundle_bytes()
        )));
    }

    if apply_content {
        const BLOCKED_HOME_PREFIXES: &[&str] = &[
            ".ssh", ".aws", ".gnupg", ".config", ".local", ".npm", ".docker", ".kube",
            ".credentials", ".heroku", ".netrc", ".env", ".gitconfig", ".git-credentials",
            ".npmrc", ".bash_profile", ".bashrc", ".zshrc", ".profile", ".pgpass", ".gem",
        ];

        for entry in entries
            .iter()
            .filter(|entry| entry.path.starts_with("content/"))
        {
            let relative_path = entry.path.strip_prefix("content/").unwrap_or(&entry.path);
            let is_home_relative = relative_path.starts_with("~/")
                || relative_path.starts_with(&format!("{HOME_TOKEN}/"));
            if is_home_relative {
                return Err(BundleError::Message(format!(
                    "Home-relative content path \"{relative_path}\" is not allowed. Bundle content must be project-relative."
                )));
            }

            for prefix in BLOCKED_HOME_PREFIXES {
                if relative_path.contains(&format!("/{prefix}/"))
                    || relative_path.starts_with(&format!("{prefix}/"))
                {
                    return Err(BundleError::Message(format!(
                        "Blocked content path prefix: \"{relative_path}\""
                    )));
                }
            }

            let resolved = resolve_bundle_path(relative_path, home_dir, project_path);
            if !is_strictly_under(&resolved, home_dir) && !is_strictly_under(&resolved, project_path)
            {
                return Err(BundleError::Message(format!(
                    "Content path \"{relative_path}\" resolves outside home and project directories"
                )));
            }
        }
    }

    let target_home = options.home_dir.clone();
    let target_platform = options
        .target_platform
        .clone()
        .unwrap_or_else(|| current_platform().to_string());
    let target_hostname = hostname();

    let source_machine = manifest.source_machine.clone();
    let source_home = source_machine.as_ref().map(|machine| {
        normalise_home_for_platform(&machine.home_dir, &machine.platform)
    });
    let normalised_target_home =
        normalise_home_for_platform(&target_home, &target_platform);

    let mut remapped_paths = Vec::new();
    for entry in entries
        .iter()
        .filter(|entry| entry.path.starts_with("content/") && entry.entry_type == TarEntryType::File)
    {
        let relative_path = entry.path.strip_prefix("content/").unwrap_or(&entry.path);
        if let Some(home_relative) = relative_path.strip_prefix(&format!("{HOME_TOKEN}/")) {
            let source_abs = source_home
                .as_ref()
                .map(|home| format!("{home}/{home_relative}"))
                .unwrap_or_else(|| format!("source:{home_relative}"));
            let target_abs = format!("{normalised_target_home}/{home_relative}");
            remapped_paths.push(format!("{source_abs} → {target_abs}"));
        }
    }

    let cross_os = source_machine
        .as_ref()
        .is_some_and(|machine| machine.platform != target_platform);
    let mut os_differences = Vec::new();
    if cross_os {
        if let Some(machine) = &source_machine {
            os_differences.push(format!(
                "{} → {target_platform} (cross-OS restore)",
                machine.platform
            ));
            if machine.platform == "darwin" && target_platform != "darwin" {
                os_differences.push(
                    "macOS extended attributes and ACLs not preserved on non-macOS".to_string(),
                );
            }
            if machine.platform == "darwin" {
                os_differences.push(
                    "macOS uses case-insensitive FS by default; Linux is case-sensitive — filename conflicts possible".to_string(),
                );
            }
            if target_platform == "win32" {
                os_differences.push(
                    "Windows uses \\ path separator — paths will be normalized".to_string(),
                );
            }
            os_differences.push(
                "Binary/script files copied as-is; manual line-ending normalization may be needed"
                    .to_string(),
            );
        }
    }

    let evidence_entry = entries
        .iter()
        .find(|entry| entry.path == "snapshot/evidence.json");
    let source_mcp_binaries = evidence_entry
        .map(|entry| {
            let evidence: Vec<DiscoveredItem> = serde_json::from_slice(&entry.content).unwrap_or_default();
            extract_mcp_binaries(&evidence, source_home.as_deref())
        })
        .unwrap_or_default();
    let mcp_binary_report = check_mcp_binary_availability(&source_mcp_binaries);

    let machine_diff = MachineDiff {
        source_home: source_machine
            .as_ref()
            .map(|machine| machine.home_dir.clone())
            .unwrap_or_else(|| "unknown".to_string()),
        target_home: target_home.clone(),
        source_platform: source_machine
            .as_ref()
            .map(|machine| machine.platform.clone())
            .unwrap_or_else(|| "unknown".to_string()),
        target_platform: target_platform.clone(),
        source_hostname: source_machine
            .as_ref()
            .map(|machine| machine.hostname.clone())
            .unwrap_or_else(|| "unknown".to_string()),
        target_hostname: target_hostname.clone(),
        cross_os,
        os_differences,
        remapped_paths,
        source_mcp_binaries,
        mcp_binary_report,
    };

    let mut warnings = Vec::new();
    if let Some(warning) = signature_verification.warning {
        warnings.push(warning);
    }
    if let Some(warning) = trust_warning {
        warnings.push(warning);
    }
    if let Some(machine) = &source_machine {
        if machine.home_dir != target_home {
            warnings.push(format!(
                "Home directory differs: source={}, target={target_home}. {} path(s) will be remapped.",
                machine.home_dir,
                machine_diff.remapped_paths.len()
            ));
        }
        if machine.platform != target_platform {
            warnings.push(format!(
                "Platform differs: source={}, target={target_platform}. Cross-OS restore may have issues with binary paths and file permissions.",
                machine.platform
            ));
        }
    }

    let unavailable_binaries: Vec<_> = machine_diff
        .mcp_binary_report
        .iter()
        .filter(|report| !report.available_on_target)
        .collect();
    if !unavailable_binaries.is_empty() {
        warnings.push(format!(
            "{} MCP binary/bundles not found on this machine: {}",
            unavailable_binaries.len(),
            unavailable_binaries
                .iter()
                .map(|report| report.command.as_str())
                .collect::<Vec<_>>()
                .join(", ")
        ));
    }

    let source_evidence = evidence_entry
        .map(|entry| serde_json::from_slice::<Vec<DiscoveredItem>>(&entry.content))
        .transpose()?
        .unwrap_or_default();
    let target_evidence = scan_project(&ScanOptions {
        project_path: options.project_path.clone(),
        home_dir: options.home_dir.clone(),
        store_dir: options.store_dir.clone(),
        explain: None,
        agent: None,
        scope: None,
    })
    .evidence;

    let readiness = build_readiness_report(
        &source_evidence,
        &ReadinessOptions {
            source_home_dir: source_home.as_deref(),
            target_platform: Some(&target_platform),
            apply_content,
            target_evidence: Some(&target_evidence),
            process_env: None,
            path_env: None,
        },
    );

    let blocked_items: Vec<_> = readiness
        .items
        .iter()
        .filter(|item| item.category == ReadinessCategory::Blocked)
        .collect();
    if apply_content && !dry_run && !blocked_items.is_empty() {
        return Err(BundleError::Message(format!(
            "{}: {}",
            blocked_items[0].code, blocked_items[0].problem
        )));
    }

    if dry_run {
        let evidence_count = entries
            .iter()
            .filter(|entry| entry.path.starts_with("snapshot/"))
            .count();
        return Ok(BundleImportResult {
            snapshot_name: manifest.snapshot_name,
            evidence_count,
            includes_content: manifest.includes_content,
            content_applied: false,
            quarantined_content_dir: None,
            warnings,
            machine_diff: Some(machine_diff),
            readiness,
        });
    }

    let graph_entry = entries
        .iter()
        .find(|entry| entry.path == "snapshot/graph.json")
        .ok_or_else(|| BundleError::Message("Invalid bundle: missing snapshot data files".into()))?;
    let audit_entry = entries
        .iter()
        .find(|entry| entry.path == "snapshot/audit-findings.json")
        .ok_or_else(|| BundleError::Message("Invalid bundle: missing snapshot data files".into()))?;
    let provenance_entry = entries
        .iter()
        .find(|entry| entry.path == "snapshot/provenance.json");
    let evidence_entry = evidence_entry
        .ok_or_else(|| BundleError::Message("Invalid bundle: missing snapshot data files".into()))?;

    let imported_evidence = resolve_snapshot_paths_for_import(
        serde_json::from_slice(&evidence_entry.content)?,
        home_dir,
    )?;
    let imported_graph = resolve_snapshot_paths_for_import(
        serde_json::from_slice(&graph_entry.content)?,
        home_dir,
    )?;
    let imported_provenance = if let Some(entry) = provenance_entry {
        resolve_snapshot_paths_for_import(serde_json::from_slice(&entry.content)?, home_dir)?
    } else {
        Vec::new()
    };

    let snapshot = Snapshot {
        manifest: SnapshotManifest {
            schema_version: "0.1".to_string(),
            name: manifest.snapshot_name.clone(),
            created_at: manifest.created_at.clone(),
            project_path: manifest.project_path.clone(),
            security: SnapshotSecurity {
                raw_secrets_included: false,
                redaction_policy: "metadata-only".to_string(),
            },
        },
        evidence: imported_evidence.clone(),
        graph: imported_graph,
        audit_findings: serde_json::from_slice(&audit_entry.content)?,
        provenance: imported_provenance,
        content: None,
    };

    write_snapshot(
        Path::new(&options.store_dir),
        StoreSnapshot::from(snapshot),
        options.agent,
    )?;

    let mut content_applied = false;
    let mut quarantined_content_dir = None;
    if apply_content {
        let apply_entries: Vec<_> = entries
            .iter()
            .filter(|entry| entry.path.starts_with("content/") && entry.entry_type == TarEntryType::File)
            .collect();

        if quarantine {
            let safe_snapshot_name = manifest
                .snapshot_name
                .chars()
                .map(|ch| {
                    if ch.is_ascii_alphanumeric() || matches!(ch, '.' | '_' | '-') {
                        ch
                    } else {
                        '-'
                    }
                })
                .collect::<String>();
            let quarantine_dir = Path::new(&options.store_dir)
                .join("quarantine")
                .join(format!("{safe_snapshot_name}-{}", now_millis()));
            for entry in apply_entries {
                let relative_path = entry.path.strip_prefix("content/").unwrap_or(&entry.path);
                let quarantine_path = quarantine_dir.join(
                    relative_path.replace(HOME_TOKEN, "home"),
                );
                if let Some(parent) = quarantine_path.parent() {
                    fs::create_dir_all(parent)?;
                    #[cfg(unix)]
                    {
                        use std::os::unix::fs::PermissionsExt;
                        fs::set_permissions(parent, fs::Permissions::from_mode(0o700))?;
                    }
                }
                fs::write(&quarantine_path, &entry.content)?;
            }
            warnings.push(format!(
                "Content files quarantined for inspection at {}; no target files were modified.",
                quarantine_dir.display()
            ));
            quarantined_content_dir = Some(quarantine_dir.to_string_lossy().to_string());
        } else {
            let roots = ConfinementRoots {
                home_dir: home_dir.to_path_buf(),
                project_path: project_path.to_path_buf(),
            };
            for entry in apply_entries {
                let relative_path = entry.path.strip_prefix("content/").unwrap_or(&entry.path);
                let resolved = resolve_bundle_path(relative_path, home_dir, project_path);
                validate_constrained_write_path(&resolved, &roots).map_err(BundleError::Message)?;
                if let Some(parent) = resolved.parent() {
                    fs::create_dir_all(parent)?;
                }
                let content = String::from_utf8(entry.content.clone())
                    .map_err(|error| BundleError::Message(error.to_string()))?;
                write_file_atomically(&resolved, &content)
                    .map_err(|error| BundleError::Message(error.to_string()))?;
            }
            content_applied = true;
        }
    }

    Ok(BundleImportResult {
        snapshot_name: manifest.snapshot_name,
        evidence_count: imported_evidence.len(),
        includes_content: manifest.includes_content,
        content_applied,
        quarantined_content_dir,
        warnings,
        machine_diff: Some(machine_diff),
        readiness,
    })
}

pub fn bundle_inspect(bundle_path: &str) -> BundleResult<BundleInspectResult> {
    let path = Path::new(bundle_path);
    let (entries, bundle_checksum) = read_tar(path)?;

    let manifest_entry = entries
        .iter()
        .find(|entry| entry.path == ".hem/manifest.json")
        .ok_or_else(|| BundleError::Message("Invalid bundle: missing .hem/manifest.json".into()))?;
    let manifest: BundleManifest = serde_json::from_slice(&manifest_entry.content)?;

    let checksums = entries
        .iter()
        .find(|entry| entry.path == ".hem/checksums.json")
        .map(|entry| serde_json::from_slice::<BundleChecksums>(&entry.content))
        .transpose()?;

    Ok(BundleInspectResult {
        bundle_path: bundle_path.to_string(),
        format_version: manifest.format_version,
        snapshot_name: manifest.snapshot_name,
        created_at: manifest.created_at,
        project_path: manifest.project_path,
        includes_content: manifest.includes_content,
        content_file_count: manifest.content_file_count,
        content_total_bytes: manifest.content_total_bytes,
        checksum_algorithm: checksums
            .as_ref()
            .map(|value| value.algorithm.clone())
            .unwrap_or_else(|| "SHA-256".to_string()),
        bundle_checksum,
        is_signed: manifest.security.signed,
        signature_algorithm: manifest.security.signature_algorithm,
        source_machine: manifest.source_machine,
    })
}

pub fn bundle_verify(options: &BundleVerifyOptions) -> BundleResult<BundleVerifyResult> {
    let (entries, _) = read_tar(Path::new(&options.bundle_path))?;
    let signature_key = resolve_signature_key(options.signature_key.as_deref());
    let mut warnings = Vec::new();
    let mut errors = Vec::new();
    let mut format_version = None;

    if let Some(format_entry) = entries.iter().find(|entry| entry.path == ".hem/format-version") {
        let version = String::from_utf8_lossy(&format_entry.content)
            .trim()
            .to_string();
        if version != FORMAT_VERSION {
            errors.push(format!(
                "Unsupported bundle format version: \"{version}\" (expected \"{FORMAT_VERSION}\")"
            ));
        } else {
            format_version = version.parse().ok();
        }
    } else {
        errors.push("Invalid bundle: missing .hem/format-version".to_string());
    }

    let manifest_entry = entries
        .iter()
        .find(|entry| entry.path == ".hem/manifest.json");
    let Some(manifest_entry) = manifest_entry else {
        errors.push("Invalid bundle: missing .hem/manifest.json".to_string());
        return Ok(BundleVerifyResult {
            bundle_path: options.bundle_path.clone(),
            valid: false,
            format_version,
            snapshot_name: None,
            checksums: BundleVerifyChecksumResult {
                checked: false,
                ok: false,
                entries_checked: 0,
            },
            signature: BundleVerifySignatureResult {
                signed: false,
                checked: false,
                ok: true,
                algorithm: None,
            },
            warnings,
            errors,
        });
    };

    let manifest: BundleManifest = match serde_json::from_slice(&manifest_entry.content) {
        Ok(manifest) => manifest,
        Err(error) => {
            errors.push(format!("Invalid bundle manifest JSON: {error}"));
            return Ok(BundleVerifyResult {
                bundle_path: options.bundle_path.clone(),
                valid: false,
                format_version,
                snapshot_name: None,
                checksums: BundleVerifyChecksumResult {
                    checked: false,
                    ok: false,
                    entries_checked: 0,
                },
                signature: BundleVerifySignatureResult {
                    signed: false,
                    checked: false,
                    ok: true,
                    algorithm: None,
                },
                warnings,
                errors,
            });
        }
    };

    let signature_verification =
        verify_bundle_signature(&entries, &manifest, signature_key.as_deref());
    let signature = BundleVerifySignatureResult {
        signed: manifest.security.signed,
        checked: signature_verification.checked,
        ok: signature_verification.ok,
        algorithm: manifest.security.signature_algorithm.clone(),
    };
    if let Some(warning) = signature_verification.warning {
        if signature_verification.ok {
            warnings.push(warning);
        } else {
            errors.push(warning);
        }
    }

    let mut checksum_result = BundleVerifyChecksumResult {
        checked: false,
        ok: false,
        entries_checked: 0,
    };
    if let Some(checksums_entry) = entries.iter().find(|entry| entry.path == ".hem/checksums.json")
    {
        checksum_result.checked = true;
        let checksums: BundleChecksums = serde_json::from_slice(&checksums_entry.content)?;
        for entry in &entries {
            if entry.path == ".hem/checksums.json" {
                continue;
            }
            let Some(expected) = checksums.entries.get(&entry.path) else {
                continue;
            };
            checksum_result.entries_checked += 1;
            let actual = sha256_hex(&entry.content);
            if &actual != expected {
                errors.push(format!(
                    "Checksum mismatch for \"{}\": expected {expected}, got {actual}",
                    entry.path
                ));
            }
        }
        checksum_result.ok = !errors.iter().any(|error| error.starts_with("Checksum mismatch"));
    } else {
        errors.push("Invalid bundle: missing .hem/checksums.json".to_string());
    }

    Ok(BundleVerifyResult {
        bundle_path: options.bundle_path.clone(),
        valid: errors.is_empty(),
        format_version,
        snapshot_name: Some(manifest.snapshot_name),
        checksums: checksum_result,
        signature,
        warnings,
        errors,
    })
}

struct SignatureVerification {
    ok: bool,
    checked: bool,
    warning: Option<String>,
}

fn resolve_signature_key(explicit_key: Option<&str>) -> Option<String> {
    explicit_key
        .map(str::to_string)
        .or_else(|| std::env::var("HEM_BUNDLE_KEY").ok())
}

fn clone_manifest_without_signature(manifest: &BundleManifest) -> BundleManifest {
    let mut cloned = manifest.clone();
    cloned.security.signature = None;
    cloned
}

fn canonical_signature_payload(entries: &[TarEntry], manifest: &BundleManifest) -> Vec<u8> {
    let mut hmac_entries: Vec<_> = entries
        .iter()
        .filter(|entry| {
            entry.path != ".hem/manifest.json" && entry.path != ".hem/checksums.json"
        })
        .filter(|entry| entry.entry_type == TarEntryType::File)
        .collect();
    hmac_entries.sort_by(|left, right| left.path.cmp(&right.path));

    let mut payload =
        serde_json::to_vec(&clone_manifest_without_signature(manifest)).unwrap_or_default();
    payload.push(b'\n');

    for entry in hmac_entries {
        payload.extend_from_slice(format!("{}\n{}\n", entry.path, entry.content.len()).as_bytes());
        payload.extend_from_slice(&entry.content);
        payload.push(b'\n');
    }

    payload
}

fn sign_bundle_entries(
    entries: &[TarEntry],
    manifest: &BundleManifest,
    key: &str,
) -> BundleResult<String> {
    let mut mac = HmacSha256::new_from_slice(key.as_bytes())?;
    mac.update(&canonical_signature_payload(entries, manifest));
    Ok(mac
        .finalize()
        .into_bytes()
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect())
}

fn verify_bundle_signature(
    entries: &[TarEntry],
    manifest: &BundleManifest,
    key: Option<&str>,
) -> SignatureVerification {
    if !manifest.security.signed {
        return SignatureVerification {
            ok: true,
            checked: false,
            warning: None,
        };
    }
    let Some(key) = key else {
        return SignatureVerification {
            ok: false,
            checked: false,
            warning: Some(
                "Bundle is signed, but no signature key was provided; signature was not verified."
                    .to_string(),
            ),
        };
    };
    let Some(expected) = manifest.security.signature.as_deref() else {
        return SignatureVerification {
            ok: false,
            checked: true,
            warning: Some(
                "Signed bundle manifest is missing security.signature.".to_string(),
            ),
        };
    };
    let actual = sign_bundle_entries(entries, manifest, key).unwrap_or_default();
    SignatureVerification {
        ok: actual == expected,
        checked: true,
        warning: if actual == expected {
            None
        } else {
            Some("Bundle signature verification failed.".to_string())
        },
    }
}

fn key_fingerprint(key: &str) -> String {
    sha256_hex(key.as_bytes())
}

fn trusted_key_path(store_dir: &Path) -> PathBuf {
    store_dir.join("trust").join("bundle-signing-key.json")
}

fn enforce_bundle_key_trust(
    store_dir: &Path,
    key: &str,
    trust: bool,
) -> BundleResult<Option<String>> {
    let fingerprint = key_fingerprint(key);
    let file_path = trusted_key_path(store_dir);
    let trusted: Option<serde_json::Value> = fs::read_to_string(&file_path)
        .ok()
        .and_then(|text| serde_json::from_str(&text).ok());

    if trusted
        .as_ref()
        .and_then(|value| value.get("fingerprint"))
        .and_then(Value::as_str)
        .is_none()
    {
        if !trust {
            return Ok(Some(
                "No trusted bundle signing key is recorded. Re-run with --trust after verifying the source.".to_string(),
            ));
        }
        if let Some(parent) = file_path.parent() {
            fs::create_dir_all(parent)?;
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                fs::set_permissions(parent, fs::Permissions::from_mode(0o700))?;
            }
        }
        let payload = serde_json::json!({
            "fingerprint": fingerprint,
            "trustedAt": chrono_like_now(),
        });
        fs::write(&file_path, format!("{}\n", serde_json::to_string_pretty(&payload)?))?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            fs::set_permissions(&file_path, fs::Permissions::from_mode(0o600))?;
        }
        return Ok(Some(format!(
            "Trusted bundle signing key fingerprint {}…",
            &fingerprint[..12.min(fingerprint.len())]
        )));
    }

    let stored = trusted
        .as_ref()
        .and_then(|value| value.get("fingerprint"))
        .and_then(Value::as_str)
        .unwrap_or_default();
    if stored != fingerprint {
        return Err(BundleError::Message(format!(
            "Bundle signing key fingerprint {}… does not match trusted key fingerprint {}…",
            &fingerprint[..12.min(fingerprint.len())],
            &stored[..12.min(stored.len())]
        )));
    }

    Ok(None)
}

fn normalise_home_for_platform(home_dir: &str, machine_platform: &str) -> String {
    if machine_platform == "linux" {
        if let Some(username) = home_dir
            .strip_prefix("/Users/")
            .and_then(|rest| rest.trim_end_matches('/').split('/').next())
        {
            return format!("/home/{username}");
        }
    }
    if machine_platform == "darwin" {
        if let Some(username) = home_dir
            .strip_prefix("/home/")
            .and_then(|rest| rest.trim_end_matches('/').split('/').next())
        {
            return format!("/Users/{username}");
        }
    }
    home_dir.to_string()
}

fn normalise_source_path(source_path: &str, home_dir: &str) -> String {
    if let Some(rest) = source_path.strip_prefix("~/") {
        return format!("{HOME_TOKEN}/{rest}");
    }

    let resolved_source = resolve_path_string(source_path);
    let resolved_home = resolve_path_string(home_dir);
    if Path::new(source_path).is_absolute()
        && (resolved_source == resolved_home
            || resolved_source.starts_with(&format!(
                "{}{}",
                resolved_home,
                std::path::MAIN_SEPARATOR
            )))
    {
        let home_relative = path_relative(&resolved_home, &resolved_source);
        if home_relative.is_empty() {
            return HOME_TOKEN.to_string();
        }
        return format!("{HOME_TOKEN}/{home_relative}");
    }

    source_path.to_string()
}

fn resolve_bundle_path(normalised_path: &str, home_dir: &Path, project_path: &Path) -> PathBuf {
    if let Some(rest) = normalised_path.strip_prefix(&format!("{HOME_TOKEN}/")) {
        return home_dir.join(rest);
    }
    project_path.join(normalised_path)
}

fn normalise_snapshot_paths_for_bundle<T>(items: &[T], home_dir: &str) -> Vec<T>
where
    T: Clone + SourcePathAccessor,
{
    items
        .iter()
        .map(|item| {
            let mut cloned = item.clone();
            cloned.set_source_path(normalise_source_path(item.source_path(), home_dir));
            cloned
        })
        .collect()
}

fn resolve_snapshot_path_for_import(source_path: &str, home_dir: &Path) -> Result<String, BundleError> {
    if let Some(rest) = source_path.strip_prefix(&format!("{HOME_TOKEN}/")) {
        validate_home_relative_import_segment(rest).map_err(BundleError::Message)?;
        return Ok(home_dir.join(rest).to_string_lossy().to_string());
    }
    if source_path.contains("..") {
        return Err(BundleError::Message(format!(
            "Path traversal detected: \"{source_path}\" contains \"..\""
        )));
    }
    Ok(source_path.to_string())
}

fn resolve_snapshot_paths_for_import<T>(
    items: Vec<T>,
    home_dir: &Path,
) -> Result<Vec<T>, BundleError>
where
    T: SourcePathAccessor,
{
    items
        .into_iter()
        .map(|mut item| {
            let resolved = resolve_snapshot_path_for_import(item.source_path(), home_dir)?;
            item.set_source_path(resolved);
            Ok(item)
        })
        .collect()
}

trait SourcePathAccessor: Clone {
    fn source_path(&self) -> &str;
    fn set_source_path(&mut self, path: String);
}

impl SourcePathAccessor for DiscoveredItem {
    fn source_path(&self) -> &str {
        &self.source_path
    }
    fn set_source_path(&mut self, path: String) {
        self.source_path = path;
    }
}

impl SourcePathAccessor for GraphNode {
    fn source_path(&self) -> &str {
        &self.source_path
    }
    fn set_source_path(&mut self, path: String) {
        self.source_path = path;
    }
}

impl SourcePathAccessor for ProvenanceEntry {
    fn source_path(&self) -> &str {
        &self.source_path
    }
    fn set_source_path(&mut self, path: String) {
        self.source_path = path;
    }
}

fn capture_source_machine() -> SourceMachine {
    SourceMachine {
        home_dir: std::env::var("HOME").unwrap_or_default(),
        platform: current_platform().to_string(),
        hostname: hostname(),
    }
}

fn resolve_source_path(source_path: &str, home_dir: &str, project_path: &str) -> PathBuf {
    if let Some(rest) = source_path.strip_prefix("~/") {
        return Path::new(home_dir).join(rest);
    }
    if Path::new(source_path).is_absolute() {
        return PathBuf::from(source_path);
    }
    Path::new(project_path).join(source_path)
}

fn is_strictly_under(resolved: &Path, root: &Path) -> bool {
    let root_str = root.to_string_lossy();
    let resolved_str = resolved.to_string_lossy();
    resolved_str == root_str
        || resolved_str.starts_with(&format!(
            "{root_str}{}",
            std::path::MAIN_SEPARATOR
        ))
}

fn compute_entry_checksums(entries: &[TarEntry]) -> HashMap<String, String> {
    entries
        .iter()
        .map(|entry| (entry.path.clone(), sha256_hex(&entry.content)))
        .collect()
}

fn sha256_hex(data: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(data);
    hasher
        .finalize()
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

fn json_pretty_line<T: serde::Serialize>(value: &T) -> BundleResult<Vec<u8>> {
    Ok(format!("{}\n", serde_json::to_string_pretty(value)?).into_bytes())
}

fn update_manifest_entry(entries: &mut [TarEntry], manifest: &BundleManifest) -> BundleResult<()> {
    if let Some(index) = entries
        .iter()
        .position(|entry| entry.path == ".hem/manifest.json")
    {
        entries[index].content = json_pretty_line(manifest)?;
    }
    Ok(())
}

fn dir_entry(path: &str, mtime: u64) -> TarEntry {
    TarEntry {
        path: path.to_string(),
        content: Vec::new(),
        mode: 0o755,
        mtime,
        entry_type: TarEntryType::Directory,
    }
}

fn file_entry(path: &str, content: Vec<u8>, mtime: u64) -> TarEntry {
    TarEntry {
        path: path.to_string(),
        content,
        mode: 0o644,
        mtime,
        entry_type: TarEntryType::File,
    }
}

fn now_millis() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .unwrap_or(0)
}

fn hostname() -> String {
    std::env::var("HOSTNAME")
        .or_else(|_| std::env::var("COMPUTERNAME"))
        .unwrap_or_else(|_| "unknown".to_string())
}

fn resolve_path_string(path: &str) -> String {
    Path::new(path)
        .canonicalize()
        .unwrap_or_else(|_| PathBuf::from(path))
        .to_string_lossy()
        .to_string()
}

fn pathdiff(base: &Path, target: &Path) -> Option<PathBuf> {
    let mut base_components = base.components().collect::<Vec<_>>();
    let mut target_components = target.components().collect::<Vec<_>>();
    while !base_components.is_empty()
        && !target_components.is_empty()
        && base_components[0] == target_components[0]
    {
        base_components.remove(0);
        target_components.remove(0);
    }
    let mut result = PathBuf::new();
    for _ in base_components {
        result.push("..");
    }
    for component in target_components {
        result.push(component.as_os_str());
    }
    Some(result)
}

fn path_relative(base: &str, target: &str) -> String {
    let relative = pathdiff(Path::new(base), Path::new(target))
        .map(|path| path.to_string_lossy().replace('\\', "/"))
        .unwrap_or_else(|| target.to_string());
    if relative == "." {
        String::new()
    } else {
        relative
    }
}

fn chrono_like_now() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00.000Z".to_string())
}

use serde_json::Value;