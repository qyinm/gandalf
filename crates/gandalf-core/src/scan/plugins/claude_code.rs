use std::path::Path;

use crate::types::{AgentId, EvidenceKind, EvidenceParser};

use super::super::{home_target, project_target, ScanTarget, ScanTargetOverrides};
use super::ScannerPlugin;

pub struct ClaudeCodeScanner;

impl ScannerPlugin for ClaudeCodeScanner {
    fn agent_id(&self) -> AgentId {
        AgentId::ClaudeCode
    }

    fn agent_name(&self) -> &'static str {
        "Claude Code"
    }

    fn description(&self) -> &'static str {
        "Claude Code agent configuration (prompts, MCP servers, settings, skills)"
    }

    fn targets(&self, project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
        vec![
            project_target(
                project_path,
                "CLAUDE.md",
                AgentId::ClaudeCode,
                EvidenceKind::AgentInstruction,
                EvidenceParser::Markdown,
                ScanTargetOverrides::default(),
            ),
            project_target(
                project_path,
                ".mcp.json",
                AgentId::ClaudeCode,
                EvidenceKind::AgentConfig,
                EvidenceParser::Json,
                ScanTargetOverrides::default(),
            ),
            project_target(
                project_path,
                ".claude/settings.json",
                AgentId::ClaudeCode,
                EvidenceKind::AgentConfig,
                EvidenceParser::Json,
                ScanTargetOverrides::default(),
            ),
            home_target(
                home_dir,
                ".claude/settings.json",
                AgentId::ClaudeCode,
                EvidenceKind::AgentConfig,
                EvidenceParser::Json,
                ScanTargetOverrides::default(),
            ),
            home_target(
                home_dir,
                ".claude.json",
                AgentId::ClaudeCode,
                EvidenceKind::AgentConfig,
                EvidenceParser::Json,
                ScanTargetOverrides {
                    metadata_only: Some(true),
                    sensitivity: Some("metadata".to_string()),
                    ..ScanTargetOverrides::default()
                },
            ),
            home_target(
                home_dir,
                ".claude/agents",
                AgentId::ClaudeCode,
                EvidenceKind::Unsupported,
                EvidenceParser::Filesystem,
                ScanTargetOverrides {
                    directory: Some(true),
                    ..ScanTargetOverrides::default()
                },
            ),
            home_target(
                home_dir,
                ".claude/skills",
                AgentId::ClaudeCode,
                EvidenceKind::Skill,
                EvidenceParser::Filesystem,
                ScanTargetOverrides {
                    directory: Some(true),
                    ..ScanTargetOverrides::default()
                },
            ),
        ]
    }
}

pub fn claude_code_scanner() -> ClaudeCodeScanner {
    ClaudeCodeScanner
}