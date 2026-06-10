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
    active_profile: Option<ProfileSummary>,
    current_snapshot_id: Option<String>,
    protection: String,
    highest_risk: Option<String>,
    working_changes: u32,
    changelog: Vec<ChangelogEntry>,
    surfaces: Vec<SetupSurface>,
}

#[tauri::command]
fn desktop_home_state() -> DesktopHomeState {
    DesktopHomeState {
        active_profile: None,
        current_snapshot_id: None,
        protection: "off".into(),
        highest_risk: None,
        working_changes: 0,
        surfaces: Vec::new(),
        changelog: Vec::new(),
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
