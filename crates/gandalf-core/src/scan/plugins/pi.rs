use std::collections::{HashMap, HashSet};
use std::env;
use std::fs;
use std::path::{Component, Path, PathBuf};

use regex::Regex;
use serde_json::{json, Map, Value};

use crate::types::{
    AgentId, CaptureStatus, DiscoveredItem, EvidenceConfidence, EvidenceKind, EvidenceParser,
    EvidenceScope, RestorePolicy,
};

use super::super::base::{
    array_of_strings, as_object, is_object, metadata_string_array, scanner_item_id,
    unquote_yaml_scalar,
};
use super::super::filesystem::scan_targets;
use super::super::{home_target, project_target, ScanTarget, ScanTargetOverrides};
use super::{ScannerContext, ScannerPlugin};

#[derive(Debug, Clone)]
struct PiSkillTarget {
    absolute_path: PathBuf,
    source_path: String,
    scope: EvidenceScope,
    precedence: u32,
    include_root_files: bool,
    source: String,
}

#[derive(Debug, Clone)]
struct PiSkillFile {
    file_path: PathBuf,
    skill_dir: PathBuf,
    root: PathBuf,
}

#[derive(Debug, Clone)]
struct PiExtensionTarget {
    absolute_path: PathBuf,
    source_path: String,
    scope: EvidenceScope,
    precedence: u32,
    source: String,
}

#[derive(Debug, Clone)]
struct PiExtensionFile {
    file_path: PathBuf,
    root: PathBuf,
}

#[derive(Debug, Clone)]
struct PiFrontmatter {
    name: Option<String>,
    description: Option<String>,
    disable_model_invocation: Option<bool>,
    size_bytes: u64,
}

pub struct PiAgentScanner;

impl ScannerPlugin for PiAgentScanner {
    fn agent_id(&self) -> AgentId {
        AgentId::PiAgent
    }

    fn agent_name(&self) -> &'static str {
        "Pi Agent"
    }

    fn description(&self) -> &'static str {
        "Pi coding agent configuration (settings, models, agents, extensions, skills, themes, prompts)"
    }

    fn targets(&self, project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
        vec![
            project_target(
                project_path,
                ".pi/settings.json",
                AgentId::PiAgent,
                EvidenceKind::AgentConfig,
                EvidenceParser::Json,
                ScanTargetOverrides::default(),
            ),
            project_target(
                project_path,
                ".pi/themes",
                AgentId::PiAgent,
                EvidenceKind::Unsupported,
                EvidenceParser::Filesystem,
                ScanTargetOverrides {
                    directory: Some(true),
                    sensitivity: Some("themes".to_string()),
                    ..Default::default()
                },
            ),
            project_target(
                project_path,
                ".pi/prompts",
                AgentId::PiAgent,
                EvidenceKind::AgentInstruction,
                EvidenceParser::Filesystem,
                ScanTargetOverrides {
                    directory: Some(true),
                    sensitivity: Some("prompt_templates".to_string()),
                    ..Default::default()
                },
            ),
            home_target(
                home_dir,
                ".pi/agent/settings.json",
                AgentId::PiAgent,
                EvidenceKind::AgentConfig,
                EvidenceParser::Json,
                ScanTargetOverrides::default(),
            ),
            home_target(
                home_dir,
                ".pi/agent/models.json",
                AgentId::PiAgent,
                EvidenceKind::AgentConfig,
                EvidenceParser::Json,
                ScanTargetOverrides {
                    metadata_only: Some(true),
                    sensitivity: Some("model_registry".to_string()),
                    ..Default::default()
                },
            ),
            home_target(
                home_dir,
                ".pi/agents",
                AgentId::PiAgent,
                EvidenceKind::Unsupported,
                EvidenceParser::Filesystem,
                ScanTargetOverrides {
                    directory: Some(true),
                    sensitivity: Some("custom_agents".to_string()),
                    ..Default::default()
                },
            ),
            home_target(
                home_dir,
                ".pi/agent/themes",
                AgentId::PiAgent,
                EvidenceKind::Unsupported,
                EvidenceParser::Filesystem,
                ScanTargetOverrides {
                    directory: Some(true),
                    sensitivity: Some("themes".to_string()),
                    ..Default::default()
                },
            ),
            home_target(
                home_dir,
                ".pi/agent/prompts",
                AgentId::PiAgent,
                EvidenceKind::AgentInstruction,
                EvidenceParser::Filesystem,
                ScanTargetOverrides {
                    directory: Some(true),
                    sensitivity: Some("prompt_templates".to_string()),
                    ..Default::default()
                },
            ),
        ]
    }

    fn scan(&self, context: &ScannerContext) -> Option<Vec<DiscoveredItem>> {
        let config_evidence = scan_targets(&self.targets(
            &context.project_path,
            &context.home_dir,
        ));
        let mut extension_evidence = Vec::new();
        let mut skill_evidence = Vec::new();

        for target in pi_extension_targets(&context.project_path, &context.home_dir) {
            extension_evidence.extend(scan_pi_extension_target(&target));
        }

        for target in pi_skill_targets(&context.project_path, &context.home_dir) {
            skill_evidence.extend(scan_pi_skill_target(&target));
        }

        let mut evidence = config_evidence;
        evidence.extend(dedupe_pi_extensions(extension_evidence));
        evidence.extend(dedupe_pi_skills(skill_evidence));
        Some(evidence)
    }
}

