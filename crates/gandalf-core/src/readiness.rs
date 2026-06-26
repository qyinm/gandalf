use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};

use regex::Regex;
use crate::types::{
    DiscoveredItem, EvidenceKind, McpBinaryInfo, McpBinaryKind, McpBinaryReport, ReadinessAction,
    ReadinessCategory, ReadinessItem, ReadinessReport, Severity,
};

const READINESS_CATEGORIES: [ReadinessCategory; 6] = [
    ReadinessCategory::Ready,
    ReadinessCategory::NeedsManualAction,
    ReadinessCategory::Warning,
    ReadinessCategory::Unverified,
    ReadinessCategory::Unsupported,
    ReadinessCategory::Blocked,
];

#[derive(Debug, Clone, Default)]
pub struct ReadinessOptions<'a> {
    pub source_home_dir: Option<&'a str>,
    pub target_platform: Option<&'a str>,
    pub apply_content: bool,
    pub target_evidence: Option<&'a [DiscoveredItem]>,
    pub process_env: Option<&'a HashMap<String, String>>,
    pub path_env: Option<&'a str>,
}

pub fn current_platform() -> String {
    match std::env::consts::OS {
        "macos" => "darwin".to_string(),
        "windows" => "win32".to_string(),
        other => other.to_string(),
    }
}

pub fn classify_mcp_binary(command: &str, home_dir: Option<&str>) -> McpBinaryKind {
    if command == "npx" || command == "uvx" {
        return McpBinaryKind::PackageRunner;
    }
    if Path::new(command).is_absolute() {
        if home_dir.is_some_and(|home| is_strictly_under(command, home)) {
            return McpBinaryKind::SourceLocalPath;
        }
        return McpBinaryKind::PathBinary;
    }
    McpBinaryKind::Command
}

pub fn extract_mcp_binaries(
    evidence: &[DiscoveredItem],
    source_home_dir: Option<&str>,
) -> Vec<McpBinaryInfo> {
    let mut binaries = Vec::new();
    for item in evidence {
        if item.kind != EvidenceKind::McpServer {
            continue;
        }
        let value = item.value.as_ref();
        let command = value
            .and_then(|v| v.get("command"))
            .and_then(|v| v.as_str());
        let url = value.and_then(|v| v.get("url")).and_then(|v| v.as_str());

        if command.is_some() || url.is_some() {
            let args = value
                .and_then(|v| v.get("args"))
                .and_then(|v| v.as_array())
                .map(|items| {
                    items
                        .iter()
                        .filter_map(|v| v.as_str().map(str::to_string))
                        .collect::<Vec<_>>()
                })
                .filter(|args| !args.is_empty());
            let safe_url = url.map(sanitize_remote_url);
            let command_text = command
                .map(str::to_string)
                .or_else(|| safe_url.clone())
                .unwrap_or_else(|| "unknown".to_string());
            binaries.push(McpBinaryInfo {
                evidence_id: item.id.clone(),
                command: command_text,
                args,
                url: safe_url,
                binary_kind: Some(if url.is_some() {
                    McpBinaryKind::RemoteUrl
                } else {
                    classify_mcp_binary(command.unwrap_or(""), source_home_dir)
                }),
            });
        }
    }
    binaries
}

pub fn check_mcp_binary_availability(source_binaries: &[McpBinaryInfo]) -> Vec<McpBinaryReport> {
    let mut reports = Vec::new();
    for bin in source_binaries {
        if bin.url.is_some() {
            reports.push(McpBinaryReport {
                evidence_id: bin.evidence_id.clone(),
                command: bin.url.clone().unwrap_or_else(|| bin.command.clone()),
                available_on_target: true,
                binary_kind: Some(McpBinaryKind::RemoteUrl),
                resolved_path: None,
                warning: Some(
                    "Remote URL — availability cannot be verified locally".to_string(),
                ),
            });
            continue;
        }

        if bin.binary_kind == Some(McpBinaryKind::SourceLocalPath) {
            reports.push(McpBinaryReport {
                evidence_id: bin.evidence_id.clone(),
                command: bin.command.clone(),
                available_on_target: false,
                binary_kind: bin.binary_kind,
                resolved_path: None,
                warning: Some(format!(
                    "MCP command points to a source machine local binary path ({}); install or remap it on this machine.",
                    bin.command
                )),
            });
            continue;
        }

        let resolved = find_executable_on_path(&bin.command, None);
        let available = !resolved.is_empty();
        let warning = match bin.binary_kind {
                Some(McpBinaryKind::PackageRunner) if available => Some(format!(
                    "Package runner {} is available at {}; package arguments may still differ on this machine.",
                    bin.command, resolved
                )),
                Some(McpBinaryKind::PackageRunner) => Some(format!(
                    "Package runner {} not found on this machine; MCP package cannot be launched.",
                    bin.command
                )),
                _ if !available => Some(format!(
                    "Binary \"{}\" not found on this machine",
                    bin.command
                )),
                _ => None,
            };
        reports.push(McpBinaryReport {
            evidence_id: bin.evidence_id.clone(),
            command: bin.command.clone(),
            available_on_target: available,
            binary_kind: bin.binary_kind,
            resolved_path: if resolved.is_empty() {
                None
            } else {
                Some(resolved)
            },
            warning,
        });
    }
    reports
}

