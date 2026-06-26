use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{Path, PathBuf};

use regex::Regex;
use serde_json::{json, Map, Value};

use crate::parsers::parse_json;
use crate::policy::ignored_directory;
use crate::types::{
    AgentId, CaptureStatus, DiscoveredItem, EvidenceConfidence, EvidenceKind, EvidenceParser,
    EvidenceScope, RestorePolicy,
};

use super::super::base::{
    as_object, metadata_string_array, normalize_source_path, scanner_item_id, unquote_yaml_scalar,
    EvidenceBaseTarget, ItemIdTarget, ScannerBase,
};
use super::super::{home_target, project_target, ScanTarget, ScanTargetOverrides};
use super::{ScannerContext, ScannerPlugin};

const MAX_SKILL_DEPTH: u32 = 8;

pub struct CursorScanner;

impl ScannerPlugin for CursorScanner {
    fn agent_id(&self) -> AgentId {
        AgentId::Cursor
    }

    fn agent_name(&self) -> &'static str {
        "Cursor"
    }

    fn description(&self) -> &'static str {
        "Cursor editor configuration (MCP servers, skills, hooks)"
    }

    fn targets(&self, project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
        cursor_mcp_targets(project_path, home_dir)
    }

    fn scan(&self, context: &ScannerContext) -> Option<Vec<DiscoveredItem>> {
        let base = ScannerBase::new(AgentId::Cursor);
        let mcp_evidence = scan_cursor_mcp_servers(
            &context.project_path,
            &context.home_dir,
            &base,
        );
        let hook_evidence = scan_cursor_hooks(
            &context.project_path,
            &context.home_dir,
            &base,
        );
        let mut skill_evidence = Vec::new();
        for target in cursor_skill_targets(&context.project_path, &context.home_dir) {
            skill_evidence.extend(scan_cursor_skill_directory(&target));
        }

        let cursor_evidence = if mcp_evidence.is_empty()
            && skill_evidence.is_empty()
            && hook_evidence.is_empty()
        {
            Vec::new()
        } else {
            let mut evidence = mcp_evidence;
            evidence.extend(dedupe_skills_by_name(skill_evidence));
            evidence.extend(hook_evidence);
            evidence.push(cursor_team_hooks_blind_spot());
            evidence
        };

        Some(cursor_evidence)
    }
}

pub fn cursor_scanner() -> CursorScanner {
    CursorScanner
}

fn cursor_mcp_targets(project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
    let overrides = ScanTargetOverrides {
        sensitivity: Some("command_config".to_string()),
        content_policy: Some("structured_safe_fields_only".to_string()),
        ..Default::default()
    };
    vec![
        project_target(
            project_path,
            ".cursor/mcp.json",
            AgentId::Cursor,
            EvidenceKind::AgentConfig,
            EvidenceParser::Json,
            overrides.clone(),
        ),
        home_target(
            home_dir,
            ".cursor/mcp.json",
            AgentId::Cursor,
            EvidenceKind::AgentConfig,
            EvidenceParser::Json,
            overrides,
        ),
    ]
}

fn scan_cursor_mcp_servers(
    project_path: &Path,
    home_dir: &Path,
    base: &ScannerBase,
) -> Vec<DiscoveredItem> {
    let mut evidence = Vec::new();
    for target in cursor_mcp_targets(project_path, home_dir) {
        evidence.extend(scan_cursor_mcp_file(&target, base));
    }
    evidence
}

