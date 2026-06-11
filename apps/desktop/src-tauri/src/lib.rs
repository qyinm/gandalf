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
    let project_path = desktop_project_path();
    let home_dir = home_dir();
    let profile_state = ensure_profile_state(home_dir.as_deref()).unwrap_or_else(|_| default_profile_state());
    let inventory = scan_inventory(&project_path, home_dir.as_deref());

    DesktopHomeState {
        active_profile: profile_state.active,
        profiles: profile_state.profiles,
        current_snapshot_id: None,
        protection: "off".into(),
        highest_risk: None,
        working_changes: 0,
        surfaces: surfaces_for(&inventory),
        changelog: Vec::new(),
        inventory,
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
    let mut seen = HashSet::new();
    let mut inventory = DesktopInventory::default();

    scan_mcp_json(
        &project_path.join(".mcp.json"),
        "Claude Code",
        "project",
        project_path,
        home_dir,
        &mut inventory.mcp,
        &mut seen,
    );
    scan_mcp_json(
        &project_path.join(".cursor/mcp.json"),
        "Cursor",
        "project",
        project_path,
        home_dir,
        &mut inventory.mcp,
        &mut seen,
    );
    scan_hooks_json(
        &project_path.join(".cursor/hooks.json"),
        "Cursor",
        "project",
        project_path,
        home_dir,
        &mut inventory.hooks,
        &mut seen,
    );
    scan_hooks_json(
        &project_path.join(".codex/hooks.json"),
        "Codex",
        "project",
        project_path,
        home_dir,
        &mut inventory.hooks,
        &mut seen,
    );
    scan_skill_roots(
        &[
            project_path.join(".codex/skills"),
            project_path.join(".cursor/skills"),
            project_path.join(".claude/skills"),
            project_path.join(".agents/skills"),
            project_path.join(".pi/skills"),
        ],
        "Project",
        "project",
        project_path,
        home_dir,
        &mut inventory.skills,
        &mut seen,
    );

    if let Some(home) = home_dir {
        scan_mcp_json(
            &home.join(".cursor/mcp.json"),
            "Cursor",
            "user",
            project_path,
            Some(home),
            &mut inventory.mcp,
            &mut seen,
        );
        scan_settings_json(
            &home.join(".claude/settings.json"),
            "Claude Code",
            "user",
            project_path,
            Some(home),
            &mut inventory,
            &mut seen,
        );
        scan_codex_config_toml(
            &home.join(".codex/config.toml"),
            project_path,
            Some(home),
            &mut inventory,
            &mut seen,
        );
        scan_hooks_json(
            &home.join(".cursor/hooks.json"),
            "Cursor",
            "user",
            project_path,
            Some(home),
            &mut inventory.hooks,
            &mut seen,
        );
        scan_hooks_json(
            &home.join(".codex/hooks.json"),
            "Codex",
            "user",
            project_path,
            Some(home),
            &mut inventory.hooks,
            &mut seen,
        );
        scan_skill_roots(
            &[
                home.join(".codex/skills"),
                home.join(".codex/plugins/cache"),
                home.join(".codex/vendor_imports/skills"),
                home.join(".cursor/skills"),
                home.join(".claude/skills"),
                home.join(".agents/skills"),
                home.join(".pi/agent/skills"),
            ],
            "User",
            "user",
            project_path,
            Some(home),
            &mut inventory.skills,
            &mut seen,
        );
    }

    inventory.skills.sort_by(item_order);
    inventory.mcp.sort_by(item_order);
    inventory.hooks.sort_by(item_order);
    inventory
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

fn scan_mcp_json(
    path: &Path,
    agent: &str,
    scope: &str,
    project_path: &Path,
    home_dir: Option<&Path>,
    items: &mut Vec<InventoryItem>,
    seen: &mut HashSet<String>,
) {
    let Some(root) = read_json_object(path) else {
        return;
    };
    let Some(servers) = root.get("mcpServers").and_then(Value::as_object) else {
        return;
    };

    for (name, value) in servers {
        let disabled = value
            .get("disabled")
            .and_then(Value::as_bool)
            .unwrap_or(false);
        let detail = value
            .get("command")
            .and_then(Value::as_str)
            .or_else(|| value.get("url").and_then(Value::as_str))
            .map(str::to_owned);
        push_item(
            items,
            seen,
            InventoryItem {
                id: format!("mcp:{agent}:{scope}:{name}:{}", path.display()),
                name: name.clone(),
                agent: agent.into(),
                source_path: display_path(path, project_path, home_dir),
                scope: scope.into(),
                status: Some(if disabled { "disabled" } else { "enabled" }.into()),
                detail,
            },
        );
    }
}

fn scan_settings_json(
    path: &Path,
    agent: &str,
    scope: &str,
    project_path: &Path,
    home_dir: Option<&Path>,
    inventory: &mut DesktopInventory,
    seen: &mut HashSet<String>,
) {
    let Some(root) = read_json_object(path) else {
        return;
    };
    if root.get("mcpServers").is_some() {
        scan_mcp_json(
            path,
            agent,
            scope,
            project_path,
            home_dir,
            &mut inventory.mcp,
            seen,
        );
    }
    collect_hooks_from_value(
        path,
        agent,
        scope,
        project_path,
        home_dir,
        root.get("hooks").unwrap_or(&Value::Null),
        &mut inventory.hooks,
        seen,
    );
}

fn scan_hooks_json(
    path: &Path,
    agent: &str,
    scope: &str,
    project_path: &Path,
    home_dir: Option<&Path>,
    items: &mut Vec<InventoryItem>,
    seen: &mut HashSet<String>,
) {
    let Some(root) = read_json_object(path) else {
        return;
    };
    collect_hooks_from_value(
        path,
        agent,
        scope,
        project_path,
        home_dir,
        root.get("hooks").unwrap_or(&Value::Null),
        items,
        seen,
    );
}

fn collect_hooks_from_value(
    path: &Path,
    agent: &str,
    scope: &str,
    project_path: &Path,
    home_dir: Option<&Path>,
    hooks: &Value,
    items: &mut Vec<InventoryItem>,
    seen: &mut HashSet<String>,
) {
    let Some(events) = hooks.as_object() else {
        return;
    };

    for (event_name, definitions) in events {
        let Some(groups) = definitions.as_array() else {
            continue;
        };
        for (group_index, group) in groups.iter().enumerate() {
            let matcher = group.get("matcher").and_then(Value::as_str).unwrap_or("*");
            let nested_hooks = group
                .get("hooks")
                .and_then(Value::as_array)
                .unwrap_or(groups);
            for (hook_index, hook) in nested_hooks.iter().enumerate() {
                let command = hook
                    .get("command")
                    .and_then(Value::as_str)
                    .map(str::to_owned);
                push_item(
                    items,
                    seen,
                    InventoryItem {
                        id: format!(
                            "hook:{agent}:{scope}:{event_name}:{group_index}:{hook_index}:{}",
                            path.display()
                        ),
                        name: format!("{event_name}.{matcher}"),
                        agent: agent.into(),
                        source_path: display_path(path, project_path, home_dir),
                        scope: scope.into(),
                        status: hook.get("type").and_then(Value::as_str).map(str::to_owned),
                        detail: command,
                    },
                );
            }
        }
    }
}

fn scan_codex_config_toml(
    path: &Path,
    project_path: &Path,
    home_dir: Option<&Path>,
    inventory: &mut DesktopInventory,
    seen: &mut HashSet<String>,
) {
    let Ok(text) = fs::read_to_string(path) else {
        return;
    };
    let mut current_mcp: Option<String> = None;
    let mut current_hook: Option<String> = None;
    let mut disabled = false;
    let mut detail: Option<String> = None;

    for line in text.lines().chain([""].iter().copied()) {
        let trimmed = line.split('#').next().unwrap_or("").trim();
        if trimmed.starts_with("[") {
            flush_codex_mcp(
                path,
                project_path,
                home_dir,
                inventory,
                seen,
                current_mcp.take(),
                disabled,
                detail.take(),
            );
            disabled = false;
            current_hook = None;
            current_mcp = codex_mcp_section_name(trimmed);
            if let Some(event) = codex_hook_section_name(trimmed) {
                current_hook = Some(event.clone());
                push_item(
                    &mut inventory.hooks,
                    seen,
                    InventoryItem {
                        id: format!("hook:Codex:user:{event}:{}", path.display()),
                        name: event,
                        agent: "Codex".into(),
                        source_path: display_path(path, project_path, home_dir),
                        scope: "user".into(),
                        status: Some("toml".into()),
                        detail: None,
                    },
                );
            }
            continue;
        }
        if current_mcp.is_some() {
            if let Some(value) = bool_assignment(trimmed, "disabled") {
                disabled = value;
            }
            if detail.is_none() {
                detail = string_assignment(trimmed, "command")
                    .or_else(|| string_assignment(trimmed, "url"));
            }
        }
        if current_hook.is_some() && detail.is_none() {
            detail = string_assignment(trimmed, "command");
        }
    }
}

fn flush_codex_mcp(
    path: &Path,
    project_path: &Path,
    home_dir: Option<&Path>,
    inventory: &mut DesktopInventory,
    seen: &mut HashSet<String>,
    name: Option<String>,
    disabled: bool,
    detail: Option<String>,
) {
    let Some(name) = name else {
        return;
    };
    push_item(
        &mut inventory.mcp,
        seen,
        InventoryItem {
            id: format!("mcp:Codex:user:{name}:{}", path.display()),
            name,
            agent: "Codex".into(),
            source_path: display_path(path, project_path, home_dir),
            scope: "user".into(),
            status: Some(if disabled { "disabled" } else { "enabled" }.into()),
            detail,
        },
    );
}

fn scan_skill_roots(
    roots: &[PathBuf],
    agent: &str,
    scope: &str,
    project_path: &Path,
    home_dir: Option<&Path>,
    items: &mut Vec<InventoryItem>,
    seen: &mut HashSet<String>,
) {
    for root in roots {
        if !root.is_dir() {
            continue;
        }
        let mut stack = vec![(root.clone(), 0usize)];
        while let Some((dir, depth)) = stack.pop() {
            if depth > 8 {
                continue;
            }
            let skill_file = dir.join("SKILL.md");
            if skill_file.is_file() {
                let name = skill_name(&skill_file).unwrap_or_else(|| {
                    dir.file_name()
                        .and_then(|part| part.to_str())
                        .unwrap_or("skill")
                        .to_owned()
                });
                push_item(
                    items,
                    seen,
                    InventoryItem {
                        id: format!("skill:{agent}:{scope}:{}", dir.display()),
                        name,
                        agent: agent.into(),
                        source_path: display_path(&dir, project_path, home_dir),
                        scope: scope.into(),
                        status: skill_status(&skill_file),
                        detail: skill_description(&skill_file),
                    },
                );
                continue;
            }
            let Ok(entries) = fs::read_dir(&dir) else {
                continue;
            };
            for entry in entries.flatten() {
                let path = entry.path();
                if path.is_dir() {
                    stack.push((path, depth + 1));
                } else if depth == 0 && path.extension().and_then(|ext| ext.to_str()) == Some("md")
                {
                    let name = skill_name(&path).or_else(|| {
                        path.file_stem()
                            .and_then(|part| part.to_str())
                            .map(str::to_owned)
                    });
                    if let Some(name) = name {
                        push_item(
                            items,
                            seen,
                            InventoryItem {
                                id: format!("skill:{agent}:{scope}:{}", path.display()),
                                name,
                                agent: agent.into(),
                                source_path: display_path(&path, project_path, home_dir),
                                scope: scope.into(),
                                status: skill_status(&path),
                                detail: skill_description(&path),
                            },
                        );
                    }
                }
            }
        }
    }
}

fn read_json_object(path: &Path) -> Option<serde_json::Map<String, Value>> {
    let text = fs::read_to_string(path).ok()?;
    serde_json::from_str::<Value>(&text)
        .ok()?
        .as_object()
        .cloned()
}

fn skill_name(path: &Path) -> Option<String> {
    frontmatter_value(path, "name")
}

fn skill_description(path: &Path) -> Option<String> {
    frontmatter_value(path, "description")
}

fn skill_status(path: &Path) -> Option<String> {
    if skill_description(path).is_some() {
        None
    } else {
        Some("missing description".into())
    }
}

fn frontmatter_value(path: &Path, key: &str) -> Option<String> {
    let text = fs::read_to_string(path).ok()?;
    let mut in_frontmatter = false;
    for (index, line) in text.lines().enumerate() {
        let trimmed = line.trim();
        if index == 0 && trimmed == "---" {
            in_frontmatter = true;
            continue;
        }
        if in_frontmatter && trimmed == "---" {
            break;
        }
        if in_frontmatter {
            let prefix = format!("{key}:");
            if let Some(value) = trimmed.strip_prefix(&prefix) {
                let value = value.trim().trim_matches('"').trim_matches('\'');
                if !value.is_empty() {
                    return Some(value.to_owned());
                }
            }
        }
    }
    None
}

fn codex_mcp_section_name(line: &str) -> Option<String> {
    let section = line.strip_prefix('[')?.strip_suffix(']')?;
    let name = section.strip_prefix("mcp_servers.")?;
    if name.contains('.') || name.is_empty() {
        None
    } else {
        Some(name.trim_matches('"').to_owned())
    }
}

fn codex_hook_section_name(line: &str) -> Option<String> {
    let section = line.strip_prefix("[[")?.strip_suffix("]]")?;
    let name = section.strip_prefix("hooks.")?;
    let name = name.strip_suffix(".hooks").unwrap_or(name);
    if name.is_empty() {
        None
    } else {
        Some(name.trim_matches('"').to_owned())
    }
}

fn bool_assignment(line: &str, key: &str) -> Option<bool> {
    let value = line.strip_prefix(&format!("{key} = "))?.trim();
    match value {
        "true" => Some(true),
        "false" => Some(false),
        _ => None,
    }
}

fn string_assignment(line: &str, key: &str) -> Option<String> {
    let value = line.strip_prefix(&format!("{key} = "))?.trim();
    Some(value.trim_matches('"').trim_matches('\'').to_owned()).filter(|value| !value.is_empty())
}

fn push_item(items: &mut Vec<InventoryItem>, seen: &mut HashSet<String>, item: InventoryItem) {
    if seen.insert(item.id.clone()) {
        items.push(item);
    }
}

fn item_order(left: &InventoryItem, right: &InventoryItem) -> std::cmp::Ordering {
    left.agent
        .cmp(&right.agent)
        .then(left.scope.cmp(&right.scope))
        .then(left.name.cmp(&right.name))
}

fn display_path(path: &Path, project_path: &Path, home_dir: Option<&Path>) -> String {
    if let Ok(relative) = path.strip_prefix(project_path) {
        let value = relative.to_string_lossy().replace('\\', "/");
        return if value.is_empty() { ".".into() } else { value };
    }
    if let Some(home) = home_dir {
        if let Ok(relative) = path.strip_prefix(home) {
            return format!("~/{}", relative.to_string_lossy().replace('\\', "/"));
        }
    }
    path.to_string_lossy().replace('\\', "/")
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