pub fn pi_agent_scanner() -> PiAgentScanner {
    PiAgentScanner
}

fn pi_extension_targets(project_path: &Path, home_dir: &Path) -> Vec<PiExtensionTarget> {
    let mut targets = vec![
        make_pi_extension_target(home_dir, ".pi/agent/extensions", EvidenceScope::User, 10, "auto"),
        make_pi_extension_target(project_path, ".pi/extensions", EvidenceScope::Project, 40, "auto"),
    ];
    targets.extend(configured_extension_targets(project_path, home_dir));
    targets.extend(package_extension_targets(project_path, home_dir));
    targets
}

fn make_pi_extension_target(
    root: &Path,
    relative_path: &str,
    scope: EvidenceScope,
    precedence: u32,
    source: &str,
) -> PiExtensionTarget {
    PiExtensionTarget {
        absolute_path: root.join(relative_path),
        source_path: if scope == EvidenceScope::User {
            format!("~/{relative_path}")
        } else {
            relative_path.to_string()
        },
        scope,
        precedence,
        source: source.to_string(),
    }
}

fn pi_skill_targets(project_path: &Path, home_dir: &Path) -> Vec<PiSkillTarget> {
    let mut targets = vec![
        make_pi_skill_target(home_dir, ".pi/agent/skills", EvidenceScope::User, 10, true, "pi"),
        make_pi_skill_target(project_path, ".pi/skills", EvidenceScope::Project, 40, true, "pi"),
        make_pi_skill_target(home_dir, ".agents/skills", EvidenceScope::User, 15, false, "agents"),
    ];
    targets.extend(ancestor_agent_skill_targets(project_path));
    targets.extend(configured_skill_targets(project_path, home_dir));
    targets.extend(package_skill_targets(project_path, home_dir));
    targets
}

fn make_pi_skill_target(
    root: &Path,
    relative_path: &str,
    scope: EvidenceScope,
    precedence: u32,
    include_root_files: bool,
    source: &str,
) -> PiSkillTarget {
    PiSkillTarget {
        absolute_path: root.join(relative_path),
        source_path: if scope == EvidenceScope::User {
            format!("~/{relative_path}")
        } else {
            relative_path.to_string()
        },
        scope,
        precedence,
        include_root_files,
        source: source.to_string(),
    }
}

