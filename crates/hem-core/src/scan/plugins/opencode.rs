use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{Path, PathBuf};

use regex::Regex;
use serde_json::json;

use crate::types::{
    AgentId, CaptureStatus, DiscoveredItem, EvidenceConfidence, EvidenceKind, EvidenceParser,
    RestorePolicy,
};

use super::super::base::{metadata_string_array, scanner_item_id, unquote_yaml_scalar};
use super::super::filesystem::scan_targets;
use super::super::{home_target, project_target, ScanTarget, ScanTargetOverrides};
use super::{ScannerContext, ScannerPlugin};

const MAX_SKILL_DEPTH: u32 = 8;

pub struct OpenCodeScanner;

impl ScannerPlugin for OpenCodeScanner {
    fn agent_id(&self) -> AgentId {
        AgentId::Opencode
    }

    fn agent_name(&self) -> &'static str {
        "OpenCode"
    }

    fn description(&self) -> &'static str {
        "OpenCode CLI configuration (MCP servers, plugins, providers, skills)"
    }

    fn targets(&self, _project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
        vec![home_target(
            home_dir,
            ".config/opencode/opencode.json",
            AgentId::Opencode,
            EvidenceKind::AgentConfig,
            EvidenceParser::Json,
            ScanTargetOverrides::default(),
        )]
    }

    fn scan(&self, context: &ScannerContext) -> Option<Vec<DiscoveredItem>> {
        let config_evidence = scan_targets(&self.targets(
            &context.project_path,
            &context.home_dir,
        ));
        let mut skill_evidence = vec![builtin_customize_opencode_skill()];

        for target in opencode_skill_targets(&context.project_path, &context.home_dir) {
            skill_evidence.extend(scan_opencode_skill_directory(&target));
        }

        let mut evidence = config_evidence;
        evidence.extend(dedupe_skills_by_name(skill_evidence));
        Some(evidence)
    }
}

pub fn opencode_scanner() -> OpenCodeScanner {
    OpenCodeScanner
}