fn scan_cursor_mcp_file(target: &ScanTarget, base: &ScannerBase) -> Vec<DiscoveredItem> {
    let text = match fs::read_to_string(&target.absolute_path) {
        Ok(text) => text,
        Err(_) => return Vec::new(),
    };

    let parsed = parse_json(&text);
    let crate::parsers::ParseResult::Err(failure) = parsed else {
        let crate::parsers::ParseResult::Ok(success) = parsed else {
            return Vec::new();
        };
        let value = success.value;
        let servers = as_object(&value)
            .and_then(|root| root.get("mcpServers"))
            .and_then(as_object);
        if servers.is_none() {
            return vec![base.captured(
                &EvidenceBaseTarget::from(target),
                EvidenceKind::AgentConfig,
                None,
                Some(value),
            )];
        }

        return servers
            .expect("checked above")
            .iter()
            .map(|(name, server_value)| {
                let server_record = as_object(server_value).cloned().unwrap_or_default();
                let server_value = sanitize_mcp_server(&server_record);
                let transport = transport_for_mcp_server(&server_value);
                let remote = transport != "stdio"
                    && server_value
                        .get("url")
                        .and_then(|v| v.as_str())
                        .is_some();
                let mut metadata = json!({
                    "transport": transport,
                    "remote": remote,
                    "source": target.scope.as_str(),
                    "authConfigured": server_value.get("auth").is_some(),
                    "interpolationFields": interpolation_fields_for_mcp_server(&server_value),
                });
                if let Some(env_file) = server_value.get("envFile") {
                    if let Some(obj) = metadata.as_object_mut() {
                        obj.insert("envFile".to_string(), env_file.clone());
                    }
                }

                let mut item = base.captured(
                    &EvidenceBaseTarget {
                        sensitivity: "command_config".to_string(),
                        content_policy: "structured_safe_fields_only".to_string(),
                        ..EvidenceBaseTarget::from(target)
                    },
                    EvidenceKind::McpServer,
                    Some(metadata),
                    Some(Value::Object(server_value)),
                );
                item.id = base.item_id(&ItemIdTarget::from(target), &format!("mcp-{name}"));
                item.name = Some(name.clone());
                item
            })
            .collect();
    };

    vec![base.parse_failed(
        &EvidenceBaseTarget::from(target),
        EvidenceKind::AgentConfig,
        &failure.error,
    )]
}

fn sanitize_mcp_server(value: &Map<String, Value>) -> Map<String, Value> {
    let mut sanitized = Map::new();
    for (key, nested_value) in value {
        if key == "url" {
            if let Some(url) = nested_value.as_str() {
                sanitized.insert(key.clone(), json!(redact_url(url)));
                continue;
            }
        }
        sanitized.insert(key.clone(), nested_value.clone());
    }
    sanitized
}

fn transport_for_mcp_server(value: &Map<String, Value>) -> String {
    let type_value = value
        .get("type")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_ascii_lowercase();
    if type_value == "stdio" {
        return "stdio".to_string();
    }
    if type_value == "sse" {
        return "sse".to_string();
    }
    if matches!(
        type_value.as_str(),
        "streamable-http" | "streamable_http" | "http"
    ) {
        return "streamable-http".to_string();
    }
    if value.contains_key("command") {
        return "stdio".to_string();
    }
    if value.contains_key("url") {
        return "streamable-http".to_string();
    }
    "unknown".to_string()
}

fn interpolation_fields_for_mcp_server(value: &Map<String, Value>) -> Vec<String> {
    ["command", "args", "env", "url", "headers"]
        .into_iter()
        .filter(|field| {
            value
                .get(*field)
                .is_some_and(|nested| contains_interpolation(nested))
        })
        .map(str::to_string)
        .collect()
}

fn contains_interpolation(value: &Value) -> bool {
    static PATTERN: std::sync::OnceLock<Regex> = std::sync::OnceLock::new();
    let pattern = PATTERN.get_or_init(|| {
        Regex::new(r"\$\{(?:env:[^}]+|userHome|workspaceFolder|workspaceFolderBasename|pathSeparator|/)\}")
            .expect("valid interpolation regex")
    });

    match value {
        Value::String(text) => pattern.is_match(text),
        Value::Array(items) => items.iter().any(contains_interpolation),
        Value::Object(map) => map.values().any(contains_interpolation),
        _ => false,
    }
}

fn redact_url(value: &str) -> String {
    static AUTH: std::sync::OnceLock<Regex> = std::sync::OnceLock::new();
    static QUERY: std::sync::OnceLock<Regex> = std::sync::OnceLock::new();
    let auth = AUTH.get_or_init(|| {
        Regex::new(r"//([^/@:]+)(?::[^/@]*)?@").expect("valid url auth regex")
    });
    let query = QUERY.get_or_init(|| {
        Regex::new(r"([?&][^=]+)=([^&]+)").expect("valid url fallback regex")
    });
    let without_auth = auth.replace(value, "//[redacted]:[redacted]@");
    query.replace_all(&without_auth, "$1=[redacted]").to_string()
}

