use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{Path, PathBuf};
use std::time::{Duration, Instant};

use regex::Regex;
use serde_json::{json, Map, Value};

use crate::parsers::parse_toml_scalar;
use crate::policy::MAX_DIRECTORY_ENTRIES;
use crate::types::{
    AgentId, DiscoveredItem, EvidenceKind, EvidenceParser, EvidenceScope,
};

use super::super::base::{as_object, scanner_item_id, EvidenceBaseTarget, ScannerBase};
use super::super::filesystem::scan_targets;
use super::super::{home_target, project_target, ScanTarget, ScanTargetOverrides};
use super::{ScannerContext, ScannerPlugin};

const CODEX_SKILL_MAX_FILES_PER_ROOT: usize = 500;
const CODEX_SKILL_SCAN_BUDGET_MS: u64 = 1000;

pub struct CodexScanner;

impl ScannerPlugin for CodexScanner {
    fn agent_id(&self) -> AgentId {
        AgentId::Codex
    }

    fn agent_name(&self) -> &'static str {
        "Codex"
    }

    fn description(&self) -> &'static str {
        "Codex agent configuration (prompts, config, MCP servers, skills)"
    }

    fn targets(&self, project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
        vec![
            project_target(
                project_path,
                "AGENTS.md",
                AgentId::Codex,
                EvidenceKind::AgentInstruction,
                EvidenceParser::Markdown,
                ScanTargetOverrides::default(),
            ),
            home_target(
                home_dir,
                ".codex/config.toml",
                AgentId::Codex,
                EvidenceKind::AgentConfig,
                EvidenceParser::Toml,
                ScanTargetOverrides::default(),
            ),
        ]
    }

    fn scan(&self, context: &ScannerContext) -> Option<Vec<DiscoveredItem>> {
        let base = ScannerBase::new(AgentId::Codex);
        let in_scope = |target: &ScanTarget| {
            context.scope.is_none() || Some(target.scope) == context.scope
        };

        let mut evidence = scan_targets(
            &self
                .targets(&context.project_path, &context.home_dir)
                .into_iter()
                .filter(|target| in_scope(target))
                .collect::<Vec<_>>(),
        );

        if context.scope.is_none() || context.scope == Some(EvidenceScope::User) {
            evidence.extend(scan_codex_mcp_servers(&context.home_dir, &base));
        }

        evidence.extend(scan_codex_hooks(
            &context.project_path,
            &context.home_dir,
            context.scope,
            &base,
        ));

        let mut skill_evidence = Vec::new();
        for target in codex_skill_targets(&context.home_dir)
            .into_iter()
            .filter(|target| in_scope(target))
        {
            skill_evidence.extend(scan_codex_skill_directory(&target, &base));
        }
        evidence.extend(dedupe_skills_by_source(skill_evidence));
        Some(evidence)
    }
}

fn codex_skill_targets(home_dir: &Path) -> Vec<ScanTarget> {
    vec![
        home_target(
            home_dir,
            ".codex/skills",
            AgentId::Codex,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            ScanTargetOverrides {
                directory: Some(true),
                ..Default::default()
            },
        ),
        home_target(
            home_dir,
            ".codex/plugins/cache",
            AgentId::Codex,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            ScanTargetOverrides {
                directory: Some(true),
                ..Default::default()
            },
        ),
        home_target(
            home_dir,
            ".codex/vendor_imports/skills",
            AgentId::Codex,
            EvidenceKind::Skill,
            EvidenceParser::Filesystem,
            ScanTargetOverrides {
                directory: Some(true),
                ..Default::default()
            },
        ),
    ]
}

