pub mod claude_code;
pub mod codex;
pub mod cursor;
pub mod opencode;
pub mod pi;
pub mod project;

use std::path::{Path, PathBuf};

use crate::types::{AgentId, DiscoveredItem, EvidenceScope};

use super::ScanTarget;

pub use claude_code::claude_code_scanner;
pub use codex::CodexScanner;
pub use cursor::cursor_scanner;
pub use opencode::opencode_scanner;
pub use pi::pi_agent_scanner;
pub use project::project_scanner;

#[derive(Debug, Clone)]
pub struct ScannerContext {
    pub project_path: PathBuf,
    pub home_dir: PathBuf,
    pub store_dir: String,
    pub explain: bool,
    pub scope: Option<EvidenceScope>,
}

pub trait ScannerPlugin {
    fn agent_id(&self) -> AgentId;
    fn agent_name(&self) -> &'static str;
    fn description(&self) -> &'static str;
    fn targets(&self, project_path: &Path, home_dir: &Path) -> Vec<ScanTarget>;

    fn scan(&self, _context: &ScannerContext) -> Option<Vec<DiscoveredItem>> {
        None
    }
}