use std::path::Path;

use crate::types::{AgentId, EvidenceKind, EvidenceParser};

use super::super::{project_target, ScanTarget, ScanTargetOverrides};
use super::ScannerPlugin;

pub struct ProjectScanner;

impl ScannerPlugin for ProjectScanner {
    fn agent_id(&self) -> AgentId {
        AgentId::Project
    }

    fn agent_name(&self) -> &'static str {
        "Project"
    }

    fn description(&self) -> &'static str {
        "Project-level environment variable inventory"
    }

    fn targets(&self, project_path: &Path, _home_dir: &Path) -> Vec<ScanTarget> {
        vec![project_target(
            project_path,
            ".env",
            AgentId::Project,
            EvidenceKind::EnvKey,
            EvidenceParser::Dotenv,
            ScanTargetOverrides {
                sensitivity: Some("env_key_inventory".to_string()),
                content_policy: Some("key_inventory_only".to_string()),
                ..ScanTargetOverrides::default()
            },
        )]
    }
}

pub fn project_scanner() -> ProjectScanner {
    ProjectScanner
}