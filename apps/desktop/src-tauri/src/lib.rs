use serde::Serialize;

#[derive(Serialize)]
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

#[derive(Serialize)]
struct SetupSurface {
    id: String,
    label: String,
    count: u32,
    risk: String,
    description: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct DesktopHomeState {
    active_profile: ProfileSummary,
    current_snapshot_id: String,
    protection: String,
    highest_risk: String,
    working_changes: u32,
    changelog: Vec<ChangelogEntry>,
    surfaces: Vec<SetupSurface>,
}

#[tauri::command]
fn desktop_home_state() -> DesktopHomeState {
    DesktopHomeState {
        active_profile: ProfileSummary {
            name: "Default".into(),
            scope: "personal".into(),
            sync_state: "local_only".into(),
            ahead: 0,
            behind: 0,
        },
        current_snapshot_id: "8f3a2c7".into(),
        protection: "on".into(),
        highest_risk: "medium".into(),
        working_changes: 3,
        surfaces: vec![
            SetupSurface {
                id: "setup".into(),
                label: "Setup".into(),
                count: 7,
                risk: "medium".into(),
                description: "Codex config, permissions, env key inventory".into(),
            },
            SetupSurface {
                id: "mcp".into(),
                label: "MCP".into(),
                count: 2,
                risk: "high".into(),
                description: "Configured MCP servers and required env keys".into(),
            },
            SetupSurface {
                id: "skills".into(),
                label: "Skills".into(),
                count: 4,
                risk: "low".into(),
                description: "Installed Codex skills detected in user-global roots".into(),
            },
            SetupSurface {
                id: "hooks".into(),
                label: "Hooks".into(),
                count: 1,
                risk: "medium".into(),
                description: "Executable setup hooks requiring review".into(),
            },
        ],
        changelog: vec![
            ChangelogEntry {
                id: "8f3a2c7".into(),
                title: "MCP server changed: figma".into(),
                time: "12 min ago".into(),
                source: "auto".into(),
                risk: "high".into(),
            },
            ChangelogEntry {
                id: "72ab91d".into(),
                title: "Snapshot created from Default".into(),
                time: "1h ago".into(),
                source: "manual".into(),
                risk: "medium".into(),
            },
            ChangelogEntry {
                id: "19df02a".into(),
                title: "Initial Codex setup captured".into(),
                time: "Yesterday".into(),
                source: "manual".into(),
                risk: "low".into(),
            },
        ],
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![desktop_home_state])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
