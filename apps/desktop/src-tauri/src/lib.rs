use hem_core::{
    build_graph, default_store_dir, diff_graphs, list_agents, list_snapshots, list_timeline_entries,
    scan_project,
    types::{AgentId, DiscoveredItem, EvidenceKind, EvidenceParser, ScanOptions},
    TimelineListOptions,
};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashSet;
use std::env;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::Duration;

const DEFAULT_PROFILE_NAME: &str = "default";

#[derive(Clone, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct ProfileSummary {
    name: String,
    scope: String,
    sync_state: String,
    ahead: u32,
    behind: u32,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct ChangelogEntry {
    id: String,
    title: String,
    time: String,
    source: String,
    risk: String,
}

#[derive(Serialize, Clone)]
struct SetupSurface {
    id: String,
    label: String,
    count: u32,
    risk: String,
    description: String,
}

#[derive(Serialize, Clone)]
#[serde(rename_all = "camelCase")]
struct InventoryItem {
    id: String,
    name: String,
    agent: String,
    source_path: String,
    scope: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    status: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    detail: Option<String>,
}

#[derive(Serialize, Default)]
struct DesktopInventory {
    skills: Vec<InventoryItem>,
    mcp: Vec<InventoryItem>,
    hooks: Vec<InventoryItem>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct DesktopHomeState {
    active_profile: Option<ProfileSummary>,
    profiles: Vec<ProfileSummary>,
    current_snapshot_id: Option<String>,
    protection: String,
    highest_risk: Option<String>,
    working_changes: u32,
    changelog: Vec<ChangelogEntry>,
    surfaces: Vec<SetupSurface>,
    inventory: DesktopInventory,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct NotificationPermission {
    granted: bool,
    status: String,
}

#[tauri::command]
fn desktop_home_state() -> DesktopHomeState {
    build_desktop_home_state(&desktop_project_path(), home_dir().as_deref())
}

fn build_desktop_home_state(project_path: &Path, home_dir: Option<&Path>) -> DesktopHomeState {
    let home = home_dir_for_scan(project_path, home_dir);
    let store_dir = default_store_dir(&home);
    let profile_state =
        ensure_profile_state(Some(&home)).unwrap_or_else(|_| default_profile_state());
    let inventory = scan_inventory(project_path, Some(&home));
    let latest_snapshot = latest_snapshot_reference(&store_dir);
    let current_snapshot_id = latest_snapshot.as_ref().map(|(name, _)| name.clone());
    let protection = if current_snapshot_id.is_some() {
        "on".into()
    } else {
        "off".into()
    };

    let (working_changes, highest_risk) =
        working_change_summary(project_path, &home, &store_dir, latest_snapshot.as_ref());
    let changelog = timeline_changelog(&store_dir, project_path);

    DesktopHomeState {
        active_profile: profile_state.active,
        profiles: profile_state.profiles,
        current_snapshot_id,
        protection,
        highest_risk,
        working_changes,
        surfaces: surfaces_for(&inventory),
        changelog,
        inventory,
    }
}

fn latest_snapshot_reference(store_dir: &Path) -> Option<(String, Option<AgentId>)> {
    let mut snapshots = Vec::new();
    for agent in list_agents(store_dir).unwrap_or_default() {
        for name in list_snapshots(store_dir, Some(agent)).unwrap_or_default() {
            snapshots.push((name, Some(agent)));
        }
    }
    for name in list_snapshots(store_dir, None).unwrap_or_default() {
        snapshots.push((name, None));
    }
    snapshots.sort_by(|left, right| left.0.cmp(&right.0));
    snapshots.pop()
}

fn working_change_summary(
    project_path: &Path,
    home_dir: &Path,
    store_dir: &Path,
    latest_snapshot: Option<&(String, Option<AgentId>)>,
) -> (u32, Option<String>) {
    let Some((snapshot_name, agent)) = latest_snapshot else {
        return (0, None);
    };
    let scan = scan_project(&ScanOptions {
        project_path: project_path.display().to_string(),
        home_dir: home_dir.display().to_string(),
        store_dir: store_dir.display().to_string(),
        explain: None,
        agent: None,
        scope: None,
    });
    let current_graph = build_graph(&scan.evidence);
    let snapshot_graph = match hem_core::read_snapshot(store_dir, snapshot_name, *agent) {
        Ok(snapshot) => snapshot.graph,
        Err(_) => return (0, None),
    };
    let diff = diff_graphs(&snapshot_graph, &current_graph);
    let working_changes = diff.semantic_changes.len() as u32;
    let highest_risk = diff
        .semantic_changes
        .iter()
        .map(|change| severity_label(change.severity))
        .max_by_key(severity_rank)
        .flatten();
    (working_changes, highest_risk)
}

fn severity_label(severity: hem_core::Severity) -> Option<String> {
    match severity {
        hem_core::Severity::None => None,
        hem_core::Severity::Low => Some("low".into()),
        hem_core::Severity::Medium => Some("medium".into()),
        hem_core::Severity::High => Some("high".into()),
        hem_core::Severity::Critical => Some("high".into()),
    }
}

fn severity_rank(label: &Option<String>) -> u8 {
    match label.as_deref() {
        Some("high") => 3,
        Some("medium") => 2,
        Some("low") => 1,
        _ => 0,
    }
}

fn timeline_changelog(store_dir: &Path, project_path: &Path) -> Vec<ChangelogEntry> {
    let project_path_string = project_path.display().to_string();
    let entries = list_timeline_entries(
        store_dir,
        TimelineListOptions {
            agent: None,
            project_path: Some(project_path_string.as_str()),
            limit: Some(10),
            on_corrupt_entry: None,
        },
    )
    .unwrap_or_default();

    entries
        .into_iter()
        .map(|entry| ChangelogEntry {
            id: entry.id,
            title: entry.title,
            time: entry.created_at,
            source: timeline_source_label(entry.source),
            risk: timeline_risk_label(entry.restore_readiness),
        })
        .collect()
}

fn timeline_source_label(source: hem_core::TimelineEntrySource) -> String {
    match source {
        hem_core::TimelineEntrySource::Manual => "manual".into(),
    }
}

fn timeline_risk_label(readiness: hem_core::TimelineRestoreReadiness) -> String {
    match readiness {
        hem_core::TimelineRestoreReadiness::Full => "low".into(),
        hem_core::TimelineRestoreReadiness::Partial => "medium".into(),
        hem_core::TimelineRestoreReadiness::ObserveOnly => "high".into(),
    }
}

struct ProfileState {
    active: Option<ProfileSummary>,
    profiles: Vec<ProfileSummary>,
}

fn ensure_profile_state(home_dir: Option<&Path>) -> std::io::Result<ProfileState> {
    let Some(home_dir) = home_dir else {
        return Ok(default_profile_state());
    };

    let store_dir = home_dir.join(".hem");
    let profiles_dir = store_dir.join("profiles");
    let default_profile_dir = profiles_dir.join(DEFAULT_PROFILE_NAME);
    let current_profile_path = store_dir.join("current-profile");
    let default_profile_path = default_profile_dir.join("profile.json");

    fs::create_dir_all(&default_profile_dir)?;
    if !default_profile_path.exists() {
        write_profile(&default_profile_path, &default_profile())?;
    }
    if !current_profile_path.exists() {
        fs::write(&current_profile_path, format!("{DEFAULT_PROFILE_NAME}\n"))?;
    }

    let active_name = fs::read_to_string(&current_profile_path)
        .ok()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| DEFAULT_PROFILE_NAME.to_string());

    let mut profiles = read_profiles(&profiles_dir)?;
    if !profiles.iter().any(|profile| profile.name == DEFAULT_PROFILE_NAME) {
        profiles.push(default_profile());
    }
    profiles.sort_by(|left, right| match (left.name == active_name, right.name == active_name) {
        (true, false) => std::cmp::Ordering::Less,
        (false, true) => std::cmp::Ordering::Greater,
        _ => left.name.cmp(&right.name),
    });

    let active = profiles
        .iter()
        .find(|profile| profile.name == active_name)
        .cloned()
        .or_else(|| profiles.first().cloned());

    Ok(ProfileState { active, profiles })
}

fn default_profile_state() -> ProfileState {
    let profile = default_profile();
    ProfileState {
        active: Some(profile.clone()),
        profiles: vec![profile],
    }
}

fn default_profile() -> ProfileSummary {
    ProfileSummary {
        name: DEFAULT_PROFILE_NAME.into(),
        scope: "personal".into(),
        sync_state: "local_only".into(),
        ahead: 0,
        behind: 0,
    }
}

fn write_profile(path: &Path, profile: &ProfileSummary) -> std::io::Result<()> {
    let payload = serde_json::to_vec_pretty(profile)
        .map_err(|error| std::io::Error::new(std::io::ErrorKind::InvalidData, error))?;
    fs::write(path, payload)
}

fn read_profiles(profiles_dir: &Path) -> std::io::Result<Vec<ProfileSummary>> {
    let mut profiles = Vec::new();
    for entry in fs::read_dir(profiles_dir)? {
        let Ok(entry) = entry else {
            continue;
        };
        let path = entry.path().join("profile.json");
        if !path.exists() {
            continue;
        }
        let Ok(payload) = fs::read_to_string(path) else {
            continue;
        };
        if let Ok(profile) = serde_json::from_str::<ProfileSummary>(&payload) {
            profiles.push(profile);
        }
    }
    Ok(profiles)
}

#[tauri::command]
fn request_notification_permission() -> Result<NotificationPermission, String> {
    native_notifications::request_permission()
}

fn scan_inventory(project_path: &Path, home_dir: Option<&Path>) -> DesktopInventory {
    let home = home_dir_for_scan(project_path, home_dir);
    let store_dir = default_store_dir(&home);
    let scan = scan_project(&ScanOptions {
        project_path: project_path.display().to_string(),
        home_dir: home.display().to_string(),
        store_dir: store_dir.display().to_string(),
        explain: None,
        agent: None,
        scope: None,
    });

    inventory_from_evidence(&scan.evidence)
}

fn home_dir_for_scan(project_path: &Path, home_dir: Option<&Path>) -> PathBuf {
    home_dir
        .map(Path::to_path_buf)
        .or_else(|| env::var_os("HOME").map(PathBuf::from))
        .unwrap_or_else(|| project_path.join(".hem-no-home"))
}

fn inventory_from_evidence(evidence: &[DiscoveredItem]) -> DesktopInventory {
    let mut seen = HashSet::new();
    let mut inventory = DesktopInventory::default();

    for item in evidence {
        let bucket = match item.kind {
            EvidenceKind::Skill => Some(&mut inventory.skills),
            EvidenceKind::McpServer => Some(&mut inventory.mcp),
            EvidenceKind::Hook => Some(&mut inventory.hooks),
            _ => None,
        };
        let Some(bucket) = bucket else {
            continue;
        };

        let inventory_item = discovered_item_to_inventory_item(item);
        if seen.insert(inventory_item.id.clone()) {
            bucket.push(inventory_item);
        }
    }

    inventory.skills.sort_by(item_order);
    inventory.mcp.sort_by(item_order);
    inventory.hooks.sort_by(item_order);
    inventory
}

fn discovered_item_to_inventory_item(item: &DiscoveredItem) -> InventoryItem {
    let (status, detail) = match item.kind {
        EvidenceKind::McpServer => (mcp_status(item.value.as_ref()), mcp_detail(item.value.as_ref())),
        EvidenceKind::Hook => (hook_status(item), hook_detail(item)),
        EvidenceKind::Skill => (skill_status(item), skill_detail(item)),
        _ => (None, None),
    };

    InventoryItem {
        id: item.id.clone(),
        name: item
            .name
            .clone()
            .unwrap_or_else(|| item.source_path.clone()),
        agent: display_agent(item.agent),
        source_path: item.source_path.clone(),
        scope: item.scope.as_str().into(),
        status,
        detail,
    }
}

fn display_agent(agent: AgentId) -> String {
    match agent {
        AgentId::ClaudeCode => "Claude Code".into(),
        AgentId::Codex => "Codex".into(),
        AgentId::Cursor => "Cursor".into(),
        AgentId::Project => "Project".into(),
        AgentId::Opencode => "opencode".into(),
        AgentId::PiAgent => "pi-agent".into(),
        AgentId::Unknown => "unknown".into(),
    }
}

fn mcp_status(value: Option<&Value>) -> Option<String> {
    let value = value?;
    if value.get("disabled").and_then(Value::as_bool).unwrap_or(false) {
        return Some("disabled".into());
    }
    if value.get("enabled").and_then(Value::as_bool) == Some(false) {
        return Some("disabled".into());
    }
    Some("enabled".into())
}

fn mcp_detail(value: Option<&Value>) -> Option<String> {
    let value = value?;
    value
        .get("command")
        .and_then(Value::as_str)
        .or_else(|| value.get("url").and_then(Value::as_str))
        .map(str::to_owned)
}

fn hook_status(item: &DiscoveredItem) -> Option<String> {
    item.metadata
        .as_ref()
        .or(item.value.as_ref())
        .and_then(|value| value.get("type").and_then(Value::as_str))
        .map(str::to_owned)
        .or_else(|| {
            if item.parser == EvidenceParser::Toml {
                Some("toml".into())
            } else {
                None
            }
        })
}

fn hook_detail(item: &DiscoveredItem) -> Option<String> {
    item.metadata
        .as_ref()
        .or(item.value.as_ref())
        .and_then(|value| value.get("command").and_then(Value::as_str))
        .map(str::to_owned)
}

fn skill_detail(item: &DiscoveredItem) -> Option<String> {
    item.metadata
        .as_ref()
        .and_then(|value| value.get("description").and_then(Value::as_str))
        .map(str::to_owned)
}

fn skill_status(item: &DiscoveredItem) -> Option<String> {
    if skill_detail(item).is_some() {
        None
    } else {
        Some("missing description".into())
    }
}

fn surfaces_for(inventory: &DesktopInventory) -> Vec<SetupSurface> {
    vec![
        surface("mcp", "MCP", inventory.mcp.len()),
        surface("skills", "Skills", inventory.skills.len()),
        surface("hooks", "Hooks", inventory.hooks.len()),
    ]
}

fn surface(id: &str, label: &str, count: usize) -> SetupSurface {
    let noun = match id {
        "mcp" => "MCP server",
        "skills" => "skill",
        "hooks" => "hook",
        _ => "item",
    };
    SetupSurface {
        id: id.into(),
        label: label.into(),
        count: count as u32,
        risk: "low".into(),
        description: format!(
            "{count} {noun}{} installed",
            if count == 1 { "" } else { "s" }
        ),
    }
}

fn item_order(left: &InventoryItem, right: &InventoryItem) -> std::cmp::Ordering {
    left.agent
        .cmp(&right.agent)
        .then(left.scope.cmp(&right.scope))
        .then(left.name.cmp(&right.name))
}

fn desktop_project_path() -> PathBuf {
    let cwd = env::current_dir().unwrap_or_else(|_| PathBuf::from("."));
    if cwd.file_name().and_then(|name| name.to_str()) == Some("src-tauri") {
        return cwd.parent().map(Path::to_path_buf).unwrap_or(cwd);
    }
    cwd
}

fn home_dir() -> Option<PathBuf> {
    env::var_os("HOME").map(PathBuf::from)
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            desktop_home_state,
            request_notification_permission
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

#[cfg(target_os = "macos")]
mod native_notifications {
    use super::{Duration, NotificationPermission};
    use block2::RcBlock;
    use objc2::runtime::Bool;
    use objc2_foundation::NSError;
    use objc2_user_notifications::{UNAuthorizationOptions, UNUserNotificationCenter};
    use std::sync::mpsc;

    pub fn request_permission() -> Result<NotificationPermission, String> {
        let center = UNUserNotificationCenter::currentNotificationCenter();
        let options = UNAuthorizationOptions::Alert
            | UNAuthorizationOptions::Sound
            | UNAuthorizationOptions::Badge;
        let (sender, receiver) = mpsc::sync_channel(1);
        let completion = RcBlock::new(move |granted: Bool, error: *mut NSError| {
            let result = if error.is_null() {
                Ok(granted.as_bool())
            } else {
                Err("macOS notification authorization failed".to_owned())
            };
            let _ = sender.send(result);
        });

        center.requestAuthorizationWithOptions_completionHandler(options, &completion);

        match receiver.recv_timeout(Duration::from_secs(30)) {
            Ok(Ok(true)) => Ok(NotificationPermission {
                granted: true,
                status: "granted".into(),
            }),
            Ok(Ok(false)) => Ok(NotificationPermission {
                granted: false,
                status: "denied".into(),
            }),
            Ok(Err(message)) => Err(message),
            Err(_) => Err("Timed out waiting for macOS notification authorization".into()),
        }
    }
}

#[cfg(not(target_os = "macos"))]
mod native_notifications {
    use super::NotificationPermission;

    pub fn request_permission() -> Result<NotificationPermission, String> {
        Err("Notification permission requests are only implemented for macOS".into())
    }
}

#[cfg(test)]
mod desktop_home_state_tests {
    use super::build_desktop_home_state;
    use hem_core::{
        capture_current_state, capture_timeline_snapshot, write_snapshot,
        types::{AgentId, EvidenceScope, RuntimeOptions},
        CaptureTimelineOptions, StoreSnapshot,
    };
    use std::fs;
    use tempfile::TempDir;

    fn write_project_mcp(project_path: &std::path::Path, command: &str) {
        let payload = serde_json::json!({
            "mcpServers": {
                "github": { "command": command }
            }
        });
        fs::write(
            project_path.join(".mcp.json"),
            serde_json::to_string_pretty(&payload).expect("serialize mcp"),
        )
        .expect("write mcp");
    }

    #[test]
    fn populates_current_snapshot_id_from_store() {
        let root = TempDir::new().expect("temp dir");
        let project_path = root.path().join("project");
        let home_dir = root.path().join("home");
        let store_dir = home_dir.join(".hem");
        fs::create_dir_all(&project_path).expect("mkdir project");
        fs::create_dir_all(&home_dir).expect("mkdir home");

        let runtime = RuntimeOptions {
            project_path: project_path.display().to_string(),
            home_dir: home_dir.display().to_string(),
            store_dir: store_dir.display().to_string(),
            agent: Some(AgentId::Codex),
            scope: Some(EvidenceScope::User),
            capture_content: Some(false),
        };

        write_project_mcp(&project_path, "gh-baseline");
        let state = capture_current_state(&runtime, "desktop-baseline").expect("capture");
        write_snapshot(&store_dir, StoreSnapshot::from(state.snapshot), Some(AgentId::Codex))
            .expect("write snapshot");

        capture_timeline_snapshot(
            &runtime,
            &CaptureTimelineOptions {
                capture_id: Some("desktop-test-capture".to_string()),
                snapshot_name: Some("desktop-baseline".to_string()),
                title: Some("Desktop baseline".to_string()),
                skip_unchanged: false,
            },
        )
        .expect("timeline capture");

        write_project_mcp(&project_path, "gh-changed");

        let home = build_desktop_home_state(&project_path, Some(&home_dir));
        assert_eq!(
            home.current_snapshot_id.as_deref(),
            Some("desktop-baseline"),
            "expected latest snapshot id, got {:?}",
            home.current_snapshot_id
        );
        assert_eq!(home.protection, "on");
        assert!(!home.surfaces.is_empty());
        assert!(
            home.working_changes > 0,
            "expected drift after mcp change, got {}",
            home.working_changes
        );
        assert!(
            !home.changelog.is_empty(),
            "expected timeline changelog entries"
        );

        if let Ok(scratch) = std::env::var("HEM_SCRATCH_DIR") {
            let serialized =
                serde_json::to_string_pretty(&home).expect("serialize desktop home state");
            fs::write(format!("{scratch}/desktop-home-state.json"), serialized)
                .expect("write desktop home state evidence");
        }
    }
}