fn scan_codex_mcp_servers(home_dir: &Path, base: &ScannerBase) -> Vec<DiscoveredItem> {
    let target = home_target(
        home_dir,
        ".codex/config.toml",
        AgentId::Codex,
        EvidenceKind::AgentConfig,
        EvidenceParser::Toml,
        ScanTargetOverrides::default(),
    );
    let text = match fs::read_to_string(&target.absolute_path) {
        Ok(text) => text,
        Err(_) => return Vec::new(),
    };

    codex_mcp_servers_from_toml(&text)
        .into_iter()
        .map(|(name, server_value)| {
            let evidence_target = EvidenceBaseTarget {
                agent: target.agent,
                source_path: target.source_path.clone(),
                scope: target.scope,
                precedence: target.precedence,
                parser: target.parser,
                sensitivity: "command_config".to_string(),
                content_policy: "structured_safe_fields_only".to_string(),
            };
            let mut item = base.captured(
                &evidence_target,
                EvidenceKind::McpServer,
                None,
                Some(server_value),
            );
            item.id = scanner_item_id(
                target.scope,
                target.agent,
                &target.source_path,
                &format!("mcp-{name}"),
            );
            item.name = Some(name);
            item
        })
        .collect()
}

fn codex_mcp_servers_from_toml(text: &str) -> HashMap<String, Value> {
    let mut servers: HashMap<String, Map<String, Value>> = HashMap::new();
    let mut current_server: Option<String> = None;
    let mut current_nested_path: Vec<String> = Vec::new();

    let lines: Vec<&str> = text.split('\n').collect();
    let mut index = 0usize;
    while index < lines.len() {
        let line = strip_toml_comment(lines[index]).trim().to_string();
        index += 1;
        if line.is_empty() || line.starts_with('#') {
            continue;
        }

        if line.starts_with('[') && line.ends_with(']') {
            let section_path = split_toml_dotted_name(&line[1..line.len() - 1]);
            if section_path.first().map(|s| s.as_str()) == Some("mcp_servers")
                && section_path.len() >= 2
            {
                current_server = Some(section_path[1].clone());
                current_nested_path = section_path[2..].to_vec();
                servers.entry(section_path[1].clone()).or_default();
            } else {
                current_server = None;
                current_nested_path.clear();
            }
            continue;
        }

        let Some(server_name) = current_server.clone() else {
            continue;
        };

        let Some((key, mut raw_value)) = parse_key_value_line(&line) else {
            continue;
        };

        if raw_value.trim().starts_with('[') && !complete_toml_array(&raw_value) {
            let mut array_lines = vec![raw_value];
            while index < lines.len() {
                let continuation = strip_toml_comment(lines[index]).trim().to_string();
                index += 1;
                array_lines.push(continuation);
                if complete_toml_array(&array_lines.join(" ")) {
                    break;
                }
            }
            raw_value = array_lines.join(" ");
        }

        let server = servers.entry(server_name).or_default();
        let path_parts: Vec<String> = current_nested_path
            .iter()
            .chain(split_toml_dotted_name(&key).iter())
            .cloned()
            .collect();
        assign_toml_value(server, &path_parts, &raw_value);
    }

    servers
        .into_iter()
        .map(|(name, map)| (name, Value::Object(map)))
        .collect()
}

fn assign_toml_value(target: &mut Map<String, Value>, path_parts: &[String], raw_value: &str) {
    if path_parts.is_empty() {
        return;
    }

    if path_parts[0] == "env" && path_parts.len() >= 2 {
        let env_keys = target
            .entry("envKeys".to_string())
            .or_insert_with(|| Value::Array(Vec::new()));
        if let Value::Array(keys) = env_keys {
            let key = &path_parts[1];
            if !keys.iter().any(|v| v.as_str() == Some(key)) {
                keys.push(Value::String(key.clone()));
            }
        }
        return;
    }

    let mut cursor = target;
    for part in &path_parts[..path_parts.len() - 1] {
        let entry = cursor
            .entry(part.clone())
            .or_insert_with(|| Value::Object(Map::new()));
        if let Value::Object(map) = entry {
            cursor = map;
        } else {
            return;
        }
    }

    let key = path_parts.last().unwrap();
    let parsed = if secret_like_path(path_parts) {
        Value::String("[redacted]".to_string())
    } else {
        parse_toml_scalar(raw_value)
    };
    cursor.insert(key.clone(), parsed);
}

fn secret_like_path(path_parts: &[String]) -> bool {
    let joined = path_parts.join(".");
    Regex::new(r"(?i)(api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)")
        .map(|re| re.is_match(&joined))
        .unwrap_or(false)
}

