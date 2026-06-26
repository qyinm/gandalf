use std::collections::HashMap;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};
use serde_json::Value;
use sha2::{Digest, Sha256};
use uuid::Uuid;

use crate::types::{
    AgentId, AuditFinding, DiscoveredItem, Severity, Snapshot, SnapshotContentEntry,
    SnapshotManifest, TimelineEntry,
};

const CONTENT_DIR: &str = "content";
const TIMELINE_EVENTS_DIR: &str = "timeline/events";

const AGENT_STORE_DIRS: &[AgentId] = &[
    AgentId::ClaudeCode,
    AgentId::Codex,
    AgentId::Cursor,
    AgentId::Opencode,
    AgentId::PiAgent,
    AgentId::Project,
    AgentId::Unknown,
];

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ChecksumRecord {
    source_path: String,
    checksum: String,
}

type ChecksumMap = HashMap<String, ChecksumRecord>;

#[derive(Debug, Clone)]
pub struct StoreSnapshot {
    pub manifest: SnapshotManifest,
    pub evidence: Vec<DiscoveredItem>,
    pub graph: Vec<crate::types::GraphNode>,
    pub audit_findings: Vec<AuditFinding>,
    pub provenance: Vec<crate::types::ProvenanceEntry>,
    pub content: Option<Vec<SnapshotContentEntry>>,
    pub checksums: Option<ChecksumMap>,
    pub redactions: Option<Vec<Value>>,
}

impl From<Snapshot> for StoreSnapshot {
    fn from(snapshot: Snapshot) -> Self {
        let Snapshot {
            manifest,
            evidence,
            graph,
            audit_findings,
            provenance,
            content,
        } = snapshot;
        Self {
            manifest,
            evidence,
            graph,
            audit_findings,
            provenance,
            content,
            checksums: None,
            redactions: None,
        }
    }
}

pub struct TimelineListOptions<'a> {
    pub agent: Option<AgentId>,
    pub project_path: Option<&'a str>,
    pub limit: Option<usize>,
    pub on_corrupt_entry: Option<&'a mut dyn FnMut(TimelineCorruptEvent)>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TimelineCorruptEvent {
    pub file_path: PathBuf,
    pub error: String,
}