fn cursor_skill_targets(project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
    let directory = ScanTargetOverrides {
        directory: Some(true),
        ..Default::default()
    };
    let explicit_targets = vec![
        project_target(
            project_path,
            ".cursor/skills",
            AgentId::Cursor,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        project_target(
            project_path,
            ".agents/skills",
            AgentId::Cursor,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        project_target(
            project_path,
            ".claude/skills",
            AgentId::Cursor,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        project_target(
            project_path,
            ".codex/skills",
            AgentId::Cursor,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".cursor/skills",
            AgentId::Cursor,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".agents/skills",
            AgentId::Cursor,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".claude/skills",
            AgentId::Cursor,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory.clone(),
        ),
        home_target(
            home_dir,
            ".codex/skills",
            AgentId::Cursor,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            directory,
        ),
    ];

    let mut targets = HashMap::new();
    for target in explicit_targets
        .into_iter()
        .chain(nested_cursor_skill_targets(project_path))
    {
        targets.insert(target.source_path.clone(), target);
    }
    targets.into_values().collect()
}

fn nested_cursor_skill_targets(project_path: &Path) -> Vec<ScanTarget> {
    let mut roots = Vec::new();
    walk_for_nested_skill_roots(project_path, project_path, &mut roots, 0);
    roots
}

fn walk_for_nested_skill_roots(
    project_path: &Path,
    absolute_dir: &Path,
    targets: &mut Vec<ScanTarget>,
    depth: u32,
) {
    if depth > MAX_SKILL_DEPTH {
        return;
    }

    let entries = match fs::read_dir(absolute_dir) {
        Ok(entries) => entries,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let file_name = entry.file_name().to_string_lossy().to_string();
        let file_type = match entry.file_type() {
            Ok(file_type) => file_type,
            Err(_) => continue,
        };
        if !file_type.is_dir() || ignored_directory(&file_name) {
            continue;
        }

        let absolute_path = entry.path();
        if fs::symlink_metadata(&absolute_path)
            .map(|meta| meta.file_type().is_symlink())
            .unwrap_or(true)
        {
            continue;
        }

        if file_name == ".cursor" || file_name == ".agents" {
            let skills_path = absolute_path.join("skills");
            if fs::symlink_metadata(&skills_path)
                .map(|meta| meta.is_dir())
                .unwrap_or(false)
            {
                targets.push(ScanTarget {
                    absolute_path: skills_path.clone(),
                    source_path: normalize_source_path(project_path, &skills_path),
                    scope: EvidenceScope::Project,
                    precedence: 40,
                    agent: AgentId::Cursor,
                    kind: EvidenceKind::Skill,
                    parser: EvidenceParser::Filesystem,
                    sensitivity: "metadata".to_string(),
                    content_policy: "metadata_only".to_string(),
                    directory: true,
                    metadata_only: false,
                });
            }
        }

        walk_for_nested_skill_roots(project_path, &absolute_path, targets, depth + 1);
    }
}

fn scan_cursor_skill_directory(target: &ScanTarget) -> Vec<DiscoveredItem> {
    let skill_files = find_skill_files(&target.absolute_path);
    let mut evidence = Vec::new();
    let scope_root = scope_root_for_skill_target(target);

    for skill_file in skill_files {
        let Some(frontmatter) = read_skill_frontmatter(&skill_file) else {
            continue;
        };
        let skill_dir = skill_file.parent().unwrap_or(Path::new(""));
        let directory_name = skill_dir
            .file_name()
            .map(|name| name.to_string_lossy().to_string())
            .unwrap_or_default();
        let Some(name) = frontmatter.name else {
            continue;
        };
        let Some(description) = frontmatter.description else {
            continue;
        };
        if !valid_skill_name(&name) || name != directory_name {
            continue;
        }

        let relative_skill_dir = skill_dir
            .strip_prefix(&target.absolute_path)
            .map(|path| path.to_string_lossy().replace('\\', "/"))
            .unwrap_or_default();
        let source_path = if relative_skill_dir.is_empty() {
            target.source_path.clone()
        } else {
            format!("{}/{}", target.source_path, relative_skill_dir)
        };

        let mut metadata = json!({
            "present": true,
            "entrypoint": skill_file.file_name().map(|n| n.to_string_lossy().to_string()).unwrap_or_else(|| "SKILL.md".to_string()),
            "entrypointStatus": "captured",
            "entrypointSizeBytes": frontmatter.size_bytes,
            "declaredName": name,
            "directoryName": directory_name,
            "nameMatchesDirectory": true,
            "description": description,
            "sourceRoot": target.source_path,
            "scopeRoot": scope_root,
        });
        if let Some(paths) = frontmatter.paths {
            if let Some(obj) = metadata.as_object_mut() {
                obj.insert("paths".to_string(), json!(paths));
            }
        }
        if let Some(disable) = frontmatter.disable_model_invocation {
            if let Some(obj) = metadata.as_object_mut() {
                obj.insert("disableModelInvocation".to_string(), json!(disable));
            }
        }
        if let Some(skill_metadata) = frontmatter.metadata {
            if let Some(obj) = metadata.as_object_mut() {
                obj.insert("skillMetadata".to_string(), json!(skill_metadata));
            }
        }

        evidence.push(DiscoveredItem {
            id: scanner_item_id(
                target.scope,
                AgentId::Cursor,
                &source_path,
                "skill",
            ),
            agent: AgentId::Cursor,
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
            name: Some(name),
            value: None,
            checksum: None,
            metadata: Some(metadata),
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
        let metadata = match fs::symlink_metadata(&absolute_path) {
            Ok(metadata) => metadata,
            Err(_) => continue,
        };
        if metadata.file_type().is_symlink() {
            continue;
        }
        if metadata.is_dir() {
            let name = entry.file_name().to_string_lossy().to_string();
            if !ignored_directory(&name) {
                walk_skill_files(&absolute_path, files, depth + 1, seen);
            }
            continue;
        }
        if metadata.is_file() && entry.file_name() == "SKILL.md" {
            files.push(absolute_path);
        }
    }
}

struct CursorSkillFrontmatter {
    name: Option<String>,
    description: Option<String>,
    paths: Option<Vec<String>>,
    disable_model_invocation: Option<bool>,
    metadata: Option<Map<String, Value>>,
    size_bytes: u64,
}

fn read_skill_frontmatter(file_path: &Path) -> Option<CursorSkillFrontmatter> {
    let metadata = fs::metadata(file_path).ok()?;
    let text = fs::read_to_string(file_path).ok()?;
    let captures = Regex::new(r"^---\r?\n([\s\S]*?)\r?\n---")
        .ok()?
        .captures(&text);
    let Some(captures) = captures else {
        return Some(CursorSkillFrontmatter {
            name: None,
            description: None,
            paths: None,
            disable_model_invocation: None,
            metadata: None,
            size_bytes: metadata.len(),
        });
    };

    let body = captures.get(1)?.as_str();
    let lines: Vec<&str> = body.split("\n").collect();
    let mut frontmatter = CursorSkillFrontmatter {
        name: None,
        description: None,
        paths: None,
        disable_model_invocation: None,
        metadata: None,
        size_bytes: metadata.len(),
    };

    let scalar_re = Regex::new(r"^(name|description|disable-model-invocation):\s*(.*)$").ok()?;
    let metadata_re = Regex::new(r"^\s+([A-Za-z0-9_.-]+):\s*(.*)$").ok()?;
    let mut index = 0usize;
    while index < lines.len() {
        let line = lines[index].trim();
        if let Some(caps) = scalar_re.captures(line) {
            let key = caps.get(1)?.as_str();
            let value = unquote_yaml_scalar(caps.get(2)?.as_str());
            match key {
                "name" => frontmatter.name = Some(value),
                "description" => frontmatter.description = Some(value),
                "disable-model-invocation" => {
                    frontmatter.disable_model_invocation = Some(value == "true");
                }
                _ => {}
            }
            index += 1;
            continue;
        }
        if line == "paths:" {
            let mut values = Vec::new();
            while index + 1 < lines.len() && lines[index + 1].trim().starts_with("- ") {
                index += 1;
                values.push(unquote_yaml_scalar(
                    lines[index].trim().trim_start_matches("- "),
                ));
            }
            frontmatter.paths = Some(values);
            index += 1;
            continue;
        }
        if line == "metadata:" {
            let mut metadata_map = Map::new();
            while index + 1 < lines.len() && lines[index + 1].starts_with("  ") {
                index += 1;
                if let Some(caps) = metadata_re.captures(lines[index]) {
                    metadata_map.insert(
                        caps.get(1)?.as_str().to_string(),
                        json!(unquote_yaml_scalar(caps.get(2)?.as_str())),
                    );
                }
            }
            frontmatter.metadata = Some(metadata_map);
            index += 1;
            continue;
        }
        index += 1;
    }

    Some(frontmatter)
}

fn scope_root_for_skill_target(target: &ScanTarget) -> Option<String> {
    if target.scope != EvidenceScope::Project {
        return None;
    }
    let marker = Regex::new(r"^(.*?)(?:^|/)(?:\.cursor|\.agents)/skills$")
        .ok()?
        .captures(&target.source_path)?;
    let prefix = marker.get(1)?.as_str();
    if prefix.is_empty() {
        Some(".".to_string())
    } else {
        Some(prefix.to_string())
    }
}

fn valid_skill_name(name: &str) -> bool {
    Regex::new(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
        .map(|re| re.is_match(name))
        .unwrap_or(false)
}

fn cursor_hook_targets(project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
    let overrides = ScanTargetOverrides {
        sensitivity: Some("command_config".to_string()),
        content_policy: Some("structured_safe_fields_only".to_string()),
        ..Default::default()
    };
    let mut targets = vec![
        project_target(
            project_path,
            ".cursor/hooks.json",
            AgentId::Cursor,
            EvidenceKind::Hook,
            EvidenceParser::Json,
            overrides.clone(),
        ),
        home_target(
            home_dir,
            ".cursor/hooks.json",
            AgentId::Cursor,
            EvidenceKind::Hook,
            EvidenceParser::Json,
            overrides,
        ),
    ];

    #[cfg(target_os = "macos")]
    {
        targets.push(ScanTarget {
            absolute_path: PathBuf::from("/Library/Application Support/Cursor/hooks.json"),
            source_path: "/Library/Application Support/Cursor/hooks.json".to_string(),
            scope: EvidenceScope::Managed,
            precedence: 80,
            agent: AgentId::Cursor,
            kind: EvidenceKind::Hook,
            parser: EvidenceParser::Json,
            sensitivity: "command_config".to_string(),
            content_policy: "structured_safe_fields_only".to_string(),
            directory: false,
            metadata_only: false,
        });
    }

    targets
}

fn scan_cursor_hooks(
    project_path: &Path,
    home_dir: &Path,
    base: &ScannerBase,
) -> Vec<DiscoveredItem> {
    let mut evidence = Vec::new();
    for target in cursor_hook_targets(project_path, home_dir) {
        evidence.extend(scan_cursor_hook_file(&target, base));
    }
    evidence
}

fn scan_cursor_hook_file(target: &ScanTarget, base: &ScannerBase) -> Vec<DiscoveredItem> {
    let text = match fs::read_to_string(&target.absolute_path) {
        Ok(text) => text,
        Err(_) => return Vec::new(),
    };

    let parsed = parse_json(&text);
    let crate::parsers::ParseResult::Err(failure) = parsed else {
        let crate::parsers::ParseResult::Ok(success) = parsed else {
            return Vec::new();
        };
        let hooks = as_object(&success.value)
            .and_then(|root| root.get("hooks"))
            .and_then(as_object);
        let Some(hooks) = hooks else {
            return Vec::new();
        };

        let mut evidence = Vec::new();
        for (event_name, definitions) in hooks {
            let Some(definitions) = definitions.as_array() else {
                continue;
            };
            for (hook_index, definition) in definitions.iter().enumerate() {
                let Some(definition) = as_object(definition) else {
                    continue;
                };
                let hook_type = definition
                    .get("type")
                    .and_then(|v| v.as_str())
                    .unwrap_or("command");
                let command = definition.get("command").and_then(|v| v.as_str());
                let hook_value = cursor_hook_value(definition, hook_type, command);

                let mut item = base.captured(
                    &EvidenceBaseTarget {
                        sensitivity: "command_config".to_string(),
                        content_policy: "structured_safe_fields_only".to_string(),
                        ..EvidenceBaseTarget::from(target)
                    },
                    EvidenceKind::Hook,
                    Some(json!({
                        "executable": hook_type == "command" && command.is_some(),
                        "policyEvaluated": hook_type == "prompt",
                        "eventName": event_name,
                        "hookIndex": hook_index,
                        "hookCategory": hook_category(event_name),
                        "source": cursor_hook_source(target.scope),
                        "sourcePriority": cursor_hook_source_priority(target.scope),
                    })),
                    Some(Value::Object(hook_value)),
                );
                item.id = base.item_id(
                    &ItemIdTarget::from(target),
                    &format!("hook-{event_name}-{hook_index}"),
                );
                item.name = Some(format!("{event_name}.{hook_index}"));
                evidence.push(item);
            }
        }
        return evidence;
    };

    vec![base.parse_failed(
        &EvidenceBaseTarget::from(target),
        EvidenceKind::Hook,
        &failure.error,
    )]
}

fn cursor_hook_value(
    definition: &Map<String, Value>,
    hook_type: &str,
    command: Option<&str>,
) -> Map<String, Value> {
    let mut value = Map::new();
    value.insert("type".to_string(), json!(hook_type));
    if let Some(command) = command {
        value.insert("command".to_string(), json!(command));
    }
    for field in ["timeout", "loop_limit", "failClosed", "matcher"] {
        if let Some(nested) = definition.get(field) {
            value.insert(field.to_string(), nested.clone());
        }
    }
    value
}

fn hook_category(event_name: &str) -> &'static str {
    if matches!(event_name, "beforeTabFileRead" | "afterTabFileEdit") {
        "tab"
    } else if event_name == "workspaceOpen" {
        "app_lifecycle"
    } else {
        "agent"
    }
}

fn cursor_hook_source(scope: EvidenceScope) -> &'static str {
    if scope == EvidenceScope::Managed {
        "enterprise"
    } else {
        scope.as_str()
    }
}

fn cursor_hook_source_priority(scope: EvidenceScope) -> u32 {
    match scope {
        EvidenceScope::Managed => 40,
        EvidenceScope::Project => 30,
        EvidenceScope::User => 10,
        EvidenceScope::Unknown => 0,
    }
}

fn cursor_team_hooks_blind_spot() -> DiscoveredItem {
    DiscoveredItem {
        id: "managed.cursor.cursor-team-hooks.unsupported".to_string(),
        agent: AgentId::Cursor,
        kind: EvidenceKind::Unsupported,
        source_path: "<cursor-team-hooks>".to_string(),
        scope: EvidenceScope::Managed,
        precedence: 70,
        parser: EvidenceParser::Unknown,
        sensitivity: "metadata".to_string(),
        content_policy: "metadata_only".to_string(),
        restore_policy: RestorePolicy::NotSupported,
        capture_status: CaptureStatus::Unsupported,
        confidence: EvidenceConfidence::Medium,
        name: Some("Cursor team hooks".to_string()),
        value: None,
        checksum: None,
        metadata: Some(json!({
            "reason": "cloud_distributed_hooks_not_locally_readable",
            "source": "team",
            "sourcePriority": 35,
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
            let existing = &result[existing_index];
            if item.precedence > existing.precedence {
                let mut duplicate_sources =
                    metadata_string_array(existing.metadata.as_ref().and_then(|m| m.get("duplicateSources")));
                duplicate_sources.insert(0, existing.source_path.clone());
                let mut merged = item;
                if let Some(metadata) = merged.metadata.as_mut() {
                    if let Some(obj) = metadata.as_object_mut() {
                        obj.insert("duplicateSources".to_string(), json!(duplicate_sources));
                    }
                }
                result[existing_index] = merged;
            } else {
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
            }
        } else {
            skill_indexes.insert(name, result.len());
            result.push(item);
        }
    }

    result
}