pub fn build_readiness_report(
    source_evidence: &[DiscoveredItem],
    options: &ReadinessOptions<'_>,
) -> ReadinessReport {
    let default_platform = current_platform();
    let target_platform = options
        .target_platform
        .unwrap_or(default_platform.as_str());
    let source_binaries = extract_mcp_binaries(source_evidence, options.source_home_dir);
    let mcp_reports = check_mcp_binary_availability(&source_binaries);
    let mut items = Vec::new();

    if target_platform != "darwin" {
        items.push(ReadinessItem {
            id: "platform.apply-content-macos-only".to_string(),
            category: if options.apply_content {
                ReadinessCategory::Blocked
            } else {
                ReadinessCategory::Unsupported
            },
            severity: if options.apply_content {
                Severity::High
            } else {
                Severity::Medium
            },
            code: "GANDALF_MACOS_APPLY_ONLY".to_string(),
            problem: "Bundle content apply is Mac-only in this release.".to_string(),
            cause: format!("Target platform is {target_platform}."),
            fix: if options.apply_content {
                "Run dry-run or inspect here, or apply the bundle on macOS.".to_string()
            } else {
                "Dry-run and inspect remain available here; apply the bundle on macOS.".to_string()
            },
            path: None,
            evidence_id: None,
            command: None,
            actions: None,
        });
    }

    for report in mcp_reports {
        items.push(readiness_item_for_mcp_report(&report));
    }

    let target_env_keys = env_key_set(options.target_evidence.unwrap_or(&[]), false);
    let source_env_keys = env_key_set(source_evidence, true);
    let process_env = options.process_env;
    let mut sorted_keys: Vec<_> = source_env_keys.iter().cloned().collect();
    sorted_keys.sort();
    for key in sorted_keys {
        if target_env_keys.contains(&key)
            || process_env.is_some_and(|env| env.contains_key(&key))
        {
            continue;
        }
        items.push(ReadinessItem {
            id: format!("env.{key}"),
            category: ReadinessCategory::NeedsManualAction,
            severity: Severity::Medium,
            code: "GANDALF_ENV_VALUE_REQUIRED".to_string(),
            problem: format!("Environment key {key} needs a value on this machine."),
            cause: "The bundle records the key name only; raw env values are omitted by policy."
                .to_string(),
            fix: "Add the value manually or through your preferred secret manager before running tools that need it.".to_string(),
            path: Some(".env".to_string()),
            evidence_id: None,
            command: None,
            actions: Some(vec![ReadinessAction {
                label: "Set env value manually".to_string(),
                command: None,
                url: None,
            }]),
        });
    }

    ReadinessReport {
        target_platform: target_platform.to_string(),
        summary: summarize(&items),
        items,
    }
}