fn ancestor_agent_skill_targets(project_path: &Path) -> Vec<PiSkillTarget> {
    let mut targets = Vec::new();
    let repo_root = find_git_repo_root(project_path);
    let mut dir = project_path.canonicalize().unwrap_or_else(|_| project_path.to_path_buf());

    loop {
        let absolute_path = dir.join(".agents").join("skills");
        let source_path = absolute_path
            .strip_prefix(project_path)
            .ok()
            .map(|relative| relative.to_string_lossy().replace('\\', "/"))
            .filter(|value| !value.is_empty())
            .unwrap_or_else(|| ".agents/skills".to_string());

        targets.push(PiSkillTarget {
            absolute_path,
            source_path,
            scope: EvidenceScope::Project,
            precedence: 35,
            include_root_files: false,
            source: "agents".to_string(),
        });

        if repo_root.as_ref() == Some(&dir) || dir.parent().is_none_or(|parent| parent == dir) {
            break;
        }
        dir = dir.parent().unwrap_or(&dir).to_path_buf();
    }

    targets
}

fn find_git_repo_root(start_dir: &Path) -> Option<PathBuf> {
    let mut dir = start_dir.canonicalize().unwrap_or_else(|_| start_dir.to_path_buf());
    loop {
        if dir.join(".git").exists() {
            return Some(dir);
        }
        let parent = dir.parent()?;
        if parent == dir {
            return None;
        }
        dir = parent.to_path_buf();
    }
}

fn configured_extension_targets(project_path: &Path, home_dir: &Path) -> Vec<PiExtensionTarget> {
    let settings = [
        (
            home_dir.join(".pi/agent/settings.json"),
            home_dir.join(".pi/agent"),
            EvidenceScope::User,
            20u32,
        ),
        (
            project_path.join(".pi/settings.json"),
            project_path.join(".pi"),
            EvidenceScope::Project,
            50u32,
        ),
    ];
    let mut targets = Vec::new();
    for (path, base_dir, scope, precedence) in settings {
        let Some(value) = read_json_object(&path) else {
            continue;
        };
        for raw_path in array_of_strings(value.get("extensions")) {
            let absolute_path = resolve_configured_path(&raw_path, &base_dir, home_dir);
            let source_path = display_path(&absolute_path, home_dir, project_path);
            targets.push(PiExtensionTarget {
                absolute_path,
                source_path,
                scope,
                precedence,
                source: "settings".to_string(),
            });
        }
    }
    targets
}

fn configured_skill_targets(project_path: &Path, home_dir: &Path) -> Vec<PiSkillTarget> {
    let settings = [
        (
            home_dir.join(".pi/agent/settings.json"),
            home_dir.join(".pi/agent"),
            EvidenceScope::User,
            20u32,
        ),
        (
            project_path.join(".pi/settings.json"),
            project_path.join(".pi"),
            EvidenceScope::Project,
            50u32,
        ),
    ];
    let mut targets = Vec::new();
    for (path, base_dir, scope, precedence) in settings {
        let Some(value) = read_json_object(&path) else {
            continue;
        };
        for raw_path in array_of_strings(value.get("skills")) {
            let absolute_path = resolve_configured_path(&raw_path, &base_dir, home_dir);
            let source_path = display_path(&absolute_path, home_dir, project_path);
            targets.push(PiSkillTarget {
                absolute_path,
                source_path,
                scope,
                precedence,
                include_root_files: true,
                source: "settings".to_string(),
            });
        }
    }
    targets
}