fn opencode_skill_targets(project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
    let directory = ScanTargetOverrides {
        directory: Some(true),
        ..Default::default()
    };
    vec![
        project_target(
            project_path,
            ".opencode/skills",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".config/opencode/skills",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        project_target(
            project_path,
            ".opencode/skill",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".config/opencode/skill",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        project_target(
            project_path,
            ".claude/skills",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".claude/skills",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        project_target(
            project_path,
            ".agents/skills",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".agents/skills",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".cache/opencode/packages",
            AgentId::Opencode,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory,
        ),
    ]
}

fn scan_opencode_skill_directory(target: &ScanTarget) -> Vec<DiscoveredItem> {
    let skill_files = find_skill_files(&target.absolute_path);
    let mut evidence = Vec::new();

    for skill_file in skill_files {
        let Some(frontmatter) = read_skill_frontmatter(&skill_file) else {
            continue;
        };
        let Some(name) = frontmatter.name else {
            continue;
        };
        let Some(description) = frontmatter.description else {
            continue;
        };

        let skill_dir = skill_file.parent().unwrap_or(Path::new(""));
        let directory_name = skill_dir
            .file_name()
            .map(|value| value.to_string_lossy().to_string())
            .unwrap_or_default();
        if !valid_skill_name(&name) {
            continue;
        }

        let relative_skill_dir = skill_dir
            .strip_prefix(&target.absolute_path)
            .map(|path| path.to_string_lossy().replace('\\', "/"))
            .unwrap_or_default();
        let source_path = format!("{}/{}", target.source_path, relative_skill_dir);
        let entrypoint_status = skill_entrypoint_status(&target.absolute_path, &skill_file);

        evidence.push(DiscoveredItem {
            id: scanner_item_id(target.scope, target.agent, &source_path, "skill"),
            agent: target.agent,
            kind: EvidenceKind::Skill,
            source_path,
            scope: target.scope,
            precedence: target.precedence,
            parser: EvidenceParser::Filesystem,
            sensitivity: target.sensitivity.clone(),
            content_policy: target.content_policy.clone(),
            restore_policy: RestorePolicy::FullContentSupported,
            capture_status: CaptureStatus::Captured,
            confidence: EvidenceConfidence::High,
            name: Some(name.clone()),
            value: None,
            checksum: None,
            metadata: Some(json!({
                "present": true,
                "entrypoint": "SKILL.md",
                "entrypointStatus": entrypoint_status,
                "entrypointSizeBytes": frontmatter.size_bytes,
                "declaredName": name,
                "directoryName": directory_name,
                "nameMatchesDirectory": name == directory_name,
                "description": description,
            })),
        });
    }

    evidence
}

fn find_skill_files(root: &Path) -> Vec<PathBuf> {
    let mut files = Vec::new();
    walk_skill_files(root, &mut files, 0, &mut HashSet::new());
    files
}

fn walk_skill_files(dir: &Path, files: &mut Vec<PathBuf>, depth: u32, seen: &mut HashSet<String>) {
    if depth > MAX_SKILL_DEPTH {
        return;
    }

    let resolved = match fs::canonicalize(dir) {
        Ok(path) => path,
        Err(_) => return,
    };
    let resolved_key = resolved.to_string_lossy().to_string();
    if !seen.insert(resolved_key) {
        return;
    }

    let entries = match fs::read_dir(dir) {
        Ok(entries) => entries,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let absolute_path = entry.path();
        let metadata = match fs::metadata(&absolute_path) {
            Ok(metadata) => metadata,
            Err(_) => continue,
        };

        if metadata.is_dir() {
            walk_skill_files(&absolute_path, files, depth + 1, seen);
            continue;
        }

        if metadata.is_file() && entry.file_name().to_string_lossy().eq_ignore_ascii_case("skill.md")
        {
            files.push(absolute_path);
        }
    }
}

struct OpenCodeSkillFrontmatter {
    name: Option<String>,
    description: Option<String>,
    size_bytes: u64,
}

fn read_skill_frontmatter(file_path: &Path) -> Option<OpenCodeSkillFrontmatter> {
    let metadata = fs::metadata(file_path).ok()?;
    let text = fs::read_to_string(file_path).ok()?;
    let captures = Regex::new(r"^---\n([\s\S]*?)\n---").ok()?.captures(&text)?;
    let body = captures.get(1)?.as_str();
    let mut frontmatter = OpenCodeSkillFrontmatter {
        name: None,
        description: None,
        size_bytes: metadata.len(),
    };
    let field_re = Regex::new(r"^(name|description):\s*(.*)$").ok()?;
    for line in body.split('\n') {
        if let Some(caps) = field_re.captures(line.trim()) {
            let key = caps.get(1)?.as_str();
            let value = unquote_yaml_scalar(caps.get(2)?.as_str());
            match key {
                "name" => frontmatter.name = Some(value),
                "description" => frontmatter.description = Some(value),
                _ => {}
            }
        }
    }
    Some(frontmatter)
}

fn skill_entrypoint_status(root: &Path, skill_file: &Path) -> String {
    let relative_parts = skill_file
        .strip_prefix(root)
        .map(|path| path.components().collect::<Vec<_>>())
        .unwrap_or_default();
    let mut cursor = root.to_path_buf();

    for part in relative_parts {
        cursor.push(part.as_os_str());
        match fs::symlink_metadata(&cursor) {
            Ok(metadata) if metadata.file_type().is_symlink() => {
                let part_name = part.as_os_str().to_string_lossy();
                return if part_name == "SKILL.md" {
                    "symlink_followed".to_string()
                } else {
                    "symlink_directory_followed".to_string()
                };
            }
            Err(_) => return "captured".to_string(),
            _ => {}
        }
    }

    "captured".to_string()
}

fn valid_skill_name(name: &str) -> bool {
    Regex::new(r"^[a-z0-9]+(-[a-z0-9]+)*$")
        .map(|re| re.is_match(name) && name.len() <= 64)
        .unwrap_or(false)
}

fn builtin_customize_opencode_skill() -> DiscoveredItem {
    DiscoveredItem {
        id: "managed.opencode.built-in.customize-opencode.skill".to_string(),
        agent: AgentId::Opencode,
        kind: EvidenceKind::Skill,
        source_path: "<built-in>".to_string(),
        scope: crate::types::EvidenceScope::Managed,
        precedence: 100,
        parser: EvidenceParser::Filesystem,
        sensitivity: "metadata".to_string(),
        content_policy: "metadata_only".to_string(),
        restore_policy: RestorePolicy::NotSupported,
        capture_status: CaptureStatus::Captured,
        confidence: EvidenceConfidence::High,
        name: Some("customize-opencode".to_string()),
        value: None,
        checksum: None,
        metadata: Some(json!({
            "present": true,
            "builtIn": true,
            "declaredName": "customize-opencode",
            "description": "Use when editing or creating opencode configuration, agents, skills, plugins, MCP servers, or permission rules.",
        })),
    }
}

fn dedupe_skills_by_name(evidence: Vec<DiscoveredItem>) -> Vec<DiscoveredItem> {
    let mut result = Vec::new();
    let mut skill_indexes: HashMap<String, usize> = HashMap::new();

    for item in evidence {
        if item.kind != EvidenceKind::Skill {
            result.push(item);
            continue;
        }
        let Some(name) = item.name.clone() else {
            result.push(item);
            continue;
        };

        if let Some(existing_index) = skill_indexes.get(&name).copied() {
            let mut existing_item = result[existing_index].clone();
            let mut duplicate_sources = metadata_string_array(
                existing_item.metadata.as_ref().and_then(|m| m.get("duplicateSources")),
            );
            duplicate_sources.push(item.source_path);
            if let Some(metadata) = existing_item.metadata.as_mut() {
                if let Some(obj) = metadata.as_object_mut() {
                    obj.insert("duplicateSources".to_string(), json!(duplicate_sources));
                }
            }
            result[existing_index] = existing_item;
        } else {
            skill_indexes.insert(name, result.len());
            result.push(item);
        }
    }

    result
}