fn complete_toml_array(value: &str) -> bool {
    let mut quote: Option<char> = None;
    let mut depth = 0i32;

    for (index, ch) in value.char_indices() {
        let previous = value[..index].chars().last();
        if (ch == '"' || ch == '\'') && quote.is_none() {
            quote = Some(ch);
            continue;
        }
        if Some(ch) == quote && previous != Some('\\') {
            quote = None;
            continue;
        }
        if quote.is_some() {
            continue;
        }
        if ch == '[' {
            depth += 1;
        }
        if ch == ']' {
            depth -= 1;
        }
    }

    depth == 0
}

fn strip_toml_comment(raw_line: &str) -> String {
    let mut quote: Option<char> = None;
    for (index, ch) in raw_line.char_indices() {
        let previous = raw_line[..index].chars().last();
        if (ch == '"' || ch == '\'') && quote.is_none() {
            quote = Some(ch);
            continue;
        }
        if Some(ch) == quote && previous != Some('\\') {
            quote = None;
            continue;
        }
        if ch == '#' && quote.is_none() {
            return raw_line[..index].to_string();
        }
    }
    raw_line.to_string()
}

fn split_toml_dotted_name(name: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut current = String::new();
    let mut quote: Option<char> = None;

    for ch in name.chars() {
        if (ch == '"' || ch == '\'') && quote.is_none() {
            quote = Some(ch);
            continue;
        }
        if Some(ch) == quote {
            quote = None;
            continue;
        }
        if ch == '.' && quote.is_none() {
            parts.push(current.trim().to_string());
            current.clear();
            continue;
        }
        current.push(ch);
    }
    parts.push(current.trim().to_string());
    parts.into_iter().filter(|p| !p.is_empty()).collect()
}

fn parse_key_value_line(line: &str) -> Option<(String, String)> {
    let re = Regex::new(r"^([A-Za-z0-9_.-]+)\s*=\s*(.*)$").ok()?;
    let caps = re.captures(line)?;
    Some((caps[1].to_string(), caps[2].to_string()))
}

fn scan_codex_skill_directory(target: &ScanTarget, base: &ScannerBase) -> Vec<DiscoveredItem> {
    let scan = find_skill_files(&target.absolute_path);
    let mut evidence = Vec::new();

    for skill_file in scan.files {
        let frontmatter = read_skill_frontmatter(&skill_file);
        let skill_dir = skill_file.parent().unwrap_or(&skill_file);
        let relative_skill_dir = skill_dir
            .strip_prefix(&target.absolute_path)
            .map(|p| p.to_string_lossy().replace('\\', "/"))
            .unwrap_or_default();
        let source_path = if relative_skill_dir.is_empty() {
            target.source_path.clone()
        } else {
            format!("{}/{}", target.source_path, relative_skill_dir)
        };
        let directory_name = skill_dir
            .file_name()
            .map(|n| n.to_string_lossy().to_string())
            .unwrap_or_default();
        let name = frontmatter
            .as_ref()
            .and_then(|f| f.name.clone())
            .unwrap_or(directory_name.clone());

        let mut metadata = Map::new();
        metadata.insert("present".to_string(), Value::Bool(true));
        metadata.insert(
            "entrypoint".to_string(),
            Value::String(
                skill_file
                    .file_name()
                    .map(|n| n.to_string_lossy().to_string())
                    .unwrap_or_else(|| "SKILL.md".to_string()),
            ),
        );
        metadata.insert("entrypointStatus".to_string(), Value::String("captured".to_string()));
        metadata.insert("directoryName".to_string(), Value::String(directory_name.clone()));
        metadata.insert(
            "nameMatchesDirectory".to_string(),
            Value::Bool(name == directory_name),
        );

        let mut item = base.captured(
            &EvidenceBaseTarget {
                agent: target.agent,
                source_path: source_path.clone(),
                scope: target.scope,
                precedence: target.precedence,
                parser: target.parser,
                sensitivity: target.sensitivity.clone(),
                content_policy: target.content_policy.clone(),
            },
            EvidenceKind::Skill,
            Some(Value::Object(metadata)),
            None,
        );
        item.id = scanner_item_id(target.scope, target.agent, &source_path, "skill");
        item.name = Some(name);
        evidence.push(item);
    }

    evidence
}