fn package_extension_targets(project_path: &Path, home_dir: &Path) -> Vec<PiExtensionTarget> {
    let settings = [
        (
            home_dir.join(".pi/agent/settings.json"),
            EvidenceScope::User,
            25u32,
        ),
        (
            project_path.join(".pi/settings.json"),
            EvidenceScope::Project,
            55u32,
        ),
    ];
    let mut targets = Vec::new();
    for (path, scope, precedence) in settings {
        let Some(value) = read_json_object(&path) else {
            continue;
        };
        for spec in array_of_strings(value.get("packages")) {
            let Some(package_root) = resolve_package_root(&spec) else {
                continue;
            };
            let Some(package_json) = read_json_object(&package_root.join("package.json")) else {
                continue;
            };
            let pi_config = package_json
                .get("pi")
                .filter(|value| is_object(value))
                .and_then(as_object)
                .cloned()
                .unwrap_or_default();
            let extension_paths = array_of_strings(pi_config.get("extensions"));
            let raw_paths = if extension_paths.is_empty() {
                vec!["extensions".to_string()]
            } else {
                extension_paths
            };
            for raw_path in raw_paths {
                let absolute_path = resolve_configured_path(&raw_path, &package_root, home_dir);
                let source_path = display_path(&absolute_path, home_dir, project_path);
                targets.push(PiExtensionTarget {
                    absolute_path,
                    source_path,
                    scope,
                    precedence,
                    source: "package".to_string(),
                });
            }
        }
    }
    targets
}

fn package_skill_targets(project_path: &Path, home_dir: &Path) -> Vec<PiSkillTarget> {
    let settings = [
        (
            home_dir.join(".pi/agent/settings.json"),
            EvidenceScope::User,
            25u32,
        ),
        (
            project_path.join(".pi/settings.json"),
            EvidenceScope::Project,
            55u32,
        ),
    ];
    let mut targets = Vec::new();
    for (path, scope, precedence) in settings {
        let Some(value) = read_json_object(&path) else {
            continue;
        };
        for spec in array_of_strings(value.get("packages")) {
            let Some(package_root) = resolve_package_root(&spec) else {
                continue;
            };
            let Some(package_json) = read_json_object(&package_root.join("package.json")) else {
                continue;
            };
            let pi_config = package_json
                .get("pi")
                .filter(|value| is_object(value))
                .and_then(as_object)
                .cloned()
                .unwrap_or_default();
            let skill_paths = array_of_strings(pi_config.get("skills"));
            let raw_paths = if skill_paths.is_empty() {
                vec!["skills".to_string()]
            } else {
                skill_paths
            };
            for raw_path in raw_paths {
                let absolute_path = resolve_configured_path(&raw_path, &package_root, home_dir);
                let source_path = display_path(&absolute_path, home_dir, project_path);
                targets.push(PiSkillTarget {
                    absolute_path,
                    source_path,
                    scope,
                    precedence,
                    include_root_files: true,
                    source: "package".to_string(),
                });
            }
        }
    }
    targets
}

fn resolve_package_root(spec: &str) -> Option<PathBuf> {
    let package_name = package_name_from_spec(spec)?;
    for root in node_module_roots() {
        let package_root = root.join(&package_name);
        if package_root.join("package.json").exists() {
            return Some(package_root);
        }
    }
    None
}

fn package_name_from_spec(spec: &str) -> Option<String> {
    let mut value = spec.to_string();
    if let Some(stripped) = value.strip_prefix("npm:") {
        value = stripped.to_string();
    }
    if let Some(stripped) = value.strip_prefix('@') {
        let mut parts = stripped.split('/');
        let scope = parts.next()?;
        let name = parts.next()?;
        return Some(format!("@{scope}/{}", name.split('@').next()?));
    }
    Some(value.split('@').next()?.to_string())
}

fn node_module_roots() -> Vec<PathBuf> {
    let mut roots = Vec::new();
    if let Ok(exec_path) = env::current_exe() {
        if let Some(parent) = exec_path.parent() {
            roots.push(parent.join("..").join("lib").join("node_modules"));
        }
    }
    roots.push(PathBuf::from("/opt/homebrew/lib/node_modules"));
    roots.push(PathBuf::from("/usr/local/lib/node_modules"));

    let mut deduped = HashSet::new();
    roots
        .into_iter()
        .filter_map(|root| root.canonicalize().ok().or(Some(root)))
        .filter(|root| deduped.insert(root.clone()))
        .collect()
}