pub fn readiness_item_for_mcp_report(report: &McpBinaryReport) -> ReadinessItem {
    if report.binary_kind == Some(McpBinaryKind::RemoteUrl) {
        return ReadinessItem {
            id: format!("mcp.{}.remote", report.evidence_id),
            category: ReadinessCategory::Unverified,
            severity: Severity::Low,
            code: "GANDALF_REMOTE_MCP_UNVERIFIED".to_string(),
            problem: "Remote MCP URL cannot be verified locally.".to_string(),
            cause: report
                .warning
                .clone()
                .unwrap_or_else(|| {
                    "Remote MCP availability depends on network and provider state.".to_string()
                }),
            fix: "Verify the remote endpoint outside gandalf if this MCP server is required."
                .to_string(),
            evidence_id: Some(report.evidence_id.clone()),
            command: Some(report.command.clone()),
            path: None,
            actions: None,
        };
    }

    if report.available_on_target {
        return ReadinessItem {
            id: format!("mcp.{}.available", report.evidence_id),
            category: ReadinessCategory::Ready,
            severity: Severity::None,
            code: "GANDALF_MCP_COMMAND_AVAILABLE".to_string(),
            problem: format!("MCP command {} is available.", report.command),
            cause: report
                .resolved_path
                .as_ref()
                .map(|path| format!("Resolved to {path}."))
                .unwrap_or_else(|| "The command is available on PATH.".to_string()),
            fix: "No action needed.".to_string(),
            evidence_id: Some(report.evidence_id.clone()),
            command: Some(report.command.clone()),
            path: None,
            actions: None,
        };
    }

    if report.binary_kind == Some(McpBinaryKind::SourceLocalPath) {
        return ReadinessItem {
            id: format!("mcp.{}.source-local-path", report.evidence_id),
            category: ReadinessCategory::NeedsManualAction,
            severity: Severity::Medium,
            code: "GANDALF_SOURCE_LOCAL_MCP_PATH".to_string(),
            problem: "MCP command points to a source-machine local path.".to_string(),
            cause: report.warning.clone().unwrap_or_else(|| {
                format!("The source command path is {}.", report.command)
            }),
            fix: "Install the MCP server on this Mac and update the command path if needed."
                .to_string(),
            evidence_id: Some(report.evidence_id.clone()),
            command: Some(report.command.clone()),
            path: None,
            actions: Some(vec![ReadinessAction {
                label: "Install or remap local MCP binary".to_string(),
                command: None,
                url: None,
            }]),
        };
    }

    ReadinessItem {
        id: format!("mcp.{}.missing", report.evidence_id),
        category: ReadinessCategory::NeedsManualAction,
        severity: Severity::Medium,
        code: "GANDALF_MCP_COMMAND_MISSING".to_string(),
        problem: format!("MCP command {} is missing on this machine.", report.command),
        cause: report.warning.clone().unwrap_or_else(|| {
            format!("The command {} was not found on PATH.", report.command)
        }),
        fix: install_hint_for_command(&report.command, report.binary_kind),
        evidence_id: Some(report.evidence_id.clone()),
        command: Some(report.command.clone()),
        path: None,
        actions: Some(install_actions_for_command(
            &report.command,
            report.binary_kind,
        )),
    }
}

#[derive(Debug, Clone, Default)]
pub struct ReadinessFormatOptions {
    pub max_items: Option<usize>,
    pub include_fixes: bool,
    pub include_actions: bool,
}

pub fn format_readiness_summary_lines(
    report: &ReadinessReport,
    options: &ReadinessFormatOptions,
) -> Vec<String> {
    let max_items = options.max_items.unwrap_or(5);
    let mut lines = vec![
        "Readiness:".to_string(),
        format!("  ready: {}", report.summary.get(&ReadinessCategory::Ready).copied().unwrap_or(0)),
        format!(
            "  needs manual action: {}",
            report
                .summary
                .get(&ReadinessCategory::NeedsManualAction)
                .copied()
                .unwrap_or(0)
        ),
        format!(
            "  warnings: {}",
            report
                .summary
                .get(&ReadinessCategory::Warning)
                .copied()
                .unwrap_or(0)
        ),
        format!(
            "  unverified: {}",
            report
                .summary
                .get(&ReadinessCategory::Unverified)
                .copied()
                .unwrap_or(0)
        ),
        format!(
            "  unsupported: {}",
            report
                .summary
                .get(&ReadinessCategory::Unsupported)
                .copied()
                .unwrap_or(0)
        ),
        format!(
            "  blocked: {}",
            report
                .summary
                .get(&ReadinessCategory::Blocked)
                .copied()
                .unwrap_or(0)
        ),
    ];

    let actionable: Vec<_> = report
        .items
        .iter()
        .filter(|item| {
            matches!(
                item.category,
                ReadinessCategory::Blocked | ReadinessCategory::NeedsManualAction
            )
        })
        .collect();

    for item in actionable.iter().take(max_items) {
        lines.push(format!("  - {}", item.problem));
        if options.include_fixes {
            lines.push(format!("    fix: {}", item.fix));
        }
        if options.include_actions {
            if let Some(actions) = &item.actions {
                for action in actions {
                    lines.push(format!(
                        "    action: {}",
                        action.command.as_deref().unwrap_or(&action.label)
                    ));
                }
            }
        }
    }
    if actionable.len() > max_items {
        lines.push(format!(
            "  ... and {} more action item(s)",
            actionable.len() - max_items
        ));
    }

    lines
}