fn codex_hook_targets(project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
    vec![
        project_target(
            project_path,
            ".codex/hooks.json",
            AgentId::Codex,
            EvidenceKind::Hook,
            EvidenceParser::Json,
            ScanTargetOverrides {
                sensitivity: Some("command_config".to_string()),
                content_policy: Some("structured_safe_fields_only".to_string()),
                ..Default::default()
            },
        ),
        home_target(
            home_dir,
            ".codex/hooks.json",
            AgentId::Codex,
            EvidenceKind::Hook,
            EvidenceParser::Json,
            ScanTargetOverrides {
                sensitivity: Some("command_config".to_string()),
                content_policy: Some("structured_safe_fields_only".to_string()),
                ..Default::default()
            },
        ),
    ]
}

fn codex_inline_hook_targets(project_path: &Path, home_dir: &Path) -> Vec<ScanTarget> {
    vec![
        project_target(
            project_path,
            ".codex/config.toml",
            AgentId::Codex,
            EvidenceKind::Hook,
            EvidenceParser::Toml,
            ScanTargetOverrides {
                sensitivity: Some("command_config".to_string()),
                content_policy: Some("structured_safe_fields_only".to_string()),
                ..Default::default()
            },
        ),
        home_target(
            home_dir,
            ".codex/config.toml",
            AgentId::Codex,
            EvidenceKind::Hook,
            EvidenceParser::Toml,
            ScanTargetOverrides {
                sensitivity: Some("command_config".to_string()),
                content_policy: Some("structured_safe_fields_only".to_string()),
                ..Default::default()
            },
        ),
    ]
}

fn scan_codex_hooks(
    project_path: &Path,
    home_dir: &Path,
    scope: Option<EvidenceScope>,
    base: &ScannerBase,
) -> Vec<DiscoveredItem> {
    let in_scope = |target: &ScanTarget| scope.is_none() || Some(target.scope) == scope;
    let mut evidence = Vec::new();

    for target in codex_hook_targets(project_path, home_dir)
        .into_iter()
        .filter(|target| in_scope(target))
    {
        evidence.extend(scan_codex_hooks_file(&target, base));
    }
    for target in codex_inline_hook_targets(project_path, home_dir)
        .into_iter()
        .filter(|target| in_scope(target))
    {
        evidence.extend(scan_codex_inline_hooks_file(&target, base));
    }

    evidence
}

fn scan_codex_hooks_file(target: &ScanTarget, base: &ScannerBase) -> Vec<DiscoveredItem> {
    let text = match fs::read_to_string(&target.absolute_path) {
        Ok(text) => text,
        Err(_) => return Vec::new(),
    };

    let value: Value = match serde_json::from_str(&text) {
        Ok(value) => value,
        Err(error) => {
            return vec![{
                let mut item = base.parse_failed(
                    &EvidenceBaseTarget::from(target),
                    EvidenceKind::Hook,
                    &error.to_string(),
                );
                item.parser = EvidenceParser::Json;
                item
            }];
        }
    };

    codex_hook_items_from_value(target, &value, base)
}

fn scan_codex_inline_hooks_file(target: &ScanTarget, base: &ScannerBase) -> Vec<DiscoveredItem> {
    let text = match fs::read_to_string(&target.absolute_path) {
        Ok(text) => text,
        Err(_) => return Vec::new(),
    };
    codex_inline_hook_items_from_toml(target, &text, base)
}