fn scan_pi_extension_target(target: &PiExtensionTarget) -> Vec<DiscoveredItem> {
    let extension_files = find_pi_extension_files(&target.absolute_path);
    let mut evidence = Vec::new();

    for extension_file in extension_files {
        let Ok(metadata) = fs::metadata(&extension_file.file_path) else {
            continue;
        };
        let real_file_path = fs::canonicalize(&extension_file.file_path).ok();

        let source_path = display_extension_source_path(target, &extension_file);
        let entrypoint = dirname_basename(&extension_file.file_path);
        let is_index_entrypoint = entrypoint == "index.ts" || entrypoint == "index.js";

        evidence.push(DiscoveredItem {
            id: scanner_item_id(target.scope, AgentId::PiAgent, &source_path, "extension"),
            agent: AgentId::PiAgent,
            kind: EvidenceKind::Extension,
            source_path,
            scope: target.scope,
            precedence: target.precedence,
            parser: EvidenceParser::Filesystem,
            sensitivity: "command_config".to_string(),
            content_policy: "metadata_only".to_string(),
            restore_policy: RestorePolicy::FullContentSupported,
            capture_status: CaptureStatus::Captured,
            confidence: EvidenceConfidence::High,
            name: Some(extension_name_from_path(
                &extension_file.file_path,
                &extension_file.root,
            )),
            value: None,
            checksum: None,
            metadata: Some(json!({
                "present": true,
                "source": target.source,
                "executable": true,
                "entrypoint": entrypoint,
                "extensionStyle": if is_index_entrypoint { "directory_index" } else { "single_file" },
                "sizeBytes": metadata.len(),
                "realPath": real_file_path.map(|path| path.to_string_lossy().to_string()),
            })),
        });
    }

    evidence
}

fn find_pi_extension_files(root: &Path) -> Vec<PiExtensionFile> {
    let metadata = match fs::metadata(root) {
        Ok(metadata) => metadata,
        Err(_) => return Vec::new(),
    };
    if metadata.is_file() {
        return if is_extension_file(root) {
            vec![PiExtensionFile {
                file_path: root.to_path_buf(),
                root: root.to_path_buf(),
            }]
        } else {
            Vec::new()
        };
    }
    if !metadata.is_dir() {
        return Vec::new();
    }
    collect_pi_extension_entries(root, root)
}

fn collect_pi_extension_entries(dir: &Path, root: &Path) -> Vec<PiExtensionFile> {
    let manifest_entries = resolve_pi_extension_entries(dir, root);
    if !manifest_entries.is_empty() {
        return manifest_entries;
    }

    let entries = match fs::read_dir(dir) {
        Ok(entries) => entries,
        Err(_) => return Vec::new(),
    };

    let mut discovered = Vec::new();
    for entry in entries.flatten() {
        let name = entry.file_name().to_string_lossy().to_string();
        if name.starts_with('.') || name == "node_modules" {
            continue;
        }
        let entry_path = entry.path();
        let metadata = match fs::metadata(&entry_path) {
            Ok(metadata) => metadata,
            Err(_) => continue,
        };
        if metadata.is_file() && is_extension_file(&entry_path) {
            discovered.push(PiExtensionFile {
                file_path: entry_path,
                root: root.to_path_buf(),
            });
        } else if metadata.is_dir() {
            discovered.extend(resolve_pi_extension_entries(&entry_path, root));
        }
    }
    discovered
}