#[derive(Debug, thiserror::Error)]
pub enum StoreError {
    #[error("Unsafe snapshot name: {0:?}")]
    UnsafeSnapshotName(String),
    #[error("Unsafe snapshot content path: {0:?}")]
    UnsafeContentPath(String),
    #[error(transparent)]
    Io(#[from] io::Error),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
}

pub fn default_store_dir(home_dir: &Path) -> PathBuf {
    home_dir.join(".gandalf")
}

pub fn agent_store_dir(store_dir: &Path, agent: Option<AgentId>) -> PathBuf {
    match agent {
        Some(agent) => store_dir.join(agent.as_str()),
        None => store_dir.to_path_buf(),
    }
}

fn snapshot_dir(store_dir: &Path, name: &str, agent: Option<AgentId>) -> PathBuf {
    agent_store_dir(store_dir, agent).join(name)
}

pub fn ensure_store(store_dir: &Path) -> Result<Vec<AuditFinding>, StoreError> {
    let existed = path_exists(store_dir)?;
    fs::create_dir_all(store_dir)?;
    if !existed {
        set_mode(store_dir, 0o700)?;
    }

    let mode = file_mode(store_dir)?;
    if mode & 0o022 == 0 {
        return Ok(Vec::new());
    }

    Ok(vec![AuditFinding {
        code: "WORLD_WRITABLE_STORE".to_string(),
        severity: Severity::High,
        problem: "The local gandalf snapshot store is writable by group or world.".to_string(),
        cause: format!("Store permissions are {mode:o}."),
        fix: "Restrict the store directory to the current user with chmod 700.".to_string(),
        path: Some(store_dir.to_string_lossy().to_string()),
        evidence_id: None,
    }])
}

pub fn list_agents(store_dir: &Path) -> Result<Vec<AgentId>, StoreError> {
    if !path_exists(store_dir)? {
        return Ok(Vec::new());
    }

    let mut agents = Vec::new();
    for entry in fs::read_dir(store_dir)? {
        let entry = entry?;
        if !entry.file_type()?.is_dir() {
            continue;
        }
        let name = entry.file_name().to_string_lossy().to_string();
        if !is_safe_agent_name(&name) {
            continue;
        }
        let agent = AgentId::from_str(&name);
        if !AGENT_STORE_DIRS.contains(&agent) {
            continue;
        }
        let sub = fs::read_dir(entry.path())?;
        if sub.filter_map(Result::ok).any(|e| e.file_type().map(|t| t.is_dir()).unwrap_or(false))
        {
            agents.push(agent);
        }
    }
    agents.sort_by_key(|a| a.as_str().to_string());
    Ok(agents)
}

pub fn write_snapshot(
    store_dir: &Path,
    snapshot: StoreSnapshot,
    agent: Option<AgentId>,
) -> Result<(), StoreError> {
    let name = validate_snapshot_name(&snapshot.manifest.name)?;
    let dir = snapshot_dir(store_dir, &name, agent);

    ensure_store(store_dir)?;
    if agent.is_some() {
        ensure_store(&agent_store_dir(store_dir, agent))?;
    }
    fs::create_dir_all(&dir)?;
    set_mode(&dir, 0o700)?;

    let checksums = snapshot
        .checksums
        .clone()
        .unwrap_or_else(|| checksums_from_evidence(&snapshot.evidence));
    let redactions = snapshot.redactions.clone().unwrap_or_default();

    write_json_atomic(&dir.join("manifest.json"), &snapshot.manifest)?;
    write_json_atomic(&dir.join("evidence.json"), &snapshot.evidence)?;
    write_json_atomic(&dir.join("graph.json"), &snapshot.graph)?;
    write_json_atomic(&dir.join("audit-findings.json"), &snapshot.audit_findings)?;
    write_json_atomic(&dir.join("provenance.json"), &snapshot.provenance)?;
    write_json_atomic(&dir.join("checksums.json"), &checksums)?;
    write_json_atomic(&dir.join("redactions.json"), &redactions)?;

    if let Some(content) = &snapshot.content {
        if !content.is_empty() {
            let content_dir = dir.join(CONTENT_DIR);
            if content_dir.exists() {
                fs::remove_dir_all(&content_dir)?;
            }
            fs::create_dir_all(&content_dir)?;
            set_mode(&content_dir, 0o700)?;

            for entry in content {
                if entry.capture_status != "captured" {
                    continue;
                }
                let Some(text) = &entry.content else {
                    continue;
                };
                if !is_safe_snapshot_relative_path(&entry.storage_path)
                    || !entry.storage_path.starts_with(&format!("{CONTENT_DIR}/"))
                {
                    return Err(StoreError::UnsafeContentPath(entry.storage_path.clone()));
                }
                let content_path = dir.join(&entry.storage_path);
                if let Some(parent) = content_path.parent() {
                    fs::create_dir_all(parent)?;
                }
                write_text_atomic(&content_path, text)?;
            }

            let index: Vec<SnapshotContentEntry> = content
                .iter()
                .map(|entry| SnapshotContentEntry {
                    content: None,
                    ..entry.clone()
                })
                .collect();
            write_json_atomic(&dir.join("content-index.json"), &index)?;
        }
    }

    Ok(())
}

pub fn read_snapshot(
    store_dir: &Path,
    name: &str,
    agent: Option<AgentId>,
) -> Result<Snapshot, StoreError> {
    let safe_name = validate_snapshot_name(name)?;
    let dir = snapshot_dir(store_dir, &safe_name, agent);

    let manifest: SnapshotManifest = read_json(&dir.join("manifest.json"))?;
    let evidence: Vec<DiscoveredItem> = read_json(&dir.join("evidence.json"))?;
    let graph = read_json(&dir.join("graph.json"))?;
    let audit_findings = read_json(&dir.join("audit-findings.json"))?;
    let provenance = read_json(&dir.join("provenance.json"))?;
    let content = read_optional_json(&dir.join("content-index.json"))?;

    Ok(Snapshot {
        manifest,
        evidence,
        graph,
        audit_findings,
        provenance,
        content,
    })
}

pub fn read_snapshot_content(
    store_dir: &Path,
    name: &str,
    entry: &SnapshotContentEntry,
    agent: Option<AgentId>,
) -> Result<String, StoreError> {
    let safe_name = validate_snapshot_name(name)?;
    if !is_safe_snapshot_relative_path(&entry.storage_path)
        || !entry.storage_path.starts_with(&format!("{CONTENT_DIR}/"))
    {
        return Err(StoreError::UnsafeContentPath(entry.storage_path.clone()));
    }
    let path = snapshot_dir(store_dir, &safe_name, agent).join(&entry.storage_path);
    Ok(fs::read_to_string(path)?)
}

pub fn list_snapshots(store_dir: &Path, agent: Option<AgentId>) -> Result<Vec<String>, StoreError> {
    let base_dir = agent_store_dir(store_dir, agent);
    if !path_exists(&base_dir)? {
        return Ok(Vec::new());
    }

    let mut names = Vec::new();
    for entry in fs::read_dir(&base_dir)? {
        let entry = entry?;
        if !entry.file_type()?.is_dir() {
            continue;
        }
        let name = entry.file_name().to_string_lossy().to_string();
        if !is_safe_snapshot_name(&name) {
            continue;
        }
        if agent.is_some() {
            names.push(name);
        } else if base_dir.join(&name).join("manifest.json").exists() {
            names.push(name);
        }
    }
    names.sort();
    Ok(names)
}

pub fn snapshot_exists(store_dir: &Path, name: &str, agent: Option<AgentId>) -> Result<bool, StoreError> {
    let safe_name = validate_snapshot_name(name)?;
    Ok(snapshot_dir(store_dir, &safe_name, agent)
        .join("manifest.json")
        .exists())
}

pub fn append_timeline_entry(store_dir: &Path, entry: &TimelineEntry) -> Result<(), StoreError> {
    validate_snapshot_name(&entry.after_snapshot_name)?;
    if let Some(before) = &entry.before_snapshot_name {
        validate_snapshot_name(before)?;
    }
    ensure_store(store_dir)?;
    let dir = store_dir.join(TIMELINE_EVENTS_DIR);
    fs::create_dir_all(&dir)?;
    set_mode(&dir, 0o700)?;
    write_json_atomic(&timeline_entry_path(store_dir, entry), entry)?;
    Ok(())
}

pub fn list_timeline_entries(
    store_dir: &Path,
    mut options: TimelineListOptions<'_>,
) -> Result<Vec<TimelineEntry>, StoreError> {
    let dir = store_dir.join(TIMELINE_EVENTS_DIR);
    if !path_exists(&dir)? {
        return Ok(Vec::new());
    }

    let mut entries = read_timeline_entries(&dir, options.on_corrupt_entry)?;
    if let Some(project_path) = options.project_path {
        let project_path = resolve_path_str(project_path);
        entries.retain(|entry| resolve_path_str(&entry.project_path) == project_path);
    }
    if let Some(agent) = options.agent {
        entries.retain(|entry| {
            entry.agent == Some(agent) || entry.agents.contains(&agent)
        });
    }
    entries.sort_by(|a, b| b.observed_at.cmp(&a.observed_at));
    if let Some(limit) = options.limit {
        entries.truncate(limit);
    }
    Ok(entries)
}

pub fn latest_timeline_entry(
    store_dir: &Path,
    options: TimelineListOptions<'_>,
) -> Result<Option<TimelineEntry>, StoreError> {
    let mut opts = options;
    opts.limit = Some(1);
    Ok(list_timeline_entries(store_dir, opts)?.into_iter().next())
}

pub fn find_timeline_entry(
    store_dir: &Path,
    reference: &str,
    mut options: TimelineListOptions<'_>,
) -> Result<Option<TimelineEntry>, StoreError> {
    options.limit = None;
    Ok(list_timeline_entries(store_dir, options)?
        .into_iter()
        .find(|entry| entry.id == reference || entry.after_snapshot_name == reference))
}

pub fn state_hash(snapshot: &Snapshot) -> String {
    let payload = serde_json::json!({
        "evidence": snapshot.evidence,
        "graph": snapshot.graph,
        "auditFindings": snapshot.audit_findings,
        "provenance": snapshot.provenance,
    });
    let digest = Sha256::digest(serde_json::to_string(&payload).unwrap_or_default());
    format!("sha256:{:x}", digest)
}

fn validate_snapshot_name(name: &str) -> Result<String, StoreError> {
    if !is_safe_snapshot_name(name) {
        return Err(StoreError::UnsafeSnapshotName(name.to_string()));
    }
    Ok(name.to_string())
}

fn is_safe_snapshot_name(name: &str) -> bool {
    !name.trim().is_empty()
        && !name.contains("..")
        && !name.contains('/')
        && !name.contains('\\')
}

fn is_safe_agent_name(name: &str) -> bool {
    regex::Regex::new(r"^[a-z][a-z0-9_-]*$")
        .expect("regex")
        .is_match(name)
        && !name.contains("..")
        && !name.contains('/')
}

fn is_safe_snapshot_relative_path(name: &str) -> bool {
    !name.trim().is_empty()
        && !Path::new(name).is_absolute()
        && !name.contains('\\')
        && !name.split('/').any(|part| part == "..")
}

fn write_json_atomic<T: Serialize>(file_path: &Path, value: &T) -> Result<(), StoreError> {
    let serialized = format!("{}\n", serde_json::to_string_pretty(value)?);
    write_text_atomic(file_path, &serialized)
}

fn write_text_atomic(file_path: &Path, value: &str) -> Result<(), StoreError> {
    let temp_path = PathBuf::from(format!(
        "{}.{}.{}.tmp",
        file_path.display(),
        std::process::id(),
        Uuid::new_v4()
    ));
    if let Err(error) = (|| {
        fs::write(&temp_path, value)?;
        fs::rename(&temp_path, file_path)?;
        Ok::<(), io::Error>(())
    })() {
        let _ = fs::remove_file(&temp_path);
        return Err(error.into());
    }
    Ok(())
}

fn read_json<T: for<'de> Deserialize<'de>>(file_path: &Path) -> Result<T, StoreError> {
    Ok(serde_json::from_str(&fs::read_to_string(file_path)?)?)
}

fn read_optional_json<T: for<'de> Deserialize<'de>>(
    file_path: &Path,
) -> Result<Option<T>, StoreError> {
    match fs::read_to_string(file_path) {
        Ok(text) => Ok(Some(serde_json::from_str(&text)?)),
        Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(None),
        Err(error) => Err(error.into()),
    }
}

fn timeline_entry_path(store_dir: &Path, entry: &TimelineEntry) -> PathBuf {
    let observed = entry
        .observed_at
        .replace([':', '.'], "-");
    store_dir
        .join(TIMELINE_EVENTS_DIR)
        .join(format!("{observed}-{}.json", entry.id))
}

fn read_timeline_entries(
    dir: &Path,
    mut on_corrupt_entry: Option<&mut dyn FnMut(TimelineCorruptEvent)>,
) -> Result<Vec<TimelineEntry>, StoreError> {
    let mut entries = Vec::new();
    for entry in fs::read_dir(dir)? {
        let entry = entry?;
        if !entry.file_type()?.is_file() {
            continue;
        }
        let name = entry.file_name().to_string_lossy().to_string();
        if !name.ends_with(".json") {
            continue;
        }
        let path = entry.path();
        match read_json::<Value>(&path) {
            Ok(raw) => match normalize_timeline_entry(raw) {
                Ok(normalized) => entries.push(normalized),
            Err(error) => {
                if let Some(handler) = on_corrupt_entry.as_deref_mut() {
                    handler(TimelineCorruptEvent {
                        file_path: path,
                        error: format!("invalid timeline JSON: {error}"),
                    });
                }
            }
            },
            Err(error) => {
                if let Some(handler) = on_corrupt_entry.as_deref_mut() {
                    handler(TimelineCorruptEvent {
                        file_path: path,
                        error: format!("invalid timeline JSON: {error}"),
                    });
                }
            }
        }
    }
    Ok(entries)
}

fn normalize_timeline_entry(raw: Value) -> Result<TimelineEntry, StoreError> {
    let record = raw.as_object().ok_or_else(|| {
        StoreError::Io(io::Error::new(
            io::ErrorKind::InvalidData,
            "timeline event is not an object",
        ))
    })?;
    let legacy_daemon_run_id = record
        .get("daemonRunId")
        .and_then(|v| v.as_str())
        .map(str::to_string);
    let capture_id = record
        .get("captureId")
        .and_then(|v| v.as_str())
        .filter(|value| !value.is_empty())
        .map(str::to_string)
        .or(legacy_daemon_run_id)
        .or_else(|| record.get("id").and_then(|v| v.as_str()).map(str::to_string))
        .unwrap_or_else(|| "legacy".to_string());

    let mut normalized = raw;
    if let Some(obj) = normalized.as_object_mut() {
        obj.insert(
            "source".to_string(),
            Value::String("manual".to_string()),
        );
        obj.insert("captureId".to_string(), Value::String(capture_id.clone()));
    }
    let mut entry: TimelineEntry = serde_json::from_value(normalized)?;
    entry.source = crate::types::TimelineEntrySource::Manual;
    entry.capture_id = capture_id;
    Ok(entry)
}

fn checksums_from_evidence(evidence: &[DiscoveredItem]) -> ChecksumMap {
    let mut checksums = HashMap::new();
    for item in evidence {
        if let Some(checksum) = &item.checksum {
            if !checksum.is_empty() {
                checksums.insert(
                    item.id.clone(),
                    ChecksumRecord {
                        source_path: item.source_path.clone(),
                        checksum: checksum.clone(),
                    },
                );
            }
        }
    }
    checksums
}

fn path_exists(target_path: &Path) -> Result<bool, StoreError> {
    match fs::metadata(target_path) {
        Ok(_) => Ok(true),
        Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(false),
        Err(error) => Err(error.into()),
    }
}

fn resolve_path_str(path: &str) -> String {
    let path = PathBuf::from(path);
    fs::canonicalize(&path)
        .unwrap_or(path)
        .to_string_lossy()
        .to_string()
}

#[cfg(unix)]
fn set_mode(path: &Path, mode: u32) -> Result<(), StoreError> {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(mode))?;
    Ok(())
}

#[cfg(not(unix))]
fn set_mode(_path: &Path, _mode: u32) -> Result<(), StoreError> {
    Ok(())
}

#[cfg(unix)]
fn file_mode(path: &Path) -> Result<u32, StoreError> {
    use std::os::unix::fs::PermissionsExt;
    Ok(fs::metadata(path)?.permissions().mode() & 0o777)
}

#[cfg(not(unix))]
fn file_mode(_path: &Path) -> Result<u32, StoreError> {
    Ok(0o700)
}