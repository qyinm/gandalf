use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct SnapError {
    pub code: String,
    pub problem: String,
    pub cause: String,
    pub fix: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub path: Option<String>,
}

pub fn format_snap_error(error: &SnapError) -> String {
    let mut lines = vec![
        error.code.clone(),
        format!("Problem: {}", error.problem),
        format!("Cause: {}", error.cause),
        format!("Fix: {}", error.fix),
    ];

    if let Some(path) = &error.path {
        lines.push(format!("Path: {path}"));
    }

    format!("{}\n", lines.join("\n"))
}