fn env_key_set(evidence: &[DiscoveredItem], include_mcp_env_keys: bool) -> HashSet<String> {
    let mut keys = HashSet::new();
    for item in evidence {
        if item.kind == EvidenceKind::EnvKey {
            let key = item
                .name
                .clone()
                .or_else(|| {
                    item.value
                        .as_ref()
                        .and_then(|v| v.get("key"))
                        .and_then(|v| v.as_str())
                        .map(str::to_string)
                });
            if let Some(key) = key {
                keys.insert(key);
            }
        }
        if include_mcp_env_keys && item.kind == EvidenceKind::McpServer {
            if let Some(env_keys) = item
                .value
                .as_ref()
                .and_then(|v| v.get("envKeys"))
                .and_then(|v| v.as_array())
            {
                for key in env_keys {
                    if let Some(key) = key.as_str() {
                        keys.insert(key.to_string());
                    }
                }
            }
        }
    }
    keys
}

fn summarize(items: &[ReadinessItem]) -> HashMap<ReadinessCategory, u32> {
    let mut summary = HashMap::new();
    for category in READINESS_CATEGORIES {
        summary.insert(category, 0);
    }
    for item in items {
        *summary.entry(item.category).or_insert(0) += 1;
    }
    summary
}

fn install_hint_for_command(command: &str, kind: Option<McpBinaryKind>) -> String {
    if command == "npx" {
        return "Install Node.js on this Mac, then rerun the dry-run.".to_string();
    }
    if command == "uvx" {
        return "Install uv on this Mac, then rerun the dry-run.".to_string();
    }
    if command == "gh" {
        return "Install GitHub CLI on this Mac and authenticate it if the MCP server needs GitHub access.".to_string();
    }
    if kind == Some(McpBinaryKind::PackageRunner) {
        return format!(
            "Install package runner {command} on this Mac, then rerun the dry-run."
        );
    }
    format!(
        "Install {command} on this Mac or update the MCP command to a local path that exists."
    )
}

fn install_actions_for_command(command: &str, kind: Option<McpBinaryKind>) -> Vec<ReadinessAction> {
    if command == "npx" {
        return vec![ReadinessAction {
            label: "Install Node.js".to_string(),
            command: Some("brew install node".to_string()),
            url: None,
        }];
    }
    if command == "uvx" {
        return vec![ReadinessAction {
            label: "Install uv".to_string(),
            command: Some("brew install uv".to_string()),
            url: None,
        }];
    }
    if command == "gh" {
        return vec![ReadinessAction {
            label: "Install GitHub CLI".to_string(),
            command: Some("brew install gh".to_string()),
            url: None,
        }];
    }
    if kind == Some(McpBinaryKind::PackageRunner) {
        return vec![ReadinessAction {
            label: format!("Install {command}"),
            command: None,
            url: None,
        }];
    }
    vec![ReadinessAction {
        label: format!("Install {command}"),
        command: None,
        url: None,
    }]
}

fn find_executable_on_path(command: &str, path_env: Option<&str>) -> String {
    let path_env = path_env
        .map(str::to_string)
        .or_else(|| std::env::var("PATH").ok())
        .unwrap_or_default();

    if Path::new(command).is_absolute() {
        return if executable_path(command) {
            command.to_string()
        } else {
            String::new()
        };
    }

    for dir in path_env.split(std::path::MAIN_SEPARATOR) {
        if dir.is_empty() {
            continue;
        }
        let candidate = Path::new(dir).join(command);
        if executable_path(&candidate.to_string_lossy()) {
            return candidate.to_string_lossy().to_string();
        }
    }
    String::new()
}

fn executable_path(candidate: &str) -> bool {
    let path = Path::new(candidate);
    if !path.is_file() {
        return false;
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        return fs::metadata(path)
            .map(|metadata| metadata.permissions().mode() & 0o111 != 0)
            .unwrap_or(false);
    }
    #[cfg(not(unix))]
    {
        path.is_file()
    }
}

use std::fs;