fn codex_inline_hook_items_from_toml(
    target: &ScanTarget,
    text: &str,
    base: &ScannerBase,
) -> Vec<DiscoveredItem> {
    #[derive(Default)]
    struct HookGroup {
        event_name: String,
        matcher: String,
        hooks: Vec<Map<String, Value>>,
    }

    let mut groups: Vec<HookGroup> = Vec::new();
    let mut current_group: Option<usize> = None;
    let mut current_hook: Option<usize> = None;

    for raw_line in text.split('\n') {
        let line = strip_toml_comment(raw_line);
        let line = line.trim();
        if line.is_empty() {
            continue;
        }

        if let Some(caps) = Regex::new(r"^\[\[([^\]]+)]]$")
            .ok()
            .and_then(|re| re.captures(line))
        {
            let section_path = split_toml_dotted_name(&caps[1]);
            if section_path.len() == 2 && section_path[0] == "hooks" {
                groups.push(HookGroup {
                    event_name: section_path[1].clone(),
                    matcher: "*".to_string(),
                    hooks: Vec::new(),
                });
                current_group = Some(groups.len() - 1);
                current_hook = None;
            } else if section_path.len() == 3
                && section_path[0] == "hooks"
                && section_path[2] == "hooks"
            {
                if current_group.is_none()
                    || groups
                        .get(current_group.unwrap())
                        .map(|g| g.event_name.as_str())
                        != Some(&section_path[1])
                {
                    groups.push(HookGroup {
                        event_name: section_path[1].clone(),
                        matcher: "*".to_string(),
                        hooks: Vec::new(),
                    });
                    current_group = Some(groups.len() - 1);
                }
                if let Some(group_index) = current_group {
                    groups[group_index].hooks.push(Map::new());
                    current_hook = Some(groups[group_index].hooks.len() - 1);
                }
            } else {
                current_group = None;
                current_hook = None;
            }
            continue;
        }

        if Regex::new(r"^\[([^\]]+)]$")
            .ok()
            .and_then(|re| re.captures(line))
            .is_some()
        {
            current_group = None;
            current_hook = None;
            continue;
        }

        let Some((key, raw_value)) = parse_key_value_line(line) else {
            continue;
        };
        let Some(group_index) = current_group else {
            continue;
        };

        let parsed = if secret_like_path(&[key.clone()]) {
            Value::String("[redacted]".to_string())
        } else {
            parse_toml_scalar(&raw_value)
        };

        if let Some(hook_index) = current_hook {
            groups[group_index].hooks[hook_index].insert(key, parsed);
        } else if key == "matcher" {
            if let Value::String(matcher) = parsed {
                groups[group_index].matcher = matcher;
            }
        }
    }

    let mut hooks = Map::new();
    for group in groups {
        let entry = hooks
            .entry(group.event_name.clone())
            .or_insert_with(|| Value::Array(Vec::new()));
        if let Value::Array(items) = entry {
            items.push(json!({
                "matcher": group.matcher,
                "hooks": group.hooks,
            }));
        }
    }

    codex_hook_items_from_value(target, &json!({ "hooks": hooks }), base)
}

fn codex_hook_items_from_value(
    target: &ScanTarget,
    value: &Value,
    base: &ScannerBase,
) -> Vec<DiscoveredItem> {
    let Some(hooks_value) = value.get("hooks").and_then(as_object) else {
        return Vec::new();
    };

    let mut evidence = Vec::new();
    let evidence_target = EvidenceBaseTarget::from(target);

    for (event_name, event_hooks) in hooks_value {
        let Some(event_hooks) = event_hooks.as_array() else {
            continue;
        };
        for (group_index, group_value) in event_hooks.iter().enumerate() {
            let Some(group) = as_object(group_value) else {
                continue;
            };
            let matcher = group
                .get("matcher")
                .and_then(Value::as_str)
                .unwrap_or("*");
            let Some(nested_hooks) = group.get("hooks").and_then(|v| v.as_array()) else {
                continue;
            };

            for (hook_index, hook_value) in nested_hooks.iter().enumerate() {
                let Some(hook) = as_object(hook_value) else {
                    continue;
                };
                let command = hook.get("command").and_then(Value::as_str);
                let hook_type = hook
                    .get("type")
                    .and_then(Value::as_str)
                    .unwrap_or("command");
                let timeout = hook.get("timeout").and_then(Value::as_f64);
                let name = format!("{event_name}.{matcher}");

                let mut hook_payload = Map::new();
                hook_payload.insert("type".to_string(), Value::String(hook_type.to_string()));
                if let Some(command) = command {
                    hook_payload.insert("command".to_string(), Value::String(command.to_string()));
                }
                if let Some(timeout) = timeout {
                    hook_payload.insert("timeout".to_string(), json!(timeout));
                }

                let mut metadata = Map::new();
                metadata.insert(
                    "executable".to_string(),
                    Value::Bool(hook_type == "command" && command.is_some()),
                );
                metadata.insert("eventName".to_string(), Value::String(event_name.clone()));
                metadata.insert("matcher".to_string(), Value::String(matcher.to_string()));
                metadata.insert("hookIndex".to_string(), json!(hook_index));
                metadata.insert("groupIndex".to_string(), json!(group_index));
                metadata.insert(
                    "source".to_string(),
                    Value::String(if target.scope == EvidenceScope::Managed {
                        "plugin".to_string()
                    } else {
                        target.scope.as_str().to_string()
                    }),
                );

                let mut item = base.captured(
                    &evidence_target,
                    EvidenceKind::Hook,
                    Some(Value::Object(metadata)),
                    Some(Value::Object(hook_payload)),
                );
                item.id = scanner_item_id(
                    target.scope,
                    target.agent,
                    &target.source_path,
                    &format!("hook-{event_name}-{group_index}-{hook_index}"),
                );
                item.name = Some(name);
                item.parser = if target.parser == EvidenceParser::Toml {
                    EvidenceParser::Toml
                } else {
                    EvidenceParser::Json
                };
                evidence.push(item);
            }
        }
    }

    evidence
}