fn resolve_pi_extension_entries(dir: &Path, root: &Path) -> Vec<PiExtensionFile> {
    let package_json = read_json_object(&dir.join("package.json"));
    let pi_config = package_json
        .as_ref()
        .and_then(|value| value.get("pi"))
        .filter(|value| is_object(value))
        .and_then(as_object)
        .cloned()
        .unwrap_or_default();
    let manifest_extensions = array_of_strings(pi_config.get("extensions"));
    let mut files = Vec::new();
    for raw_path in manifest_extensions {
        let resolved_path = dir.join(raw_path);
        files.extend(find_pi_extension_files_from_manifest_path(&resolved_path, root));
    }
    if !files.is_empty() {
        return files;
    }

    for index_file in [dir.join("index.ts"), dir.join("index.js")] {
        if fs::metadata(&index_file).map(|meta| meta.is_file()).unwrap_or(false) {
            return vec![PiExtensionFile {
                file_path: index_file,
                root: root.to_path_buf(),
            }];
        }
    }
    Vec::new()
}

fn find_pi_extension_files_from_manifest_path(
    absolute_path: &Path,
    root: &Path,
) -> Vec<PiExtensionFile> {
    let metadata = match fs::metadata(absolute_path) {
        Ok(metadata) => metadata,
        Err(_) => return Vec::new(),
    };
    if metadata.is_file() {
        return if is_extension_file(absolute_path) {
            vec![PiExtensionFile {
                file_path: absolute_path.to_path_buf(),
                root: root.to_path_buf(),
            }]
        } else {
            Vec::new()
        };
    }
    if metadata.is_dir() {
        return collect_pi_extension_entries(absolute_path, root);
    }
    Vec::new()
}

fn is_extension_file(file_path: &Path) -> bool {
    file_path
        .extension()
        .and_then(|ext| ext.to_str())
        .is_some_and(|ext| ext == "ts" || ext == "js")
}

fn extension_name_from_path(file_path: &Path, root: &Path) -> String {
    let entrypoint = dirname_basename(file_path);
    if entrypoint == "index.ts" || entrypoint == "index.js" {
        if root.join("package.json").exists() {
            return dirname_basename(root);
        }
        return file_path
            .parent()
            .map(dirname_basename)
            .unwrap_or(entrypoint);
    }
    entrypoint
        .strip_suffix(".ts")
        .or_else(|| entrypoint.strip_suffix(".js"))
        .unwrap_or(&entrypoint)
        .to_string()
}

fn scan_pi_skill_target(target: &PiSkillTarget) -> Vec<DiscoveredItem> {
    let skill_files = find_pi_skill_files(&target.absolute_path, target.include_root_files);
    let mut evidence = Vec::new();

    for skill_file in skill_files {
        let Some(frontmatter) = read_skill_frontmatter(&skill_file.file_path) else {
            continue;
        };
        if frontmatter.description.as_ref().is_none_or(|value| value.trim().is_empty()) {
            continue;
        }
        let description = frontmatter.description.clone().unwrap_or_default();
        let name = frontmatter
            .name
            .clone()
            .unwrap_or_else(|| dirname_basename(&skill_file.skill_dir));
        let source_path = display_skill_source_path(target, &skill_file);
        let entrypoint_status =
            skill_entrypoint_status(&target.absolute_path, &skill_file.file_path);

        evidence.push(DiscoveredItem {
            id: scanner_item_id(target.scope, AgentId::PiAgent, &source_path, "skill"),
            agent: AgentId::PiAgent,
            kind: EvidenceKind::Skill,
            source_path,
            scope: target.scope,
            precedence: target.precedence,
            parser: EvidenceParser::Filesystem,
            sensitivity: "metadata".to_string(),
            content_policy: "metadata_only".to_string(),
            restore_policy: RestorePolicy::FullContentSupported,
            capture_status: CaptureStatus::Captured,
            confidence: EvidenceConfidence::High,
            name: Some(name.clone()),
            value: None,
            checksum: None,
            metadata: Some(json!({
                "present": true,
                "source": target.source,
                "entrypoint": if skill_file.file_path.to_string_lossy().ends_with("/SKILL.md") {
                    json!("SKILL.md")
                } else {
                    json!(dirname_basename(&skill_file.file_path))
                },
                "entrypointStatus": entrypoint_status,
                "entrypointSizeBytes": frontmatter.size_bytes,
                "declaredName": frontmatter.name,
                "directoryName": dirname_basename(&skill_file.skill_dir),
                "nameMatchesDirectory": name == dirname_basename(&skill_file.skill_dir),
                "description": description,
                "disableModelInvocation": frontmatter.disable_model_invocation == Some(true),
            })),
        });
    }

    evidence
}