fn sanitize_remote_url(raw_url: &str) -> String {
    if let Ok(mut url) = url::parse_url(raw_url) {
        url.username.clear();
        url.password.clear();
        url.fragment = None;
        let sensitive = Regex::new(
            r"(?i)(api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)",
        )
        .expect("regex");
        let keys: Vec<_> = url
            .query
            .keys()
            .filter(|key| sensitive.is_match(key))
            .cloned()
            .collect();
        for key in keys {
            url.query.insert(key, "[redacted]".to_string());
        }
        return url.to_string();
    }
    "[remote-url]".to_string()
}

mod url {
    use std::collections::HashMap;

    #[derive(Debug, Clone)]
    pub struct ParsedUrl {
        pub scheme: String,
        pub username: String,
        pub password: String,
        pub host: String,
        pub path: String,
        pub query: HashMap<String, String>,
        pub fragment: Option<String>,
    }

    impl ParsedUrl {
        pub fn query_pairs(&self) -> impl Iterator<Item = (String, String)> + '_ {
            self.query
                .iter()
                .map(|(k, v)| (k.clone(), v.clone()))
        }

        pub fn query_pairs_mut(&mut self) -> QueryPairsMut<'_> {
            QueryPairsMut { query: &mut self.query }
        }

        pub fn to_string(&self) -> String {
            let mut out = format!("{}://", self.scheme);
            if !self.username.is_empty() || !self.password.is_empty() {
                out.push_str(&self.username);
                if !self.password.is_empty() {
                    out.push(':');
                    out.push_str(&self.password);
                }
                out.push('@');
            }
            out.push_str(&self.host);
            out.push_str(&self.path);
            if !self.query.is_empty() {
                out.push('?');
                let pairs: Vec<_> = self
                    .query
                    .iter()
                    .map(|(k, v)| format!("{}={}", percent_encode(k), percent_encode(v)))
                    .collect();
                out.push_str(&pairs.join("&"));
            }
            if let Some(fragment) = &self.fragment {
                out.push('#');
                out.push_str(fragment);
            }
            out
        }
    }

    pub struct QueryPairsMut<'a> {
        query: &'a mut HashMap<String, String>,
    }

    impl QueryPairsMut<'_> {
        pub fn clear_pair(&mut self, key: &str) {
            self.query.remove(key);
        }

        pub fn append_pair(&mut self, key: &str, value: &str) {
            self.query.insert(key.to_string(), value.to_string());
        }
    }

    pub fn parse_url(raw: &str) -> Result<ParsedUrl, ()> {
        let (scheme, rest) = raw.split_once("://").ok_or(())?;
        let (authority_and_path, fragment) = rest
            .split_once('#')
            .map(|(left, right)| (left, Some(right.to_string())))
            .unwrap_or((rest, None));
        let (authority_and_path, query) = authority_and_path
            .split_once('?')
            .map(|(left, right)| (left, parse_query(right)))
            .unwrap_or((authority_and_path, HashMap::new()));

        let (authority, path) = authority_and_path
            .split_once('/')
            .map(|(left, right)| (left, format!("/{right}")))
            .unwrap_or((authority_and_path, String::new()));

        let (username, host) = authority
            .rsplit_once('@')
            .map(|(creds, host)| (creds, host))
            .unwrap_or(("", authority));
        let (username, password) = username
            .split_once(':')
            .map(|(u, p)| (u.to_string(), p.to_string()))
            .unwrap_or_else(|| (username.to_string(), String::new()));

        Ok(ParsedUrl {
            scheme: scheme.to_string(),
            username,
            password,
            host: host.to_string(),
            path,
            query,
            fragment,
        })
    }

    fn parse_query(query: &str) -> HashMap<String, String> {
        query
            .split('&')
            .filter_map(|pair| {
                let (key, value) = pair.split_once('=')?;
                Some((key.to_string(), value.to_string()))
            })
            .collect()
    }

    fn percent_encode(value: &str) -> String {
        value
            .bytes()
            .map(|byte| match byte {
                b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                    (byte as char).to_string()
                }
                _ => format!("%{byte:02X}"),
            })
            .collect()
    }
}

fn is_strictly_under(resolved: &str, root: &str) -> bool {
    let normalized = PathBuf::from(root);
    let resolved = PathBuf::from(resolved);
    if resolved == normalized {
        return true;
    }
    let mut prefix = normalized;
    prefix.push("");
    resolved.starts_with(prefix)
}