struct SkillFrontmatter {
    name: Option<String>,
}

struct SkillFileScan {
    files: Vec<PathBuf>,
}

fn find_skill_files(root: &Path) -> SkillFileScan {
    let mut files = Vec::new();
    let deadline = Instant::now() + Duration::from_millis(CODEX_SKILL_SCAN_BUDGET_MS);
    walk_skill_files(root, &mut files, 0, &mut HashSet::new(), deadline);
    SkillFileScan { files }
}

fn walk_skill_files(
    dir: &Path,
    files: &mut Vec<PathBuf>,
    depth: u32,
    seen: &mut HashSet<PathBuf>,
    deadline: Instant,
) {
    if depth > 8 || Instant::now() > deadline || files.len() >= CODEX_SKILL_MAX_FILES_PER_ROOT {
        return;
    }

    let resolved = match fs::canonicalize(dir) {
        Ok(path) => path,
        Err(_) => return,
    };
    if !seen.insert(resolved) {
        return;
    }

    let entries = match fs::read_dir(dir) {
        Ok(entries) => entries,
        Err(_) => return,
    };

    let mut collected = entries.filter_map(Result::ok).collect::<Vec<_>>();
    if collected.len() > MAX_DIRECTORY_ENTRIES as usize {
        collected.truncate(MAX_DIRECTORY_ENTRIES as usize);
    }

    for entry in collected {
        if Instant::now() > deadline || files.len() >= CODEX_SKILL_MAX_FILES_PER_ROOT {
            return;
        }

        let path = entry.path();
        let metadata = match entry.metadata() {
            Ok(m) => m,
            Err(_) => continue,
        };

        if metadata.is_dir() {
            walk_skill_files(&path, files, depth + 1, seen, deadline);
        } else if metadata.is_file()
            && entry
                .file_name()
                .to_string_lossy()
                .eq_ignore_ascii_case("skill.md")
        {
            files.push(path);
        }
    }
}

fn read_skill_frontmatter(file_path: &Path) -> Option<SkillFrontmatter> {
    let text = fs::read_to_string(file_path).ok()?;
    let re = Regex::new(r"^---\r?\n([\s\S]*?)\r?\n---").ok()?;
    let caps = re.captures(&text)?;
    let mut frontmatter = SkillFrontmatter { name: None };
    for line in caps[1].split('\n') {
        if let Some(caps) = Regex::new(r"^(name|description):\s*(.*)$")
            .ok()?
            .captures(line.trim())
        {
            if &caps[1] == "name" {
                frontmatter.name = Some(
                    caps[2]
                        .trim()
                        .trim_start_matches(['\'', '"'])
                        .trim_end_matches(['\'', '"'])
                        .to_string(),
                );
            }
        }
    }
    Some(frontmatter)
}

fn dedupe_skills_by_source(evidence: Vec<DiscoveredItem>) -> Vec<DiscoveredItem> {
    let mut seen = HashSet::new();
    let mut result = Vec::new();
    for item in evidence {
        if seen.insert(item.source_path.clone()) {
            result.push(item);
        }
    }
    result
}