fn find_pi_skill_files(root: &Path, include_root_files: bool) -> Vec<PiSkillFile> {
    let mut files = Vec::new();
    walk_pi_skill_files(root, root, include_root_files, &mut files, &mut HashSet::new());
    files
}

fn walk_pi_skill_files(
    dir: &Path,
    root: &Path,
    include_root_files: bool,
    files: &mut Vec<PiSkillFile>,
    seen: &mut HashSet<String>,
) {
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
    let entry_names: Vec<_> = entries
        .flatten()
        .map(|entry| (entry.file_name(), entry.path()))
        .collect();

    if entry_names.iter().any(|(name, _)| name == "SKILL.md") {
        let file_path = dir.join("SKILL.md");
        if fs::metadata(&file_path).map(|meta| meta.is_file()).unwrap_or(false) {
            files.push(PiSkillFile {
                file_path,
                skill_dir: dir.to_path_buf(),
                root: root.to_path_buf(),
            });
        }
        return;
    }

    for (name, file_path) in entry_names {
        let name = name.to_string_lossy();
        if name.starts_with('.') || name == "node_modules" {
            continue;
        }
        let metadata = match fs::metadata(&file_path) {
            Ok(metadata) => metadata,
            Err(_) => continue,
        };
        if metadata.is_dir() {
            walk_pi_skill_files(&file_path, root, false, files, seen);
            continue;
        }
        if include_root_files && metadata.is_file() && name.ends_with(".md") {
            files.push(PiSkillFile {
                file_path: file_path.clone(),
                skill_dir: file_path.parent().unwrap_or(dir).to_path_buf(),
                root: root.to_path_buf(),
            });
        }
    }
}

fn read_skill_frontmatter(file_path: &Path) -> Option<PiFrontmatter> {
    let metadata = fs::metadata(file_path).ok()?;
    let text = fs::read_to_string(file_path).ok()?;
    let captures = Regex::new(r"^---\n([\s\S]*?)\n---").ok()?.captures(&text)?;
    let body = captures.get(1)?.as_str();
    let mut frontmatter = PiFrontmatter {
        name: None,
        description: None,
        disable_model_invocation: None,
        size_bytes: metadata.len(),
    };
    let field_re = Regex::new(r"^(name|description|disable-model-invocation):\s*(.*)$").ok()?;
    for line in body.split('\n') {
        if let Some(caps) = field_re.captures(line.trim()) {
            let key = caps.get(1)?.as_str();
            let value = unquote_yaml_scalar(caps.get(2)?.as_str());
            match key {
                "disable-model-invocation" => {
                    frontmatter.disable_model_invocation = Some(value == "true");
                }
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
        .map(|path| {
            path.components()
                .filter(|component| !matches!(component, Component::RootDir | Component::Prefix(_)))
                .map(|component| component.as_os_str().to_string_lossy().to_string())
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();
    let mut cursor = root.to_path_buf();
    for part in relative_parts {
        cursor.push(&part);
        match fs::symlink_metadata(&cursor) {
            Ok(metadata) if metadata.file_type().is_symlink() => {
                return if part == "SKILL.md" {
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

fn dedupe_pi_extensions(evidence: Vec<DiscoveredItem>) -> Vec<DiscoveredItem> {
    let mut result = Vec::new();
    let mut real_paths = HashSet::new();
    for item in evidence {
        let real_path = item
            .metadata
            .as_ref()
            .and_then(|metadata| metadata.get("realPath"))
            .and_then(|value| value.as_str());
        if let Some(real_path) = real_path {
            if !real_paths.insert(real_path.to_string()) {
                continue;
            }
        }
        result.push(item);
    }
    result
}

fn dedupe_pi_skills(evidence: Vec<DiscoveredItem>) -> Vec<DiscoveredItem> {
    let mut result = Vec::new();
    let mut skill_indexes: HashMap<String, usize> = HashMap::new();
    let mut real_paths = HashSet::new();

    for item in evidence {
        let real_path = item
            .metadata
            .as_ref()
            .and_then(|metadata| metadata.get("realPath"))
            .and_then(|value| value.as_str());
        if let Some(real_path) = real_path {
            if real_paths.contains(real_path) {
                continue;
            }
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
            if let Some(real_path) = real_path {
                real_paths.insert(real_path.to_string());
            }
            result.push(item);
        }
    }

    result
}

fn display_extension_source_path(target: &PiExtensionTarget, extension_file: &PiExtensionFile) -> String {
    let relative_path = extension_file
        .file_path
        .strip_prefix(&target.absolute_path)
        .map(|path| path.to_string_lossy().replace('\\', "/"))
        .unwrap_or_default();
    if relative_path.is_empty() {
        target.source_path.clone()
    } else {
        format!("{}/{}", target.source_path, relative_path)
    }
}

fn display_skill_source_path(target: &PiSkillTarget, skill_file: &PiSkillFile) -> String {
    let relative_path = skill_file
        .file_path
        .strip_prefix(&target.absolute_path)
        .map(|path| path.to_string_lossy().replace('\\', "/"))
        .unwrap_or_default();
    if relative_path.is_empty() || relative_path == "SKILL.md" {
        return target.source_path.clone();
    }
    if let Some(stripped) = relative_path.strip_suffix("/SKILL.md") {
        return format!("{}/{}", target.source_path, stripped);
    }
    format!("{}/{}", target.source_path, relative_path)
}

fn read_json_object(file_path: &Path) -> Option<Map<String, Value>> {
    let text = fs::read_to_string(file_path).ok()?;
    let value: Value = serde_json::from_str(&text).ok()?;
    value.as_object().cloned()
}

fn resolve_configured_path(raw_path: &str, base_dir: &Path, home_dir: &Path) -> PathBuf {
    if raw_path == "~" {
        return home_dir.to_path_buf();
    }
    if let Some(stripped) = raw_path.strip_prefix("~/") {
        return home_dir.join(stripped);
    }
    if Path::new(raw_path).is_absolute() {
        return PathBuf::from(raw_path);
    }
    base_dir.join(raw_path)
}

fn display_path(absolute_path: &Path, home_dir: &Path, project_path: &Path) -> String {
    let resolved = absolute_path
        .canonicalize()
        .unwrap_or_else(|_| absolute_path.to_path_buf());
    let resolved_home = home_dir
        .canonicalize()
        .unwrap_or_else(|_| home_dir.to_path_buf());
    let resolved_project = project_path
        .canonicalize()
        .unwrap_or_else(|_| project_path.to_path_buf());

    if resolved == resolved_home
        || resolved
            .strip_prefix(&resolved_home)
            .is_ok_and(|_| resolved.starts_with(&resolved_home))
    {
        return format!(
            "~/{}",
            resolved
                .strip_prefix(&resolved_home)
                .map(|path| path.to_string_lossy().replace('\\', "/"))
                .unwrap_or_default()
        );
    }
    if resolved == resolved_project {
        return ".".to_string();
    }
    if let Ok(relative) = resolved.strip_prefix(&resolved_project) {
        return relative.to_string_lossy().replace('\\', "/");
    }
    resolved.to_string_lossy().replace('\\', "/")
}

fn dirname_basename(file_path: &Path) -> String {
    file_path
        .file_name()
        .map(|name| name.to_string_lossy().to_string())
        .unwrap_or_else(|| file_path.to_string_lossy().to_string())
}